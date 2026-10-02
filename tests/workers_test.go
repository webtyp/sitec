package sitec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/pwa"
	"webtyp.com/sitec"
)

func writeWorkerFixture(t *testing.T, root string, workerName string) {
	t.Helper()
	write := func(p, content string) {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	wcwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	write(filepath.Join(root, "go.mod"), `module example.com/workerapp

go 1.26.8

require (
	webtyp.com/css v0.4.22
	webtyp.com/sitec v0.0.0
	webtyp.com/pwa v0.1.1
	webtyp.com/html v0.0.24
)

`+func() string {
	repls := webtypReplaces(t)
	if repls == "" {
		return "replace webtyp.com/sitec => " + filepath.Dir(wcwd)
	}
	return "replace webtyp.com/sitec => " + filepath.Dir(wcwd) + "\n" + repls
}())

	write(filepath.Join(root, "web/client.go"), "package main\nfunc main() {}")
	write(filepath.Join(root, "html.go"), `//go:build !wasm

package app

import (
	"webtyp.com/css"
)

type App struct{}
func (a *App) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)
	if workerName != "" {
		write(filepath.Join(root, "web/workers", workerName, "main.go"), "package main\nfunc main() {}")
	}
}

func TestWorkers_ReleaseBuildsPlainAndSIMD(t *testing.T) {
	root := t.TempDir()
	writeWorkerFixture(t, root, "echo")

	cfg := sitec.BuildConfig{
		RootDir:   root,
		OutputDir: filepath.Join(root, "out"),
		Mode:      sitec.ModeRelease,
	}

	out, err := sitec.BuildWithConfig(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	arts := out.Artifacts()

	var plainScript, simdScript, plainWasm, simdWasm string

	for _, a := range arts {
		if a.Path == "/echo.worker.js" {
			plainScript = string(a.Content)
		}
		if a.Path == "/echo.simd.worker.js" {
			simdScript = string(a.Content)
		}
		if strings.HasPrefix(a.Path, "/echo.") && strings.HasSuffix(a.Path, ".wasm") && !strings.Contains(a.Path, "simd") {
			if pwa.IsHashedName(a.Path) {
				plainWasm = a.Path
			}
		}
		if strings.HasPrefix(a.Path, "/echo.simd.") && strings.HasSuffix(a.Path, ".wasm") {
			if pwa.IsHashedName(a.Path) {
				simdWasm = a.Path
			}
		}
	}

	if plainScript == "" {
		t.Error("Missing echo.worker.js")
	}
	if simdScript == "" {
		t.Error("Missing echo.simd.worker.js")
	}
	if plainWasm == "" {
		t.Error("Missing hashed plain wasm artifact")
	}
	if simdWasm == "" {
		t.Error("Missing hashed simd wasm artifact")
	}

	if plainScript != "" && !strings.Contains(plainScript, plainWasm) {
		t.Errorf("plain script does not reference plain wasm %s, got: %s", plainWasm, plainScript)
	}
	if simdScript != "" && !strings.Contains(simdScript, simdWasm) {
		t.Errorf("simd script does not reference simd wasm %s, got: %s", simdWasm, simdScript)
	}
}

func TestWorkers_DevBuildsOncePointsBothScripts(t *testing.T) {
	root := t.TempDir()
	writeWorkerFixture(t, root, "echo")

	cfg := sitec.BuildConfig{
		RootDir:   root,
		OutputDir: filepath.Join(root, "out"),
		Mode:      sitec.ModeDev,
	}

	out, err := sitec.BuildWithConfig(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	arts := out.Artifacts()

	var plainScript, simdScript, wasmPath string

	for _, a := range arts {
		if a.Path == "/echo.worker.js" {
			plainScript = string(a.Content)
		}
		if a.Path == "/echo.simd.worker.js" {
			simdScript = string(a.Content)
		}
		if a.Path == "/echo.wasm" {
			wasmPath = a.Path
		}
	}

	if plainScript == "" || simdScript == "" || wasmPath == "" {
		t.Errorf("Missing dev mode artifacts. plainScript=%q, simdScript=%q, wasmPath=%q", plainScript, simdScript, wasmPath)
	}

	if !strings.Contains(plainScript, "/echo.wasm") {
		t.Errorf("plain script does not reference /echo.wasm, got: %s", plainScript)
	}
	if !strings.Contains(simdScript, "/echo.wasm") {
		t.Errorf("simd script does not reference /echo.wasm, got: %s", simdScript)
	}
}

func TestWorkers_BadName(t *testing.T) {
	root := t.TempDir()
	writeWorkerFixture(t, root, "Echo_1")

	cfg := sitec.BuildConfig{
		RootDir:   root,
		OutputDir: filepath.Join(root, "out"),
		Mode:      sitec.ModeDev,
	}

	_, err := sitec.BuildWithConfig(cfg)
	if err == nil {
		t.Fatal("Expected error for bad worker name, got nil")
	}

	expectedErr := `sitec: worker directory "Echo_1": use lowercase letters, digits and dashes`
	if !strings.Contains(err.Error(), expectedErr) {
		t.Fatalf("Expected error %q, got: %v", expectedErr, err)
	}
}

func TestWorkers_NoWorkersNoOutput(t *testing.T) {
	root := t.TempDir()
	writeWorkerFixture(t, root, "")

	cfg := sitec.BuildConfig{
		RootDir:   root,
		OutputDir: filepath.Join(root, "out"),
		Mode:      sitec.ModeDev,
	}

	out, err := sitec.BuildWithConfig(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, a := range out.Artifacts() {
		if strings.Contains(a.Path, "worker") || strings.Contains(a.Path, "echo") {
			t.Errorf("Expected no worker artifacts, got %s", a.Path)
		}
	}
}
