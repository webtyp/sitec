//go:build !wasm

package sitec_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/sitec"
)

func TestOneShotBuild_ConsumerFixture(t *testing.T) {
	baseDir := t.TempDir()
	appDir := filepath.Join(baseDir, "app")
	outDir := filepath.Join(appDir, "web", "public")

	wcwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(wcwd)

	fixtureDir, err := filepath.Abs("../testdata/consumer")
	if err != nil {
		t.Fatal(err)
	}

	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cssBytes, err := os.ReadFile(filepath.Join(fixtureDir, "css.go"))
	if err != nil {
		t.Fatalf("failed to read testdata/consumer/css.go: %v", err)
	}
	svgBytes, err := os.ReadFile(filepath.Join(fixtureDir, "svg.go"))
	if err != nil {
		t.Fatalf("failed to read testdata/consumer/svg.go: %v", err)
	}

	write(filepath.Join(appDir, "css.go"), string(cssBytes))
	write(filepath.Join(appDir, "svg.go"), string(svgBytes))

	goModContent := `module example.com/consumer

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/sitec v0.0.0
	webtyp.com/svg v0.1.21
)

replace webtyp.com/sitec => ` + repoRoot + "\n" + webtypReplaces(t)

	write(filepath.Join(appDir, "go.mod"), goModContent)

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v, output: %s", err, string(out))
	}

	// Write a stale artifact to verify outDir is cleaned prior to emission
	staleFile := filepath.Join(outDir, "stale.txt")
	write(staleFile, "stale content")

	if err := sitec.Build(appDir, "web/public"); err != nil {
		t.Fatalf("sitec.Build failed: %v", err)
	}

	// 1. Assert stale file was removed
	if _, err := os.Stat(staleFile); err == nil {
		t.Errorf("expected stale file %s to be removed by Build, but it still exists", staleFile)
	}

	// 2. Assert style.css exists and is non-empty
	cssContent, err := os.ReadFile(releaseHashed(t, outDir, "style", ".css"))
	if err != nil {
		t.Fatalf("expected style.css to be emitted: %v", err)
	}
	if len(cssContent) == 0 {
		t.Errorf("expected style.css to be non-empty")
	}
	if !strings.Contains(string(cssContent), "background-color") {
		t.Errorf("expected style.css to contain background-color, got: %s", string(cssContent))
	}

	// 3. The icon sprite lives only inside the HTML: no icons*.svg file, and index.html
	// carries the consumer's symbol.
	if matches, _ := filepath.Glob(filepath.Join(outDir, "icons*.svg")); len(matches) != 0 {
		t.Errorf("a sprite file was written: %v; the sprite must live only in index.html", matches)
	}
	htmlContent, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatalf("expected index.html to be emitted: %v", err)
	}
	if !strings.Contains(string(htmlContent), "consumer-icon") {
		t.Errorf("expected index.html to contain the consumer-icon symbol")
	}
}

func TestOneShotBuild_WithoutMinify(t *testing.T) {
	baseDir := t.TempDir()
	appDir := filepath.Join(baseDir, "app")
	outDir := filepath.Join(appDir, "web", "public")

	wcwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(wcwd)

	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(appDir, "css.go"), `//go:build !wasm

package app

import "webtyp.com/css"

type Styles struct{}

func (s *Styles) RenderCSS() *css.Stylesheet {
	return css.NewStylesheet(
		css.Raw("body {\n  margin: 0;\n}"),
	)
}
`)

	goModContent := `module example.com/app

go 1.25.2

require (
	webtyp.com/css v0.4.15
	webtyp.com/sitec v0.0.0
)

replace webtyp.com/sitec => ` + repoRoot + "\n" + webtypReplaces(t)

	write(filepath.Join(appDir, "go.mod"), goModContent)

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v, output: %s", err, string(out))
	}

	if err := sitec.Build(appDir, "web/public", sitec.WithoutMinify()); err != nil {
		t.Fatalf("sitec.Build failed: %v", err)
	}

	cssContent, err := os.ReadFile(releaseHashed(t, outDir, "style", ".css"))
	if err != nil {
		t.Fatalf("expected style.css to be emitted: %v", err)
	}
	if !strings.Contains(string(cssContent), "margin: 0") {
		t.Errorf("expected style.css to contain margin: 0, got: %s", string(cssContent))
	}
}

// releaseHashed returns the path of the single content-hashed file <name>.<hash><ext> a release
// build writes to outDir; a release build never writes the unhashed <name><ext>.
func releaseHashed(t *testing.T, outDir, name, ext string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(outDir, name+ext)); err == nil {
		t.Errorf("release build wrote an unhashed %s%s", name, ext)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, name+".*"+ext))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly one %s.<hash>%s in %s, got %v (%v)", name, ext, outDir, matches, err)
	}
	return matches[0]
}
