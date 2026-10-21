---
PLAN: "refactor: build web/browser.go with -tags browser into browser.wasm and probe js/wasm with -tags=browser"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 11030676366596612176
---

> Read `AGENTS.md` at the repo root before any change.

# `webtyp/sitec` — `web/browser.go` → `browser.wasm` (`-tags browser`)

Part of [BUILD_DESTINATIONS_MASTER_PLAN.md](https://github.com/webtyp/docs/blob/main/BUILD_DESTINATIONS_MASTER_PLAN.md) (D1, D3, D4, D7, D12, D15):
- Every build destination in `web/` is named after its target: `web/browser.go` (`//go:build browser`) → `browser.wasm` (`browser.<hash>.wasm` in release), `web/edge.go` (`//go:build wasm && !browser`) → `edge.wasm`, `web/server.go` (`//go:build !wasm`) → `server`.
- Only the browser frontend compiler passes `-tags browser` (D4); `NewWasmBuilder` for other entries (`web/edge.go`, `web/workers/<name>/main.go`) does **not** pass `-tags browser`, so `web/browser.go` and `web/edge.go` coexist in `web/` without symbol collisions.
- `reach.go` probes the `js/wasm` dependency graph with `-tags=browser` (D7) so packages imported by `//go:build browser` files (`web/browser.go`, `modules/<m>/browser.go`) remain in the reachable asset union.

> **Anti-footgun:** `sitec` is backend build tooling. Standard library imports (`os`, `os/exec`, `path/filepath`, `strings`, etc.) are legitimate here — do **not** replace them.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Design gate

1. **Prior art:** `package.json` `browser` condition and WebTyp's `BUILD_DESTINATIONS` D1/D4/D7 (`web/browser.go` → `browser.wasm` with `-tags browser`).
2. **Novice-name test:** `WasmBuildOptions` zero value compiles `web/browser.go` into `browser.wasm` (`browser.<hash>.wasm` in release); no new exported symbols.
3. **Complexity ledger:** +0 exported symbols / −1 mismatch (`web/client.go` / `client.wasm` removed).
4. **Where it belongs:** `sitec` owns `WasmBuildOptions`, `NewDefaultWasmBuilder`, `buildPipeline`, and `computeReachability`.
5. **What it deletes:** all `web/client.go` and `client.wasm` defaults and references in `wasm_builder_exec.go`, `build.go`, tests, and docs (no backwards-compatibility fallback to `web/client.go`).

---

## Stage 1 — Update `wasm_builder_exec.go`, `build.go`, and `reach.go`

Files: `wasm_builder_exec.go`, `build.go`, `reach.go`

1. In `wasm_builder_exec.go`:
   - Define named constants for the default browser entry and output:
     ```go
     const (
     	defaultBrowserEntryFile = "browser.go"
     	defaultBrowserOutput    = "browser.wasm"
     	browserBuildTag         = "browser"
     )
     ```
   - Update `WasmBuildOptions` doc comments:
     - Zero value builds a site frontend: `web/browser.go → browser.wasm`.
     - `Entry` doc comment: `Empty means "web/browser.go".`
     - `OutputName` doc comment: `Empty means "browser".`
   - Update `entry()` and `filename()`:
     ```go
     func (o WasmBuildOptions) entry() string {
     	if o.Entry == "" {
     		return filepath.Join("web", defaultBrowserEntryFile)
     	}
     	return o.Entry
     }

     func (o WasmBuildOptions) filename() string {
     	if o.OutputName == "" {
     		return defaultBrowserOutput
     	}
     	return o.OutputName + ".wasm"
     }
     ```
   - Update `NewDefaultWasmBuilder` doc comment:
     ```go
     // NewDefaultWasmBuilder builds a site frontend from web/browser.go.
     // In release (devMode false) the binary is named by its content hash (browser.<hash>.wasm) and
     // the page bootstrap loads that name.
     ```
   - In `defaultWasmBuilder.Build`: determine whether the entry is the browser frontend (`isBrowser := filepath.Base(entry) == defaultBrowserEntryFile`).
     - In the TinyGo branch (`!w.stdlib`): if `isBrowser`, append `"-tags", browserBuildTag` to `args` before `"-no-debug", "-o", tmpOutPath, "."`.
     - In the standard Go branch (`w.stdlib`): if `isBrowser`, invoke `execCommand("go", "build", "-tags", browserBuildTag, "-o", tmpOutPath, ".")`; otherwise invoke `execCommand("go", "build", "-o", tmpOutPath, ".")`.

2. In `build.go`:
   - Change line 367 from:
     ```go
     if _, err := os.Stat(filepath.Join(root, "web", "client.go")); err == nil {
     ```
     to:
     ```go
     if _, err := os.Stat(filepath.Join(root, "web", defaultBrowserEntryFile)); err == nil {
     ```

3. In `reach.go`:
   - In `goListDeps` (around line 71), when `len(env) > 0` (the `js/wasm` browser target), pass `"-tags=" + browserBuildTag`:
     ```go
     if len(env) > 0 {
     	data, err = tc.ListEnv(rootDir, env, "-e", "-tags="+browserBuildTag, "-deps", pattern)
     } else {
     	data, err = tc.List(rootDir, "-e", "-deps", pattern)
     }
     ```

---

## Stage 2 — Update tests and add coexistence test

Files: `tests/build_wasm_output_test.go`, `tests/workers_test.go`, `tests/serve_http_test.go`, `tests/emit_write_binary_test.go`, `caso1_serve_internal_test.go`

1. In `tests/build_wasm_output_test.go`:
   - Replace every `"client.go"` fixture with `"browser.go"` containing `"//go:build browser\n\npackage main\nfunc main() {}"`.
   - Replace expected `"client.wasm"` with `"browser.wasm"`, `"client."` with `"browser."`, and `"web/client.go"` with `"web/browser.go"`.
   - Add `TestWasmBuild_CoexistingBrowserAndEdgeInWebDir`:
     - Write `go.mod` (`module testapp\n\ngo 1.22\n`), `web/browser.go` (`//go:build browser\n\npackage main\nfunc main() {}`), and `web/edge.go` (`//go:build wasm && !browser\n\npackage main\nfunc main() {}`) in the same `web/` directory.
     - Run `sitec.NewDefaultWasmBuilder(true).Build(tmpDir)` and assert it succeeds with `out.Filename == "browser.wasm"`.
     - Run `sitec.NewWasmBuilder(true, sitec.WasmBuildOptions{Entry: "web/edge.go", OutputName: "edge"}).Build(tmpDir)` and assert it succeeds with `out.Filename == "edge.wasm"`.
2. In `tests/workers_test.go`:
   - Update line 49 from `write(filepath.Join(root, "web/client.go"), "package main\nfunc main() {}")` to `write(filepath.Join(root, "web/browser.go"), "//go:build browser\n\npackage main\nfunc main() {}")`.
3. In `tests/serve_http_test.go`:
   - Replace `"client.wasm"` and `"/client.wasm"` with `"browser.wasm"` and `"/browser.wasm"` (lines 155–162).
4. In `tests/emit_write_binary_test.go`:
   - Replace `"client.wasm"` and `"/client.wasm"` with `"browser.wasm"` and `"/browser.wasm"` (lines 35–64).
5. In `caso1_serve_internal_test.go`:
   - Update comment on line 30 from `client.wasm` to `browser.wasm`.

Run `gotest ./...` and verify all tests pass.

---

## Stage 3 — Update documentation

File: `docs/ARCHITECTURE.md`

1. In `docs/ARCHITECTURE.md` (line 241), replace `client.3f9a1c2b.wasm` with `browser.3f9a1c2b.wasm`.

---

## Deletions and acceptance criteria

- `grep -rn --exclude="LAST_PLAN_EXECUTED.md" "client\.wasm\|web/client\.go\|client\.3f9a1c2b\.wasm" .` returns empty.
- `gotest ./...` passes with zero failures.

## Stages

| # | Files | Goal |
|---|---|---|
| 1 | `wasm_builder_exec.go`, `build.go`, `reach.go` | Default to `web/browser.go` → `browser.wasm` with `-tags browser` and probe `js/wasm` with `-tags=browser` |
| 2 | `tests/build_wasm_output_test.go`, `tests/workers_test.go`, `tests/serve_http_test.go`, `tests/emit_write_binary_test.go`, `caso1_serve_internal_test.go` | Update tests to `web/browser.go` / `browser.wasm` and test coexistence with `web/edge.go` |
| 3 | `docs/ARCHITECTURE.md` | Update hashed WASM example to `browser.3f9a1c2b.wasm` |

## Executor notes

All steps were successfully completed. `web/client.go` defaults and `client.wasm` artifacts were fully replaced with `web/browser.go` and `browser.wasm`. `gotest ./...` verified all tests successfully pass. The codebase was built properly.
