// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package solverdelegate

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

// Solver7702DelegateMetaData contains all meta data concerning the Solver7702Delegate contract.
var Solver7702DelegateMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"approvedCallers\",\"type\":\"address[5]\",\"internalType\":\"address[5]\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"fallback\",\"stateMutability\":\"payable\"},{\"type\":\"error\",\"name\":\"Unauthorized\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
	ID:  "Solver7702Delegate",
}

// Solver7702Delegate is an auto generated Go binding around an Ethereum contract.
type Solver7702Delegate struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *Solver7702Delegate) GetABI() abi.ABI {
	return c.abi
}

// NewSolver7702Delegate creates a new instance of Solver7702Delegate.
func NewSolver7702Delegate() *Solver7702Delegate {
	parsed, err := Solver7702DelegateMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &Solver7702Delegate{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *Solver7702Delegate) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address[5] approvedCallers) returns()
func (solver7702Delegate *Solver7702Delegate) PackConstructor(approvedCallers [5]common.Address) []byte {
	enc, err := solver7702Delegate.abi.Pack("", approvedCallers)
	if err != nil {
		panic(err)
	}
	return enc
}

// UnpackError attempts to decode the provided error data using user-defined
// error definitions.
func (solver7702Delegate *Solver7702Delegate) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], solver7702Delegate.abi.Errors["Unauthorized"].ID.Bytes()[:4]) {
		return solver7702Delegate.UnpackUnauthorizedError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// Solver7702DelegateUnauthorized represents a Unauthorized error raised by the Solver7702Delegate contract.
type Solver7702DelegateUnauthorized struct {
	Sender common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error Unauthorized(address sender)
func Solver7702DelegateUnauthorizedErrorID() common.Hash {
	return common.HexToHash("0x8e4a23d6a5d81f013eca4bc92aeb9214ccafcaebd1f097c350c922d6e19122d5")
}

// UnpackUnauthorizedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error Unauthorized(address sender)
func (solver7702Delegate *Solver7702Delegate) UnpackUnauthorizedError(raw []byte) (*Solver7702DelegateUnauthorized, error) {
	out := new(Solver7702DelegateUnauthorized)
	if err := solver7702Delegate.abi.UnpackIntoInterface(out, "Unauthorized", raw); err != nil {
		return nil, err
	}
	return out, nil
}
