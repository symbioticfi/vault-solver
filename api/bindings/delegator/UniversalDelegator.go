// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package delegator

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

// UniversalDelegatorMetaData contains all meta data concerning the UniversalDelegator contract.
var UniversalDelegatorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"limitOf\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"}]",
	ID:  "UniversalDelegator",
}

// UniversalDelegator is an auto generated Go binding around an Ethereum contract.
type UniversalDelegator struct {
	abi abi.ABI
}

// NewUniversalDelegator creates a new instance of UniversalDelegator.
func NewUniversalDelegator() *UniversalDelegator {
	parsed, err := UniversalDelegatorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &UniversalDelegator{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *UniversalDelegator) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackLimitOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x546a2ca4.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function limitOf(address adapter) view returns(uint256)
func (universalDelegator *UniversalDelegator) PackLimitOf(adapter common.Address) []byte {
	enc, err := universalDelegator.abi.Pack("limitOf", adapter)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLimitOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x546a2ca4.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function limitOf(address adapter) view returns(uint256)
func (universalDelegator *UniversalDelegator) TryPackLimitOf(adapter common.Address) ([]byte, error) {
	return universalDelegator.abi.Pack("limitOf", adapter)
}

// UnpackLimitOf is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x546a2ca4.
//
// Solidity: function limitOf(address adapter) view returns(uint256)
func (universalDelegator *UniversalDelegator) UnpackLimitOf(data []byte) (*big.Int, error) {
	out, err := universalDelegator.abi.Unpack("limitOf", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
