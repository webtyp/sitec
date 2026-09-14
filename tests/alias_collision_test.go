//go:build !wasm

package sitec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/modfind"
	"webtyp.com/sitec"
)

func TestExtractAll_AliasCollisionAcrossPackages(t *testing.T) {
	baseDir := t.TempDir()
	appDir := filepath.Join(baseDir, "app")
	libDir := filepath.Join(baseDir, "appointment_booking")

	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(appDir, "go.mod"), `module example.com/app

go 1.24

require example.com/appointment_booking v0.0.0
replace example.com/appointment_booking => ../appointment_booking
`)
	write(filepath.Join(appDir, "main.go"), "package main\n\nfunc main() {}\n")

	write(filepath.Join(appDir, "modules", "appointment_booking", "css.go"), `//go:build !wasm

package appointment_booking

type stylesheet string

func (s stylesheet) String() string { return string(s) }

type LocalModule struct{}

func (m *LocalModule) RenderCSS() stylesheet { return ".local-booking{color:blue}" }
`)

	write(filepath.Join(libDir, "go.mod"), "module example.com/appointment_booking\n\ngo 1.24\n")
	write(filepath.Join(libDir, "css.go"), `//go:build !wasm

package appointment_booking

type stylesheet string

func (s stylesheet) String() string { return string(s) }

type UpstreamLib struct{}

func (u *UpstreamLib) RenderCSS() stylesheet { return ".upstream-booking{color:green}" }
`)

	e := sitec.New(appDir)
	e.SetLog(t.Log)
	f := modfind.New()
	f.Seed(appDir, []modfind.Module{
		{Path: "example.com/app", Dir: appDir},
		{Path: "example.com/appointment_booking", Dir: libDir},
	})
	e.SetFinder(f)

	e.SetGraphLister(func(rootDir, pattern, goos, goarch string) ([]string, error) {
		return []string{
			"example.com/app",
			"example.com/app/modules/appointment_booking",
			"example.com/appointment_booking",
		}, nil
	})

	all, err := e.ExtractAll()
	if err != nil {
		t.Fatalf("ExtractAll failed: %v", err)
	}

	var sawAppCSS, sawUpstreamCSS bool
	for _, a := range all {
		if a.ModuleName == "example.com/app" && strings.Contains(a.CSS, ".local-booking") {
			sawAppCSS = true
		}
		if a.ModuleName == "example.com/appointment_booking" && strings.Contains(a.CSS, ".upstream-booking") {
			sawUpstreamCSS = true
		}
	}

	if !sawAppCSS {
		t.Error("expected local module example.com/app CSS to be present in extracted assets")
	}
	if !sawUpstreamCSS {
		t.Error("expected upstream module example.com/appointment_booking CSS to be present in extracted assets")
	}
}
