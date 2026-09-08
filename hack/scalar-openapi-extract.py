#!/usr/bin/env python3
# The LI.FI order server (NestJS + Scalar UI) has no raw OpenAPI JSON endpoint: every /docs/* path
# serves the Scalar HTML, which embeds the full spec inline in the `Scalar.createApiReference(..., { ...
# "content": { <openapi> } ... })` config. This reads that HTML on stdin, brace-matches the "content"
# object, and writes the pretty-printed OpenAPI JSON to stdout. Used by `make refresh-lifi-openapi`.
import json
import sys


def extract(html: str) -> dict:
    key = '"content":'
    idx = html.find(key)
    if idx == -1:
        raise SystemExit("scalar-openapi-extract: no embedded \"content\" object found in HTML")
    start = html.index("{", idx)
    depth = 0
    in_str = False
    esc = False
    for i in range(start, len(html)):
        c = html[i]
        if in_str:
            if esc:
                esc = False
            elif c == "\\":
                esc = True
            elif c == '"':
                in_str = False
        elif c == '"':
            in_str = True
        elif c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return json.loads(html[start : i + 1])
    raise SystemExit("scalar-openapi-extract: unbalanced braces extracting \"content\" object")


def drop_dynamic_examples(value: object) -> None:
    """Remove server-clock examples that otherwise make every refresh look like schema drift."""
    if isinstance(value, dict):
        properties = value.get("properties")
        if isinstance(properties, dict):
            min_valid_until = properties.get("minValidUntil")
            if isinstance(min_valid_until, dict):
                min_valid_until.pop("example", None)
        for child in value.values():
            drop_dynamic_examples(child)
    elif isinstance(value, list):
        for child in value:
            drop_dynamic_examples(child)


def main() -> None:
    spec = extract(sys.stdin.read())
    if "openapi" not in spec:
        raise SystemExit("scalar-openapi-extract: extracted object is not an OpenAPI document")
    drop_dynamic_examples(spec)
    json.dump(spec, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
