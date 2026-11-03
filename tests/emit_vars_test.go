//go:build !wasm

package sitec_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/modfind"
	"webtyp.com/sitec"
)

func setupVarTest(t *testing.T, varCode string) string {
	tmpDir, err := os.MkdirTemp("", "sitec_emit_vars_*")
	if err != nil {
		t.Fatal(err)
	}

	gomodContent := `module example.com/app

go 1.25.2

require (
	webtyp.com/html v0.0.17
)
` + webtypReplaces(t)
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomodContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, "config"), 0755); err != nil {
		t.Fatal(err)
	}

	fullCode := `package config

import "webtyp.com/html"

` + varCode

	if err := os.WriteFile(filepath.Join(tmpDir, "config", "brand.go"), []byte(fullCode), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed in tmpDir: %v, output: %s", err, string(out))
	}

	return tmpDir
}

func TestVarProducer_Pages(t *testing.T) {
	tmpDir := setupVarTest(t, `
type brand struct{ name string }

func (b brand) RenderPages() []html.Page {
	if b.name == "" {
		return nil
	}
	return []html.Page{{Path: "/", Body: "<p>" + b.name + "</p>"}}
}

var Brand = brand{name: "Acme"}
`)
	defer os.RemoveAll(tmpDir)

	finder := fakeModules([]modfind.Module{
		{Path: "example.com/app", Dir: tmpDir},
	})
	e := sitec.New(tmpDir)
	e.SetFinder(finder)
	all, err := e.ExtractAll()
	if err != nil {
		t.Fatalf("ExtractAll failed: %v", err)
	}
	if len(all) == 0 {
		t.Fatalf("expected extracted assets")
	}

	var rootAsset *sitec.Assets
	for _, a := range all {
		if a.ModuleName == "example.com/app/config" {
			rootAsset = a
			break
		}
	}
	if rootAsset == nil {
		t.Fatalf("expected asset for config package")
	}

	if len(rootAsset.Pages) != 1 {
		t.Fatalf("expected 1 page extracted from var, got %d", len(rootAsset.Pages))
	}
	p := rootAsset.Pages[0]
	if p.Path != "/" {
		t.Errorf("expected Path '/', got %s", p.Path)
	}
	if !strings.Contains(p.Body, "Acme") {
		t.Errorf("expected body to contain Acme, got %s", p.Body)
	}
}

func TestVarProducer_HTML(t *testing.T) {
	tmpDir := setupVarTest(t, `
type brand struct{ name string }

func (b brand) RenderHTML() string {
	return "<p>loading " + b.name + "</p>"
}

var Brand = brand{name: "Acme"}
`)
	defer os.RemoveAll(tmpDir)

	finder := fakeModules([]modfind.Module{
		{Path: "example.com/app", Dir: tmpDir},
	})
	e := sitec.New(tmpDir)
	e.SetFinder(finder)
	all, err := e.ExtractAll()
	if err != nil {
		t.Fatalf("ExtractAll failed: %v", err)
	}

	var rootAsset *sitec.Assets
	for _, a := range all {
		if a.ModuleName == "example.com/app/config" {
			rootAsset = a
			break
		}
	}
	if rootAsset == nil {
		t.Fatalf("expected asset for config package")
	}

	if !strings.Contains(rootAsset.HTML, "loading Acme") {
		t.Errorf("expected HTML to contain 'loading Acme', got %s", rootAsset.HTML)
	}
}

func TestVarProducer_OtherTypeIgnored(t *testing.T) {
	tmpDir := setupVarTest(t, `
type normalProducer struct{}
func (n *normalProducer) RenderHTML() string {
	return "normal"
}

var Port = 8080
var Names = []string{"a"}
var Brand = struct{}{}
`)
	defer os.RemoveAll(tmpDir)

	finder := fakeModules([]modfind.Module{
		{Path: "example.com/app", Dir: tmpDir},
	})
	e := sitec.New(tmpDir)
	e.SetFinder(finder)
	all, err := e.ExtractAll()
	if err != nil {
		t.Fatalf("ExtractAll failed: %v", err)
	}

	var rootAsset *sitec.Assets
	for _, a := range all {
		if a.ModuleName == "example.com/app/config" {
			rootAsset = a
			break
		}
	}
	if rootAsset == nil {
		t.Fatalf("expected asset for config package")
	}

	if rootAsset.HTML != "normal" {
		t.Errorf("expected HTML to be 'normal', got %s", rootAsset.HTML)
	}
}
