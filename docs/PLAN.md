---
PLAN: "fix: disambiguate SSR module import aliases that share a last path segment"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 14885612290372753622
PR: https://github.com/webtyp/sitec/pull/23
---

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

# Plan — fix the `m_<name>` alias collision in `modulesToAliases`

You are an agent with **no prior context** and you have **only this repository**
(`webtyp.com/sitec`). Everything you need is inline.

## 1. The problem, reproduced live

`select.go`'s `modulesToAliases` derives a generated-code import alias for
every SSR-relevant package from the **last path segment of its import path**
alone:

```go
// select.go, current code
parts := strings.Split(m.path, "/")
alias := strings.ReplaceAll(parts[len(parts)-1], "-", "_")
alias = aliasPrefix + alias // "m_" + last segment
```

Two **different** import paths that happen to share a last segment collide on
the same alias, and nothing detects it. This is not hypothetical — it broke a
real project's dev server:

```
$ webtyp dev   # inside veltylabs/mjosefa-cms
./main.go:29:2: m_appointment_booking redeclared in this block
	./main.go:13:2: other declaration of m_appointment_booking
./main.go:29:2: "github.com/veltylabs/appointment_booking" imported as m_appointment_booking and not used
./main.go:412:35: undefined: m_appointment_booking.Module
```

`github.com/veltylabs/mjosefa-cms/modules/appointment_booking` (an
application's own thin wrapper package) and
`github.com/veltylabs/appointment_booking` (the library it wraps) both end in
`/appointment_booking`, so both compute the alias `m_appointment_booking`. The
generated `.ssr_extract/main.go` (built by `invokeSSRExtractorOnce` in
`extract.go`) ends up importing the same alias twice, fails to compile, and
SSR is blocked for the whole project until a human notices and works around
it.

**This is not an edge case unique to that one project.** The
`modules/<name>` wrapper package next to a same-named upstream library is a
**standard, repeated pattern** in the `veltylabs` ecosystem — every domain
module there (`staff_manager`, `device_manager`, `item_catalog`,
`clinical_encounter`, `patient_directory`, `business_calendar`,
`appointment_booking`) is wrapped by a same-named local package for exactly
this reason. Any one of them collides the moment BOTH the wrapper and the
library it wraps are independently SSR-relevant (each has its own
`IconSvg()`/`RenderCSS()` etc.) — which is increasingly the norm, not the
exception, as this ecosystem grows. Fixing this only in one consuming
project's folder layout is not an option: the fix belongs in the alias
generator, which is the one place that can see both colliding paths at once
and is the one place a workaround would otherwise be needed in every project
that hits it.

## 2. Design gate

This plan changes no exported symbol — `modulesToAliases`, `moduleAlias`, and
`aliasPrefix` are all unexported, and the fix changes only unexported
behavior. The api-design gate does not apply; skip to stages.

## 3. Decisions already taken — do not revisit

1. **Disambiguate by walking up additional path segments, not by appending a
   numeric suffix.** `m_modules_appointment_booking` vs
   `m_veltylabs_appointment_booking` tells a human reading the generated
   `main.go` (or a build error naming that alias) which package is which.
   `m_appointment_booking` vs `m_appointment_booking_2` does not — and worse,
   which one gets the bare name and which gets `_2` depends on iteration
   order, so the same project can print a different alias for the same
   package on a different run. Path-segment disambiguation is stable: it
   depends only on the two paths themselves.
2. **Detect the collision by full path, not by re-deriving and comparing
   aliases blindly.** Two `module` entries can legitimately share a `path`
   already (the existing `seen[pkgPath]` dedup in the caller that builds
   `modules` — see `discoverModules` if you need to confirm — prevents
   that upstream); a TRUE collision here is always two *different* full paths
   producing the same short alias. The fix must key on `(alias, path)`, so
   that the same path is never treated as colliding with itself in a second
   pass.
3. **No limit assumed on how many extra segments are needed.** Two paths that
   differ at all (guaranteed, since they are different Go import paths) can
   always be disambiguated by walking toward the module root — worst case,
   the full paths themselves are unique. Do not hardcode "try one extra
   segment, then give up."

## 4. Stages

### Stage 1 — the fix, in `select.go`

Replace the single-pass alias derivation inside `modulesToAliases`'s loop with
a collision-aware version. The exact insertion point is where `alias` is
computed today (`select.go`, inside the `for _, m := range
expandToSSRPackages(...)` loop, right before `ma := moduleAlias{...}`).

Add, above `modulesToAliases` (or as a new unexported helper next to it):

```go
// aliasFor returns a Go import alias for path, unique against every path
// already recorded in used (a path -> alias map built incrementally as the
// caller processes each module). It starts from the last path segment
// (today's behavior) and, on collision with a DIFFERENT path, walks one more
// segment toward the module root at a time until the alias is unique.
//
// A collision is only ever between two DIFFERENT full paths: the caller
// dedupes on path before this is reached, so the same path is never
// re-processed here.
func aliasFor(path string, used map[string]string) string {
	parts := strings.Split(path, "/")
	depth := 1
	for {
		start := len(parts) - depth
		if start < 0 {
			start = 0
		}
		candidate := aliasPrefix + strings.ReplaceAll(strings.Join(parts[start:], "_"), "-", "_")
		if existingPath, ok := used[candidate]; !ok || existingPath == path {
			return candidate
		}
		if start == 0 {
			// Ran out of segments — both full paths, sanitized the same way,
			// produce the same string. This can only happen if the two paths
			// are identical once "-" is normalized to "_", which the caller's
			// own path-level dedup already rules out. Returning candidate
			// here (rather than panicking) keeps this function total; the
			// caller's own bookkeeping still records the true path for both,
			// so a second real collision surfaces as a Go compile error
			// exactly as visible as today's bug, never a silent one.
			return candidate
		}
		depth++
	}
}
```

Then in `modulesToAliases`, replace:

```go
parts := strings.Split(m.path, "/")
alias := strings.ReplaceAll(parts[len(parts)-1], "-", "_")
alias = aliasPrefix + alias
```

with:

```go
alias := aliasFor(m.path, aliasByCandidate)
aliasByCandidate[alias] = m.path
```

where `aliasByCandidate := map[string]string{}` is declared once, before the
`for _, m := range expandToSSRPackages(...)` loop begins (next to `var
skipped []string` and `var aliases []moduleAlias`).

**Anti-footgun.** `used[candidate]` must map **alias → path**, not path →
alias — the lookup at collision-detection time is "is this candidate alias
already taken, and if so, by the path I'm currently processing or a different
one", which needs the alias as the key.

### Stage 2 — regression test

New file **at the repository root** (not under `tests/`):
`alias_collision_internal_test.go`, `package sitec`, `//go:build !wasm`. This
repo's one documented exception to "every test lives in `tests/`" is exactly
this shape — white-box access to an unexported function. Copy the justifying
header comment style from the existing exception,
`emit_flush_internal_test.go`, adapted to name `aliasFor`/`modulesToAliases`
instead of `Compiler`'s fields.

Required cases:

| Test | Asserts |
|---|---|
| `TestAliasFor_NoCollisionKeepsLastSegment` | `aliasFor("github.com/veltylabs/staff_manager", map[string]string{})` → `"m_staff_manager"` — unchanged behavior when there is nothing to disambiguate |
| `TestAliasFor_CollisionWalksUpOneSegment` | **exact expected values, worked out below — assert these literal strings, do not recompute them.** Process `"github.com/veltylabs/mjosefa-cms/modules/appointment_booking"` FIRST, threading `used` into a second call with `"github.com/veltylabs/appointment_booking"`: the first call returns `"m_appointment_booking"` (no collision yet, `used` was empty) and records `used["m_appointment_booking"] = "github.com/veltylabs/mjosefa-cms/modules/appointment_booking"`; the second call's first-choice candidate `"m_appointment_booking"` collides with a *different* recorded path, so it walks one segment up to `"m_veltylabs_appointment_booking"`, which is unused, and returns that. Also test the reverse order: process `"github.com/veltylabs/appointment_booking"` FIRST — it returns `"m_appointment_booking"` — then `"github.com/veltylabs/mjosefa-cms/modules/appointment_booking"` returns `"m_modules_appointment_booking"` (one segment up from *its own* last segment, `modules/appointment_booking`). The two orders must produce different pairs — the point of this second half is confirming the shorter alias always goes to whichever path was processed FIRST, never to a fixed one of the two paths by content. |
| `TestAliasFor_SamePathIsNotACollisionWithItself` | calling `aliasFor` twice with the *same* path and a `used` map already containing that path's own alias returns the same alias both times, unchanged |
| `TestAliasFor_ReturnsDifferentAliasesForThreeWayCollision` | three distinct paths all ending in `/foo` (e.g. `a/x/foo`, `b/y/foo`, `c/z/foo`) each get a distinct alias after all three are processed in sequence through the same `used` map |

Then add **one end-to-end regression test in `tests/`**, following
`tests/reach_partial_test.go`'s exact fixture-building pattern (a temp `appDir`
built with `sitec.New(appDir)`, two on-disk packages under different import
paths sharing a last segment, both scannable with an SSR feature) —
`TestExtractAll_AliasCollisionAcrossPackages` — asserting the full pipeline
(not just the unit-level `aliasFor`) produces a generated `main.go` that
compiles, or at minimum that `ExtractAll` (or whichever public entry point
`reach_partial_test.go` calls) returns no error and both packages' features
are present in the aggregated output.

