// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package multicall3

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

// Multicall3Call3 is an auto generated low-level Go binding around an user-defined struct.
type Multicall3Call3 struct {
	Target       common.Address
	AllowFailure bool
	CallData     []byte
}

// Multicall3Result is an auto generated low-level Go binding around an user-defined struct.
type Multicall3Result struct {
	Success    bool
	ReturnData []byte
}

// Multicall3MetaData contains all meta data concerning the Multicall3 contract.
var Multicall3MetaData = bind.MetaData{
	ABI: "[{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"allowFailure\",\"type\":\"bool\"},{\"internalType\":\"bytes\",\"name\":\"callData\",\"type\":\"bytes\"}],\"internalType\":\"structMulticall3.Call3[]\",\"name\":\"calls\",\"type\":\"tuple[]\"}],\"name\":\"aggregate3\",\"outputs\":[{\"components\":[{\"internalType\":\"bool\",\"name\":\"success\",\"type\":\"bool\"},{\"internalType\":\"bytes\",\"name\":\"returnData\",\"type\":\"bytes\"}],\"internalType\":\"structMulticall3.Result[]\",\"name\":\"returnData\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	ID:  "Multicall3",
}

// Multicall3 is an auto generated Go binding around an Ethereum contract.
type Multicall3 struct {
	abi abi.ABI
}

// NewMulticall3 creates a new instance of Multicall3.
func NewMulticall3() *Multicall3 {
	parsed, err := Multicall3MetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &Multicall3{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *Multicall3) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackAggregate3 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x82ad56cb.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function aggregate3((address,bool,bytes)[] calls) view returns((bool,bytes)[] returnData)
func (multicall3 *Multicall3) PackAggregate3(calls []Multicall3Call3) []byte {
	enc, err := multicall3.abi.Pack("aggregate3", calls)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAggregate3 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x82ad56cb.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function aggregate3((address,bool,bytes)[] calls) view returns((bool,bytes)[] returnData)
func (multicall3 *Multicall3) TryPackAggregate3(calls []Multicall3Call3) ([]byte, error) {
	return multicall3.abi.Pack("aggregate3", calls)
}

// UnpackAggregate3 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x82ad56cb.
//
// Solidity: function aggregate3((address,bool,bytes)[] calls) view returns((bool,bytes)[] returnData)
func (multicall3 *Multicall3) UnpackAggregate3(data []byte) ([]Multicall3Result, error) {
	out, err := multicall3.abi.Unpack("aggregate3", data)
	if err != nil {
		return *new([]Multicall3Result), err
	}
	out0 := *abi.ConvertType(out[0], new([]Multicall3Result)).(*[]Multicall3Result)
	return out0, nil
}
