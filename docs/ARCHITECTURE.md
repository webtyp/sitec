# Architecture of `sitec`

Defines the **what** and **why** of asset extraction. Abstract structure only —
exact detection rules, merge semantics and error conditions are in
[SPECS.md](SPECS.md); the reasoning and rejected alternatives are in
[DESIGN.md](DESIGN.md).

---

## 1. What `sitec` is

The build-time extractor. It finds every package in a project that produces
assets, runs their producers, and hands the results to `assetmin` as one set per
module.

It exists so that a component author writes **only** the component. There is no
registry to update, no init function, no manifest — a package that declares a
producer is collected because it declares one.

Module discovery is delegated to `webtyp/modfind`, so lookups are shared and
cached across `webtyp` tools.

---

## 2. Position in the suite

| Module | Owns | Never does |
|---|---|---|
| `webtyp/css` | **Values** — the token catalog, light/dark switching, contrast guarantees | Know anything about components |
| `webtyp/widget` | **Decisions** — which token applies to which part in which state | Invent a value |
| `sitec` | **Delivery** — collect the sheets actually used, order and deduplicate them | Know what a widget is |

See [diagrams/EXTRACTION.md](diagrams/EXTRACTION.md).

`sitec` knows nothing about widgets, surfaces or layers. It knows that some
packages produce strings of CSS and that those strings must arrive at `assetmin`
complete, ordered, and without redundancy.

**Deduplication belongs here and nowhere else.** A stylesheet is built from one
component's declarations and cannot know how many other components exist, so it
cannot decide what is redundant. `ssr` merges all of them and is the only layer
that sees the duplication.

---

## 3. The producer contract

A package is collected if it declares at least one method with one of these
names, on any type, in any non-test file.

| Method | Result field | Meaning |
|---|---|---|
| `RootCSS()` | `RootCSS` | `:root` token declarations |
| `RenderCSS()` | `CSS` | component CSS, scoped to the component |
| `RenderHTML()` | `HTML` | prerendered markup |
| `RenderJS()` | `JS` | scripts |
| `IconSvg()` | `Icons` | sprite, merged across packages |
| `Fonts()` | `Fonts` | typeface identity (`font.Declaration`); one per module |
| `RenderSite()` | `Site` | site declaration (`*sitec.Site`): public URL + static assets. Only the project root may declare it; a second root or any non-root module declaring it is a build error/warning respectively |
| `Favicon()` | `Favicon` | site icon (`favicon.Source` via `webtyp.com/image/favicon`): derives `icon-32.png`, `icon-192.png`, `apple-touch-icon.png`, `favicon.ico` and `favicon.svg` (when SVG provided). Only the project root may declare it; a non-root module declaring it fails the build naming the module |

A module's `Site` travels with the module: the root's declaration is what the
assembler listens to. It resolves the effective `SiteURL` once (the project
wins over `BuildConfig.SiteURL`, with a warning when both disagree) and feeds
two outputs downstream: `sitemap.xml` when `URL` is non-empty, and the static
assets copied verbatim (united with `BuildConfig.StaticAssets`, deduped). A
declared-but-absent static asset fails the build — the project owns its
identity, so a missing logo must fail in CI, not in production.

`RenderSite()` also makes the intent explicit, which turns silent failures
into checks: a site declared without `RenderPages()` is a build error (the
output would be an app shell, not a site), and pages without a `RenderSite()`
warn that the output will ship without sitemap or static assets.

Obligations on the author:

1. **Zero-value instantiation.** Producers run on `&T{}`. A producer must not
   read fields.
2. **Purity.** Same input, same bytes. Extraction is cached by content hash and
   its output is committed downstream.
3. **Any number of types per package** may declare producers.
4. **Declaring none while importing an asset-producing library is an error**, not
   a skip.

### 3.1 Detection matches the method name only

Never the signature, never the return type, never the package that type comes
from. This is what allowed `IconSvg()` to change its return type without touching
the extractor, and it is why the generated program can type the sprite as `any`:
that program is compiled against whatever the target package actually returns.

Preserving this property is a hard constraint on any change to detection.

The price of matching by name is that **a producer's name must be unique in the
ecosystem**: any scanned package declaring a method or function with that name
is called as a producer. Before adding one, grep every `webtyp.com/*` module for
the name. `Artifacts` failed this test (`min.Handler.Artifacts()`,
`sitec.Output.Artifacts()`), so the large-artifacts producer is
`ArtifactSources()`; `TestArtifacts_CommonNameIsNotAProducer` keeps it that way.

---

## 4. Extraction model

`ssr` does not parse Go to evaluate producers — it **compiles and runs them**. A
program is generated that imports every collected package, instantiates each
producer type, calls its methods, and encodes the results as JSON on stdout.

This is why detection needs only to identify *names and types*, and why producers
may return any type with the right shape: correctness is delegated to the Go
compiler rather than reimplemented.

Results are cached by a content hash over every non-test `.go` file in the module
set, so an unchanged project does not recompile.

---

## 5. Merge and ordering guarantees

