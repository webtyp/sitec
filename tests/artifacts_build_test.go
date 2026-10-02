//go:build !wasm

package sitec_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/artifacts"
	"webtyp.com/pwa"
	"webtyp.com/sitec"
)

const artifactsModelPath = "models/decider.wtypw"

// writeArtifactsApp writes a project whose root declares ArtifactSources() with one 3 MiB file (created
// unless missing is true). withPWA also declares Favicon() and PWA(). Returns the file's bytes.
func writeArtifactsApp(t *testing.T, appDir string, withPWA, missing bool) []byte {
	t.Helper()
	wcwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	write := func(p, content string) {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(appDir, "go.mod"), `module example.com/artifactsapp

go 1.25.2

require (
	webtyp.com/artifacts v0.1.1
	webtyp.com/css v0.4.15
	webtyp.com/device v0.1.0
	webtyp.com/image v0.1.0
	webtyp.com/pwa v0.1.0
	webtyp.com/sitec v0.0.0
)

replace webtyp.com/sitec => `+filepath.Dir(wcwd)+"\n"+webtypReplaces(t))

	write(filepath.Join(appDir, "artifacts.go"), `//go:build !wasm

package app

import (
	"webtyp.com/artifacts"
	"webtyp.com/device"
)

func ArtifactSources() []artifacts.Source {
	return []artifacts.Source{{ID: "decider-0.8b", Version: "q4-2026-09",
		File: "`+artifactsModelPath+`", Needs: device.Requirement{MinTier: device.TierSIMD}}}
}
`)

	app := `package app

import "webtyp.com/css"

type App struct{}

func (a *App) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
func (a *App) RenderHTML() string         { return "<div>artifacts</div>" }
`
	if withPWA {
		if err := os.WriteFile(filepath.Join(appDir, "logo.png"), generate512PNG(t), 0644); err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(appDir, "pwa.go"), `package app

import (
	_ "embed"

	"webtyp.com/image/favicon"
	"webtyp.com/pwa"
)

//go:embed logo.png
var logo []byte

func (a *App) Favicon() favicon.Source { return favicon.Source{Raster: logo} }

func (a *App) PWA() pwa.Config {
	return pwa.Config{Name: "Artifacts", ShortName: "Art", ThemeColor: "#0055ff", BackgroundColor: "#ffffff"}
}
`)
	}
	write(filepath.Join(appDir, "app.go"), app)

	var data []byte
	if !missing {
		data = make([]byte, 3<<20)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(appDir, "models"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(appDir, artifactsModelPath), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v %s", err, out)
	}
	return data
}

