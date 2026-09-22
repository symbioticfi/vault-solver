# Solver7702Delegate artifact

`artifact.json` is the unmodified compiler artifact from
[CoW Protocol Solver7702Delegate](https://github.com/cowprotocol/solver-7702-delegate/tree/5bb720265b69d9d97ab4b18f8de7fc3940a22b35),
revision `5bb720265b69d9d97ab4b18f8de7fc3940a22b35`,
`out/Solver7702Delegate.sol/Solver7702Delegate.json`. Used under the included MIT license.

Refresh with `make refresh-solver-delegate`. It vendors the artifact and ABI and runs the pinned abigen
generator; do not hand-edit the ABI, artifact or generated binding. Runtime and immutable references
validate installed delegations. The constructor binding and creation bytecode are used by the local
Anvil test; runtime calls use the upstream packed fallback format, `bytes20(target) || calldata`.
