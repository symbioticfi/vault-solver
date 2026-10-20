// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package connector

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = bytes.Equal
	_ = errors.New
	_ = big.NewInt
	_ = common.Big1
	_ = types.BloomLookup
	_ = abi.ConvertType
)

// ILiquidLaneConnectorDiscount is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneConnectorDiscount struct {
	TokenToRedeem common.Address
	Discount      *big.Int
	Signer        common.Address
	Protocol      common.Address
	Nonce         *big.Int
	Deadline      *big.Int
}

// ILiquidLaneConnectorDiscountSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneConnectorDiscountSwap struct {
	Discount         ILiquidLaneConnectorDiscount
	SignerSignature  []byte
	ProtocolDeadline *big.Int
}

// ILiquidLaneConnectorSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneConnectorSwap struct {
	Recipient common.Address
	TokenIn   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

// LiquidLaneConnectorMetaData contains all meta data concerning the LiquidLaneConnector contract.
var LiquidLaneConnectorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"newFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMerkleAccountFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newOracleRegistry\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MERKLE_ACCOUNT_FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"ORACLE_REGISTRY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"account\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"addTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expectedOracle\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"assetLimits\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"maxAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"duration\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"periodStart\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"spent\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAmountOut\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"invalidateNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isUsedNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"invalidated\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oracles\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"oracle\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"outputAsset\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"remainingAssets\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"removeTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMaxAssetsPerDuration\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMaxAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"newDuration\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMinDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMinDiscount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setSigner\",\"inputs\":[{\"name\":\"newSigner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"signer\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"newSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneConnector.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneConnector.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneConnector.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"AddTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"oracle\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DoSwap\",\"inputs\":[{\"name\":\"newSwap\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structILiquidLaneConnector.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"InvalidateNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RemoveTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMaxAssetsPerDuration\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"maxAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"duration\",\"type\":\"uint48\",\"indexed\":false,\"internalType\":\"uint48\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMinDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"minDiscount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetSigner\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyUsedNonce\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AssetLimitExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExpiredSwap\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAccount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDiscount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDuration\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOracle\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidShortString\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSigner\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSwapRate\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTokenToRedeem\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"StringTooLong\",\"inputs\":[{\"name\":\"str\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"UnconfiguredAsset\",\"inputs\":[]}]",
	ID:  "LiquidLaneConnector",
}

// LiquidLaneConnector is an auto generated Go binding around an Ethereum contract.
type LiquidLaneConnector struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *LiquidLaneConnector) GetABI() abi.ABI {
	return c.abi
}

// NewLiquidLaneConnector creates a new instance of LiquidLaneConnector.
func NewLiquidLaneConnector() *LiquidLaneConnector {
	parsed, err := LiquidLaneConnectorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &LiquidLaneConnector{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *LiquidLaneConnector) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address newFactory, address newMerkleAccountFactory, address newOracleRegistry) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackConstructor(newFactory common.Address, newMerkleAccountFactory common.Address, newOracleRegistry common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("", newFactory, newMerkleAccountFactory, newOracleRegistry)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dd31000.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function FACTORY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackFACTORY() []byte {
	enc, err := liquidLaneConnector.abi.Pack("FACTORY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dd31000.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function FACTORY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackFACTORY() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("FACTORY")
}

// UnpackFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackFACTORY(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("FACTORY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackMERKLEACCOUNTFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x631506fe.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function MERKLE_ACCOUNT_FACTORY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackMERKLEACCOUNTFACTORY() []byte {
	enc, err := liquidLaneConnector.abi.Pack("MERKLE_ACCOUNT_FACTORY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMERKLEACCOUNTFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x631506fe.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function MERKLE_ACCOUNT_FACTORY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackMERKLEACCOUNTFACTORY() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("MERKLE_ACCOUNT_FACTORY")
}

// UnpackMERKLEACCOUNTFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x631506fe.
//
// Solidity: function MERKLE_ACCOUNT_FACTORY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackMERKLEACCOUNTFACTORY(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("MERKLE_ACCOUNT_FACTORY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackORACLEREGISTRY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x78b122d8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function ORACLE_REGISTRY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackORACLEREGISTRY() []byte {
	enc, err := liquidLaneConnector.abi.Pack("ORACLE_REGISTRY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackORACLEREGISTRY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x78b122d8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function ORACLE_REGISTRY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackORACLEREGISTRY() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("ORACLE_REGISTRY")
}

// UnpackORACLEREGISTRY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x78b122d8.
//
// Solidity: function ORACLE_REGISTRY() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackORACLEREGISTRY(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("ORACLE_REGISTRY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackAccount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5dab2420.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function account() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackAccount() []byte {
	enc, err := liquidLaneConnector.abi.Pack("account")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAccount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5dab2420.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function account() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackAccount() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("account")
}

// UnpackAccount is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5dab2420.
//
// Solidity: function account() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackAccount(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("account", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackAddTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x26c52565.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function addTokenToRedeem(address tokenToRedeem, address expectedOracle) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackAddTokenToRedeem(tokenToRedeem common.Address, expectedOracle common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("addTokenToRedeem", tokenToRedeem, expectedOracle)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAddTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x26c52565.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function addTokenToRedeem(address tokenToRedeem, address expectedOracle) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackAddTokenToRedeem(tokenToRedeem common.Address, expectedOracle common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("addTokenToRedeem", tokenToRedeem, expectedOracle)
}

// PackAssetLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x560ee9dd.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function assetLimits(address tokenToRedeem) view returns(uint256 maxAssets, uint48 duration, uint48 periodStart, uint256 spent)
func (liquidLaneConnector *LiquidLaneConnector) PackAssetLimits(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("assetLimits", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAssetLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x560ee9dd.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function assetLimits(address tokenToRedeem) view returns(uint256 maxAssets, uint48 duration, uint48 periodStart, uint256 spent)
func (liquidLaneConnector *LiquidLaneConnector) TryPackAssetLimits(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("assetLimits", tokenToRedeem)
}

// AssetLimitsOutput serves as a container for the return parameters of contract
// method AssetLimits.
type AssetLimitsOutput struct {
	MaxAssets   *big.Int
	Duration    *big.Int
	PeriodStart *big.Int
	Spent       *big.Int
}

// UnpackAssetLimits is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x560ee9dd.
//
// Solidity: function assetLimits(address tokenToRedeem) view returns(uint256 maxAssets, uint48 duration, uint48 periodStart, uint256 spent)
func (liquidLaneConnector *LiquidLaneConnector) UnpackAssetLimits(data []byte) (AssetLimitsOutput, error) {
	out, err := liquidLaneConnector.abi.Unpack("assetLimits", data)
	outstruct := new(AssetLimitsOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.MaxAssets = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.Duration = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.PeriodStart = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.Spent = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (liquidLaneConnector *LiquidLaneConnector) PackEip712Domain() []byte {
	enc, err := liquidLaneConnector.abi.Pack("eip712Domain")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (liquidLaneConnector *LiquidLaneConnector) TryPackEip712Domain() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("eip712Domain")
}

// Eip712DomainOutput serves as a container for the return parameters of contract
// method Eip712Domain.
type Eip712DomainOutput struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}

// UnpackEip712Domain is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (liquidLaneConnector *LiquidLaneConnector) UnpackEip712Domain(data []byte) (Eip712DomainOutput, error) {
	out, err := liquidLaneConnector.abi.Unpack("eip712Domain", data)
	outstruct := new(Eip712DomainOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)
	return *outstruct, nil
}

// PackGetAmountOut is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca706bcf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) PackGetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("getAmountOut", tokenToRedeem, amountIn)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetAmountOut is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca706bcf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) TryPackGetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("getAmountOut", tokenToRedeem, amountIn)
}

// UnpackGetAmountOut is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xca706bcf.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) UnpackGetAmountOut(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("getAmountOut", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackInitialize(initialVersion uint64, owner common.Address, data []byte) []byte {
	enc, err := liquidLaneConnector.abi.Pack("initialize", initialVersion, owner, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackInitialize(initialVersion uint64, owner common.Address, data []byte) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("initialize", initialVersion, owner, data)
}

// PackInvalidateNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0d3762b5.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function invalidateNonce(address tokenToRedeem, uint256 nonce) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackInvalidateNonce(tokenToRedeem common.Address, nonce *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("invalidateNonce", tokenToRedeem, nonce)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInvalidateNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0d3762b5.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function invalidateNonce(address tokenToRedeem, uint256 nonce) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackInvalidateNonce(tokenToRedeem common.Address, nonce *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("invalidateNonce", tokenToRedeem, nonce)
}

// PackIsUsedNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ee60fa7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool invalidated)
func (liquidLaneConnector *LiquidLaneConnector) PackIsUsedNonce(tokenToRedeem common.Address, nonce *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("isUsedNonce", tokenToRedeem, nonce)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsUsedNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ee60fa7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool invalidated)
func (liquidLaneConnector *LiquidLaneConnector) TryPackIsUsedNonce(tokenToRedeem common.Address, nonce *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("isUsedNonce", tokenToRedeem, nonce)
}

