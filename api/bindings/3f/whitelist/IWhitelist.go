// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package whitelist

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

// IWhitelistMetaData contains all meta data concerning the IWhitelist contract.
var IWhitelistMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"isWhitelisted\",\"inputs\":[{\"name\":\"a\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"enumIWhitelist.WhitelistStatus\"}],\"stateMutability\":\"view\"}]",
	ID:  "IWhitelist",
}

// IWhitelist is an auto generated Go binding around an Ethereum contract.
type IWhitelist struct {
	abi abi.ABI
}

// NewIWhitelist creates a new instance of IWhitelist.
func NewIWhitelist() *IWhitelist {
	parsed, err := IWhitelistMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IWhitelist{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IWhitelist) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackIsWhitelisted is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3af32abf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isWhitelisted(address a) view returns(uint8)
func (iWhitelist *IWhitelist) PackIsWhitelisted(a common.Address) []byte {
	enc, err := iWhitelist.abi.Pack("isWhitelisted", a)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsWhitelisted is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3af32abf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isWhitelisted(address a) view returns(uint8)
func (iWhitelist *IWhitelist) TryPackIsWhitelisted(a common.Address) ([]byte, error) {
	return iWhitelist.abi.Pack("isWhitelisted", a)
}

// UnpackIsWhitelisted is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x3af32abf.
//
// Solidity: function isWhitelisted(address a) view returns(uint8)
func (iWhitelist *IWhitelist) UnpackIsWhitelisted(data []byte) (uint8, error) {
	out, err := iWhitelist.abi.Unpack("isWhitelisted", data)
	if err != nil {
		return *new(uint8), err
	}
	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)
	return out0, nil
}
