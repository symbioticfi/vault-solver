// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package threefpolicy

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

// IThreeFPolicyMetaData contains all meta data concerning the IThreeFPolicy contract.
var IThreeFPolicyMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"FACILITY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"REQUESTS_FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"account\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"assetLimits\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"underlyingAsset\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"maxAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"duration\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"periodStart\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"spent\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"consume\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yieldAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minYieldPerRequest\",\"inputs\":[{\"name\":\"underlyingAsset\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"remainingAssets\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"underlyingAsset\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"setMaxAssetsPerDuration\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"underlyingAsset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMaxAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"newDuration\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMinYieldPerRequest\",\"inputs\":[{\"name\":\"underlyingAsset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMinimum\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"Consume\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"underlyingAsset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMaxAssetsPerDuration\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"underlyingAsset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"maxAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"duration\",\"type\":\"uint48\",\"indexed\":false,\"internalType\":\"uint48\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMinYieldPerRequest\",\"inputs\":[{\"name\":\"underlyingAsset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"minimum\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAsset\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAssets\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidRequest\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidRequestName\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"RequestMismatch\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooLowYield\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"UnsupportedTarget\",\"inputs\":[]}]",
	ID:  "IThreeFPolicy",
}

// IThreeFPolicy is an auto generated Go binding around an Ethereum contract.
type IThreeFPolicy struct {
	abi abi.ABI
}

