package sitec_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"webtyp.com/sitec"
)

// fakeTranslations is the test double for sitec.Translations: it records the
// calls and returns a configurable JSON payload or error.
type fakeTranslations struct {
	mu        sync.Mutex
	syncs     int
	bundles   int
	payload   []byte
	bundleErr error
}

func (f *fakeTranslations) SyncTranslations(string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncs++
	return nil
}

func (f *fakeTranslations) BundleTranslations(string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bundles++
	return f.payload, f.bundleErr
}

func (f *fakeTranslations) set(payload []byte, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payload, f.bundleErr = payload, err
}

var langPayload = []byte(`{"default":"es"}`)

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

func TestTranslations_EmitsExternalFileAndMetaAfterScan(t *testing.T) {
	c, _ := newTranslationsCompiler(t)
	f := &fakeTranslations{payload: langPayload}
	c.SetTranslations(f)

	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	if f.syncs != 1 {
		t.Errorf("SyncTranslations calls = %d, want 1", f.syncs)
	}
	got := indexHTML(t, c)
	if strings.Contains(got, "<script type=\"application/json\"") || strings.Contains(got, "id=\"webtyp-lang\"") {
		t.Fatalf("index.html must not inline dictionary <script>, got:\n%s", got)
	}
	if !strings.Contains(got, `name="webtyp-lang"`) {
		t.Fatalf("index.html must carry <meta name=\"webtyp-lang\">, got:\n%s", got)
	}
	var langArt *sitec.Artifact
	for _, a := range c.List() {
		a := a
		if strings.HasPrefix(strings.TrimPrefix(a.Path, "/"), "lang.") && strings.HasSuffix(a.Path, ".json") {
			langArt = &a
		}
	}
	if langArt == nil || string(langArt.Content) != string(langPayload) {
		t.Fatalf("expected external lang.<hash>.json artifact with %s, got %+v", langPayload, langArt)
	}
}

func TestTranslations_RefreshRebundlesWithoutSync(t *testing.T) {
	c, _ := newTranslationsCompiler(t)
	f := &fakeTranslations{payload: langPayload}
	c.SetTranslations(f)
	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}

	f.set([]byte(`{"default":"fr"}`), nil)
	c.RefreshTranslations()
	if f.syncs != 1 {
		t.Errorf("a lang.json edit must not sync: SyncTranslations calls = %d, want 1", f.syncs)
	}
	var found bool
	for _, a := range c.List() {
		if strings.HasPrefix(strings.TrimPrefix(a.Path, "/"), "lang.") && strings.Contains(string(a.Content), `"fr"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("external lang.<hash>.json must carry the re-bundled dictionary")
	}
}

func TestTranslations_BundleErrorKeepsPreviousAndStaysSilent(t *testing.T) {
	c, logs := newTranslationsCompiler(t)
	f := &fakeTranslations{payload: langPayload}
	c.SetTranslations(f)
	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}

	f.set(nil, errors.New("boom"))
	for i := 0; i < 3; i++ {
		if err := c.RouteExtractedAssets(nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := indexHTML(t, c); !strings.Contains(got, "webtyp-lang") {
		t.Errorf("a bundle error must keep the previous meta tag, got:\n%s", got)
	}
	if len(*logs) != 0 {
		t.Errorf("the provider reports its own errors; the compiler must not: %v", *logs)
	}
}

func TestTranslations_NoProviderNoElement(t *testing.T) {
	c, _ := newTranslationsCompiler(t)
	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	if got := indexHTML(t, c); strings.Contains(got, "webtyp-lang") {
		t.Errorf("without SetTranslations no dictionary meta is emitted, got:\n%s", got)
	}
}

// rootExtractor returns one fixed module for every extraction.
type rootExtractor struct{}

func (rootExtractor) ExtractModule(string) (*sitec.Assets, error) {
	return &sitec.Assets{ModuleName: "root", IsRoot: true}, nil
}

func (rootExtractor) ExtractAll() ([]*sitec.Assets, error) {
	return []*sitec.Assets{{ModuleName: "root", IsRoot: true}}, nil
}

// TestTranslations_SyncOncePerStartup walks the app's startup order: the
// synchronous root reload, the full scan, then the watcher registering the
// files that already exist ("scan" events). The dictionary is synced once —
// each extra sync repeated every lang diagnostic in the TUI.
func TestTranslations_SyncOncePerStartup(t *testing.T) {
	c, _ := newTranslationsCompiler(t)
	f := &fakeTranslations{payload: langPayload}
	c.SetTranslations(f)
	c.SetSSRExtractor(rootExtractor{})
	c.EnableSSRMode()

	if err := c.ReloadSSRModule("/p"); err != nil {
		t.Fatal(err)
	}
	if err := c.RouteExtractedAssets(nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"style.css", "main.js", "icons.svg", "index.html"} {
		if err := c.NewFileEvent(name, name[strings.LastIndex(name, "."):], "/p/docs/"+name, "scan"); err != nil {
			t.Fatal(err)
		}
	}
	if f.syncs != 1 {
		t.Errorf("SyncTranslations calls at startup = %d, want 1", f.syncs)
	}

	// A real edit after startup still reloads its module and syncs.
	if err := c.ReloadSSRModule("/p"); err != nil {
		t.Fatal(err)
	}
	if f.syncs != 2 {
		t.Errorf("a module reload after the scan must sync: calls = %d, want 2", f.syncs)
	}
}
