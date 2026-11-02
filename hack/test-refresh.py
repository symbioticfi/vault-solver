#!/usr/bin/env python3
"""Exercise refresh failure/success without upstream APIs or generated clients."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent.parent

class RefreshTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)
        shutil.copy(SOURCE / 'Makefile', self.root)
        shutil.copytree(SOURCE / 'hack', self.root / 'hack', ignore=shutil.ignore_patterns('__pycache__'))
        (self.root / 'openapi').mkdir()
        (self.root / 'api/abi').mkdir(parents=True)

    def make(self, target, *variables, env=None):
        return subprocess.run(['make', target, *variables], cwd=self.root, env=env,
                              capture_output=True, text=True, timeout=30)

    def test_openapi_failures_preserve_original(self):
        for target, variable, destination in [
            ('refresh-openapi', 'OPENAPI_URL', '3f-bf.openapi.json'),
            ('refresh-rfq-openapi', 'RFQ_OPENAPI_URL', 'rfq-backend.openapi.json'),
            ('refresh-rfq-internal-openapi', 'RFQ_INTERNAL_OPENAPI_URL', 'rfq-backend-internal.openapi.json'),
            ('refresh-lifi-openapi', 'LIFI_OPENAPI_URL', 'lifi-order.openapi.json'),
            ('refresh-uniswapx-openapi', 'UNISWAPX_OPENAPI_URL', 'uniswapx-service.openapi.json'),
            ('refresh-redstone-guide', 'REDSTONE_GUIDE_URL', 'redstone-oev-atom-integration-guide.md'),
        ]:
            for content in ((None, '') if target == 'refresh-redstone-guide' else (None, '', 'invalid schema')):
                with self.subTest(target=target, content=content):
                    dest = self.root / 'openapi' / destination
                    dest.write_text('known-good\n')
                    fixture = self.root / 'fixture'
                    if fixture.exists(): fixture.unlink()
                    if content is not None: fixture.write_text(content)
                    result = self.make(target, f'{variable}={fixture.as_uri()}')
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertEqual(dest.read_text(), 'known-good\n')

    def test_fetch_failure_after_valid_prefix_preserves_original(self):
        fakebin = self.root / 'fakebin'
        fakebin.mkdir()
        fakecurl = fakebin / 'curl'
        fakecurl.write_text("#!/bin/sh\necho '{\"openapi\":\"3.0.0\",\"paths\":{}}'\nexit 1\n")
        fakecurl.chmod(0o755)
        dest = self.root / 'openapi/3f-bf.openapi.json'
        dest.write_text('known-good\n')
        env = dict(os.environ, PATH=str(fakebin) + os.pathsep + os.environ['PATH'])
        result = self.make('refresh-openapi', 'OPENAPI_URL=invalid://unused', env=env)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(dest.read_text(), 'known-good\n')

    def test_json_success_replaces_valid_spec(self):
        fixture = self.root / 'fixture.json'
        fixture.write_text('{"openapi":"3.0.0","paths":{}}')
        dest = self.root / 'openapi/3f-bf.openapi.json'
        dest.write_text('old')
        result = self.make('refresh-openapi', f'OPENAPI_URL={fixture.as_uri()}')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('3.0.0', dest.read_text())

    def test_abi_extraction_failure_preserves_all_originals(self):
        output = self.root / 'out'
        for name, content in [('First', '{"abi":[{"type":"function","name":"new"}]}'), ('Second', 'invalid')]:
            directory = output / f'{name}.sol'
            directory.mkdir(parents=True)
            (directory / f'{name}.json').write_text(content)
            (self.root / f'api/abi/{name}.json').write_text('known-good\n')
        result = self.make('refresh-abi', 'ABIS=First Second', 'CORE_MIRROR_ABIS=', f'FORGE_OUT={output}')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        for name in ('First', 'Second'):
            self.assertEqual((self.root / f'api/abi/{name}.json').read_text(), 'known-good\n')

    def test_graphql_fetch_failure_preserves_original(self):
        dest = self.root / 'api/graphql/morpho/schema.graphql'
        dest.parent.mkdir(parents=True)
        dest.write_text('known-good\n')
        fakebin = self.root / 'fakebin'
        fakebin.mkdir()
        fakego = fakebin / 'go'
        fakego.write_text('#!/bin/sh\necho invalid\nexit 1\n')
        fakego.chmod(0o755)
        env = dict(os.environ, PATH=str(fakebin) + os.pathsep + os.environ['PATH'])
        result = self.make('refresh-morpho-graphql-schema', env=env)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(dest.read_text(), 'known-good\n')

if __name__ == '__main__':
    unittest.main()
