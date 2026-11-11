// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package lens

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

// FrontendLiquidityLensMaxAssetsResult is an auto generated low-level Go binding around an user-defined struct.
type FrontendLiquidityLensMaxAssetsResult struct {
	Success   bool
	MaxAssets *big.Int
}

// FrontendLiquidityLensMetaData contains all meta data concerning the FrontendLiquidityLens contract.
var FrontendLiquidityLensMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getMaxAssetsBatch\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokensToRedeem\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[{\"name\":\"results\",\"type\":\"tuple[]\",\"internalType\":\"structFrontendLiquidityLens.MaxAssetsResult[]\",\"components\":[{\"name\":\"success\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"maxAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getTokenMaxAssets\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"maxAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"NotSelf\",\"inputs\":[]}]",
	ID:  "FrontendLiquidityLens",
}

// FrontendLiquidityLens is an auto generated Go binding around an Ethereum contract.
type FrontendLiquidityLens struct {
	abi abi.ABI
}

// NewFrontendLiquidityLens creates a new instance of FrontendLiquidityLens.
func NewFrontendLiquidityLens() *FrontendLiquidityLens {
	parsed, err := FrontendLiquidityLensMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &FrontendLiquidityLens{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *FrontendLiquidityLens) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackGetMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x22135549.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getMaxAssets(address adapter) returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) PackGetMaxAssets(adapter common.Address) []byte {
	enc, err := frontendLiquidityLens.abi.Pack("getMaxAssets", adapter)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x22135549.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getMaxAssets(address adapter) returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) TryPackGetMaxAssets(adapter common.Address) ([]byte, error) {
	return frontendLiquidityLens.abi.Pack("getMaxAssets", adapter)
}

// UnpackGetMaxAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x22135549.
//
// Solidity: function getMaxAssets(address adapter) returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) UnpackGetMaxAssets(data []byte) (*big.Int, error) {
	out, err := frontendLiquidityLens.abi.Unpack("getMaxAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackGetMaxAssets0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x291a304c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getMaxAssets(address adapter, address tokenToRedeem) returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) PackGetMaxAssets0(adapter common.Address, tokenToRedeem common.Address) []byte {
	enc, err := frontendLiquidityLens.abi.Pack("getMaxAssets0", adapter, tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetMaxAssets0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x291a304c.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getMaxAssets(address adapter, address tokenToRedeem) returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) TryPackGetMaxAssets0(adapter common.Address, tokenToRedeem common.Address) ([]byte, error) {
	return frontendLiquidityLens.abi.Pack("getMaxAssets0", adapter, tokenToRedeem)
}

// UnpackGetMaxAssets0 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x291a304c.
//
// Solidity: function getMaxAssets(address adapter, address tokenToRedeem) returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) UnpackGetMaxAssets0(data []byte) (*big.Int, error) {
	out, err := frontendLiquidityLens.abi.Unpack("getMaxAssets0", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackGetMaxAssetsBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4ab500c6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getMaxAssetsBatch(address adapter, address[] tokensToRedeem) returns((bool,uint256)[] results)
func (frontendLiquidityLens *FrontendLiquidityLens) PackGetMaxAssetsBatch(adapter common.Address, tokensToRedeem []common.Address) []byte {
	enc, err := frontendLiquidityLens.abi.Pack("getMaxAssetsBatch", adapter, tokensToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetMaxAssetsBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4ab500c6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getMaxAssetsBatch(address adapter, address[] tokensToRedeem) returns((bool,uint256)[] results)
func (frontendLiquidityLens *FrontendLiquidityLens) TryPackGetMaxAssetsBatch(adapter common.Address, tokensToRedeem []common.Address) ([]byte, error) {
	return frontendLiquidityLens.abi.Pack("getMaxAssetsBatch", adapter, tokensToRedeem)
}

// UnpackGetMaxAssetsBatch is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x4ab500c6.
//
// Solidity: function getMaxAssetsBatch(address adapter, address[] tokensToRedeem) returns((bool,uint256)[] results)
func (frontendLiquidityLens *FrontendLiquidityLens) UnpackGetMaxAssetsBatch(data []byte) ([]FrontendLiquidityLensMaxAssetsResult, error) {
	out, err := frontendLiquidityLens.abi.Unpack("getMaxAssetsBatch", data)
	if err != nil {
		return *new([]FrontendLiquidityLensMaxAssetsResult), err
	}
	out0 := *abi.ConvertType(out[0], new([]FrontendLiquidityLensMaxAssetsResult)).(*[]FrontendLiquidityLensMaxAssetsResult)
	return out0, nil
}

// PackGetTokenMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x72590c04.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getTokenMaxAssets(address adapter, address tokenToRedeem, uint256 maxAssets) view returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) PackGetTokenMaxAssets(adapter common.Address, tokenToRedeem common.Address, maxAssets *big.Int) []byte {
	enc, err := frontendLiquidityLens.abi.Pack("getTokenMaxAssets", adapter, tokenToRedeem, maxAssets)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetTokenMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x72590c04.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getTokenMaxAssets(address adapter, address tokenToRedeem, uint256 maxAssets) view returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) TryPackGetTokenMaxAssets(adapter common.Address, tokenToRedeem common.Address, maxAssets *big.Int) ([]byte, error) {
	return frontendLiquidityLens.abi.Pack("getTokenMaxAssets", adapter, tokenToRedeem, maxAssets)
}

// UnpackGetTokenMaxAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x72590c04.
//
// Solidity: function getTokenMaxAssets(address adapter, address tokenToRedeem, uint256 maxAssets) view returns(uint256)
func (frontendLiquidityLens *FrontendLiquidityLens) UnpackGetTokenMaxAssets(data []byte) (*big.Int, error) {
	out, err := frontendLiquidityLens.abi.Unpack("getTokenMaxAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// UnpackError attempts to decode the provided error data using user-defined
// error definitions.
func (frontendLiquidityLens *FrontendLiquidityLens) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], frontendLiquidityLens.abi.Errors["NotSelf"].ID.Bytes()[:4]) {
		return frontendLiquidityLens.UnpackNotSelfError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// FrontendLiquidityLensNotSelf represents a NotSelf error raised by the FrontendLiquidityLens contract.
type FrontendLiquidityLensNotSelf struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotSelf()
func FrontendLiquidityLensNotSelfErrorID() common.Hash {
	return common.HexToHash("0x29c3b7ee14a7adaa7fba7b043229509fe963f719303620e91e88c61c6b12de34")
}

// UnpackNotSelfError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotSelf()
func (frontendLiquidityLens *FrontendLiquidityLens) UnpackNotSelfError(raw []byte) (*FrontendLiquidityLensNotSelf, error) {
	out := new(FrontendLiquidityLensNotSelf)
	if err := frontendLiquidityLens.abi.UnpackIntoInterface(out, "NotSelf", raw); err != nil {
		return nil, err
	}
	return out, nil
}
