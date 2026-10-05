package sitec

import (
	"os"
	"path/filepath"
	"strings"

	"webtyp.com/artifacts"
	"webtyp.com/fmt"
)

// buildManifest is artifacts.BuildManifest; a variable so a test can count the hashing.
var buildManifest = artifacts.BuildManifest

// emitArtifacts (called by RouteExtractedAssets, c.mu held) writes /artifacts.json for the root's ArtifactSources() and remembers where each
// declared file is on disk. It measures the files (SHA-256 of hundreds of MB) only when one of
// them changed size or date since the last call: the daemon routes assets on every SSR change.
func (c *Compiler) emitArtifacts() error {
	if len(c.artifactSources) == 0 {
		c.large, c.largeKey = nil, ""
		return nil
	}
	var key strings.Builder
	for _, src := range c.artifactSources {
		path := src.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.RootDir, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errf("sitec: artifact %s: %v", src.ID, err)
		}
		fmt.Fprintf(&key, "%s|%s|%s|%d|%d\n", src.ID, src.Version, src.File, info.Size(), info.ModTime().UnixNano())
	}
	if key.String() == c.largeKey {
		return nil
	}
	m, files, err := buildManifest(c.RootDir, c.artifactSources)
	if err != nil {
		return err
	}
	data, err := m.Encode()
	if err != nil {
		return err
	}
	if err := c.writeLocked(strings.TrimPrefix(artifacts.ManifestPath, "/"), data, mediaTypeJSON); err != nil {
		return err
	}
	c.large, c.largeKey = files, key.String()
	return nil
}

// LargeFile returns where on disk the declared artifact served at url is (sitec/serve streams it
// in development; Build places it in release).
func (c *Compiler) LargeFile(url string) (path string, ok bool) {
	for _, f := range c.large {
		if f.URL == url {
			return f.Path, true
		}
	}
	return "", false
}

// LargeFiles returns the declared artifacts with their path on disk.
func (c *Compiler) LargeFiles() []artifacts.LocalFile { return c.large }
