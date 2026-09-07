---
PLAN: "feat!: rename AssetMin to Compiler; read declared routes; check path collisions"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Phase 2b of ROUTES_SINGLE_SOURCE_MASTER_PLAN.md.
> **Blocked until `webtyp.com/router/routescan` is published** (phase 1 gate).

## Prerequisite — install the test runner

External agents run in isolated environments where `gotest` is not installed.
Run this **before anything else**; the acceptance criteria depend on it:

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

Then use `gotest` for the whole suite and `gotest -run TestName` for one test.
Never call `go test` directly: `gotest` handles `-vet`, `-race`, `-cover`, the
WASM suite and the README badges.

# Plan — routes in the build surface

## Context (the executing agent has none — read this fully)

`sitec` is a site compiler: it walks a Go source tree and produces the
deployable static surface — stylesheet, script bundle, SVG sprite, font
declaration and HTML shell. It runs to completion and exits. `sitec build -o dir`
writes the output and prints a JSON manifest on stdout; `sitec check` validates
without writing anything and is used as a CI gate.

An application declares every route it answers in one file, `routes/routes.go`.
`webtyp.com/router/routescan` reads that file at build time and returns
`[]routescan.Decl{Method, Path, Line}`.

### The failure this prevents

The compiled surface is served by a static asset layer that matches **before**
any dynamic handler. If a declared route has the same path as a produced static
artifact, the artifact wins and the route is silently unreachable — the caller
gets a file, and nothing is logged. `sitec` is the only tool that knows both
sides, so it is where the collision must be caught.

### Anti-footgun

This repository is a compiler, not WASM code: the standard library is legitimate
here. Do not "fix" stdlib imports.

## Design gate

Required by skill **api-design** because this plan renames an exported type.

**Prior art (naming).** The type that walks sources and emits a deployable
surface is called `Compiler` in TypeScript's API, `Builder` in Vite/Rollup, and
`Compiler` in webpack. None of them names it after one of its steps.

**Novice-name test.** `AssetMin` reads as "asset minifier". The type holds the
CSS, JS, SVG, favicon and HTML emitters, the SSR state machine, the dedup map
and the disk mirror; minifying is one of its steps. The name understates it by an
order of magnitude, and a reader looking for the compiler will not find it under
that name.

**Ledger.** Concepts −0, lookups −1, ways to do it 0. It is a rename.

**Why now.** 45 files inside `sitec`, 6 in `app`. There are no external users
yet; the cost only grows, and the published tag freezes the name.

## Stage 0 — rename `AssetMin` to `Compiler`

Pure rename, no behaviour change:

| Old | New |
|---|---|
| `AssetMin` | `Compiler` |
| `NewAssetMin(ac *Config) *AssetMin` | `NewCompiler(cfg *Config) *Compiler` |

The field `Output.am *AssetMin` becomes `Output.c *Compiler`. Rename the
receiver `c *AssetMin` consistently; do not leave a mixed `am`/`c` convention.

`webtyp.com/app` breaks until it follows (6 files: `minify_toggle.go`,
`section-build.go`, `ssr_loader.go`, `ssr_watcher.go`, `image_processor.go`,
`ssr_loader_test.go`). That is expected and tracked in the master plan.

**Acceptance:** `grep -rn "AssetMin" --include='*.go' .` → empty.

## Stage 1 — dependency and scan

`go get webtyp.com/router@latest`.

Add to `build.go`:

```go
// Routes returns the routes the project declared in routes/routes.go, in
// source order. Empty when the project declares none.
func (s *Output) Routes() []routescan.Decl
```

`Build` calls `routescan.Scan(cfg.RootDir)` and stores the result on `Output`.
A scan error fails `Build` — it is a malformed declaration, not a missing one.
A project without `routes/routes.go` produces an empty slice and no error.

## Stage 2 — the collision check

Add to `build.go`:

```go
// ErrRouteCollides is returned when a declared route has the same path as a
// produced static artifact. The artifact is served first, so the route would be
// unreachable with no error anywhere.
const ErrRouteCollides = "route %s %s (routes/routes.go:%d) collides with the static asset %s — the asset is served first and the route would never run"
```

The comparison is between a `Decl.Path` and an artifact path, both normalised to
a leading `/` and no trailing `/`. A `Decl` whose path contains `{` is a
parameterised route and is compared only up to the segment before it; a route
that is a prefix of an artifact path does **not** collide — only an exact match
does.

Wire it into **both** entry points:

- `Build` returns the error, so a colliding project never produces output.
- `Check(rootDir string, log func(...any)) ([]string, error)` reports it as one
  of its findings, so CI fails before a deploy.

## Stage 3 — the manifest

`cmd/sitec/main.go` adds the routes to the `build` manifest, so a caller reading
stdout sees what the site answers dynamically:

```json
{
  "status": "success",
  "command": "build",
  "artifacts": [ … ],
  "routes": [ { "method": "GET", "path": "/api/contacto" } ]
}
```

Add a `Routes []Route` field to `Manifest` and a `Route{Method, Path string}`
type next to the existing `Artifact` type in that file. Omit the field when
empty (`json:"routes,omitempty"`).

`cmd/` stays thin: it maps `site.Routes()` into the manifest type and prints.
Every decision stays in the library.

## Constraints

- **No hardcoded strings.** Path separators, the `{` marker and every message
  are named constants.
- **stdout = data, stderr = logs.** Unchanged: the manifest goes to stdout, the
  `Log` callback writes to stderr. A collision message is a returned error, not
  a print.

## Tests

Table-driven, each case writing a fixture tree into `t.TempDir()`:

1. Project with no `routes/routes.go` → `Routes()` empty, `Build` succeeds.
2. Project with three routes → `Routes()` returns them in source order.
3. A route `/style.css` against a produced `style.css` artifact → `Build`
   returns the verbatim error with the right line number, and writes nothing.
4. The same case through `Check` → reported as a finding.
5. A route `/api/contacto` with no matching artifact → no error.
6. A parameterised route `/api/orders/{id}` → no collision against
   `/api/orders`.
7. A malformed path in `routes/routes.go` → `Build` fails with the `routescan`
   error.

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` → clean.
2. `grep -n "routescan" build.go` → non-empty.
3. Test 3 fails against `main` today (it is the regression proof).
4. `sitec build -o out` on a fixture prints a manifest whose `routes` array
   matches the declarations.

## Stages

| # | Stage | File(s) | Gate |
|---|---|---|---|
| 0 | rename `AssetMin` → `Compiler` | `emit_*.go`, `build.go`, `ssr_loader.go` | grep empty |
| 1 | dep + `Output.Routes()` | `build.go` | tests 1, 2 |
| 2 | collision check | `build.go` | tests 3–7 |
| 3 | manifest | `cmd/sitec/main.go` | criterion 4 |

Sequential.