Run `gotest ./...` — everything green, including every pre-existing test
untouched.

### Stage 3 — documentation

- `docs/ARCHITECTURE.md`: if it documents the alias-generation scheme
  anywhere, correct it to describe the collision-aware version. If it does
  not mention alias generation at all, no change is needed here — do not add
  a new section speculatively.
- Do **not** link any permanent document to `docs/PLAN.md` — it is deleted
  when this lands.

## 5. Stages table

| # | Stage | Files | Acceptance |
|---|---|---|---|
| 1 | Fix | `select.go` | `aliasFor` + `aliasByCandidate` map; collision-aware |
| 2 | Tests | `alias_collision_internal_test.go` (new, root), `tests/*_test.go` (new e2e case) | 4 unit cases + 1 e2e case, all green |
| 3 | Docs | `docs/ARCHITECTURE.md` | corrected only if it already documents this scheme |

## 6. Acceptance criteria

- Reproduce the exact bug report's shape as a test case (two paths ending in
  `/appointment_booking`, one nested under `modules/`) and confirm it no
  longer collides.
- `grep -n "func aliasFor" select.go` → present.
- The existing bare-alias behavior (`m_staff_manager` for
  `github.com/veltylabs/staff_manager` alone, no collision) is **unchanged** —
  this is an additive disambiguation, never a renaming of every alias.
- No new `*_test.go` outside `tests/` except the one named, justified
  exception at the repository root.
- `gotest ./...` green.
