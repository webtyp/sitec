package serve

import (
	"os"
	"path"
	"strconv"
	"strings"

	"webtyp.com/pwa"
	"webtyp.com/router"
	"webtyp.com/sitec"
)

const (
	notFoundBody  = "404 no encontrado"
	mediatypeText = "text/plain; charset=utf-8"
	mediatypeBin  = "application/octet-stream"
	largeChunk    = 1 << 20 // bytes read and written at a time from a large file
)

// largeFiles is what the Compiler offers for the files declared by ArtifactSources(): they are
// too large to hold in its FS, so they are streamed from disk.
type largeFiles interface {
	LargeFile(url string) (path string, ok bool)
}

// RegisterRoutes expone el FS completo bajo una sola ruta comodín.
//
// Una ruta por artefacto no sirve: se registrarían en un instante y el sitio
// se completa después, así que el servidor quedaba sirviendo la foto del
// arranque —un style.css sin el CSS de ninguna dependencia— y las rutas
// nacidas más tarde no existían.
func RegisterRoutes(r router.Router, fs sitec.FS) {
	r.PublicAsset("/", func(ctx router.Context) {
		key := ctx.Path()

		if strings.HasPrefix(key, pwa.ArtifactsDir) {
			if lf, ok := fs.(largeFiles); ok {
				if p, ok := lf.LargeFile(key); ok {
					serveLarge(ctx, key, p)
					return
				}
			}
		}

		content, mediatype, ok := fs.Read(key)
		if !ok {
			lastSegment := path.Base(key)
			if !strings.Contains(lastSegment, ".") {
				retryKey := strings.TrimRight(key, "/") + "/"
				content, mediatype, ok = fs.Read(retryKey)
			}
		}

		if !ok {
			ctx.SetHeader("Content-Type", mediatypeText)
			ctx.WriteStatus(404)
			ctx.Write([]byte(notFoundBody))
			return
		}

		ctx.SetHeader("Content-Type", mediatype)

		isDevMutableText := strings.Contains(mediatype, "text/")
		if isDevMutableText ||
			strings.Contains(mediatype, "text/html") ||
			strings.Contains(mediatype, "application/javascript") ||
			strings.Contains(mediatype, "text/javascript") {
			ctx.SetHeader("Cache-Control", "no-cache, no-store, must-revalidate")
		} else {
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
		}

		ctx.Write(content)
	})
}

// serveLarge streams the file at p with Range support (webtyp.com/artifacts downloads in 8 MiB
// ranges and requires 206), in largeChunk pieces: never the whole range in one buffer.
func serveLarge(ctx router.Context, url, p string) {
	f, err := os.Open(p)
	if err != nil {
		notFound(ctx)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		notFound(ctx)
		return
	}
	size := info.Size()
	ctx.SetHeader("Content-Type", mediatypeBin)
	ctx.SetHeader("Accept-Ranges", "bytes")
	ctx.SetHeader("Cache-Control", pwa.CacheControl(url))

	start, end := int64(0), size-1
	status := 200
	if rh := ctx.GetHeader("Range"); rh != "" {
		a, b, ok := parseRange(rh, size)
		if !ok {
			ctx.SetHeader("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
			ctx.WriteStatus(416)
			return
		}
		start, end, status = a, b, 206
		ctx.SetHeader("Content-Range", "bytes "+strconv.FormatInt(a, 10)+"-"+strconv.FormatInt(b, 10)+"/"+strconv.FormatInt(size, 10))
	}
	ctx.SetHeader("Content-Length", strconv.FormatInt(end-start+1, 10))
	ctx.WriteStatus(status)

	buf := make([]byte, largeChunk)
	for off := start; off <= end; {
		n := int64(len(buf))
		if rest := end - off + 1; rest < n {
			n = rest
		}
		read, err := f.ReadAt(buf[:n], off)
		if read > 0 {
			if _, werr := ctx.Write(buf[:read]); werr != nil {
				return
			}
			off += int64(read)
		}
		if err != nil {
			return
		}
	}
}

// parseRange reads one "bytes=a-b" or "bytes=a-" range; b is clamped to size−1.
func parseRange(h string, size int64) (start, end int64, ok bool) {
	spec, found := strings.CutPrefix(h, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	a, b, found := strings.Cut(spec, "-")
	if !found || a == "" {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(a, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end = size - 1
	if b != "" {
		e, err := strconv.ParseInt(b, 10, 64)
		if err != nil || e < start {
			return 0, 0, false
		}
		if e < end {
			end = e
		}
	}
	return start, end, true
}

func notFound(ctx router.Context) {
	ctx.SetHeader("Content-Type", mediatypeText)
	ctx.WriteStatus(404)
	ctx.Write([]byte(notFoundBody))
}
