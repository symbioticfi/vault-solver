// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package vaultv2

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

// IVaultV2InitParams is an auto generated low-level Go binding around an user-defined struct.
type IVaultV2InitParams struct {
	Name                          string
	Symbol                        string
	Asset                         common.Address
	DepositWhitelist              bool
	DepositorToWhitelist          common.Address
	DepositLimit                  *big.Int
	IsDepositLimit                bool
	DefaultAdminRoleHolder        common.Address
	ManagementFeeRoleHolder       common.Address
	PerformanceFeeRoleHolder      common.Address
	DepositLimitSetRoleHolder     common.Address
	DepositorWhitelistRoleHolder  common.Address
	IsDepositLimitSetRoleHolder   common.Address
	DepositWhitelistSetRoleHolder common.Address
}

// IVaultV2MetaData contains all meta data concerning the IVaultV2 contract.
var IVaultV2MetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"accrueInterest\",\"inputs\":[],\"outputs\":[{\"name\":\"managementFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"performanceFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocolFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"balanceOfAt\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"delegator\",\"inputs\":[],\"outputs\":[{\"name\":\"delegatorAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"depositLimit\",\"inputs\":[],\"outputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"depositWhitelist\",\"inputs\":[],\"outputs\":[{\"name\":\"enabled\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAccrueInterest\",\"inputs\":[],\"outputs\":[{\"name\":\"newTotalAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"managementFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"performanceFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocolFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isDepositLimit\",\"inputs\":[],\"outputs\":[{\"name\":\"enabled\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isDepositorWhitelisted\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"whitelisted\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isInitialized\",\"inputs\":[],\"outputs\":[{\"name\":\"initialized\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastProtocolFeeReceiver\",\"inputs\":[],\"outputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastProtocolManagementFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastProtocolPerformanceFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastUpdate\",\"inputs\":[],\"outputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"managementFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"managementFeeReceiver\",\"inputs\":[],\"outputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"performanceFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"performanceFeeReceiver\",\"inputs\":[],\"outputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pull\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"push\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"redeemable\",\"inputs\":[],\"outputs\":[{\"name\":\"shares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDelegator\",\"inputs\":[{\"name\":\"delegator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositLimit\",\"inputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositWhitelist\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositorWhitelistStatus\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setIsDepositLimit\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setManagementFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPerformanceFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setSlasher\",\"inputs\":[{\"name\":\"slasher\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalSupplyAt\",\"inputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"supply\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawable\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdrawalQueue\",\"inputs\":[],\"outputs\":[{\"name\":\"withdrawalQueueAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"AccrueInterest\",\"inputs\":[{\"name\":\"newTotalAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"managementFeeShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"performanceFeeShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"protocolFeeShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Claim\",\"inputs\":[{\"name\":\"claimer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialize\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structIVaultV2.InitParams\",\"components\":[{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"symbol\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositWhitelist\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"depositorToWhitelist\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"isDepositLimit\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"defaultAdminRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"managementFeeRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"performanceFeeRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositLimitSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositorWhitelistRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"isDepositLimitSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositWhitelistSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Pull\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Push\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"owner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDelegator\",\"inputs\":[{\"name\":\"delegator\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositLimit\",\"inputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositWhitelist\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositorWhitelistStatus\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetIsDepositLimit\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetManagementFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetPerformanceFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetWithdrawalQueue\",\"inputs\":[{\"name\":\"withdrawalQueue\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"UpdateProtocolFee\",\"inputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"managementFee\",\"type\":\"uint96\",\"indexed\":false,\"internalType\":\"uint96\"},{\"name\":\"performanceFee\",\"type\":\"uint96\",\"indexed\":false,\"internalType\":\"uint96\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DelegatorAlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DepositLimitReached\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"FeeTooHigh\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientFreeAssets\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAddress\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDelegator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDepositorToWhitelist\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotDelegator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotWhitelistedDepositor\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotWithdrawalQueue\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"PendingWithdrawalQueue\",\"inputs\":[]}]",
}

// IVaultV2ABI is the input ABI used to generate the binding from.
// Deprecated: Use IVaultV2MetaData.ABI instead.
var IVaultV2ABI = IVaultV2MetaData.ABI

// IVaultV2 is an auto generated Go binding around an Ethereum contract.
type IVaultV2 struct {
	IVaultV2Caller     // Read-only binding to the contract
	IVaultV2Transactor // Write-only binding to the contract
	IVaultV2Filterer   // Log filterer for contract events
}

// IVaultV2Caller is an auto generated read-only Go binding around an Ethereum contract.
type IVaultV2Caller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultV2Transactor is an auto generated write-only Go binding around an Ethereum contract.
type IVaultV2Transactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultV2Filterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IVaultV2Filterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultV2Session is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IVaultV2Session struct {
	Contract     *IVaultV2         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IVaultV2CallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IVaultV2CallerSession struct {
	Contract *IVaultV2Caller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// IVaultV2TransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IVaultV2TransactorSession struct {
	Contract     *IVaultV2Transactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// IVaultV2Raw is an auto generated low-level Go binding around an Ethereum contract.
type IVaultV2Raw struct {
	Contract *IVaultV2 // Generic contract binding to access the raw methods on
}

// IVaultV2CallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IVaultV2CallerRaw struct {
	Contract *IVaultV2Caller // Generic read-only contract binding to access the raw methods on
}

// IVaultV2TransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IVaultV2TransactorRaw struct {
	Contract *IVaultV2Transactor // Generic write-only contract binding to access the raw methods on
}

// NewIVaultV2 creates a new instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2(address common.Address, backend bind.ContractBackend) (*IVaultV2, error) {
	contract, err := bindIVaultV2(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IVaultV2{IVaultV2Caller: IVaultV2Caller{contract: contract}, IVaultV2Transactor: IVaultV2Transactor{contract: contract}, IVaultV2Filterer: IVaultV2Filterer{contract: contract}}, nil
}

// NewIVaultV2Caller creates a new read-only instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2Caller(address common.Address, caller bind.ContractCaller) (*IVaultV2Caller, error) {
	contract, err := bindIVaultV2(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IVaultV2Caller{contract: contract}, nil
}

// NewIVaultV2Transactor creates a new write-only instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2Transactor(address common.Address, transactor bind.ContractTransactor) (*IVaultV2Transactor, error) {
	contract, err := bindIVaultV2(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IVaultV2Transactor{contract: contract}, nil
}

// NewIVaultV2Filterer creates a new log filterer instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2Filterer(address common.Address, filterer bind.ContractFilterer) (*IVaultV2Filterer, error) {
	contract, err := bindIVaultV2(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IVaultV2Filterer{contract: contract}, nil
}

// bindIVaultV2 binds a generic wrapper to an already deployed contract.
func bindIVaultV2(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IVaultV2MetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IVaultV2 *IVaultV2Raw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IVaultV2.Contract.IVaultV2Caller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IVaultV2 *IVaultV2Raw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.Contract.IVaultV2Transactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IVaultV2 *IVaultV2Raw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IVaultV2.Contract.IVaultV2Transactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IVaultV2 *IVaultV2CallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IVaultV2.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IVaultV2 *IVaultV2TransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IVaultV2 *IVaultV2TransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IVaultV2.Contract.contract.Transact(opts, method, params...)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_IVaultV2 *IVaultV2Caller) FACTORY(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "FACTORY")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_IVaultV2 *IVaultV2Session) FACTORY() (common.Address, error) {
	return _IVaultV2.Contract.FACTORY(&_IVaultV2.CallOpts)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_IVaultV2 *IVaultV2CallerSession) FACTORY() (common.Address, error) {
	return _IVaultV2.Contract.FACTORY(&_IVaultV2.CallOpts)
}

