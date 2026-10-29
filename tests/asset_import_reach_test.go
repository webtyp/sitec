//go:build !wasm

package sitec_test

import (
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/modfind"
	"webtyp.com/sitec"
)

// TestExtractAll_PackageImportedOnlyByAnAssetFileIsReachable is the shape of
// mjosefa-cms with components/targetlist → components/listgap:
//
//	app/web/browser/main.go  //go:build wasm   imports dep/list
//	dep/list/list.go         (no tag)          reached by the js/wasm graph
//	dep/list/css.go          //go:build !wasm  imports dep/gap
//	dep/gap/css.go           //go:build !wasm  no producer: a helper list uses
//
// No probe sees dep/gap: the native graph never reaches dep/list (its only
// importer is wasm-only) and the js/wasm graph drops list/css.go. The
// extractor compiles list/css.go natively all the same, so whatever it imports
// is in the extractor's build: dep/gap is reachable by definition.
func TestExtractAll_PackageImportedOnlyByAnAssetFileIsReachable(t *testing.T) {
	base := t.TempDir()
	appDir := filepath.Join(base, "app")
	depDir := filepath.Join(base, "dep")

	writeTree(t, depDir, map[string]string{
		"go.mod":       "module example.com/dep\n\ngo 1.24\n",
		"list/list.go": "package list\n\ntype List struct{}\n",
		"list/css.go": `//go:build !wasm

package list

import "example.com/dep/gap"
` + stylesheetHelper + `
func (l *List) RenderCSS() stylesheet { return stylesheet(".list{display:grid}" + gap.Rule()) }
`,
		"gap/css.go": `//go:build !wasm

package gap

// Rule is the spacing every list shares.
func Rule() string { return ".gap{margin:0}" }
`,
	})

	writeTree(t, appDir, map[string]string{
		"go.mod": `module example.com/app

go 1.24

require example.com/dep v0.0.0

replace example.com/dep => ../dep
`,
		"web/browser/main.go": `//go:build wasm

package main

import _ "example.com/dep/list"

func main() {}
`,
		"config/css.go": `package config
` + stylesheetHelper + `
type App struct{}

func (a *App) RenderCSS() stylesheet { return ".app{color:green}" }
`,
	})

	e := sitec.New(appDir)
	e.SetLog(t.Log)
	e.SetFinder(fakeModules([]modfind.Module{
		{Path: "example.com/app", Dir: appDir, IsMain: true},
		{Path: "example.com/dep", Dir: depDir},
	}))

	all, err := e.ExtractAll()
	if err != nil {
		t.Fatalf("ExtractAll: %v", err)
	}
	var css strings.Builder
	for _, a := range all {
		css.WriteString(a.RootCSS)
		css.WriteString(a.CSS)
	}
	for _, want := range []string{".app{color:green}", ".list{display:grid}.gap{margin:0}"} {
		if !strings.Contains(css.String(), want) {
			t.Errorf("style.css lacks %q\n  got: %q", want, css.String())
		}
	}
}
