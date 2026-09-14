package sitec

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func findProjectRoot(startDir string) (string, error) {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found in %s or parent directories", startDir)
		}
		dir = parent
	}
}

func containsModule(mods []module, m module) bool {
	for _, x := range mods {
		if x.path == m.path && x.dir == m.dir {
			return true
		}
	}
	return false
}

func expandToSSRPackages(modules []module, scanner *scanner, assetLibraries []string) []module {
	var out []module
	seen := make(map[string]bool)

	for _, m := range modules {
		if m.dir == "" {
			if !seen[m.path] {
				seen[m.path] = true
				out = append(out, m)
			}
			continue
		}

		filepath.WalkDir(m.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != m.dir {
				name := d.Name()
				if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
					name == "vendor" || name == "testdata" || name == "node_modules" {
					return filepath.SkipDir
				}
				// A nested go.mod is its own module: it comes from the finder, not from here.
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}

			feats, err := scanner.scanPackage(path)
			if err != nil {
				return nil
			}

			// Un package main no se puede importar, así que nunca puede aportar assets.
			if feats.PkgName == mainPackageName {
				return nil // no seleccionar; seguir bajando a subdirectorios
			}

			hasProducers := len(feats.Producers) > 0

			var hasCSSGo bool
			if _, err := os.Stat(filepath.Join(path, cssSourceFile)); err == nil {
				hasCSSGo = true
			}

			var importedLib string
			if !hasProducers {
				for imp := range feats.Imports {
					for _, lib := range assetLibraries {
						if imp == lib || strings.HasSuffix(imp, "/"+lib) {
							importedLib = imp
							break
						}
					}
					if importedLib != "" {
						break
					}
				}
			}

			if hasProducers || hasCSSGo || importedLib != "" {
				pkgPath := m.path
				if rel, err := filepath.Rel(m.dir, path); err == nil && rel != "." {
					pkgPath = m.path + "/" + filepath.ToSlash(rel)
				}
				if !seen[pkgPath] {
					seen[pkgPath] = true
					out = append(out, module{path: pkgPath, dir: path})
				}
			}
			return nil
		})
	}
	return out
}

// aliasFor returns a Go import alias for path, unique against every path
// already recorded in used (a path -> alias map built incrementally as the
// caller processes each module). It starts from the last path segment
// (today's behavior) and, on collision with a DIFFERENT path, walks one more
// segment toward the module root at a time until the alias is unique.
//
// A collision is only ever between two DIFFERENT full paths: the caller
// dedupes on path before this is reached, so the same path is never
// re-processed here.
func aliasFor(path string, used map[string]string) string {
	parts := strings.Split(path, "/")
	depth := 1
	replacer := strings.NewReplacer("-", "_", ".", "_")
	for {
		start := len(parts) - depth
		if start < 0 {
			start = 0
		}
		candidate := aliasPrefix + replacer.Replace(strings.Join(parts[start:], "_"))
		if existingPath, ok := used[candidate]; !ok || existingPath == path {
			return candidate
		}
		if start == 0 {
			// Ran out of segments — both full paths, sanitized the same way,
			// produce the same string. This can only happen if the two paths
			// are identical once "-" is normalized to "_", which the caller's
			// own path-level dedup already rules out. Returning candidate
			// here (rather than panicking) keeps this function total; the
			// caller's own bookkeeping still records the true path for both,
			// so a second real collision surfaces as a Go compile error
			// exactly as visible as today's bug, never a silent one.
			return candidate
		}
		depth++
	}
}

func modulesToAliases(modules []module, scanner *scanner, assetLibraries []string, rootDir string, lister GraphLister, log func(...any), verbose bool) ([]moduleAlias, error) {
	var reachLog func(...any)
	if verbose {
		reachLog = log
	}
	reach := computeReachability(rootDir, lister, reachLog)

	var skipped []string
	var aliases []moduleAlias
	aliasByCandidate := make(map[string]string)
	for _, m := range expandToSSRPackages(modules, scanner, assetLibraries) {
		// reach.partial: at least one build target's probe failed, so the
		// reachable set is incomplete. Filtering on incomplete data risks
		// excluding a package that IS reachable — silently dropping its
		// styles, exactly the failure this whole mechanism exists to catch.
		// Skip filtering entirely for this pass; the next scan retries with
		// (usually) a warm module cache.
		if reach.known && !reach.partial && !reach.set[m.path] {
			skipped = append(skipped, m.path)
			continue
		}

		alias := aliasFor(m.path, aliasByCandidate)
		aliasByCandidate[alias] = m.path

		ma := moduleAlias{
			Path:  m.path,
			Alias: alias,
		}

		if m.dir != "" {
			feats, err := scanner.scanPackage(m.dir)
			if err != nil {
				return nil, err
			}

			groups := make(map[string]*receiverFeature)
			for _, prod := range feats.Producers {
				if prod.IsGeneric {
					return nil, fmt.Errorf("ssr: package %s declares producer %s on generic type %s[…]; generic receivers cannot be instantiated as a zero value — use a concrete type", m.path, prod.Name+"()", prod.ReceiverType)
				}
				rf, ok := groups[prod.ReceiverType]
				if !ok {
					rf = &receiverFeature{Name: prod.ReceiverType}
					groups[prod.ReceiverType] = rf
				}
				switch prod.Name {
				case "RootCSS":
					rf.HasRoot = true
				case "RenderCSS":
					rf.HasRender = true
				case "RenderHTML":
					rf.HasHTML = true
				case "RenderJS":
					rf.HasJS = true
				case "IconSvg":
					rf.HasIcons = true
				case "Fonts":
					rf.HasFonts = true
				case "RenderPages":
					rf.HasPages = true
				case "RenderSite":
					rf.HasSite = true
				case "Favicon":
					rf.HasFavicon = true
				}
			}

			var receivers []receiverFeature
			for _, rf := range groups {
				receivers = append(receivers, *rf)
			}
			sort.Slice(receivers, func(i, j int) bool {
				return receivers[i].Name < receivers[j].Name
			})

			if len(receivers) == 0 {
				absRoot, _ := filepath.Abs(rootDir)
				isLocal := absRoot != "" && strings.HasPrefix(m.dir, absRoot)
				if isLocal {
					if _, err := os.Stat(filepath.Join(m.dir, cssSourceFile)); err == nil {
						return nil, fmt.Errorf("ssr: package %s has %s but declares no RootCSS() or RenderCSS(); expected: func (w *T) RenderCSS() *css.Stylesheet", m.path, cssSourceFile)
					}

					var importedLib string
					for imp := range feats.Imports {
						for _, lib := range assetLibraries {
							if imp == lib || strings.HasSuffix(imp, "/"+lib) {
								importedLib = imp
								break
							}
						}
						if importedLib != "" {
							break
						}
					}
					if importedLib != "" {
						return nil, fmt.Errorf("ssr: package %s imports %s but declares no producer; expected: func (w *T) RenderCSS() *css.Stylesheet", m.path, importedLib)
					}
				}
			}

			ma.Receivers = receivers
		}

		aliases = append(aliases, ma)
	}

	if log != nil && verbose && len(skipped) > 0 {
		log(fmt.Sprintf(skippedUnreachableSummaryFmt, len(skipped), strings.Join(skipped, ", ")))
	}

	return aliases, nil
}
