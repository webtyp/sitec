package sitec

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"path"
	"strings"

	"webtyp.com/lang"
	"webtyp.com/pwa"
	"webtyp.com/svg"
)

// CSPManifestFile is written into OutputDir so server/httpd can build a strict
// hash-based script-src Content-Security-Policy without hardcoding hashes.
const CSPManifestFile = "csp.json"

// CSPManifest holds the SRI hashes of the built script bundles.
type CSPManifest struct {
	ScriptHashes []string `json:"script_hashes"`
}

func sriSHA384(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha512.Sum384(data)
	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
}

type htmlShellOptions struct {
	faviconLinks string
	spriteURL    string
	langURL      string
	cssURL       string
	cssIntegrity string
	jsURL        string
	jsIntegrity  string
	jsIsModule   bool
}

func buildHTMLShell(o htmlShellOptions) (openHTML, closeHTML []byte) {
	var head strings.Builder
	head.WriteString("<!doctype html>\n<html>\n<head>\n")
	head.WriteString("\t<meta charset=\"utf-8\">\n")
	head.WriteString("\t<meta name=\"viewport\" content=\"width=device-width, initial-scale=1, viewport-fit=cover\">\n")
	head.WriteString("\t<title></title>\n")
	if o.faviconLinks != "" {
		head.WriteByte('\t')
		head.WriteString(o.faviconLinks)
		head.WriteByte('\n')
	}
	if o.spriteURL != "" {
		head.WriteString("\t<meta name=\"")
		head.WriteString(svg.SpriteMetaName)
		head.WriteString("\" content=\"")
		head.WriteString(o.spriteURL)
		head.WriteString("\">\n")
	}
	if o.langURL != "" {
		head.WriteString("\t<meta name=\"")
		head.WriteString(lang.MetaName)
		head.WriteString("\" content=\"")
		head.WriteString(o.langURL)
		head.WriteString("\">\n")
	}
	head.WriteString("\t<link rel=\"stylesheet\" href=\"")
	head.WriteString(o.cssURL)
	head.WriteByte('"')
	if o.cssIntegrity != "" {
		head.WriteString(" integrity=\"")
		head.WriteString(o.cssIntegrity)
		head.WriteString("\" crossorigin=\"anonymous\"")
	}
	head.WriteString(" type=\"text/css\" />\n</head>\n<body>")

	var closeBuf strings.Builder
	closeBuf.WriteString("</div>\n<script src=\"")
	closeBuf.WriteString(o.jsURL)
	closeBuf.WriteByte('"')
	if o.jsIntegrity != "" {
		closeBuf.WriteString(" integrity=\"")
		closeBuf.WriteString(o.jsIntegrity)
		closeBuf.WriteString("\" crossorigin=\"anonymous\"")
	}
	if o.jsIsModule {
		closeBuf.WriteString(" type=\"module\"></script>\n</body>\n</html>")
	} else {
		closeBuf.WriteString(" type=\"text/javascript\"></script>\n</body>\n</html>")
	}

	return []byte(head.String()), []byte(closeBuf.String())
}

// assetURL formats a generated asset's URL path using the same relative/absolute
// rule as main CSS and JS (relative when the project has no static pages).
func (c *Compiler) assetURL(filename string) string {
	if c.hasPages {
		return path.Join("/", c.AssetsURLPrefix, filename)
	}
	return path.Join(c.AssetsURLPrefix, filename)
}

func (c *Compiler) upsertHashedAssetLocked(slot **asset, baseName, mediatype string, content []byte) string {
	if *slot != nil {
		delete(c.allAssets, (*slot).outputPath)
	}
	if len(content) == 0 {
		*slot = nil
		return ""
	}
	hashedName := pwa.HashedName(baseName, content)
	a := newAssetFile(hashedName, mediatype, c.Config, nil)
	a.urlPath = c.assetURL(hashedName)
	a.contentMiddle = []*ContentFile{{Path: baseName, Content: content}}
	a.mu.Lock()
	a.cachedMinified = content
	a.cacheValid = true
	a.mu.Unlock()
	*slot = a
	c.allAssets[a.outputPath] = a
	return a.urlPath
}

func (c *Compiler) refreshSpriteAssetLocked() {
	raw := []byte(c.renderSpriteNoLock())
	content := raw
	if m := c.activeMinifier(); m != nil {
		if minBytes, err := m.Bytes("image/svg+xml", raw); err == nil {
			content = minBytes
		}
	}
	c.spriteURL = c.upsertHashedAssetLocked(&c.spriteAsset, "sprite.svg", "image/svg+xml", content)
	svg.SetSpriteURL(c.spriteURL)
	if c.spriteURL != "" && c.indexHtmlHandler != nil {
		for _, cf := range c.indexHtmlHandler.contentMiddle {
			if cf != nil && len(cf.Content) > 0 {
				cf.Content = []byte(rewriteSpriteRefs(string(cf.Content), c.spriteURL))
			}
		}
		c.indexHtmlHandler.InvalidateCache()
	}
}

func (c *Compiler) refreshLangAssetLocked(payload []byte) {
	c.translationsURL = c.upsertHashedAssetLocked(&c.langAsset, "lang.json", "application/json", payload)
}

