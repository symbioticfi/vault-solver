// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package whitelist

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

// IWhitelistMetaData contains all meta data concerning the IWhitelist contract.
var IWhitelistMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"isWhitelisted\",\"inputs\":[{\"name\":\"a\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"enumIWhitelist.WhitelistStatus\"}],\"stateMutability\":\"view\"}]",
}

// IWhitelistABI is the input ABI used to generate the binding from.
// Deprecated: Use IWhitelistMetaData.ABI instead.
var IWhitelistABI = IWhitelistMetaData.ABI

// IWhitelist is an auto generated Go binding around an Ethereum contract.
type IWhitelist struct {
	IWhitelistCaller     // Read-only binding to the contract
	IWhitelistTransactor // Write-only binding to the contract
	IWhitelistFilterer   // Log filterer for contract events
}

// IWhitelistCaller is an auto generated read-only Go binding around an Ethereum contract.
type IWhitelistCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IWhitelistTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IWhitelistTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IWhitelistFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IWhitelistFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IWhitelistSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IWhitelistSession struct {
	Contract     *IWhitelist       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IWhitelistCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IWhitelistCallerSession struct {
	Contract *IWhitelistCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// IWhitelistTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IWhitelistTransactorSession struct {
	Contract     *IWhitelistTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// IWhitelistRaw is an auto generated low-level Go binding around an Ethereum contract.
type IWhitelistRaw struct {
	Contract *IWhitelist // Generic contract binding to access the raw methods on
}

// IWhitelistCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IWhitelistCallerRaw struct {
	Contract *IWhitelistCaller // Generic read-only contract binding to access the raw methods on
}

// IWhitelistTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IWhitelistTransactorRaw struct {
	Contract *IWhitelistTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIWhitelist creates a new instance of IWhitelist, bound to a specific deployed contract.
func NewIWhitelist(address common.Address, backend bind.ContractBackend) (*IWhitelist, error) {
	contract, err := bindIWhitelist(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IWhitelist{IWhitelistCaller: IWhitelistCaller{contract: contract}, IWhitelistTransactor: IWhitelistTransactor{contract: contract}, IWhitelistFilterer: IWhitelistFilterer{contract: contract}}, nil
}

// NewIWhitelistCaller creates a new read-only instance of IWhitelist, bound to a specific deployed contract.
func NewIWhitelistCaller(address common.Address, caller bind.ContractCaller) (*IWhitelistCaller, error) {
	contract, err := bindIWhitelist(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IWhitelistCaller{contract: contract}, nil
}

// NewIWhitelistTransactor creates a new write-only instance of IWhitelist, bound to a specific deployed contract.
func NewIWhitelistTransactor(address common.Address, transactor bind.ContractTransactor) (*IWhitelistTransactor, error) {
	contract, err := bindIWhitelist(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IWhitelistTransactor{contract: contract}, nil
}

// NewIWhitelistFilterer creates a new log filterer instance of IWhitelist, bound to a specific deployed contract.
func NewIWhitelistFilterer(address common.Address, filterer bind.ContractFilterer) (*IWhitelistFilterer, error) {
	contract, err := bindIWhitelist(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IWhitelistFilterer{contract: contract}, nil
}

// bindIWhitelist binds a generic wrapper to an already deployed contract.
func bindIWhitelist(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IWhitelistMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IWhitelist *IWhitelistRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IWhitelist.Contract.IWhitelistCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IWhitelist *IWhitelistRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IWhitelist.Contract.IWhitelistTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IWhitelist *IWhitelistRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IWhitelist.Contract.IWhitelistTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IWhitelist *IWhitelistCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IWhitelist.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IWhitelist *IWhitelistTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IWhitelist.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IWhitelist *IWhitelistTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IWhitelist.Contract.contract.Transact(opts, method, params...)
}

// IsWhitelisted is a free data retrieval call binding the contract method 0x3af32abf.
//
// Solidity: function isWhitelisted(address a) view returns(uint8)
func (_IWhitelist *IWhitelistCaller) IsWhitelisted(opts *bind.CallOpts, a common.Address) (uint8, error) {
	var out []interface{}
	err := _IWhitelist.contract.Call(opts, &out, "isWhitelisted", a)

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// IsWhitelisted is a free data retrieval call binding the contract method 0x3af32abf.
//
// Solidity: function isWhitelisted(address a) view returns(uint8)
func (_IWhitelist *IWhitelistSession) IsWhitelisted(a common.Address) (uint8, error) {
	return _IWhitelist.Contract.IsWhitelisted(&_IWhitelist.CallOpts, a)
}

// IsWhitelisted is a free data retrieval call binding the contract method 0x3af32abf.
//
// Solidity: function isWhitelisted(address a) view returns(uint8)
func (_IWhitelist *IWhitelistCallerSession) IsWhitelisted(a common.Address) (uint8, error) {
	return _IWhitelist.Contract.IsWhitelisted(&_IWhitelist.CallOpts, a)
}
