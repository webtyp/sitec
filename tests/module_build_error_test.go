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
	err     error
	runDirs []string
}

func (f *failingToolchain) List(dir string, args ...string) ([]byte, error) {
	return nil, f.err
}

func (f *failingToolchain) ListEnv(dir string, env []string, args ...string) ([]byte, error) {
	return nil, f.err
}

func (f *failingToolchain) Run(dir string, args ...string) ([]byte, error) {
	f.runDirs = append(f.runDirs, dir)
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

	// Local replace module discovered as part of appDir's module graph.
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

	// Standalone module NOT in appDir's module graph.
	standaloneMod := t.TempDir()
	standaloneSub := filepath.Join(standaloneMod, "ui")
	if err := os.MkdirAll(standaloneSub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(standaloneMod, "go.mod"), []byte("module example.com/standalone\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(standaloneSub, "css.go"), []byte("package ui\nfunc RenderCSS() string { return \"\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rawErr := errors.New("missing go.sum entry for module providing package example.com/x")
	tc := &failingToolchain{err: rawErr}

	ext := sitec.New(appDir)
	ext.SetToolchain(tc)
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

	// ExtractModule for a local replace module runs in appDir (the app's module context),
	// never inside replaceMod where app-only transitive deps are absent from go.sum.
	tc.runDirs = nil
	_, err = ext.ExtractModule(replaceSub)
	if err == nil {
		t.Fatal("expected ExtractModule to fail")
	}
	modErr = nil
	if !errors.As(err, &modErr) {
		t.Fatalf("expected *sitec.ModuleBuildError, got %T (%v)", err, err)
	}
	if modErr.ModuleDir != appDir {
		t.Errorf("ExtractModule(replaceSub) ModuleDir = %q, want appDir %q", modErr.ModuleDir, appDir)
	}
	if len(tc.runDirs) != 1 || tc.runDirs[0] != filepath.Join(appDir, ".ssr_extract") {
		t.Errorf("ExtractModule(replaceSub) ran toolchain in %v, want %q", tc.runDirs, filepath.Join(appDir, ".ssr_extract"))
	}

	// ExtractModule for a standalone module outside appDir's graph runs in standaloneMod.
	tc.runDirs = nil
	_, err = ext.ExtractModule(standaloneSub)
	if err == nil {
		t.Fatal("expected ExtractModule(standaloneSub) to fail")
	}
	modErr = nil
	if !errors.As(err, &modErr) {
		t.Fatalf("expected *sitec.ModuleBuildError, got %T (%v)", err, err)
	}
	if modErr.ModuleDir != standaloneMod {
		t.Errorf("ExtractModule(standaloneSub) ModuleDir = %q, want %q", modErr.ModuleDir, standaloneMod)
	}
	if len(tc.runDirs) != 1 || tc.runDirs[0] != filepath.Join(standaloneMod, ".ssr_extract") {
		t.Errorf("ExtractModule(standaloneSub) ran toolchain in %v, want %q", tc.runDirs, filepath.Join(standaloneMod, ".ssr_extract"))
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
