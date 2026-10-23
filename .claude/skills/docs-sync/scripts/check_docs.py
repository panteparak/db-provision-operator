#!/usr/bin/env python3
"""Mechanical docs-vs-code drift checks for db-provision-operator.

Run from the repo root:  python3 .claude/skills/docs-sync/scripts/check_docs.py
Exit 1 when any check finds a gap. Each finding is "CHECK: detail".
Only catches names/values that must appear verbatim; prose accuracy is the
skill's manual pass.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path.cwd()
DOCS = ROOT / "docs"


def read(p):
    return (ROOT / p).read_text()


def go_sources(*dirs):
    for d in dirs:
        for p in (ROOT / d).rglob("*.go"):
            if not p.name.endswith("_test.go") and "zz_generated" not in p.name:
                yield p


def docs_text(exclude=("api/reference.md",)):
    return "\n".join(p.read_text() for p in DOCS.rglob("*.md")
                     if str(p.relative_to(DOCS)) not in exclude)


findings = []
dead = []  # constants nothing uses: reported, but not a docs failure
docs = docs_text()
sources = {p: p.read_text() for p in go_sources("api", "internal", "cmd")}
uncommented = [re.sub(r"^\s*//.*$", "", t, flags=re.M) for t in sources.values()]


def constants(pattern):
    """Yield (const_name, value, is_used) for consts matching pattern.
    Used = referenced by name anywhere besides its own definition."""
    for text in sources.values():
        for name, value in re.findall(pattern, text):
            refs = sum(len(re.findall(rf"\b{name}\b", t)) for t in uncommented)
            yield name, value, refs > 1
claude_md = read("CLAUDE.md") if (ROOT / "CLAUDE.md").exists() else ""

# 1. Annotations: every Annotation* constant documented; every annotation
#    named in docs or CLAUDE.md exists in code.
for name, a, used in sorted(constants(r'\b(Annotation\w*)\s*=\s*"(dbops\.dbprovision\.io/[a-z0-9-]+)"')):
    if not used:
        dead.append(f"dead-code annotation: {name} = {a}")
    elif a not in docs:
        findings.append(f"annotation-undocumented: {a}")
all_code = "\n".join(sources.values())
for src, text in (("docs", docs), ("CLAUDE.md", claude_md)):
    # skip kind.group/name references; strip kubectl's trailing "-" (remove syntax)
    found = re.findall(r"(?<![\w.])dbops\.dbprovision\.io/[a-z0-9-]+(?![a-z0-9-]*\.)", text)
    for a in sorted({x.rstrip("-") for x in found}):
        if f'"{a}"' not in all_code:
            findings.append(f"annotation-unknown ({src}): {a}")

# 2. Condition reasons: every reason the code sets appears in docs.
reasons = set()
for name, value, used in sorted(constants(r'\b(Reason\w*)\s*=\s*"(\w+)"')):
    if used:
        reasons.add(value)
    else:
        dead.append(f"dead-code reason: {name} = {value}")
for text in sources.values():  # literal reasons: Set*Condition(&c, status, "Reason", ...)
    reasons |= set(re.findall(r'Set\w*Condition\([^;]*?Condition(?:True|False|Unknown),\s*"(\w+)"', text, re.S))
for r in sorted(reasons):
    if not re.search(rf"\b{r}\b", docs):
        findings.append(f"reason-undocumented: {r}")

# 3. Metrics: every metric name appears in docs/operations/monitoring.md.
metrics_go = read("internal/metrics/metrics.go")
ns = (re.search(r'\bnamespace\s*=\s*"(\w+)"', metrics_go) or [None, ""])[1]
monitoring = read("docs/operations/monitoring.md")
for name in sorted(set(re.findall(r'\bName:\s*"(\w+)"', metrics_go))):
    full = f"{ns}_{name}" if ns else name
    if full not in monitoring:
        findings.append(f"metric-undocumented: {full}")

# 4. Drift fields: every literal Field in the drift service is in the drift guide.
drift_doc = read("docs/user-guide/drift-detection.md")
for p in (ROOT / "internal/service/drift").glob("*.go"):
    if p.name.endswith("_test.go"):
        continue
    for f in set(re.findall(r'Field:\s*"([\w.]+)"', p.read_text())):
        if f"`{f}`" not in drift_doc and f'"{f}"' not in drift_doc:
            findings.append(f"drift-field-undocumented: {f} ({p.name})")

# 5. Version: mkdocs extra.version matches the chart appVersion.
chart = read("charts/db-provision-operator/Chart.yaml")
app_ver = re.search(r'^appVersion:\s*"?([^"\n]+)"?', chart, re.M)[1]
doc_ver = re.search(r"^\s+version:\s*([^\s]+)", read("mkdocs.yml"), re.M)
if doc_ver and doc_ver[1] != app_ver:
    findings.append(f"version-mismatch: mkdocs.yml {doc_ver[1]} != chart appVersion {app_ver}")

# 6. Nav: each docs dir's .pages lists exactly the .md files in it.
for pages in DOCS.rglob(".pages"):
    listed = set(re.findall(r":\s*([\w./-]+\.md)\s*$", pages.read_text(), re.M))
    present = {p.name for p in pages.parent.glob("*.md")}
    for f in sorted(present - listed):
        findings.append(f"nav-missing: {pages.parent.relative_to(ROOT)}/{f} not in .pages")
    for f in sorted(listed - present):
        findings.append(f"nav-dangling: {pages.relative_to(ROOT)} lists missing {f}")

# 7. Make targets referenced in docs/CLAUDE.md exist.
targets = set(re.findall(r"^([\w-]+):", read("Makefile"), re.M))
for src, text in (("docs", docs), ("CLAUDE.md", claude_md)):
    for t in sorted(set(re.findall(r"\bmake ([a-z][\w-]+)", text))):
        if t not in targets:
            findings.append(f"make-target-unknown ({src}): make {t}")

for f in findings + dead:
    print(f)
print(f"\n{len(findings)} finding(s), {len(dead)} dead-code note(s)", file=sys.stderr)
sys.exit(1 if findings else 0)
