#!/usr/bin/env python3
"""Exercise the offline CI entrypoint with isolated schema registries."""
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent


class CoverageTests(unittest.TestCase):
    def test_offline_entrypoint(self):
        for mode, expected in [('registered', 0), ('unregistered', 1), ('missing', 1)]:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / 'hack').mkdir()
                (root / 'openapi').mkdir()
                shutil.copy(SOURCE / 'check-schema-drift.py', root / 'hack')
                # This URL must never be fetched by offline CI.
                sources = [{'name': 'test', 'vendored': 'openapi/test.json', 'mode': 'auto', 'fetch': 'json', 'url': 'invalid://no-network'}]
                (root / 'hack/schema-sources.json').write_text(json.dumps({'sources': sources}))
                if mode != 'missing':
                    (root / 'openapi/test.json').write_text('{}')
                if mode == 'unregistered':
                    (root / 'openapi/extra.json').write_text('{}')
                result = subprocess.run([sys.executable, 'hack/check-schema-drift.py', '--check-coverage'],
                                        cwd=root, capture_output=True, text=True, timeout=5)
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                self.assertNotIn('could not fetch', result.stdout)


if __name__ == '__main__':
    unittest.main()
