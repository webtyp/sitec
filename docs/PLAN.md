---
PLAN: "feat!: the browser main is web/browser/main.go (one main per folder)"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 8284510115111700650
PR: https://github.com/webtyp/sitec/pull/32
REVIEWED_COMMIT: 8cf4df20419a6607678c3b9386fb887fca81519a
---

> Read AGENTS.md at the repo root before any change.

# Browser entry: `web/browser/main.go`

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Why

The ecosystem moves every binary main into its own folder under `web/` (the folder is the
binary): `web/browser/main.go` → `web/public/browser.wasm`, `web/server/main.go` →
`web/server/server`, `web/edge/main.go` → `web/edge/edge.wasm`. Three mains in one `web/`
directory forced a destination tag of our own (`browser`) on the browser main, and gopls never
analyses a file gated on a custom positive tag. No backwards compatibility: `web/browser.go` stops
being recognised in the same change.

The browser build still passes `-tags browser` (it keeps `!browser` server files out of
`browser.wasm`). Only the entry path and its detection change.

## Stage 1 — `wasm_builder_exec.go`

1. Replace `defaultBrowserEntryFile = "browser.go"` with:

```go
// defaultBrowserEntry is the browser main, relative to the project root: one
// main per folder, the folder is the binary.
defaultBrowserEntry = "web/browser/main.go"
```

   (keep it in the same `const` block; `defaultBrowserOutput` and `browserBuildTag` stay).
2. `WasmBuildOptions.entry()`: when `o.Entry == ""` return
   `filepath.FromSlash(defaultBrowserEntry)`.
3. The `Entry` field comment: `// Empty means "web/browser/main.go".`; the type comment: the zero
   value builds `web/browser/main.go → browser.wasm`; `NewDefaultWasmBuilder` comment: "from
   web/browser/main.go".
4. Browser detection in `Build`: replace
   `isBrowser := filepath.Base(entry) == defaultBrowserEntryFile` with
   `isBrowser := filepath.Clean(entry) == filepath.FromSlash(defaultBrowserEntry)`.

## Stage 2 — `build.go`

Line with `os.Stat(filepath.Join(root, "web", defaultBrowserEntryFile))` →
`os.Stat(filepath.Join(root, filepath.FromSlash(defaultBrowserEntry)))`.

## Stage 3 — comments and docs

- `reach.go`: in the `destinationTags` comment, `web/browser.go is \`browser\`` → `web/browser/main.go is built with -tags browser`.
- `docs/ARCHITECTURE.md`: every `web/browser.go` → `web/browser/main.go`; in «Reachability»,
  "(`web/browser.go`)" → "(`web/browser/main.go`)".
- `AGENTS.md`: outside the `<!-- webtyp:base -->` block (never edit that block), every
  `web/browser.go` → `web/browser/main.go`.

## Stage 4 — tests

Update every fixture path, no new behaviour:

- `tests/build_wasm_output_test.go`, `tests/workers_test.go`,
  `tests/unreachable_package_test.go`: every fixture key `"web/browser.go"` →
  `"web/browser/main.go"`; every expectation that names `web/browser.go` likewise.
- In `tests/unreachable_package_test.go` the fixture main moves to `web/browser/main.go` but
  **keeps `//go:build browser`**: those two tests exercise a `browser`-tagged file (the case that
  still exists for files of a package shared by two binaries), and without the tag the old probe
  reproduced by `TestExtractAll_ProbeMissingConfigurationFailsLoudly` would reach `theme` natively
  and the test would stop proving anything. Do not change either test's assertions.
- Add one test in `tests/build_wasm_output_test.go`:
  `TestBuild_LegacyBrowserGoIsNotAnEntry` — setup: a project with only `web/browser.go`
  (package main, `func main(){}`); call the same builder the file's other tests call with a zero
  `WasmBuildOptions`; expected: error containing `input file not found` and `web/browser/main.go`.

`grep -rn 'web/browser.go\|defaultBrowserEntryFile' --include=*.go .` → empty.

## Delivery check

```bash
gotest
grep -rn 'web/browser.go\|defaultBrowserEntryFile' --include=*.go --include=*.md . | grep -v docs/PLAN.md   # empty
```

A PR whose tests could not run is a failed delivery. Environment:
`GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`.

Never pause to ask permission or report progress: when the delivery check passes, commit, push
and open the PR. Stuck for 30 minutes → open the PR anyway listing what fails.

## Stages

| # | Files | Result |
|---|---|---|
| 1 | `wasm_builder_exec.go` | default entry `web/browser/main.go`, detection by path |
| 2 | `build.go` | build detects the new entry |
| 3 | `reach.go`, `docs/ARCHITECTURE.md`, `AGENTS.md` | docs |
| 4 | `tests/build_wasm_output_test.go`, `tests/workers_test.go`, `tests/unreachable_package_test.go` | fixtures + legacy-path test |
