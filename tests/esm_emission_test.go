//go:build !wasm

package sitec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/js"
)

type esmModuleProvider struct {
	scripts []*js.Script
}

func (p esmModuleProvider) RenderJS() []*js.Script { return p.scripts }

func TestSitec_EmitsModuleScriptTag(t *testing.T) {
	env := setupTestEnv("esm_module_tag", t)
	am := env.AssetsHandler

	err := am.RegisterComponents(esmModuleProvider{
		scripts: []*js.Script{
			{Name: "", Content: "import { signal } from './runtime.js'; export const n = signal(1);", Module: true},
		},
	})
	if err != nil {
		t.Fatalf("RegisterComponents failed: %v", err)
	}
	if err := am.RouteExtractedAssets(nil); err != nil {
		t.Fatalf("RouteExtractedAssets failed: %v", err)
	}
	if err := am.FlushToDisk(); err != nil {
		t.Fatalf("FlushToDisk failed: %v", err)
	}

	htmlStr := indexHTML(t, am)
	if !strings.Contains(htmlStr, `type="module"`) {
		t.Fatalf("expected <script type=\"module\"> in index.html, got:\n%s", htmlStr)
	}
	if !strings.Contains(htmlStr, `integrity="sha384-`) {
		t.Fatalf("expected SRI integrity attribute in index.html, got:\n%s", htmlStr)
	}
	if _, err := os.Stat(filepath.Join(env.OutDir, "runtime.js")); err != nil {
		t.Fatalf("expected runtime.js to be emitted: %v", err)
	}
}

func TestSitec_CachesCompiledPackage(t *testing.T) {
	env := setupTestEnv("esm_pkg_cache", t)
	am := env.AssetsHandler

	pkgDir := filepath.Join(t.TempDir(), "validator")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(pkgDir, "check.go")
	if err := os.WriteFile(srcPath, []byte("package validator\n\nfunc IsNonEmpty(s string) bool { return len(s) > 0 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s1, err := am.CompileESMPackage(pkgDir)
	if err != nil {
		t.Fatalf("first CompileESMPackage failed: %v", err)
	}
	if !s1.Module || !strings.Contains(s1.Content, "export function IsNonEmpty") {
		t.Fatalf("unexpected compiled ESM: %+v", s1)
	}
	if am.ESMCacheHits() != 0 {
		t.Fatalf("expected 0 cache hits on first compile, got %d", am.ESMCacheHits())
	}

	s2, err := am.CompileESMPackage(pkgDir)
	if err != nil {
		t.Fatalf("second CompileESMPackage failed: %v", err)
	}
	if am.ESMCacheHits() != 1 || s2.Content != s1.Content {
		t.Fatalf("expected 1 cache hit with identical content, hits=%d", am.ESMCacheHits())
	}

	if err := os.WriteFile(srcPath, []byte("package validator\n\nfunc IsNonEmpty(s string) bool { return len(s) > 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s3, err := am.CompileESMPackage(pkgDir)
	if err != nil {
		t.Fatalf("third CompileESMPackage failed: %v", err)
	}
	if !strings.Contains(s3.Content, "> 1") {
		t.Fatalf("expected recompiled output after edit, got:\n%s", s3.Content)
	}
}

func TestSitec_KeepsLazyWasmWorkers(t *testing.T) {
	env := setupTestEnv("esm_lazy_workers", t)
	am := env.AssetsHandler

	err := am.RegisterComponents(esmModuleProvider{
		scripts: []*js.Script{
			{Name: "", Content: "export const app = true;", Module: true},
			js.LazyWorkerClient("pdf_client.js", "/pdf.worker.js"),
			js.WebWorker("pdf.worker.js", "/pdf.wasm", js.RuntimeTinyGo),
		},
	})
	if err != nil {
		t.Fatalf("RegisterComponents failed: %v", err)
	}
	if err := am.RouteExtractedAssets(nil); err != nil {
		t.Fatalf("RouteExtractedAssets failed: %v", err)
	}
	if err := am.FlushToDisk(); err != nil {
		t.Fatalf("FlushToDisk failed: %v", err)
	}

	for _, name := range []string{"runtime.js", "pdf_client.js", "pdf.worker.js"} {
		if _, err := os.Stat(filepath.Join(env.OutDir, name)); err != nil {
			t.Fatalf("expected %s to be emitted: %v", name, err)
		}
	}
}
