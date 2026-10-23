---
name: operator-diagrams
description: Creates and updates diagrams of db-provision-operator grounded in its code — CRD dependency graph, reconcile loop, deletion/force-delete confirmation lifecycle, drift detect/correct flow, password rotation, backup/restore pipeline, adapter/engine architecture. Produces inline Mermaid for the mkdocs site and, when archify is installed, explorable HTML diagrams. Use when the user asks to draw, diagram, visualize, or graph the operator's architecture, a controller flow, a resource lifecycle or state machine, or to refresh stale diagrams in docs/.
---

# Operator Diagrams

Every box and arrow must trace to code. Read the source files listed below
before drawing; never draw from docs alone (they may be stale — see the
`docs-sync` skill).

## Pick the output

| Output | When | Where |
|---|---|---|
| **Mermaid** (default) | Diagram lives in the docs site | Inline ` ```mermaid ` fence in the relevant `docs/**/*.md` (mkdocs superfences renders it). |
| **Archify HTML** | Explorable/presentation diagram, too dense for Mermaid, or user asks for archify / interactive / export | Spec: `docs/diagrams/<name>.<type>.json`; output: `docs/assets/diagrams/<name>.html`, linked from the doc page. |

Archify is a user-level skill (`~/.claude/skills/archify`). If it's missing,
fall back to Mermaid and say so. When using it, load the `archify` skill and
follow its authoring/validate/deliver workflow exactly — run its CLI from its
own directory with absolute paths to the spec and output in this repo. Commit
the JSON spec next to the HTML so the diagram can be regenerated.

## Canonical diagrams

| Diagram | Archify type / Mermaid | Source of truth | Lives in |
|---|---|---|---|
| CRD dependency graph | `architecture` / `graph TD` | `CLAUDE.md` graph, `instanceRef`/`userRef`/`roleRef`/`databaseRef` in `api/v1alpha1/*_types.go` | `docs/architecture/overview.md` |
| Component architecture (controllers → handler → repository → adapter → engine) | `architecture` / `graph TD` | `internal/features/*/`, `internal/service/`, `internal/adapter/<engine>/` | `docs/architecture/overview.md` |
| Reconcile loop | `workflow` / `flowchart` | `Reconcile()` in `internal/features/<crd>/controller.go` | `docs/architecture/design.md` |
| Deletion + force-delete confirmation | `lifecycle` / `stateDiagram-v2` | `handleDeletion()`, `internal/util/deletion_confirmation.go`, `PhasePendingDeletion` | `docs/user-guide/deletion-protection.md` |
| Drift detect → correct | `workflow` / `flowchart` | `internal/controller/drift/orchestrator.go`, `internal/service/drift/` | `docs/user-guide/drift-detection.md` |
| Password rotation (role-inheritance) | `sequence` / `sequenceDiagram` | `handleRotation()` in `internal/features/user/controller.go`, `RotateWithStrategy()` / `CleanupDeprecatedUsers()` | `docs/user-guide/users.md` |
| Backup / restore | `dataflow` / `flowchart LR` | `internal/features/backup*/`, `internal/features/restore/`, `internal/storage/` | `docs/user-guide/backups.md` |
| Resource phases | `lifecycle` / `stateDiagram-v2` | `Phase*` constants in `api/v1alpha1/common_types.go` | per-CRD user-guide page |

## Workflow

1. Identify the diagram (table above, or a new one — add it to the table).
2. Read its source files; list the real nodes/states/transitions with
   `file:line` evidence before drawing.
3. Grep docs for an existing version (`grep -rn '```mermaid' docs`) — update it
   in place rather than adding a second copy.
4. Draw. Keep one main path, ≤12 primary nodes; split into two diagrams rather
   than cramming. Use real names (CRD kinds, phase/condition names, function
   names) so readers can grep them.
5. Verify: Mermaid → `make docs-build` (strict) and eyeball with
   `make docs-serve`; Archify → its `deliver` command must exit 0.
6. Report what changed and the evidence for any edge that isn't obvious.

## Mermaid conventions

- `graph TD` for structure, `flowchart` for processes, `stateDiagram-v2` for
  phases, `sequenceDiagram` for multi-actor flows (controller ↔ DB ↔ Secret).
- Label decision edges with the condition (`-->|children exist|`).
- Annotations and condition reasons in edge labels use exact strings
  (`force-delete`, `ReasonDependenciesExist`).
