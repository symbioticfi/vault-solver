#!/usr/bin/env python3
"""Normalize the upstream UniswapX OpenAPI document for local client generation.

The vendored swagger.json stays byte-for-byte upstream apart from jq formatting. Chain 31337 is added
only to the generated client's enum so the same strict decoder can exercise the local integration stack.
The order response gets a discriminator over its existing type field so openapi-generator can select the
documented oneOf variant without its incompatible validator.v2 fallback.
"""

import json
import sys


document = json.load(sys.stdin)

chain_id = document["components"]["schemas"]["ChainId"]
if 31337 not in chain_id["enum"]:
    chain_id["enum"].append(31337)

orders = document["components"]["schemas"]["GetOrdersResponse"]["properties"]["orders"]["items"]
orders["discriminator"] = {
    "propertyName": "type",
    "mapping": {
        "Dutch": "#/components/schemas/DutchOrderEntity",
        "DutchLimit": "#/components/schemas/DutchOrderEntity",
        "Limit": "#/components/schemas/DutchOrderEntity",
        "Dutch_V2": "#/components/schemas/DutchV2OrderEntity",
        "Dutch_V3": "#/components/schemas/DutchV3OrderEntity",
        "Priority": "#/components/schemas/PriorityOrderEntity",
        "Hybrid": "#/components/schemas/HybridOrderEntity",
        "Relay": "#/components/schemas/RelayOrderEntity",
    },
}

json.dump(document, sys.stdout, indent=2)
sys.stdout.write("\n")
