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


def main() -> None:
    spec = extract(sys.stdin.read())
    if "openapi" not in spec:
        raise SystemExit("scalar-openapi-extract: extracted object is not an OpenAPI document")
    json.dump(spec, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