// BalanceOfAt is a free data retrieval call binding the contract method 0x95c3a492.
//
// Solidity: function balanceOfAt(address account, uint48 timestamp) view returns(uint256 balance)
func (_IVaultV2 *IVaultV2Caller) BalanceOfAt(opts *bind.CallOpts, account common.Address, timestamp *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "balanceOfAt", account, timestamp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOfAt is a free data retrieval call binding the contract method 0x95c3a492.
//
// Solidity: function balanceOfAt(address account, uint48 timestamp) view returns(uint256 balance)
func (_IVaultV2 *IVaultV2Session) BalanceOfAt(account common.Address, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.BalanceOfAt(&_IVaultV2.CallOpts, account, timestamp)
}

// BalanceOfAt is a free data retrieval call binding the contract method 0x95c3a492.
//
// Solidity: function balanceOfAt(address account, uint48 timestamp) view returns(uint256 balance)
func (_IVaultV2 *IVaultV2CallerSession) BalanceOfAt(account common.Address, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.BalanceOfAt(&_IVaultV2.CallOpts, account, timestamp)
}

// Delegator is a free data retrieval call binding the contract method 0xce9b7930.
//
// Solidity: function delegator() view returns(address delegatorAddress)
func (_IVaultV2 *IVaultV2Caller) Delegator(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "delegator")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Delegator is a free data retrieval call binding the contract method 0xce9b7930.
//
// Solidity: function delegator() view returns(address delegatorAddress)
func (_IVaultV2 *IVaultV2Session) Delegator() (common.Address, error) {
	return _IVaultV2.Contract.Delegator(&_IVaultV2.CallOpts)
}

// Delegator is a free data retrieval call binding the contract method 0xce9b7930.
//
// Solidity: function delegator() view returns(address delegatorAddress)
func (_IVaultV2 *IVaultV2CallerSession) Delegator() (common.Address, error) {
	return _IVaultV2.Contract.Delegator(&_IVaultV2.CallOpts)
}

// DepositLimit is a free data retrieval call binding the contract method 0xecf70858.
//
// Solidity: function depositLimit() view returns(uint256 limit)
func (_IVaultV2 *IVaultV2Caller) DepositLimit(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "depositLimit")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// DepositLimit is a free data retrieval call binding the contract method 0xecf70858.
//
// Solidity: function depositLimit() view returns(uint256 limit)
func (_IVaultV2 *IVaultV2Session) DepositLimit() (*big.Int, error) {
	return _IVaultV2.Contract.DepositLimit(&_IVaultV2.CallOpts)
}

// DepositLimit is a free data retrieval call binding the contract method 0xecf70858.
//
// Solidity: function depositLimit() view returns(uint256 limit)
func (_IVaultV2 *IVaultV2CallerSession) DepositLimit() (*big.Int, error) {
	return _IVaultV2.Contract.DepositLimit(&_IVaultV2.CallOpts)
}

// DepositWhitelist is a free data retrieval call binding the contract method 0x48d3b775.
//
// Solidity: function depositWhitelist() view returns(bool enabled)
func (_IVaultV2 *IVaultV2Caller) DepositWhitelist(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "depositWhitelist")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// DepositWhitelist is a free data retrieval call binding the contract method 0x48d3b775.
//
// Solidity: function depositWhitelist() view returns(bool enabled)
func (_IVaultV2 *IVaultV2Session) DepositWhitelist() (bool, error) {
	return _IVaultV2.Contract.DepositWhitelist(&_IVaultV2.CallOpts)
}

// DepositWhitelist is a free data retrieval call binding the contract method 0x48d3b775.
//
// Solidity: function depositWhitelist() view returns(bool enabled)
func (_IVaultV2 *IVaultV2CallerSession) DepositWhitelist() (bool, error) {
	return _IVaultV2.Contract.DepositWhitelist(&_IVaultV2.CallOpts)
}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (_IVaultV2 *IVaultV2Caller) FreeAssets(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "freeAssets")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (_IVaultV2 *IVaultV2Session) FreeAssets() (*big.Int, error) {
	return _IVaultV2.Contract.FreeAssets(&_IVaultV2.CallOpts)
}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (_IVaultV2 *IVaultV2CallerSession) FreeAssets() (*big.Int, error) {
	return _IVaultV2.Contract.FreeAssets(&_IVaultV2.CallOpts)
}

// GetAccrueInterest is a free data retrieval call binding the contract method 0x0445a611.
//
// Solidity: function getAccrueInterest() view returns(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2Caller) GetAccrueInterest(opts *bind.CallOpts) (struct {
	NewTotalAssets       *big.Int
	ManagementFeeShares  *big.Int
	PerformanceFeeShares *big.Int
	ProtocolFeeShares    *big.Int
}, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "getAccrueInterest")

	outstruct := new(struct {
		NewTotalAssets       *big.Int
		ManagementFeeShares  *big.Int
		PerformanceFeeShares *big.Int
		ProtocolFeeShares    *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.NewTotalAssets = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.ManagementFeeShares = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.PerformanceFeeShares = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.ProtocolFeeShares = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetAccrueInterest is a free data retrieval call binding the contract method 0x0445a611.
//
// Solidity: function getAccrueInterest() view returns(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2Session) GetAccrueInterest() (struct {
	NewTotalAssets       *big.Int
	ManagementFeeShares  *big.Int
	PerformanceFeeShares *big.Int
	ProtocolFeeShares    *big.Int
}, error) {
	return _IVaultV2.Contract.GetAccrueInterest(&_IVaultV2.CallOpts)
}

// GetAccrueInterest is a free data retrieval call binding the contract method 0x0445a611.
//
// Solidity: function getAccrueInterest() view returns(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2CallerSession) GetAccrueInterest() (struct {
	NewTotalAssets       *big.Int
	ManagementFeeShares  *big.Int
	PerformanceFeeShares *big.Int
	ProtocolFeeShares    *big.Int
}, error) {
	return _IVaultV2.Contract.GetAccrueInterest(&_IVaultV2.CallOpts)
}

// IsDepositLimit is a free data retrieval call binding the contract method 0xa1b12202.
//
// Solidity: function isDepositLimit() view returns(bool enabled)
func (_IVaultV2 *IVaultV2Caller) IsDepositLimit(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "isDepositLimit")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsDepositLimit is a free data retrieval call binding the contract method 0xa1b12202.
//
// Solidity: function isDepositLimit() view returns(bool enabled)
func (_IVaultV2 *IVaultV2Session) IsDepositLimit() (bool, error) {
	return _IVaultV2.Contract.IsDepositLimit(&_IVaultV2.CallOpts)
}

// IsDepositLimit is a free data retrieval call binding the contract method 0xa1b12202.
//
// Solidity: function isDepositLimit() view returns(bool enabled)
func (_IVaultV2 *IVaultV2CallerSession) IsDepositLimit() (bool, error) {
	return _IVaultV2.Contract.IsDepositLimit(&_IVaultV2.CallOpts)
}

// IsDepositorWhitelisted is a free data retrieval call binding the contract method 0x794b15b7.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool whitelisted)
func (_IVaultV2 *IVaultV2Caller) IsDepositorWhitelisted(opts *bind.CallOpts, account common.Address) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "isDepositorWhitelisted", account)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsDepositorWhitelisted is a free data retrieval call binding the contract method 0x794b15b7.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool whitelisted)
func (_IVaultV2 *IVaultV2Session) IsDepositorWhitelisted(account common.Address) (bool, error) {
	return _IVaultV2.Contract.IsDepositorWhitelisted(&_IVaultV2.CallOpts, account)
}

// IsDepositorWhitelisted is a free data retrieval call binding the contract method 0x794b15b7.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool whitelisted)
func (_IVaultV2 *IVaultV2CallerSession) IsDepositorWhitelisted(account common.Address) (bool, error) {
	return _IVaultV2.Contract.IsDepositorWhitelisted(&_IVaultV2.CallOpts, account)
}

// IsInitialized is a free data retrieval call binding the contract method 0x392e53cd.
//
// Solidity: function isInitialized() view returns(bool initialized)
func (_IVaultV2 *IVaultV2Caller) IsInitialized(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "isInitialized")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsInitialized is a free data retrieval call binding the contract method 0x392e53cd.
//
// Solidity: function isInitialized() view returns(bool initialized)
func (_IVaultV2 *IVaultV2Session) IsInitialized() (bool, error) {
	return _IVaultV2.Contract.IsInitialized(&_IVaultV2.CallOpts)
}

// IsInitialized is a free data retrieval call binding the contract method 0x392e53cd.
//
// Solidity: function isInitialized() view returns(bool initialized)
func (_IVaultV2 *IVaultV2CallerSession) IsInitialized() (bool, error) {
	return _IVaultV2.Contract.IsInitialized(&_IVaultV2.CallOpts)
}

// LastProtocolFeeReceiver is a free data retrieval call binding the contract method 0xef9c691f.
//
// Solidity: function lastProtocolFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2Caller) LastProtocolFeeReceiver(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "lastProtocolFeeReceiver")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// LastProtocolFeeReceiver is a free data retrieval call binding the contract method 0xef9c691f.
//
// Solidity: function lastProtocolFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2Session) LastProtocolFeeReceiver() (common.Address, error) {
	return _IVaultV2.Contract.LastProtocolFeeReceiver(&_IVaultV2.CallOpts)
}

