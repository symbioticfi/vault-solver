// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package delegator

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// UniversalDelegatorMetaData contains all meta data concerning the UniversalDelegator contract.
var UniversalDelegatorMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"limitOf\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"}]",
}

// UniversalDelegatorABI is the input ABI used to generate the binding from.
// Deprecated: Use UniversalDelegatorMetaData.ABI instead.
var UniversalDelegatorABI = UniversalDelegatorMetaData.ABI

// UniversalDelegator is an auto generated Go binding around an Ethereum contract.
type UniversalDelegator struct {
	UniversalDelegatorCaller     // Read-only binding to the contract
	UniversalDelegatorTransactor // Write-only binding to the contract
	UniversalDelegatorFilterer   // Log filterer for contract events
}

// UniversalDelegatorCaller is an auto generated read-only Go binding around an Ethereum contract.
type UniversalDelegatorCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// UniversalDelegatorTransactor is an auto generated write-only Go binding around an Ethereum contract.
type UniversalDelegatorTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// UniversalDelegatorFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type UniversalDelegatorFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// UniversalDelegatorSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type UniversalDelegatorSession struct {
	Contract     *UniversalDelegator // Generic contract binding to set the session for
	CallOpts     bind.CallOpts       // Call options to use throughout this session
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// UniversalDelegatorCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type UniversalDelegatorCallerSession struct {
	Contract *UniversalDelegatorCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts             // Call options to use throughout this session
}

// UniversalDelegatorTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type UniversalDelegatorTransactorSession struct {
	Contract     *UniversalDelegatorTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// UniversalDelegatorRaw is an auto generated low-level Go binding around an Ethereum contract.
type UniversalDelegatorRaw struct {
	Contract *UniversalDelegator // Generic contract binding to access the raw methods on
}

// UniversalDelegatorCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type UniversalDelegatorCallerRaw struct {
	Contract *UniversalDelegatorCaller // Generic read-only contract binding to access the raw methods on
}

// UniversalDelegatorTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type UniversalDelegatorTransactorRaw struct {
	Contract *UniversalDelegatorTransactor // Generic write-only contract binding to access the raw methods on
}

// NewUniversalDelegator creates a new instance of UniversalDelegator, bound to a specific deployed contract.
func NewUniversalDelegator(address common.Address, backend bind.ContractBackend) (*UniversalDelegator, error) {
	contract, err := bindUniversalDelegator(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &UniversalDelegator{UniversalDelegatorCaller: UniversalDelegatorCaller{contract: contract}, UniversalDelegatorTransactor: UniversalDelegatorTransactor{contract: contract}, UniversalDelegatorFilterer: UniversalDelegatorFilterer{contract: contract}}, nil
}

// NewUniversalDelegatorCaller creates a new read-only instance of UniversalDelegator, bound to a specific deployed contract.
func NewUniversalDelegatorCaller(address common.Address, caller bind.ContractCaller) (*UniversalDelegatorCaller, error) {
	contract, err := bindUniversalDelegator(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &UniversalDelegatorCaller{contract: contract}, nil
}

// NewUniversalDelegatorTransactor creates a new write-only instance of UniversalDelegator, bound to a specific deployed contract.
func NewUniversalDelegatorTransactor(address common.Address, transactor bind.ContractTransactor) (*UniversalDelegatorTransactor, error) {
	contract, err := bindUniversalDelegator(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &UniversalDelegatorTransactor{contract: contract}, nil
}

// NewUniversalDelegatorFilterer creates a new log filterer instance of UniversalDelegator, bound to a specific deployed contract.
func NewUniversalDelegatorFilterer(address common.Address, filterer bind.ContractFilterer) (*UniversalDelegatorFilterer, error) {
	contract, err := bindUniversalDelegator(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &UniversalDelegatorFilterer{contract: contract}, nil
}

// bindUniversalDelegator binds a generic wrapper to an already deployed contract.
func bindUniversalDelegator(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := UniversalDelegatorMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_UniversalDelegator *UniversalDelegatorRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _UniversalDelegator.Contract.UniversalDelegatorCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_UniversalDelegator *UniversalDelegatorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _UniversalDelegator.Contract.UniversalDelegatorTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_UniversalDelegator *UniversalDelegatorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _UniversalDelegator.Contract.UniversalDelegatorTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_UniversalDelegator *UniversalDelegatorCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _UniversalDelegator.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_UniversalDelegator *UniversalDelegatorTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _UniversalDelegator.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_UniversalDelegator *UniversalDelegatorTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _UniversalDelegator.Contract.contract.Transact(opts, method, params...)
}

// LimitOf is a free data retrieval call binding the contract method 0x546a2ca4.
//
// Solidity: function limitOf(address adapter) view returns(uint256)
func (_UniversalDelegator *UniversalDelegatorCaller) LimitOf(opts *bind.CallOpts, adapter common.Address) (*big.Int, error) {
	var out []interface{}
	err := _UniversalDelegator.contract.Call(opts, &out, "limitOf", adapter)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// LimitOf is a free data retrieval call binding the contract method 0x546a2ca4.
//
// Solidity: function limitOf(address adapter) view returns(uint256)
func (_UniversalDelegator *UniversalDelegatorSession) LimitOf(adapter common.Address) (*big.Int, error) {
	return _UniversalDelegator.Contract.LimitOf(&_UniversalDelegator.CallOpts, adapter)
}

// LimitOf is a free data retrieval call binding the contract method 0x546a2ca4.
//
// Solidity: function limitOf(address adapter) view returns(uint256)
func (_UniversalDelegator *UniversalDelegatorCallerSession) LimitOf(adapter common.Address) (*big.Int, error) {
	return _UniversalDelegator.Contract.LimitOf(&_UniversalDelegator.CallOpts, adapter)
}
