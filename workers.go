package sitec

import (
	"os"
	"path/filepath"
	"regexp"

	"webtyp.com/fmt"
	"webtyp.com/js"
)

const workersDir = "web/workers"

var workerNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func buildWorkers(root string, c *Compiler, devMode bool) error {
	wDir := filepath.Join(root, workersDir)
	entries, err := os.ReadDir(wDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !workerNameRe.MatchString(name) {
			return fmt.Errf("sitec: worker directory %q: use lowercase letters, digits and dashes", name)
		}

		mainGo := filepath.Join(wDir, name, "main.go")
		if _, err := os.Stat(mainGo); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		entryPath := filepath.Join(workersDir, name, "main.go")

		if devMode {
			opts := WasmBuildOptions{
				Entry:      entryPath,
				OutputName: name,
				Speed:      true,
				HashName:   false,
			}
			builder := NewWasmBuilder(true, opts)
			out, err := builder.Build(root)
			if err != nil {
				return err
			}

			if err := c.Write(out.Filename, out.Binary, "application/wasm"); err != nil {
				return err
			}

			plainScript := js.WebWorker(name+".worker.js", "/"+out.Filename)
			simdScript := js.WebWorker(name+".simd.worker.js", "/"+out.Filename)

			if err := c.Write(plainScript.Name, []byte(plainScript.Content), "text/javascript"); err != nil {
				return err
			}
			if err := c.Write(simdScript.Name, []byte(simdScript.Content), "text/javascript"); err != nil {
				return err
			}
		} else {
			// Plain build
			plainOpts := WasmBuildOptions{
				Entry:      entryPath,
				OutputName: name,
				Speed:      true,
				HashName:   true,
			}
			plainBuilder := NewWasmBuilder(false, plainOpts)
			plainOut, err := plainBuilder.Build(root)
			if err != nil {
				return err
			}
			if err := c.Write(plainOut.Filename, plainOut.Binary, "application/wasm"); err != nil {
				return err
			}

			// SIMD build
			simdOpts := WasmBuildOptions{
				Entry:      entryPath,
				OutputName: name + ".simd",
				SIMD:       true,
				Speed:      true,
				HashName:   true,
			}
			simdBuilder := NewWasmBuilder(false, simdOpts)
			simdOut, err := simdBuilder.Build(root)
			if err != nil {
				return err
			}
			if err := c.Write(simdOut.Filename, simdOut.Binary, "application/wasm"); err != nil {
				return err
			}

			plainScript := js.WebWorker(name+".worker.js", "/"+plainOut.Filename)
			simdScript := js.WebWorker(name+".simd.worker.js", "/"+simdOut.Filename)

			if err := c.Write(plainScript.Name, []byte(plainScript.Content), "text/javascript"); err != nil {
				return err
			}
			if err := c.Write(simdScript.Name, []byte(simdScript.Content), "text/javascript"); err != nil {
				return err
			}
		}
	}

	return nil
}
