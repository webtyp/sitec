package sitec

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"webtyp.com/fmt"
	"webtyp.com/js"
	"webtyp.com/js/esm"
)

type esmCacheKey struct {
	pkgDir     string
	sourceHash string
}

type esmPkgCache struct {
	mu      sync.RWMutex
	entries map[esmCacheKey]*js.Script
	hits    int
}

func newESMPkgCache() *esmPkgCache {
	return &esmPkgCache{
		entries: make(map[esmCacheKey]*js.Script),
	}
}

// CompileESMPackage compiles a strict-subset Go package directory to an ES Module Script,
// caching the result by (directory, content hash) for fast dev hot-reload.
func (c *Compiler) CompileESMPackage(dir string) (*js.Script, error) {
	c.mu.Lock()
	if c.esmCache == nil {
		c.esmCache = newESMPkgCache()
	}
	cache := c.esmCache
	c.mu.Unlock()

	hash, err := hashGoDir(dir)
	if err != nil {
		return nil, err
	}
	key := esmCacheKey{pkgDir: filepath.Clean(dir), sourceHash: hash}

	cache.mu.RLock()
	if cached, ok := cache.entries[key]; ok {
		cache.mu.RUnlock()
		cache.mu.Lock()
		cache.hits++
		cache.mu.Unlock()
		return cached, nil
	}
	cache.mu.RUnlock()

	scr, err := esm.CompilePackage(dir)
	if err != nil {
		return nil, err
	}

	cache.mu.Lock()
	cache.entries[key] = scr
	cache.mu.Unlock()
	return scr, nil
}

// ESMCacheHits reports how many times CompileESMPackage returned a cached script.
func (c *Compiler) ESMCacheHits() int {
	c.mu.Lock()
	cache := c.esmCache
	c.mu.Unlock()
	if cache == nil {
		return 0
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return cache.hits
}

func hashGoDir(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Err("failed to read package dir", dir, err)
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, p := range paths {
		_, _ = h.Write([]byte(filepath.Base(p)))
		_, _ = h.Write([]byte{0})
		f, err := os.Open(p)
		if err != nil {
			return "", fmt.Err("failed to open file", p, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", fmt.Err("failed to hash file", p, err)
		}
		f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
