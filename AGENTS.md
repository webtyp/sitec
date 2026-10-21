# AGENTS.md — `webtyp/sitec`

<!-- webtyp:base — copied from devskills skill agents-md; do not edit here -->
## Ecosystem rules — read before any change

`docs/PLAN.md` is the spec: no design judgment; anything unclear is a question in the PR, not a guess.

- Public API: typed (no `any`/`interface{}`), explicit, minimal; never export a symbol only a test uses.
- One intent, one path: the old path is deleted in the same change. No silent fallback, `TODO`, commented code or stub.
- Fail at compile time, else a loud diagnostic — never silently. Zero values deny.
- A defect in a dependency is reported, never wrapped or patched here.
- No string literals in logic: named constants. The same logic never lives in two repos.
- `cmd/*/main.go`: parse flags, inject, print/exit only. stdout = data, stderr = logs, exit codes are the contract.
- WASM code: `webtyp/fmt`, `webtyp/time`, `webtyp/json` instead of stdlib; no `map`; `dom.Element` embedded by value.
- `css.go`, `svg.go`, `js.go`, `html.go` carry `//go:build !wasm`; no `ssr.go`, no `front.go`.
- Code and keys in English; UI text is `lang.Text`, translations live in `lang.json`.
- Tests cover behaviour and domain rules, never the look (CSS, colours, markup order, class strings). In `tests/` (external package), real stack with fakes only at the edge; run `gotest`.
- Docs (`README.md`, `docs/*`) updated with the API; permanent docs never link `docs/PLAN.md`.
- Before the PR: `git diff --stat main` lists every file of the plan's stages, and nothing the plan did not order is deleted.
<!-- /webtyp:base -->

## This repo

`sitec` is the **site compiler**: it takes a Go source tree and emits the deployable static surface (`style.css`, `script.js`, inline SVG sprite inside `index.html`, fonts, `browser.wasm`, Web Workers, PWA manifest and service worker).

- **Backend tooling:** `sitec` runs on the developer host and in CI. Standard library packages (`os`, `os/exec`, `encoding/json`, `go/ast`, `go/parser`, `sync`, `io`, `strings`) are legitimate here; do NOT replace them with WASM-only wrappers. Use `webtyp.com/fmt` (`fmt.Err`) for error construction.
- **Tests:** all tests in `tests/` (`package sitec_test`), grouped by filename prefix (`emit_`, `serve_`, `extract_`), never subdirectories. Root-level tests (`*_internal_test.go`) are permitted only with a first-line `// Root-level test (justified): exercises ...` comment when testing an unexported seam.
- **Invariants:**
  1. Reachable packages are the **union** of the native server graph and the `GOOS=js GOARCH=wasm -tags=browser` graph (`reach.go`).
  2. Scope is anchored at the start directory, not `go.mod` root.
  3. `package main` directories are never asset-producer packages.
  4. If a reachability probe fails, degrade open (`partial: true`), never closed.
  5. An empty extraction is an error, never a silent success.
  6. Default mode is in-memory (`NewMemFS`).
