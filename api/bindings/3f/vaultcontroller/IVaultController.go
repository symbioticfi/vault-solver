// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package vaultcontroller

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

// IVaultControllerMetaData contains all meta data concerning the IVaultController contract.
var IVaultControllerMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"allowance\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"yt\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[{\"name\":\"result\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allowancesOf\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"pt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"yt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"approveBatch\",\"inputs\":[{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"asset\",\"inputs\":[],\"outputs\":[{\"name\":\"assetAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"balanceOf\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"yt\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"balancesOf\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"pt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"yt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"burnAll\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ptShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"canWithdraw\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"convertToAssets\",\"inputs\":[{\"name\":\"ptShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"convertToShares\",\"inputs\":[{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"ptShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"decimals\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"name\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"ptToken\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"symbol\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalSupplies\",\"inputs\":[],\"outputs\":[{\"name\":\"pt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"yt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalSupply\",\"inputs\":[{\"name\":\"yt\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferBatch\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferFromBatch\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"ytToken\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"}]",
}

// IVaultControllerABI is the input ABI used to generate the binding from.
// Deprecated: Use IVaultControllerMetaData.ABI instead.
var IVaultControllerABI = IVaultControllerMetaData.ABI

// IVaultController is an auto generated Go binding around an Ethereum contract.
type IVaultController struct {
	IVaultControllerCaller     // Read-only binding to the contract
	IVaultControllerTransactor // Write-only binding to the contract
	IVaultControllerFilterer   // Log filterer for contract events
}

// IVaultControllerCaller is an auto generated read-only Go binding around an Ethereum contract.
type IVaultControllerCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultControllerTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IVaultControllerTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultControllerFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IVaultControllerFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultControllerSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IVaultControllerSession struct {
	Contract     *IVaultController // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IVaultControllerCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IVaultControllerCallerSession struct {
	Contract *IVaultControllerCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts           // Call options to use throughout this session
}

// IVaultControllerTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IVaultControllerTransactorSession struct {
	Contract     *IVaultControllerTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts           // Transaction auth options to use throughout this session
}

// IVaultControllerRaw is an auto generated low-level Go binding around an Ethereum contract.
type IVaultControllerRaw struct {
	Contract *IVaultController // Generic contract binding to access the raw methods on
}

// IVaultControllerCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IVaultControllerCallerRaw struct {
	Contract *IVaultControllerCaller // Generic read-only contract binding to access the raw methods on
}

// IVaultControllerTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IVaultControllerTransactorRaw struct {
	Contract *IVaultControllerTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIVaultController creates a new instance of IVaultController, bound to a specific deployed contract.
