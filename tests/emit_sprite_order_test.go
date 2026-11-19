//go:build !wasm

package sitec_test

import (
	"strings"
	"testing"

	"webtyp.com/html"
	"webtyp.com/sitec"
	"webtyp.com/svg/sprite"
)

// TestPagesCarryEveryModulesIcons reproduces what shipped in mjosefa-website:
// the root project owns the pages, its dependencies own the glyphs, and the
// emitted index.html carried an EMPTY sprite — every <use href="#…"> in the
// markup pointed at a symbol that was not there, so no icon in the site drew.
//
// The order below is the one that matters: the page owner arrives FIRST, the
// module contributing icons after it. Rendering pages while still walking the
// module list bakes in whatever the sprite happens to hold at that moment.
func TestPagesCarryEveryModulesIcons(t *testing.T) {
	am := sitec.NewCompiler(&sitec.Config{OutputDir: "web/public"})

	pageOwner := &sitec.Assets{
		ModuleName: "example.com/site",
		IsRoot:     true,
		Pages: []html.Page{
			{
				Path: "/",
				Doc:  html.DocumentOptions{Title: "Home"},
				Body: `<svg><use href="#nav-menu"/></svg>`,
			},
		},
	}

	iconOwner := &sitec.Assets{
		ModuleName: "example.com/components",
		Icons: sprite.NewSprite(
			sprite.Define("nav-menu", "0 0 24 24",
				sprite.Path("M3 18h18v-2H3v2zm0-5h18v-2H3v2zm0-7v2h18V6H3z"),
			),
		),
	}

	if err := am.RouteExtractedAssets([]*sitec.Assets{pageOwner, iconOwner}); err != nil {
		t.Fatalf("RouteExtractedAssets failed: %v", err)
	}

	pageBytes, _, ok := am.Read("web/public/index.html")
	if !ok {
		t.Fatal("expected index.html to be emitted")
	}
	page := string(pageBytes)

	var spriteBody string
	for _, a := range am.List() {
		if strings.HasPrefix(strings.TrimPrefix(a.Path, "/"), "sprite.") && strings.HasSuffix(a.Path, ".svg") {
			spriteBody = string(a.Content)
		}
	}
	if !strings.Contains(spriteBody, `id="nav-menu"`) {
		t.Errorf("external sprite is missing the dependency's symbol:\n%s", spriteBody)
	}
	if !strings.Contains(page, `#nav-menu`) || !strings.Contains(page, `sprite.`) {
		t.Errorf("index.html must rewrite <use href> to external sprite.<hash>.svg#nav-menu:\n%s", page)
	}
}
