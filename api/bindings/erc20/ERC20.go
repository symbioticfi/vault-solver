// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package erc20

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

// ERC20MetaData contains all meta data concerning the ERC20 contract.
var ERC20MetaData = bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"allowance\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"needed\",\"type\":\"uint256\"}],\"name\":\"ERC20InsufficientAllowance\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"balance\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"needed\",\"type\":\"uint256\"}],\"name\":\"ERC20InsufficientBalance\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"approver\",\"type\":\"address\"}],\"name\":\"ERC20InvalidApprover\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"receiver\",\"type\":\"address\"}],\"name\":\"ERC20InvalidReceiver\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"}],\"name\":\"ERC20InvalidSender\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"}],\"name\":\"ERC20InvalidSpender\",\"type\":\"error\"}]",
	ID:  "ERC20",
}

// ERC20 is an auto generated Go binding around an Ethereum contract.
type ERC20 struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *ERC20) GetABI() abi.ABI {
	return c.abi
}

// NewERC20 creates a new instance of ERC20.
func NewERC20() *ERC20 {
	parsed, err := ERC20MetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &ERC20{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *ERC20) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function decimals() view returns(uint8)
func (eRC20 *ERC20) PackDecimals() []byte {
	enc, err := eRC20.abi.Pack("decimals")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function decimals() view returns(uint8)
func (eRC20 *ERC20) TryPackDecimals() ([]byte, error) {
	return eRC20.abi.Pack("decimals")
}

// UnpackDecimals is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (eRC20 *ERC20) UnpackDecimals(data []byte) (uint8, error) {
	out, err := eRC20.abi.Unpack("decimals", data)
	if err != nil {
		return *new(uint8), err
	}
	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)
	return out0, nil
}

