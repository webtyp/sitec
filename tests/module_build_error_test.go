package sitec_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/modfind"
	"webtyp.com/sitec"
)

type failingToolchain struct {
	err error
}

func (f failingToolchain) List(dir string, args ...string) ([]byte, error) {
	return nil, f.err
}

func (f failingToolchain) ListEnv(dir string, env []string, args ...string) ([]byte, error) {
	return nil, f.err
}

func (f failingToolchain) Run(dir string, args ...string) ([]byte, error) {
	return nil, f.err
}

type staticFinder struct {
	mods []modfind.Module
	err  error
}

func (s staticFinder) Discover(rootDir string) ([]modfind.Module, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.mods, nil
}

func TestExtractor_ModuleBuildError_ExtractAllAndExtractModule(t *testing.T) {
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte("module example.com/app\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "css.go"), []byte("package app\nfunc RenderCSS() string { return \"\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	replaceMod := t.TempDir()
	replaceSub := filepath.Join(replaceMod, "ui")
	if err := os.MkdirAll(replaceSub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replaceMod, "go.mod"), []byte("module example.com/replaced\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replaceSub, "css.go"), []byte("package ui\nfunc RenderCSS() string { return \"\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rawErr := errors.New("missing go.sum entry for module providing package example.com/x")

	ext := sitec.New(appDir)
	ext.SetToolchain(failingToolchain{err: rawErr})
	ext.SetFinder(staticFinder{mods: []modfind.Module{
		{Path: "example.com/app", Dir: appDir},
		{Path: "example.com/replaced", Dir: replaceMod},
	}})

	_, err := ext.ExtractAll()
	if err == nil {
		t.Fatal("expected ExtractAll to fail")
	}
	var modErr *sitec.ModuleBuildError
	if !errors.As(err, &modErr) {
		t.Fatalf("expected *sitec.ModuleBuildError, got %T (%v)", err, err)
	}
	if modErr.ModuleDir != appDir {
		t.Errorf("ExtractAll ModuleDir = %q, want %q", modErr.ModuleDir, appDir)
	}

	// ExtractModule from a nested subdir of a local replace module reports the replace module root.
	_, err = ext.ExtractModule(replaceSub)
	if err == nil {
		t.Fatal("expected ExtractModule to fail")
	}
	modErr = nil
	if !errors.As(err, &modErr) {
		t.Fatalf("expected *sitec.ModuleBuildError, got %T (%v)", err, err)
	}
	if modErr.ModuleDir != replaceMod {
		t.Errorf("ExtractModule ModuleDir = %q, want %q", modErr.ModuleDir, replaceMod)
	}

	// Discover failure in ExtractAll also carries the scan root.
	extDiscoverFail := sitec.New(appDir)
	extDiscoverFail.SetFinder(staticFinder{err: rawErr})
	_, err = extDiscoverFail.ExtractAll()
	modErr = nil
	if !errors.As(err, &modErr) {
		t.Fatalf("expected *sitec.ModuleBuildError on discover failure, got %T (%v)", err, err)
	}
	if modErr.ModuleDir != appDir {
		t.Errorf("discover failure ModuleDir = %q, want %q", modErr.ModuleDir, appDir)
	}
}
