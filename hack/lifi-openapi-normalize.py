#!/usr/bin/env python3
# Normalize the LI.FI order-server OpenAPI spec so the Java openapi-generator emits compiling Go.
#
# The vendored openapi/lifi-order.openapi.json is the RAW contract of record (pulled verbatim from the
# order server's Scalar /docs page) and MUST stay unedited. Two upstream defects in that raw spec make the
# generated Go uncompilable, so `make refresh-lifi-client` pipes the raw spec through this shim first. This
# reads the raw spec on stdin and writes the normalized spec to stdout, applying ONLY these two
# deterministic fixes:
#
#   (a) Dangling oneOf $refs. QuoteDto.order is `oneOf: [Oif3009OrderDto, OifEscrowOrderDto,
#       OifUserOpenIntentOrderDto]`, but none of those three schemas are defined in components.schemas
#       (upstream forgot to register the NestJS DTOs — likely a missing @ApiExtraModels). The generator
#       then emits a oneOf wrapper referencing three undefined Go types. Any property/schema whose value is
#       a `$ref` (or a oneOf/anyOf/allOf of $refs) pointing at a missing schema is replaced with a
#       permissive `{"type": "object", "additionalProperties": true}` passthrough — the solver flow does
#       not need the nested order type inside the quote object.
#
#   (b) Multi-tag operations. `/quote/request` is tagged ["Quotes","Bridge API"] and `/quotes/submit`
#       ["Quotes","Solver API"]. openapi-generator emits each operation's request struct into EVERY tag's
#       api_<tag>.go file, so a multi-tagged operation yields duplicate package-level types. Each
#       operation's `tags` is collapsed to a single entry: prefer a tag ending in "API" (the concrete
#       surface, e.g. "Bridge API"), else the first tag.
import json
import sys


def _is_ref_to_missing(node: dict, defined: set) -> bool:
    ref = node.get("$ref")
    return isinstance(ref, str) and ref.startswith("#/components/schemas/") and ref.rsplit("/", 1)[-1] not in defined


def _has_dangling_ref(node, defined: set) -> bool:
    # A schema node is "dangling" if it is (or is composed via oneOf/anyOf/allOf of) a $ref whose target
    # is not defined in components.schemas.
    if not isinstance(node, dict):
        return False
    if _is_ref_to_missing(node, defined):
        return True
    for kw in ("oneOf", "anyOf", "allOf"):
        members = node.get(kw)
        if isinstance(members, list) and any(_is_ref_to_missing(m, defined) for m in members if isinstance(m, dict)):
            return True
    return False


PASSTHROUGH = {"type": "object", "additionalProperties": True}


def _fix_dangling(node, defined: set):
    # Recursively replace any schema node that points at a missing $ref with a permissive passthrough,
    # preserving the node's own description/example if present.
    if isinstance(node, list):
        return [_fix_dangling(v, defined) for v in node]
    if not isinstance(node, dict):
        return node
    if _has_dangling_ref(node, defined):
        out = dict(PASSTHROUGH)
        for keep in ("description", "example", "title"):
            if keep in node:
                out[keep] = node[keep]
        return out
    return {k: _fix_dangling(v, defined) for k, v in node.items()}


def _collapse_tags(spec: dict) -> None:
    methods = {"get", "put", "post", "delete", "patch", "options", "head", "trace"}
    for path_item in spec.get("paths", {}).values():
        if not isinstance(path_item, dict):
            continue
        for method, op in path_item.items():
            if method.lower() not in methods or not isinstance(op, dict):
                continue
            tags = op.get("tags")
            if isinstance(tags, list) and len(tags) > 1:
                api_tags = [t for t in tags if isinstance(t, str) and t.strip().endswith("API")]
                op["tags"] = [api_tags[0] if api_tags else tags[0]]


def main() -> None:
    spec = json.load(sys.stdin)
    defined = set(spec.get("components", {}).get("schemas", {}).keys())
    if "components" in spec and "schemas" in spec["components"]:
        spec["components"]["schemas"] = _fix_dangling(spec["components"]["schemas"], defined)
    if "paths" in spec:
        spec["paths"] = _fix_dangling(spec["paths"], defined)
    _collapse_tags(spec)
    json.dump(spec, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
