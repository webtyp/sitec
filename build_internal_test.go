// Root-level test (justified): exercises linkFile, placeLargeFiles — the large-file placement step of Build, observable from outside only by building a real artifacts app.
//go:build !wasm

package sitec

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/artifacts"
)

func TestPlaceLargeFiles_CopiesWhenLinkFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "model.bin")
	data := bytes.Repeat([]byte("weights"), 1<<16)
	if err := os.WriteFile(src, data, 0644); err != nil {
		t.Fatal(err)
	}
	linked := 0
	linkFile = func(string, string) error { linked++; return errors.New("cross-device link") }
	t.Cleanup(func() { linkFile = os.Link })

	out := filepath.Join(dir, "out")
	err := placeLargeFiles(out, []artifacts.LocalFile{{URL: "/artifacts/model.v1.bin", Path: src}})
	if err != nil {
		t.Fatalf("placeLargeFiles: %v", err)
	}
	if linked != 1 {
		t.Errorf("link tried %d times, want 1", linked)
	}
	got, err := os.ReadFile(filepath.Join(out, "artifacts", "model.v1.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("copy differs from the source")
	}
}