func TestArtifacts_BuildPlacesFilesAndManifest(t *testing.T) {
	appDir := t.TempDir()
	data := writeArtifactsApp(t, appDir, false, false)

	if err := sitec.Build(appDir, "out"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	out := filepath.Join(appDir, "out")

	placed, err := os.ReadFile(filepath.Join(out, "artifacts", "decider-0.8b.q4-2026-09.wtypw"))
	if err != nil {
		t.Fatalf("placed file: %v", err)
	}
	if !bytes.Equal(placed, data) {
		t.Fatal("placed file differs from the declared one")
	}

	raw, err := os.ReadFile(filepath.Join(out, "artifacts.json"))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	m, err := artifacts.ParseManifest(raw)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	a, ok := m.Find("decider-0.8b")
	if !ok {
		t.Fatalf("manifest has no decider-0.8b: %s", raw)
	}
	sum := sha256.Sum256(data)
	if a.SHA256 != hex.EncodeToString(sum[:]) || a.Size != int64(len(data)) {
		t.Errorf("manifest entry %+v does not match the file", a)
	}
	if a.URL != pwa.ArtifactsDir+"decider-0.8b.q4-2026-09.wtypw" {
		t.Errorf("URL = %q", a.URL)
	}
}

func TestArtifacts_NotInMemoryArtifacts(t *testing.T) {
	appDir := t.TempDir()
	writeArtifactsApp(t, appDir, false, false)

	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var hasManifest bool
	for _, a := range out.Artifacts() {
		p := "/" + strings.TrimPrefix(a.Path, "/")
		if strings.HasPrefix(p, pwa.ArtifactsDir) {
			t.Errorf("large file %s is an in-memory artifact", a.Path)
		}
		if p == artifacts.ManifestPath {
			hasManifest = true
		}
	}
	if !hasManifest {
		t.Error("no /artifacts.json artifact")
	}
	if n := len(out.LargeFiles()); n != 1 {
		t.Errorf("LargeFiles() has %d entries, want 1", n)
	}
}

func TestArtifacts_ReleasePWAPrecachesManifestNotFiles(t *testing.T) {
	appDir := t.TempDir()
	writeArtifactsApp(t, appDir, true, false)

	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var sw string
	for _, a := range out.Artifacts() {
		if strings.TrimPrefix(a.Path, "/") == "sw.js" {
			sw = string(a.Content)
		}
	}
	if sw == "" {
		t.Fatal("no sw.js")
	}
	if !strings.Contains(sw, artifacts.ManifestPath) {
		t.Error("sw.js does not precache /artifacts.json")
	}
	if strings.Contains(sw, pwa.ArtifactsDir) {
		t.Error("sw.js precaches a file under /artifacts/")
	}
}

func TestArtifacts_MissingFileFailsBuild(t *testing.T) {
	appDir := t.TempDir()
	writeArtifactsApp(t, appDir, false, true)

	err := sitec.Build(appDir, "out")
	if err == nil || !strings.Contains(err.Error(), "decider-0.8b") {
		t.Fatalf("got %v, want an error naming decider-0.8b", err)
	}
}

func TestArtifacts_NonRootError(t *testing.T) {
	wcwd, _ := os.Getwd()
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

replace webtyp.com/sitec => `+filepath.Dir(wcwd)+`
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
	webtyp.com/artifacts v0.1.1
	webtyp.com/css v0.4.15
)
`+webtypReplaces(t))
	write(filepath.Join(libDir, "widget.go"), `package widget

import (
	"webtyp.com/artifacts"
	"webtyp.com/css"
)

type Widget struct{}
func (w *Widget) ArtifactSources() []artifacts.Source { return []artifacts.Source{{ID: "x", Version: "1", File: "x.bin"}} }
func (w *Widget) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)
	for _, dir := range []string{libDir, appDir} {
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go mod tidy in %s: %v %s", dir, err, out)
		}
	}

	_, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err == nil {
		t.Fatal("expected an error when a non-root module declares ArtifactSources()")
	}
	if !strings.Contains(err.Error(), "only the root module") || !strings.Contains(err.Error(), "ArtifactSources()") ||
		!strings.Contains(err.Error(), "example.com/widget") {
		t.Errorf("got %v, want msgArtifactsNonRoot naming example.com/widget", err)
	}
}

// Producers are matched by name only (docs/ARCHITECTURE.md §3.1), so their names must be unique in
// the ecosystem: Artifacts() is declared by min.Handler and sitec.Output, and a project declaring one
// must not have it called as the declaration.
func TestArtifacts_CommonNameIsNotAProducer(t *testing.T) {
	wcwd, _ := os.Getwd()
	appDir := t.TempDir()
	write := func(p, c string) {
		if err := os.WriteFile(p, []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(appDir, "go.mod"), `module example.com/app

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/sitec v0.0.0
)

replace webtyp.com/sitec => `+filepath.Dir(wcwd)+"\n"+webtypReplaces(t))
	write(filepath.Join(appDir, "app.go"), `package app

import "webtyp.com/css"

type App struct{}
func (a *App) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
func (a *App) Artifacts() []string      { return []string{"not a declaration"} }
`)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v %s", err, out)
	}

	out, err := sitec.BuildWithConfig(sitec.BuildConfig{RootDir: appDir, Mode: sitec.ModeRelease})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, a := range out.Artifacts() {
		if "/"+strings.TrimPrefix(a.Path, "/") == artifacts.ManifestPath {
			t.Error("an Artifacts() []string produced /artifacts.json")
		}
	}
}