// LastProtocolFeeReceiver is a free data retrieval call binding the contract method 0xef9c691f.
//
// Solidity: function lastProtocolFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2CallerSession) LastProtocolFeeReceiver() (common.Address, error) {
	return _IVaultV2.Contract.LastProtocolFeeReceiver(&_IVaultV2.CallOpts)
}

// LastProtocolManagementFee is a free data retrieval call binding the contract method 0x192fb170.
//
// Solidity: function lastProtocolManagementFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Caller) LastProtocolManagementFee(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "lastProtocolManagementFee")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// LastProtocolManagementFee is a free data retrieval call binding the contract method 0x192fb170.
//
// Solidity: function lastProtocolManagementFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Session) LastProtocolManagementFee() (*big.Int, error) {
	return _IVaultV2.Contract.LastProtocolManagementFee(&_IVaultV2.CallOpts)
}

// LastProtocolManagementFee is a free data retrieval call binding the contract method 0x192fb170.
//
// Solidity: function lastProtocolManagementFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2CallerSession) LastProtocolManagementFee() (*big.Int, error) {
	return _IVaultV2.Contract.LastProtocolManagementFee(&_IVaultV2.CallOpts)
}

// LastProtocolPerformanceFee is a free data retrieval call binding the contract method 0xf56f1926.
//
// Solidity: function lastProtocolPerformanceFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Caller) LastProtocolPerformanceFee(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "lastProtocolPerformanceFee")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// LastProtocolPerformanceFee is a free data retrieval call binding the contract method 0xf56f1926.
//
// Solidity: function lastProtocolPerformanceFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Session) LastProtocolPerformanceFee() (*big.Int, error) {
	return _IVaultV2.Contract.LastProtocolPerformanceFee(&_IVaultV2.CallOpts)
}

// LastProtocolPerformanceFee is a free data retrieval call binding the contract method 0xf56f1926.
//
// Solidity: function lastProtocolPerformanceFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2CallerSession) LastProtocolPerformanceFee() (*big.Int, error) {
	return _IVaultV2.Contract.LastProtocolPerformanceFee(&_IVaultV2.CallOpts)
}

// LastUpdate is a free data retrieval call binding the contract method 0xc0463711.
//
// Solidity: function lastUpdate() view returns(uint48 timestamp)
func (_IVaultV2 *IVaultV2Caller) LastUpdate(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "lastUpdate")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// LastUpdate is a free data retrieval call binding the contract method 0xc0463711.
//
// Solidity: function lastUpdate() view returns(uint48 timestamp)
func (_IVaultV2 *IVaultV2Session) LastUpdate() (*big.Int, error) {
	return _IVaultV2.Contract.LastUpdate(&_IVaultV2.CallOpts)
}

// LastUpdate is a free data retrieval call binding the contract method 0xc0463711.
//
// Solidity: function lastUpdate() view returns(uint48 timestamp)
func (_IVaultV2 *IVaultV2CallerSession) LastUpdate() (*big.Int, error) {
	return _IVaultV2.Contract.LastUpdate(&_IVaultV2.CallOpts)
}

// ManagementFee is a free data retrieval call binding the contract method 0xa6f7f5d6.
//
// Solidity: function managementFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Caller) ManagementFee(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "managementFee")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ManagementFee is a free data retrieval call binding the contract method 0xa6f7f5d6.
//
// Solidity: function managementFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Session) ManagementFee() (*big.Int, error) {
	return _IVaultV2.Contract.ManagementFee(&_IVaultV2.CallOpts)
}

// ManagementFee is a free data retrieval call binding the contract method 0xa6f7f5d6.
//
// Solidity: function managementFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2CallerSession) ManagementFee() (*big.Int, error) {
	return _IVaultV2.Contract.ManagementFee(&_IVaultV2.CallOpts)
}

// ManagementFeeReceiver is a free data retrieval call binding the contract method 0x43039947.
//
// Solidity: function managementFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2Caller) ManagementFeeReceiver(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "managementFeeReceiver")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// ManagementFeeReceiver is a free data retrieval call binding the contract method 0x43039947.
//
// Solidity: function managementFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2Session) ManagementFeeReceiver() (common.Address, error) {
	return _IVaultV2.Contract.ManagementFeeReceiver(&_IVaultV2.CallOpts)
}

// ManagementFeeReceiver is a free data retrieval call binding the contract method 0x43039947.
//
// Solidity: function managementFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2CallerSession) ManagementFeeReceiver() (common.Address, error) {
	return _IVaultV2.Contract.ManagementFeeReceiver(&_IVaultV2.CallOpts)
}

// PerformanceFee is a free data retrieval call binding the contract method 0x87788782.
//
// Solidity: function performanceFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Caller) PerformanceFee(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "performanceFee")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// PerformanceFee is a free data retrieval call binding the contract method 0x87788782.
//
// Solidity: function performanceFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2Session) PerformanceFee() (*big.Int, error) {
	return _IVaultV2.Contract.PerformanceFee(&_IVaultV2.CallOpts)
}

// PerformanceFee is a free data retrieval call binding the contract method 0x87788782.
//
// Solidity: function performanceFee() view returns(uint96 fee)
func (_IVaultV2 *IVaultV2CallerSession) PerformanceFee() (*big.Int, error) {
	return _IVaultV2.Contract.PerformanceFee(&_IVaultV2.CallOpts)
}

// PerformanceFeeReceiver is a free data retrieval call binding the contract method 0x82cf16df.
//
// Solidity: function performanceFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2Caller) PerformanceFeeReceiver(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "performanceFeeReceiver")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PerformanceFeeReceiver is a free data retrieval call binding the contract method 0x82cf16df.
//
// Solidity: function performanceFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2Session) PerformanceFeeReceiver() (common.Address, error) {
	return _IVaultV2.Contract.PerformanceFeeReceiver(&_IVaultV2.CallOpts)
}

// PerformanceFeeReceiver is a free data retrieval call binding the contract method 0x82cf16df.
//
// Solidity: function performanceFeeReceiver() view returns(address receiver)
func (_IVaultV2 *IVaultV2CallerSession) PerformanceFeeReceiver() (common.Address, error) {
	return _IVaultV2.Contract.PerformanceFeeReceiver(&_IVaultV2.CallOpts)
}

// TotalSupplyAt is a free data retrieval call binding the contract method 0xe9efd4cf.
//
// Solidity: function totalSupplyAt(uint48 timestamp) view returns(uint256 supply)
func (_IVaultV2 *IVaultV2Caller) TotalSupplyAt(opts *bind.CallOpts, timestamp *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "totalSupplyAt", timestamp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupplyAt is a free data retrieval call binding the contract method 0xe9efd4cf.
//
// Solidity: function totalSupplyAt(uint48 timestamp) view returns(uint256 supply)
func (_IVaultV2 *IVaultV2Session) TotalSupplyAt(timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.TotalSupplyAt(&_IVaultV2.CallOpts, timestamp)
}

// TotalSupplyAt is a free data retrieval call binding the contract method 0xe9efd4cf.
//
// Solidity: function totalSupplyAt(uint48 timestamp) view returns(uint256 supply)
func (_IVaultV2 *IVaultV2CallerSession) TotalSupplyAt(timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.TotalSupplyAt(&_IVaultV2.CallOpts, timestamp)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_IVaultV2 *IVaultV2Caller) Version(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "version")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_IVaultV2 *IVaultV2Session) Version() (uint64, error) {
	return _IVaultV2.Contract.Version(&_IVaultV2.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_IVaultV2 *IVaultV2CallerSession) Version() (uint64, error) {
	return _IVaultV2.Contract.Version(&_IVaultV2.CallOpts)
}

// WithdrawalQueue is a free data retrieval call binding the contract method 0x37d5fe99.
//
// Solidity: function withdrawalQueue() view returns(address withdrawalQueueAddress)
func (_IVaultV2 *IVaultV2Caller) WithdrawalQueue(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawalQueue")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// WithdrawalQueue is a free data retrieval call binding the contract method 0x37d5fe99.
//
// Solidity: function withdrawalQueue() view returns(address withdrawalQueueAddress)
func (_IVaultV2 *IVaultV2Session) WithdrawalQueue() (common.Address, error) {
	return _IVaultV2.Contract.WithdrawalQueue(&_IVaultV2.CallOpts)
}

// WithdrawalQueue is a free data retrieval call binding the contract method 0x37d5fe99.
//
// Solidity: function withdrawalQueue() view returns(address withdrawalQueueAddress)
func (_IVaultV2 *IVaultV2CallerSession) WithdrawalQueue() (common.Address, error) {
	return _IVaultV2.Contract.WithdrawalQueue(&_IVaultV2.CallOpts)
}

// AccrueInterest is a paid mutator transaction binding the contract method 0xa6afed95.
//
// Solidity: function accrueInterest() returns(uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2Transactor) AccrueInterest(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "accrueInterest")
}

// AccrueInterest is a paid mutator transaction binding the contract method 0xa6afed95.
//
// Solidity: function accrueInterest() returns(uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2Session) AccrueInterest() (*types.Transaction, error) {
	return _IVaultV2.Contract.AccrueInterest(&_IVaultV2.TransactOpts)
}

