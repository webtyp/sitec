package sitec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"webtyp.com/fmt"
	"webtyp.com/js"
	"webtyp.com/pwa"
	"webtyp.com/tinygo"
)

// defaultBrowserEntry is the browser main, relative to the project root: one
// main per folder, the folder is the binary. It always compiles to an ES Module
// with goesm (buildBrowserModule); WebAssembly is only for Web Workers and the edge.
const defaultBrowserEntry = "web/browser/main.go"

// browserBuildTag is the destination tag of the browser binary (reach.go probes it).
const browserBuildTag = "browser"

// localPackagePrefix marks a WasmBuildOptions.Package inside the project
// ("./web/edge"); anything else is an import path ("webtyp.com/pdf/webworker").
const localPackagePrefix = "./"

// WasmBuildOptions selects what to compile, with which compiler, and what to call the result.
// Only Web Workers are WebAssembly: the page and the edge compile to ES Modules with goesm.
type WasmBuildOptions struct {
	// Package is the Go package to compile, resolved against the project's go.mod:
	// "./web/edge" or "./web/webworkers/<name>" for the project's own mains, an import
	// path such as "webtyp.com/pdf/webworker" for a library's Web Worker.
	Package string
	// OutputName is the artifact name without the .wasm extension.
	OutputName string
	// Runtime is the compiler: js.RuntimeTinyGo or js.RuntimeGo, as the package's main.go
	// declares it (WorkerRuntime).
	Runtime js.Runtime

	// SIMD builds with TinyGo +simd128 and -opt=2 (Web Workers that run models); TinyGo only:
	// Go has no WebAssembly SIMD (GOWASM accepts satconv, signext). SIMD implies Speed.
	SIMD bool
	// Speed builds TinyGo with -opt=2 (Web Workers are compiled for speed).
	Speed bool
	// HashName hashes the content and uses it in the output name.
	HashName bool
}

type defaultWasmBuilder struct {
	opts WasmBuildOptions
}

// NewWasmBuilder compiles opts.Package with opts.Runtime to <OutputName>.wasm (content-hashed
// when opts.HashName). Package, OutputName and Runtime are required.
//
// Example: sitec.NewWasmBuilder(sitec.WasmBuildOptions{Package: "webtyp.com/pdf/webworker", OutputName: "pdf", Runtime: js.RuntimeTinyGo})
func NewWasmBuilder(opts WasmBuildOptions) WasmBuilder {
	return &defaultWasmBuilder{opts: opts}
}

const simdTargetJSON = `{"inherits":["wasm"],"features":"+bulk-memory,+bulk-memory-opt,+call-indirect-overlong,+mutable-globals,+nontrapping-fptoint,+sign-ext,-multivalue,-reference-types,+simd128","cflags":["-msimd128"]}`

var execCommand = exec.Command

