package sitec_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/js"
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

	write(filepath.Join(root, "web/browser/main.go"), "//go:build browser\n\npackage main\nfunc main() {}")
	write(filepath.Join(root, "html.go"), `//go:build !wasm

package app

import (
	"webtyp.com/css"
)

type App struct{}
func (a *App) RenderCSS() *css.Stylesheet { return css.NewStylesheet() }
`)
	if workerName != "" {
		write(filepath.Join(root, sitec.WorkersDir, workerName, "main.go"), "//go:build wasm && tinygo\n\npackage main\nfunc main() {}\n")
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v %s", err, out)
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

	if plainWasm == simdWasm {
		t.Errorf("plain and SIMD builds share the name %s", plainWasm)
	}
	// The binaries are TinyGo's: each script must carry TinyGo's wasm_exec glue, not Go's.
	const tinygoGlue = "modified for use by the TinyGo compiler"
	if !strings.Contains(plainScript, tinygoGlue) || !strings.Contains(simdScript, tinygoGlue) {
		t.Error("a release worker script does not load TinyGo's runtime glue")
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

// writeWorkerLibrary writes, next to root, a library module example.com/<name> that ships
// webworker/main.go, requires it from the app with a replace and, when imported, makes an app
// package import it.
func writeWorkerLibrary(t *testing.T, root, name string, imported bool) {
	t.Helper()
	libDir := filepath.Join(filepath.Dir(root), "lib-"+name)
	modPath := "example.com/" + name
	files := map[string]string{
		filepath.Join(libDir, "go.mod"):               "module " + modPath + "\n\ngo 1.22\n",
		filepath.Join(libDir, name+".go"):             "package " + name + "\n\nfunc Ready() bool { return true }\n",
		filepath.Join(libDir, "webworker", "main.go"): "//go:build wasm && tinygo\n\npackage main\n\nfunc main() {}\n",
	}
	if imported {
		files[filepath.Join(root, "uses"+name, "uses.go")] = "package uses" + name + "\n\nimport \"" + modPath + "\"\n\nvar _ = " + name + ".Ready\n"
	}
	for p, src := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"mod", "edit", "-require=" + modPath + "@v0.0.0", "-replace=" + modPath + "=" + libDir},
		{"mod", "tidy"},
	} {
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v %s", args, err, out)
		}
	}
}

// A library that ships webworker/main.go becomes /<library>.wasm + /<library>.worker.js as soon
// as the app's build reaches it; the app writes nothing.
func TestFindWebWorkers_LibraryReachedByTheBuild(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	writeWorkerFixture(t, root, "")
	writeWorkerLibrary(t, root, "pdflib", true)

	workers, err := sitec.FindWebWorkers(root, sitec.NewExecToolchain())
	if err != nil {
		t.Fatal(err)
	}
	if len(workers) != 1 || workers[0] != (sitec.WebWorker{Name: "pdflib", Package: "example.com/pdflib/webworker", Runtime: js.RuntimeTinyGo}) {
		t.Fatalf("FindWebWorkers = %+v, want the pdflib Web Worker", workers)
	}

	c := sitec.NewCompiler(&sitec.Config{RootDir: root, OutputDir: "web/public", DevMode: true})
	c.SetFS(sitec.NewMemFS())
	if err := c.BuildWorkers(); err != nil {
		t.Fatal(err)
	}
	if script, _, ok := c.Read("/pdflib.worker.js"); !ok || !strings.Contains(string(script), "/pdflib.wasm") {
		t.Errorf("/pdflib.worker.js missing or not pointing at /pdflib.wasm")
	}
	if _, _, ok := c.Read("/pdflib.wasm"); !ok {
		t.Errorf("/pdflib.wasm was not built")
	}
}

// A library the build does not reach ships nothing, even if it is in go.mod.
func TestFindWebWorkers_UnreachedLibraryShipsNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	writeWorkerFixture(t, root, "")
	writeWorkerLibrary(t, root, "pdflib", false)

	workers, err := sitec.FindWebWorkers(root, sitec.NewExecToolchain())
	if err != nil {
		t.Fatal(err)
	}
	if len(workers) != 0 {
		t.Errorf("FindWebWorkers = %+v, want none", workers)
	}
}

// The app's own Worker and a library's Worker with the same name collide: the error names both.
func TestFindWebWorkers_DuplicateNameIsAnError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	writeWorkerFixture(t, root, "pdflib")
	writeWorkerLibrary(t, root, "pdflib", true)

	_, err := sitec.FindWebWorkers(root, sitec.NewExecToolchain())
	if err == nil || !strings.Contains(err.Error(), "./"+sitec.WorkersDir+"/pdflib") || !strings.Contains(err.Error(), "example.com/pdflib/webworker") {
		t.Errorf("want an error naming both Workers, got: %v", err)
	}
}