// AccrueInterest is a paid mutator transaction binding the contract method 0xa6afed95.
//
// Solidity: function accrueInterest() returns(uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2TransactorSession) AccrueInterest() (*types.Transaction, error) {
	return _IVaultV2.Contract.AccrueInterest(&_IVaultV2.TransactOpts)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (_IVaultV2 *IVaultV2Transactor) Initialize(opts *bind.TransactOpts, initialVersion uint64, owner common.Address, data []byte) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "initialize", initialVersion, owner, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (_IVaultV2 *IVaultV2Session) Initialize(initialVersion uint64, owner common.Address, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Initialize(&_IVaultV2.TransactOpts, initialVersion, owner, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Initialize(initialVersion uint64, owner common.Address, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Initialize(&_IVaultV2.TransactOpts, initialVersion, owner, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_IVaultV2 *IVaultV2Transactor) Migrate(opts *bind.TransactOpts, newVersion uint64, data []byte) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "migrate", newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_IVaultV2 *IVaultV2Session) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Migrate(&_IVaultV2.TransactOpts, newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Migrate(&_IVaultV2.TransactOpts, newVersion, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_IVaultV2 *IVaultV2Transactor) Multicall(opts *bind.TransactOpts, data [][]byte) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "multicall", data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_IVaultV2 *IVaultV2Session) Multicall(data [][]byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Multicall(&_IVaultV2.TransactOpts, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Multicall(&_IVaultV2.TransactOpts, data)
}

// Pull is a paid mutator transaction binding the contract method 0x97b41a12.
//
// Solidity: function pull(uint256 assets, address receiver) returns()
func (_IVaultV2 *IVaultV2Transactor) Pull(opts *bind.TransactOpts, assets *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "pull", assets, receiver)
}

// Pull is a paid mutator transaction binding the contract method 0x97b41a12.
//
// Solidity: function pull(uint256 assets, address receiver) returns()
func (_IVaultV2 *IVaultV2Session) Pull(assets *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.Pull(&_IVaultV2.TransactOpts, assets, receiver)
}

// Pull is a paid mutator transaction binding the contract method 0x97b41a12.
//
// Solidity: function pull(uint256 assets, address receiver) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Pull(assets *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.Pull(&_IVaultV2.TransactOpts, assets, receiver)
}

// Push is a paid mutator transaction binding the contract method 0xc80fbe4e.
//
// Solidity: function push(uint256 assets, address owner) returns()
func (_IVaultV2 *IVaultV2Transactor) Push(opts *bind.TransactOpts, assets *big.Int, owner common.Address) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "push", assets, owner)
}

// Push is a paid mutator transaction binding the contract method 0xc80fbe4e.
//
// Solidity: function push(uint256 assets, address owner) returns()
func (_IVaultV2 *IVaultV2Session) Push(assets *big.Int, owner common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.Push(&_IVaultV2.TransactOpts, assets, owner)
}

// Push is a paid mutator transaction binding the contract method 0xc80fbe4e.
//
// Solidity: function push(uint256 assets, address owner) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Push(assets *big.Int, owner common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.Push(&_IVaultV2.TransactOpts, assets, owner)
}

// Redeemable is a paid mutator transaction binding the contract method 0x2d7ecd11.
//
// Solidity: function redeemable() returns(uint256 shares)
func (_IVaultV2 *IVaultV2Transactor) Redeemable(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "redeemable")
}

// Redeemable is a paid mutator transaction binding the contract method 0x2d7ecd11.
//
// Solidity: function redeemable() returns(uint256 shares)
func (_IVaultV2 *IVaultV2Session) Redeemable() (*types.Transaction, error) {
	return _IVaultV2.Contract.Redeemable(&_IVaultV2.TransactOpts)
}

// Redeemable is a paid mutator transaction binding the contract method 0x2d7ecd11.
//
// Solidity: function redeemable() returns(uint256 shares)
func (_IVaultV2 *IVaultV2TransactorSession) Redeemable() (*types.Transaction, error) {
	return _IVaultV2.Contract.Redeemable(&_IVaultV2.TransactOpts)
}

// SetDelegator is a paid mutator transaction binding the contract method 0x83cd9cc3.
//
// Solidity: function setDelegator(address delegator) returns()
func (_IVaultV2 *IVaultV2Transactor) SetDelegator(opts *bind.TransactOpts, delegator common.Address) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setDelegator", delegator)
}

// SetDelegator is a paid mutator transaction binding the contract method 0x83cd9cc3.
//
// Solidity: function setDelegator(address delegator) returns()
func (_IVaultV2 *IVaultV2Session) SetDelegator(delegator common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDelegator(&_IVaultV2.TransactOpts, delegator)
}

// SetDelegator is a paid mutator transaction binding the contract method 0x83cd9cc3.
//
// Solidity: function setDelegator(address delegator) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetDelegator(delegator common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDelegator(&_IVaultV2.TransactOpts, delegator)
}

// SetDepositLimit is a paid mutator transaction binding the contract method 0xbdc8144b.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (_IVaultV2 *IVaultV2Transactor) SetDepositLimit(opts *bind.TransactOpts, limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setDepositLimit", limit)
}

// SetDepositLimit is a paid mutator transaction binding the contract method 0xbdc8144b.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (_IVaultV2 *IVaultV2Session) SetDepositLimit(limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositLimit(&_IVaultV2.TransactOpts, limit)
}

// SetDepositLimit is a paid mutator transaction binding the contract method 0xbdc8144b.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetDepositLimit(limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositLimit(&_IVaultV2.TransactOpts, limit)
}

// SetDepositWhitelist is a paid mutator transaction binding the contract method 0x4105a7dd.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (_IVaultV2 *IVaultV2Transactor) SetDepositWhitelist(opts *bind.TransactOpts, status bool) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setDepositWhitelist", status)
}

// SetDepositWhitelist is a paid mutator transaction binding the contract method 0x4105a7dd.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (_IVaultV2 *IVaultV2Session) SetDepositWhitelist(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositWhitelist(&_IVaultV2.TransactOpts, status)
}

// SetDepositWhitelist is a paid mutator transaction binding the contract method 0x4105a7dd.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetDepositWhitelist(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositWhitelist(&_IVaultV2.TransactOpts, status)
}

// SetDepositorWhitelistStatus is a paid mutator transaction binding the contract method 0xa2861466.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (_IVaultV2 *IVaultV2Transactor) SetDepositorWhitelistStatus(opts *bind.TransactOpts, account common.Address, status bool) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setDepositorWhitelistStatus", account, status)
}

// SetDepositorWhitelistStatus is a paid mutator transaction binding the contract method 0xa2861466.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (_IVaultV2 *IVaultV2Session) SetDepositorWhitelistStatus(account common.Address, status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositorWhitelistStatus(&_IVaultV2.TransactOpts, account, status)
}

// SetDepositorWhitelistStatus is a paid mutator transaction binding the contract method 0xa2861466.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetDepositorWhitelistStatus(account common.Address, status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositorWhitelistStatus(&_IVaultV2.TransactOpts, account, status)
}

// SetIsDepositLimit is a paid mutator transaction binding the contract method 0x5346e34f.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (_IVaultV2 *IVaultV2Transactor) SetIsDepositLimit(opts *bind.TransactOpts, status bool) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setIsDepositLimit", status)
}

// SetIsDepositLimit is a paid mutator transaction binding the contract method 0x5346e34f.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (_IVaultV2 *IVaultV2Session) SetIsDepositLimit(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetIsDepositLimit(&_IVaultV2.TransactOpts, status)
}

// SetIsDepositLimit is a paid mutator transaction binding the contract method 0x5346e34f.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetIsDepositLimit(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetIsDepositLimit(&_IVaultV2.TransactOpts, status)
}

// SetManagementFee is a paid mutator transaction binding the contract method 0xf5cd33d2.
//
// Solidity: function setManagementFee(uint96 fee, address receiver) returns()
func (_IVaultV2 *IVaultV2Transactor) SetManagementFee(opts *bind.TransactOpts, fee *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setManagementFee", fee, receiver)
}

