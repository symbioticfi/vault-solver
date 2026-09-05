#!/usr/bin/env python3
"""Diff every vendored third-party schema against its live source.

Reads hack/schema-sources.json, fetches each `auto` source, and compares it to the
vendored copy. Also fails when a schema file exists in the repo but is not registered
in the manifest, so a newly vendored integration cannot escape drift detection.

Generated clients tolerate added/removed fields (hack/openapi-relax-client.py), so this
check exists for the drift that tolerance cannot absorb: renamed fields, changed enums,
new required request fields, altered semantics in descriptions.

Usage: check-schema-drift.py [--report FILE] [--only NAME]
Exit code is always 0; the outcome is reported as `status=clean|drift` on stdout and,
when running under GitHub Actions, appended to $GITHUB_OUTPUT.
"""

from __future__ import annotations

import argparse
import difflib
import json
import os
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
MANIFEST = ROOT / "hack" / "schema-sources.json"
GQLFETCH = "github.com/suessflorian/gqlfetch/gqlfetch@v0.7.0"
# Vendored-schema locations the manifest must account for.
DISCOVER_GLOBS = ("openapi/*", "api/graphql/*/schema.graphql")
MAX_DIFF_LINES = 200
TIMEOUT = 90


def normalize(text: str, fetch: str) -> str:
    """Canonical form for comparison: JSON is sorted; GraphQL has one final newline."""
    if fetch in ("json", "scalar-html"):
        return json.dumps(json.loads(text), indent=2, sort_keys=True) + "\n"
    if fetch == "graphql":
        return text.rstrip() + "\n"
    return text


def fetch_live(source: dict) -> str:
    fetch, url = source["fetch"], source["url"]
    if fetch == "graphql":
        out = subprocess.run(
            ["go", "run", GQLFETCH, "-endpoint", url],
            cwd=ROOT, capture_output=True, text=True, timeout=TIMEOUT * 4, check=True,
        )
        return out.stdout
    body = subprocess.run(
        ["curl", "-fsSL", "--max-time", str(TIMEOUT), url],
        capture_output=True, text=True, timeout=TIMEOUT + 30, check=True,
    ).stdout
    if fetch == "scalar-html":
        body = subprocess.run(
            [sys.executable, str(ROOT / "hack" / "scalar-openapi-extract.py")],
            input=body, capture_output=True, text=True, timeout=TIMEOUT, check=True,
        ).stdout
    return body


def check(source: dict, report: list[str]) -> bool:
    """Returns True when this source has drifted."""
    name, vendored_rel = source["name"], source["vendored"]
    vendored_path = ROOT / vendored_rel

    if not vendored_path.exists():
        report.append(f"- **{name}**: vendored file `{vendored_rel}` is missing")
        return True

    if source["mode"] != "auto":
        note = source.get("note", "")
        report.append(f"- {name}: manual source, not auto-checked. {note}")
        return False

    try:
        live_raw = fetch_live(source)
    except subprocess.CalledProcessError as err:
        detail = (err.stderr or "").strip().splitlines()[-1:] or ["no stderr"]
        report.append(f"- {name}: could not fetch live source; treating as drift until it can be verified ({detail[0]})")
        return True
    except subprocess.TimeoutExpired:
        report.append(f"- {name}: fetching live source timed out; treating as drift until it can be verified")
        return True

    try:
        live = normalize(live_raw, source["fetch"])
        current = normalize(vendored_path.read_text(), source["fetch"])
    except json.JSONDecodeError as err:
        report.append(f"- {name}: live source is not valid JSON; treating as drift until it can be verified ({err})")
        return True

    if live == current:
        report.append(f"- {name}: up to date")
        return False

    diff = list(difflib.unified_diff(
        current.splitlines(), live.splitlines(),
        fromfile=f"vendored/{vendored_rel}", tofile="live", lineterm="",
    ))
    shown = diff[:MAX_DIFF_LINES]
    truncated = len(diff) - len(shown)
    report += [
        f"- **{name}**: DRIFTED (`{vendored_rel}`)",
        "",
        f"  Refresh: `{source['refresh']}`",
        "",
        "  <details><summary>diff (vendored → live)</summary>",
        "",
        "  ```diff",
        *(f"  {line}" for line in shown),
        *([f"  ... {truncated} more diff lines"] if truncated > 0 else []),
        "  ```",
        "",
        "  </details>",
        "",
    ]
    return True


def check_coverage(sources: list[dict], report: list[str]) -> bool:
    registered = {s["vendored"] for s in sources}
    found: set[str] = set()
    for pattern in DISCOVER_GLOBS:
        for path in ROOT.glob(pattern):
            if path.is_file():
                found.add(path.relative_to(ROOT).as_posix())
    unregistered = sorted(found - registered)
    if unregistered:
        report.append("")
        report.append("- **Unregistered vendored schemas** (add them to `hack/schema-sources.json`):")
        report += [f"  - `{path}`" for path in unregistered]
        return True
    missing = sorted(registered - found)
    if missing:
        report.append("")
        report.append(f"- Manifest lists files not found on disk: {', '.join(missing)}")
        return True
    return False


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", default="drift-report.md")
    parser.add_argument("--only", help="check a single source by (substring of) name")
    args = parser.parse_args()

    sources = json.loads(MANIFEST.read_text())["sources"]
    selected = [s for s in sources if not args.only or args.only.lower() in s["name"].lower()]

    report: list[str] = []
    drifted = [check(s, report) for s in selected]
    coverage_drift = check_coverage(sources, report) if not args.only else False

    status = "drift" if (any(drifted) or coverage_drift) else "clean"
    body = "\n".join(report) + "\n"
    pathlib.Path(args.report).write_text(body)
    print(body)
    print(f"status={status}")
    if out := os.environ.get("GITHUB_OUTPUT"):
        with open(out, "a") as fh:
            fh.write(f"status={status}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
