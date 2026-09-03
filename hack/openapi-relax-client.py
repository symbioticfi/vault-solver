#!/usr/bin/env python3
"""Strip the required-property decode checks from a generated OpenAPI Go client.

Integration APIs (3F, LI.FI, UniswapX, RFQ backend) evolve without warning. Two of
the three drift directions are handled natively by the generator, via
OPENAPI_TOLERANT_PROPS in the Makefile:

  - fields ADDED upstream    -> disallowAdditionalPropertiesIfNotPresent=false
  - enum values ADDED upstream -> enumUnknownDefaultCase=true

The third has no generator flag: openapi-generator always emits a `requiredProperties`
loop that fails the whole decode with "no value given for required property X" when the
server DROPS a field. 3F removing `cadence` from ResolvedSettlementDto blanked the entire
auction feed that way (VAULT-SOLVER-B), even though nothing reads that field. This script
removes those loops after generation.

Field types are left alone, so required fields keep their non-pointer Go types and simply
zero-value when absent; callers already guard with the generated Get*Ok() accessors.

oneOf/anyOf variants are deliberately left strict: the generator discriminates
between them by trying each variant and seeing which one fails to decode, so
relaxing a variant makes the wrong branch match. Variant types, and every type
reachable from them, are skipped.

Idempotent: safe to re-run over an already-relaxed tree.
"""

import collections
import pathlib
import re
import sys

REQUIRED_BLOCK = re.compile(
    r"\n\t// This validates that all required properties are included in the JSON object"
    r".*?"
    r"\n\tfor _, requiredProperty := range requiredProperties \{"
    r"\n\t\tif _, exists := allProperties\[requiredProperty\]; !exists \{"
    r"\n\t\t\treturn fmt\.Errorf\(\"no value given for required property %v\", requiredProperty\)"
    r"\n\t\t\}"
    r"\n\t\}\n",
    re.DOTALL,
)


# The generator's oneOf/anyOf wrapper ends its UnmarshalJSON with this error.
COMPOSITE_MARKER = re.compile(r"failed to match schemas in (anyOf|oneOf)\(")
TYPE_DECL = re.compile(r"^type (\w+) struct \{(.*?)^\}", re.MULTILINE | re.DOTALL)
FIELD_TYPE = re.compile(r"^\t\w+ +\[?\]?\*?(\w+)", re.MULTILINE)


def strict_types(files: list[pathlib.Path]) -> set[str]:
    """Types used for oneOf/anyOf discrimination, plus everything they reach."""
    decls: dict[str, str] = {}
    seeds: set[str] = set()
    for path in files:
        text = path.read_text()
        composite = bool(COMPOSITE_MARKER.search(text))
        for name, body in TYPE_DECL.findall(text):
            decls[name] = body
            if composite:
                seeds.update(FIELD_TYPE.findall(body))
    strict: set[str] = set()
    queue = collections.deque(seeds)
    while queue:
        name = queue.popleft()
        if name in strict or name not in decls:
            continue
        strict.add(name)
        queue.extend(FIELD_TYPE.findall(decls[name]))
    return strict


def declares_strict_type(text: str, strict: set[str]) -> bool:
    return any(name in strict for name, _ in TYPE_DECL.findall(text))


def prune_unused_imports(text: str) -> str:
    """Drop imports the removed validation blocks orphaned (e.g. fmt)."""
    for pkg, token in (("fmt", "fmt."), ("bytes", "bytes."), ("encoding/json", "json.")):
        line = f'\t"{pkg}"\n'
        # The qualifier ("fmt.") never appears in the import line itself, so a
        # whole-file check is enough to prove the package is unreferenced.
        if line in text and token not in text:
            text = text.replace(line, "", 1)
    return text


def relax(text: str) -> tuple[str, int, int]:
    text, dropped_required = REQUIRED_BLOCK.subn(
        "\n\t// Required-property validation removed by hack/openapi-relax-client.py:\n"
        "\t// upstream may drop fields at any time; absent values zero-value instead of\n"
        "\t// failing the whole decode.\n",
        text,
    )
    text, dropped_unknown = re.subn(
        r"[ \t]*decoder\.DisallowUnknownFields\(\)\n",
        "\t// Unknown fields tolerated (hack/openapi-relax-client.py): upstream may add\n"
        "\t// fields at any time without breaking us.\n",
        text,
    )
    text = prune_unused_imports(text)
    return text, dropped_required, dropped_unknown


def main(target: str) -> int:
    root = pathlib.Path(target)
    files = sorted(root.glob("*.go"))
    if not files:
        print(f"openapi-relax-client: no .go files under {root}", file=sys.stderr)
        return 1
    strict = strict_types(files)
    total_required = total_unknown = touched = skipped = 0
    for path in files:
        original = path.read_text()
        if declares_strict_type(original, strict):
            skipped += 1
            continue
        relaxed, n_req, n_unk = relax(original)
        if relaxed != original:
            path.write_text(relaxed)
            touched += 1
            total_required += n_req
            total_unknown += n_unk
    print(
        f"openapi-relax-client: {root}: relaxed {touched} file(s) "
        f"({total_required} required-property checks, {total_unknown} residual DisallowUnknownFields; "
        f"{skipped} kept strict for oneOf/anyOf discrimination)"
    )
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("usage: openapi-relax-client.py <generated-client-dir>", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(main(sys.argv[1]))
