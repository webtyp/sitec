package sitec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/js"
	"webtyp.com/sitec"
)

// writeWorkerPackage writes a project with go.mod and the given files (path → source).
func writeWorkerPackage(t *testing.T, root string, files map[string]string) {
	t.Helper()
	files["go.mod"] = "module testapp\n\ngo 1.22\n"
	for p, src := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWasmBuild_FailsIfLocalPackageHasNoMain(t *testing.T) {
	wb := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./" + sitec.WorkersDir + "/echo", OutputName: "echo", Runtime: js.RuntimeTinyGo})
	_, err := wb.Build(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "input file not found") {
		t.Errorf("want 'input file not found', got: %v", err)
	}
}

// Without Package or OutputName there is nothing to compile or name: the builder says so
// instead of guessing a default (the page itself is never WebAssembly).
func TestWasmBuild_RequiresPackageAndOutputName(t *testing.T) {
	for _, opts := range []sitec.WasmBuildOptions{{Runtime: js.RuntimeTinyGo}, {Package: "./x", Runtime: js.RuntimeTinyGo}, {OutputName: "x", Runtime: js.RuntimeTinyGo}} {
		if _, err := sitec.NewWasmBuilder(opts).Build(t.TempDir()); err == nil || !strings.Contains(err.Error(), "Package and OutputName") {
			t.Errorf("opts %+v: want the required-fields error, got: %v", opts, err)
		}
	}
}

// A Worker package with more than one file keeps its sibling files: the package is compiled,
// never a lone file (the goflare-demo regression).
func TestWasmBuild_CompilesSiblingFilesInSamePackage(t *testing.T) {
	root := t.TempDir()
	dir := sitec.WorkersDir + "/echo"
	writeWorkerPackage(t, root, map[string]string{
		dir + "/main.go":   "//go:build wasm && tinygo\n\npackage main\nfunc main() { helper() }",
		dir + "/access.go": "//go:build wasm && tinygo\n\npackage main\nfunc helper() {}",
	})
	out, err := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./" + dir, OutputName: "echo", Runtime: js.RuntimeTinyGo}).Build(root)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if out.Filename != "echo.wasm" || len(out.Binary) == 0 {
		t.Errorf("got %q with %d bytes, want echo.wasm with a binary", out.Filename, len(out.Binary))
	}
}

// The package resolves against the directory passed to Build, also when it is relative.
func TestWasmBuild_RelativeDirResolvesPackage(t *testing.T) {
	t.Chdir(t.TempDir())
	const projectDir = "proj"
	dir := sitec.WorkersDir + "/echo"
	writeWorkerPackage(t, projectDir, map[string]string{dir + "/main.go": "//go:build wasm && tinygo\n\npackage main\nfunc main() {}\n"})
	out, err := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./" + dir, OutputName: "echo", Runtime: js.RuntimeTinyGo}).Build(projectDir)
	if err != nil {
		t.Fatalf("Build with a relative dir failed: %v", err)
	}
	if len(out.Binary) == 0 {
		t.Error("expected non-empty binary")
	}
}

// HashName names the binary by its content; without it the name stays fixed.
func TestWasmBuild_HashNameOnlyWhenAsked(t *testing.T) {
	root := t.TempDir()
	dir := sitec.WorkersDir + "/echo"
	writeWorkerPackage(t, root, map[string]string{dir + "/main.go": "//go:build wasm && tinygo\n\npackage main\nfunc main() {}\n"})
	fixed, err := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./" + dir, OutputName: "echo", Runtime: js.RuntimeTinyGo}).Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if fixed.Filename != "echo.wasm" {
		t.Errorf("Filename = %q, want echo.wasm", fixed.Filename)
	}
	hashed, err := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./" + dir, OutputName: "echo", Runtime: js.RuntimeTinyGo, HashName: true}).Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if hashed.Filename == "echo.wasm" || !strings.HasPrefix(hashed.Filename, "echo.") || !strings.HasSuffix(hashed.Filename, ".wasm") {
		t.Errorf("hashed Filename = %q, want echo.<hash>.wasm", hashed.Filename)
	}
}

// Without a Runtime the builder does not guess a compiler.
func TestWasmBuild_RequiresRuntime(t *testing.T) {
	_, err := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./x", OutputName: "x"}).Build(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "js.RuntimeTinyGo or js.RuntimeGo") {
		t.Errorf("want the Runtime-required error, got: %v", err)
	}
}

// Go has no WebAssembly SIMD: asking for it with js.RuntimeGo is an error, not a silent plain build.
func TestWasmBuild_GoRefusesSIMD(t *testing.T) {
	_, err := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./x", OutputName: "x", Runtime: js.RuntimeGo, SIMD: true}).Build(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "SIMD needs TinyGo") {
		t.Errorf("want the SIMD-needs-TinyGo error, got: %v", err)
	}
}

// A Worker declared for Go compiles with Go's toolchain.
func TestWasmBuild_GoRuntimeCompilesWithGo(t *testing.T) {
	root := t.TempDir()
	dir := sitec.WorkersDir + "/model"
	writeWorkerPackage(t, root, map[string]string{dir + "/main.go": "//go:build wasm && !tinygo\n\npackage main\nfunc main() {}\n"})
	out, err := sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "./" + dir, OutputName: "model", Runtime: js.RuntimeGo}).Build(root)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if out.Filename != "model.wasm" || len(out.Binary) == 0 {
		t.Errorf("got %q with %d bytes", out.Filename, len(out.Binary))
	}
}

// The compiler comes from the //go:build line of the Worker's main.go; without a line that
// picks one, the error shows the two lines to choose from.
func TestWorkerRuntime_ReadsTheBuildLine(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want js.Runtime
	}{
		{"//go:build wasm && tinygo\n\npackage main\n", js.RuntimeTinyGo},
		{"//go:build wasm && !tinygo\n\npackage main\n", js.RuntimeGo},
		{"//go:build wasm\n\npackage main\n", 0},
		{"package main\n", 0},
	} {
		p := filepath.Join(t.TempDir(), "main.go")
		if err := os.WriteFile(p, []byte(tc.src), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := sitec.WorkerRuntime(p)
		if tc.want == 0 {
			if err == nil || !strings.Contains(err.Error(), "wasm && tinygo") || !strings.Contains(err.Error(), "wasm && !tinygo") {
				t.Errorf("%q: want the error showing both lines, got %v, %v", tc.src, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%q: got %v, %v; want %v", tc.src, got, err, tc.want)
		}
	}
}
