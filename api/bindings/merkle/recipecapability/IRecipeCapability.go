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
	ABI: "[{\"type\":\"function\",\"name\":\"execute\",\"inputs\":[{\"name\":\"queries\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"},{\"name\":\"steps\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"},{\"name\":\"inputs\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"outputs\":[{\"name\":\"values\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"error\",\"name\":\"CheckFailed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOperation\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidQuery\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidReference\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidRuntime\",\"inputs\":[]}]",
	ID:  "IRecipeCapability",
}

// IRecipeCapability is an auto generated Go binding around an Ethereum contract.
type IRecipeCapability struct {
	abi abi.ABI
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

// UnpackError attempts to decode the provided error data using user-defined
// error definitions.
func (iRecipeCapability *IRecipeCapability) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], iRecipeCapability.abi.Errors["CheckFailed"].ID.Bytes()[:4]) {
		return iRecipeCapability.UnpackCheckFailedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iRecipeCapability.abi.Errors["InvalidCall"].ID.Bytes()[:4]) {
		return iRecipeCapability.UnpackInvalidCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], iRecipeCapability.abi.Errors["InvalidCaller"].ID.Bytes()[:4]) {
		return iRecipeCapability.UnpackInvalidCallerError(raw[4:])
	}
	if bytes.Equal(raw[:4], iRecipeCapability.abi.Errors["InvalidOperation"].ID.Bytes()[:4]) {
		return iRecipeCapability.UnpackInvalidOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], iRecipeCapability.abi.Errors["InvalidQuery"].ID.Bytes()[:4]) {
		return iRecipeCapability.UnpackInvalidQueryError(raw[4:])
	}
	if bytes.Equal(raw[:4], iRecipeCapability.abi.Errors["InvalidReference"].ID.Bytes()[:4]) {
		return iRecipeCapability.UnpackInvalidReferenceError(raw[4:])
	}
	if bytes.Equal(raw[:4], iRecipeCapability.abi.Errors["InvalidRuntime"].ID.Bytes()[:4]) {
		return iRecipeCapability.UnpackInvalidRuntimeError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// IRecipeCapabilityCheckFailed represents a CheckFailed error raised by the IRecipeCapability contract.
type IRecipeCapabilityCheckFailed struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error CheckFailed()
func IRecipeCapabilityCheckFailedErrorID() common.Hash {
	return common.HexToHash("0xd7c1c92f5be804090974ee16e347e0dcf86078058f5840e4a58018e93693ce2b")
}

// UnpackCheckFailedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error CheckFailed()
func (iRecipeCapability *IRecipeCapability) UnpackCheckFailedError(raw []byte) (*IRecipeCapabilityCheckFailed, error) {
	out := new(IRecipeCapabilityCheckFailed)
	if err := iRecipeCapability.abi.UnpackIntoInterface(out, "CheckFailed", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IRecipeCapabilityInvalidCall represents a InvalidCall error raised by the IRecipeCapability contract.
type IRecipeCapabilityInvalidCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidCall()
func IRecipeCapabilityInvalidCallErrorID() common.Hash {
	return common.HexToHash("0xae962d4e06a5f2158f022ce0e8331adb6b53d60eb043c7902e2eaa3586c6a4fb")
}

// UnpackInvalidCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidCall()
func (iRecipeCapability *IRecipeCapability) UnpackInvalidCallError(raw []byte) (*IRecipeCapabilityInvalidCall, error) {
	out := new(IRecipeCapabilityInvalidCall)
	if err := iRecipeCapability.abi.UnpackIntoInterface(out, "InvalidCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IRecipeCapabilityInvalidCaller represents a InvalidCaller error raised by the IRecipeCapability contract.
type IRecipeCapabilityInvalidCaller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidCaller()
func IRecipeCapabilityInvalidCallerErrorID() common.Hash {
	return common.HexToHash("0x48f5c3ed50241a1b6c87d204a25d9d01339cd768de9a714ffbb53a5bb6ad572a")
}

// UnpackInvalidCallerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidCaller()
func (iRecipeCapability *IRecipeCapability) UnpackInvalidCallerError(raw []byte) (*IRecipeCapabilityInvalidCaller, error) {
	out := new(IRecipeCapabilityInvalidCaller)
	if err := iRecipeCapability.abi.UnpackIntoInterface(out, "InvalidCaller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IRecipeCapabilityInvalidOperation represents a InvalidOperation error raised by the IRecipeCapability contract.
type IRecipeCapabilityInvalidOperation struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidOperation()
func IRecipeCapabilityInvalidOperationErrorID() common.Hash {
	return common.HexToHash("0x398d4d32963991cf02978f96f1a4d81787345d957c0995bedfe70af66ce594fd")
}

// UnpackInvalidOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidOperation()
func (iRecipeCapability *IRecipeCapability) UnpackInvalidOperationError(raw []byte) (*IRecipeCapabilityInvalidOperation, error) {
	out := new(IRecipeCapabilityInvalidOperation)
	if err := iRecipeCapability.abi.UnpackIntoInterface(out, "InvalidOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IRecipeCapabilityInvalidQuery represents a InvalidQuery error raised by the IRecipeCapability contract.
type IRecipeCapabilityInvalidQuery struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidQuery()
func IRecipeCapabilityInvalidQueryErrorID() common.Hash {
	return common.HexToHash("0x19e4cb1093499c0994f1c698c47ae2ed09e8ac37618f650c826923074fd6d526")
}

// UnpackInvalidQueryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidQuery()
func (iRecipeCapability *IRecipeCapability) UnpackInvalidQueryError(raw []byte) (*IRecipeCapabilityInvalidQuery, error) {
	out := new(IRecipeCapabilityInvalidQuery)
	if err := iRecipeCapability.abi.UnpackIntoInterface(out, "InvalidQuery", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IRecipeCapabilityInvalidReference represents a InvalidReference error raised by the IRecipeCapability contract.
type IRecipeCapabilityInvalidReference struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidReference()
func IRecipeCapabilityInvalidReferenceErrorID() common.Hash {
	return common.HexToHash("0x5f41ff9294f1a1a14da42dbeef661318e70ed969230837ec353d3d643d6946e1")
}

// UnpackInvalidReferenceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidReference()
func (iRecipeCapability *IRecipeCapability) UnpackInvalidReferenceError(raw []byte) (*IRecipeCapabilityInvalidReference, error) {
	out := new(IRecipeCapabilityInvalidReference)
	if err := iRecipeCapability.abi.UnpackIntoInterface(out, "InvalidReference", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IRecipeCapabilityInvalidRuntime represents a InvalidRuntime error raised by the IRecipeCapability contract.
type IRecipeCapabilityInvalidRuntime struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidRuntime()
func IRecipeCapabilityInvalidRuntimeErrorID() common.Hash {
	return common.HexToHash("0x02e22e6c69a39fc2d7cb34bccdcd940b08139115bb1d4d1dc335d8086d0dccde")
}

// UnpackInvalidRuntimeError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidRuntime()
func (iRecipeCapability *IRecipeCapability) UnpackInvalidRuntimeError(raw []byte) (*IRecipeCapabilityInvalidRuntime, error) {
	out := new(IRecipeCapabilityInvalidRuntime)
	if err := iRecipeCapability.abi.UnpackIntoInterface(out, "InvalidRuntime", raw); err != nil {
		return nil, err
	}
	return out, nil
}