func (c *Compiler) refreshSecurityAssetsLocked() {
	cssSRI, jsSRI := c.computeBundleSRIs()

	manifest := CSPManifest{ScriptHashes: []string{}}
	if jsSRI != "" {
		manifest.ScriptHashes = append(manifest.ScriptHashes, jsSRI)
	}
	if payload, err := json.Marshal(manifest); err == nil {
		if c.cspAsset == nil {
			c.cspAsset = newAssetFile(CSPManifestFile, "application/json", c.Config, nil)
			c.cspAsset.urlPath = path.Join("/", CSPManifestFile)
		}
		c.cspAsset.mu.Lock()
		c.cspAsset.contentMiddle = []*ContentFile{{Path: CSPManifestFile, Content: payload}}
		c.cspAsset.cachedMinified = payload
		c.cspAsset.cacheValid = true
		c.cspAsset.mu.Unlock()
		c.allAssets[c.cspAsset.outputPath] = c.cspAsset
	}

	c.updateHTMLShellWithSRIs(cssSRI, jsSRI)
}

func (c *Compiler) computeBundleSRIs() (cssSRI, jsSRI string) {
	minifier := c.activeMinifier()
	if c.mainStyleCssHandler != nil {
		if b, err := c.mainStyleCssHandler.GetMinifiedContent(minifier); err == nil {
			cssSRI = sriSHA384(b)
		}
	}
	if c.mainJsHandler != nil {
		if b, err := c.mainJsHandler.GetMinifiedContent(minifier); err == nil {
			jsSRI = sriSHA384(b)
		}
	}
	return cssSRI, jsSRI
}

func (c *Compiler) updateHTMLShellWithSRIs(cssSRI, jsSRI string) {
	if c.indexHtmlHandler == nil {
		return
	}

	files := c.getFaviconFiles()
	prefix := ""
	if c.Config != nil {
		prefix = c.AssetsURLPrefix
	}
	links := string(buildFaviconHeadLinks(files, prefix))
	if links == "" && c.hasManualFaviconContent() {
		links = `<link rel="icon" type="image/svg+xml" href="` + c.faviconSvgHandler.GetURLPath() + `">`
	}

	cssURL, jsURL := "", ""
	if c.mainStyleCssHandler != nil {
		cssURL = c.mainStyleCssHandler.GetURLPath()
	}
	if c.mainJsHandler != nil {
		jsURL = c.mainJsHandler.GetURLPath()
	}

	openHTML, closeHTML := buildHTMLShell(htmlShellOptions{
		faviconLinks: links,
		spriteURL:    c.spriteURL,
		langURL:      c.translationsURL,
		cssURL:       cssURL,
		cssIntegrity: cssSRI,
		jsURL:        jsURL,
		jsIntegrity:  jsSRI,
		jsIsModule:   c.jsIsModule,
	})

	c.indexHtmlHandler.mu.Lock()
	if len(c.indexHtmlHandler.contentOpen) > 0 && c.indexHtmlHandler.contentOpen[0].Path == "index-open.html" {
		c.indexHtmlHandler.contentOpen[0].Content = openHTML
		c.indexHtmlHandler.cacheValid = false
	}
	if len(c.indexHtmlHandler.contentClose) > 0 && c.indexHtmlHandler.contentClose[0].Path == "index-close.html" {
		c.indexHtmlHandler.contentClose[0].Content = closeHTML
		c.indexHtmlHandler.cacheValid = false
	}
	c.indexHtmlHandler.mu.Unlock()
}

func (c *Compiler) updateLangAsset(payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshLangAssetLocked(payload)
	c.refreshSecurityAssetsLocked()
	if c.diskMirrored && c.fs != nil {
		if c.langAsset != nil {
			_ = c.fs.Write(c.langAsset.outputPath, c.langAsset.GetCachedMinified(), c.langAsset.mediatype)
		}
		if c.indexHtmlHandler != nil && c.indexHtmlHandler.RegenerateCache(c.activeMinifier()) == nil {
			_ = c.fs.Write(c.indexHtmlHandler.outputPath, c.indexHtmlHandler.GetCachedMinified(), c.indexHtmlHandler.mediatype)
		}
	}
}

func rewriteSpriteRefs(htmlBody, spriteURL string) string {
	if spriteURL == "" || htmlBody == "" {
		return htmlBody
	}
	s := strings.ReplaceAll(htmlBody, `href="#`, `href="`+spriteURL+`#`)
	s = strings.ReplaceAll(s, `href='#`, `href='`+spriteURL+`#`)
	return s
}

// updateReleaseIntegrityInHTML updates or inserts an HTML attribute value on a tag whose src/href matches url.
func updateReleaseIntegrityInHTML(htmlStr, tagPrefix, attrName, url, sri string) string {
	if url == "" || sri == "" {
		return htmlStr
	}
	needle := attrName + `="` + url + `"`
	idx := strings.Index(htmlStr, needle)
	if idx == -1 {
		return htmlStr
	}
	// Locate the enclosing <tag ...>
	tagStart := strings.LastIndex(htmlStr[:idx], "<"+tagPrefix)
	tagEnd := strings.Index(htmlStr[idx:], ">")
	if tagStart == -1 || tagEnd == -1 {
		return htmlStr
	}
	tagEnd += idx
	tag := htmlStr[tagStart : tagEnd+1]
	if iKey := strings.Index(tag, `integrity="`); iKey != -1 {
		valStart := iKey + len(`integrity="`)
		if iClose := strings.IndexByte(tag[valStart:], '"'); iClose != -1 {
			updated := tag[:valStart] + sri + tag[valStart+iClose:]
			return htmlStr[:tagStart] + updated + htmlStr[tagEnd+1:]
		}
	}
	insertAt := idx + len(needle)
	return htmlStr[:insertAt] + ` integrity="` + sri + `" crossorigin="anonymous"` + htmlStr[insertAt:]
}
