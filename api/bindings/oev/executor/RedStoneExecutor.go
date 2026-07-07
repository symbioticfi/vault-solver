// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package executor

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

// RedStoneExecutorMetaData contains all meta data concerning the RedStoneExecutor contract.
var RedStoneExecutorMetaData = bind.MetaData{
	ABI: "[{\"inputs\":[{\"type\":\"address\"}],\"name\":\"nonces\",\"outputs\":[{\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"type\":\"address\"}],\"name\":\"deposits\",\"outputs\":[{\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"type\":\"address\"}],\"name\":\"locked\",\"outputs\":[{\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"deposit\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"name\":\"solver\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"nonce\",\"type\":\"uint256\"}],\"name\":\"LiquidationFailed\",\"type\":\"event\"}]",
	ID:  "RedStoneExecutor",
}

// RedStoneExecutor is an auto generated Go binding around an Ethereum contract.
type RedStoneExecutor struct {
	abi abi.ABI
}

// NewRedStoneExecutor creates a new instance of RedStoneExecutor.
func NewRedStoneExecutor() *RedStoneExecutor {
	parsed, err := RedStoneExecutorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &RedStoneExecutor{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *RedStoneExecutor) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackDeposit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd0e30db0.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function deposit() payable returns()
func (redStoneExecutor *RedStoneExecutor) PackDeposit() []byte {
	enc, err := redStoneExecutor.abi.Pack("deposit")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDeposit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd0e30db0.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function deposit() payable returns()
func (redStoneExecutor *RedStoneExecutor) TryPackDeposit() ([]byte, error) {
	return redStoneExecutor.abi.Pack("deposit")
}

// PackDeposits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfc7e286d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function deposits(address ) view returns(uint256)
func (redStoneExecutor *RedStoneExecutor) PackDeposits(arg0 common.Address) []byte {
	enc, err := redStoneExecutor.abi.Pack("deposits", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDeposits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfc7e286d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function deposits(address ) view returns(uint256)
func (redStoneExecutor *RedStoneExecutor) TryPackDeposits(arg0 common.Address) ([]byte, error) {
	return redStoneExecutor.abi.Pack("deposits", arg0)
}

// UnpackDeposits is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfc7e286d.
//
// Solidity: function deposits(address ) view returns(uint256)
func (redStoneExecutor *RedStoneExecutor) UnpackDeposits(data []byte) (*big.Int, error) {
	out, err := redStoneExecutor.abi.Unpack("deposits", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackLocked is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xcbf9fe5f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function locked(address ) view returns(bool)
func (redStoneExecutor *RedStoneExecutor) PackLocked(arg0 common.Address) []byte {
	enc, err := redStoneExecutor.abi.Pack("locked", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLocked is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xcbf9fe5f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function locked(address ) view returns(bool)
func (redStoneExecutor *RedStoneExecutor) TryPackLocked(arg0 common.Address) ([]byte, error) {
	return redStoneExecutor.abi.Pack("locked", arg0)
}

// UnpackLocked is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xcbf9fe5f.
//
// Solidity: function locked(address ) view returns(bool)
func (redStoneExecutor *RedStoneExecutor) UnpackLocked(data []byte) (bool, error) {
	out, err := redStoneExecutor.abi.Unpack("locked", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackNonces is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7ecebe00.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function nonces(address ) view returns(uint256)
func (redStoneExecutor *RedStoneExecutor) PackNonces(arg0 common.Address) []byte {
	enc, err := redStoneExecutor.abi.Pack("nonces", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackNonces is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7ecebe00.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function nonces(address ) view returns(uint256)
func (redStoneExecutor *RedStoneExecutor) TryPackNonces(arg0 common.Address) ([]byte, error) {
	return redStoneExecutor.abi.Pack("nonces", arg0)
}

// UnpackNonces is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x7ecebe00.
//
// Solidity: function nonces(address ) view returns(uint256)
func (redStoneExecutor *RedStoneExecutor) UnpackNonces(data []byte) (*big.Int, error) {
	out, err := redStoneExecutor.abi.Unpack("nonces", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// RedStoneExecutorLiquidationFailed represents a LiquidationFailed event raised by the RedStoneExecutor contract.
type RedStoneExecutorLiquidationFailed struct {
	Solver common.Address
	Nonce  *big.Int
	Raw    *types.Log // Blockchain specific contextual infos
}

const RedStoneExecutorLiquidationFailedEventName = "LiquidationFailed"

// ContractEventName returns the user-defined event name.
func (RedStoneExecutorLiquidationFailed) ContractEventName() string {
	return RedStoneExecutorLiquidationFailedEventName
}

// UnpackLiquidationFailedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event LiquidationFailed(address indexed solver, uint256 nonce)
func (redStoneExecutor *RedStoneExecutor) UnpackLiquidationFailedEvent(log *types.Log) (*RedStoneExecutorLiquidationFailed, error) {
	event := "LiquidationFailed"
	if log.Topics[0] != redStoneExecutor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(RedStoneExecutorLiquidationFailed)
	if len(log.Data) > 0 {
		if err := redStoneExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range redStoneExecutor.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	if err := abi.ParseTopics(out, indexed, log.Topics[1:]); err != nil {
		return nil, err
	}
	out.Raw = log
	return out, nil
}
