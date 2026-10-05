//go:build !wasm

package sitec_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"webtyp.com/router/mock"
	"webtyp.com/sitec"
	"webtyp.com/sitec/serve"
)

const largeURL = "/artifacts/model.v1.bin"

// largeFS is an in-memory FS that also declares one large file on disk, as the Compiler does for
// ArtifactSources().
type largeFS struct {
	sitec.FS
	path string
}

func (f largeFS) LargeFile(url string) (string, bool) { return f.path, url == largeURL }

func largeFixture(t *testing.T) (*mock.Router, []byte) {
	t.Helper()
	data := bytes.Repeat([]byte("0123456789"), 300_000) // 3 MB: more than one 1 MiB piece
	p := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	r := &mock.Router{}
	serve.RegisterRoutes(r, largeFS{FS: sitec.NewMemFS(), path: p})
	return r, data
}

func getLarge(r *mock.Router, url, rangeHeader string) *mock.Context {
	ctx := &mock.Context{InPath: url, InMethod: "GET"}
	if rangeHeader != "" {
		ctx.SetHeader("Range", rangeHeader)
	}
	r.Invoke("GET", url, ctx)
	return ctx
}

func TestServe_LargeFileRange(t *testing.T) {
	r, data := largeFixture(t)
	size := strconv.Itoa(len(data))

	ctx := getLarge(r, largeURL, "bytes=10-19")
	if ctx.Status != 206 || !bytes.Equal(ctx.ResponseBody(), data[10:20]) {
		t.Fatalf("bytes=10-19: status %d, body %q", ctx.Status, ctx.ResponseBody())
	}
	if got := ctx.GetHeader("Content-Range"); got != "bytes 10-19/"+size {
		t.Errorf("Content-Range = %q", got)
	}
	if !strings.Contains(ctx.GetHeader("Cache-Control"), "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", ctx.GetHeader("Cache-Control"))
	}

	// A range crossing the 1 MiB pieces, open-ended to the end of the file.
	from := 1<<20 - 5
	ctx = getLarge(r, largeURL, "bytes="+strconv.Itoa(from)+"-")
	if ctx.Status != 206 || !bytes.Equal(ctx.ResponseBody(), data[from:]) {
		t.Fatalf("open range: status %d, %d bytes", ctx.Status, len(ctx.ResponseBody()))
	}

	ctx = getLarge(r, largeURL, "bytes="+size+"-")
	if ctx.Status != 416 || ctx.GetHeader("Content-Range") != "bytes */"+size {
		t.Errorf("out of range: status %d, Content-Range %q", ctx.Status, ctx.GetHeader("Content-Range"))
	}

	ctx = getLarge(r, largeURL, "")
	if ctx.Status != 200 || !bytes.Equal(ctx.ResponseBody(), data) {
		t.Errorf("no range: status %d, %d bytes", ctx.Status, len(ctx.ResponseBody()))
	}
}

func TestServe_LargeFileNotDeclared(t *testing.T) {
	r, _ := largeFixture(t)
	if ctx := getLarge(r, "/artifacts/other.bin", ""); ctx.Status != 404 {
		t.Errorf("undeclared artifact: status %d, want 404", ctx.Status)
	}
}
