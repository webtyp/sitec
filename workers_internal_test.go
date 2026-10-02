package sitec

import (
	"os/exec"
	"strings"
	"testing"
)

func TestWorkers_SIMDBinaryUsesSIMDTarget(t *testing.T) {
	origExecCommand := execCommand
	defer func() { execCommand = origExecCommand }()

	var capturedArgs []string
	execCommand = func(name string, arg ...string) *exec.Cmd {
		if strings.HasSuffix(name, "tinygo") || name == "tinygo" {
			capturedArgs = append(capturedArgs, arg...)
		}
		// Return a command that will just exit 0, or just run 'echo'
		return exec.Command("echo") // a safe mock command
	}

	builder := NewWasmBuilder(false, WasmBuildOptions{
		Entry:      "web/workers/echo/main.go",
		OutputName: "echo.simd",
		SIMD:       true,
		Speed:      true,
		HashName:   false,
	})

	// Let's create a fake entry file.
	tmp := t.TempDir()
	path := tmp + "/web/workers/echo/main.go"

	// Create it relative to tmp.
	_ = exec.Command("mkdir", "-p", tmp+"/web/workers/echo").Run()
	_ = exec.Command("touch", path).Run()

	_, _ = builder.Build(tmp)

	// We can check capturedArgs.
	hasTargetWasmSimd := false
	hasOpt2 := false
	for _, arg := range capturedArgs {
		if strings.Contains(arg, "wasm-simd.json") {
			hasTargetWasmSimd = true
		}
		if arg == "-opt=2" {
			hasOpt2 = true
		}
	}

	if !hasTargetWasmSimd {
		t.Errorf("Expected tinygo to be called with a target file ending in wasm-simd.json, args were: %v", capturedArgs)
	}
	if !hasOpt2 {
		t.Errorf("Expected tinygo to be called with -opt=2, args were: %v", capturedArgs)
	}
}
