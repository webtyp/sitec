---
PLAN: "fix: ExtractAll returns an empty result, not an error, when no module declares assets"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 10836106339168970717
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — an empty extraction is a result, not an error

## Why

`(*Extractor).ExtractAll()` returns an error when no module in the project
contributes styles, scripts, pages or icons:

```
sitec: no module produced assets; the stylesheet would come out empty
```

That is a normal state for a new or minimal project (a `web/client.go` that
imports nothing that declares CSS yet). But the dev daemon
(`webtyp.com/app`, `ssr_loader.go`) retries every `ExtractAll` error five
times with backoff and then logs:

```
SSR ExtractAll attempt failed (retrying): sitec: no module produced assets; … (repeated 4 times)
FATAL: SSR ExtractAll failed permanently after 5 attempts. Error: sitec: no module produced assets; …
```

A brand-new project introduces itself with a FATAL. The caller cannot tell
this case apart from a real failure without matching the error text, and the
text is translated at runtime (`lang.Translate`), so matching it is not an
option.

The daemon has already been changed to treat an **empty result with a nil
error** as a one-time notice (`SSR: no module declares styles or pages yet —
the site is served without a stylesheet`), no FATAL, no retry. This plan makes
`sitec` return exactly that.

## Design gate

- **Prior art.** `filepath.Glob` returns `nil, nil` when nothing matches;
  `os.ReadDir` on an empty directory returns an empty slice, not an error.
  "Found nothing" is a result. This repo already follows that rule one level
  down: `ExtractModule` returns `nil, nil` for a module with no SSR files
  (`tests/extract_test.go`, `TestExtractModule_NoSSRFiles`).
- **Novice-name test.** Unchanged name and signature:
  `ExtractAll() ([]*Assets, error)`. A reader expects "extract all → maybe
  none", and an error only when extraction itself failed.
- **Complexity ledger.** −1 error path, −1 message function. +0 concepts.
- **Where it belongs.** The decision "is an empty site acceptable?" belongs to
  each caller: the dev daemon accepts it (nothing written yet), a release
  build does not (see below). The extractor only reports what it found.
- **What it deletes.** `msgNoAssetsExtracted()` in `emit_core.go` and the
  `if len(all) == 0 { return nil, fmt.Err(msgNoAssetsExtracted()) }` block at
  the end of `ExtractAll` in `pipeline.go`.

**What does NOT change:** a release build of an empty project is still an
error. `buildPipeline` in `build.go` already checks `len(all) == 0` right after
calling `ExtractAll` and returns `fmt.Err(msgEmptyExtraction())`. That check
stays exactly as it is. The existing test
`tests/build_consumer_test.go` → subtest `"empty extraction invariant error"`
must keep passing unmodified, because the error now comes from that check.

## Repo rules (read before editing)

- **Every test lives in `tests/`** (package `sitec_test`). Never create a
  `*_test.go` outside `tests/`.
- Messages are built with `lang.Translate(...)` from `webtyp.com/fmt/lang`, and
  errors with `fmt.Err(...)` from `webtyp.com/fmt`. Keep that style; do not
  introduce `errors` or `fmt` from the standard library in files that do not
  already use them.
- No hardcoded strings in logic: messages stay in their `msg…()` functions.

## Stage 1 — `pipeline.go`: return what was found

At the end of `func (e *Extractor) ExtractAll()`, delete:

```go
	if len(all) == 0 {
		return nil, fmt.Err(msgNoAssetsExtracted())
	}
```

so the function ends with `return all, nil`. Update the doc comment of
`ExtractAll` (if it mentions an error for the empty case) to say:
`An empty result with a nil error means no module declares assets; the caller
decides whether that is acceptable.`

If, after the deletion, `fmt` is no longer used in `pipeline.go`, remove the
import (the compiler will tell you).

## Stage 2 — `emit_core.go`: delete the message, keep dev loading quiet

1. Delete the function `msgNoAssetsExtracted()`.
   Acceptance: `grep -rn "msgNoAssetsExtracted" .` → empty.
2. In `(*Compiler).LoadSSRModules`'s goroutine (the block that calls
   `extractor.ExtractAll()` and then `c.RouteExtractedAssets(all)`), add, right
   after the `if err != nil { … return }` block:

   ```go
   		if len(all) == 0 {
   			return // nothing declared yet; the next load retries
   		}
   ```

   so an empty project routes nothing and logs nothing.

Do NOT touch `msgEmptyExtraction()` or its use in `build.go`.

## Stage 3 — `tests/extract_test.go`: the new contract

Replace `TestExtractAll_Empty` with:

```go
// A project whose modules declare no assets is a normal state (a new or
// minimal project): ExtractAll reports "nothing found", not a failure.
func TestExtractAll_EmptyIsNotAnError(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\ngo 1.24\n"), 0644)
	e := sitec.New(root)
	f := modfind.New()
	f.Seed(root, []modfind.Module{{Path: "example.com/demo", Dir: root}})
	e.SetFinder(f)

	all, err := e.ExtractAll()
	if err != nil {
		t.Fatalf("an empty extraction must not be an error, got: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected no assets, got %d", len(all))
	}
}
```

Leave `TestExtractModule_NoSSRFiles` and every other test unchanged. Remove
the `strings` import from `extract_test.go` only if nothing else in the file
uses it.

## Stage 4 — verify

- `go test ./...` passes, including
  `tests/build_consumer_test.go` → `"empty extraction invariant error"`
  (release builds of an empty project still fail).
- `grep -rn "msgNoAssetsExtracted\|no module produced assets" --include=*.go .`
  → empty.

## Stages

| Stage | Files | Done when |
|---|---|---|
| 1 | `pipeline.go` | `ExtractAll` ends with `return all, nil`; no empty-case error |
| 2 | `emit_core.go` | `msgNoAssetsExtracted` deleted; `LoadSSRModules` returns early on an empty result |
| 3 | `tests/extract_test.go` | `TestExtractAll_EmptyIsNotAnError` replaces `TestExtractAll_Empty` and passes |
| 4 | — | full suite green; both greps empty |