1. **A module's assets include its subpackages.** Producers live in
   `config/`, `modules/x/` — rarely at the module root, which is usually
   `package main` and cannot be imported by a generated program.
2. **Stable order.** Packages are merged in sorted path order, so emitted CSS does
   not shuffle between runs.
3. **One cascade-layer statement.** The merged output declares layer order once,
   before any rule. Two packages declaring different layer orders is an error: the
   cascade of the whole application depends on it.
4. **No redundant rules.** Byte-identical declaration blocks within the same layer
   are merged into one rule with a combined selector list, preserving the position
   of the first occurrence.

---

## 6. Failure posture

**An asset that should have been collected and was not is a defect, not a
warning.** A missing stylesheet produces a component that renders unstyled while
the build stays green — the most expensive failure this module can have, because
nothing reports it and the symptom appears far from the cause.

Every detection gap is therefore resolved in the same direction: make the
extractor find it, or fail the build naming the package. Never skip quietly.

---

## 7. Release names and PWA

In release builds (`ModeRelease`), `sitec` executes a pure finalization pass (`finalizeRelease`) over all produced artifacts:

1. **PWA head and register script injection** (when `PWA()` is declared):
   - Reads `PWA()` config from the root module and install icons from `Favicon()`.
   - Injects `<link rel="manifest">` and PWA meta tags into all HTML artifacts (`app.HeadTags` before `</head>`).
   - Appends `app.RegisterScript` to the main JS script artifact.
   - Generates `manifest.webmanifest`.
2. **Content hashing and reference rewriting**:
   - Computes SHA-256 content hashes for the main CSS and JS artifacts. The icon sprite has no
     file of its own: it is rendered inside every HTML page, the only place where `<use href="#id">`
     symbols can be styled by CSS and manipulated through the DOM.
   - Renames them to content-hashed paths (e.g., `style.3f9a1c2b.css`).
   - Rewrites all quoted references inside HTML artifacts to match the new content-hashed URLs.
   - *Note:* The main JS file is hashed **after** step 1 appended the PWA register script, so its filename reflects its true final content.
3. **Service worker generation** (when `PWA()` is declared):
   - Constructs the precache manifest (`pwa.Asset` list with exact URLs and content hashes) from all finalized shell artifacts.
   - Invokes `app.ServiceWorker` from `webtyp.com/pwa` to generate `sw.js`.
   - *Note:* The service worker precaches the final content-hashed asset URLs, ensuring that cached resources never become stale or out of sync with HTML markup.

### Development vs. Release behavior

- **Development (`ModeDev`)**: Asset names remain fixed (`style.css`, `script.js`) to support instant hot-reloading. Service workers and manifests are omitted so caching does not interfere with active development.
- **Release (`ModeRelease`)**: Assets are minified, content-hashed, and PWA files (`manifest.webmanifest`, `sw.js`) are emitted when `PWA()` is declared.

### PWA declaration

A project declares its PWA configuration next to `Favicon()` on its root receiver:

```go
func (a *App) PWA() pwa.Config {
    return pwa.Config{
        Name:            "My App",
        ShortName:       "App",
        ThemeColor:      "#0080ff",
        BackgroundColor: "#ffffff",
    }
}
```

Only the root module of a project may declare `PWA()`. Declaring `PWA()` in a non-root module or without declaring `Favicon()` fails the build.

### Large artifacts

A project declares the large files its browser code downloads (model weights,
caches) with `ArtifactSources() []artifacts.Source` in its root package (`!wasm`),
next to `Favicon()` and `PWA()` (D-PWA-15):

1. Extraction brings the sources to `Compiler.artifactSources` (root only).
2. `artifacts.BuildManifest` measures each file — size and SHA-256, streamed,
   never held in memory — and `/artifacts.json` is written into the shell
   (precached like any shell file).
3. `Build` places each file at its URL under `/artifacts/` with a hard link,
   or a streamed copy when linking fails (`placeLargeFiles`). The files are
   never an in-memory `Artifact` and never precached.
4. `Output.LargeFiles()` lists URL → path on disk for deployers that upload
   instead of writing to disk.

The development daemon does not place them: an application tests its downloads
after a release `Build`.

### Serving infrastructure cache headers

When serving release artifacts, the serving layer should configure HTTP response headers as follows:

- **Hashed assets** (`style.3f9a1c2b.css`, `script.816fa8ef.js`, `client.3f9a1c2b.wasm`): `Cache-Control: public, max-age=31536000, immutable`
- **Shell and dynamic entry points** (`/`, `sw.js`, `manifest.webmanifest`): `Cache-Control: no-cache`
- **Large artifacts** (`/artifacts/…`): `Cache-Control: no-store` — `artifacts` keeps them in OPFS itself

---

## Related documents

- [SPECS.md](SPECS.md) — exact detection, merge and error behaviour.
- [DESIGN.md](DESIGN.md) — why, and what was rejected.
- [diagrams/EXTRACTION.md](diagrams/EXTRACTION.md) — the pipeline.
