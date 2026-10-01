package sitec

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"

	"webtyp.com/fmt"
	"webtyp.com/image/favicon"
	"webtyp.com/pwa"
)

// hashLen is how many hex characters of the content's SHA-256 go into a file name.
const hashLen = 8

// contentHash is the hex SHA-256 of content, 16 characters: the revision of a shell asset.
func contentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])[:16]
}

// hashedName inserts the first hashLen characters of contentHash before the extension:
// hashedName("style.css", c) == "style.3f9a1c2b.css". A name without extension gets ".<hash>".
func hashedName(name string, content []byte) string {
	hash := contentHash(content)[:hashLen]
	ext := path.Ext(name)
	if ext != "" {
		base := strings.TrimSuffix(name, ext)
		return base + "." + hash + ext
	}
	return name + "." + hash
}

// releaseInput is what the final pass needs from the compiler.
type releaseInput struct {
	CSSURL, JSURL string         // GetURLPath() of the main stylesheet and script ("" = absent)
	PWA           *pwa.Config    // nil = the project is not a PWA
	Favicons      []favicon.File // c.getFaviconFiles()
	Log           func(...any)
}

// finalizeRelease returns the artifacts of a release build: PWA tags and register script
// inserted (when PWA != nil), the main CSS and JS renamed by content hash with every
// HTML reference rewritten, and manifest.webmanifest + sw.js added (when PWA != nil).
func finalizeRelease(arts []Artifact, in releaseInput) ([]Artifact, error) {
	out := make([]Artifact, len(arts))
	copy(out, arts)

	// 1. PWA head and script (only when in.PWA != nil)
	var app pwa.App
	var hasPWA bool
	if in.PWA != nil {
		hasPWA = true
		if len(in.Favicons) == 0 {
			return nil, fmt.Err(msgPWAWithoutFavicon())
		}
		var icons []pwa.Icon
		for _, f := range in.Favicons {
			if f.Sizes == "192x192" || f.Sizes == "512x512" {
				icons = append(icons, pwa.Icon{
					URL:   path.Join("/", f.Name),
					Sizes: f.Sizes,
					Type:  f.Mediatype,
				})
			}
		}
		var err error
		app, err = pwa.New(*in.PWA, icons)
		if err != nil {
			return nil, err
		}

		// Every artifact with mediatype text/html: insert app.HeadTags immediately before </head>
		headEnd := "</head>"
		for i, art := range out {
			if art.Mediatype == "text/html" {
				htmlStr := string(art.Content)
				idx := strings.Index(htmlStr, headEnd)
				if idx == -1 {
					return nil, fmt.Err(fmt.Sprintf("sitec: %s has no </head> to insert the PWA tags into", art.Path))
				}
				newHTML := htmlStr[:idx] + app.HeadTags + htmlStr[idx:]
				out[i].Content = []byte(newHTML)
			}
		}

		// The artifact whose Path == in.JSURL: append "\n" + app.RegisterScript
		if in.JSURL != "" {
			for i, art := range out {
				if art.Path == in.JSURL {
					out[i].Content = append(out[i].Content, []byte("\n"+app.RegisterScript)...)
					break
				}
			}
		}

		// Append manifest artifact
		out = append(out, Artifact{
			Path:      pwa.ManifestPath,
			Mediatype: "application/manifest+json",
			Content:   []byte(app.Manifest),
		})
	}

	// 2. Hashed names (always in release)
	targetURLs := []string{in.CSSURL, in.JSURL}
	renames := make(map[string]string) // oldURL -> newURL

	for _, old := range targetURLs {
		if old == "" {
			continue
		}
		for i, art := range out {
			if art.Path == old {
				dir := path.Dir(old)
				base := path.Base(old)
				newName := hashedName(base, art.Content)
				newURL := path.Join(dir, newName)
				if !strings.HasPrefix(old, "/") {
					newURL = strings.TrimPrefix(newURL, "./")
					newURL = strings.TrimPrefix(newURL, "/")
				}
				renames[old] = newURL
				out[i].Path = newURL
				break
			}
		}
	}

	// In every text/html artifact replace "old" with "newURL"
	if len(renames) > 0 {
		for i, art := range out {
			if art.Mediatype == "text/html" {
				contentStr := string(art.Content)
				for old, newURL := range renames {
					contentStr = strings.ReplaceAll(contentStr, `"`+old+`"`, `"`+newURL+`"`)
				}
				out[i].Content = []byte(contentStr)
			}
		}
	}

	// 3. Service worker (only when in.PWA != nil)
	if hasPWA {
		var shell []pwa.Asset
		for _, art := range out {
			if art.Path == pwa.ServiceWorkerPath {
				continue
			}
			urlPath := art.Path
			if !strings.HasPrefix(urlPath, "/") {
				urlPath = path.Join("/", urlPath)
			}
			shell = append(shell, pwa.Asset{
				URL:      urlPath,
				Revision: contentHash(art.Content),
			})
		}
		worker, err := app.ServiceWorker(shell)
		if err != nil {
			return nil, err
		}
		out = append(out, Artifact{
			Path:      pwa.ServiceWorkerPath,
			Mediatype: "text/javascript",
			Content:   []byte(worker.Script),
		})
		if in.Log != nil {
			in.Log(fmt.Sprintf("PWA version %s, %d files precached", worker.Version, len(shell)))
		}
	}

	return out, nil
}
