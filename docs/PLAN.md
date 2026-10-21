---
PLAN: "fix: fail loudly when an asset file of a reachable package imports a package the reachability probe skipped"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.

# Reachability self-check

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Why

v0.2.68 fixed a regression where `style.css` was served empty: the reachability probe
(`reach.go`, `goListDeps`) listed the native graph without `-tags=browser`, so every package
reached only through `web/browser.go` (`//go:build browser`) and its `!wasm` asset files
(`css.go`) was reported "not in the ./... build graph" and skipped — including
`webtyp.com/css`. The only symptom was an empty stylesheet; the skip list is logged only in
verbose mode.

The probe can be checked against sitec's own data: the generated extractor compiles the asset
files (files that declare a producer such as `RenderCSS`) of every package it keeps. If such a
file imports a package that was skipped as unreachable, the probe is wrong **by definition** —
that import IS compiled. This plan turns that contradiction into an error, so the next probe
gap (a new build tag, a new destination) fails the build instead of shipping an empty
stylesheet.

No public API changes. Do NOT change `GraphLister`, `goListDeps`, `listDeps`,
`destinationTags` or `buildTargets`.

## Stage 1 — `scanner.go`: imports of asset files

In `packageFeatures` add a field:

```go
type packageFeatures struct {
	PkgName   string
	Imports   map[string]bool
	// ProducerImports is the union of the imports of the files that declare
	// at least one producer: exactly the files the generated extractor compiles.
	ProducerImports map[string]bool
	Producers       []producerDecl
}
```

In `scanPackage`, initialise `ProducerImports: make(map[string]bool)` next to `Imports`, and
inside the loop, after the existing `for imp := range feats.imports` block:

```go
if len(feats.producers) > 0 {
	for imp := range feats.imports {
		agg.ProducerImports[imp] = true
	}
}
```

## Stage 2 — `select.go`: the self-check in `modulesToAliases`

1. Build `skippedSet := make(map[string]bool)` and add each path where the code today does
   `skipped = append(skipped, m.path)`.
2. Inside the loop, where `feats` is computed for a kept package (`m.dir != ""`), record the
   importer: keep a slice `kept []keptPkg` with `type keptPkg struct{ path string; imports map[string]bool }`
   (unexported, declared in `select.go`), appending `keptPkg{m.path, feats.ProducerImports}`.
3. After the loop, **before** the existing verbose summary log, add:

```go
for _, k := range kept {
	for imp := range k.imports {
		if skippedSet[imp] {
			return nil, fmt.Errorf(unreachableButImportedFmt, imp, k.path)
		}
	}
}
```

Iterate `kept` in slice order and, for a deterministic message, sort the import paths of each
`k.imports` before checking (collect the keys, `sort.Strings`).

4. In `reach.go`, next to `skippedUnreachableSummaryFmt`, add the constant verbatim:

```go
// unreachableButImportedFmt reports a reachability probe that contradicts the
// extractor: a kept package's asset file imports a package the probe skipped.
const unreachableButImportedFmt = "sitec: package %s was skipped as unreachable, but %s imports it from an asset file that the extractor compiles; " +
	"the reachability probe is missing a build configuration (see buildTargets and destinationTags in reach.go)"
```

## Stage 3 — test `tests/unreachable_package_test.go`

Add `TestExtractAll_ProbeMissingConfigurationFailsLoudly`. Setup: copy the fixture of the
existing `TestExtractAll_PackageImportedOnlyByBrowserMainShips` in the same file (app module
with `web/browser.go` tagged `browser` importing `example.com/dep/shell`; `dep/shell/css.go`
`!wasm` importing `example.com/dep/theme`; `dep/theme/css.go` `!wasm` with `RenderCSS`).
Extract the fixture into a helper `setupBrowserOnlyProject(t) (appDir, depDir string, finder fakeModules)`
and make both tests use it.

Call: `e.SetGraphLister(lister)` where `lister` reproduces the pre-v0.2.68 probe by running the
real toolchain:

```go
lister := func(rootDir, pattern, goos, goarch string) ([]string, error) {
	args := []string{"list", "-e", "-deps"}
	cmd := exec.Command("go")
	cmd.Dir = rootDir
	cmd.Env = os.Environ()
	if goos != "" {
		args = append(args, "-tags=browser")
		cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH="+goarch)
	}
	cmd.Args = append([]string{"go"}, append(args, pattern)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}
```

Expected: `ExtractAll` returns a non-nil error whose text contains both
`example.com/dep/theme` and `example.com/dep/shell`.

The existing tests must stay green unchanged (`gotest` runs them all).

## Stage 4 — `docs/ARCHITECTURE.md`

Add a section `## Reachability` (English, after the section that describes extraction):

- The probe is the union of `go list -e -deps ./...` over `buildTargets` (native, `js/wasm`) ×
  `destinationTags` (none, `browser`). Why both tags on native: asset files are `!wasm`, and the
  packages they import are reachable only from the native graph with the destination tag of the
  binary that imports them (`web/browser.go`).
- Self-check: a skipped package imported by an asset file of a kept package is an error
  (`unreachableButImportedFmt`) — the probe and the extractor must agree.

## Delivery check

```bash
go build ./...
gotest
grep -n "unreachableButImportedFmt" reach.go select.go   # constant + use
grep -n "ProducerImports" scanner.go select.go
```

A PR that does not compile, or whose tests could not run, is a failed delivery. Environment:
`GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`.

Never pause to ask permission or report progress: when the delivery check passes, commit, push
and open the PR. Stuck for 30 minutes → open the PR anyway listing what fails.

## Stages

| # | Files | Result |
|---|---|---|
| 1 | `scanner.go` | `ProducerImports` per package |
| 2 | `select.go`, `reach.go` | error when the probe contradicts the extractor |
| 3 | `tests/unreachable_package_test.go` | failing-probe test + shared fixture helper |
| 4 | `docs/ARCHITECTURE.md` | reachability section |
