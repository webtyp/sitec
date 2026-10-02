---
PLAN: "feat: web/workers/<name> — sitec builds each Web Worker twice (plain and SIMD) with its script"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> **READY — rename to `docs/PLAN.md` to dispatch.** Its prerequisite shipped: `ArtifactSources()`
> in sitec v0.2.42 (the producer was renamed from `Artifacts()`, a name already declared by
> `min.Handler` and `Output`). Re-check the symbols below against the published tags first.

# Plan — `sitec`: Web Worker binaries, plain and SIMD

Master plans: [PWA_ARTIFACTS_MASTER_PLAN.md](https://github.com/webtyp/app/blob/main/docs/PWA_ARTIFACTS_MASTER_PLAN.md)
(D-PWA-18) and the agent wave (D16: *the Worker ships a SIMD and a plain binary*). Read
[AGENTS.md](../AGENTS.md) first.

## Why

`webtyp.com/agentworker` runs the assistant in a Web Worker whose binary the application writes
(its own `main` calling `agentworker.Serve`). The page starts it with
`agentworker.Start(Scripts{Plain: "/<name>.worker.js", SIMD: "/<name>.simd.worker.js"}, ...)` and
picks SIMD when the browser has it. Today nothing builds a Worker binary: `sitec` builds only
`web/client.go`. SIMD gives 2× end to end on the decision model (nn/docs/PERFORMANCE.md) and needs a
TinyGo target with `+simd128` and `-opt=2`.

## Design gate

1. **Prior art.** Vite (`new Worker(new URL('./w.js', import.meta.url))` → the bundler emits a
   separate hashed chunk per Worker), webpack 5 (same, `worker-loader` before), Parcel (detects
   `new Worker` and bundles it). All discover Workers from code; we use a directory convention
   instead, like `web/client.go` for the page: discoverable without running the app.
   `wasm-feature-detect` + two builds is how Squoosh and others ship SIMD and non-SIMD WebAssembly.
2. **Novice-name test.** A directory `web/workers/<name>/` with a `main.go` "is a Worker named
   <name>". Outputs: `<name>.worker.js`, `<name>.simd.worker.js` (fixed names) loading
   `<name>.<hash>.wasm` / `<name>.simd.<hash>.wasm` (hashed in release).
3. **Complexity ledger.** +1 convention, +1 option (`WasmBuildOptions.SIMD`). Ways to build a Worker: 1.
4. **Where it belongs.** `sitec` builds every wasm binary of the static surface.
5. **What it deletes.** Nothing.

## Stage 1 — SIMD builds in the wasm builder

`wasm_builder_exec.go`:
- `WasmBuildOptions` gets `SIMD bool // TinyGo with +simd128 and -opt=2 (Web Workers that run models)`.
- When `SIMD` (TinyGo only; with `stdlib` it is ignored): write the target JSON below to a temp file
  next to the temp output (constant `simdTargetJSON`, content exactly as `webtyp/nn`'s
  `testdata/wasm-simd.json`) and run `tinygo build -target <that file> -opt=2 -no-debug -o <out> .`
  instead of `-target wasm`.
  ```json
  {"inherits":["wasm"],"features":"+bulk-memory,+bulk-memory-opt,+call-indirect-overlong,+mutable-globals,+nontrapping-fptoint,+sign-ext,-multivalue,-reference-types,+simd128","cflags":["-msimd128"]}
  ```
- Plain Worker builds also use `-opt=2` (a Worker is compiled for speed, the page for size): add
  `Speed bool` to `WasmBuildOptions`, set for both Worker builds; `SIMD` implies `Speed`.
- Worker binaries get a content-hashed name in release like the page binary: add
  `HashName bool` to `WasmBuildOptions` (the page builder sets it internally as today).

## Stage 2 — discover and build Workers

New file `workers.go`:
- `workersDir = "web/workers"`; every direct subdirectory containing `main.go` is a Worker; its
  directory name is `<name>` (must match `^[a-z][a-z0-9-]*$`, else error
  `sitec: worker directory %q: use lowercase letters, digits and dashes`).
- In `buildPipeline`, after the page wasm and before `RouteExtractedAssets`, for each Worker:
  - **Release** (TinyGo): build twice with `NewWasmBuilder(false, WasmBuildOptions{Entry:
    "web/workers/<name>/main.go", OutputName: "<name>", Speed: true, HashName: true})` and the same
    with `OutputName: "<name>.simd", SIMD: true`.
  - **Dev** (`cfg.Mode == ModeDev`, Go stdlib for speed of compilation): build once, plain, fixed
    name `<name>.wasm`; both scripts point to it (the page's SIMD choice still finds a script).
  - Write each binary with `c.Write(filename, binary, "application/wasm")`.
  - Emit two standalone scripts with `js.WebWorker("<name>.worker.js", "/"+plainFile)` and
    `js.WebWorker("<name>.simd.worker.js", "/"+simdFile)` and write each `Content` with
    `c.Write(script.Name, []byte(script.Content), "text/javascript")`. Script names are fixed
    (they are revalidated, `pwa.CacheControl`), the wasm names inside them are hashed.

## Stage 3 — tests

| Test | Proves |
|---|---|
| `TestWorkers_ReleaseBuildsPlainAndSIMD` | fixture with `web/client.go` and `web/workers/echo/main.go` (`package main; func main() {}`), release → artifacts `echo.worker.js`, `echo.simd.worker.js`, one `echo.<8hex>.wasm` and one `echo.simd.<8hex>.wasm` (`pwa.IsHashedName`); each script fetches its own wasm name |
| `TestWorkers_SIMDBinaryUsesSIMDTarget` | inject the command runner (or inspect the args the builder builds — add a test seam if none exists) and assert the SIMD build passes a `-target` file whose content contains `+simd128` and the flag `-opt=2` |
| `TestWorkers_DevBuildsOncePointsBothScripts` | dev mode → one `echo.wasm`, both scripts reference `/echo.wasm` |
| `TestWorkers_BadName` | `web/workers/Echo_1/main.go` → the error |
| `TestWorkers_NoWorkersNoOutput` | no `web/workers` → no worker artifacts, build unchanged |

## Stage 4 — docs

`README.md` and `docs/ARCHITECTURE.md`: the convention, the four outputs, why two builds (D16, SIMD
2× end to end), and the page side (`agentworker.Start(Scripts{Plain: "/<name>.worker.js", SIMD:
"/<name>.simd.worker.js"}, ...)`).

## Acceptance

`gotest` green. Never run `gopush` or `codejob`.
