---
PLAN: "feat: development serves ArtifactSources from disk (Range, no copy) and builds web/workers on demand"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 1924443653718636446
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `sitec`: large artifacts and Workers in development

**Read [AGENTS.md](../AGENTS.md) first** (tests in `tests/`, white-box `*_internal_test.go` only for
unexported code, messages through `lang.Translate`, build tool: stdlib is correct here). Master
plan: [PWA_ARTIFACTS_MASTER_PLAN.md](https://github.com/webtyp/app/blob/main/docs/PWA_ARTIFACTS_MASTER_PLAN.md)
(D-PWA-15, D-PWA-18).

## Why

A project declares its large files (model weights) typed, like its CSS:

```go
//go:build !wasm
func ArtifactSources() []artifacts.Source { return []artifacts.Source{{ID: "decider-0.8b", Version: "int8-2026-10", File: "models/decider-0.8b.wtypw"}} }
```

and its Web Workers by convention (`web/workers/<name>/main.go`). Today both only exist in a
release `Build`: the development daemon (`webtyp.com/app`, which drives a `Compiler` with an
in-memory FS and serves it through `sitec/serve`) never writes `/artifacts.json`, never serves
`/artifacts/…`, and never builds a Worker. So to talk to an in-browser agent a developer must run a
separate release build and server. This plan gives the daemon what it needs; the daemon's own plan
(`webtyp/app`) calls it.

## Design gate

1. **Prior art.** Vite's dev server serves `public/` files straight from disk with Range support
   (via `sirv`), never copying them; it compiles Workers on demand (`?worker`).
2. **Novice-name test.** `(*Compiler).LargeFile(url) (path, ok)`, `(*Compiler).BuildWorkers()`,
   `WorkersDir`.
3. **Complexity ledger.** +2 exported methods, 1 constant exported (`workersDir` → `WorkersDir`).
   Ways to get `/artifacts.json`: still 1 (now emitted by the `Compiler` in both paths).
4. **Where it belongs.** Here: the `Compiler` already receives `ArtifactSources()` in
   `RouteExtractedAssets` (it stores `c.artifactSources`) and `sitec/serve` already serves its FS.
5. **What it deletes.** The manifest block in `buildPipeline` (`build.go`, the
   `if len(c.artifactSources) > 0 { … artifacts.BuildManifest … }` block): it moves into the
   `Compiler` so release and development share one path.

## Stage 1 — the manifest in the `Compiler` (`large.go`, new)

- `Compiler` fields: `large []artifacts.LocalFile` and `largeKey string`.
- `func (c *Compiler) emitArtifacts() error`, called at the **end** of `RouteExtractedAssets`
  (after `c.artifactSources` is set):
  - no sources → `c.large = nil`, `c.largeKey = ""`, return nil;
  - `key` = for each source, `ID|Version|File|size|modTime-unix-nano` of its file (`os.Stat` of
    `File` joined to `c.RootDir` unless absolute), joined with `\n`; a `Stat` error is returned as
    `sitec: artifact %s: %v` (id, error);
  - `key == c.largeKey` → return nil (no re-hash: hashing 1.2 GB takes seconds, and
    `RouteExtractedAssets` runs on every SSR change in the daemon);
  - else `m, files, err := artifacts.BuildManifest(c.RootDir, c.artifactSources)`, `m.Encode()`,
    `c.Write(strings.TrimPrefix(artifacts.ManifestPath, "/"), data, mediaTypeJSON)`, `c.large =
    files`, `c.largeKey = key`.
- `func (c *Compiler) LargeFile(url string) (path string, ok bool)` — the disk path of the declared
  file served at `url` (linear scan of `c.large`).
- `func (c *Compiler) LargeFiles() []artifacts.LocalFile`.
- `build.go`: delete the manifest block from `buildPipeline`; `Output.large` becomes
  `c.LargeFiles()`. `placeLargeFiles` and `checkRouteCollisions` keep working on it.

## Stage 2 — serving large files in development (`serve/serve.go`)

In `RegisterRoutes`, before `fs.Read`: when `strings.HasPrefix(key, pwa.ArtifactsDir)` and `fs`
implements `interface{ LargeFile(string) (string, bool) }` and it returns ok, call
`serveLarge(ctx, path)` and return. `serveLarge` (same file, unexported):

- `os.Open` + `Stat`; failure → 404 as today.
- Headers: `Content-Type: application/octet-stream`, `Accept-Ranges: bytes`,
  `Cache-Control: ` + `pwa.CacheControl(key)` (it answers `no-store` under `/artifacts/`).
- `Range: bytes=a-b` or `bytes=a-` (one range only): clamp `b` to size−1; `a` > `b` or `a` ≥ size →
  status 416 with `Content-Range: bytes */<size>`. Valid → status 206,
  `Content-Range: bytes a-b/size`, `Content-Length`, then the bytes with `f.ReadAt` in pieces of
  `largeChunk = 1 << 20`, each written with `ctx.Write` (never the whole range in one buffer).
- No `Range` → status 200, `Content-Length: size`, the whole file in the same 1 MiB pieces.

(`webtyp.com/artifacts` downloads with 8 MiB Range requests and requires 206.)

## Stage 3 — Workers on demand (`workers.go`)

- Rename `workersDir` → exported `const WorkersDir = "web/workers"`.
- `func (c *Compiler) BuildWorkers() error { return buildWorkers(c.RootDir, c, c.DevMode) }`;
  `buildPipeline` calls `c.BuildWorkers()` instead of `buildWorkers(root, c, …)` (the `Compiler`
  there is built with `RootDir: root` and `DevMode: cfg.Mode == ModeDev`).
- **Development builds change**: today a dev Worker is built with the Go toolchain. A Worker exists
  to run heavy code (models), which Go's WebAssembly runs several times slower than TinyGo's. In dev
  mode build **once with TinyGo, the SIMD target and `-opt=2`**
  (`NewWasmBuilder(false, WasmBuildOptions{Entry, OutputName: name, SIMD: true, Speed: true})`,
  not hashed: `<name>.wasm`), and point **both** scripts (`<name>.worker.js`,
  `<name>.simd.worker.js`) to it. Development machines run browsers with SIMD. Update the doc comment
  and `docs/ARCHITECTURE.md` (the "In development" paragraph) accordingly.
- `func WorkerNames(root string) ([]string, error)` — the valid worker directory names (the same
  discovery and name check `buildWorkers` does; refactor `buildWorkers` to use it). The daemon uses
  it to know which packages to watch.

## Stage 4 — tests

| Test | Where | Proves |
|---|---|---|
| `TestCompiler_EmitsManifestOnRoute` | `tests/` | a fixture project whose root declares `ArtifactSources()` with one 1 MiB file (pattern: `tests/artifacts_build_test.go`), driven like the daemon: `NewCompiler(&Config{RootDir, OutputDir, DevMode: true})`, `SetFS(NewMemFS())`, `all, _ := sitec.New(root).ExtractAll(); RouteExtractedAssets(all)` → `Read("/artifacts.json")` parses with `artifacts.ParseManifest`; `LargeFile(url)` returns the absolute file path |
| `TestCompiler_ManifestCachedUntilFileChanges` | `large_internal_test.go` | two `emitArtifacts` calls without change hash once (count calls through an injectable `var buildManifest = artifacts.BuildManifest`); touching the file (new mtime) hashes again |
| `TestServe_LargeFileRange` | `tests/` | `serve.RegisterRoutes` over a fake router context (or `httpd`'s test helpers if the repo already uses one) with an FS that implements `LargeFile`: `Range: bytes=10-19` → 206, the 10 bytes, `Content-Range: bytes 10-19/<size>`, `Cache-Control` contains `no-store`; `bytes=<size>-` → 416; no Range → 200 and every byte |
| `TestServe_LargeFileNotDeclared` | `tests/` | `/artifacts/other.bin` not declared → 404 |
| `TestWorkers_DevBuildsTinyGoSIMDOnce` | `workers_internal_test.go` | with `execCommand` stubbed (the existing seam): dev mode runs `tinygo` once with a `-target` file containing `+simd128` and `-opt=2`; never `go build` |
| `TestWorkers_DevBuildsOncePointsBothScripts` (existing) | `tests/` | still green: both scripts reference `/echo.wasm` |
| `TestCompiler_BuildWorkers` | `tests/` | `NewCompiler(&Config{RootDir: fixture, DevMode: true})` + `SetFS(NewMemFS())` + `BuildWorkers()` → `Read("/echo.worker.js")` exists |

## Stage 5 — docs

`README.md` and `docs/ARCHITECTURE.md` ("Large artifacts" and "Web Workers"): in development the
`Compiler` writes `/artifacts.json` and `sitec/serve` streams the declared files from where they
are (no copy, Range, `no-store`); Workers are built on demand with `BuildWorkers` (TinyGo SIMD in
development). Delete the sentence "The development daemon does not place them: an application tests
its downloads after a release `Build`" and "The development daemon … does not build workers yet".

## Acceptance

- `gotest` green. Never run `gopush` or `codejob`.
- `grep -n "BuildManifest" build.go` → empty; `grep -n "BuildManifest" large.go` → present.
- `grep -rn "workersDir" --include=*.go .` → empty.

| Stage | Files | Done when |
|---|---|---|
| 1 | `large.go`, `emit_core.go`, `build.go` | manifest in the Compiler, cached |
| 2 | `serve/serve.go` | Range streaming |
| 3 | `workers.go`, `build.go` | `BuildWorkers`, `WorkerNames`, dev = TinyGo SIMD |
| 4 | tests | table green |
| 5 | `README.md`, `docs/ARCHITECTURE.md` | updated |
