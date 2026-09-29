// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package reactor

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

// V2DutchOrderReactorMetaData contains all meta data concerning the V2DutchOrderReactor contract.
var V2DutchOrderReactorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"permit2\",\"stateMutability\":\"view\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIPermit2\"}]}]",
	ID:  "V2DutchOrderReactor",
}

// V2DutchOrderReactor is an auto generated Go binding around an Ethereum contract.
type V2DutchOrderReactor struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *V2DutchOrderReactor) GetABI() abi.ABI {
	return c.abi
}

// NewV2DutchOrderReactor creates a new instance of V2DutchOrderReactor.
func NewV2DutchOrderReactor() *V2DutchOrderReactor {
	parsed, err := V2DutchOrderReactorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &V2DutchOrderReactor{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *V2DutchOrderReactor) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackPermit2 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x12261ee7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function permit2() view returns(address)
func (v2DutchOrderReactor *V2DutchOrderReactor) PackPermit2() []byte {
	enc, err := v2DutchOrderReactor.abi.Pack("permit2")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPermit2 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x12261ee7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function permit2() view returns(address)
func (v2DutchOrderReactor *V2DutchOrderReactor) TryPackPermit2() ([]byte, error) {
	return v2DutchOrderReactor.abi.Pack("permit2")
}

// UnpackPermit2 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x12261ee7.
//
// Solidity: function permit2() view returns(address)
func (v2DutchOrderReactor *V2DutchOrderReactor) UnpackPermit2(data []byte) (common.Address, error) {
	out, err := v2DutchOrderReactor.abi.Unpack("permit2", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}
