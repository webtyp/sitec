---
PLAN: "feat: Artifacts() — sitec writes /artifacts.json and places the declared large files under /artifacts/"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `sitec`: a project declares its large artifacts

Master plan: [PWA_ARTIFACTS_MASTER_PLAN.md](https://github.com/webtyp/app/blob/main/docs/PWA_ARTIFACTS_MASTER_PLAN.md),
decisions D-PWA-6 and D-PWA-15. Read [AGENTS.md](../AGENTS.md) first (tests in `tests/`, white-box
`*_internal_test.go` at the root only for unexported code, messages through `lang.Translate`, this is
a build tool: the standard library is correct here).

## Why

An application with an in-browser model needs a manifest of its large files (id, version, URL, size,
SHA-256, device requirement) at `/artifacts.json`, and the files themselves served under
`/artifacts/` from the same origin. `webtyp.com/artifacts` v0.1.1 (published) already has both
halves of the contract:

```go
// in the project, a build-only file of the ROOT package (like Favicon() and PWA()):
//go:build !wasm
func Artifacts() []artifacts.Source {
	return []artifacts.Source{{ID: "decider-0.8b", Version: "q4-2026-09",
		File: "models/decider.wtypw", Needs: device.Requirement{MinTier: device.TierSIMD}}}
}

// in sitec:
m, files, err := artifacts.BuildManifest(root, srcs) // streams each file through SHA-256
data, err := m.Encode()                               // the JSON artifacts.ParseManifest reads
// files[i].URL == pwa.ArtifactsDir + "<id>.<version><ext>", files[i].Path == absolute path on disk
```

The files are hundreds of MB: they must **never** be read into memory, never be an `Artifact` in
the in-memory FS, and never be precached by the service worker (the release pass already skips
`pwa.ArtifactsDir`). `sitec` only measures them (through `BuildManifest`) and places them on disk
next to the output with a hard link (a copy when linking fails).

## Design gate

1. **Prior art.** Hugging Face Hub / WebLLM model records (id, revision, size, sha256 per file),
   Ollama manifests (digest + size per blob), Vite's `public/` copy (files copied verbatim, not
   bundled). We take: the manifest is generated from the files, never written by hand; big files are
   placed, not bundled.
2. **Novice-name test.** `func Artifacts() []artifacts.Source` — "this project's artifacts", next to
   `Favicon()` and `PWA()`. `Output.LargeFiles()` — "the large files this build places".
3. **Complexity ledger.** Project: +1 declaration. sitec API: +1 method (`Output.LargeFiles`), used
   by deployers that do not write to disk (goflare later). Ways to publish an artifact manifest: 1.
4. **Where it belongs.** Manifest format and measuring: `artifacts`. Extraction, emission and
   placement: here, like `Favicon()` (image) and `PWA()` (pwa).
5. **What it deletes.** Nothing (new capability).

## Stage 1 — extract `Artifacts()` exactly like `PWA()`

Follow every place `PWA` appears in the extraction path (it was added in v0.2.37) and add
`Artifacts` next to it:

- `scanner.go`: `"Artifacts": true` in `producerNames`.
- `select.go`: `case "Artifacts": rf.HasArtifacts = true`.
- `extract.go`: `receiverFeature.HasArtifacts`; `CollectorOutput.Artifacts []artifacts.Source
  \`json:"artifacts"\``; the raw struct; the collector template in both branches
  (`s.Artifacts = inst.Artifacts()` / `{{$alias}}.Artifacts()`); import `webtyp.com/artifacts` in the
  generated collector only when some module has `HasArtifacts` (mirror `HasAnyPWA`). `Source` and
  `device.Requirement` have only exported plain fields: `encoding/json` round-trips them.
- `assets.go`: `Assets.Artifacts []artifacts.Source // declared by Artifacts(); root only`.
- `pipeline.go`: copy into `Assets` where `PWA` is copied.
- `merge.go`: a second declaration in the same module is an error with the same wording pattern as
  PWA's: `ssr: multiple Artifacts() declarations: <a> and <b> — only one package per module may declare Artifacts()`.
- `emit_core.go` `RouteExtractedAssets`, next to the PWA step: non-root → error
  `msgArtifactsNonRoot(moduleName)` (same shape as `msgPWANonRoot`); store the root's sources in a
  new field `c.artifactSources []artifacts.Source`.

## Stage 2 — emit the manifest, remember the files

`build.go` `buildPipeline`, after `copyStaticAssets` and **before** the release pass
(`finalizeRelease`), in both modes:

