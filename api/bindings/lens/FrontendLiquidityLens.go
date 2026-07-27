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

// FrontendLiquidityLensMetaData contains all meta data concerning the FrontendLiquidityLens contract.
var FrontendLiquidityLensMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[{\"internalType\":\"address\",\"name\":\"adapter\",\"type\":\"address\"}],\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[{\"internalType\":\"address\",\"name\":\"adapter\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"tokenToRedeem\",\"type\":\"address\"}],\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\"}]",
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
