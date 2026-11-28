package sitec_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A content-hashed asset (sprite.<hash>.svg, lang.<hash>.json) keeps only its current version in
// web/public. webtyp dev mirrors the site there on every start, and each start used to add its
// version next to the ones earlier sessions left (29 lang.*.json and 21 sprite.*.svg in
// mjosefa-cms after a week). The hash stays: it is what lets a browser and the service worker
// cache the file forever (pwa.CacheImmutable).
func TestHashedAssets_FlushRemovesVersionsLeftByEarlierSessions(t *testing.T) {
	env := setupTestEnv("hashed_assets_stale", t)
	if err := os.MkdirAll(env.OutDir, 0755); err != nil {
		t.Fatal(err)
	}
	stale := []string{"sprite.deadbeef.svg", "sprite.0badf00d.svg", "lang.12345678.json"}
	unrelated := []string{"logo.12345678.png", "sprite.svg.bak", "spritex.deadbeef.svg"}
	for _, name := range append(append([]string{}, stale...), unrelated...) {
		if err := os.WriteFile(filepath.Join(env.OutDir, name), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := env.AssetsHandler.FlushToDisk(); err != nil {
		t.Fatal(err)
	}

	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(env.OutDir, name)); err == nil {
			t.Errorf("%s is a version an earlier session left: FlushToDisk must remove it", name)
		}
	}
	for _, name := range unrelated {
		if _, err := os.Stat(filepath.Join(env.OutDir, name)); err != nil {
			t.Errorf("%s is not a version of a file sitec hashes: it must stay (%v)", name, err)
		}
	}
	current, _ := filepath.Glob(filepath.Join(env.OutDir, "sprite.*.svg"))
	if len(current) != 1 {
		t.Errorf("want exactly the current sprite.<hash>.svg, got %v", current)
	}
}
