# Build-Time ESM Emission & Package Cache

How `webtyp/sitec` compiles strict-subset Go client packages via `webtyp/js/esm`, caches them by content hash, injects `runtime.js`, and emits `<script type="module">` with SRI under strict CSP.

```mermaid
flowchart TD
    Provider["RenderJS() []*js.Script (Module: true)"] --> Reg["Compiler.updateSSRModuleInSlot"]
    GoDir["Client Go package dir"] --> Cache{"esmCacheKey{PkgDir, SourceHash}"}
    Cache -->|hit| Cached["Cached *js.Script"]
    Cache -->|miss| Compile["esm.CompilePackage(dir)"]
    Compile --> Cached
    Reg --> RT["Inject js.RuntimeScript() (/runtime.js)"]
    Reg --> HTML["buildHTMLShell(jsIsModule: true)"]
    HTML --> Out["<script src=... integrity=sha384-... type=module>"]
    Workers["web/workers/<name>/main.go"] --> Lazy["/pdf.worker.js + /pdf.wasm (lazy)"]
```

1. Any provider returning `js.Script{Module: true}` activates ESM mode on `Compiler` and ensures `runtime.js` is emitted once.
2. `Compiler.CompileESMPackage(dir)` hashes the Go sources in `dir`, reuses cached ESM output on unchanged files, and recompiles via `webtyp/js/esm` on changes.
3. `buildHTMLShell` emits `<script type="module">` with `integrity="sha384-..."` and `crossorigin="anonymous"` (zero inline JS).
4. Lazy WASM Workers under `web/workers/` continue building independently (`/pdf.wasm` + `/pdf.worker.js`) without requiring `browser.wasm`.