// UnpackError attempts to decode the provided error data using user-defined
// error definitions.
func (eRC20 *ERC20) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], eRC20.abi.Errors["ERC20InsufficientAllowance"].ID.Bytes()[:4]) {
		return eRC20.UnpackERC20InsufficientAllowanceError(raw[4:])
	}
	if bytes.Equal(raw[:4], eRC20.abi.Errors["ERC20InsufficientBalance"].ID.Bytes()[:4]) {
		return eRC20.UnpackERC20InsufficientBalanceError(raw[4:])
	}
	if bytes.Equal(raw[:4], eRC20.abi.Errors["ERC20InvalidApprover"].ID.Bytes()[:4]) {
		return eRC20.UnpackERC20InvalidApproverError(raw[4:])
	}
	if bytes.Equal(raw[:4], eRC20.abi.Errors["ERC20InvalidReceiver"].ID.Bytes()[:4]) {
		return eRC20.UnpackERC20InvalidReceiverError(raw[4:])
	}
	if bytes.Equal(raw[:4], eRC20.abi.Errors["ERC20InvalidSender"].ID.Bytes()[:4]) {
		return eRC20.UnpackERC20InvalidSenderError(raw[4:])
	}
	if bytes.Equal(raw[:4], eRC20.abi.Errors["ERC20InvalidSpender"].ID.Bytes()[:4]) {
		return eRC20.UnpackERC20InvalidSpenderError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// ERC20ERC20InsufficientAllowance represents a ERC20InsufficientAllowance error raised by the ERC20 contract.
type ERC20ERC20InsufficientAllowance struct {
	Spender   common.Address
	Allowance *big.Int
	Needed    *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ERC20InsufficientAllowance(address spender, uint256 allowance, uint256 needed)
func ERC20ERC20InsufficientAllowanceErrorID() common.Hash {
	return common.HexToHash("0xfb8f41b23e99d2101d86da76cdfa87dd51c82ed07d3cb62cbc473e469dbc75c3")
}

// UnpackERC20InsufficientAllowanceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ERC20InsufficientAllowance(address spender, uint256 allowance, uint256 needed)
func (eRC20 *ERC20) UnpackERC20InsufficientAllowanceError(raw []byte) (*ERC20ERC20InsufficientAllowance, error) {
	out := new(ERC20ERC20InsufficientAllowance)
	if err := eRC20.abi.UnpackIntoInterface(out, "ERC20InsufficientAllowance", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ERC20ERC20InsufficientBalance represents a ERC20InsufficientBalance error raised by the ERC20 contract.
type ERC20ERC20InsufficientBalance struct {
	Sender  common.Address
	Balance *big.Int
	Needed  *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ERC20InsufficientBalance(address sender, uint256 balance, uint256 needed)
func ERC20ERC20InsufficientBalanceErrorID() common.Hash {
	return common.HexToHash("0xe450d38cd8d9f7d95077d567d60ed49c7254716e6ad08fc9872816c97e0ffec6")
}

// UnpackERC20InsufficientBalanceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ERC20InsufficientBalance(address sender, uint256 balance, uint256 needed)
func (eRC20 *ERC20) UnpackERC20InsufficientBalanceError(raw []byte) (*ERC20ERC20InsufficientBalance, error) {
	out := new(ERC20ERC20InsufficientBalance)
	if err := eRC20.abi.UnpackIntoInterface(out, "ERC20InsufficientBalance", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ERC20ERC20InvalidApprover represents a ERC20InvalidApprover error raised by the ERC20 contract.
type ERC20ERC20InvalidApprover struct {
	Approver common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ERC20InvalidApprover(address approver)
func ERC20ERC20InvalidApproverErrorID() common.Hash {
	return common.HexToHash("0xe602df05cc75712490294c6c104ab7c17f4030363910a7a2626411c6d3118847")
}

// UnpackERC20InvalidApproverError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ERC20InvalidApprover(address approver)
func (eRC20 *ERC20) UnpackERC20InvalidApproverError(raw []byte) (*ERC20ERC20InvalidApprover, error) {
	out := new(ERC20ERC20InvalidApprover)
	if err := eRC20.abi.UnpackIntoInterface(out, "ERC20InvalidApprover", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ERC20ERC20InvalidReceiver represents a ERC20InvalidReceiver error raised by the ERC20 contract.
type ERC20ERC20InvalidReceiver struct {
	Receiver common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ERC20InvalidReceiver(address receiver)
func ERC20ERC20InvalidReceiverErrorID() common.Hash {
	return common.HexToHash("0xec442f055133b72f3b2f9f0bb351c406b178527de2040a7d1feb4e058771f613")
}

// UnpackERC20InvalidReceiverError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ERC20InvalidReceiver(address receiver)
func (eRC20 *ERC20) UnpackERC20InvalidReceiverError(raw []byte) (*ERC20ERC20InvalidReceiver, error) {
	out := new(ERC20ERC20InvalidReceiver)
	if err := eRC20.abi.UnpackIntoInterface(out, "ERC20InvalidReceiver", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ERC20ERC20InvalidSender represents a ERC20InvalidSender error raised by the ERC20 contract.
type ERC20ERC20InvalidSender struct {
	Sender common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ERC20InvalidSender(address sender)
func ERC20ERC20InvalidSenderErrorID() common.Hash {
	return common.HexToHash("0x96c6fd1edd0cd6ef7ff0ecc0facdf53148dc0048b57fe58af65755250a7a96bd")
}

// UnpackERC20InvalidSenderError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ERC20InvalidSender(address sender)
func (eRC20 *ERC20) UnpackERC20InvalidSenderError(raw []byte) (*ERC20ERC20InvalidSender, error) {
	out := new(ERC20ERC20InvalidSender)
	if err := eRC20.abi.UnpackIntoInterface(out, "ERC20InvalidSender", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ERC20ERC20InvalidSpender represents a ERC20InvalidSpender error raised by the ERC20 contract.
type ERC20ERC20InvalidSpender struct {
	Spender common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ERC20InvalidSpender(address spender)
func ERC20ERC20InvalidSpenderErrorID() common.Hash {
	return common.HexToHash("0x94280d62c347d8d9f4d59a76ea321452406db88df38e0c9da304f58b57b373a2")
}

// UnpackERC20InvalidSpenderError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ERC20InvalidSpender(address spender)
func (eRC20 *ERC20) UnpackERC20InvalidSpenderError(raw []byte) (*ERC20ERC20InvalidSpender, error) {
	out := new(ERC20ERC20InvalidSpender)
	if err := eRC20.abi.UnpackIntoInterface(out, "ERC20InvalidSpender", raw); err != nil {
		return nil, err
	}
	return out, nil
}
