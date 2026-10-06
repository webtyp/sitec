---
PLAN: "feat: sitec syncs translations and inlines the merged dictionary in index.html"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 13641546781818272845
PR: https://github.com/webtyp/sitec/pull/29
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — sitec: translations inlined in the HTML, like the sprite

Phase **T3** of the master plan `SOURCE_SELECTION_MASTER_PLAN.md` (orchestration only — everything
this plan needs is inline). `webtyp/app` (phase D) wires the implementation after this tag exists.
This plan needs no other repo's tag: it defines an interface and tests it with a fake.

Read [AGENTS.md](../AGENTS.md) first. Rules that matter here:
- sitec is backend tooling: the standard library is legitimate.
- Tests live in `tests/`. Stage 3 of this plan aligns the AGENTS rule with the ecosystem rule.

## Why

Translations become data. A project has a `config/lang.json`, and every library a `lang.json` at its
module root. The generator `langc` (in `webtyp.com/lang/langc`, see
`https://github.com/webtyp/lang/blob/main/docs/PLAN.md`) keeps those files in step with the code
and builds the merged dictionary. The client reads that dictionary from the page itself: a
`<script type="application/json" id="webtyp-lang">` element in `index.html`. That is exactly how
sitec already ships the SVG sprite: merged from every module, written inside the HTML, and the HTML
invalidated when it changes. So there is no extra request, and a translation change regenerates the
HTML, never the Go binary.

sitec owns the HTML, so sitec calls the generator and inlines its output. It does that through an
interface the app injects, like `ImageProcessor` and `SSRExtractor`; sitec never imports `lang`.

## Design gate

1. **Prior art.** Next.js inlines `__NEXT_DATA__` as `<script type="application/json">`; Rails
   `i18n-js` and Django `JavaScriptCatalog` ship the catalog with the page; Hugo inlines data at build.
   This repo's own precedent is the sprite inside the HTML.
2. **Novice-name test.** `Translations` (the contract: "the project's translations"),
   `SetTranslations(t)` ("give sitec the translations"), `SyncTranslations` and
   `BundleTranslations` (the generator's two steps).
3. **Complexity ledger.**
   ```
   Concepts the developer must learn   +1 (Translations, same shape as ImageProcessor)
   Files they must touch to do X       +0 (the app wires it once)
   Lines at the call site              +1 in app's composition root
   Ways to do the same thing           +0
   ```
4. **Where it belongs.** sitec owns the HTML and the module scan. `lang/langc` owns the dictionary
   format and merge rules. The interface is the boundary, and sitec never parses `lang.json`.
5. **What it deletes.** Nothing here (the Go dictionaries die in other repos).

## Stage 1 — the contract and the hook

In `emit_core.go`, next to `ImageProcessor`/`SSRExtractor`:

```go
// Translations keeps the project's translation files in step with its code and
// produces the element the client reads its dictionary from.
// webtyp.com/lang/langc implements it; the app injects it.
type Translations interface {
	// SyncTranslations updates <rootDir>/config/lang.json from the code.
	SyncTranslations(rootDir string) error
	// BundleTranslations returns the complete <script type="application/json" ...>
	// element with the merged dictionary, or "" when there is nothing to ship.
	BundleTranslations(rootDir string) (string, error)
}

// SetTranslations installs the translations provider. Without it, no dictionary
// is inlined and the client shows English.
func (c *Compiler) SetTranslations(t Translations)
```

Compiler state, guarded by its own `translationsMu sync.RWMutex`: `translations Translations` and
`translationsHTML string`.

Unexported `func (c *Compiler) refreshTranslations(sync bool)`:
1. if `translations` is nil, return;
2. if `sync`, call `SyncTranslations(<project root>)`, where the project root is the root directory
   the compiler's `Config` already uses for the module scan. On error, `c.writeMessage("translations
   sync error:", err)` and continue: a sync failure never blocks the build;
3. `html, err := BundleTranslations(<project root>)`. On error, write the message and keep the
   previous `translationsHTML`;
4. if `html` differs from `translationsHTML`, store it and call
   `c.indexHtmlHandler.InvalidateCache()`, **after** releasing `translationsMu` (same lock-order rule
   as `setModuleSprite` in `emit_svg.go`).

## Stage 2 — when it runs and where it lands

- **Inline.** In `emit_core.go`, right after the sprite's `AddDynamicContent` (so the element sits in
  `<body>` before `#app` and before the client `<script>`, and exists when the WASM starts):
  ```go
  c.indexHtmlHandler.AddDynamicContent(func() []byte {
  	c.translationsMu.RLock()
  	defer c.translationsMu.RUnlock()
  	return []byte(c.translationsHTML)
  })
  ```
