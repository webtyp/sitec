package sitec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The SIMD build hands TinyGo a target file that enables simd128, and compiles for speed.
func TestWorkers_SIMDBinaryUsesSIMDTarget(t *testing.T) {
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })

	var args []string
	var target string
	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "tinygo" {
			args = arg
			for i, a := range arg {
				if a == "-target" && i+1 < len(arg) {
					data, _ := os.ReadFile(arg[i+1]) // read now: the temp dir is removed after Build
					target = string(data)
				}
			}
		}
		return exec.Command("true")
	}

	root := t.TempDir()
	entry := filepath.Join(root, "web", "workers", "echo", "main.go")
	if err := os.MkdirAll(filepath.Dir(entry), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _ = NewWasmBuilder(false, WasmBuildOptions{
		Entry: "web/workers/echo/main.go", OutputName: "echo.simd", SIMD: true, Speed: true,
	}).Build(root) // the stub writes no binary: only the arguments matter here

	if !strings.Contains(target, "+simd128") {
		t.Errorf("the -target file does not enable simd128: %q (args %v)", target, args)
	}
	hasOpt2 := false
	for _, a := range args {
		hasOpt2 = hasOpt2 || a == "-opt=2"
	}
	if !hasOpt2 {
		t.Errorf("tinygo args lack -opt=2: %v", args)
	}
}

// In development a Worker is built once, with TinyGo, the SIMD target and -opt=2 — never with Go.
func TestWorkers_DevBuildsTinyGoSIMDOnce(t *testing.T) {
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })
	var tools []string
	var target string
	execCommand = func(name string, arg ...string) *exec.Cmd {
		tools = append(tools, name)
		for i, a := range arg {
			if a == "-target" && i+1 < len(arg) {
				data, _ := os.ReadFile(arg[i+1])
				target = string(data)
			}
		}
		return exec.Command("true")
	}

	root := t.TempDir()
	entry := filepath.Join(root, WorkersDir, "echo", "main.go")
	if err := os.MkdirAll(filepath.Dir(entry), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c := NewCompiler(&Config{RootDir: root, DevMode: true})
	c.SetFS(NewMemFS())
	_ = c.BuildWorkers() // the stub writes no binary: only the commands matter

	if len(tools) != 1 || tools[0] != "tinygo" {
		t.Fatalf("commands = %v, want one tinygo build", tools)
	}
	if !strings.Contains(target, "+simd128") {
		t.Errorf("dev worker target = %q, want simd128", target)
	}
}
