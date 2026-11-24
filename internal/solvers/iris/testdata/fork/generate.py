#!/usr/bin/env python3
"""Build the fork-test recipes with the adapters BuildRecipe script against a mainnet fork."""
import argparse
import json
import os
import pathlib
import shutil
import socket
import subprocess
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent
ADAPTERS_REVISION = '0ad09b13b4bdde821f5a57e00b4c8d1d754adba7'  # adapters feat/iris-signing (PR #54)
ACCOUNT = '0x0000000000000000000000000000000000ACC001'  # Must match forkAccount in fork_test.go.
ADAPTER = '0x0000000000000000000000000000000000000103'

p = argparse.ArgumentParser()
p.add_argument('--adapters-root', type=pathlib.Path, required=True)
p.add_argument('--rpc', default=os.environ.get('ETH_RPC_URL_MAINNET', ''))
a = p.parse_args()
if not a.rpc:
    raise SystemExit('set ETH_RPC_URL_MAINNET or pass --rpc')
head = subprocess.check_output(['git', '-C', str(a.adapters_root), 'rev-parse', 'HEAD'], text=True).strip()
if head != ADAPTERS_REVISION:
    raise SystemExit(f'adapters: expected pinned revision {ADAPTERS_REVISION}, got {head}')

with socket.socket() as s:
    s.bind(('127.0.0.1', 0))
    port = s.getsockname()[1]
endpoint = f'http://127.0.0.1:{port}'
anvil = subprocess.Popen(['anvil', '--fork-url', a.rpc, '--port', str(port), '--silent'])


def rpc(method, params):
    body = json.dumps({'jsonrpc': '2.0', 'id': 1, 'method': method, 'params': params}).encode()
    request = urllib.request.Request(endpoint, body, {'Content-Type': 'application/json'})
    with urllib.request.urlopen(request, timeout=10) as response:
        reply = json.load(response)
    if 'error' in reply:
        raise SystemExit(f'{method}: {reply["error"]}')
    return reply['result']


work = a.adapters_root / 'out' / 'iris-fork-fixture'
try:
    for _ in range(60):
        try:
            rpc('eth_chainId', [])
            break
        except OSError:
            time.sleep(1)
    # The stub answers MerkleAdapter.account() (and any other call) with the fixture account.
    stub = '0x73' + ACCOUNT[2:].lower() + '60005260206000f3'
    rpc('anvil_setCode', [ADAPTER, stub])
    work.mkdir(parents=True, exist_ok=True)
    built = {}
    for name in ('signing', 'settlement'):
        source, output = work / f'{name}.input.json', work / f'{name}.json'
        shutil.copyfile(ROOT / f'{name}.input.json', source)
        subprocess.run(['forge', 'script', 'script/recipes/BuildRecipe.s.sol:BuildRecipe', '--sig', 'run(string,string)',
                        str(source.relative_to(a.adapters_root)), str(output.relative_to(a.adapters_root)),
                        '--fork-url', endpoint], cwd=a.adapters_root, check=True, stdout=subprocess.DEVNULL)
        built[name] = json.loads(output.read_text())
finally:
    anvil.terminate()
    shutil.rmtree(work, ignore_errors=True)

if any(entry['account'].lower() != ACCOUNT.lower() for entry in built.values()):
    raise SystemExit('BuildRecipe used another account')
keep = ('recipe', 'recipeHash', 'signData', 'signMask', 'signLeaf', 'approvalData', 'approvalMask', 'approvalLeaf',
        'claimDebtData', 'claimDebtMask', 'claimDebtLeaf', 'claimCollateralData', 'claimCollateralMask', 'claimCollateralLeaf',
        'withdrawBondData', 'withdrawBondMask', 'withdrawBondLeaf')
fixture = {'adaptersRevision': ADAPTERS_REVISION, 'account': ACCOUNT}
for name, entry in built.items():
    fixture[name] = {key: entry[key] for key in keep if key in entry}
(ROOT / 'recipes.json').write_text(json.dumps(fixture, indent=2) + '\n')
