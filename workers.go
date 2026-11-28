package sitec

import (
	"go/build/constraint"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"webtyp.com/fmt"
	"webtyp.com/js"
)

// WorkersDir holds the project's own Web Workers: every web/webworkers/<name>/main.go is one.
const WorkersDir = "web/webworkers"

// LibraryWorkerDir is the folder where a library ships its Web Worker binary:
// webtyp.com/pdf has pdf/webworker/main.go and becomes /pdf.wasm + /pdf.worker.js in every
// app whose build reaches webtyp.com/pdf.
const LibraryWorkerDir = "webworker"

var workerNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

const mediaTypeWasm = "application/wasm"

// WebWorker is one Web Worker the project ships: Name gives /<Name>.wasm and
// /<Name>.worker.js; Package is what gets compiled ("./web/webworkers/<name>" or
// "<library>/webworker"), with the Runtime its main.go declares (WorkerRuntime).
type WebWorker struct {
	Name    string
	Package string
	Runtime js.Runtime
}

// tinyGoTag is the build tag TinyGo sets on every build: a Worker's main.go declares its
// compiler with it.
const tinyGoTag = "tinygo"

// msgWorkerRuntimeFmt asks for the one line that declares the compiler.
const msgWorkerRuntimeFmt = "sitec: %s must declare its compiler on its first line: " +
	"//go:build wasm && tinygo (small binaries, SIMD) or //go:build wasm && !tinygo (Go's full standard library)"

// WorkerRuntime reads the compiler a Worker's main.go declares in its //go:build line: a line
// that only TinyGo satisfies is js.RuntimeTinyGo, one only Go satisfies is js.RuntimeGo; no line,
// or one both satisfy, is an error showing the two lines to choose from.
//
// Example: rt, err := sitec.WorkerRuntime("web/webworkers/model/main.go")
func WorkerRuntime(mainPath string) (js.Runtime, error) {
	src, err := os.ReadFile(mainPath)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if !constraint.IsGoBuild(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			return 0, err
		}
		withTinyGo := expr.Eval(func(tag string) bool { return tag == "wasm" || tag == "js" || tag == tinyGoTag })
		withGo := expr.Eval(func(tag string) bool { return tag == "wasm" || tag == "js" })
		switch {
		case withTinyGo && !withGo:
			return js.RuntimeTinyGo, nil
		case withGo && !withTinyGo:
			return js.RuntimeGo, nil
		}
		break
	}
	return 0, fmt.Errf(msgWorkerRuntimeFmt, mainPath)
}

// FindWebWorkers returns the project's Web Workers: its own (web/webworkers/<name>/main.go) and
// those of every library its build reaches that has a webworker/main.go. Two Workers with the
// same name are an error that names both.
//
// Example: workers, err := sitec.FindWebWorkers(root, sitec.NewExecToolchain())
func FindWebWorkers(root string, tc Toolchain) ([]WebWorker, error) {
	own, err := projectWorkers(root)
	if err != nil {
		return nil, err
	}
	libs, err := libraryWorkers(root, tc)
	if err != nil {
		return nil, err
	}
	all := append(own, libs...)
	seen := make(map[string]string, len(all))
	for _, w := range all {
		if other, dup := seen[w.Name]; dup {
			return nil, fmt.Errf(msgDuplicateWorkerFmt, w.Name, other, w.Package)
		}
		seen[w.Name] = w.Package
	}
	return all, nil
}

const msgDuplicateWorkerFmt = "sitec: two Web Workers are named %q (%s and %s); rename one"