- **After each module scan.** In `LoadSSRModules`, after `RouteExtractedAssets` succeeds, call
  `c.refreshTranslations(true)`. Apply the same in any other path that re-runs extraction for a
  module (`ReloadSSRModule`/`UpdateSSRModule`): `grep -n "RouteExtractedAssets\|func (c \*Compiler) \(Reload\|Update\)SSRModule" *.go`.
- **On a dictionary edit.** In `NewFileEvent` (`emit_events.go`): when the changed file's base name is
  `lang.json`, call `c.refreshTranslations(false)`. That covers both `config/lang.json` and a
  library's `lang.json`. Bundle only, no sync: the human or LLM just edited the file, and the
  generator must not run on every keystroke. Keep the constant `translationsFileName = "lang.json"`.
- `writeMessage` lines must not repeat while the same error persists (follow the existing
  `lastBuildErr` pattern if one exists in this repo; otherwise compare with the last message).

## Stage 3 — tests

`tests/translations_test.go` (`package sitec_test`, public API):
- a fake `Translations` (a small struct in the test file) records calls and returns a fixed element
  `<script type="application/json" id="webtyp-lang">{"default":"es"}</script>`;
- after the module scan used by the existing extraction tests, the rendered `index.html` contains the
  element exactly once, **before** `<div id="app">` and before the client `<script src=`;
  `SyncTranslations` was called once;
- a file event for `config/lang.json` re-bundles without calling `SyncTranslations`, and the HTML
  changes when the fake's element changes;
- `BundleTranslations` returning an error keeps the previous element, and the message is written
  once;
- without `SetTranslations`, the HTML has no `webtyp-lang` element.

AGENTS alignment: the rule "ni un solo `*_test.go` fuera de `tests/`" conflicts with the ecosystem
rule (skill **testing**: a root-level test is allowed only when it needs unexported identifiers, with
a top-of-file justification). Replace that section with the ecosystem rule. Then, for each
`*_internal_test.go` at the root: if it only uses exported identifiers, `git mv` it to `tests/`;
otherwise add `// Root-level test (justified): exercises <unexported identifiers> — <why>.` as its
first lines.

## Acceptance

- `gotest` passes.
- `grep -rn "webtyp.com/lang" --include='*.go' .` → empty (sitec does not depend on lang).
- `for f in *_test.go; do head -1 "$f" | grep -q "Root-level test (justified)" || echo "$f"; done` → empty.

## Stages

| # | Stage | Files |
|---|---|---|
| 1 | Contract | `emit_core.go` |
| 2 | Hook points | `emit_core.go`, `emit_events.go` |
| 3 | Tests + AGENTS | `tests/translations_test.go`, `AGENTS.md`, root `*_internal_test.go` |

## Executor notes

- The executor's PR contained no implementation (only the STATUS change). The planning agent
  implemented the plan on this branch: `Translations`/`SetTranslations`, `refreshTranslations`
  (called after every successful `RouteExtractedAssets` — wrapper around the former body, now
  `routeExtractedAssets` — and after `ReloadSSRModule`, so app's own retry loop is covered too),
  the inline after the sprite, `.json` routed to `NewFileEvent` for `lang.json` only (re-bundle, no
  sync), errors logged once while they persist.
- Tests: `tests/translations_test.go`. The seven root `*_internal_test.go` keep their place with a
  justification line (all use unexported identifiers); `AGENTS.md` now states the ecosystem rule.
