//go:build !wasm

package sitec_test

import (
	"strings"
	"testing"

	"webtyp.com/html"
	"webtyp.com/sitec"
)

type multiPageProvider struct{}

func (m *multiPageProvider) RenderPages() []html.Page {
	return []html.Page{
		{
			Path: "/",
			Doc: html.DocumentOptions{
				Title:       "Home Page Title",
				Description: "Home Page Description",
			},
			Body: "<div id='home'>Home Content</div>",
		},
		{
			Path: "/especialidades/oftalmologia/",
			Doc: html.DocumentOptions{
				Title:       "Oftalmología Chillán",
				Description: "Consulta de oftalmología en Chillán",
				Canonical:   "/especialidades/oftalmologia/",
			},
			Body: "<div id='oftalmo'>Oftalmología Content</div>",
		},
	}
}

type singleHTMLProvider struct{}

func (s *singleHTMLProvider) RenderHTML() string {
	return "<div>Single Page Content</div>"
}

func TestEmitPages_MultiPageEmission(t *testing.T) {
	ac := &sitec.Config{
		OutputDir: "web/public",
		SiteURL:   "https://clinic.example.com",
	}
	am := sitec.NewCompiler(ac)

	provider := &multiPageProvider{}
	assets := &sitec.Assets{
		ModuleName: "example.com/clinic",
		Pages:      provider.RenderPages(),
	}

	err := am.RouteExtractedAssets([]*sitec.Assets{assets})
	if err != nil {
		t.Fatalf("RouteExtractedAssets failed: %v", err)
	}

	// 1. Verify index.html
	indexPath := "web/public/index.html"
	indexBytes, _, ok := am.Read(indexPath)
	if !ok {
		t.Fatalf("expected %s to exist", indexPath)
	}
	indexStr := string(indexBytes)

	if !strings.Contains(indexStr, "<title>Home Page Title</title>") {
		t.Errorf("index.html missing correct title, got: %s", indexStr)
	}
	if !strings.Contains(indexStr, "content='Home Page Description'") && !strings.Contains(indexStr, `content="Home Page Description"`) {
		t.Errorf("index.html missing home description, got: %s", indexStr)
	}
	if !strings.Contains(indexStr, "<div id='home'>Home Content</div>") && !strings.Contains(indexStr, `<div id="home">Home Content</div>`) {
		t.Errorf("index.html missing home body content, got: %s", indexStr)
	}

	// 2. Verify especialidades/oftalmologia/index.html
	subPath := "web/public/especialidades/oftalmologia/index.html"
	subBytes, _, ok := am.Read(subPath)
	if !ok {
		t.Fatalf("expected %s to exist", subPath)
	}
	subStr := string(subBytes)

	if !strings.Contains(subStr, "Oftalmología Chillán") {
		t.Errorf("subpage missing correct title, got: %s", subStr)
	}
	if !strings.Contains(subStr, "Consulta de oftalmología en Chillán") {
		t.Errorf("subpage missing description, got: %s", subStr)
	}
	if !strings.Contains(subStr, "https://clinic.example.com/especialidades/oftalmologia/") {
		t.Errorf("subpage canonical URL not resolved to absolute, got: %s", subStr)
	}
	if !strings.Contains(subStr, "Oftalmología Content") {
		t.Errorf("subpage body missing content, got: %s", subStr)
	}
	if !strings.Contains(indexStr, `href="/style.css"`) && !strings.Contains(indexStr, `href='/style.css'`) {
		t.Errorf("home page stylesheet must stay domain-root-absolute when pages exist, got: %s", indexStr)
	}
	if !strings.Contains(subStr, `href="/style.css"`) && !strings.Contains(subStr, `href='/style.css'`) {
		t.Errorf("nested page stylesheet must stay domain-root-absolute when pages exist, got: %s", subStr)
	}
}

func TestEmitPages_Coexistence_RenderHTML_and_RenderPages(t *testing.T) {
	ac := &sitec.Config{
		OutputDir: "web/public",
	}
	am := sitec.NewCompiler(ac)

	htmlAsset := &sitec.Assets{
		ModuleName: "example.com/modA",
		HTML:       "<div>App Shell</div>",
	}
	pageAsset := &sitec.Assets{
		ModuleName: "example.com/modB",
		Pages: []html.Page{
			{
				Path: "/",
				Doc:  html.DocumentOptions{Title: "Home"},
				Body: "<div>Home Page</div>",
			},
		},
	}

	err := am.RouteExtractedAssets([]*sitec.Assets{htmlAsset, pageAsset})
	if err != nil {
		t.Fatalf("expected no error when RenderHTML and RenderPages coexist, got: %v", err)
	}

	// 1. Static page index.html at root
	indexPath := "web/public/index.html"
	indexBytes, _, ok := am.Read(indexPath)
	if !ok {
		t.Fatalf("expected %s to exist", indexPath)
	}
	indexStr := string(indexBytes)
	if !strings.Contains(indexStr, "Home Page") {
		t.Errorf("index.html missing home page content, got: %s", indexStr)
	}
	if strings.Contains(indexStr, "<script src=") {
		t.Errorf("static page index.html must not contain bootstrap script tag, got: %s", indexStr)
	}
	if !strings.Contains(indexStr, `href="/style.css"`) && !strings.Contains(indexStr, `href='/style.css'`) {
		t.Errorf("static page index.html must use absolute asset paths, got: %s", indexStr)
	}

	// 2. Shell app/index.html
	appPath := "web/public/app/index.html"
	appBytes, _, ok := am.Read(appPath)
	if !ok {
		t.Fatalf("expected %s to exist for app shell", appPath)
	}
	appStr := string(appBytes)
	if !strings.Contains(appStr, "App Shell") {
		t.Errorf("app/index.html missing shell content, got: %s", appStr)
	}
	if !strings.Contains(appStr, `<script src="/script.js"`) && !strings.Contains(appStr, `<script src='/script.js'`) {
		t.Errorf("app shell app/index.html must contain bootstrap script tag, got: %s", appStr)
	}
	if !strings.Contains(appStr, `href="/style.css"`) && !strings.Contains(appStr, `href='/style.css'`) {
		t.Errorf("app shell app/index.html must use absolute asset paths, got: %s", appStr)
	}
}

