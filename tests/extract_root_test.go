//go:build !wasm

package sitec_test

import (
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/modfind"
	"webtyp.com/sitec"
)

// TestExtractAll_ScansTheRootItWasGiven is the sitec half of D54 (a module's
// demo served without styles). A demo's browser main is generated outside the
// module, so the packages that declare CSS (`layout`, `components`) are only in
// the graph of THAT folder, never in the module's own. The extractor must list
// the graph from the root it is told with SetRootDir, not from the one it was
// built with.
func TestExtractAll_ScansTheRootItWasGiven(t *testing.T) {
	appDir, widgetDir := setupReachFixture(t)
	// The demo folder is a module of its own that replaces the others, like the
	// one the daemon generates (app/demo_module.go): that is what a scan of it sees.
	demoDir := t.TempDir()
	demoMod := "module webtyp.local/demo\n\ngo 1.24\n\n" +
		"require example.com/app v0.0.0\nrequire example.com/widget v0.0.0\n" +
		"replace example.com/app => " + filepath.ToSlash(appDir) + "\n" +
		"replace example.com/widget => " + filepath.ToSlash(widgetDir) + "\n"
	if err := os.WriteFile(filepath.Join(demoDir, "go.mod"), []byte(demoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(demoDir, "main.go"), []byte("//go:build wasm\n\npackage main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := sitec.New(appDir)
	e.SetLog(t.Log)
	e.SetFinder(fakeModules([]modfind.Module{
		{Path: "example.com/app", Dir: appDir},
		{Path: "example.com/widget", Dir: widgetDir},
	}))

	var listedRoots []string
	e.SetGraphLister(func(rootDir, pattern, goos, goarch string) ([]string, error) {
		listedRoots = append(listedRoots, rootDir)
		return []string{"example.com/app", "example.com/widget"}, nil
	})

	e.SetRootDir(demoDir)

	if _, err := e.ExtractAll(); err != nil {
		t.Fatalf("ExtractAll: %v", err)
	}
	if len(listedRoots) == 0 {
		t.Fatal("the graph was never listed")
	}
	for _, r := range listedRoots {
		if r != demoDir {
			t.Fatalf("graph listed from %q, want the root given with SetRootDir %q: the demo's layout/components CSS is invisible from the module's own root", r, demoDir)
		}
	}
}
