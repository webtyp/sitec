package sitec

import (
	"os"
	"path/filepath"
	"regexp"

	"webtyp.com/fmt"
	"webtyp.com/js"
)

// WorkersDir holds the project's Web Workers: every web/workers/<name>/main.go is one.
const WorkersDir = "web/workers"

var workerNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

const mediaTypeWasm = "application/wasm"

// WorkerNames returns the names of the project's Workers (directories under WorkersDir with a
// main.go), or an error naming a directory that is not a valid name.
func WorkerNames(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, WorkersDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !workerNameRe.MatchString(name) {
			return nil, fmt.Errf("sitec: worker directory %q: use lowercase letters, digits and dashes", name)
		}
		if _, err := os.Stat(filepath.Join(root, WorkersDir, name, "main.go")); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

// BuildWorkers builds every Worker of the project into the Compiler's FS: in release twice, plain
// and SIMD, with content-hashed names; in development once, with TinyGo, the SIMD target and
// -opt=2, as <name>.wasm, both scripts pointing to it (a Worker runs heavy code such as a model,
// which Go's WebAssembly runs several times slower; development machines have SIMD).
func (c *Compiler) BuildWorkers() error {
	return buildWorkers(c.RootDir, c, c.DevMode)
}

func buildWorkers(root string, c *Compiler, devMode bool) error {
	names, err := WorkerNames(root)
	if err != nil {
		return err
	}
	for _, name := range names {
		entry := filepath.Join(WorkersDir, name, "main.go")
		var plain, simd string
		if devMode {
			out, err := buildWorker(root, c, WasmBuildOptions{Entry: entry, OutputName: name, SIMD: true, Speed: true})
			if err != nil {
				return err
			}
			plain, simd = out, out
		} else {
			if plain, err = buildWorker(root, c, WasmBuildOptions{Entry: entry, OutputName: name, Speed: true, HashName: true}); err != nil {
				return err
			}
			if simd, err = buildWorker(root, c, WasmBuildOptions{Entry: entry, OutputName: name + ".simd", SIMD: true, Speed: true, HashName: true}); err != nil {
				return err
			}
		}
		for _, s := range []*js.Script{
			js.WebWorker(name+".worker.js", "/"+plain),
			js.WebWorker(name+".simd.worker.js", "/"+simd),
		} {
			if err := c.Write(s.Name, []byte(s.Content), "text/javascript"); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildWorker compiles one Worker binary with TinyGo, writes it and returns its file name.
func buildWorker(root string, c *Compiler, opts WasmBuildOptions) (string, error) {
	out, err := NewWasmBuilder(false, opts).Build(root)
	if err != nil {
		return "", err
	}
	return out.Filename, c.Write(out.Filename, out.Binary, mediaTypeWasm)
}
