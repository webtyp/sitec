package sitec

import (
	"strings"
	"testing"

	"webtyp.com/image/favicon"
	"webtyp.com/pwa"
)

func TestHashedName(t *testing.T) {
	content1 := []byte("body { color: red; }")
	name1 := pwa.HashedName("style.css", content1)
	if !strings.HasPrefix(name1, "style.") || !strings.HasSuffix(name1, ".css") {
		t.Fatalf("unexpected name format: %s", name1)
	}

	name2 := pwa.HashedName("style.css", content1)
	if name1 != name2 {
		t.Fatalf("expected identical hash for identical content: %s vs %s", name1, name2)
	}

	content2 := []byte("body { color: blue; }")
	name3 := pwa.HashedName("style.css", content2)
	if name1 == name3 {
		t.Fatalf("expected different hash for changed content: %s", name1)
	}

	licenseName := pwa.HashedName("LICENSE", content1)
	if !strings.HasPrefix(licenseName, "LICENSE.") || strings.Contains(licenseName, ".css") {
		t.Fatalf("unexpected extensionless hashed name: %s", licenseName)
	}
}

func TestFinalizeRelease_RenamesAndRewritesHTML(t *testing.T) {
	arts := []Artifact{
		{Path: "/", Mediatype: "text/html", Content: []byte(`<!doctype html><html><head><link rel="stylesheet" href="/style.css"></head><body><script src="/script.js"></script><svg><use href="#icon"></use></svg></body></html>`)},
		{Path: "/style.css", Mediatype: "text/css", Content: []byte("body{margin:0}")},
		{Path: "/script.js", Mediatype: "text/javascript", Content: []byte("console.log('hi')")},
	}

	in := releaseInput{
		CSSURL: "/style.css",
		JSURL:  "/script.js",
	}

	out, err := finalizeRelease(arts, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var htmlContent string
	var foundCSS, foundJS bool
	for _, a := range out {
		if a.Path == "/" {
			htmlContent = string(a.Content)
		}
		if a.Path == "/style.css" {
			foundCSS = true
		}
		if a.Path == "/script.js" {
			foundJS = true
		}
		if a.Path == pwa.ManifestPath || a.Path == pwa.ServiceWorkerPath {
			t.Fatalf("unexpected PWA artifact in release without PWA: %s", a.Path)
		}
	}

	if foundCSS || foundJS {
		t.Fatalf("unhashed assets should have been renamed: CSS=%v, JS=%v", foundCSS, foundJS)
	}

	if strings.Contains(htmlContent, `"/style.css"`) || strings.Contains(htmlContent, `"/script.js"`) {
		t.Fatalf("HTML still references unhashed URLs: %s", htmlContent)
	}
}

func TestFinalizeRelease_RelativeURLsStayRelative(t *testing.T) {
	arts := []Artifact{
		{Path: "/", Mediatype: "text/html", Content: []byte(`<!doctype html><html><head><link rel="stylesheet" href="style.css"></head><body><script src="script.js"></script></body></html>`)},
		{Path: "style.css", Mediatype: "text/css", Content: []byte("body{margin:0}")},
		{Path: "script.js", Mediatype: "text/javascript", Content: []byte("console.log('hi')")},
	}

	in := releaseInput{
		CSSURL: "style.css",
		JSURL:  "script.js",
	}

	out, err := finalizeRelease(arts, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, a := range out {
		if a.Mediatype == "text/css" || a.Mediatype == "text/javascript" {
			if strings.HasPrefix(a.Path, "/") {
				t.Fatalf("relative URL became absolute: %s", a.Path)
			}
		}
	}
}

func TestFinalizeRelease_PWA(t *testing.T) {
	arts := []Artifact{
		{Path: "/", Mediatype: "text/html", Content: []byte(`<!doctype html><html><head></head><body></body></html>`)},
		{Path: "/script.js", Mediatype: "text/javascript", Content: []byte("console.log('pwa')")},
	}

	favicons := []favicon.File{
		{Name: "icon-192.png", Sizes: "192x192", Mediatype: "image/png"},
		{Name: "icon-512.png", Sizes: "512x512", Mediatype: "image/png"},
	}

	pwaCfg := &pwa.Config{
		Name:            "Test App",
		ShortName:       "Test",
		ThemeColor:      "#000000",
		BackgroundColor: "#ffffff",
	}

	in := releaseInput{
		JSURL:    "/script.js",
		PWA:      pwaCfg,
		Favicons: favicons,
	}

	out, err := finalizeRelease(arts, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasManifest, hasSW bool
	var swContent string
	var htmlContent string
	var jsPath string

	for _, a := range out {
		if a.Path == pwa.ManifestPath {
			hasManifest = true
		}
		if a.Path == pwa.ServiceWorkerPath {
			hasSW = true
			swContent = string(a.Content)
		}
		if a.Mediatype == "text/html" {
			htmlContent = string(a.Content)
		}
		if a.Mediatype == "text/javascript" && a.Path != pwa.ServiceWorkerPath {
			jsPath = a.Path
		}
	}

	if !hasManifest || !hasSW {
		t.Fatalf("missing PWA manifest or service worker: manifest=%v, sw=%v", hasManifest, hasSW)
	}

	if !strings.Contains(htmlContent, `<link rel="manifest"`) {
		t.Fatalf("HTML missing manifest link: %s", htmlContent)
	}

	if !strings.Contains(swContent, jsPath) {
		t.Fatalf("SW precache list missing hashed JS path %s: %s", jsPath, swContent)
	}

	if strings.Contains(swContent, pwa.ServiceWorkerPath) {
		t.Fatalf("SW precache list must not contain sw.js itself")
	}
}

func TestFinalizeRelease_PWAHashesFinalFiles(t *testing.T) {
	arts := []Artifact{
		{Path: "/", Mediatype: "text/html", Content: []byte(`<!doctype html><html><head></head><body></body></html>`)},
		{Path: "/script.js", Mediatype: "text/javascript", Content: []byte("console.log('init')")},
	}

	favicons := []favicon.File{
		{Name: "icon-192.png", Sizes: "192x192", Mediatype: "image/png"},
		{Name: "icon-512.png", Sizes: "512x512", Mediatype: "image/png"},
	}

	pwaCfg := &pwa.Config{
		Name:            "App",
		ThemeColor:      "#000000",
		BackgroundColor: "#ffffff",
	}

	in := releaseInput{
		JSURL:    "/script.js",
		PWA:      pwaCfg,
		Favicons: favicons,
	}

	out, err := finalizeRelease(arts, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var jsArt Artifact
	for _, a := range out {
		if a.Mediatype == "text/javascript" && a.Path != pwa.ServiceWorkerPath {
			jsArt = a
			break
		}
	}

	expectedName := pwa.HashedName("script.js", jsArt.Content)
	if jsArt.Path != "/"+expectedName {
		t.Fatalf("expected JS name to reflect content after register script append: got %s, expected /%s", jsArt.Path, expectedName)
	}
}

func TestFinalizeRelease_PWAWithoutFavicon(t *testing.T) {
	arts := []Artifact{
		{Path: "/", Mediatype: "text/html", Content: []byte(`<!doctype html><html><head></head><body></body></html>`)},
	}

	pwaCfg := &pwa.Config{
		Name:            "App",
		ThemeColor:      "#000000",
		BackgroundColor: "#ffffff",
	}

	in := releaseInput{
		PWA: pwaCfg,
	}

	_, err := finalizeRelease(arts, in)
	if err == nil {
		t.Fatalf("expected error when PWA declared without Favicon")
	}

	if err.Error() != msgPWAWithoutFavicon() {
		t.Fatalf("expected error %q, got %q", msgPWAWithoutFavicon(), err.Error())
	}
}

func TestFinalizeRelease_PWASmallLogo(t *testing.T) {
	arts := []Artifact{
		{Path: "/", Mediatype: "text/html", Content: []byte(`<!doctype html><html><head></head><body></body></html>`)},
	}

	// Only 192, missing 512
	favicons := []favicon.File{
		{Name: "icon-192.png", Sizes: "192x192", Mediatype: "image/png"},
	}

	pwaCfg := &pwa.Config{
		Name:            "App",
		ThemeColor:      "#000000",
		BackgroundColor: "#ffffff",
	}

	in := releaseInput{
		PWA:      pwaCfg,
		Favicons: favicons,
	}

	_, err := finalizeRelease(arts, in)
	if err == nil {
		t.Fatalf("expected error when 512x512 icon is missing")
	}

	if !strings.Contains(err.Error(), "512x512") {
		t.Fatalf("expected error mentioning 512x512, got: %v", err)
	}
}

func TestFinalizeRelease_HTMLWithoutHead(t *testing.T) {
	arts := []Artifact{
		{Path: "/bad.html", Mediatype: "text/html", Content: []byte(`<div>no head tag here</div>`)},
	}

	favicons := []favicon.File{
		{Name: "icon-192.png", Sizes: "192x192", Mediatype: "image/png"},
		{Name: "icon-512.png", Sizes: "512x512", Mediatype: "image/png"},
	}

	pwaCfg := &pwa.Config{
		Name:            "App",
		ThemeColor:      "#000000",
		BackgroundColor: "#ffffff",
	}

	in := releaseInput{
		PWA:      pwaCfg,
		Favicons: favicons,
	}

	_, err := finalizeRelease(arts, in)
	if err == nil {
		t.Fatalf("expected error when HTML has no </head>")
	}

	if !strings.Contains(err.Error(), "/bad.html") {
		t.Fatalf("expected error to name artifact /bad.html, got: %v", err)
	}
}

// Large artifacts under pwa.ArtifactsDir live in OPFS (webtyp/artifacts): the service worker must
// never precache hundreds of MB of model weights into Cache Storage.
func TestFinalizeRelease_ArtifactsNeverPrecached(t *testing.T) {
	arts := []Artifact{
		{Path: "/", Mediatype: "text/html", Content: []byte(`<html><head></head><body><script src="/script.js"></script></body></html>`)},
		{Path: "/script.js", Mediatype: "text/javascript", Content: []byte("1")},
		{Path: "/artifacts.json", Mediatype: "application/json", Content: []byte("{}")},
		{Path: pwa.ArtifactsDir + "decider-0.8b.q4.wtypw", Mediatype: "application/octet-stream", Content: []byte("weights")},
	}
	icons := []favicon.File{
		{Name: "icon-192.png", Sizes: "192x192", Mediatype: "image/png"},
		{Name: "icon-512.png", Sizes: "512x512", Mediatype: "image/png"},
	}
	cfg := pwa.Config{Name: "App", ThemeColor: "#000", BackgroundColor: "#fff"}
	out, err := finalizeRelease(arts, releaseInput{JSURL: "/script.js", PWA: &cfg, Favicons: icons})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range out {
		if a.Path == pwa.ServiceWorkerPath {
			if strings.Contains(string(a.Content), pwa.ArtifactsDir) {
				t.Errorf("service worker precaches a large artifact:\n%s", a.Content)
			}
			if !strings.Contains(string(a.Content), "/artifacts.json") {
				t.Error("the artifacts manifest itself is a small shell file and must be precached")
			}
			return
		}
	}
	t.Fatal("no service worker emitted")
}
