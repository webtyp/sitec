package sitec

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"webtyp.com/fmt"
	"webtyp.com/image/favicon"
)

func (c *Compiler) getFaviconFiles() []favicon.File {
	c.faviconMu.RLock()
	defer c.faviconMu.RUnlock()
	cp := make([]favicon.File, len(c.faviconFiles))
	copy(cp, c.faviconFiles)
	return cp
}

func (c *Compiler) hasManualFaviconContent() bool {
	if c.faviconSvgHandler == nil {
		return false
	}
	c.faviconSvgHandler.mu.RLock()
	defer c.faviconSvgHandler.mu.RUnlock()
	for _, f := range c.faviconSvgHandler.contentOpen {
		if len(f.Content) > 0 {
			return true
		}
	}
	for _, f := range c.faviconSvgHandler.contentMiddle {
		if len(f.Content) > 0 {
			return true
		}
	}
	for _, f := range c.faviconSvgHandler.contentClose {
		if len(f.Content) > 0 {
			return true
		}
	}
	return len(c.faviconSvgHandler.cachedMinified) > 0
}

func (c *Compiler) getFirstFaviconURL() string {
	c.faviconMu.RLock()
	for _, f := range c.faviconFiles {
		if f.Rel != "" {
			prefix := ""
			if c.Config != nil {
				prefix = c.AssetsURLPrefix
			}
			c.faviconMu.RUnlock()
			return path.Join("/", prefix, f.Name)
		}
	}
	c.faviconMu.RUnlock()
	if c.hasManualFaviconContent() {
		return c.faviconSvgHandler.GetURLPath()
	}
	return ""
}

func buildFaviconHeadLinks(files []favicon.File, prefix string) []byte {
	var sb strings.Builder
	first := true
	for _, f := range files {
		if f.Rel == "" {
			continue
		}
		if !first {
			sb.WriteString("\n\t")
		}
		first = false
		sb.WriteString(`<link rel="`)
		sb.WriteString(f.Rel)
		sb.WriteString(`"`)
		if f.Type != "" {
			sb.WriteString(` type="`)
			sb.WriteString(f.Type)
			sb.WriteString(`"`)
		}
		if f.Sizes != "" {
			sb.WriteString(` sizes="`)
			sb.WriteString(f.Sizes)
			sb.WriteString(`"`)
		}
		href := path.Join("/", prefix, f.Name)
		sb.WriteString(` href="`)
		sb.WriteString(href)
		sb.WriteString(`">`)
	}
	return []byte(sb.String())
}

func (c *Compiler) updateHtmlFaviconLinks() {
	cssSRI, jsSRI := c.computeBundleSRIs()
	c.updateHTMLShellWithSRIs(cssSRI, jsSRI)
}

// faviconSource is the icon of the project: docs/logo/logo.png (and the optional docs/logo/logo.svg beside
// it) under the root, else the fallback the daemon handed over, else none. fromFile tells which, so an
// invalid file is reported by its name.
func (c *Compiler) faviconSource() (src *favicon.Source, fromFile bool, err error) {
	if c.Config != nil && c.RootDir != "" {
		raster, rerr := os.ReadFile(filepath.Join(c.RootDir, filepath.FromSlash(logoFile)))
		if rerr == nil {
			s := &favicon.Source{Raster: raster}
			if svg, serr := os.ReadFile(filepath.Join(c.RootDir, filepath.FromSlash(logoSVGFile))); serr == nil {
				s.SVG = svg
			}
			return s, true, nil
		}
		if !os.IsNotExist(rerr) {
			return nil, true, fmt.Err(logoFile+":", rerr)
		}
	}
	if c.Config != nil && c.FallbackFavicon != nil {
		return c.FallbackFavicon, false, nil
	}
	return nil, false, nil
}

// hintNoLogo tells an app without an icon, once per run: a scan repeats on every file event, a module is
// never told.
func (c *Compiler) hintNoLogo() {
	if c.Config == nil || !IsApp(c.RootDir) {
		return
	}
	c.faviconMu.Lock()
	done := c.noLogoHinted
	c.noLogoHinted = true
	c.faviconMu.Unlock()
	if !done {
		c.Logger(msgNoLogo())
	}
}
