#!/usr/bin/env python3
"""Reproducibly compile the eth_call-only RFQ return-data probe."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
parser = argparse.ArgumentParser()
parser.add_argument("--check", action="store_true", help="verify the committed artifact")
args = parser.parse_args()
source = HERE / "OuterFillSimulation.sol"
with tempfile.TemporaryDirectory(prefix="rfq-outer-fill-simulation-") as temporary:
    root = Path(temporary)
    (root / "src").mkdir()
    (root / "src" / source.name).write_bytes(source.read_bytes())
    (root / "foundry.toml").write_text('[profile.default]\nsolc = "0.8.38"\nevm_version = "cancun"\noptimizer = true\noptimizer_runs = 200\nvia_ir = true\nbytecode_hash = "none"\n')
    subprocess.run(["forge", "build", "--skip", "test", "--root", temporary], check=True, stdout=subprocess.DEVNULL)
    artifact = json.loads((root / "out" / source.name / "OuterFillSimulation.json").read_text())
    runtime = artifact["deployedBytecode"]["object"]
    text = json.dumps({"compiler": artifact["metadata"]["compiler"]["version"], "evmVersion": "cancun",
                       "sourceSHA256": hashlib.sha256(source.read_bytes()).hexdigest(),
                       "runtimeSHA256": hashlib.sha256(bytes.fromhex(runtime.removeprefix("0x"))).hexdigest(),
                       "abi": artifact["abi"], "runtime": runtime}, indent=2) + "\n"
    path = HERE / "OuterFillSimulation.json"
    if args.check:
        if not path.exists() or path.read_text() != text:
            raise SystemExit("stale RFQ simulation artifact")
    else:
        path.write_text(text)