func NewIVaultController(address common.Address, backend bind.ContractBackend) (*IVaultController, error) {
	contract, err := bindIVaultController(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IVaultController{IVaultControllerCaller: IVaultControllerCaller{contract: contract}, IVaultControllerTransactor: IVaultControllerTransactor{contract: contract}, IVaultControllerFilterer: IVaultControllerFilterer{contract: contract}}, nil
}

// NewIVaultControllerCaller creates a new read-only instance of IVaultController, bound to a specific deployed contract.
func NewIVaultControllerCaller(address common.Address, caller bind.ContractCaller) (*IVaultControllerCaller, error) {
	contract, err := bindIVaultController(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IVaultControllerCaller{contract: contract}, nil
}

// NewIVaultControllerTransactor creates a new write-only instance of IVaultController, bound to a specific deployed contract.
func NewIVaultControllerTransactor(address common.Address, transactor bind.ContractTransactor) (*IVaultControllerTransactor, error) {
	contract, err := bindIVaultController(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IVaultControllerTransactor{contract: contract}, nil
}

// NewIVaultControllerFilterer creates a new log filterer instance of IVaultController, bound to a specific deployed contract.
func NewIVaultControllerFilterer(address common.Address, filterer bind.ContractFilterer) (*IVaultControllerFilterer, error) {
	contract, err := bindIVaultController(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IVaultControllerFilterer{contract: contract}, nil
}

// bindIVaultController binds a generic wrapper to an already deployed contract.
func bindIVaultController(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IVaultControllerMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IVaultController *IVaultControllerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IVaultController.Contract.IVaultControllerCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IVaultController *IVaultControllerRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultController.Contract.IVaultControllerTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IVaultController *IVaultControllerRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IVaultController.Contract.IVaultControllerTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IVaultController *IVaultControllerCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IVaultController.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IVaultController *IVaultControllerTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultController.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IVaultController *IVaultControllerTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IVaultController.Contract.contract.Transact(opts, method, params...)
}

// Allowance is a free data retrieval call binding the contract method 0xfc091938.
//
// Solidity: function allowance(address owner, address spender, bool yt) view returns(uint256 result)
func (_IVaultController *IVaultControllerCaller) Allowance(opts *bind.CallOpts, owner common.Address, spender common.Address, yt bool) (*big.Int, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "allowance", owner, spender, yt)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allowance is a free data retrieval call binding the contract method 0xfc091938.
//
// Solidity: function allowance(address owner, address spender, bool yt) view returns(uint256 result)
func (_IVaultController *IVaultControllerSession) Allowance(owner common.Address, spender common.Address, yt bool) (*big.Int, error) {
	return _IVaultController.Contract.Allowance(&_IVaultController.CallOpts, owner, spender, yt)
}

// Allowance is a free data retrieval call binding the contract method 0xfc091938.
//
// Solidity: function allowance(address owner, address spender, bool yt) view returns(uint256 result)
func (_IVaultController *IVaultControllerCallerSession) Allowance(owner common.Address, spender common.Address, yt bool) (*big.Int, error) {
	return _IVaultController.Contract.Allowance(&_IVaultController.CallOpts, owner, spender, yt)
}

// AllowancesOf is a free data retrieval call binding the contract method 0xc5d31bec.
//
// Solidity: function allowancesOf(address owner, address spender) view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerCaller) AllowancesOf(opts *bind.CallOpts, owner common.Address, spender common.Address) (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "allowancesOf", owner, spender)

	outstruct := new(struct {
		Pt *big.Int
		Yt *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Pt = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.Yt = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// AllowancesOf is a free data retrieval call binding the contract method 0xc5d31bec.
//
// Solidity: function allowancesOf(address owner, address spender) view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerSession) AllowancesOf(owner common.Address, spender common.Address) (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	return _IVaultController.Contract.AllowancesOf(&_IVaultController.CallOpts, owner, spender)
}

// AllowancesOf is a free data retrieval call binding the contract method 0xc5d31bec.
//
// Solidity: function allowancesOf(address owner, address spender) view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerCallerSession) AllowancesOf(owner common.Address, spender common.Address) (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	return _IVaultController.Contract.AllowancesOf(&_IVaultController.CallOpts, owner, spender)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (_IVaultController *IVaultControllerCaller) Asset(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "asset")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (_IVaultController *IVaultControllerSession) Asset() (common.Address, error) {
	return _IVaultController.Contract.Asset(&_IVaultController.CallOpts)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (_IVaultController *IVaultControllerCallerSession) Asset() (common.Address, error) {
	return _IVaultController.Contract.Asset(&_IVaultController.CallOpts)
}

// BalanceOf is a free data retrieval call binding the contract method 0x772865e2.
//
// Solidity: function balanceOf(address account, bool yt) view returns(uint128)
func (_IVaultController *IVaultControllerCaller) BalanceOf(opts *bind.CallOpts, account common.Address, yt bool) (*big.Int, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "balanceOf", account, yt)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x772865e2.
//
// Solidity: function balanceOf(address account, bool yt) view returns(uint128)
func (_IVaultController *IVaultControllerSession) BalanceOf(account common.Address, yt bool) (*big.Int, error) {
	return _IVaultController.Contract.BalanceOf(&_IVaultController.CallOpts, account, yt)
}

// BalanceOf is a free data retrieval call binding the contract method 0x772865e2.
//
// Solidity: function balanceOf(address account, bool yt) view returns(uint128)
func (_IVaultController *IVaultControllerCallerSession) BalanceOf(account common.Address, yt bool) (*big.Int, error) {
	return _IVaultController.Contract.BalanceOf(&_IVaultController.CallOpts, account, yt)
}

// BalancesOf is a free data retrieval call binding the contract method 0x6392a51f.
//
// Solidity: function balancesOf(address account) view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerCaller) BalancesOf(opts *bind.CallOpts, account common.Address) (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "balancesOf", account)

	outstruct := new(struct {
		Pt *big.Int
		Yt *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Pt = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.Yt = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// BalancesOf is a free data retrieval call binding the contract method 0x6392a51f.
//
// Solidity: function balancesOf(address account) view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerSession) BalancesOf(account common.Address) (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	return _IVaultController.Contract.BalancesOf(&_IVaultController.CallOpts, account)
}

// BalancesOf is a free data retrieval call binding the contract method 0x6392a51f.
//
// Solidity: function balancesOf(address account) view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerCallerSession) BalancesOf(account common.Address) (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	return _IVaultController.Contract.BalancesOf(&_IVaultController.CallOpts, account)
}

// CanWithdraw is a free data retrieval call binding the contract method 0xb51459fe.
//
// Solidity: function canWithdraw() view returns(bool)
func (_IVaultController *IVaultControllerCaller) CanWithdraw(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "canWithdraw")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanWithdraw is a free data retrieval call binding the contract method 0xb51459fe.
//
// Solidity: function canWithdraw() view returns(bool)
func (_IVaultController *IVaultControllerSession) CanWithdraw() (bool, error) {
	return _IVaultController.Contract.CanWithdraw(&_IVaultController.CallOpts)
}

// CanWithdraw is a free data retrieval call binding the contract method 0xb51459fe.
//
// Solidity: function canWithdraw() view returns(bool)
func (_IVaultController *IVaultControllerCallerSession) CanWithdraw() (bool, error) {
	return _IVaultController.Contract.CanWithdraw(&_IVaultController.CallOpts)
}

// ConvertToAssets is a free data retrieval call binding the contract method 0x181e7b3b.
//
// Solidity: function convertToAssets(uint256 ptShares, uint256 ytShares) view returns(uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerCaller) ConvertToAssets(opts *bind.CallOpts, ptShares *big.Int, ytShares *big.Int) (struct {
	PAssets *big.Int
	YAssets *big.Int
}, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "convertToAssets", ptShares, ytShares)

	outstruct := new(struct {
		PAssets *big.Int
		YAssets *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.PAssets = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.YAssets = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// ConvertToAssets is a free data retrieval call binding the contract method 0x181e7b3b.
//
// Solidity: function convertToAssets(uint256 ptShares, uint256 ytShares) view returns(uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerSession) ConvertToAssets(ptShares *big.Int, ytShares *big.Int) (struct {
	PAssets *big.Int
	YAssets *big.Int
}, error) {
	return _IVaultController.Contract.ConvertToAssets(&_IVaultController.CallOpts, ptShares, ytShares)
}

// ConvertToAssets is a free data retrieval call binding the contract method 0x181e7b3b.
//
// Solidity: function convertToAssets(uint256 ptShares, uint256 ytShares) view returns(uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerCallerSession) ConvertToAssets(ptShares *big.Int, ytShares *big.Int) (struct {
	PAssets *big.Int
	YAssets *big.Int
}, error) {
	return _IVaultController.Contract.ConvertToAssets(&_IVaultController.CallOpts, ptShares, ytShares)
}

// ConvertToShares is a free data retrieval call binding the contract method 0xdb2088f4.
//
// Solidity: function convertToShares(uint256 pAssets, uint256 yAssets) view returns(uint256 ptShares, uint256 ytShares)
func (_IVaultController *IVaultControllerCaller) ConvertToShares(opts *bind.CallOpts, pAssets *big.Int, yAssets *big.Int) (struct {
	PtShares *big.Int
	YtShares *big.Int
}, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "convertToShares", pAssets, yAssets)

	outstruct := new(struct {
		PtShares *big.Int
		YtShares *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.PtShares = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.YtShares = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// ConvertToShares is a free data retrieval call binding the contract method 0xdb2088f4.
//
// Solidity: function convertToShares(uint256 pAssets, uint256 yAssets) view returns(uint256 ptShares, uint256 ytShares)
func (_IVaultController *IVaultControllerSession) ConvertToShares(pAssets *big.Int, yAssets *big.Int) (struct {
	PtShares *big.Int
	YtShares *big.Int
}, error) {
	return _IVaultController.Contract.ConvertToShares(&_IVaultController.CallOpts, pAssets, yAssets)
}

// ConvertToShares is a free data retrieval call binding the contract method 0xdb2088f4.
//
// Solidity: function convertToShares(uint256 pAssets, uint256 yAssets) view returns(uint256 ptShares, uint256 ytShares)
func (_IVaultController *IVaultControllerCallerSession) ConvertToShares(pAssets *big.Int, yAssets *big.Int) (struct {
	PtShares *big.Int
	YtShares *big.Int
}, error) {
	return _IVaultController.Contract.ConvertToShares(&_IVaultController.CallOpts, pAssets, yAssets)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_IVaultController *IVaultControllerCaller) Decimals(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "decimals")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_IVaultController *IVaultControllerSession) Decimals() (uint8, error) {
	return _IVaultController.Contract.Decimals(&_IVaultController.CallOpts)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_IVaultController *IVaultControllerCallerSession) Decimals() (uint8, error) {
	return _IVaultController.Contract.Decimals(&_IVaultController.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_IVaultController *IVaultControllerCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_IVaultController *IVaultControllerSession) Name() (string, error) {
	return _IVaultController.Contract.Name(&_IVaultController.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_IVaultController *IVaultControllerCallerSession) Name() (string, error) {
	return _IVaultController.Contract.Name(&_IVaultController.CallOpts)
}

// PtToken is a free data retrieval call binding the contract method 0xe018b0ef.
//
// Solidity: function ptToken() view returns(address)
func (_IVaultController *IVaultControllerCaller) PtToken(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "ptToken")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PtToken is a free data retrieval call binding the contract method 0xe018b0ef.
//
// Solidity: function ptToken() view returns(address)
func (_IVaultController *IVaultControllerSession) PtToken() (common.Address, error) {
	return _IVaultController.Contract.PtToken(&_IVaultController.CallOpts)
}

// PtToken is a free data retrieval call binding the contract method 0xe018b0ef.
//
// Solidity: function ptToken() view returns(address)
func (_IVaultController *IVaultControllerCallerSession) PtToken() (common.Address, error) {
	return _IVaultController.Contract.PtToken(&_IVaultController.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_IVaultController *IVaultControllerCaller) Symbol(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "symbol")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_IVaultController *IVaultControllerSession) Symbol() (string, error) {
	return _IVaultController.Contract.Symbol(&_IVaultController.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_IVaultController *IVaultControllerCallerSession) Symbol() (string, error) {
	return _IVaultController.Contract.Symbol(&_IVaultController.CallOpts)
}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerCaller) TotalAssets(opts *bind.CallOpts) (struct {
	PAssets *big.Int
	YAssets *big.Int
}, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "totalAssets")

	outstruct := new(struct {
		PAssets *big.Int
		YAssets *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.PAssets = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.YAssets = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerSession) TotalAssets() (struct {
	PAssets *big.Int
	YAssets *big.Int
}, error) {
	return _IVaultController.Contract.TotalAssets(&_IVaultController.CallOpts)
}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerCallerSession) TotalAssets() (struct {
	PAssets *big.Int
	YAssets *big.Int
}, error) {
	return _IVaultController.Contract.TotalAssets(&_IVaultController.CallOpts)
}

// TotalSupplies is a free data retrieval call binding the contract method 0xd068cdc5.
//
// Solidity: function totalSupplies() view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerCaller) TotalSupplies(opts *bind.CallOpts) (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "totalSupplies")

	outstruct := new(struct {
		Pt *big.Int
		Yt *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Pt = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.Yt = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// TotalSupplies is a free data retrieval call binding the contract method 0xd068cdc5.
//
// Solidity: function totalSupplies() view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerSession) TotalSupplies() (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	return _IVaultController.Contract.TotalSupplies(&_IVaultController.CallOpts)
}

// TotalSupplies is a free data retrieval call binding the contract method 0xd068cdc5.
//
// Solidity: function totalSupplies() view returns(uint128 pt, uint128 yt)
func (_IVaultController *IVaultControllerCallerSession) TotalSupplies() (struct {
	Pt *big.Int
	Yt *big.Int
}, error) {
	return _IVaultController.Contract.TotalSupplies(&_IVaultController.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x89942649.
//
// Solidity: function totalSupply(bool yt) view returns(uint128)
func (_IVaultController *IVaultControllerCaller) TotalSupply(opts *bind.CallOpts, yt bool) (*big.Int, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "totalSupply", yt)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x89942649.
//
// Solidity: function totalSupply(bool yt) view returns(uint128)
func (_IVaultController *IVaultControllerSession) TotalSupply(yt bool) (*big.Int, error) {
	return _IVaultController.Contract.TotalSupply(&_IVaultController.CallOpts, yt)
}

// TotalSupply is a free data retrieval call binding the contract method 0x89942649.
//
// Solidity: function totalSupply(bool yt) view returns(uint128)
func (_IVaultController *IVaultControllerCallerSession) TotalSupply(yt bool) (*big.Int, error) {
	return _IVaultController.Contract.TotalSupply(&_IVaultController.CallOpts, yt)
}

// YtToken is a free data retrieval call binding the contract method 0x42203015.
//
// Solidity: function ytToken() view returns(address)
func (_IVaultController *IVaultControllerCaller) YtToken(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultController.contract.Call(opts, &out, "ytToken")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// YtToken is a free data retrieval call binding the contract method 0x42203015.
//
// Solidity: function ytToken() view returns(address)
func (_IVaultController *IVaultControllerSession) YtToken() (common.Address, error) {
	return _IVaultController.Contract.YtToken(&_IVaultController.CallOpts)
}

// YtToken is a free data retrieval call binding the contract method 0x42203015.
//
// Solidity: function ytToken() view returns(address)
func (_IVaultController *IVaultControllerCallerSession) YtToken() (common.Address, error) {
	return _IVaultController.Contract.YtToken(&_IVaultController.CallOpts)
}

// ApproveBatch is a paid mutator transaction binding the contract method 0x75ac8912.
//
// Solidity: function approveBatch(address spender, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerTransactor) ApproveBatch(opts *bind.TransactOpts, spender common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.contract.Transact(opts, "approveBatch", spender, ptAmount, ytAmount)
}

// ApproveBatch is a paid mutator transaction binding the contract method 0x75ac8912.
//
// Solidity: function approveBatch(address spender, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerSession) ApproveBatch(spender common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.Contract.ApproveBatch(&_IVaultController.TransactOpts, spender, ptAmount, ytAmount)
}

// ApproveBatch is a paid mutator transaction binding the contract method 0x75ac8912.
//
// Solidity: function approveBatch(address spender, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerTransactorSession) ApproveBatch(spender common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.Contract.ApproveBatch(&_IVaultController.TransactOpts, spender, ptAmount, ytAmount)
}

// BurnAll is a paid mutator transaction binding the contract method 0xdd19a0d8.
//
// Solidity: function burnAll(address owner, address receiver) returns(uint256 ptShares, uint256 ytShares, uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerTransactor) BurnAll(opts *bind.TransactOpts, owner common.Address, receiver common.Address) (*types.Transaction, error) {
	return _IVaultController.contract.Transact(opts, "burnAll", owner, receiver)
}

// BurnAll is a paid mutator transaction binding the contract method 0xdd19a0d8.
//
// Solidity: function burnAll(address owner, address receiver) returns(uint256 ptShares, uint256 ytShares, uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerSession) BurnAll(owner common.Address, receiver common.Address) (*types.Transaction, error) {
	return _IVaultController.Contract.BurnAll(&_IVaultController.TransactOpts, owner, receiver)
}

// BurnAll is a paid mutator transaction binding the contract method 0xdd19a0d8.
//
// Solidity: function burnAll(address owner, address receiver) returns(uint256 ptShares, uint256 ytShares, uint256 pAssets, uint256 yAssets)
func (_IVaultController *IVaultControllerTransactorSession) BurnAll(owner common.Address, receiver common.Address) (*types.Transaction, error) {
	return _IVaultController.Contract.BurnAll(&_IVaultController.TransactOpts, owner, receiver)
}

// TransferBatch is a paid mutator transaction binding the contract method 0x161a5ef8.
//
// Solidity: function transferBatch(address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerTransactor) TransferBatch(opts *bind.TransactOpts, to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.contract.Transact(opts, "transferBatch", to, ptAmount, ytAmount)
}

// TransferBatch is a paid mutator transaction binding the contract method 0x161a5ef8.
//
// Solidity: function transferBatch(address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerSession) TransferBatch(to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.Contract.TransferBatch(&_IVaultController.TransactOpts, to, ptAmount, ytAmount)
}

// TransferBatch is a paid mutator transaction binding the contract method 0x161a5ef8.
//
// Solidity: function transferBatch(address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerTransactorSession) TransferBatch(to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.Contract.TransferBatch(&_IVaultController.TransactOpts, to, ptAmount, ytAmount)
}

// TransferFromBatch is a paid mutator transaction binding the contract method 0x68a3b3de.
//
// Solidity: function transferFromBatch(address from, address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerTransactor) TransferFromBatch(opts *bind.TransactOpts, from common.Address, to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.contract.Transact(opts, "transferFromBatch", from, to, ptAmount, ytAmount)
}

// TransferFromBatch is a paid mutator transaction binding the contract method 0x68a3b3de.
//
// Solidity: function transferFromBatch(address from, address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerSession) TransferFromBatch(from common.Address, to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.Contract.TransferFromBatch(&_IVaultController.TransactOpts, from, to, ptAmount, ytAmount)
}

// TransferFromBatch is a paid mutator transaction binding the contract method 0x68a3b3de.
//
// Solidity: function transferFromBatch(address from, address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (_IVaultController *IVaultControllerTransactorSession) TransferFromBatch(from common.Address, to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IVaultController.Contract.TransferFromBatch(&_IVaultController.TransactOpts, from, to, ptAmount, ytAmount)
}
