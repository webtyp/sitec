//go:build !wasm

package sitec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/sitec"
)

func writeFixtureFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

const fixtureAppGo = `package app

type stylesheet string

func (s stylesheet) String() string {
	return string(s)
}

func RootCSS() stylesheet {
	return stylesheet("body { color: red; }")
}
`

// Case 1: Project with no routes/routes.go -> Routes() empty, Build succeeds.
func TestRoutes_NoRoutesFile(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/app\n\ngo 1.25.2\n")
	writeFixtureFile(t, root, "app.go", fixtureAppGo)

	site, err := sitec.Build(sitec.BuildConfig{
		RootDir: root,
		Mode:    sitec.ModeRelease,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(site.Routes()) != 0 {
		t.Errorf("expected 0 routes, got %d", len(site.Routes()))
	}
}

// Case 2: Project with three routes -> Routes() returns them in source order.
func TestRoutes_ThreeRoutesInOrder(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/app\n\ngo 1.25.2\n")
	writeFixtureFile(t, root, "app.go", fixtureAppGo)
	writeFixtureFile(t, root, "routes/routes.go", `package routes

import "webtyp.com/router"

func Routes(r router.Router) {
	r.Get("/api/v1/health", nil)
	r.Post("/api/v1/users", nil)
	r.Delete("/api/v1/users/{id}", nil)
}
`)

	site, err := sitec.Build(sitec.BuildConfig{
		RootDir: root,
		Mode:    sitec.ModeRelease,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	routes := site.Routes()
	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(routes))
	}
	if routes[0].Method != "GET" || routes[0].Path != "/api/v1/health" {
		t.Errorf("route 0 mismatch: %+v", routes[0])
	}
	if routes[1].Method != "POST" || routes[1].Path != "/api/v1/users" {
		t.Errorf("route 1 mismatch: %+v", routes[1])
	}
	if routes[2].Method != "DELETE" || routes[2].Path != "/api/v1/users/{id}" {
		t.Errorf("route 2 mismatch: %+v", routes[2])
	}
}

// Case 3: A route /style.css against a produced style.css artifact -> Build returns verbatim error with right line number, writes nothing.
func TestRoutes_CollisionBuildFails(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/app\n\ngo 1.25.2\n")
	writeFixtureFile(t, root, "app.go", fixtureAppGo)
	writeFixtureFile(t, root, "routes/routes.go", `package routes

import "webtyp.com/router"

func Routes(r router.Router) {
	r.Get("/style.css", nil)
}
`)

	outDir := filepath.Join(root, "out")
	_, err := sitec.Build(sitec.BuildConfig{
		RootDir:   root,
		Mode:      sitec.ModeRelease,
		OutputDir: outDir,
	})
	if err == nil {
		t.Fatal("expected error due to route collision with static asset, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "route GET /style.css (routes/routes.go:6) collides with the static asset /style.css") &&
		!strings.Contains(errMsg, "route GET /style.css (routes/routes.go:6) collides with the static asset style.css") {
		t.Errorf("error message mismatch: %v", errMsg)
	}

	// Verify nothing was written to output directory
	if _, err := os.Stat(outDir); err == nil {
		t.Errorf("output directory %s should not exist when build fails", outDir)
	}
}

// Case 4: The same collision case through Check -> reported as finding/error.
func TestRoutes_CollisionCheckReportsError(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/app\n\ngo 1.25.2\n")
	writeFixtureFile(t, root, "app.go", fixtureAppGo)
	writeFixtureFile(t, root, "routes/routes.go", `package routes

import "webtyp.com/router"

func Routes(r router.Router) {
	r.Get("/style.css", nil)
}
`)

	_, err := sitec.Check(root, t.Log)
	if err == nil {
		t.Fatal("expected Check to fail on route collision, got nil")
	}
	if !strings.Contains(err.Error(), "collides with the static asset") {
		t.Errorf("unexpected error message from Check: %v", err)
	}
}

// Case 5: A route /api/contacto with no matching artifact -> no error.
func TestRoutes_NoCollisionSuccess(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/app\n\ngo 1.25.2\n")
	writeFixtureFile(t, root, "app.go", fixtureAppGo)
	writeFixtureFile(t, root, "routes/routes.go", `package routes

import "webtyp.com/router"

func Routes(r router.Router) {
	r.Get("/api/contacto", nil)
}
`)

	site, err := sitec.Build(sitec.BuildConfig{
		RootDir: root,
		Mode:    sitec.ModeRelease,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(site.Routes()) != 1 {
		t.Fatalf("expected 1 route, got %d", len(site.Routes()))
	}
}

// Case 6: A parameterised route /api/orders/{id} -> no collision against /api/orders.
func TestRoutes_ParameterizedRouteNoCollisionPrefix(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/app\n\ngo 1.25.2\n")
	writeFixtureFile(t, root, "app.go", fixtureAppGo)
	writeFixtureFile(t, root, "routes/routes.go", `package routes

import "webtyp.com/router"

func Routes(r router.Router) {
	r.Get("/script.js/{id}", nil)
}
`)

	site, err := sitec.Build(sitec.BuildConfig{
		RootDir: root,
		Mode:    sitec.ModeRelease,
	})
	if err != nil {
		t.Fatalf("expected no collision for parameterized subpath /script.js/{id}, got: %v", err)
	}
	if len(site.Routes()) != 1 {
		t.Errorf("expected 1 route, got %d", len(site.Routes()))
	}
}

// Case 7: Malformed path/declaration in routes/routes.go -> Build fails with routescan error.
func TestRoutes_ScanErrorFailsBuild(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/app\n\ngo 1.25.2\n")
	writeFixtureFile(t, root, "app.go", fixtureAppGo)
	writeFixtureFile(t, root, "routes/routes.go", `package routes

import "webtyp.com/router"

func Routes(r router.Router) {
	path := "/dynamic"
	r.Get(path, nil)
}
`)

	_, err := sitec.Build(sitec.BuildConfig{
		RootDir: root,
		Mode:    sitec.ModeRelease,
	})
	if err == nil {
		t.Fatal("expected Build to fail when routescan fails, got nil")
	}
}
