package sitec_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"webtyp.com/sitec"
)

// fakeTranslations is the test double for sitec.Translations: it records the
// calls and returns a configurable element or error.
type fakeTranslations struct {
	mu        sync.Mutex
	syncs     int
	bundles   int
	html      string
	bundleErr error
}

func (f *fakeTranslations) SyncTranslations(string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncs++
	return nil
}

func (f *fakeTranslations) BundleTranslations(string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bundles++
	return f.html, f.bundleErr
}

func (f *fakeTranslations) set(html string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.html, f.bundleErr = html, err
}

const langElement = `<script type="application/json" id="webtyp-lang">{"default":"es"}</script>`

func newTranslationsCompiler(t *testing.T) (*sitec.Compiler, *[]string) {
	t.Helper()
	var logs []string
	c := sitec.NewCompiler(&sitec.Config{RootDir: t.TempDir(), OutputDir: "web/public"})
	c.SetLog(func(m ...any) {
		for _, v := range m {
			if e, ok := v.(error); ok {
				logs = append(logs, e.Error())
			}
		}
	})
	c.SetFS(sitec.NewMemFS())
	return c, &logs
}

func indexHTML(t *testing.T, c *sitec.Compiler) string {
	t.Helper()
	if err := c.RegenerateHTMLCache(); err != nil {
		t.Fatal(err)
	}
	return string(c.GetCachedHTML())
}

func TestTranslations_InlinedAfterScan(t *testing.T) {
	c, _ := newTranslationsCompiler(t)
	f := &fakeTranslations{html: langElement}
	c.SetTranslations(f)

	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	if f.syncs != 1 {
		t.Errorf("SyncTranslations calls = %d, want 1", f.syncs)
	}
	got := indexHTML(t, c)
	if strings.Count(got, `id="webtyp-lang"`)+strings.Count(got, `id=webtyp-lang`) != 1 {
		t.Fatalf("want the dictionary element exactly once, got:\n%s", got)
	}
	el := strings.Index(got, "webtyp-lang")
	app := strings.Index(got, `id="app"`)
	if app < 0 {
		app = strings.Index(got, `id=app`)
	}
	if app < 0 {
		t.Fatalf("no #app mount point in:\n%s", got)
	}
	if el > app {
		t.Errorf("dictionary must come before #app:\n%s", got)
	}
	if js := strings.Index(got, "<script src"); js >= 0 && el > js {
		t.Errorf("dictionary must come before the client script:\n%s", got)
	}
}

func TestTranslations_LangJSONEditRebundlesWithoutSync(t *testing.T) {
	c, _ := newTranslationsCompiler(t)
	f := &fakeTranslations{html: langElement}
	c.SetTranslations(f)
	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}

	f.set(`<script type="application/json" id="webtyp-lang">{"default":"fr"}</script>`, nil)
	if err := c.NewFileEvent("lang.json", ".json", "/p/config/lang.json", "write"); err != nil {
		t.Fatal(err)
	}
	if f.syncs != 1 {
		t.Errorf("a lang.json edit must not sync: SyncTranslations calls = %d, want 1", f.syncs)
	}
	if got := indexHTML(t, c); !strings.Contains(got, `"fr"`) {
		t.Errorf("HTML must carry the re-bundled dictionary, got:\n%s", got)
	}

	// Any other .json is not ours.
	before := f.bundles
	if err := c.NewFileEvent("package.json", ".json", "/p/package.json", "write"); err != nil {
		t.Fatal(err)
	}
	if f.bundles != before {
		t.Errorf("a non-dictionary .json must not re-bundle")
	}
}

func TestTranslations_BundleErrorKeepsPreviousAndLogsOnce(t *testing.T) {
	c, logs := newTranslationsCompiler(t)
	f := &fakeTranslations{html: langElement}
	c.SetTranslations(f)
	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}

	f.set("", errors.New("boom"))
	for i := 0; i < 3; i++ {
		if err := c.RouteExtractedAssets(nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := indexHTML(t, c); !strings.Contains(got, "webtyp-lang") {
		t.Errorf("a bundle error must keep the previous element, got:\n%s", got)
	}
	n := 0
	for _, l := range *logs {
		if strings.Contains(l, "boom") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("a persisting error must be logged once, logged %d times: %v", n, *logs)
	}
}

func TestTranslations_NoProviderNoElement(t *testing.T) {
	c, _ := newTranslationsCompiler(t)
	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	if got := indexHTML(t, c); strings.Contains(got, "webtyp-lang") {
		t.Errorf("without SetTranslations no dictionary is inlined, got:\n%s", got)
	}
}
