// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package permit2

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

// Permit2MetaData contains all meta data concerning the Permit2 contract.
var Permit2MetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"nonceBitmap\",\"stateMutability\":\"view\",\"inputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
	ID:  "Permit2",
}

// Permit2 is an auto generated Go binding around an Ethereum contract.
type Permit2 struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *Permit2) GetABI() abi.ABI {
	return c.abi
}

// NewPermit2 creates a new instance of Permit2.
func NewPermit2() *Permit2 {
	parsed, err := Permit2MetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &Permit2{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *Permit2) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackNonceBitmap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4fe02b44.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function nonceBitmap(address , uint256 ) view returns(uint256)
func (permit2 *Permit2) PackNonceBitmap(arg0 common.Address, arg1 *big.Int) []byte {
	enc, err := permit2.abi.Pack("nonceBitmap", arg0, arg1)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackNonceBitmap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4fe02b44.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function nonceBitmap(address , uint256 ) view returns(uint256)
func (permit2 *Permit2) TryPackNonceBitmap(arg0 common.Address, arg1 *big.Int) ([]byte, error) {
	return permit2.abi.Pack("nonceBitmap", arg0, arg1)
}

// UnpackNonceBitmap is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x4fe02b44.
//
// Solidity: function nonceBitmap(address , uint256 ) view returns(uint256)
func (permit2 *Permit2) UnpackNonceBitmap(data []byte) (*big.Int, error) {
	out, err := permit2.abi.Unpack("nonceBitmap", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
