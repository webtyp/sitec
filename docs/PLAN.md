---
PLAN: "feat: content-hashed release names and PWA() — sitec emits manifest.webmanifest and sw.js through webtyp/pwa"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 11908958760702728851
PR: https://github.com/webtyp/sitec/pull/26
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> Dependencies, all published: `webtyp.com/pwa` v0.1.0 (`pwa.New`, `App.ServiceWorker`),
> `webtyp.com/js` v0.1.0 (`PageBootstrap(wasmURL)`, `DefaultWasmURL`), `webtyp.com/image` v0.1.11
> (`icon-512.png`). Run `go get webtyp.com/pwa@v0.1.0` first. `wasm_builder_exec.go` already calls
> `js.PageBootstrap(js.DefaultWasmURL)`; Stage 2 changes that argument.

# Plan — `sitec`: hashed names in release builds, and `PWA()`

Master plan: [PWA_ARTIFACTS_MASTER_PLAN.md](https://github.com/webtyp/app/blob/main/docs/PWA_ARTIFACTS_MASTER_PLAN.md),
decisions D-PWA-5 and D-PWA-11.

## Why

- **D-PWA-5:** a release build names the page binary, the main stylesheet, the main script and the
  sprite by a hash of their content (`style.3f9a1c2b.css`), so an unchanged file is never downloaded
  again and the browser can cache it forever, and a page can never run the HTML of one version
  with the wasm of another.
- **D-PWA-11:** a project that declares `PWA() pwa.Config` (like it declares `Favicon()`) gets
  `manifest.webmanifest` and `sw.js`. `sitec` does not learn what a service worker is: it hands
  `webtyp.com/pwa` the list of files with their hash and writes the bytes it gets back.

Development builds (`ModeDev`, the dev daemon) do **not** change: fixed names, no service worker
(a service worker caching the shell would fight hot reload).

## Design gate

1. **Prior art.** Vite/Rollup (`[name].[hash].js`, entry HTML rewritten by the bundler), webpack
   (`[contenthash]`, `HtmlWebpackPlugin` injects the hashed tags), Angular CLI
   (`outputHashing: all` in production only, `ngsw.json` from the emitted files). All three hash
   only in production and let the bundler, which writes the HTML, rewrite the references. The
   service-worker side follows Workbox's split: the bundler calls a separate library with the
   emitted file list (`workbox-build`), here `webtyp/pwa`.
2. **Novice-name test.** The project writes one method, `func PWA() pwa.Config` (package function
   or method on the root receiver, exactly like `Favicon()`), and reads "this project is a PWA with
   this config". No new `sitec` exported symbol.
3. **Complexity ledger.** Concepts +1 for a project (`PWA()`), +0 in `sitec`'s API. Ways to make a
   service worker: 1 (after `js.ServiceWorker` is deleted by `js/docs/PLAN.md`). The
   `/client.wasm` string replacement in `startCodeJS` dies (the bootstrap takes the URL).
4. **Where it belongs.** Hashing and reference rewriting belong to the compiler that writes the
   HTML (`sitec`); PWA file contents belong to `pwa`.
5. **What it deletes.** The `strings.ReplaceAll(runtime, "/client.wasm", ...)` in
   `emit_events.go:startCodeJS`.

## Constraints

- `sitec` is a build tool: it runs on the developer's machine and legitimately uses the standard
  library (`crypto/sha256`, `encoding/hex`, `strings`, `path`). Do not replace those imports.
- Follow [AGENTS.md](../AGENTS.md) (tests in `tests/`, messages through the `lang.Translate`
  helpers like `msgNoFavicon`).
- No hardcoded strings in logic: new names (`"client"`, `".wasm"`, the hash length) are constants.
- Run `gotest`. Never run `gopush` or `codejob`.

## Stage 1 — extract `PWA()` like `Favicon()`

Follow every place `Favicon` appears in the extraction path and add `PWA` next to it:

- `scanner.go`: `"PWA": true` in `producerNames`.
- `extract.go`: `receiverFeature.HasPWA`; `CollectorOutput.PWA *pwa.Config \`json:"pwa"\``; in the
  collector template, both branches (receiver and package function):
  `{{if .HasPWA}}{ c := inst.PWA(); s.PWA = &c }{{end}}` (and `{{$alias}}.PWA()`); the generated
  collector imports `webtyp.com/pwa` only when some module has `HasPWA` (mirror how the favicon
  import is handled; `pwa.Config` has only string fields, so `encoding/json` round-trips it).
- `assets.go`: `Assets.PWA *pwa.Config // declared by PWA(); root only; nil = not declared`.
- `pipeline.go`: copy `output.PWA` into `Assets` in both places `Favicon` is copied.
- `merge.go`: a second `PWA()` declaration in the same module is an error, same wording pattern as
  the favicon one: `ssr: multiple PWA() declarations: <a> and <b> — only one package per module may declare PWA()`.
- `emit_core.go` `RouteExtractedAssets`, next to step 0.5: a non-root module declaring `PWA()` is
  an error `msgPWANonRoot(moduleName)` (same shape as `msgFaviconNonRoot`); the root's config is
  stored in a new field `c.pwa *pwa.Config` (nil when not declared).

## Stage 2 — the wasm binary carries its hash in release builds

`wasm_builder_exec.go`:
- After reading `binary`, when the builder is **not** in dev mode, set
  `wasmFilename = hashedName(wasmFilename, binary)` (Stage 3 helper) → `client.3f9a1c2b.wasm`.
- `runtimeJS := js.PageBootstrap("/" + wasmFilename).Content` (today it passes
  `js.DefaultWasmURL`; in dev `"/" + "client.wasm"` equals it).
- `WasmOutput.Filename` is that name; `buildPipeline` already writes it with `c.Write` and passes it
  to `c.SetWasm`.
- `emit_events.go` `startCodeJS`: delete the `if filename != "" && filename != "client.wasm" {
  runtime = strings.ReplaceAll(...) }` block — the bootstrap already has the right URL.

## Stage 3 — `release.go`: a pure final pass over the artifact list

New file `release.go`:

```go
// hashLen is how many hex characters of the content's SHA-256 go into a file name.
const hashLen = 8

// contentHash is the hex SHA-256 of content, 16 characters: the revision of a shell asset.
func contentHash(content []byte) string

// hashedName inserts the first hashLen characters of contentHash before the extension:
// hashedName("style.css", c) == "style.3f9a1c2b.css". A name without extension gets ".<hash>".
func hashedName(name string, content []byte) string

// releaseInput is what the final pass needs from the compiler.
type releaseInput struct {
	CSSURL, JSURL, SpriteURL string         // GetURLPath() of the three main handlers ("" = absent)
	PWA                      *pwa.Config    // nil = the project is not a PWA
	Favicons                 []favicon.File // c.getFaviconFiles()
}

// finalizeRelease returns the artifacts of a release build: PWA tags and register script
// inserted (when PWA != nil), the main CSS, JS and sprite renamed by content hash with every
// HTML reference rewritten, and manifest.webmanifest + sw.js added (when PWA != nil).
func finalizeRelease(arts []Artifact, in releaseInput) ([]Artifact, error)
```

`finalizeRelease`, in this exact order (the order is the point — see `pwa.New` docs):

1. **PWA head and script** (only when `in.PWA != nil`):
   - Icons: from `in.Favicons`, every file with `Sizes` `192x192` or `512x512` →
     `pwa.Icon{URL: path.Join("/", f.Name), Sizes: f.Sizes, Type: f.Mediatype}`. `Favicons` empty →
     error `msgPWAWithoutFavicon()`: "the project declares PWA(); but not Favicon(); — the install
     icons come from Favicon()" (through `lang.Translate`, like `msgNoFavicon`). Any error from
     `pwa.New` (e.g. a logo under 512 px gives no 512 icon) is returned as is: its text already
     names what is missing.
   - `app, err := pwa.New(*in.PWA, icons)`.
   - Every artifact with mediatype `text/html`: insert `app.HeadTags` immediately before the first
     `</head>`. An HTML artifact without `</head>` → error
     `sitec: %s has no </head> to insert the PWA tags into` (artifact path).
   - The artifact whose `Path == in.JSURL`: append `"\n" + app.RegisterScript`.
   - Append `Artifact{Path: pwa.ManifestPath, Mediatype: "application/manifest+json", Content: app.Manifest}`.
2. **Hashed names** (always in release): for each of `in.CSSURL`, `in.JSURL`, `in.SpriteURL` that is
   non-empty and present in `arts`: `newURL := path.Join(path.Dir(old), hashedName(path.Base(old),
   content))`, keeping a relative URL relative (when `old` has no leading `/`, strip the `./` that
   `path.Join` may give and keep no leading `/`). Rename the artifact. In every `text/html` artifact
   replace `"` + old + `"` with `"` + newURL + `"` (quoted, so `/style.css` never matches inside
   another name). The JS artifact must be renamed **after** step 1 changed its content.
3. **Service worker** (only when `in.PWA != nil`): `shell` = every artifact except
   `pwa.ServiceWorkerPath`, `URL = path.Join("/", art.Path)` (the HTML at `/` stays `/`),
   `Revision = contentHash(art.Content)`. `worker, err := app.ServiceWorker(shell)`; append
   `Artifact{Path: pwa.ServiceWorkerPath, Mediatype: "text/javascript", Content: worker.Script}`.
   Log once: `PWA version <worker.Version>, <n> files precached` through the compiler logger
   (pass a `log func(...any)` in `releaseInput` if needed).

Wiring in `build.go`:
- `Output` gets a field `arts []Artifact`. `Output.Artifacts()` returns `s.arts` when non-nil,
  else `s.c.List()` (dev path unchanged).
- In `buildPipeline`, after `copyStaticAssets` and before `routescan.Scan`, when
  `cfg.Mode == ModeRelease`: `arts, err := finalizeRelease(c.List(), releaseInput{...})`; store in
  `out.arts`. `checkRouteCollisions` then runs on the final list (so a route named `/sw.js` collides
  loudly).
- `WriteTo` keeps using `diskPath(art)`: a renamed path maps to its new file name.

## Stage 4 — tests

Unit tests of the pure pass (white-box, `release_internal_test.go` in the package, since
`finalizeRelease` is unexported — AGENTS.md allows that for unexported code):

| Test | Proves |
|---|---|
| `TestHashedName` | `"style.css"` + content → `style.<8 hex>.css`; same content same name; one byte changed → other name; `"LICENSE"` → `LICENSE.<8 hex>` |
| `TestFinalizeRelease_RenamesAndRewritesHTML` | arts: `/` (html with `href="/style.css"` and `src="/script.js"`), `/style.css`, `/script.js`, `/icons.svg`; no PWA → the three renamed, the HTML points at the new names, no `/style.css"` left, no manifest, no `sw.js` |
| `TestFinalizeRelease_RelativeURLsStayRelative` | same with `style.css`/`script.js` (no leading `/`) → `style.<h>.css`, still relative |
| `TestFinalizeRelease_PWA` | PWA config + favicon files with 192/512 → HTML contains `<link rel="manifest"`; JS ends with the register script; artifacts include `pwa.ManifestPath` and `pwa.ServiceWorkerPath`; the SW lists the **hashed** JS URL and `/`; `sw.js` is not in its own list |
| `TestFinalizeRelease_PWAHashesFinalFiles` | the hash in the JS name equals `hashedName("script.js", <content with register script>)` — the script is hashed after insertion |
| `TestFinalizeRelease_PWAWithoutFavicon` | error equals `msgPWAWithoutFavicon()` |
| `TestFinalizeRelease_PWASmallLogo` | favicon files without a 512 → the `pwa` error text containing `missing 512x512` |
| `TestFinalizeRelease_HTMLWithoutHead` | error names the artifact |

Consumer-shaped (`tests/pwa_build_test.go`, black-box, follow the fixture pattern of
`tests/favicon_plan_test.go`): a temporary project whose root declares `Favicon()` (a 512×512 PNG
generated in the test) and `PWA()`; `BuildWithConfig(BuildConfig{RootDir: dir, Mode: ModeRelease})`
→ `Artifacts()` contain `manifest.webmanifest` and `sw.js`, the HTML has the manifest link, and no
artifact is named `style.css`. Same project with `Mode: ModeDev` → no `sw.js`, `style.css` keeps its
name. A non-root module declaring `PWA()` → error from `msgPWANonRoot`.

## Stage 5 — docs

- `docs/ARCHITECTURE.md`: a section "Release names and PWA" — the order of `finalizeRelease` (and
  why: the service worker precaches the final bytes), dev vs release, the `PWA()` declaration next to
  `Favicon()`, and the headers the server must send (hashed names
  `public, max-age=31536000, immutable`; `/`, `sw.js`, `manifest.webmanifest` `no-cache`) — served
  by `server`/`goflare` in a later plan, not by `sitec`.
- `README.md`: how a project becomes a PWA (declare `Favicon()` with a logo ≥ 512 px and `PWA()`),
  with a 10-line example.

## Acceptance

- `gotest` green.
- `grep -n "ReplaceAll(runtime" emit_events.go` → empty.
- `grep -rn "PageBootstrap(js.DefaultWasmURL)" wasm_builder_exec.go` → empty (it passes the built name).

| Stage | Files | Done when |
|---|---|---|
| 1 | `scanner.go`, `extract.go`, `assets.go`, `pipeline.go`, `merge.go`, `emit_core.go` | `PWA()` reaches `c.pwa` |
| 2 | `wasm_builder_exec.go`, `emit_events.go` | release wasm hashed, replacement deleted |
| 3 | `release.go`, `build.go` | final pass wired into release builds |
| 4 | `release_internal_test.go`, `tests/pwa_build_test.go` | tables green |
| 5 | `docs/ARCHITECTURE.md`, `README.md` | written |
