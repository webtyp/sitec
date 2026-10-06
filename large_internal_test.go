// Root-level test (justified): exercises buildManifest, emitArtifacts — manifest construction for large files, internal to the artifacts emitter.
//go:build !wasm

package sitec

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"webtyp.com/artifacts"
)

// Hashing hundreds of MB on every SSR change would stall the daemon: the manifest is measured
// again only when a declared file changes.
func TestCompiler_ManifestCachedUntilFileChanges(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "m.bin")
	if err := os.WriteFile(file, []byte("weights"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	orig := buildManifest
	buildManifest = func(r string, s []artifacts.Source) (artifacts.Manifest, []artifacts.LocalFile, error) {
		calls++
		return orig(r, s)
	}
	t.Cleanup(func() { buildManifest = orig })

	c := NewCompiler(&Config{RootDir: root})
	c.SetFS(NewMemFS())
	c.artifactSources = []artifacts.Source{{ID: "m", Version: "v1", File: "m.bin"}}
	for i := 0; i < 2; i++ {
		if err := c.emitArtifacts(); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("hashed %d times for an unchanged file, want 1", calls)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(file, later, later); err != nil {
		t.Fatal(err)
	}
	if err := c.emitArtifacts(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("hashed %d times after the file changed, want 2", calls)
	}
}
