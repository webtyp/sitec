//go:build !wasm

package sitec_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/sitec"
)

func generate512PNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			img.Set(x, y, color.RGBA{R: 0x00, G: 0x80, B: 0xff, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func writeTempPWAApp(t *testing.T, appDir string) {
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

	write(filepath.Join(appDir, "go.mod"), `module example.com/pwaapp

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/image v0.1.0
	webtyp.com/pwa v0.1.0
	webtyp.com/sitec v0.0.0
)

replace webtyp.com/sitec => `+repoRoot+"\n"+webtypReplaces(t))

	raster := generate512PNG(t)
	if err := os.WriteFile(filepath.Join(appDir, "logo.png"), raster, 0644); err != nil {
		t.Fatal(err)
	}

	write(filepath.Join(appDir, "app.go"), `package app

import (
	_ "embed"
	"webtyp.com/css"
	"webtyp.com/image/favicon"
	"webtyp.com/pwa"
)

//go:embed logo.png
var logo []byte

type App struct{}

func (a *App) Favicon() favicon.Source {
	return favicon.Source{Raster: logo}
}

func (a *App) PWA() pwa.Config {
	return pwa.Config{
		Name:            "My PWA App",
		ShortName:       "PWA",
		ThemeColor:      "#0055ff",
		BackgroundColor: "#ffffff",
	}
}

func (a *App) RenderCSS() *css.Stylesheet {
	return css.NewStylesheet()
}

func (a *App) RenderHTML() string {
	return "<div>Hello PWA</div>"
}
`)

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v %s", err, string(out))
	}
}

func TestPWABuildRelease(t *testing.T) {
	appDir := t.TempDir()
	writeTempPWAApp(t, appDir)

	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	arts := out.Artifacts()
	var hasManifest, hasSW, hasHashedCSS, hasHashedJS bool
	var htmlContent string

	for _, a := range arts {
		if a.Path == "/manifest.webmanifest" {
			hasManifest = true
		}
		if a.Path == "/sw.js" {
			hasSW = true
		}
		if a.Path == "/" || a.Path == "/index.html" {
			htmlContent = string(a.Content)
		}
		// An app without pages emits relative URLs (style.<hash>.css), a site absolute ones.
		base := path.Base(a.Path)
		if strings.HasPrefix(base, "style.") && strings.HasSuffix(base, ".css") && base != "style.css" {
			hasHashedCSS = true
		}
		if strings.HasPrefix(base, "script.") && strings.HasSuffix(base, ".js") && base != "script.js" {
			hasHashedJS = true
		}
		if base == "style.css" || base == "script.js" {
			t.Errorf("found unhashed %s in release build", a.Path)
		}
	}

	if !hasManifest {
		t.Errorf("release build missing manifest.webmanifest artifact")
	}
	if !hasSW {
		t.Errorf("release build missing sw.js artifact")
	}
	if !hasHashedCSS {
		t.Errorf("release build missing hashed CSS artifact")
	}
	if !hasHashedJS {
		t.Errorf("release build missing hashed JS artifact")
	}

	if !strings.Contains(htmlContent, `rel="manifest"`) && !strings.Contains(htmlContent, `rel='manifest'`) {
		t.Errorf("head missing manifest link: %s", htmlContent)
	}
}

func TestPWABuildDev(t *testing.T) {
	appDir := t.TempDir()
	writeTempPWAApp(t, appDir)

	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeDev})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	arts := out.Artifacts()
	var foundStyleCSS bool

	for _, a := range arts {
		if a.Path == "/sw.js" {
			t.Errorf("dev build should not produce sw.js")
		}
		if a.Path == "/style.css" || a.Path == "style.css" {
			foundStyleCSS = true
		}
	}

	if !foundStyleCSS {
		t.Errorf("dev build should maintain unhashed /style.css artifact")
	}
}

func TestPWANonRootError(t *testing.T) {
	wcwd, _ := os.Getwd()
	repoRoot := filepath.Dir(wcwd)
	appDir := t.TempDir()
	libDir := filepath.Join(appDir, "libwidget")

	write := func(p, c string) {
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(c), 0644)
	}

	write(filepath.Join(appDir, "go.mod"), `module example.com/app

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/sitec v0.0.0
	example.com/widget v0.0.0
)

replace webtyp.com/sitec => `+repoRoot+`
replace example.com/widget => ./libwidget
`+webtypReplaces(t))

	write(filepath.Join(appDir, "app.go"), `package app

import (
	"webtyp.com/css"
	_ "example.com/widget"
)

type App struct{}
func (a *App) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)

	write(filepath.Join(libDir, "go.mod"), `module example.com/widget

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/pwa v0.1.0
)
`+webtypReplaces(t))

	write(filepath.Join(libDir, "widget.go"), `package widget

import (
	"webtyp.com/css"
	"webtyp.com/pwa"
)

type Widget struct{}
func (w *Widget) PWA() pwa.Config { return pwa.Config{Name: "Widget"} }
func (w *Widget) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	cmd.CombinedOutput()

	cmd2 := exec.Command("go", "mod", "tidy")
	cmd2.Dir = libDir
	cmd2.CombinedOutput()

	_, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err == nil {
		t.Fatalf("expected error when non-root declares PWA(), got nil")
	}

	if !strings.Contains(err.Error(), "only the root module") {
		t.Errorf("expected error msgPWANonRoot, got: %v", err)
	}
	if !strings.Contains(err.Error(), "example.com/widget") && !strings.Contains(err.Error(), "widget") {
		t.Errorf("expected error to name widget module, got: %v", err)
	}
}
