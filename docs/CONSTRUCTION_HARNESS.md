# The construction harness — typed, explicit, hard to get wrong

> **This was a copy. The harness is one single document now:**
> **[webtyp/devskills → skills/api-design/SKILL.md](https://github.com/webtyp/devskills/blob/main/skills/api-design/SKILL.md)**
> (locally, after running the `devskills` CLI: `~/skills/api-design/SKILL.md`).

**The typed, explicit code is itself the harness.** The code is largely written
by an agent that does not know the library; that agent must produce correct code
guided only by the method signatures, and the compiler must **reject** what is
wrong. A manual pushes correctness onto the reader; a harness pushes it onto the
compiler — the right path is the only one that exists, and the wrong one does not
compile.

The rules this library follows — typed over `any`, illegal states
unrepresentable, closed by default, one intent one path, lego pieces never
forks, fix at the root never at the leaf, no `internal/`, and the
consumer-shaped test that must prove an API before it is published — are all in
that skill, together with the five gate answers every `docs/PLAN.md` must carry
before a public API is changed. This page is a pointer so the two never drift.
