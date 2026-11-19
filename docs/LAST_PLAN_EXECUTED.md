---
PLAN: "feat: external hashed sprite and lang files, SRI sha384 hashes and csp.json manifest"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to proceed or finalize — nobody answers, and a session that waits ends without your work.

# Plan: `sitec` — HTML sin nada en línea, SRI SHA-384, `sprite.<hash>.svg`, `lang.<hash>.json` y `csp.json`

Master: `SECURITY_OBSERVABILITY_MASTER_PLAN.md` (D3, O3b-8).
**Prerrequisitos:** `svg` (O3b-2), `lang` (O3b-3), `js` (O3b-4).

## Tools

```bash
export GOPRIVATE=webtyp.com/*,github.com/veltylabs/*
export GOFLAGS=-mod=mod
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Ecosystem search

- Buscado en `~/Dev/Project/webtyp` (`sitec`, `pwa`, `svg`, `lang`, `server`).
- `webtyp.com/pwa` ya provee `pwa.HashedName(name string, content []byte) string` (usado en `sitec/wasm_builder_exec.go:169`).
- `sitec` es el único compilador de activos (`index.html`, `main.js`, `style.css`, páginas SSR, sprite SVG y diccionario de traducciones).
- **Decisión:** extender `sitec` para sacar el sprite y el diccionario fuera del HTML a archivos con hash, firmar `<script>` y `<link rel="stylesheet">` con SRI SHA-384, y escribir `csp.json` con los hashes de scripts para que `server/httpd` arme la CSP estricta.

## Design gate

1. **Prior art:**
   - [MDN — Subresource Integrity (SRI)](https://developer.mozilla.org/en-US/docs/Web/Security/Subresource_Integrity): `integrity="sha384-<base64>" crossorigin="anonymous"` en `<script>` y `<link rel="stylesheet">`.
   - [web.dev — Strict CSP](https://web.dev/articles/strict-csp): el compilador calcula los hashes SHA-384 y los entrega al servidor en un manifiesto de build para la cabecera `Content-Security-Policy`.
   - [Vite / Next.js asset manifest](https://vitejs.dev/guide/backend-integration.html): el build emite un JSON pequeño (`csp.json`) en el directorio público junto a los activos con hash.
2. **Novice-name test:**
   - `sitec.CSPManifestFile = "csp.json"` — archivo JSON en `OutputDir` con `{"script_hashes":["sha384-..."]}`.
   - `sitec.Translations.BundleTranslations(rootDir string) ([]byte, error)` — alineado con `langc.Translations` (O3b-3).
3. **Complexity ledger:**
   - Conceptos a aprender: `+1` (`csp.json`, leído automáticamente por `server/httpd`, nunca a mano por la app) / `-0`
   - Archivos que toca una app: `0`
   - Líneas en el call site de la app: `0`
   - Formas de hacer lo mismo: `0` (se elimina la inyección en línea de sprite y `<script id="webtyp-lang">` en `index.html` y páginas SSR).
4. **Where it belongs:** `sitec` produce los bytes finales (minificados) de `main.js`, `style.css`, `sprite.<hash>.svg` y `lang.<hash>.json`; solo él puede calcular sus hashes exactos al compilar y escribir `index.html` + `csp.json`.
5. **What does this change delete?**
   - Borra `c.indexHtmlHandler.AddDynamicContent(renderSprite)` (`emit_core.go:700-702`).
   - Borra `c.indexHtmlHandler.AddDynamicContent(translationsHTML)` (`emit_core.go:707-711`).
   - Borra `c.renderSpriteNoLock() + p.Body` en páginas estáticas (`emit_route.go:111`).
   - Borra el campo `translationsHTML string` de `Compiler` (`emit_core.go:151`).

## APIs used

| Symbol | Exact signature | Tag | File |
|---|---|---|---|
| `pwa.HashedName` | `func HashedName(name string, content []byte) string` | `v0.2.13` | `webtyp.com/pwa` |
| `svg.SetSpriteURL` | `func SetSpriteURL(url string)` | *(O3b-2)* | `webtyp.com/svg` |
| `svg.SpriteMetaName` | `const SpriteMetaName = "webtyp-sprite"` | *(O3b-2)* | `webtyp.com/svg` |
| `lang.MetaName` | `const MetaName = "webtyp-lang"` | *(O3b-3)* | `webtyp.com/lang` |

If a signature in the published tag differs from this table, stop and name the symbol in the PR; never adapt it by guessing.

## Reglas de entrega y anti-footguns

- `sitec` es tooling de servidor (`!wasm`): stdlib `crypto/sha512`, `encoding/base64`, `encoding/json` son legítimas aquí.
- **Todos los sitios que construyen `<head>`/`index-open.html`/`index-close.html` deben actualizarse juntos:**
  1. `emit_html.go` (`NewHtmlHandler`, `generateStylesheetLink`, `generateJavaScriptTag`)
  2. `emit_favicon.go:125-156` (`updateHtmlFaviconLinks`)
  3. `emit_route.go:97-118` (`emitPages`)
- Hacer commit después de cada etapa. No usar `git reset`, `git restore`, `git checkout -- <file>` ni `git stash`.

---

## Etapa 1: Diagrama y pruebas (TDD)

1. Crear `docs/diagrams/STRICT_CSP_ASSETS.md` (contexto, `flowchart TD`, walkthrough numerado):
   - `mainStyleCssHandler` y `mainJsHandler` minifican → `sriHash(bytes)` calcula `"sha384-" + base64(sha384(bytes))`.
   - `sprite` se emite como `sprite.<hash>.svg` (sin atributo `style`), `svg.SetSpriteURL(url)` se aplica antes de SSR, y `<meta name="webtyp-sprite" content="...">` entra en `<head>`.
   - `translations` se emite como `lang.<hash>.json` (cuando hay diccionario) y `<meta name="webtyp-lang" content="...">` entra en `<head>`.
   - `index.html` recibe `<link rel="stylesheet" href="..." integrity="sha384-..." crossorigin="anonymous">` y `<script src="..." integrity="sha384-..." crossorigin="anonymous"></script>`.
   - `csp.json` escribe `{"script_hashes":["sha384-..."]}` en `OutputDir`.
2. Agregar pruebas en `tests/strict_csp_assets_test.go` (y actualizar `tests/translations_test.go`, `tests/emit_svg_test.go` y `docs/TESTS.md` bajo `| Test | Rule |`):
   - `TestEmitHTML_NoInlineScriptStyleOrSVG`: el `index.html` emitido no contiene `<svg`, `<style`, `style=` ni `<script` con cuerpo interno.
   - `TestEmitHTML_SRIIntegrityMatchesAssets`: los atributos `integrity="sha384-..."` de `<link rel="stylesheet">` y `<script>` coinciden exactamente con el SHA-384 de los bytes finales servidos de `style.css` y `main.js`, y ambos llevan `crossorigin="anonymous"`.
   - `TestEmitSprite_ExternalHashedFile`: el sprite se emite como archivo `sprite.<hash>.svg` en `OutputDir` y `<head>` contiene `<meta name="webtyp-sprite" content="...">`.
   - `TestEmitTranslations_ExternalHashedJSON`: el diccionario se emite como `lang.<hash>.json` y `<head>` contiene `<meta name="webtyp-lang" content="...">` (ningún `<script id="webtyp-lang">` en HTML).
   - `TestEmitCSPManifest_ContainsScriptHashes`: `csp.json` contiene exactamente el `sha384-...` de `main.js`.

## Etapa 2: Sprite y diccionario externos (`emit_svg.go`, `emit_core.go`, `emit_route.go`)

1. En `emit_core.go`:
   - Actualizar interfaz `Translations`: `BundleTranslations(rootDir string) ([]byte, error)`.
   - Reemplazar `translationsHTML string` por `translationsJSON []byte`, `translationsURL string`, `spriteURL string`.
   - Eliminar los dos `c.indexHtmlHandler.AddDynamicContent(...)` de sprite y traducciones (`:700-711`).
   - Emitir el sprite como un `*asset` propio (`sprite.<hash>.svg`, `image/svg+xml`) vía `pwa.HashedName("sprite.svg", spriteBytes)` y guardar `c.spriteURL`; llamar `svg.SetSpriteURL(c.spriteURL)` antes de `emitPages`.
   - En `refreshTranslations`: cuando `len(payload) > 0`, emitir `lang.<hash>.json` (`application/json`) vía `pwa.HashedName("lang.json", payload)` y guardar `c.translationsURL`.
2. En `emit_route.go:111`:
   - Reemplazar `bodyContent := c.renderSpriteNoLock() + p.Body` por `bodyContent := p.Body` (las páginas ya usan `<use href="/sprite.<hash>.svg#..."/>` gracias a `svg.SetSpriteURL`).

## Etapa 3: SRI SHA-384 y manifiesto `csp.json` (`emit_sri.go`, `emit_html.go`, `emit_favicon.go`)

1. Crear `emit_sri.go`:
   - `const CSPManifestFile = "csp.json"`
   - `type CSPManifest struct { ScriptHashes []string \`json:"script_hashes"\` }`
   - `func sriSHA384(data []byte) string`: usa `crypto/sha512.Sum384(data)` + `base64.StdEncoding.EncodeToString` con prefijo `"sha384-"`.
   - Método `(c *Compiler) refreshHTMLSecurityTags()` que calcula los hashes de los bytes actuales de `mainStyleCssHandler` y `mainJsHandler`, reconstruye `index-open.html` (con favicon links, `<meta name="webtyp-sprite">`, `<meta name="webtyp-lang">` si aplica, y `<link rel="stylesheet" href="..." integrity="..." crossorigin="anonymous" />`) e `index-close.html` (con `<script src="..." integrity="..." crossorigin="anonymous"></script>`), y actualiza el `*asset` de `csp.json`.
2. Unificar `emit_html.go` y `emit_favicon.go:125-156` para que ambos usen el mismo constructor de `<head>` y cierre de `<body>` (evitando duplicación entre dos copias de `index-open.html`).
3. Invocar `refreshHTMLSecurityTags()` cada vez que cambia `style.css`, `main.js`, sprite, traducciones o favicons.

## Etapa 4: Traducciones, verificación y documentación

1. Si se agregó o cambió algún texto de `lang.Translate`/`fmt.Err`, ejecutar `langc sync .` y verificar cero claves sin traducir y cero advertencias de reutilización.
2. Actualizar `README.md` (≤ 150 líneas, 4 secciones obligatorias), `docs/ARCHITECTURE.md` (4 secciones) y `docs/TESTS.md`.
3. Ejecutar `gotest`.

## Tabla de etapas

| Etapa | Archivos | Qué |
|---|---|---|
| 1 | `docs/diagrams/STRICT_CSP_ASSETS.md`, `tests/strict_csp_assets_test.go`, `tests/translations_test.go`, `tests/emit_svg_test.go`, `docs/TESTS.md` | Diagrama y pruebas TDD |
| 2 | `emit_core.go`, `emit_svg.go`, `emit_route.go` | Sprite `sprite.<hash>.svg` y traducciones `lang.<hash>.json` fuera del HTML |
| 3 | `emit_sri.go`, `emit_html.go`, `emit_favicon.go` | SRI SHA-384, `<meta>` tags, unificación de `index-open.html` y `csp.json` |
| 4 | `lang.json`, `README.md`, `docs/ARCHITECTURE.md`, `docs/TESTS.md` | Traducciones, documentación y `gotest` verde |
