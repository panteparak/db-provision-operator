---
name: docs-sync
description: Audits and syncs db-provision-operator documentation (docs/ mkdocs site, CLAUDE.md, Helm chart docs) against the Go code — annotations, condition reasons, metrics, drift fields, make targets, version, nav, generated API reference. Use when the user asks to audit docs, check docs vs code, find stale/outdated docs, sync docs after a code change, or before a release; also after changing CRD types, annotations, conditions, metrics, drift detection, or Makefile targets.
---

# Docs Sync

Code is the source of truth. Docs follow code, never the reverse — if docs
describe behavior the code lacks, either fix the docs or flag a code bug to the
user; don't change code to match docs silently.

## Quick start

```bash
python3 .claude/skills/docs-sync/scripts/check_docs.py   # exit 1 = findings
```

Each line is `check: detail`; only findings set the exit code, `dead-code` notes don't. Fix, re-run until clean (or every remaining
finding is consciously accepted and listed in your report).

## Workflow

1. **Mechanical pass** — run the script. Triage each finding:

   | Finding | Usual fix |
   |---|---|
   | `annotation-undocumented` | Add to the annotation table in `CLAUDE.md` and the relevant `docs/user-guide/*.md`. If the constant is unused (`grep -rn <ConstName> internal api`), report it as dead code instead. |
   | `annotation-unknown` | Docs name an annotation the code doesn't define — remove or rename to the real constant. |
   | `reason-undocumented` | Add to `docs/operations/troubleshooting.md` (what it means, what to do). |
   | `metric-undocumented` | Add to `docs/operations/monitoring.md` with type, labels (from `internal/metrics/metrics.go`) and meaning. |
   | `drift-field-undocumented` | Add a row to the per-field table in `docs/user-guide/drift-detection.md` with its classification (`Destructive`/`Immutable` from `internal/service/drift/*.go`). |
   | `version-mismatch` | `mkdocs.yml` `extra.version` follows `charts/db-provision-operator/Chart.yaml` `appVersion`. |
   | `nav-missing` / `nav-dangling` | Fix the directory's `.pages` (awesome-pages plugin). |
   | `make-target-unknown` | Replace with the real target (`grep -n '^[a-z-]*:' Makefile`). |
   | `dead-code …` (note, doesn't fail) | Constant nothing uses. If docs describe it as working, that's a code bug — flag it and mark the doc "not implemented"; otherwise list it for removal. Don't document dead reasons. |

2. **Semantic pass** — the script can't judge prose. For each area touched by
   the change (or all of them for a full audit), read code and doc side by side:

   | Doc | Source of truth |
   |---|---|
   | `docs/api/reference.md` | Generated — `make generate-api-docs`, never hand-edit. Stale if `git diff` shows changes after regenerating. |
   | `docs/user-guide/<crd>.md` | `api/v1alpha1/<crd>_types.go` (fields, defaults `+kubebuilder:default`, enums, validation) and `internal/features/<crd>/` (behavior). |
   | `docs/user-guide/deletion-protection.md`, CLAUDE.md deletion section | `handleDeletion()` in each controller, `internal/util/deletion_confirmation.go`. |
   | `docs/user-guide/drift-detection.md` | `internal/service/drift/`, `internal/controller/drift/orchestrator.go`, `api/v1alpha1/driftpolicy_types.go`. |
   | `docs/engines/<engine>.md` | `internal/adapter/<engine>/` — what is actually implemented vs no-op. |
   | `docs/development/rbac.md` | `config/rbac/role.yaml` (from `make manifests`). |
   | YAML examples in `docs/examples/`, `docs/index.md` | Every field must exist in the CRD types; spot-check with `grep -n 'json:"<field>' api/v1alpha1/*.go`. |

   Check claims of the form "X happens when Y": defaults, which fields are
   mutable, what each annotation does, which engines support a feature.

3. **Verify** — `make docs-build` (mkdocs `--strict`, catches broken links and
   nav). Re-run the script.

4. **Report** — list what was fixed, and separately anything that looks like a
   code bug (docs describe sane behavior the code lacks) for the user to decide.

## Rules

- Keep doc changes in the same commit as the code change when syncing after a
  feature; a standalone audit gets its own `docs:` commit.
- Don't document internals users can't observe (private helpers, test utils).
- Per-engine differences go in mkdocs tabs (`=== "PostgreSQL"`), not prose
  caveats scattered across pages.
- Extending the script: add a numbered check that compares a code literal set
  against a doc text set; keep it stdlib-only.