// projectWorkers lists web/webworkers/<name>/main.go.
func projectWorkers(root string) ([]WebWorker, error) {
	entries, err := os.ReadDir(filepath.Join(root, WorkersDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []WebWorker
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !workerNameRe.MatchString(name) {
			return nil, fmt.Errf("sitec: worker directory %q: use lowercase letters, digits and dashes", name)
		}
		mainPath := filepath.Join(root, WorkersDir, name, mainFile)
		if _, err := os.Stat(mainPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		rt, err := WorkerRuntime(mainPath)
		if err != nil {
			return nil, err
		}
		out = append(out, WebWorker{Name: name, Package: localPackagePrefix + WorkersDir + "/" + name, Runtime: rt})
	}
	return out, nil
}

// depsWithDirFormat prints "<import path>\t<dir>" for every dependency outside the main module
// (the standard library has no Module and the project's own packages are Main).
const depsWithDirFormat = `{{if .Module}}{{if not .Module.Main}}{{.ImportPath}}{{"\t"}}{{.Dir}}{{end}}{{end}}`

// libraryWorkers finds, in the union of every build graph of the project (the same targets and
// destination tags reach.go probes), the packages that ship webworker/main.go.
func libraryWorkers(root string, tc Toolchain) ([]WebWorker, error) {
	dirs := make(map[string]string)
	for _, target := range buildTargets {
		for _, tags := range destinationTags {
			args := []string{"-e", "-deps", "-f", depsWithDirFormat}
			if tags != "" {
				args = append(args, "-tags="+tags)
			}
			args = append(args, "./...")
			var data []byte
			var err error
			if target.GOOS != "" {
				data, err = tc.ListEnv(root, []string{"GOOS=" + target.GOOS, "GOARCH=" + target.GOARCH}, args...)
			} else {
				data, err = tc.List(root, args...)
			}
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(string(data), "\n") {
				importPath, dir, ok := strings.Cut(strings.TrimSpace(line), "\t")
				if ok && dir != "" {
					dirs[importPath] = dir
				}
			}
		}
	}
	var out []WebWorker
	for importPath, dir := range dirs {
		mainPath := filepath.Join(dir, LibraryWorkerDir, mainFile)
		if _, err := os.Stat(mainPath); err != nil {
			continue
		}
		rt, err := WorkerRuntime(mainPath)
		if err != nil {
			return nil, err
		}
		out = append(out, WebWorker{Name: path.Base(importPath), Package: importPath + "/" + LibraryWorkerDir, Runtime: rt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// BuildWorkers builds every Web Worker of the project into the Compiler's FS, with the compiler
// each main.go declares. TinyGo: in release twice, plain and SIMD, with content-hashed names; in
// development once, with the SIMD target and -opt=2, as <name>.wasm (development machines have
// SIMD). Go has no WebAssembly SIMD: built once (hashed in release). Both scripts of a Worker
// point at its binaries.
func (c *Compiler) BuildWorkers() error {
	workers, err := FindWebWorkers(c.RootDir, NewExecToolchain())
	if err != nil {
		return err
	}
	return buildWorkers(c.RootDir, c, c.DevMode, workers)
}

func buildWorkers(root string, c *Compiler, devMode bool, workers []WebWorker) error {
	for _, w := range workers {
		base := WasmBuildOptions{Package: w.Package, OutputName: w.Name, Runtime: w.Runtime, Speed: true, HashName: !devMode}
		var plain, simd string
		var err error
		switch {
		case w.Runtime == js.RuntimeGo:
			if plain, err = buildWorker(root, c, base); err != nil {
				return err
			}
			simd = plain
		case devMode:
			dev := base
			dev.SIMD = true
			if plain, err = buildWorker(root, c, dev); err != nil {
				return err
			}
			simd = plain
		default:
			if plain, err = buildWorker(root, c, base); err != nil {
				return err
			}
			withSIMD := base
			withSIMD.OutputName, withSIMD.SIMD = w.Name+".simd", true
			if simd, err = buildWorker(root, c, withSIMD); err != nil {
				return err
			}
		}
		for _, s := range []*js.Script{
			js.WebWorker(w.Name+".worker.js", "/"+plain, w.Runtime),
			js.WebWorker(w.Name+".simd.worker.js", "/"+simd, w.Runtime),
		} {
			if err := c.Write(s.Name, []byte(s.Content), "text/javascript"); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildWorker compiles one Worker binary with TinyGo, writes it and returns its file name.
func buildWorker(root string, c *Compiler, opts WasmBuildOptions) (string, error) {
	out, err := NewWasmBuilder(opts).Build(root)
	if err != nil {
		return "", err
	}
	return out.Filename, c.Write(out.Filename, out.Binary, mediaTypeWasm)
}