// NewIThreeFPolicy creates a new instance of IThreeFPolicy.
func NewIThreeFPolicy() *IThreeFPolicy {
	parsed, err := IThreeFPolicyMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IThreeFPolicy{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IThreeFPolicy) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackFACILITY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x26f33656.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function FACILITY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) PackFACILITY() []byte {
	enc, err := iThreeFPolicy.abi.Pack("FACILITY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFACILITY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x26f33656.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function FACILITY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) TryPackFACILITY() ([]byte, error) {
	return iThreeFPolicy.abi.Pack("FACILITY")
}

// UnpackFACILITY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x26f33656.
//
// Solidity: function FACILITY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) UnpackFACILITY(data []byte) (common.Address, error) {
	out, err := iThreeFPolicy.abi.Unpack("FACILITY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dd31000.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function FACTORY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) PackFACTORY() []byte {
	enc, err := iThreeFPolicy.abi.Pack("FACTORY")
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
func (iThreeFPolicy *IThreeFPolicy) TryPackFACTORY() ([]byte, error) {
	return iThreeFPolicy.abi.Pack("FACTORY")
}

// UnpackFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) UnpackFACTORY(data []byte) (common.Address, error) {
	out, err := iThreeFPolicy.abi.Unpack("FACTORY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackREQUESTSFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x896edd76.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function REQUESTS_FACTORY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) PackREQUESTSFACTORY() []byte {
	enc, err := iThreeFPolicy.abi.Pack("REQUESTS_FACTORY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackREQUESTSFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x896edd76.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function REQUESTS_FACTORY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) TryPackREQUESTSFACTORY() ([]byte, error) {
	return iThreeFPolicy.abi.Pack("REQUESTS_FACTORY")
}

// UnpackREQUESTSFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x896edd76.
//
// Solidity: function REQUESTS_FACTORY() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) UnpackREQUESTSFACTORY(data []byte) (common.Address, error) {
	out, err := iThreeFPolicy.abi.Unpack("REQUESTS_FACTORY", data)
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
func (iThreeFPolicy *IThreeFPolicy) PackAccount() []byte {
	enc, err := iThreeFPolicy.abi.Pack("account")
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
func (iThreeFPolicy *IThreeFPolicy) TryPackAccount() ([]byte, error) {
	return iThreeFPolicy.abi.Pack("account")
}

// UnpackAccount is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5dab2420.
//
// Solidity: function account() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) UnpackAccount(data []byte) (common.Address, error) {
	out, err := iThreeFPolicy.abi.Unpack("account", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackAssetLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xcd6493bb.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function assetLimits(address asset, address underlyingAsset) view returns(uint256 maxAssets, uint48 duration, uint48 periodStart, uint256 spent)
func (iThreeFPolicy *IThreeFPolicy) PackAssetLimits(asset common.Address, underlyingAsset common.Address) []byte {
	enc, err := iThreeFPolicy.abi.Pack("assetLimits", asset, underlyingAsset)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAssetLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xcd6493bb.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function assetLimits(address asset, address underlyingAsset) view returns(uint256 maxAssets, uint48 duration, uint48 periodStart, uint256 spent)
func (iThreeFPolicy *IThreeFPolicy) TryPackAssetLimits(asset common.Address, underlyingAsset common.Address) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("assetLimits", asset, underlyingAsset)
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
// from invoking the contract method with ID 0xcd6493bb.
//
// Solidity: function assetLimits(address asset, address underlyingAsset) view returns(uint256 maxAssets, uint48 duration, uint48 periodStart, uint256 spent)
func (iThreeFPolicy *IThreeFPolicy) UnpackAssetLimits(data []byte) (AssetLimitsOutput, error) {
	out, err := iThreeFPolicy.abi.Unpack("assetLimits", data)
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

// PackConsume is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7edab8a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function consume(address request, uint256 assets, uint256 yieldAssets) returns()
func (iThreeFPolicy *IThreeFPolicy) PackConsume(request common.Address, assets *big.Int, yieldAssets *big.Int) []byte {
	enc, err := iThreeFPolicy.abi.Pack("consume", request, assets, yieldAssets)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackConsume is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7edab8a6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function consume(address request, uint256 assets, uint256 yieldAssets) returns()
func (iThreeFPolicy *IThreeFPolicy) TryPackConsume(request common.Address, assets *big.Int, yieldAssets *big.Int) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("consume", request, assets, yieldAssets)
}

// PackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (iThreeFPolicy *IThreeFPolicy) PackInitialize(initialVersion uint64, owner common.Address, data []byte) []byte {
	enc, err := iThreeFPolicy.abi.Pack("initialize", initialVersion, owner, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (iThreeFPolicy *IThreeFPolicy) TryPackInitialize(initialVersion uint64, owner common.Address, data []byte) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("initialize", initialVersion, owner, data)
}

// PackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2abe3048.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (iThreeFPolicy *IThreeFPolicy) PackMigrate(newVersion uint64, data []byte) []byte {
	enc, err := iThreeFPolicy.abi.Pack("migrate", newVersion, data)
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
func (iThreeFPolicy *IThreeFPolicy) TryPackMigrate(newVersion uint64, data []byte) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("migrate", newVersion, data)
}

// PackMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1fd317e3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minYieldPerRequest(address underlyingAsset) view returns(uint256)
func (iThreeFPolicy *IThreeFPolicy) PackMinYieldPerRequest(underlyingAsset common.Address) []byte {
	enc, err := iThreeFPolicy.abi.Pack("minYieldPerRequest", underlyingAsset)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1fd317e3.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minYieldPerRequest(address underlyingAsset) view returns(uint256)
func (iThreeFPolicy *IThreeFPolicy) TryPackMinYieldPerRequest(underlyingAsset common.Address) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("minYieldPerRequest", underlyingAsset)
}

// UnpackMinYieldPerRequest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1fd317e3.
//
// Solidity: function minYieldPerRequest(address underlyingAsset) view returns(uint256)
func (iThreeFPolicy *IThreeFPolicy) UnpackMinYieldPerRequest(data []byte) (*big.Int, error) {
	out, err := iThreeFPolicy.abi.Unpack("minYieldPerRequest", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMulticall is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xac9650d8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function multicall(bytes[] data) returns()
func (iThreeFPolicy *IThreeFPolicy) PackMulticall(data [][]byte) []byte {
	enc, err := iThreeFPolicy.abi.Pack("multicall", data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMulticall is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xac9650d8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function multicall(bytes[] data) returns()
func (iThreeFPolicy *IThreeFPolicy) TryPackMulticall(data [][]byte) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("multicall", data)
}

// PackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x764ef286.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function remainingAssets(address asset, address underlyingAsset) view returns(uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) PackRemainingAssets(asset common.Address, underlyingAsset common.Address) []byte {
	enc, err := iThreeFPolicy.abi.Pack("remainingAssets", asset, underlyingAsset)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x764ef286.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function remainingAssets(address asset, address underlyingAsset) view returns(uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) TryPackRemainingAssets(asset common.Address, underlyingAsset common.Address) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("remainingAssets", asset, underlyingAsset)
}

// UnpackRemainingAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x764ef286.
//
// Solidity: function remainingAssets(address asset, address underlyingAsset) view returns(uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) UnpackRemainingAssets(data []byte) (*big.Int, error) {
	out, err := iThreeFPolicy.abi.Unpack("remainingAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackSetMaxAssetsPerDuration is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7abc2f4b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setMaxAssetsPerDuration(address asset, address underlyingAsset, uint256 newMaxAssets, uint48 newDuration) returns()
func (iThreeFPolicy *IThreeFPolicy) PackSetMaxAssetsPerDuration(asset common.Address, underlyingAsset common.Address, newMaxAssets *big.Int, newDuration *big.Int) []byte {
	enc, err := iThreeFPolicy.abi.Pack("setMaxAssetsPerDuration", asset, underlyingAsset, newMaxAssets, newDuration)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetMaxAssetsPerDuration is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7abc2f4b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setMaxAssetsPerDuration(address asset, address underlyingAsset, uint256 newMaxAssets, uint48 newDuration) returns()
func (iThreeFPolicy *IThreeFPolicy) TryPackSetMaxAssetsPerDuration(asset common.Address, underlyingAsset common.Address, newMaxAssets *big.Int, newDuration *big.Int) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("setMaxAssetsPerDuration", asset, underlyingAsset, newMaxAssets, newDuration)
}

// PackSetMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6c5d770.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setMinYieldPerRequest(address underlyingAsset, uint256 newMinimum) returns()
func (iThreeFPolicy *IThreeFPolicy) PackSetMinYieldPerRequest(underlyingAsset common.Address, newMinimum *big.Int) []byte {
	enc, err := iThreeFPolicy.abi.Pack("setMinYieldPerRequest", underlyingAsset, newMinimum)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6c5d770.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setMinYieldPerRequest(address underlyingAsset, uint256 newMinimum) returns()
func (iThreeFPolicy *IThreeFPolicy) TryPackSetMinYieldPerRequest(underlyingAsset common.Address, newMinimum *big.Int) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("setMinYieldPerRequest", underlyingAsset, newMinimum)
}

// PackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function version() view returns(uint64)
func (iThreeFPolicy *IThreeFPolicy) PackVersion() []byte {
	enc, err := iThreeFPolicy.abi.Pack("version")
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
func (iThreeFPolicy *IThreeFPolicy) TryPackVersion() ([]byte, error) {
	return iThreeFPolicy.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (iThreeFPolicy *IThreeFPolicy) UnpackVersion(data []byte) (uint64, error) {
	out, err := iThreeFPolicy.abi.Unpack("version", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// IThreeFPolicyConsume represents a Consume event raised by the IThreeFPolicy contract.
type IThreeFPolicyConsume struct {
	Asset           common.Address
	UnderlyingAsset common.Address
	Assets          *big.Int
	Raw             *types.Log // Blockchain specific contextual infos
}

const IThreeFPolicyConsumeEventName = "Consume"

// ContractEventName returns the user-defined event name.
func (IThreeFPolicyConsume) ContractEventName() string {
	return IThreeFPolicyConsumeEventName
}

// UnpackConsumeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Consume(address indexed asset, address indexed underlyingAsset, uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) UnpackConsumeEvent(log *types.Log) (*IThreeFPolicyConsume, error) {
	event := "Consume"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != iThreeFPolicy.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(IThreeFPolicyConsume)
	if len(log.Data) > 0 {
		if err := iThreeFPolicy.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iThreeFPolicy.abi.Events[event].Inputs {
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

// IThreeFPolicySetMaxAssetsPerDuration represents a SetMaxAssetsPerDuration event raised by the IThreeFPolicy contract.
type IThreeFPolicySetMaxAssetsPerDuration struct {
	Asset           common.Address
	UnderlyingAsset common.Address
	MaxAssets       *big.Int
	Duration        *big.Int
	Raw             *types.Log // Blockchain specific contextual infos
}

const IThreeFPolicySetMaxAssetsPerDurationEventName = "SetMaxAssetsPerDuration"

// ContractEventName returns the user-defined event name.
func (IThreeFPolicySetMaxAssetsPerDuration) ContractEventName() string {
	return IThreeFPolicySetMaxAssetsPerDurationEventName
}

// UnpackSetMaxAssetsPerDurationEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetMaxAssetsPerDuration(address indexed asset, address indexed underlyingAsset, uint256 maxAssets, uint48 duration)
func (iThreeFPolicy *IThreeFPolicy) UnpackSetMaxAssetsPerDurationEvent(log *types.Log) (*IThreeFPolicySetMaxAssetsPerDuration, error) {
	event := "SetMaxAssetsPerDuration"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != iThreeFPolicy.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(IThreeFPolicySetMaxAssetsPerDuration)
	if len(log.Data) > 0 {
		if err := iThreeFPolicy.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iThreeFPolicy.abi.Events[event].Inputs {
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

// IThreeFPolicySetMinYieldPerRequest represents a SetMinYieldPerRequest event raised by the IThreeFPolicy contract.
type IThreeFPolicySetMinYieldPerRequest struct {
	UnderlyingAsset common.Address
	Minimum         *big.Int
	Raw             *types.Log // Blockchain specific contextual infos
}

const IThreeFPolicySetMinYieldPerRequestEventName = "SetMinYieldPerRequest"

// ContractEventName returns the user-defined event name.
func (IThreeFPolicySetMinYieldPerRequest) ContractEventName() string {
	return IThreeFPolicySetMinYieldPerRequestEventName
}

// UnpackSetMinYieldPerRequestEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetMinYieldPerRequest(address indexed underlyingAsset, uint256 minimum)
func (iThreeFPolicy *IThreeFPolicy) UnpackSetMinYieldPerRequestEvent(log *types.Log) (*IThreeFPolicySetMinYieldPerRequest, error) {
	event := "SetMinYieldPerRequest"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != iThreeFPolicy.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(IThreeFPolicySetMinYieldPerRequest)
	if len(log.Data) > 0 {
		if err := iThreeFPolicy.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iThreeFPolicy.abi.Events[event].Inputs {
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
func (iThreeFPolicy *IThreeFPolicy) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["AlreadyInitialized"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["InvalidAsset"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackInvalidAssetError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["InvalidAssets"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackInvalidAssetsError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["InvalidCaller"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackInvalidCallerError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["InvalidRequest"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackInvalidRequestError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["InvalidRequestName"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackInvalidRequestNameError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["NotFactory"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackNotFactoryError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["RequestMismatch"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackRequestMismatchError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["TooLowYield"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackTooLowYieldError(raw[4:])
	}
	if bytes.Equal(raw[:4], iThreeFPolicy.abi.Errors["UnsupportedTarget"].ID.Bytes()[:4]) {
		return iThreeFPolicy.UnpackUnsupportedTargetError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// IThreeFPolicyAlreadyInitialized represents a AlreadyInitialized error raised by the IThreeFPolicy contract.
type IThreeFPolicyAlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyInitialized()
func IThreeFPolicyAlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x0dc149f07762891dbcea3fe72770f3d63a1863fc54b2f084e8c59ec476996927")
}

// UnpackAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyInitialized()
func (iThreeFPolicy *IThreeFPolicy) UnpackAlreadyInitializedError(raw []byte) (*IThreeFPolicyAlreadyInitialized, error) {
	out := new(IThreeFPolicyAlreadyInitialized)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "AlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyInvalidAsset represents a InvalidAsset error raised by the IThreeFPolicy contract.
type IThreeFPolicyInvalidAsset struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAsset()
func IThreeFPolicyInvalidAssetErrorID() common.Hash {
	return common.HexToHash("0xc891add29a7ab9d41e37185f104bcdb2f36b82070c641cc70969e3180fe2f51a")
}

// UnpackInvalidAssetError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAsset()
func (iThreeFPolicy *IThreeFPolicy) UnpackInvalidAssetError(raw []byte) (*IThreeFPolicyInvalidAsset, error) {
	out := new(IThreeFPolicyInvalidAsset)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "InvalidAsset", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyInvalidAssets represents a InvalidAssets error raised by the IThreeFPolicy contract.
type IThreeFPolicyInvalidAssets struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAssets()
func IThreeFPolicyInvalidAssetsErrorID() common.Hash {
	return common.HexToHash("0x60dd3f448144d4e0eb37fdff3e4d839dfbcb63882f1685952bdd4da4746acf9e")
}

// UnpackInvalidAssetsError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAssets()
func (iThreeFPolicy *IThreeFPolicy) UnpackInvalidAssetsError(raw []byte) (*IThreeFPolicyInvalidAssets, error) {
	out := new(IThreeFPolicyInvalidAssets)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "InvalidAssets", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyInvalidCaller represents a InvalidCaller error raised by the IThreeFPolicy contract.
type IThreeFPolicyInvalidCaller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidCaller()
func IThreeFPolicyInvalidCallerErrorID() common.Hash {
	return common.HexToHash("0x48f5c3ed50241a1b6c87d204a25d9d01339cd768de9a714ffbb53a5bb6ad572a")
}

// UnpackInvalidCallerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidCaller()
func (iThreeFPolicy *IThreeFPolicy) UnpackInvalidCallerError(raw []byte) (*IThreeFPolicyInvalidCaller, error) {
	out := new(IThreeFPolicyInvalidCaller)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "InvalidCaller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyInvalidRequest represents a InvalidRequest error raised by the IThreeFPolicy contract.
type IThreeFPolicyInvalidRequest struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidRequest()
func IThreeFPolicyInvalidRequestErrorID() common.Hash {
	return common.HexToHash("0x41abc801f6a855c0688b235b52f8ccd8ca8f11c80e9742366ea28d37ccdd5ca2")
}

// UnpackInvalidRequestError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidRequest()
func (iThreeFPolicy *IThreeFPolicy) UnpackInvalidRequestError(raw []byte) (*IThreeFPolicyInvalidRequest, error) {
	out := new(IThreeFPolicyInvalidRequest)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "InvalidRequest", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyInvalidRequestName represents a InvalidRequestName error raised by the IThreeFPolicy contract.
type IThreeFPolicyInvalidRequestName struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidRequestName()
func IThreeFPolicyInvalidRequestNameErrorID() common.Hash {
	return common.HexToHash("0xb2c030b21b9955e1d1328eb8086b8679bf6e05ac298014d410fec421b2e21f76")
}

// UnpackInvalidRequestNameError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidRequestName()
func (iThreeFPolicy *IThreeFPolicy) UnpackInvalidRequestNameError(raw []byte) (*IThreeFPolicyInvalidRequestName, error) {
	out := new(IThreeFPolicyInvalidRequestName)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "InvalidRequestName", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyNotFactory represents a NotFactory error raised by the IThreeFPolicy contract.
type IThreeFPolicyNotFactory struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotFactory()
func IThreeFPolicyNotFactoryErrorID() common.Hash {
	return common.HexToHash("0x32cc723614e775fc4a8386492bc9a860c12fe98d5f5f28ec17e265818645b229")
}

// UnpackNotFactoryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotFactory()
func (iThreeFPolicy *IThreeFPolicy) UnpackNotFactoryError(raw []byte) (*IThreeFPolicyNotFactory, error) {
	out := new(IThreeFPolicyNotFactory)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "NotFactory", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyRequestMismatch represents a RequestMismatch error raised by the IThreeFPolicy contract.
type IThreeFPolicyRequestMismatch struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error RequestMismatch()
func IThreeFPolicyRequestMismatchErrorID() common.Hash {
	return common.HexToHash("0xba343de5ac92b526ccb4835c12bdcc8d2c0835645b4036f28a934a6532c6763b")
}

// UnpackRequestMismatchError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error RequestMismatch()
func (iThreeFPolicy *IThreeFPolicy) UnpackRequestMismatchError(raw []byte) (*IThreeFPolicyRequestMismatch, error) {
	out := new(IThreeFPolicyRequestMismatch)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "RequestMismatch", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyTooLowYield represents a TooLowYield error raised by the IThreeFPolicy contract.
type IThreeFPolicyTooLowYield struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooLowYield()
func IThreeFPolicyTooLowYieldErrorID() common.Hash {
	return common.HexToHash("0xec84af7bf6cfbe9482148973e3ddd1942a9f2808b5f046694f7cfe46aa2ce953")
}

// UnpackTooLowYieldError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooLowYield()
func (iThreeFPolicy *IThreeFPolicy) UnpackTooLowYieldError(raw []byte) (*IThreeFPolicyTooLowYield, error) {
	out := new(IThreeFPolicyTooLowYield)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "TooLowYield", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IThreeFPolicyUnsupportedTarget represents a UnsupportedTarget error raised by the IThreeFPolicy contract.
type IThreeFPolicyUnsupportedTarget struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error UnsupportedTarget()
func IThreeFPolicyUnsupportedTargetErrorID() common.Hash {
	return common.HexToHash("0x778b288ae0dc0fbe4e54c1d096d582bd4a8284c4ee9335abe017393dbbe5cff2")
}

// UnpackUnsupportedTargetError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error UnsupportedTarget()
func (iThreeFPolicy *IThreeFPolicy) UnpackUnsupportedTargetError(raw []byte) (*IThreeFPolicyUnsupportedTarget, error) {
	out := new(IThreeFPolicyUnsupportedTarget)
	if err := iThreeFPolicy.abi.UnpackIntoInterface(out, "UnsupportedTarget", raw); err != nil {
		return nil, err
	}
	return out, nil
}
