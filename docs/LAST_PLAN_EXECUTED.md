---
PLAN: "feat: build-time ESM module compilation, per-package cache, and strict CSP module emission"
EXECUTOR: local
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.
> **Rama de trabajo:** `feat/frontend-js` (crear desde `main` antes de editar; nunca tocar `main` hasta cerrar la ola).
> **Dependencia local:** requiere `webtyp/js` en rama `feat/frontend-js` (`replace webtyp.com/js => ../js` en `go.mod` durante desarrollo local).

# Plan — `webtyp/sitec` (Fase 1b / 3a de `FRONTEND_JS_MASTER_PLAN`)

Objetivo: extender `sitec` para que descubra y compile en build-time los módulos JS generados desde Go (`js.Script{Module: true}` y paquetes cliente vía `webtyp/js/esm`), con caché por paquete, minificación, hash de contenido + SRI, emisión de `<script type="module">` bajo CSP estricta, manteniendo intacto el compilador de Workers WASM perezosos (`web/workers/<name>/main.go` → `/pdf.wasm`).

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
export PATH="$PATH:$(go env GOPATH)/bin"
export GOPRIVATE="webtyp.com/*,github.com/veltylabs/*"
export GOFLAGS="-mod=mod"
```

## Ecosystem search

- Buscado en `~/Dev/Project/webtyp/sitec`: `sitec` ya descubre `RenderJS() []*js.Script` (`scanner.go:98`, `emit_ssr_register.go:15`, `extract.go:234`), minifica con `github.com/tdewolff/minify/v2`, gestiona caché en `cache.go`, y compila `web/workers/<name>/main.go` en `wasm_builder_exec.go`.
- Decisión: **extender** `sitec` sobre `RenderJS()` y `cache.go` (mismo patrón build-time que `RenderCSS()`; cero `.js` comiteados en librerías/módulos — D7).

## Design gate (api-design)

1. **Prior art:**
   - [`Vite`](https://vite.dev/guide/features.html#npm-dependency-resolving-and-pre-bundling): emisión de `<script type="module">` con hash de contenido y precarga de módulos.
   - [`esbuild`](https://esbuild.github.io/api/#format): empaquetado ESM con *tree-shaking* y minificación en milisegundos.
   - [`sitec` CSS pipeline](https://github.com/webtyp/sitec): descubrimiento AST + caché + emisión con hash + SRI en HTML.
   - **Por qué diferimos:** `sitec` ya es el compilador único de activos de `webtyp` en Go puro; añadir emisión ESM aquí evita introducir Node/npm en el toolchain de desarrollo o CI.
2. **Novice-name test:** reutiliza `RenderJS() []*js.Script` (ya existente) con `Script.Module == true`; en `sitec.Config`, `JSModules bool` (o activación automática cuando hay `Script.Module == true` sin requerir flag extra — preferido: automático, cero flags nuevos).
3. **Complexity ledger:**
   - Conceptos nuevos: `+0` (reutiliza `RenderJS()` y `web/workers/`).
   - Flags nuevos en `sitec.Config`: `+0` (detección automática por `Script.Module`).
   - Formas de hacer lo mismo: `+0` (un solo pipeline de JS en `sitec`).
4. **Where it belongs:** `webtyp/sitec` es el único compilador de activos de salida (`web/public/`). Importa `webtyp/js` y `webtyp/js/esm`; ningún módulo de dominio importa `sitec`.
5. **What it deletes:** en Fase 4 eliminará la inyección obligatoria de `PageBootstrap("/browser.wasm")` cuando el proyecto no tenga `web/browser/main.go` WASM.

## APIs used

| Symbol | Exact signature | Tag | File |
|---|---|---|---|
| `js.Script` | `type Script struct { Name string; Content string; Module bool }` | `feat/frontend-js` | `webtyp.com/js/js.go` |
| `js.RuntimeScript` | `func RuntimeScript() *Script` | `feat/frontend-js` | `webtyp.com/js/runtime_emit.go` |
| `esm.CompilePackage` | `func CompilePackage(dir string) (*js.Script, error)` | `feat/frontend-js` | `webtyp.com/js/esm/compile.go` |

If a signature differs after `webtyp/js` Stage 3 lands, update this table first; never guess.

## Etapas

### Etapa 1 — Diagrama y pruebas en rojo (TDD)
- Crear `docs/diagrams/ESM_EMISSION.md` (`flowchart TD`: `RenderJS()` + runtime `js.RuntimeScript()` → caché por paquete → minificación → `/app.<hash>.js` + `<script type="module" src="..." integrity="sha384-...">` + Workers `/pdf.worker.js` + `/pdf.wasm`).
- Crear `tests/esm_emission_test.go` y registrar en `docs/TESTS.md`:
  - `TestSitec_EmitsModuleScriptTag`: cuando un productor devuelve `js.Script{Module: true}`, el HTML emite `<script type="module" src="/app.js">` (dev) y `<script type="module" src="/app.<hash>.js" integrity="sha384-...">` (release), sin inline JS (CSP estricta).
  - `TestSitec_CachesCompiledPackage`: segunda pasada sin cambios de hash de fuente reutiliza la entrada de `cache.go`.
  - `TestSitec_KeepsLazyWasmWorkers`: `web/workers/pdf/main.go` sigue compilando a `/pdf.wasm` y `/pdf.worker.js` aunque la app no tenga `browser.wasm`.

### Etapa 2 — Emisión de módulos ESM y SRI en HTML
- Editar `emit_html.go` y `emit_injection.go`: soportar emisión de `<script type="module">` con `integrity` (SRI) cuando `Script.Module == true`, separado de scripts clásicos.
- Editar `emit_asset.go` y `emit_flush.go`: inyectar `js.RuntimeScript()` una sola vez como base del bundle ESM y emitir cada `Script{Module: true}` minificado.

### Etapa 3 — Caché por paquete para `esm.CompilePackage`
- Editar `cache.go`: añadir clave tipada `esmCacheKey{PkgPath, SourceHash}` para cachear la salida de `esm.CompilePackage` en memoria/disco durante `webtyp dev` (hot-reload en milisegundos).

### Etapa 4 — Modo sin `browser.wasm` + Workers perezosos intactos
- Editar `wasm_builder_exec.go`: no fallar ni inyectar `PageBootstrap` cuando el proyecto compila su frontend a ESM y no tiene `browser.wasm`, manteniendo intacta la compilación de `web/workers/<name>/main.go`.

### Etapa 5 — Documentación
- Actualizar `README.md` (≤150 líneas) y `docs/ARCHITECTURE.md`.

## Delivery check

- `gotest` en verde (con `replace webtyp.com/js => ../js` durante desarrollo en rama `feat/frontend-js`).
- `grep -rn "TODO\|FIXME\|Deprecated" --include='*.go' .` sin hallazgos nuevos.
- Commit tras cada etapa en rama `feat/frontend-js`.

| Etapa | Qué | Estado |
|---|---|---|
| 1 | Diagrama `ESM_EMISSION.md` + pruebas TDD en rojo | ⬜ |
| 2 | Emisión `<script type="module">` + SRI + `RuntimeScript()` | ⬜ |
| 3 | Caché por paquete en `cache.go` | ⬜ |
| 4 | Frontend sin `browser.wasm` manteniendo `web/workers/` | ⬜ |
| 5 | `README.md` + `docs/ARCHITECTURE.md` | ⬜ |