```go
if len(c.artifactSources) > 0 {
	m, files, err := artifacts.BuildManifest(root, c.artifactSources)
	if err != nil { return nil, err }
	data, err := m.Encode()
	if err != nil { return nil, err }
	if err := c.Write(strings.TrimPrefix(artifacts.ManifestPath, "/"), data, mediaTypeJSON); err != nil { return nil, err }
	largeFiles = files
}
```

`mediaTypeJSON = "application/json"` as a constant. The manifest is small and **is** part of the
precached shell (the release pass keeps it; only `pwa.ArtifactsDir` is skipped).

`Output` gets a field `large []artifacts.LocalFile` and:

```go
// LargeFiles returns the declared artifacts this build places under pwa.ArtifactsDir, with the
// path of each on disk. They are never part of Artifacts(): they are too large to hold in memory.
func (s *Output) LargeFiles() []artifacts.LocalFile
```

`checkRouteCollisions` must also see the large files' URLs: build a `[]Artifact{{Path: f.URL}}`
list (no content) for them and include it in the check, so a route named `/artifacts/...` collides
loudly.

## Stage 3 — place the files on disk

`Build(rootDir, outDir)` (the one-shot that writes to disk; it already wipes `outPath`): after
`out.WriteTo(NewOsFS())`, call new unexported `placeLargeFiles(outPath, out.LargeFiles())`:

- destination `filepath.Join(outPath, filepath.FromSlash(strings.TrimPrefix(f.URL, "/")))`;
  `os.MkdirAll` its directory;
- `linkFile(f.Path, dest)` (`var linkFile = os.Link`, for the test); when it fails (other filesystem, Windows without permission), stream a
  copy with `io.Copy` between `os.Open` / `os.Create` — never `os.ReadFile`;
- any error is returned, naming the file: `sitec: placing %s: %v`.

The dev daemon path (`Compiler` without `Build`) is out of scope: in development an application
tests downloads after a release `Build` (write that sentence in the docs).

## Stage 4 — tests

| Test | Where | Proves |
|---|---|---|
| `TestArtifacts_BuildPlacesFilesAndManifest` | `tests/` | a fixture project (pattern: `tests/pwa_build_test.go`) whose root declares `Artifacts()` with one 3 MiB file under `models/`; `sitec.Build(root, "out")` → `out/artifacts/<id>.<version>.<ext>` exists with identical bytes; `out/artifacts.json` parses with `artifacts.ParseManifest`, its `sha256` equals the file's |
| `TestArtifacts_NotInMemoryArtifacts` | `tests/` | `BuildWithConfig` → no `Artifact` path starts with `pwa.ArtifactsDir`; `LargeFiles()` has one entry |
| `TestArtifacts_ReleasePWAPrecachesManifestNotFiles` | `tests/` | same project also declaring `Favicon()` (512 PNG) and `PWA()` (with colors) in release → `sw.js` contains `/artifacts.json` and not `/artifacts/` |
| `TestArtifacts_MissingFileFailsBuild` | `tests/` | a declared file that does not exist → `Build` error containing the id |
| `TestArtifacts_NonRootError` | `tests/` | a non-root module declaring `Artifacts()` → `msgArtifactsNonRoot` |
| `TestPlaceLargeFiles_CopiesWhenLinkFails` | `build_internal_test.go` | the link call is indirected (`var linkFile = os.Link`); the test stubs it to return an error and checks the placed file is a byte-identical copy |

## Stage 5 — docs

- `README.md`: next to `Favicon()`/`PWA()`, how to declare `Artifacts()` (the 6-line example above),
  that files are placed under `/artifacts/`, never bundled or precached, and that the server sends
  them with `Cache-Control: no-store` (`server/httpd` does, through `pwa.CacheControl`).
- `docs/ARCHITECTURE.md`: a short section "Large artifacts": extraction → `BuildManifest` →
  `/artifacts.json` in the shell → `placeLargeFiles` (link, else streamed copy) → `LargeFiles()` for
  deployers that upload instead of writing to disk.

## Acceptance

- `gotest` green (`go install webtyp.com/devflow/cmd/gotest@latest`). Never run `gopush`/`codejob`.
- `grep -rn "ReadFile" build.go` shows no read of a large file (only what existed before).

| Stage | Files | Done when |
|---|---|---|
| 1 | `scanner.go`, `select.go`, `extract.go`, `assets.go`, `pipeline.go`, `merge.go`, `emit_core.go` | `Artifacts()` reaches `c.artifactSources` |
| 2 | `build.go` | manifest emitted, `LargeFiles()` |
| 3 | `build.go` | files placed by `Build` |
| 4 | tests | table green |
| 5 | `README.md`, `docs/ARCHITECTURE.md` | written |
