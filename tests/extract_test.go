package sitec_test

import (
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/modfind"
	"webtyp.com/sitec"
)

// A project whose modules declare no assets is a normal state (a new or
// minimal project): ExtractAll reports "nothing found", not a failure.
func TestExtractAll_EmptyIsNotAnError(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\ngo 1.24\n"), 0644)
	e := sitec.New(root)
	f := fakeModules([]modfind.Module{{Path: "example.com/demo", Dir: root}})
	e.SetFinder(f)

	all, err := e.ExtractAll()
	if err != nil {
		t.Fatalf("an empty extraction must not be an error, got: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected no assets, got %d", len(all))
	}
}

func TestExtractModule_NoSSRFiles(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\ngo 1.24\n"), 0644)
	e := sitec.New(root)
	a, err := e.ExtractModule(root)
	if err != nil {
		t.Fatal(err)
	}
	if a != nil {
		t.Error("expected nil for module with no SSR files")
	}
}