func (w *defaultWasmBuilder) Build(dir string) (WasmOutput, error) {
	if w.opts.Package == "" || w.opts.OutputName == "" {
		return WasmOutput{}, fmt.Err(msgWasmOptionsRequired)
	}
	if w.opts.Runtime != js.RuntimeTinyGo && w.opts.Runtime != js.RuntimeGo {
		return WasmOutput{}, fmt.Err(msgWasmRuntimeRequired)
	}
	if w.opts.Runtime == js.RuntimeGo && w.opts.SIMD {
		return WasmOutput{}, fmt.Err(msgGoHasNoWasmSIMD)
	}
	if strings.HasPrefix(w.opts.Package, localPackagePrefix) {
		mainPath := filepath.Join(dir, filepath.FromSlash(w.opts.Package), mainFile)
		if _, err := os.Stat(mainPath); err != nil {
			return WasmOutput{}, fmt.Err("input file not found: ", filepath.Join(w.opts.Package, mainFile), " must exist")
		}
	}

	wasmFilename := w.opts.OutputName + wasmExt
	tmpOutDir, err := os.MkdirTemp("", "sitec_wasm_*")
	if err != nil {
		return WasmOutput{}, err
	}
	defer os.RemoveAll(tmpOutDir)
	tmpOutPath := filepath.Join(tmpOutDir, wasmFilename)

	var cmd *exec.Cmd
	if w.opts.Runtime == js.RuntimeGo {
		cmd = execCommand("go", "build", "-o", tmpOutPath, w.opts.Package)
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	} else {
		if _, err := tinygo.EnsureInstalled(); err != nil {
			return WasmOutput{}, fmt.Err("cannot install TinyGo: ", err)
		}
		args, err := w.tinyGoArgs(tmpOutDir, tmpOutPath)
		if err != nil {
			return WasmOutput{}, err
		}
		cmd = execCommand("tinygo", args...)
		cmd.Env = append(tinygo.GetEnv(), "GOOS=js", "GOARCH=wasm")
	}
	// Run from the project root so the package resolves against the project's
	// go.mod: a library's Web Worker is built with the versions the app uses.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return WasmOutput{}, fmt.Err("compilation failed: ", string(out), err)
	}

	binary, err := os.ReadFile(tmpOutPath)
	if err != nil {
		return WasmOutput{}, err
	}
	if w.opts.HashName {
		wasmFilename = pwa.HashedName(wasmFilename, binary)
	}
	return WasmOutput{Binary: binary, Filename: wasmFilename}, nil
}

// tinyGoArgs are the TinyGo build arguments: the SIMD target or plain wasm, -opt=2 for speed.
func (w *defaultWasmBuilder) tinyGoArgs(tmpOutDir, tmpOutPath string) ([]string, error) {
	args := []string{"build"}
	if w.opts.SIMD {
		targetPath := filepath.Join(tmpOutDir, "wasm-simd.json")
		if err := os.WriteFile(targetPath, []byte(simdTargetJSON), 0644); err != nil {
			return nil, err
		}
		args = append(args, "-target", targetPath, "-opt=2")
	} else {
		args = append(args, "-target", "wasm")
		if w.opts.Speed {
			args = append(args, "-opt=2")
		}
	}
	// -no-debug: a shipped web binary is never a source-level debug target —
	// DWARF info alone is the difference between ~99 KiB and ~456 KiB for a
	// minimal binary.
	return append(args, "-no-debug", "-o", tmpOutPath, w.opts.Package), nil
}

const (
	mainFile = "main.go"
	wasmExt  = ".wasm"

	msgWasmOptionsRequired = "sitec: WasmBuildOptions needs Package and OutputName"
	msgWasmRuntimeRequired = "sitec: WasmBuildOptions needs Runtime: js.RuntimeTinyGo or js.RuntimeGo"
	msgGoHasNoWasmSIMD     = "sitec: SIMD needs TinyGo: Go has no WebAssembly SIMD"
)

// buildBrowserModule compiles web/browser (pkgDir) with goesm to one ES Module.
func buildBrowserModule(pkgDir string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "sitec-goesm-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	cmd := execCommand("goesm", "build", "-minify", "-o", tmpDir, ".")
	cmd.Dir = pkgDir
	env := os.Environ()
	if go127 := findSitecGo127Toolchain(); go127 != "" {
		env = sitecEnvOverride(env, "GOROOT", go127)
		env = sitecEnvOverride(env, "PATH", filepath.Join(go127, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Err("goesm compilation failed: ", string(out), err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, err
	}
	var binary []byte
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			binary, err = os.ReadFile(filepath.Join(tmpDir, e.Name()))
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if len(binary) == 0 {
		return nil, fmt.Err("goesm produced no .js output")
	}
	return binary, nil
}

func findSitecGo127Toolchain() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(gopath, "pkg", "mod", "golang.org", "toolchain*1.27*"))
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func sitecEnvOverride(env []string, key, val string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return append(out, prefix+val)
}
