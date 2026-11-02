// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

// External-world stand-ins only. The execution path uses unmodified canonical
// Reactor, Executor, RecipeExecutor, MerkleAccount and LiquidLaneConnector bytecode.
contract FixtureToken {
    uint8 public constant decimals = 6;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;
    function mint(address to, uint256 amount) external { balanceOf[to] += amount; }
    function burn(address from, uint256 amount) external { balanceOf[from] -= amount; }
    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        return true;
    }
    function transfer(address to, uint256 amount) external returns (bool) {
        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount;
        return true;
    }
    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        if (allowance[from][msg.sender] != type(uint256).max) allowance[from][msg.sender] -= amount;
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        return true;
    }
}

contract FixtureWorld {
    address public immutable asset;
    mapping(address => bytes32) public roots;
    mapping(address => mapping(address => address)) public oracles;
    constructor(address fundingAsset) { asset = fundingAsset; }
    function isEntity(address) external pure returns (bool) { return true; }
    function vault() external view returns (address) { return address(this); }
    function setRoot(address account, bytes32 root) external { roots[account] = root; }
    function setOracle(address connector, address rwa) external { oracles[connector][rwa] = address(this); }
    function getPrice() external pure returns (uint256) { return 1e18; }
}
