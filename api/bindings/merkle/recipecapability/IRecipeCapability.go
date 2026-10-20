// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package recipecapability

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

// IRecipeCapabilityMetaData contains all meta data concerning the IRecipeCapability contract.
var IRecipeCapabilityMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"execute\",\"inputs\":[{\"name\":\"queries\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"},{\"name\":\"steps\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"},{\"name\":\"inputs\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"outputs\":[{\"name\":\"values\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"nonpayable\"}]",
	ID:  "IRecipeCapability",
}

// IRecipeCapability is an auto generated Go binding around an Ethereum contract.
type IRecipeCapability struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *IRecipeCapability) GetABI() abi.ABI {
	return c.abi
}

// NewIRecipeCapability creates a new instance of IRecipeCapability.
func NewIRecipeCapability() *IRecipeCapability {
	parsed, err := IRecipeCapabilityMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IRecipeCapability{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IRecipeCapability) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackExecute is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc3b55944.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function execute(bytes[] queries, bytes[] steps, uint256[] inputs) returns(uint256[] values)
func (iRecipeCapability *IRecipeCapability) PackExecute(queries [][]byte, steps [][]byte, inputs []*big.Int) []byte {
	enc, err := iRecipeCapability.abi.Pack("execute", queries, steps, inputs)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackExecute is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc3b55944.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function execute(bytes[] queries, bytes[] steps, uint256[] inputs) returns(uint256[] values)
func (iRecipeCapability *IRecipeCapability) TryPackExecute(queries [][]byte, steps [][]byte, inputs []*big.Int) ([]byte, error) {
	return iRecipeCapability.abi.Pack("execute", queries, steps, inputs)
}

// UnpackExecute is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc3b55944.
//
// Solidity: function execute(bytes[] queries, bytes[] steps, uint256[] inputs) returns(uint256[] values)
func (iRecipeCapability *IRecipeCapability) UnpackExecute(data []byte) ([]*big.Int, error) {
	out, err := iRecipeCapability.abi.Unpack("execute", data)
	if err != nil {
		return *new([]*big.Int), err
	}
	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)
	return out0, nil
}