// SetManagementFee is a paid mutator transaction binding the contract method 0xf5cd33d2.
//
// Solidity: function setManagementFee(uint96 fee, address receiver) returns()
func (_IVaultV2 *IVaultV2Session) SetManagementFee(fee *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetManagementFee(&_IVaultV2.TransactOpts, fee, receiver)
}

// SetManagementFee is a paid mutator transaction binding the contract method 0xf5cd33d2.
//
// Solidity: function setManagementFee(uint96 fee, address receiver) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetManagementFee(fee *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetManagementFee(&_IVaultV2.TransactOpts, fee, receiver)
}

// SetPerformanceFee is a paid mutator transaction binding the contract method 0xc4fef03e.
//
// Solidity: function setPerformanceFee(uint96 fee, address receiver) returns()
func (_IVaultV2 *IVaultV2Transactor) SetPerformanceFee(opts *bind.TransactOpts, fee *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setPerformanceFee", fee, receiver)
}

// SetPerformanceFee is a paid mutator transaction binding the contract method 0xc4fef03e.
//
// Solidity: function setPerformanceFee(uint96 fee, address receiver) returns()
func (_IVaultV2 *IVaultV2Session) SetPerformanceFee(fee *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetPerformanceFee(&_IVaultV2.TransactOpts, fee, receiver)
}

// SetPerformanceFee is a paid mutator transaction binding the contract method 0xc4fef03e.
//
// Solidity: function setPerformanceFee(uint96 fee, address receiver) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetPerformanceFee(fee *big.Int, receiver common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetPerformanceFee(&_IVaultV2.TransactOpts, fee, receiver)
}

// SetSlasher is a paid mutator transaction binding the contract method 0xaabc2496.
//
// Solidity: function setSlasher(address slasher) returns()
func (_IVaultV2 *IVaultV2Transactor) SetSlasher(opts *bind.TransactOpts, slasher common.Address) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setSlasher", slasher)
}

// SetSlasher is a paid mutator transaction binding the contract method 0xaabc2496.
//
// Solidity: function setSlasher(address slasher) returns()
func (_IVaultV2 *IVaultV2Session) SetSlasher(slasher common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetSlasher(&_IVaultV2.TransactOpts, slasher)
}

// SetSlasher is a paid mutator transaction binding the contract method 0xaabc2496.
//
// Solidity: function setSlasher(address slasher) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetSlasher(slasher common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetSlasher(&_IVaultV2.TransactOpts, slasher)
}

// Withdrawable is a paid mutator transaction binding the contract method 0x50188301.
//
// Solidity: function withdrawable() returns(uint256 assets)
func (_IVaultV2 *IVaultV2Transactor) Withdrawable(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "withdrawable")
}

// Withdrawable is a paid mutator transaction binding the contract method 0x50188301.
//
// Solidity: function withdrawable() returns(uint256 assets)
func (_IVaultV2 *IVaultV2Session) Withdrawable() (*types.Transaction, error) {
	return _IVaultV2.Contract.Withdrawable(&_IVaultV2.TransactOpts)
}

// Withdrawable is a paid mutator transaction binding the contract method 0x50188301.
//
// Solidity: function withdrawable() returns(uint256 assets)
func (_IVaultV2 *IVaultV2TransactorSession) Withdrawable() (*types.Transaction, error) {
	return _IVaultV2.Contract.Withdrawable(&_IVaultV2.TransactOpts)
}

// IVaultV2AccrueInterestIterator is returned from FilterAccrueInterest and is used to iterate over the raw logs and unpacked data for AccrueInterest events raised by the IVaultV2 contract.
type IVaultV2AccrueInterestIterator struct {
	Event *IVaultV2AccrueInterest // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2AccrueInterestIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2AccrueInterest)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2AccrueInterest)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2AccrueInterestIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2AccrueInterestIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2AccrueInterest represents a AccrueInterest event raised by the IVaultV2 contract.
type IVaultV2AccrueInterest struct {
	NewTotalAssets       *big.Int
	ManagementFeeShares  *big.Int
	PerformanceFeeShares *big.Int
	ProtocolFeeShares    *big.Int
	Raw                  types.Log // Blockchain specific contextual infos
}

// FilterAccrueInterest is a free log retrieval operation binding the contract event 0x4dec04e750ca11537cabcd8a9eab06494de08da3735bc8871cd41250e190bc04.
//
// Solidity: event AccrueInterest(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2Filterer) FilterAccrueInterest(opts *bind.FilterOpts) (*IVaultV2AccrueInterestIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "AccrueInterest")
	if err != nil {
		return nil, err
	}
	return &IVaultV2AccrueInterestIterator{contract: _IVaultV2.contract, event: "AccrueInterest", logs: logs, sub: sub}, nil
}

