#!/usr/bin/env python3
"""Normalize the RFQ backend OpenAPI document for local client generation.

The vendored spec stays byte-for-byte upstream apart from jq formatting. One construct has to be
rewritten before generation: a property whose whole schema is `{"type": "null"}` (zod's `z.null()`,
used for `ApprovalCheckResponse.cancel`). openapi-generator 7.24.0 maps that to the literal Go type
`nil`, which does not compile:

    Cancel nil `json:"cancel"`

An empty schema is the honest equivalent — a value carrying no type information — and generates
`interface{}`, which is what earlier generator versions produced for this property. `anyOf` unions
that merely *include* `{"type": "null"}` are left alone: they are the idiomatic "nullable X" spelling
and the generator already handles them as pointers.
"""

import json
import sys


def normalize(node: object, inside_composite: bool = False) -> object:
    if isinstance(node, list):
        return [normalize(item, inside_composite) for item in node]
    if not isinstance(node, dict):
        return node

    # A bare null-typed schema outside anyOf/oneOf/allOf: drop the type, keeping any annotations.
    if not inside_composite and node.get("type") == "null":
        return {key: value for key, value in node.items() if key != "type"}

    return {
        key: normalize(value, key in ("anyOf", "oneOf", "allOf"))
        for key, value in node.items()
    }


json.dump(normalize(json.load(sys.stdin)), sys.stdout, indent=2)
