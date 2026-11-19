//go:build !wasm

package sitec_test

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"webtyp.com/sitec"
	"webtyp.com/svg"
	"webtyp.com/svg/sprite"
)

func expectedSRI(b []byte) string {
	sum := sha512.Sum384(b)
	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
}

func TestEmitHTML_NoInlineScriptStyleOrSVG(t *testing.T) {
	env := setupTestEnv("no_inline_csp", t)
	am := env.AssetsHandler
	am.SetTranslations(&fakeTranslations{payload: []byte(`{"default":"es"}`)})
	s := sprite.NewSprite(sprite.Define(svg.Icon("ic-plus"), "0 0 16 16", sprite.Path("M1 2h3")))
	if err := am.UpdateSSRModule("mod", ".x{color:red}", nil, "<main>Hi</main>", s); err != nil {
		t.Fatal(err)
	}
	if err := am.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	html := indexHTML(t, am)
	for _, forbidden := range []string{"<svg", "<style", "style="} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("index.html must not contain %q under strict CSP:\n%s", forbidden, html)
		}
	}
	if strings.Contains(html, `<script type="application/json"`) {
		t.Fatalf("index.html must not inline translations script:\n%s", html)
	}
}

func TestEmitHTML_SRIIntegrityMatchesAssets(t *testing.T) {
	env := setupTestEnv("sri_integrity", t)
	am := env.AssetsHandler
	if err := am.UpdateSSRModule("mod", ".box { color: blue; }", nil, "<main>Hi</main>", nil); err != nil {
		t.Fatal(err)
	}
	if err := am.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	html := indexHTML(t, am)

	cssBytes, err := am.GetMinifiedCSS()
	if err != nil {
		t.Fatal(err)
	}
	jsBytes, err := am.GetMinifiedJS()
	if err != nil {
		t.Fatal(err)
	}

	wantCSS := `integrity="` + expectedSRI(cssBytes) + `" crossorigin="anonymous"`
	wantJS := `integrity="` + expectedSRI(jsBytes) + `" crossorigin="anonymous"`
	if !strings.Contains(html, wantCSS) {
		t.Errorf("index.html missing CSS SRI %s:\n%s", wantCSS, html)
	}
	if !strings.Contains(html, wantJS) {
		t.Errorf("index.html missing JS SRI %s:\n%s", wantJS, html)
	}
}

func TestEmitCSPManifest_ContainsScriptHashes(t *testing.T) {
	env := setupTestEnv("csp_manifest", t)
	am := env.AssetsHandler
	if err := am.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	if err := am.RegenerateHTMLCache(); err != nil {
		t.Fatal(err)
	}

	jsBytes, err := am.GetMinifiedJS()
	if err != nil {
		t.Fatal(err)
	}
	wantHash := expectedSRI(jsBytes)

	raw, _, ok := am.Read("/" + sitec.CSPManifestFile)
	if !ok {
		t.Fatalf("missing /%s artifact", sitec.CSPManifestFile)
	}
	var m sitec.CSPManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal csp.json: %v", err)
	}
	if len(m.ScriptHashes) != 1 || m.ScriptHashes[0] != wantHash {
		t.Fatalf("csp.json script_hashes = %v, want [%s]", m.ScriptHashes, wantHash)
	}
}
