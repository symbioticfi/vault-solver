// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package oracle

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

// MorphoOracleMetaData contains all meta data concerning the MorphoOracle contract.
var MorphoOracleMetaData = bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"price\",\"outputs\":[{\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	ID:  "MorphoOracle",
}

// MorphoOracle is an auto generated Go binding around an Ethereum contract.
type MorphoOracle struct {
	abi abi.ABI
}

// NewMorphoOracle creates a new instance of MorphoOracle.
func NewMorphoOracle() *MorphoOracle {
	parsed, err := MorphoOracleMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &MorphoOracle{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *MorphoOracle) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackPrice is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa035b1fe.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function price() view returns(uint256)
func (morphoOracle *MorphoOracle) PackPrice() []byte {
	enc, err := morphoOracle.abi.Pack("price")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPrice is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa035b1fe.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function price() view returns(uint256)
func (morphoOracle *MorphoOracle) TryPackPrice() ([]byte, error) {
	return morphoOracle.abi.Pack("price")
}

// UnpackPrice is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xa035b1fe.
//
// Solidity: function price() view returns(uint256)
func (morphoOracle *MorphoOracle) UnpackPrice(data []byte) (*big.Int, error) {
	out, err := morphoOracle.abi.Unpack("price", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
