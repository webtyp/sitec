//go:build !wasm

package sitec_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/html"
	"webtyp.com/image/favicon"
	"webtyp.com/sitec"
)

func pngBytesHelper(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 0x09, G: 0x6b, B: 0xf0, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func writeTempAppWithFaviconHelper(t *testing.T, appDir string, raster []byte, svg []byte) {
	t.Helper()
	wcwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(wcwd)
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(appDir, "go.mod"), `module example.com/app

go 1.25.2

require (
	webtyp.com/html v0.0.17
	webtyp.com/image v0.1.0
	webtyp.com/sitec v0.0.0
)

replace webtyp.com/sitec => `+repoRoot+"\n"+webtypReplaces(t))
	// The icon is the file docs/logo/logo.png (+ optional logo.svg).
	if err := os.MkdirAll(filepath.Join(appDir, "docs", "logo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "docs", "logo", "logo.png"), raster, 0644); err != nil {
		t.Fatal(err)
	}
	if len(svg) > 0 {
		write(filepath.Join(appDir, "docs", "logo", "logo.svg"), string(svg))
	}
	write(filepath.Join(appDir, "brand.go"), `package app

import "webtyp.com/css"

type Brand struct{}

func (b *Brand) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v %s", err, string(out))
	}
}

// D57: the icon set comes from docs/logo/logo.png.
func TestFaviconFromLogoFile(t *testing.T) {
	appDir := t.TempDir()
	raster := pngBytesHelper(t, 256, 256)
	writeTempAppWithFaviconHelper(t, appDir, raster, nil)
	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	found := false
	for _, a := range out.Artifacts() {
		if a.Path == "/icon-32.png" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /icon-32.png artifact from docs/logo/logo.png, got none; artifacts: %v", out.Artifacts())
	}
}

func TestFaviconEmitsFullSet(t *testing.T) {
	appDir := t.TempDir()
	raster := pngBytesHelper(t, 256, 256)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40"/></svg>`)
	writeTempAppWithFaviconHelper(t, appDir, raster, svg)
	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"/icon-32.png", "/icon-192.png", "/apple-touch-icon.png", "/favicon.ico", "/favicon.svg"}
	found := map[string]bool{}
	for _, a := range out.Artifacts() {
		found[a.Path] = true
	}
	for _, w := range want {
		if !found[w] {
			t.Errorf("expected %s in artifacts, got %v", w, found)
		}
	}
}

