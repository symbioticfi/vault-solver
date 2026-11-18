// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package underlyingasset

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

// IThreeFUnderlyingAssetMetaData contains all meta data concerning the IThreeFUnderlyingAsset contract.
var IThreeFUnderlyingAssetMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"underlying\",\"inputs\":[],\"outputs\":[{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"}]",
	ID:  "IThreeFUnderlyingAsset",
}

// IThreeFUnderlyingAsset is an auto generated Go binding around an Ethereum contract.
type IThreeFUnderlyingAsset struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *IThreeFUnderlyingAsset) GetABI() abi.ABI {
	return c.abi
}

// NewIThreeFUnderlyingAsset creates a new instance of IThreeFUnderlyingAsset.
func NewIThreeFUnderlyingAsset() *IThreeFUnderlyingAsset {
	parsed, err := IThreeFUnderlyingAssetMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IThreeFUnderlyingAsset{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IThreeFUnderlyingAsset) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackUnderlying is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6f307dc3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function underlying() view returns(address asset)
func (iThreeFUnderlyingAsset *IThreeFUnderlyingAsset) PackUnderlying() []byte {
	enc, err := iThreeFUnderlyingAsset.abi.Pack("underlying")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackUnderlying is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6f307dc3.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function underlying() view returns(address asset)
func (iThreeFUnderlyingAsset *IThreeFUnderlyingAsset) TryPackUnderlying() ([]byte, error) {
	return iThreeFUnderlyingAsset.abi.Pack("underlying")
}

// UnpackUnderlying is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6f307dc3.
//
// Solidity: function underlying() view returns(address asset)
func (iThreeFUnderlyingAsset *IThreeFUnderlyingAsset) UnpackUnderlying(data []byte) (common.Address, error) {
	out, err := iThreeFUnderlyingAsset.abi.Unpack("underlying", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}
