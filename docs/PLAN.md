---
PLAN: "feat: exported package variables of the app are page and loading-HTML producers"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 3500701303836341610
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

## Tools

```
go install webtyp.com/devflow/cmd/gotest@latest
```

## Why

An app today writes a type only so sitec finds it (`config/html.go` in `veltylabs/mjosefa-cms`):

```go
type PreAuthScreen struct{ dom.Element }
func (p *PreAuthScreen) RenderPages() []html.Page { return appkit.LoginPages(Brand) }
func (p *PreAuthScreen) RenderHTML() string        { return appkit.LoadingHTML() }
```

The app already declares the value that knows how to render: `var Brand = appkit.Brand{Name: "MJosefa CMS", LogoMark: "..."}`.
After this plan, sitec also calls `RenderPages()` and `RenderHTML()` on **exported package-level variables of the app's own
packages** whose value has those methods, so that file is deleted. (`appkit.Brand` gains the two methods in another plan;
a zero `Brand` renders nothing, so the zero-value receiver discovery that already exists stays harmless.)

## Ecosystem search

Searched `sitec` (`scanner.go`, `select.go`, `extract.go`): producers are only functions and methods, instantiated as a
zero value (`inst := &alias.Type{}`). No other tool discovers values. **Extend** sitec.

## Rules

- Only packages whose directory is inside the app's project root (`findProjectRoot(startDir)`, `select.go:12`) are scanned
  for variables. Library packages are never (their exported vars would be called with whatever they hold).
- Only `RenderPages() []html.Page` and `RenderHTML() string`. No other producer name is matched on variables.
- The generated code uses a type assertion, so a variable of any other type compiles and is skipped:

```go
if p, ok := any(cfg.Brand).(interface{ RenderPages() []html.Page }); ok {
	s.Pages = append(s.Pages, p.RenderPages()...)
}
if p, ok := any(cfg.Brand).(interface{ RenderHTML() string }); ok {
	s.HTML += p.RenderHTML()
}
```

## Stages

### Stage 1 — tests first (`tests/emit_vars_test.go`, new)

Copy the setup of `tests/emit_pages_test.go` (a temporary app module with `config/`). Three tests:

| Test | Setup in the temp app's `config/` | Expected |
|---|---|---|
| `TestVarProducer_Pages` | `type brand struct{ name string }`, `func (b brand) RenderPages() []html.Page { if b.name == "" { return nil }; return []html.Page{{Path: "/", Body: "<p>" + b.name + "</p>"}} }`, `var Brand = brand{name: "Acme"}` | the extracted pages contain exactly one page, `Path == "/"`, body contains `Acme` |
| `TestVarProducer_HTML` | same type with `func (b brand) RenderHTML() string { return "<p>loading " + b.name + "</p>" }` | extracted HTML contains `loading Acme` |
| `TestVarProducer_OtherTypeIgnored` | `var Port = 8080` and `var Names = []string{"a"}` next to a normal producer | extraction succeeds (the generated main compiles), output unchanged |

Commit.

### Stage 2 — scanner (`scanner.go`)

- `fileFeatures` gains `vars []string`: exported names of package-level `var` declarations (`*ast.GenDecl` with
  `Tok == token.VAR`, each `*ast.ValueSpec` name with `ast.IsExported(name)`).
- The package-level aggregate returned by `scanPackage` gains `Vars []string` (all files of the package, in file order).

### Stage 3 — selection and template (`select.go`, `extract.go`)

- `moduleAlias` gains `Vars []string`, filled only when `m.dir` is inside the project root (compute
  `root, err := findProjectRoot(startDir)` once in `modulesToAliases`; inside = `strings.HasPrefix(m.dir+"/", root+"/")`).
- A module with `len(Vars) > 0` counts for `HasAnyFeature` (so its import is emitted) and for `HasAnyPages`
  (so `webtyp.com/html` is imported).
- In the template, inside each module block next to the receivers, emit per variable the two assertions shown in
  **Rules**, wrapped in the same `func(){ defer recover … }()` as the receivers, with `Type: "{{.}}"`.

Commit.

### Stage 4 — documentation

`README.md` (≤150 lines, its 4 sections): in the producers section add one paragraph and this example:

```go
// config/brand.go — sitec calls RenderPages and RenderHTML on the value
var Brand = appkit.Brand{Name: "Acme", LogoMark: logo}
```

`docs/ARCHITECTURE.md`: one line in the discovery section: values are discovered only in the app's own packages.

## Delivery check

`gotest` passes (it builds and vets). A PR that does not compile, or whose tests could not run, is a failed delivery.
Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`.
Never ask permission; commit after every stage; never `git reset`/`restore`/`stash` your own work. Stuck 30 minutes:
open the PR listing what fails.

| Stage | Files |
|---|---|
| 1 | `tests/emit_vars_test.go` |
| 2 | `scanner.go` |
| 3 | `select.go`, `extract.go` |
| 4 | `README.md`, `docs/ARCHITECTURE.md` |