// WatchAccrueInterest is a free log subscription operation binding the contract event 0x4dec04e750ca11537cabcd8a9eab06494de08da3735bc8871cd41250e190bc04.
//
// Solidity: event AccrueInterest(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2Filterer) WatchAccrueInterest(opts *bind.WatchOpts, sink chan<- *IVaultV2AccrueInterest) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "AccrueInterest")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2AccrueInterest)
				if err := _IVaultV2.contract.UnpackLog(event, "AccrueInterest", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseAccrueInterest is a log parse operation binding the contract event 0x4dec04e750ca11537cabcd8a9eab06494de08da3735bc8871cd41250e190bc04.
//
// Solidity: event AccrueInterest(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (_IVaultV2 *IVaultV2Filterer) ParseAccrueInterest(log types.Log) (*IVaultV2AccrueInterest, error) {
	event := new(IVaultV2AccrueInterest)
	if err := _IVaultV2.contract.UnpackLog(event, "AccrueInterest", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2ClaimIterator is returned from FilterClaim and is used to iterate over the raw logs and unpacked data for Claim events raised by the IVaultV2 contract.
type IVaultV2ClaimIterator struct {
	Event *IVaultV2Claim // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2ClaimIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Claim)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2Claim)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2ClaimIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2ClaimIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Claim represents a Claim event raised by the IVaultV2 contract.
type IVaultV2Claim struct {
	Claimer  common.Address
	Receiver common.Address
	TokenId  *big.Int
	Assets   *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterClaim is a free log retrieval operation binding the contract event 0x865ca08d59f5cb456e85cd2f7ef63664ea4f73327414e9d8152c4158b0e94645.
//
// Solidity: event Claim(address indexed claimer, address indexed receiver, uint256 tokenId, uint256 assets)
func (_IVaultV2 *IVaultV2Filterer) FilterClaim(opts *bind.FilterOpts, claimer []common.Address, receiver []common.Address) (*IVaultV2ClaimIterator, error) {

	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Claim", claimerRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2ClaimIterator{contract: _IVaultV2.contract, event: "Claim", logs: logs, sub: sub}, nil
}

// WatchClaim is a free log subscription operation binding the contract event 0x865ca08d59f5cb456e85cd2f7ef63664ea4f73327414e9d8152c4158b0e94645.
//
// Solidity: event Claim(address indexed claimer, address indexed receiver, uint256 tokenId, uint256 assets)
func (_IVaultV2 *IVaultV2Filterer) WatchClaim(opts *bind.WatchOpts, sink chan<- *IVaultV2Claim, claimer []common.Address, receiver []common.Address) (event.Subscription, error) {

	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Claim", claimerRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Claim)
				if err := _IVaultV2.contract.UnpackLog(event, "Claim", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseClaim is a log parse operation binding the contract event 0x865ca08d59f5cb456e85cd2f7ef63664ea4f73327414e9d8152c4158b0e94645.
//
// Solidity: event Claim(address indexed claimer, address indexed receiver, uint256 tokenId, uint256 assets)
func (_IVaultV2 *IVaultV2Filterer) ParseClaim(log types.Log) (*IVaultV2Claim, error) {
	event := new(IVaultV2Claim)
	if err := _IVaultV2.contract.UnpackLog(event, "Claim", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2InitializeIterator is returned from FilterInitialize and is used to iterate over the raw logs and unpacked data for Initialize events raised by the IVaultV2 contract.
type IVaultV2InitializeIterator struct {
	Event *IVaultV2Initialize // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2InitializeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Initialize)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2Initialize)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2InitializeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2InitializeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Initialize represents a Initialize event raised by the IVaultV2 contract.
type IVaultV2Initialize struct {
	Params IVaultV2InitParams
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterInitialize is a free log retrieval operation binding the contract event 0xbbf5a13edd1b1ed3dbe1bcabac683ad6bad3a11cdbec977dd2a462fde3805a14.
//
// Solidity: event Initialize((string,string,address,bool,address,uint256,bool,address,address,address,address,address,address,address) params)
func (_IVaultV2 *IVaultV2Filterer) FilterInitialize(opts *bind.FilterOpts) (*IVaultV2InitializeIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return &IVaultV2InitializeIterator{contract: _IVaultV2.contract, event: "Initialize", logs: logs, sub: sub}, nil
}

// WatchInitialize is a free log subscription operation binding the contract event 0xbbf5a13edd1b1ed3dbe1bcabac683ad6bad3a11cdbec977dd2a462fde3805a14.
//
// Solidity: event Initialize((string,string,address,bool,address,uint256,bool,address,address,address,address,address,address,address) params)
func (_IVaultV2 *IVaultV2Filterer) WatchInitialize(opts *bind.WatchOpts, sink chan<- *IVaultV2Initialize) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Initialize)
				if err := _IVaultV2.contract.UnpackLog(event, "Initialize", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInitialize is a log parse operation binding the contract event 0xbbf5a13edd1b1ed3dbe1bcabac683ad6bad3a11cdbec977dd2a462fde3805a14.
//
// Solidity: event Initialize((string,string,address,bool,address,uint256,bool,address,address,address,address,address,address,address) params)
func (_IVaultV2 *IVaultV2Filterer) ParseInitialize(log types.Log) (*IVaultV2Initialize, error) {
	event := new(IVaultV2Initialize)
	if err := _IVaultV2.contract.UnpackLog(event, "Initialize", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2PullIterator is returned from FilterPull and is used to iterate over the raw logs and unpacked data for Pull events raised by the IVaultV2 contract.
type IVaultV2PullIterator struct {
	Event *IVaultV2Pull // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2PullIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Pull)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2Pull)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2PullIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2PullIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Pull represents a Pull event raised by the IVaultV2 contract.
type IVaultV2Pull struct {
	Assets   *big.Int
	Receiver common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterPull is a free log retrieval operation binding the contract event 0xd732b38a108d3e36408cf50fc7a4c5223556e15155834fac3927a92363f2d78e.
//
// Solidity: event Pull(uint256 assets, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) FilterPull(opts *bind.FilterOpts, receiver []common.Address) (*IVaultV2PullIterator, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Pull", receiverRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2PullIterator{contract: _IVaultV2.contract, event: "Pull", logs: logs, sub: sub}, nil
}

// WatchPull is a free log subscription operation binding the contract event 0xd732b38a108d3e36408cf50fc7a4c5223556e15155834fac3927a92363f2d78e.
//
// Solidity: event Pull(uint256 assets, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) WatchPull(opts *bind.WatchOpts, sink chan<- *IVaultV2Pull, receiver []common.Address) (event.Subscription, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Pull", receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Pull)
				if err := _IVaultV2.contract.UnpackLog(event, "Pull", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParsePull is a log parse operation binding the contract event 0xd732b38a108d3e36408cf50fc7a4c5223556e15155834fac3927a92363f2d78e.
//
// Solidity: event Pull(uint256 assets, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) ParsePull(log types.Log) (*IVaultV2Pull, error) {
	event := new(IVaultV2Pull)
	if err := _IVaultV2.contract.UnpackLog(event, "Pull", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2PushIterator is returned from FilterPush and is used to iterate over the raw logs and unpacked data for Push events raised by the IVaultV2 contract.
type IVaultV2PushIterator struct {
	Event *IVaultV2Push // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2PushIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Push)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2Push)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2PushIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2PushIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Push represents a Push event raised by the IVaultV2 contract.
type IVaultV2Push struct {
	Assets *big.Int
	Owner  common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterPush is a free log retrieval operation binding the contract event 0x7e983771788a1b65d6dc0ecdf3fbc77d2d354dc3748aec0a4dcb505a28295e2a.
//
// Solidity: event Push(uint256 assets, address indexed owner)
func (_IVaultV2 *IVaultV2Filterer) FilterPush(opts *bind.FilterOpts, owner []common.Address) (*IVaultV2PushIterator, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Push", ownerRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2PushIterator{contract: _IVaultV2.contract, event: "Push", logs: logs, sub: sub}, nil
}

// WatchPush is a free log subscription operation binding the contract event 0x7e983771788a1b65d6dc0ecdf3fbc77d2d354dc3748aec0a4dcb505a28295e2a.
//
// Solidity: event Push(uint256 assets, address indexed owner)
func (_IVaultV2 *IVaultV2Filterer) WatchPush(opts *bind.WatchOpts, sink chan<- *IVaultV2Push, owner []common.Address) (event.Subscription, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Push", ownerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Push)
				if err := _IVaultV2.contract.UnpackLog(event, "Push", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParsePush is a log parse operation binding the contract event 0x7e983771788a1b65d6dc0ecdf3fbc77d2d354dc3748aec0a4dcb505a28295e2a.
//
// Solidity: event Push(uint256 assets, address indexed owner)
func (_IVaultV2 *IVaultV2Filterer) ParsePush(log types.Log) (*IVaultV2Push, error) {
	event := new(IVaultV2Push)
	if err := _IVaultV2.contract.UnpackLog(event, "Push", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDelegatorIterator is returned from FilterSetDelegator and is used to iterate over the raw logs and unpacked data for SetDelegator events raised by the IVaultV2 contract.
type IVaultV2SetDelegatorIterator struct {
	Event *IVaultV2SetDelegator // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetDelegatorIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDelegator)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetDelegator)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetDelegatorIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDelegatorIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDelegator represents a SetDelegator event raised by the IVaultV2 contract.
type IVaultV2SetDelegator struct {
	Delegator common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterSetDelegator is a free log retrieval operation binding the contract event 0xdb2160616f776a37b24808115554e79439bf26cccbbd4438190cc6d28e80ecd1.
//
// Solidity: event SetDelegator(address indexed delegator)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDelegator(opts *bind.FilterOpts, delegator []common.Address) (*IVaultV2SetDelegatorIterator, error) {

	var delegatorRule []interface{}
	for _, delegatorItem := range delegator {
		delegatorRule = append(delegatorRule, delegatorItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDelegator", delegatorRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDelegatorIterator{contract: _IVaultV2.contract, event: "SetDelegator", logs: logs, sub: sub}, nil
}

// WatchSetDelegator is a free log subscription operation binding the contract event 0xdb2160616f776a37b24808115554e79439bf26cccbbd4438190cc6d28e80ecd1.
//
// Solidity: event SetDelegator(address indexed delegator)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDelegator(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDelegator, delegator []common.Address) (event.Subscription, error) {

	var delegatorRule []interface{}
	for _, delegatorItem := range delegator {
		delegatorRule = append(delegatorRule, delegatorItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDelegator", delegatorRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDelegator)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDelegator", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetDelegator is a log parse operation binding the contract event 0xdb2160616f776a37b24808115554e79439bf26cccbbd4438190cc6d28e80ecd1.
//
// Solidity: event SetDelegator(address indexed delegator)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDelegator(log types.Log) (*IVaultV2SetDelegator, error) {
	event := new(IVaultV2SetDelegator)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDelegator", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDepositLimitIterator is returned from FilterSetDepositLimit and is used to iterate over the raw logs and unpacked data for SetDepositLimit events raised by the IVaultV2 contract.
type IVaultV2SetDepositLimitIterator struct {
	Event *IVaultV2SetDepositLimit // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetDepositLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDepositLimit)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetDepositLimit)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetDepositLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDepositLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDepositLimit represents a SetDepositLimit event raised by the IVaultV2 contract.
type IVaultV2SetDepositLimit struct {
	Limit *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterSetDepositLimit is a free log retrieval operation binding the contract event 0x854df3eb95564502c8bc871ebdd15310ee26270f955f6c6bd8cea68e75045bc0.
//
// Solidity: event SetDepositLimit(uint256 limit)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDepositLimit(opts *bind.FilterOpts) (*IVaultV2SetDepositLimitIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDepositLimit")
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDepositLimitIterator{contract: _IVaultV2.contract, event: "SetDepositLimit", logs: logs, sub: sub}, nil
}

// WatchSetDepositLimit is a free log subscription operation binding the contract event 0x854df3eb95564502c8bc871ebdd15310ee26270f955f6c6bd8cea68e75045bc0.
//
// Solidity: event SetDepositLimit(uint256 limit)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDepositLimit(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDepositLimit) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDepositLimit")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDepositLimit)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDepositLimit", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetDepositLimit is a log parse operation binding the contract event 0x854df3eb95564502c8bc871ebdd15310ee26270f955f6c6bd8cea68e75045bc0.
//
// Solidity: event SetDepositLimit(uint256 limit)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDepositLimit(log types.Log) (*IVaultV2SetDepositLimit, error) {
	event := new(IVaultV2SetDepositLimit)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDepositLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDepositWhitelistIterator is returned from FilterSetDepositWhitelist and is used to iterate over the raw logs and unpacked data for SetDepositWhitelist events raised by the IVaultV2 contract.
type IVaultV2SetDepositWhitelistIterator struct {
	Event *IVaultV2SetDepositWhitelist // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetDepositWhitelistIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDepositWhitelist)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetDepositWhitelist)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetDepositWhitelistIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDepositWhitelistIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDepositWhitelist represents a SetDepositWhitelist event raised by the IVaultV2 contract.
type IVaultV2SetDepositWhitelist struct {
	Status bool
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSetDepositWhitelist is a free log retrieval operation binding the contract event 0x3e12b7b36c75ac9609a3f58609b331210428e1a85909132638955ba0301eec33.
//
// Solidity: event SetDepositWhitelist(bool status)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDepositWhitelist(opts *bind.FilterOpts) (*IVaultV2SetDepositWhitelistIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDepositWhitelist")
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDepositWhitelistIterator{contract: _IVaultV2.contract, event: "SetDepositWhitelist", logs: logs, sub: sub}, nil
}

// WatchSetDepositWhitelist is a free log subscription operation binding the contract event 0x3e12b7b36c75ac9609a3f58609b331210428e1a85909132638955ba0301eec33.
//
// Solidity: event SetDepositWhitelist(bool status)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDepositWhitelist(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDepositWhitelist) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDepositWhitelist")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDepositWhitelist)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDepositWhitelist", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetDepositWhitelist is a log parse operation binding the contract event 0x3e12b7b36c75ac9609a3f58609b331210428e1a85909132638955ba0301eec33.
//
// Solidity: event SetDepositWhitelist(bool status)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDepositWhitelist(log types.Log) (*IVaultV2SetDepositWhitelist, error) {
	event := new(IVaultV2SetDepositWhitelist)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDepositWhitelist", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDepositorWhitelistStatusIterator is returned from FilterSetDepositorWhitelistStatus and is used to iterate over the raw logs and unpacked data for SetDepositorWhitelistStatus events raised by the IVaultV2 contract.
type IVaultV2SetDepositorWhitelistStatusIterator struct {
	Event *IVaultV2SetDepositorWhitelistStatus // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetDepositorWhitelistStatusIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDepositorWhitelistStatus)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetDepositorWhitelistStatus)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetDepositorWhitelistStatusIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDepositorWhitelistStatusIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDepositorWhitelistStatus represents a SetDepositorWhitelistStatus event raised by the IVaultV2 contract.
type IVaultV2SetDepositorWhitelistStatus struct {
	Account common.Address
	Status  bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterSetDepositorWhitelistStatus is a free log retrieval operation binding the contract event 0xf991b1ecfb5115cbb36a2b2e2240c058406d2acc2fcc6e9e2dc99d845ff70a62.
//
// Solidity: event SetDepositorWhitelistStatus(address indexed account, bool status)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDepositorWhitelistStatus(opts *bind.FilterOpts, account []common.Address) (*IVaultV2SetDepositorWhitelistStatusIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDepositorWhitelistStatus", accountRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDepositorWhitelistStatusIterator{contract: _IVaultV2.contract, event: "SetDepositorWhitelistStatus", logs: logs, sub: sub}, nil
}

// WatchSetDepositorWhitelistStatus is a free log subscription operation binding the contract event 0xf991b1ecfb5115cbb36a2b2e2240c058406d2acc2fcc6e9e2dc99d845ff70a62.
//
// Solidity: event SetDepositorWhitelistStatus(address indexed account, bool status)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDepositorWhitelistStatus(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDepositorWhitelistStatus, account []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDepositorWhitelistStatus", accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDepositorWhitelistStatus)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDepositorWhitelistStatus", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetDepositorWhitelistStatus is a log parse operation binding the contract event 0xf991b1ecfb5115cbb36a2b2e2240c058406d2acc2fcc6e9e2dc99d845ff70a62.
//
// Solidity: event SetDepositorWhitelistStatus(address indexed account, bool status)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDepositorWhitelistStatus(log types.Log) (*IVaultV2SetDepositorWhitelistStatus, error) {
	event := new(IVaultV2SetDepositorWhitelistStatus)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDepositorWhitelistStatus", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetIsDepositLimitIterator is returned from FilterSetIsDepositLimit and is used to iterate over the raw logs and unpacked data for SetIsDepositLimit events raised by the IVaultV2 contract.
type IVaultV2SetIsDepositLimitIterator struct {
	Event *IVaultV2SetIsDepositLimit // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetIsDepositLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetIsDepositLimit)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetIsDepositLimit)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetIsDepositLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetIsDepositLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetIsDepositLimit represents a SetIsDepositLimit event raised by the IVaultV2 contract.
type IVaultV2SetIsDepositLimit struct {
	Status bool
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSetIsDepositLimit is a free log retrieval operation binding the contract event 0xfa7a25a0b611d4ba3c0ea990e90dc23d484a5dd7a1be4733fef2946ba74530c6.
//
// Solidity: event SetIsDepositLimit(bool status)
func (_IVaultV2 *IVaultV2Filterer) FilterSetIsDepositLimit(opts *bind.FilterOpts) (*IVaultV2SetIsDepositLimitIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetIsDepositLimit")
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetIsDepositLimitIterator{contract: _IVaultV2.contract, event: "SetIsDepositLimit", logs: logs, sub: sub}, nil
}

// WatchSetIsDepositLimit is a free log subscription operation binding the contract event 0xfa7a25a0b611d4ba3c0ea990e90dc23d484a5dd7a1be4733fef2946ba74530c6.
//
// Solidity: event SetIsDepositLimit(bool status)
func (_IVaultV2 *IVaultV2Filterer) WatchSetIsDepositLimit(opts *bind.WatchOpts, sink chan<- *IVaultV2SetIsDepositLimit) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetIsDepositLimit")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetIsDepositLimit)
				if err := _IVaultV2.contract.UnpackLog(event, "SetIsDepositLimit", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetIsDepositLimit is a log parse operation binding the contract event 0xfa7a25a0b611d4ba3c0ea990e90dc23d484a5dd7a1be4733fef2946ba74530c6.
//
// Solidity: event SetIsDepositLimit(bool status)
func (_IVaultV2 *IVaultV2Filterer) ParseSetIsDepositLimit(log types.Log) (*IVaultV2SetIsDepositLimit, error) {
	event := new(IVaultV2SetIsDepositLimit)
	if err := _IVaultV2.contract.UnpackLog(event, "SetIsDepositLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetManagementFeeIterator is returned from FilterSetManagementFee and is used to iterate over the raw logs and unpacked data for SetManagementFee events raised by the IVaultV2 contract.
type IVaultV2SetManagementFeeIterator struct {
	Event *IVaultV2SetManagementFee // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetManagementFeeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetManagementFee)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetManagementFee)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetManagementFeeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetManagementFeeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetManagementFee represents a SetManagementFee event raised by the IVaultV2 contract.
type IVaultV2SetManagementFee struct {
	Fee      *big.Int
	Receiver common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterSetManagementFee is a free log retrieval operation binding the contract event 0x7fdd7f02425820fbdb2857a5af2bcf61347a9dc00fb775254391a87d89dc3c22.
//
// Solidity: event SetManagementFee(uint256 fee, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) FilterSetManagementFee(opts *bind.FilterOpts, receiver []common.Address) (*IVaultV2SetManagementFeeIterator, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetManagementFee", receiverRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetManagementFeeIterator{contract: _IVaultV2.contract, event: "SetManagementFee", logs: logs, sub: sub}, nil
}

// WatchSetManagementFee is a free log subscription operation binding the contract event 0x7fdd7f02425820fbdb2857a5af2bcf61347a9dc00fb775254391a87d89dc3c22.
//
// Solidity: event SetManagementFee(uint256 fee, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) WatchSetManagementFee(opts *bind.WatchOpts, sink chan<- *IVaultV2SetManagementFee, receiver []common.Address) (event.Subscription, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetManagementFee", receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetManagementFee)
				if err := _IVaultV2.contract.UnpackLog(event, "SetManagementFee", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetManagementFee is a log parse operation binding the contract event 0x7fdd7f02425820fbdb2857a5af2bcf61347a9dc00fb775254391a87d89dc3c22.
//
// Solidity: event SetManagementFee(uint256 fee, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) ParseSetManagementFee(log types.Log) (*IVaultV2SetManagementFee, error) {
	event := new(IVaultV2SetManagementFee)
	if err := _IVaultV2.contract.UnpackLog(event, "SetManagementFee", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetPerformanceFeeIterator is returned from FilterSetPerformanceFee and is used to iterate over the raw logs and unpacked data for SetPerformanceFee events raised by the IVaultV2 contract.
type IVaultV2SetPerformanceFeeIterator struct {
	Event *IVaultV2SetPerformanceFee // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetPerformanceFeeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetPerformanceFee)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetPerformanceFee)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetPerformanceFeeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetPerformanceFeeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetPerformanceFee represents a SetPerformanceFee event raised by the IVaultV2 contract.
type IVaultV2SetPerformanceFee struct {
	Fee      *big.Int
	Receiver common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterSetPerformanceFee is a free log retrieval operation binding the contract event 0xb34da6bc684962bf7736edd02c9595d94cf7a469ab5df1e3edc693a7cb332d93.
//
// Solidity: event SetPerformanceFee(uint256 fee, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) FilterSetPerformanceFee(opts *bind.FilterOpts, receiver []common.Address) (*IVaultV2SetPerformanceFeeIterator, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetPerformanceFee", receiverRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetPerformanceFeeIterator{contract: _IVaultV2.contract, event: "SetPerformanceFee", logs: logs, sub: sub}, nil
}

// WatchSetPerformanceFee is a free log subscription operation binding the contract event 0xb34da6bc684962bf7736edd02c9595d94cf7a469ab5df1e3edc693a7cb332d93.
//
// Solidity: event SetPerformanceFee(uint256 fee, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) WatchSetPerformanceFee(opts *bind.WatchOpts, sink chan<- *IVaultV2SetPerformanceFee, receiver []common.Address) (event.Subscription, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetPerformanceFee", receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetPerformanceFee)
				if err := _IVaultV2.contract.UnpackLog(event, "SetPerformanceFee", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetPerformanceFee is a log parse operation binding the contract event 0xb34da6bc684962bf7736edd02c9595d94cf7a469ab5df1e3edc693a7cb332d93.
//
// Solidity: event SetPerformanceFee(uint256 fee, address indexed receiver)
func (_IVaultV2 *IVaultV2Filterer) ParseSetPerformanceFee(log types.Log) (*IVaultV2SetPerformanceFee, error) {
	event := new(IVaultV2SetPerformanceFee)
	if err := _IVaultV2.contract.UnpackLog(event, "SetPerformanceFee", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetWithdrawalQueueIterator is returned from FilterSetWithdrawalQueue and is used to iterate over the raw logs and unpacked data for SetWithdrawalQueue events raised by the IVaultV2 contract.
type IVaultV2SetWithdrawalQueueIterator struct {
	Event *IVaultV2SetWithdrawalQueue // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2SetWithdrawalQueueIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetWithdrawalQueue)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2SetWithdrawalQueue)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2SetWithdrawalQueueIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetWithdrawalQueueIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetWithdrawalQueue represents a SetWithdrawalQueue event raised by the IVaultV2 contract.
type IVaultV2SetWithdrawalQueue struct {
	WithdrawalQueue common.Address
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterSetWithdrawalQueue is a free log retrieval operation binding the contract event 0x754f348799e85fd8a8e2c9273d677d3e7275ca1e49dd980c9d5f5f0f0134e091.
//
// Solidity: event SetWithdrawalQueue(address indexed withdrawalQueue)
func (_IVaultV2 *IVaultV2Filterer) FilterSetWithdrawalQueue(opts *bind.FilterOpts, withdrawalQueue []common.Address) (*IVaultV2SetWithdrawalQueueIterator, error) {

	var withdrawalQueueRule []interface{}
	for _, withdrawalQueueItem := range withdrawalQueue {
		withdrawalQueueRule = append(withdrawalQueueRule, withdrawalQueueItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetWithdrawalQueue", withdrawalQueueRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetWithdrawalQueueIterator{contract: _IVaultV2.contract, event: "SetWithdrawalQueue", logs: logs, sub: sub}, nil
}

// WatchSetWithdrawalQueue is a free log subscription operation binding the contract event 0x754f348799e85fd8a8e2c9273d677d3e7275ca1e49dd980c9d5f5f0f0134e091.
//
// Solidity: event SetWithdrawalQueue(address indexed withdrawalQueue)
func (_IVaultV2 *IVaultV2Filterer) WatchSetWithdrawalQueue(opts *bind.WatchOpts, sink chan<- *IVaultV2SetWithdrawalQueue, withdrawalQueue []common.Address) (event.Subscription, error) {

	var withdrawalQueueRule []interface{}
	for _, withdrawalQueueItem := range withdrawalQueue {
		withdrawalQueueRule = append(withdrawalQueueRule, withdrawalQueueItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetWithdrawalQueue", withdrawalQueueRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetWithdrawalQueue)
				if err := _IVaultV2.contract.UnpackLog(event, "SetWithdrawalQueue", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetWithdrawalQueue is a log parse operation binding the contract event 0x754f348799e85fd8a8e2c9273d677d3e7275ca1e49dd980c9d5f5f0f0134e091.
//
// Solidity: event SetWithdrawalQueue(address indexed withdrawalQueue)
func (_IVaultV2 *IVaultV2Filterer) ParseSetWithdrawalQueue(log types.Log) (*IVaultV2SetWithdrawalQueue, error) {
	event := new(IVaultV2SetWithdrawalQueue)
	if err := _IVaultV2.contract.UnpackLog(event, "SetWithdrawalQueue", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2UpdateProtocolFeeIterator is returned from FilterUpdateProtocolFee and is used to iterate over the raw logs and unpacked data for UpdateProtocolFee events raised by the IVaultV2 contract.
type IVaultV2UpdateProtocolFeeIterator struct {
	Event *IVaultV2UpdateProtocolFee // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IVaultV2UpdateProtocolFeeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2UpdateProtocolFee)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IVaultV2UpdateProtocolFee)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IVaultV2UpdateProtocolFeeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2UpdateProtocolFeeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2UpdateProtocolFee represents a UpdateProtocolFee event raised by the IVaultV2 contract.
type IVaultV2UpdateProtocolFee struct {
	Receiver       common.Address
	ManagementFee  *big.Int
	PerformanceFee *big.Int
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterUpdateProtocolFee is a free log retrieval operation binding the contract event 0xe0317d2dfc8a343ba6f58ba1ea66140c1bbda69c52250f8fd297e4d566c8d672.
//
// Solidity: event UpdateProtocolFee(address indexed receiver, uint96 managementFee, uint96 performanceFee)
func (_IVaultV2 *IVaultV2Filterer) FilterUpdateProtocolFee(opts *bind.FilterOpts, receiver []common.Address) (*IVaultV2UpdateProtocolFeeIterator, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "UpdateProtocolFee", receiverRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2UpdateProtocolFeeIterator{contract: _IVaultV2.contract, event: "UpdateProtocolFee", logs: logs, sub: sub}, nil
}

// WatchUpdateProtocolFee is a free log subscription operation binding the contract event 0xe0317d2dfc8a343ba6f58ba1ea66140c1bbda69c52250f8fd297e4d566c8d672.
//
// Solidity: event UpdateProtocolFee(address indexed receiver, uint96 managementFee, uint96 performanceFee)
func (_IVaultV2 *IVaultV2Filterer) WatchUpdateProtocolFee(opts *bind.WatchOpts, sink chan<- *IVaultV2UpdateProtocolFee, receiver []common.Address) (event.Subscription, error) {

	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "UpdateProtocolFee", receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2UpdateProtocolFee)
				if err := _IVaultV2.contract.UnpackLog(event, "UpdateProtocolFee", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUpdateProtocolFee is a log parse operation binding the contract event 0xe0317d2dfc8a343ba6f58ba1ea66140c1bbda69c52250f8fd297e4d566c8d672.
//
// Solidity: event UpdateProtocolFee(address indexed receiver, uint96 managementFee, uint96 performanceFee)
func (_IVaultV2 *IVaultV2Filterer) ParseUpdateProtocolFee(log types.Log) (*IVaultV2UpdateProtocolFee, error) {
	event := new(IVaultV2UpdateProtocolFee)
	if err := _IVaultV2.contract.UnpackLog(event, "UpdateProtocolFee", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
