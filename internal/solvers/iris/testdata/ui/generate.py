#!/usr/bin/env python3
"""Regenerate recipes.json from the curator UI's own Iris recipe compiler.

Runs dump.test.ts inside a curator-frontend checkout at REVISION with its dependencies
installed (pnpm install), then removes it again.
"""

import argparse
import os
import pathlib
import shutil
import subprocess

REVISION = "d6809886e5d278bb7cc15189a025cbd5f4650627"
HERE = pathlib.Path(__file__).resolve().parent


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--frontend-root", required=True, type=pathlib.Path)
    root = parser.parse_args().frontend_root.resolve()
    head = subprocess.run(["git", "-C", root, "rev-parse", "HEAD"], capture_output=True, text=True)
    revision = head.stdout.strip() if head.returncode == 0 else REVISION
    if revision != REVISION:
        raise SystemExit(f"curator-frontend is at {revision}, want {REVISION}")
    target = root / "src" / "zz-iris-fixture"
    target.mkdir(exist_ok=True)
    shutil.copy(HERE / "dump.test.ts", target / "dump.test.ts")
    try:
        env = dict(os.environ, IRIS_FIXTURE_OUT=str(HERE / "recipes.json"), IRIS_UI_REVISION=REVISION,
                   NODE_OPTIONS="--no-experimental-webstorage")
        subprocess.run(["npx", "vitest", "run", "src/zz-iris-fixture/dump.test.ts"], cwd=root, env=env, check=True)
    finally:
        shutil.rmtree(target)


if __name__ == "__main__":
    main()