// UnpackIsUsedNonce is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0ee60fa7.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool invalidated)
func (liquidLaneConnector *LiquidLaneConnector) UnpackIsUsedNonce(data []byte) (bool, error) {
	out, err := liquidLaneConnector.abi.Unpack("isUsedNonce", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2abe3048.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackMigrate(newVersion uint64, data []byte) []byte {
	enc, err := liquidLaneConnector.abi.Pack("migrate", newVersion, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2abe3048.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackMigrate(newVersion uint64, data []byte) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("migrate", newVersion, data)
}

// PackMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd9104c14.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 discount)
func (liquidLaneConnector *LiquidLaneConnector) PackMinDiscount(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("minDiscount", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd9104c14.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 discount)
func (liquidLaneConnector *LiquidLaneConnector) TryPackMinDiscount(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("minDiscount", tokenToRedeem)
}

// UnpackMinDiscount is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd9104c14.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 discount)
func (liquidLaneConnector *LiquidLaneConnector) UnpackMinDiscount(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("minDiscount", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackOracles is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaddd5099.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function oracles(address tokenToRedeem) view returns(address oracle)
func (liquidLaneConnector *LiquidLaneConnector) PackOracles(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("oracles", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOracles is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaddd5099.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function oracles(address tokenToRedeem) view returns(address oracle)
func (liquidLaneConnector *LiquidLaneConnector) TryPackOracles(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("oracles", tokenToRedeem)
}

// UnpackOracles is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xaddd5099.
//
// Solidity: function oracles(address tokenToRedeem) view returns(address oracle)
func (liquidLaneConnector *LiquidLaneConnector) UnpackOracles(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("oracles", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackOutputAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x69cb2a8b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function outputAsset() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackOutputAsset() []byte {
	enc, err := liquidLaneConnector.abi.Pack("outputAsset")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOutputAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x69cb2a8b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function outputAsset() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackOutputAsset() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("outputAsset")
}

// UnpackOutputAsset is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x69cb2a8b.
//
// Solidity: function outputAsset() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackOutputAsset(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("outputAsset", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackOwner() []byte {
	enc, err := liquidLaneConnector.abi.Pack("owner")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function owner() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackOwner() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackOwner(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("owner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd91bbc69.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function remainingAssets(address tokenToRedeem) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) PackRemainingAssets(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("remainingAssets", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd91bbc69.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function remainingAssets(address tokenToRedeem) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) TryPackRemainingAssets(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("remainingAssets", tokenToRedeem)
}

// UnpackRemainingAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd91bbc69.
//
// Solidity: function remainingAssets(address tokenToRedeem) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) UnpackRemainingAssets(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("remainingAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackRemoveTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe250fef7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function removeTokenToRedeem(address tokenToRedeem) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackRemoveTokenToRedeem(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("removeTokenToRedeem", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRemoveTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe250fef7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function removeTokenToRedeem(address tokenToRedeem) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackRemoveTokenToRedeem(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("removeTokenToRedeem", tokenToRedeem)
}

// PackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function renounceOwnership() returns()
func (liquidLaneConnector *LiquidLaneConnector) PackRenounceOwnership() []byte {
	enc, err := liquidLaneConnector.abi.Pack("renounceOwnership")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function renounceOwnership() returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackRenounceOwnership() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("renounceOwnership")
}

// PackSetMaxAssetsPerDuration is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x940e98c9.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setMaxAssetsPerDuration(address tokenToRedeem, uint256 newMaxAssets, uint48 newDuration) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackSetMaxAssetsPerDuration(tokenToRedeem common.Address, newMaxAssets *big.Int, newDuration *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("setMaxAssetsPerDuration", tokenToRedeem, newMaxAssets, newDuration)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetMaxAssetsPerDuration is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x940e98c9.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setMaxAssetsPerDuration(address tokenToRedeem, uint256 newMaxAssets, uint48 newDuration) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackSetMaxAssetsPerDuration(tokenToRedeem common.Address, newMaxAssets *big.Int, newDuration *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("setMaxAssetsPerDuration", tokenToRedeem, newMaxAssets, newDuration)
}

// PackSetMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0517ef9a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setMinDiscount(address tokenToRedeem, uint256 newMinDiscount) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackSetMinDiscount(tokenToRedeem common.Address, newMinDiscount *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("setMinDiscount", tokenToRedeem, newMinDiscount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0517ef9a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setMinDiscount(address tokenToRedeem, uint256 newMinDiscount) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackSetMinDiscount(tokenToRedeem common.Address, newMinDiscount *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("setMinDiscount", tokenToRedeem, newMinDiscount)
}

// PackSetSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6c19e783.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setSigner(address newSigner) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackSetSigner(newSigner common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("setSigner", newSigner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6c19e783.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setSigner(address newSigner) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackSetSigner(newSigner common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("setSigner", newSigner)
}

// PackSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x238ac933.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function signer() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackSigner() []byte {
	enc, err := liquidLaneConnector.abi.Pack("signer")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x238ac933.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function signer() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackSigner() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("signer")
}

// UnpackSigner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x238ac933.
//
// Solidity: function signer() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackSigner(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("signer", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackSwap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x53f549d8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function swap((address,address,uint256,uint256) newSwap) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackSwap(newSwap ILiquidLaneConnectorSwap) []byte {
	enc, err := liquidLaneConnector.abi.Pack("swap", newSwap)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSwap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x53f549d8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function swap((address,address,uint256,uint256) newSwap) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackSwap(newSwap ILiquidLaneConnectorSwap) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("swap", newSwap)
}

// PackSwap0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8fa5c671.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneConnector *LiquidLaneConnector) PackSwap0(discountSwap ILiquidLaneConnectorDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("swap0", discountSwap, protocolSignature, recipient, amountIn)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSwap0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8fa5c671.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneConnector *LiquidLaneConnector) TryPackSwap0(discountSwap ILiquidLaneConnectorDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("swap0", discountSwap, protocolSignature, recipient, amountIn)
}

// UnpackSwap0 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8fa5c671.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneConnector *LiquidLaneConnector) UnpackSwap0(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("swap0", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (liquidLaneConnector *LiquidLaneConnector) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("transferOwnership", newOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (liquidLaneConnector *LiquidLaneConnector) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("transferOwnership", newOwner)
}

// PackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function version() view returns(uint64)
func (liquidLaneConnector *LiquidLaneConnector) PackVersion() []byte {
	enc, err := liquidLaneConnector.abi.Pack("version")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function version() view returns(uint64)
func (liquidLaneConnector *LiquidLaneConnector) TryPackVersion() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (liquidLaneConnector *LiquidLaneConnector) UnpackVersion(data []byte) (uint64, error) {
	out, err := liquidLaneConnector.abi.Unpack("version", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// LiquidLaneConnectorAddTokenToRedeem represents a AddTokenToRedeem event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorAddTokenToRedeem struct {
	TokenToRedeem common.Address
	Oracle        common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorAddTokenToRedeemEventName = "AddTokenToRedeem"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorAddTokenToRedeem) ContractEventName() string {
	return LiquidLaneConnectorAddTokenToRedeemEventName
}

// UnpackAddTokenToRedeemEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event AddTokenToRedeem(address indexed tokenToRedeem, address indexed oracle)
func (liquidLaneConnector *LiquidLaneConnector) UnpackAddTokenToRedeemEvent(log *types.Log) (*LiquidLaneConnectorAddTokenToRedeem, error) {
	event := "AddTokenToRedeem"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorAddTokenToRedeem)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorDoSwap represents a DoSwap event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorDoSwap struct {
	NewSwap ILiquidLaneConnectorSwap
	Raw     *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorDoSwapEventName = "DoSwap"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorDoSwap) ContractEventName() string {
	return LiquidLaneConnectorDoSwapEventName
}

// UnpackDoSwapEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event DoSwap((address,address,uint256,uint256) newSwap)
func (liquidLaneConnector *LiquidLaneConnector) UnpackDoSwapEvent(log *types.Log) (*LiquidLaneConnectorDoSwap, error) {
	event := "DoSwap"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorDoSwap)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorEIP712DomainChanged represents a EIP712DomainChanged event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorEIP712DomainChanged struct {
	Raw *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorEIP712DomainChangedEventName = "EIP712DomainChanged"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorEIP712DomainChanged) ContractEventName() string {
	return LiquidLaneConnectorEIP712DomainChangedEventName
}

// UnpackEIP712DomainChangedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event EIP712DomainChanged()
func (liquidLaneConnector *LiquidLaneConnector) UnpackEIP712DomainChangedEvent(log *types.Log) (*LiquidLaneConnectorEIP712DomainChanged, error) {
	event := "EIP712DomainChanged"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorEIP712DomainChanged)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorInitialized represents a Initialized event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInitialized struct {
	Version uint64
	Raw     *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorInitializedEventName = "Initialized"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorInitialized) ContractEventName() string {
	return LiquidLaneConnectorInitializedEventName
}

// UnpackInitializedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialized(uint64 version)
func (liquidLaneConnector *LiquidLaneConnector) UnpackInitializedEvent(log *types.Log) (*LiquidLaneConnectorInitialized, error) {
	event := "Initialized"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorInitialized)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorInvalidateNonce represents a InvalidateNonce event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidateNonce struct {
	TokenToRedeem common.Address
	Nonce         *big.Int
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorInvalidateNonceEventName = "InvalidateNonce"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorInvalidateNonce) ContractEventName() string {
	return LiquidLaneConnectorInvalidateNonceEventName
}

// UnpackInvalidateNonceEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event InvalidateNonce(address indexed tokenToRedeem, uint256 nonce)
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidateNonceEvent(log *types.Log) (*LiquidLaneConnectorInvalidateNonce, error) {
	event := "InvalidateNonce"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorInvalidateNonce)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorOwnershipTransferred represents a OwnershipTransferred event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorOwnershipTransferred) ContractEventName() string {
	return LiquidLaneConnectorOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (liquidLaneConnector *LiquidLaneConnector) UnpackOwnershipTransferredEvent(log *types.Log) (*LiquidLaneConnectorOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorRemoveTokenToRedeem represents a RemoveTokenToRedeem event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorRemoveTokenToRedeem struct {
	TokenToRedeem common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorRemoveTokenToRedeemEventName = "RemoveTokenToRedeem"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorRemoveTokenToRedeem) ContractEventName() string {
	return LiquidLaneConnectorRemoveTokenToRedeemEventName
}

// UnpackRemoveTokenToRedeemEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event RemoveTokenToRedeem(address indexed tokenToRedeem)
func (liquidLaneConnector *LiquidLaneConnector) UnpackRemoveTokenToRedeemEvent(log *types.Log) (*LiquidLaneConnectorRemoveTokenToRedeem, error) {
	event := "RemoveTokenToRedeem"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorRemoveTokenToRedeem)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorSetMaxAssetsPerDuration represents a SetMaxAssetsPerDuration event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorSetMaxAssetsPerDuration struct {
	TokenToRedeem common.Address
	MaxAssets     *big.Int
	Duration      *big.Int
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorSetMaxAssetsPerDurationEventName = "SetMaxAssetsPerDuration"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorSetMaxAssetsPerDuration) ContractEventName() string {
	return LiquidLaneConnectorSetMaxAssetsPerDurationEventName
}

// UnpackSetMaxAssetsPerDurationEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetMaxAssetsPerDuration(address indexed tokenToRedeem, uint256 maxAssets, uint48 duration)
func (liquidLaneConnector *LiquidLaneConnector) UnpackSetMaxAssetsPerDurationEvent(log *types.Log) (*LiquidLaneConnectorSetMaxAssetsPerDuration, error) {
	event := "SetMaxAssetsPerDuration"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorSetMaxAssetsPerDuration)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorSetMinDiscount represents a SetMinDiscount event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorSetMinDiscount struct {
	TokenToRedeem common.Address
	MinDiscount   *big.Int
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorSetMinDiscountEventName = "SetMinDiscount"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorSetMinDiscount) ContractEventName() string {
	return LiquidLaneConnectorSetMinDiscountEventName
}

// UnpackSetMinDiscountEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetMinDiscount(address indexed tokenToRedeem, uint256 minDiscount)
func (liquidLaneConnector *LiquidLaneConnector) UnpackSetMinDiscountEvent(log *types.Log) (*LiquidLaneConnectorSetMinDiscount, error) {
	event := "SetMinDiscount"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorSetMinDiscount)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// LiquidLaneConnectorSetSigner represents a SetSigner event raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorSetSigner struct {
	Signer common.Address
	Raw    *types.Log // Blockchain specific contextual infos
}

const LiquidLaneConnectorSetSignerEventName = "SetSigner"

// ContractEventName returns the user-defined event name.
func (LiquidLaneConnectorSetSigner) ContractEventName() string {
	return LiquidLaneConnectorSetSignerEventName
}

// UnpackSetSignerEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetSigner(address indexed signer)
func (liquidLaneConnector *LiquidLaneConnector) UnpackSetSignerEvent(log *types.Log) (*LiquidLaneConnectorSetSigner, error) {
	event := "SetSigner"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneConnector.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneConnectorSetSigner)
	if len(log.Data) > 0 {
		if err := liquidLaneConnector.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneConnector.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}

// UnpackError attempts to decode the provided error data using user-defined
// error definitions.
func (liquidLaneConnector *LiquidLaneConnector) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["AlreadyInitialized"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["AlreadyUsedNonce"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackAlreadyUsedNonceError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["AssetLimitExceeded"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackAssetLimitExceededError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["ExpiredSwap"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackExpiredSwapError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidAccount"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidCaller"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidCallerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidDiscount"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidDiscountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidDuration"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidDurationError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidInitialization"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidInitializationError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidOracle"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidOracleError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidShortString"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidShortStringError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidSignature"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidSignatureError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidSigner"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidSignerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidSwapRate"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidSwapRateError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["InvalidTokenToRedeem"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackInvalidTokenToRedeemError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["NotFactory"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackNotFactoryError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["NotInitializing"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackNotInitializingError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["OwnableInvalidOwner"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackOwnableInvalidOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["OwnableUnauthorizedAccount"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackOwnableUnauthorizedAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["ReentrancyGuardReentrantCall"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackReentrancyGuardReentrantCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["StringTooLong"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackStringTooLongError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneConnector.abi.Errors["UnconfiguredAsset"].ID.Bytes()[:4]) {
		return liquidLaneConnector.UnpackUnconfiguredAssetError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// LiquidLaneConnectorAlreadyInitialized represents a AlreadyInitialized error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorAlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyInitialized()
func LiquidLaneConnectorAlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x0dc149f07762891dbcea3fe72770f3d63a1863fc54b2f084e8c59ec476996927")
}

// UnpackAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyInitialized()
func (liquidLaneConnector *LiquidLaneConnector) UnpackAlreadyInitializedError(raw []byte) (*LiquidLaneConnectorAlreadyInitialized, error) {
	out := new(LiquidLaneConnectorAlreadyInitialized)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "AlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorAlreadyUsedNonce represents a AlreadyUsedNonce error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorAlreadyUsedNonce struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyUsedNonce()
func LiquidLaneConnectorAlreadyUsedNonceErrorID() common.Hash {
	return common.HexToHash("0x5247cb318fe1d375b7264beff576b70703d01bb4ac69fa1e1530f58741bcaf65")
}

// UnpackAlreadyUsedNonceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyUsedNonce()
func (liquidLaneConnector *LiquidLaneConnector) UnpackAlreadyUsedNonceError(raw []byte) (*LiquidLaneConnectorAlreadyUsedNonce, error) {
	out := new(LiquidLaneConnectorAlreadyUsedNonce)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "AlreadyUsedNonce", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorAssetLimitExceeded represents a AssetLimitExceeded error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorAssetLimitExceeded struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AssetLimitExceeded()
func LiquidLaneConnectorAssetLimitExceededErrorID() common.Hash {
	return common.HexToHash("0x60b61aa9fddeb67a340693aaa708501fda9326e5142914e68238c77925c8c334")
}

// UnpackAssetLimitExceededError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AssetLimitExceeded()
func (liquidLaneConnector *LiquidLaneConnector) UnpackAssetLimitExceededError(raw []byte) (*LiquidLaneConnectorAssetLimitExceeded, error) {
	out := new(LiquidLaneConnectorAssetLimitExceeded)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "AssetLimitExceeded", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorExpiredSwap represents a ExpiredSwap error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorExpiredSwap struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ExpiredSwap()
func LiquidLaneConnectorExpiredSwapErrorID() common.Hash {
	return common.HexToHash("0x1c74180ce4ab063a7fe469e2a3b2a873fe947c3b7649f1a95af498681712959d")
}

// UnpackExpiredSwapError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ExpiredSwap()
func (liquidLaneConnector *LiquidLaneConnector) UnpackExpiredSwapError(raw []byte) (*LiquidLaneConnectorExpiredSwap, error) {
	out := new(LiquidLaneConnectorExpiredSwap)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "ExpiredSwap", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidAccount represents a InvalidAccount error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidAccount struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAccount()
func LiquidLaneConnectorInvalidAccountErrorID() common.Hash {
	return common.HexToHash("0x6d187b28dc0ac7ec3747dcca312a7f01229ba51941237588cf813e48090f2a50")
}

// UnpackInvalidAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAccount()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidAccountError(raw []byte) (*LiquidLaneConnectorInvalidAccount, error) {
	out := new(LiquidLaneConnectorInvalidAccount)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidCaller represents a InvalidCaller error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidCaller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidCaller()
func LiquidLaneConnectorInvalidCallerErrorID() common.Hash {
	return common.HexToHash("0x48f5c3ed50241a1b6c87d204a25d9d01339cd768de9a714ffbb53a5bb6ad572a")
}

// UnpackInvalidCallerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidCaller()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidCallerError(raw []byte) (*LiquidLaneConnectorInvalidCaller, error) {
	out := new(LiquidLaneConnectorInvalidCaller)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidCaller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidDiscount represents a InvalidDiscount error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidDiscount struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidDiscount()
func LiquidLaneConnectorInvalidDiscountErrorID() common.Hash {
	return common.HexToHash("0x997ea3601b175042a225bcd305c8e0434449bb795e78e4cb3f93a718c1a88caf")
}

// UnpackInvalidDiscountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidDiscount()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidDiscountError(raw []byte) (*LiquidLaneConnectorInvalidDiscount, error) {
	out := new(LiquidLaneConnectorInvalidDiscount)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidDiscount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidDuration represents a InvalidDuration error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidDuration struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidDuration()
func LiquidLaneConnectorInvalidDurationErrorID() common.Hash {
	return common.HexToHash("0x76166401494e5a511541446d9197f2558d8ed87f455106fcd343b5ffbb4fc7bb")
}

// UnpackInvalidDurationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidDuration()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidDurationError(raw []byte) (*LiquidLaneConnectorInvalidDuration, error) {
	out := new(LiquidLaneConnectorInvalidDuration)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidDuration", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidInitialization represents a InvalidInitialization error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidInitialization struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidInitialization()
func LiquidLaneConnectorInvalidInitializationErrorID() common.Hash {
	return common.HexToHash("0xf92ee8a957075833165f68c320933b1a1294aafc84ee6e0dd3fb178008f9aaf5")
}

// UnpackInvalidInitializationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidInitialization()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidInitializationError(raw []byte) (*LiquidLaneConnectorInvalidInitialization, error) {
	out := new(LiquidLaneConnectorInvalidInitialization)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidInitialization", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidOracle represents a InvalidOracle error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidOracle struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidOracle()
func LiquidLaneConnectorInvalidOracleErrorID() common.Hash {
	return common.HexToHash("0x9589a27d464cce309224596a505cbfd22e5fde1f0f420cecf8a6b6c1d65791b6")
}

// UnpackInvalidOracleError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidOracle()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidOracleError(raw []byte) (*LiquidLaneConnectorInvalidOracle, error) {
	out := new(LiquidLaneConnectorInvalidOracle)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidOracle", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidShortString represents a InvalidShortString error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidShortString struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidShortString()
func LiquidLaneConnectorInvalidShortStringErrorID() common.Hash {
	return common.HexToHash("0xb3512b0c6163e5f0bafab72bb631b9d58cd7a731b082f910338aa21c83d5c274")
}

// UnpackInvalidShortStringError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidShortString()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidShortStringError(raw []byte) (*LiquidLaneConnectorInvalidShortString, error) {
	out := new(LiquidLaneConnectorInvalidShortString)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidShortString", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidSignature represents a InvalidSignature error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidSignature struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidSignature()
func LiquidLaneConnectorInvalidSignatureErrorID() common.Hash {
	return common.HexToHash("0x8baa579fce362245063d36f11747a89dd489c54795634fc673cc0e0db51fedc5")
}

// UnpackInvalidSignatureError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidSignature()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidSignatureError(raw []byte) (*LiquidLaneConnectorInvalidSignature, error) {
	out := new(LiquidLaneConnectorInvalidSignature)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidSignature", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidSigner represents a InvalidSigner error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidSigner struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidSigner()
func LiquidLaneConnectorInvalidSignerErrorID() common.Hash {
	return common.HexToHash("0x815e1d64efb74fbe314c20a2b8a2335d18bce12a19165e447fa36bcb35959528")
}

// UnpackInvalidSignerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidSigner()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidSignerError(raw []byte) (*LiquidLaneConnectorInvalidSigner, error) {
	out := new(LiquidLaneConnectorInvalidSigner)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidSigner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidSwapRate represents a InvalidSwapRate error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidSwapRate struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidSwapRate()
func LiquidLaneConnectorInvalidSwapRateErrorID() common.Hash {
	return common.HexToHash("0x285217814127ad6ebb3698d6e77d442a1bfc2ce040377b7a098f217e85a0789e")
}

// UnpackInvalidSwapRateError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidSwapRate()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidSwapRateError(raw []byte) (*LiquidLaneConnectorInvalidSwapRate, error) {
	out := new(LiquidLaneConnectorInvalidSwapRate)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidSwapRate", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorInvalidTokenToRedeem represents a InvalidTokenToRedeem error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorInvalidTokenToRedeem struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidTokenToRedeem()
func LiquidLaneConnectorInvalidTokenToRedeemErrorID() common.Hash {
	return common.HexToHash("0x997c4c8864a0026f8a0f15ff73d73b124e940d19db898a179d4983158f41cdf5")
}

// UnpackInvalidTokenToRedeemError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidTokenToRedeem()
func (liquidLaneConnector *LiquidLaneConnector) UnpackInvalidTokenToRedeemError(raw []byte) (*LiquidLaneConnectorInvalidTokenToRedeem, error) {
	out := new(LiquidLaneConnectorInvalidTokenToRedeem)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "InvalidTokenToRedeem", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorNotFactory represents a NotFactory error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorNotFactory struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotFactory()
func LiquidLaneConnectorNotFactoryErrorID() common.Hash {
	return common.HexToHash("0x32cc723614e775fc4a8386492bc9a860c12fe98d5f5f28ec17e265818645b229")
}

// UnpackNotFactoryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotFactory()
func (liquidLaneConnector *LiquidLaneConnector) UnpackNotFactoryError(raw []byte) (*LiquidLaneConnectorNotFactory, error) {
	out := new(LiquidLaneConnectorNotFactory)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "NotFactory", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorNotInitializing represents a NotInitializing error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorNotInitializing struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitializing()
func LiquidLaneConnectorNotInitializingErrorID() common.Hash {
	return common.HexToHash("0xd7e6bcf8597daa127dc9f0048d2f08d5ef140a2cb659feabd700beff1f7a8302")
}

// UnpackNotInitializingError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitializing()
func (liquidLaneConnector *LiquidLaneConnector) UnpackNotInitializingError(raw []byte) (*LiquidLaneConnectorNotInitializing, error) {
	out := new(LiquidLaneConnectorNotInitializing)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "NotInitializing", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorOwnableInvalidOwner represents a OwnableInvalidOwner error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorOwnableInvalidOwner struct {
	Owner common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableInvalidOwner(address owner)
func LiquidLaneConnectorOwnableInvalidOwnerErrorID() common.Hash {
	return common.HexToHash("0x1e4fbdf7f3ef8bcaa855599e3abf48b232380f183f08f6f813d9ffa5bd585188")
}

// UnpackOwnableInvalidOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableInvalidOwner(address owner)
func (liquidLaneConnector *LiquidLaneConnector) UnpackOwnableInvalidOwnerError(raw []byte) (*LiquidLaneConnectorOwnableInvalidOwner, error) {
	out := new(LiquidLaneConnectorOwnableInvalidOwner)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "OwnableInvalidOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorOwnableUnauthorizedAccount represents a OwnableUnauthorizedAccount error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorOwnableUnauthorizedAccount struct {
	Account common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func LiquidLaneConnectorOwnableUnauthorizedAccountErrorID() common.Hash {
	return common.HexToHash("0x118cdaa7a341953d1887a2245fd6665d741c67c8c50581daa59e1d03373fa188")
}

// UnpackOwnableUnauthorizedAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func (liquidLaneConnector *LiquidLaneConnector) UnpackOwnableUnauthorizedAccountError(raw []byte) (*LiquidLaneConnectorOwnableUnauthorizedAccount, error) {
	out := new(LiquidLaneConnectorOwnableUnauthorizedAccount)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "OwnableUnauthorizedAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorReentrancyGuardReentrantCall represents a ReentrancyGuardReentrantCall error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorReentrancyGuardReentrantCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ReentrancyGuardReentrantCall()
func LiquidLaneConnectorReentrancyGuardReentrantCallErrorID() common.Hash {
	return common.HexToHash("0x3ee5aeb571de7fc460830b4d0017439a1ca56fb0bc39062227ade4fe4a24c1ca")
}

// UnpackReentrancyGuardReentrantCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ReentrancyGuardReentrantCall()
func (liquidLaneConnector *LiquidLaneConnector) UnpackReentrancyGuardReentrantCallError(raw []byte) (*LiquidLaneConnectorReentrancyGuardReentrantCall, error) {
	out := new(LiquidLaneConnectorReentrancyGuardReentrantCall)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "ReentrancyGuardReentrantCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func LiquidLaneConnectorSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (liquidLaneConnector *LiquidLaneConnector) UnpackSafeERC20FailedOperationError(raw []byte) (*LiquidLaneConnectorSafeERC20FailedOperation, error) {
	out := new(LiquidLaneConnectorSafeERC20FailedOperation)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorStringTooLong represents a StringTooLong error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorStringTooLong struct {
	Str string
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error StringTooLong(string str)
func LiquidLaneConnectorStringTooLongErrorID() common.Hash {
	return common.HexToHash("0x305a27a93f8e33b7392df0a0f91d6fc63847395853c45991eec52dbf24d72381")
}

// UnpackStringTooLongError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error StringTooLong(string str)
func (liquidLaneConnector *LiquidLaneConnector) UnpackStringTooLongError(raw []byte) (*LiquidLaneConnectorStringTooLong, error) {
	out := new(LiquidLaneConnectorStringTooLong)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "StringTooLong", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneConnectorUnconfiguredAsset represents a UnconfiguredAsset error raised by the LiquidLaneConnector contract.
type LiquidLaneConnectorUnconfiguredAsset struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error UnconfiguredAsset()
func LiquidLaneConnectorUnconfiguredAssetErrorID() common.Hash {
	return common.HexToHash("0x552a0531c16f1a1cb5775b34916021b261addb8704ee0960c2f52e5afe6ef591")
}

// UnpackUnconfiguredAssetError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error UnconfiguredAsset()
func (liquidLaneConnector *LiquidLaneConnector) UnpackUnconfiguredAssetError(raw []byte) (*LiquidLaneConnectorUnconfiguredAsset, error) {
	out := new(LiquidLaneConnectorUnconfiguredAsset)
	if err := liquidLaneConnector.abi.UnpackIntoInterface(out, "UnconfiguredAsset", raw); err != nil {
		return nil, err
	}
	return out, nil
}