func TestEmitPages_Collision_MultipleRenderHTML(t *testing.T) {
	ac := &sitec.Config{
		OutputDir: "web/public",
	}
	am := sitec.NewCompiler(ac)

	htmlAsset1 := &sitec.Assets{
		ModuleName: "example.com/modA",
		HTML:       "<div>Shell A</div>",
	}
	htmlAsset2 := &sitec.Assets{
		ModuleName: "example.com/modB",
		HTML:       "<div>Shell B</div>",
	}

	err := am.RouteExtractedAssets([]*sitec.Assets{htmlAsset1, htmlAsset2})
	if err == nil {
		t.Fatalf("expected collision error when multiple modules declare RenderHTML, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "example.com/modA") || !strings.Contains(errStr, "example.com/modB") {
		t.Errorf("expected collision error to name both modules, got: %v", err)
	}
}

func TestEmitPages_Collision_DuplicatePages(t *testing.T) {
	ac := &sitec.Config{
		OutputDir: "web/public",
	}
	am := sitec.NewCompiler(ac)

	pageAsset1 := &sitec.Assets{
		ModuleName: "example.com/mod1",
		Pages: []html.Page{
			{
				Path: "/about/",
				Doc:  html.DocumentOptions{Title: "About Mod 1"},
			},
		},
	}
	pageAsset2 := &sitec.Assets{
		ModuleName: "example.com/mod2",
		Pages: []html.Page{
			{
				Path: "/about/",
				Doc:  html.DocumentOptions{Title: "About Mod 2"},
			},
		},
	}

	err := am.RouteExtractedAssets([]*sitec.Assets{pageAsset1, pageAsset2})
	if err == nil {
		t.Fatalf("expected collision error for duplicate page paths, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "example.com/mod1") || !strings.Contains(errStr, "example.com/mod2") {
		t.Errorf("expected collision error to name both modules, got: %v", err)
	}
}

func TestEmitPages_RenderHTML_Only_Regression(t *testing.T) {
	ac := &sitec.Config{
		OutputDir: "web/public",
	}
	am := sitec.NewCompiler(ac)

	htmlAsset := &sitec.Assets{
		ModuleName: "example.com/legacy",
		HTML:       "<div>Legacy Single Page</div>",
	}

	err := am.RouteExtractedAssets([]*sitec.Assets{htmlAsset})
	if err != nil {
		t.Fatalf("unexpected error for RenderHTML-only module: %v", err)
	}

	indexPath := "web/public/index.html"
	indexBytes, _, ok := am.Read(indexPath)
	if !ok {
		t.Fatalf("expected %s to exist", indexPath)
	}

	if !strings.Contains(string(indexBytes), "Legacy Single Page") {
		t.Errorf("index.html missing legacy content, got: %s", string(indexBytes))
	}
}

func TestEmitPages_RenderHTML_Only_RelativeAssetPaths(t *testing.T) {
	ac := &sitec.Config{
		OutputDir: "web/public",
	}
	am := sitec.NewCompiler(ac)

	htmlAsset := &sitec.Assets{
		ModuleName: "example.com/app",
		HTML:       "<div>App Shell</div>",
	}

	err := am.RouteExtractedAssets([]*sitec.Assets{htmlAsset})
	if err != nil {
		t.Fatalf("unexpected error for RenderHTML-only module: %v", err)
	}

	indexPath := "web/public/index.html"
	indexBytes, _, ok := am.Read(indexPath)
	if !ok {
		t.Fatalf("expected %s to exist", indexPath)
	}
	indexStr := string(indexBytes)

	if !strings.Contains(indexStr, `href="style.css"`) && !strings.Contains(indexStr, `href='style.css'`) {
		t.Errorf("expected relative stylesheet href when no pages are declared, got: %s", indexStr)
	}
	if !strings.Contains(indexStr, `src="script.js"`) && !strings.Contains(indexStr, `src='script.js'`) {
		t.Errorf("expected relative script src when no pages are declared, got: %s", indexStr)
	}
	if strings.Contains(indexStr, `href="/style.css"`) || strings.Contains(indexStr, `src="/script.js"`) {
		t.Errorf("asset paths must not be domain-root-absolute when no pages are declared, got: %s", indexStr)
	}
}
