#!/usr/bin/env python3
"""Vendor canonical Foundry artifacts plus the external-world fixture. No network."""
import argparse
import json
import pathlib
import subprocess

ROOT = pathlib.Path(__file__).resolve().parent
SOURCES = {
    'rfq': 'de6dbd617bf3c1e8b4120419cbe5800efae7d410',
    'adapters': '9b693f34f6819f6c4e09f3751d84c2d1098a92c1',
}
p = argparse.ArgumentParser()
p.add_argument('--rfq-root', type=pathlib.Path, required=True)
p.add_argument('--adapters-root', type=pathlib.Path, required=True)
p.add_argument('--solc', required=True)
a = p.parse_args()
for name, revision in SOURCES.items():
    root = getattr(a, name + '_root')
    head = subprocess.check_output(['git', '-C', str(root), 'rev-parse', 'HEAD'], text=True).strip()
    if head != revision:
        raise SystemExit(f'{name}: expected pinned revision {revision}, got {head}')
artifacts = {}
for name, origin in [('Reactor', 'rfq'), ('Executor', 'rfq'), ('RecipeExecutor', 'adapters'), ('MerkleAccount', 'adapters'), ('LiquidLaneConnector', 'adapters')]:
    raw = json.loads((getattr(a, origin + '_root') / 'out' / (name + '.sol') / (name + '.json')).read_text())
    if raw['bytecode'].get('linkReferences'):
        raise SystemExit(f'{name}: unresolved library links')
    metadata = raw['metadata']
    provenance = {'compiler': metadata['compiler'], 'settings': metadata['settings'], 'sources': {path: value['keccak256'] for path, value in metadata['sources'].items()}}
    provenance['settings'].pop('outputSelection', None)
    artifacts[name] = {'abi': raw['abi'], 'bytecode': raw['bytecode']['object'], 'metadata': provenance}
fixture = subprocess.run([a.solc, '--combined-json', 'abi,bin', '--optimize', str(ROOT / 'Fixture.sol')], check=True, capture_output=True, text=True)
for name, raw in json.loads(fixture.stdout)['contracts'].items():
    artifacts[name.split(':')[-1]] = {'abi': raw['abi'], 'bytecode': '0x' + raw['bin']}
(ROOT / 'artifacts.json').write_text(json.dumps({'revisions': SOURCES, 'contracts': artifacts}, indent=2) + '\n')
