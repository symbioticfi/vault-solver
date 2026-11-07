// SPDX-License-Identifier: MIT
pragma solidity 0.8.38;

/// @dev eth_call-only return-data probe installed at the original recipe executor.
/// The unchanged outer fill runs separately; this probe cannot prove settlement.
contract OuterFillSimulation {
    bytes32 private constant IMPLEMENTATION = keccak256("vault-solver.rfq.outer-fill-simulation");

    error RFQRecipeResult(address executor, address caller, bytes32 calldataHash, bytes32 runtimeHash, bytes result);
    error RFQRecipeFailure(bytes reason);

    fallback() external payable {
        bytes32 slot = IMPLEMENTATION;
        address implementation;
        assembly { implementation := tload(slot) }
        if (implementation != address(0)) {
            // Preserve self-calls made by the original executor runtime.
            assembly {
                calldatacopy(0, 0, calldatasize())
                let success := delegatecall(gas(), implementation, 0, calldatasize(), 0, 0)
                returndatacopy(0, 0, returndatasize())
                if iszero(success) { revert(0, returndatasize()) }
                return(0, returndatasize())
            }
        }

        // The host appends abi.encode(runtime, caller, calldataHash), then its length.
        uint256 length;
        assembly {
            codecopy(0, sub(codesize(), 32), 32)
            length := mload(0)
        }
        require(length <= address(this).code.length - 32, "invalid probe configuration");
        bytes memory configuration = new bytes(length);
        assembly { codecopy(add(configuration, 32), sub(sub(codesize(), 32), length), length) }
        (bytes memory runtime, address caller, bytes32 calldataHash) = abi.decode(configuration, (bytes, address, bytes32));
        require(msg.sender == caller && keccak256(msg.data) == calldataHash, "unexpected recipe call");
        require(runtime.length != 0 && runtime.length <= 24576, "invalid original runtime");
        // Same runtime-copy pattern as SDK recipes/lens/BatchSimulation.sol.
        bytes memory creation = abi.encodePacked(hex"61", uint16(runtime.length), hex"80600a3d393df3", runtime);
        assembly { implementation := create(0, add(creation, 32), mload(creation)) }
        require(implementation != address(0), "runtime deployment failed");
        assembly { tstore(slot, implementation) }
        (bool success, bytes memory result) = implementation.delegatecall(msg.data);
        if (!success) revert RFQRecipeFailure(result);
        // Bubble the original execute's values through Executor/Reactor without
        // replacing either contract's signature, funding or authorization logic.
        revert RFQRecipeResult(address(this), msg.sender, calldataHash, keccak256(runtime), result);
    }
}
