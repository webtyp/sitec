//go:build !wasm

package sitec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/artifacts"
	"webtyp.com/sitec"
)

// The daemon drives a Compiler with an in-memory FS; routing the extracted assets is enough for
// it to publish /artifacts.json and know where each declared file is on disk.
func TestCompiler_EmitsManifestOnRoute(t *testing.T) {
	root := t.TempDir()
	data := writeArtifactsApp(t, root, false, false)

	c := sitec.NewCompiler(&sitec.Config{RootDir: root, OutputDir: "web/public", DevMode: true})
	c.SetFS(sitec.NewMemFS())
	all, err := sitec.New(root).ExtractAll()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RouteExtractedAssets(all); err != nil {
		t.Fatal(err)
	}
	raw, _, ok := c.Read("/artifacts.json")
	if !ok {
		t.Fatal("no /artifacts.json in the Compiler")
	}
	m, err := artifacts.ParseManifest(raw)
	if err != nil || len(m.Artifacts) != 1 {
		t.Fatalf("manifest %s: %v", raw, err)
	}
	p, ok := c.LargeFile(m.Artifacts[0].URL)
	if !ok || p != filepath.Join(root, artifactsModelPath) {
		t.Fatalf("LargeFile(%s) = %q, %v", m.Artifacts[0].URL, p, ok)
	}
	if got, _ := os.ReadFile(p); len(got) != len(data) {
		t.Errorf("the path does not point at the declared file")
	}
}

// In development a Worker is built once (TinyGo, SIMD) and both scripts load it.
func TestCompiler_BuildWorkers(t *testing.T) {
	root := t.TempDir()
	writeWorkerFixture(t, root, "echo")
	c := sitec.NewCompiler(&sitec.Config{RootDir: root, OutputDir: "web/public", DevMode: true})
	c.SetFS(sitec.NewMemFS())
	if err := c.BuildWorkers(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/echo.worker.js", "/echo.simd.worker.js"} {
		js, _, ok := c.Read(name)
		if !ok || !strings.Contains(string(js), "/echo.wasm") {
			t.Errorf("%s missing or not pointing at /echo.wasm", name)
		}
	}
	if names, err := sitec.WorkerNames(root); err != nil || len(names) != 1 || names[0] != "echo" {
		t.Errorf("WorkerNames = %v, %v", names, err)
	}
}
