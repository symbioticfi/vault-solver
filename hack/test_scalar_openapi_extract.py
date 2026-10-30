"""Ensure LI.FI's clock examples do not hide contract or unrelated example changes."""

import copy
import json
import pathlib
import subprocess
import sys
import unittest


class ScalarOpenAPITest(unittest.TestCase):
    def test_normalizes_only_quote_example_integer_expiries(self):
        quotes = [{"expiry": 1900000000, "fromChain": "1"}, {"expiry": "invalid"}, {"expiry": True}]
        content = {"application/json": {"examples": {"clock": {"value": {"quotes": quotes}}}}}
        spec = {
            "openapi": "3.1.0",
            "components": {"schemas": {"Quote": {
                "properties": {"expiry": {"type": "integer", "maximum": 4102444800}},
                "required": ["expiry"],
            }}},
            "paths": {"/quotes/submit": {"post": {"requestBody": {"content": content}}}},
            "example": {"expiry": 1819119550},
        }
        expected = copy.deepcopy(spec)

        for timestamp in (1819119550, 1819119569):
            with self.subTest(timestamp=timestamp):
                quotes[0]["expiry"] = timestamp
                result = subprocess.run(
                    [sys.executable, str(pathlib.Path(__file__).with_name("scalar-openapi-extract.py"))],
                    input='<script>{"content":' + json.dumps(spec) + '}</script>',
                    text=True, capture_output=True, check=True,
                )
                self.assertEqual(json.loads(result.stdout), expected)


if __name__ == "__main__":
    unittest.main()