func TestFaviconLinksInHead(t *testing.T) {
	appDir := t.TempDir()
	raster := pngBytesHelper(t, 256, 256)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40"/></svg>`)
	writeTempAppWithFaviconHelper(t, appDir, raster, svg)
	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var htmlContent string
	for _, a := range out.Artifacts() {
		if a.Path == "/" {
			htmlContent = string(a.Content)
		}
	}
	if htmlContent == "" {
		t.Fatalf("no / artifact")
	}
	if !strings.Contains(htmlContent, `rel="icon"`) {
		t.Errorf("head missing icon links: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, `/icon-32.png`) {
		t.Errorf("missing icon-32 link: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, `/icon-192.png`) {
		t.Errorf("missing icon-192 link: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, `apple-touch-icon`) || !strings.Contains(htmlContent, `/apple-touch-icon.png`) {
		t.Errorf("missing apple-touch-icon link: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, `/favicon.svg`) {
		t.Errorf("missing favicon.svg link: %s", htmlContent)
	}
	if strings.Contains(htmlContent, `favicon.ico`) {
		t.Errorf("head should not link favicon.ico, got: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, `type="image/png"`) {
		t.Errorf("expected type image/png for png icons: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, `sizes="32x32"`) {
		t.Errorf("expected sizes 32x32: %s", htmlContent)
	}
}

func TestFaviconInvalidLogoFailsBuild(t *testing.T) {
	appDir := t.TempDir()
	raster := pngBytesHelper(t, 800, 600)
	writeTempAppWithFaviconHelper(t, appDir, raster, nil)
	_, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err == nil {
		t.Fatalf("expected error for 800x600 logo, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cuadrado") && !strings.Contains(msg, "256") {
		t.Errorf("expected favicon.Derive error message, got: %v", msg)
	}
	if !strings.Contains(msg, "docs/logo/logo.png") {
		t.Errorf("the error must name the file to fix, got: %v", msg)
	}
}

func TestNoFaviconNoLinkNoFile(t *testing.T) {
	appDir := t.TempDir()
	wcwd, _ := os.Getwd()
	repoRoot := filepath.Dir(wcwd)
	write := func(p, c string) {
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(c), 0644)
	}
	write(filepath.Join(appDir, "go.mod"), `module example.com/app

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/html v0.0.17
	webtyp.com/sitec v0.0.0
)

replace webtyp.com/sitec => `+repoRoot+"\n"+webtypReplaces(t))
	write(filepath.Join(appDir, "app.go"), `package app

import (
	"webtyp.com/css"
	"webtyp.com/html"
)

type P struct{}
func (p *P) RenderPages() []html.Page { return []html.Page{{Path: "/", Doc: html.DocumentOptions{Title: "hi"}, Body: "hi"}} }
func (p *P) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	cmd.CombinedOutput()
	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build without favicon should not fail, got: %v", err)
	}
	for _, a := range out.Artifacts() {
		if a.Path == "/favicon.svg" {
			t.Errorf("should not emit favicon.svg when there is no logo and no existing file, got artifact %v", a.Path)
		}
		if a.Path == "/icon-32.png" {
			t.Errorf("should not emit icon-32 when no favicon")
		}
	}
	var htmlContent string
	for _, a := range out.Artifacts() {
		if a.Path == "/" {
			htmlContent = string(a.Content)
		}
	}
	if strings.Contains(htmlContent, `rel="icon"`) || strings.Contains(htmlContent, `rel='icon'`) || strings.Contains(htmlContent, `apple-touch-icon`) {
		t.Errorf("head should contain no icon links when no favicon, got: %s", htmlContent)
	}
	_ = html.Page{}
}

func TestExistingFaviconSvgSurvives(t *testing.T) {
	appDir := t.TempDir()
	wcwd, _ := os.Getwd()
	repoRoot := filepath.Dir(wcwd)
	write := func(p, c string) {
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(c), 0644)
	}
	write(filepath.Join(appDir, "go.mod"), `module example.com/app

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/html v0.0.17
	webtyp.com/sitec v0.0.0
)

replace webtyp.com/sitec => `+repoRoot+"\n"+webtypReplaces(t))
	write(filepath.Join(appDir, "app.go"), `package app

import (
	"webtyp.com/css"
	"webtyp.com/html"
)

type P struct{}
func (p *P) RenderPages() []html.Page { return []html.Page{{Path: "/", Doc: html.DocumentOptions{Title: "hi"}, Body: "hi"}} }
func (p *P) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	cmd.CombinedOutput()
	faviconPath := filepath.Join(appDir, "web", "public", "favicon.svg")
	os.MkdirAll(filepath.Dir(faviconPath), 0755)
	original := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="red"/></svg>`
	os.WriteFile(faviconPath, []byte(original), 0644)
	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease, OutputDir: "web/public"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	b, err := os.ReadFile(faviconPath)
	if err != nil {
		t.Fatalf("favicon.svg should survive on disk: %v", err)
	}
	if string(b) != original {
		t.Errorf("favicon.svg content changed, expected %q got %q", original, string(b))
	}
	var htmlContent string
	for _, a := range out.Artifacts() {
		if a.Path == "/" {
			htmlContent = string(a.Content)
		}
	}
	if !strings.Contains(htmlContent, `/favicon.svg`) {
		t.Errorf("head should link existing favicon.svg, got: %s", htmlContent)
	}
	count := strings.Count(htmlContent, `rel="icon"`) + strings.Count(htmlContent, `rel='icon'`) + strings.Count(htmlContent, `rel=\"icon\"`)
	if count != 1 {
		t.Errorf("expected 1 icon link for existing svg, got %d: %s", count, htmlContent)
	}
}

// D56/D57: the icon of a module's demo is a default the daemon hands over (in memory, nothing in the repo),
// an app is told once when it has none, a module is never told, and the file wins over the default.
const faviconDocsLink = "https://github.com/webtyp/image/blob/main/README.md#favicon-backend"

func faviconCompiler(t *testing.T, root string, fallback *favicon.Source) (*sitec.Compiler, *[]string) {
	t.Helper()
	am := sitec.NewCompiler(&sitec.Config{RootDir: root, OutputDir: t.TempDir(), FallbackFavicon: fallback})
	var lines []string
	am.SetLog(func(m ...any) {
		parts := make([]string, len(m))
		for i, v := range m {
			parts[i], _ = v.(string)
		}
		lines = append(lines, strings.Join(parts, " "))
	})
	return am, &lines
}

func hasArtifact(am *sitec.Compiler, path string) bool {
	for _, a := range am.List() {
		if a.Path == path {
			return true
		}
	}
	return false
}

func mkApp(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFavicon_FallbackIconServesAModuleDemo(t *testing.T) {
	root := t.TempDir() // a module: no docs/logo, no config/
	fb := &favicon.Source{Raster: pngBytesHelper(t, 512, 512)}
	am, lines := faviconCompiler(t, root, fb)
	if err := am.RouteExtractedAssets([]*sitec.Assets{{ModuleName: "example.com/mod", IsRoot: true}}); err != nil {
		t.Fatal(err)
	}
	if !hasArtifact(am, "/icon-32.png") {
		t.Errorf("the demo must get the default icon, got %v", am.List())
	}
	if len(*lines) != 0 {
		t.Errorf("a module with an icon says nothing, got %v", *lines)
	}
}

func TestFavicon_LogoFileWinsOverTheFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "logo"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := pngBytesHelper(t, 300, 300)
	if err := os.WriteFile(filepath.Join(root, "docs", "logo", "logo.png"), mine, 0o644); err != nil {
		t.Fatal(err)
	}
	am, _ := faviconCompiler(t, root, &favicon.Source{Raster: pngBytesHelper(t, 512, 512)})
	if err := am.RouteExtractedAssets([]*sitec.Assets{{ModuleName: "example.com/app", IsRoot: true}}); err != nil {
		t.Fatal(err)
	}
	if !hasArtifact(am, "/icon-32.png") {
		t.Fatalf("docs/logo/logo.png must produce the icon set, got %v", am.List())
	}
	if hasArtifact(am, "/icon-512.png") {
		t.Error("a 300 px logo must not be served as a 512 px icon: the fallback leaked in")
	}
}

func TestFavicon_AppWithoutLogoIsToldOnce(t *testing.T) {
	root := t.TempDir()
	mkApp(t, root)
	am, lines := faviconCompiler(t, root, nil)
	assets := []*sitec.Assets{{ModuleName: "example.com/app", IsRoot: true}}
	for i := 0; i < 2; i++ { // a second scan after a file event
		if err := am.RouteExtractedAssets(assets); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, l := range *lines {
		if strings.Contains(l, "docs/logo/logo.png") && strings.Contains(l, faviconDocsLink) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want one notice naming docs/logo/logo.png and %s, got %d in %v", faviconDocsLink, n, *lines)
	}
}

func TestFavicon_ModuleWithoutLogoIsQuiet(t *testing.T) {
	am, lines := faviconCompiler(t, t.TempDir(), nil)
	if err := am.RouteExtractedAssets([]*sitec.Assets{{ModuleName: "example.com/mod", IsRoot: true}}); err != nil {
		t.Fatal(err)
	}
	for _, l := range *lines {
		if strings.Contains(strings.ToLower(l), "favicon") || strings.Contains(l, "logo") {
			t.Errorf("a module has no icon of its own and must not be told to add one, got %q", l)
		}
	}
}

func TestIsApp(t *testing.T) {
	cases := map[string]string{"config folder": "config/routes.go", "web binary": "web/server/main.go"}
	for name, marker := range cases {
		root := t.TempDir()
		p := filepath.Join(root, marker)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !sitec.IsApp(root) {
			t.Errorf("%s: want an app", name)
		}
	}
	if sitec.IsApp(t.TempDir()) {
		t.Error("an empty folder (a module) is not an app")
	}
}
