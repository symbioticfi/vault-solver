// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package adapter

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

// Offer is an auto generated low-level Go binding around an user-defined struct.
type Offer struct {
	Maker          common.Address
	Amount         *big.Int
	ExpectedReturn *big.Int
	Nonce          *big.Int
	Expiration     *big.Int
	UseCallback    bool
}

// BridgeFacilitatorAdapterMetaData contains all meta data concerning the BridgeFacilitatorAdapter contract.
var BridgeFacilitatorAdapterMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"requestWhitelist\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vaultFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"adapterFactory\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"REQUEST_WHITELIST\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeRequests\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"deallocated\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isValidSignature\",\"inputs\":[{\"name\":\"hash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"maxConcurrentLoans\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minRequestYieldBps\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"offerSigner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"onRequestConsumed\",\"inputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structOffer\",\"components\":[{\"name\":\"maker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedReturn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expiration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"useCallback\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"principal\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yield\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"outstandingPrincipal\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"perRequestMaxCollateral\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"positions\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"principal\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"ytExpected\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"openedAt\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"redeemed\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"realizedPrincipal\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"redeem\",\"inputs\":[{\"name\":\"requests\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestDeallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setExposureLimits\",\"inputs\":[{\"name\":\"perRequestMaxCollateral_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"totalMaxCollateral_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minRequestYieldBps_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxConcurrentLoans_\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setOfferSigner\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalMaxCollateral\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"vault\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PositionOpened\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"principal\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"ytExpected\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PositionRedeemed\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"principal\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"yield\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetExposureLimits\",\"inputs\":[{\"name\":\"perRequestMaxCollateral\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"totalMaxCollateral\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"minRequestYieldBps\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"maxConcurrentLoans\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetOfferSigner\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetVault\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AssetMismatch\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientLiquidity\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotAttested\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"PerRequestCapExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeCastOverflowedUintDowncast\",\"inputs\":[{\"name\":\"bits\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SleeveCapExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooManyConcurrentLoans\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"YieldTooLow\",\"inputs\":[]}]",
}

// BridgeFacilitatorAdapterABI is the input ABI used to generate the binding from.
// Deprecated: Use BridgeFacilitatorAdapterMetaData.ABI instead.
var BridgeFacilitatorAdapterABI = BridgeFacilitatorAdapterMetaData.ABI

// BridgeFacilitatorAdapter is an auto generated Go binding around an Ethereum contract.
type BridgeFacilitatorAdapter struct {
	BridgeFacilitatorAdapterCaller     // Read-only binding to the contract
	BridgeFacilitatorAdapterTransactor // Write-only binding to the contract
	BridgeFacilitatorAdapterFilterer   // Log filterer for contract events
}

// BridgeFacilitatorAdapterCaller is an auto generated read-only Go binding around an Ethereum contract.
type BridgeFacilitatorAdapterCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BridgeFacilitatorAdapterTransactor is an auto generated write-only Go binding around an Ethereum contract.
type BridgeFacilitatorAdapterTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BridgeFacilitatorAdapterFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type BridgeFacilitatorAdapterFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BridgeFacilitatorAdapterSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type BridgeFacilitatorAdapterSession struct {
	Contract     *BridgeFacilitatorAdapter // Generic contract binding to set the session for
	CallOpts     bind.CallOpts             // Call options to use throughout this session
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// BridgeFacilitatorAdapterCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type BridgeFacilitatorAdapterCallerSession struct {
	Contract *BridgeFacilitatorAdapterCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                   // Call options to use throughout this session
}

// BridgeFacilitatorAdapterTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type BridgeFacilitatorAdapterTransactorSession struct {
	Contract     *BridgeFacilitatorAdapterTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                   // Transaction auth options to use throughout this session
}

// BridgeFacilitatorAdapterRaw is an auto generated low-level Go binding around an Ethereum contract.
type BridgeFacilitatorAdapterRaw struct {
	Contract *BridgeFacilitatorAdapter // Generic contract binding to access the raw methods on
}

// BridgeFacilitatorAdapterCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type BridgeFacilitatorAdapterCallerRaw struct {
	Contract *BridgeFacilitatorAdapterCaller // Generic read-only contract binding to access the raw methods on
}

// BridgeFacilitatorAdapterTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type BridgeFacilitatorAdapterTransactorRaw struct {
	Contract *BridgeFacilitatorAdapterTransactor // Generic write-only contract binding to access the raw methods on
}

// NewBridgeFacilitatorAdapter creates a new instance of BridgeFacilitatorAdapter, bound to a specific deployed contract.
func NewBridgeFacilitatorAdapter(address common.Address, backend bind.ContractBackend) (*BridgeFacilitatorAdapter, error) {
	contract, err := bindBridgeFacilitatorAdapter(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapter{BridgeFacilitatorAdapterCaller: BridgeFacilitatorAdapterCaller{contract: contract}, BridgeFacilitatorAdapterTransactor: BridgeFacilitatorAdapterTransactor{contract: contract}, BridgeFacilitatorAdapterFilterer: BridgeFacilitatorAdapterFilterer{contract: contract}}, nil
}

// NewBridgeFacilitatorAdapterCaller creates a new read-only instance of BridgeFacilitatorAdapter, bound to a specific deployed contract.
func NewBridgeFacilitatorAdapterCaller(address common.Address, caller bind.ContractCaller) (*BridgeFacilitatorAdapterCaller, error) {
	contract, err := bindBridgeFacilitatorAdapter(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterCaller{contract: contract}, nil
}

// NewBridgeFacilitatorAdapterTransactor creates a new write-only instance of BridgeFacilitatorAdapter, bound to a specific deployed contract.
func NewBridgeFacilitatorAdapterTransactor(address common.Address, transactor bind.ContractTransactor) (*BridgeFacilitatorAdapterTransactor, error) {
	contract, err := bindBridgeFacilitatorAdapter(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterTransactor{contract: contract}, nil
}

// NewBridgeFacilitatorAdapterFilterer creates a new log filterer instance of BridgeFacilitatorAdapter, bound to a specific deployed contract.
func NewBridgeFacilitatorAdapterFilterer(address common.Address, filterer bind.ContractFilterer) (*BridgeFacilitatorAdapterFilterer, error) {
	contract, err := bindBridgeFacilitatorAdapter(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterFilterer{contract: contract}, nil
}

// bindBridgeFacilitatorAdapter binds a generic wrapper to an already deployed contract.
func bindBridgeFacilitatorAdapter(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := BridgeFacilitatorAdapterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _BridgeFacilitatorAdapter.Contract.BridgeFacilitatorAdapterCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.BridgeFacilitatorAdapterTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.BridgeFacilitatorAdapterTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _BridgeFacilitatorAdapter.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.contract.Transact(opts, method, params...)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) FACTORY(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "FACTORY")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) FACTORY() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.FACTORY(&_BridgeFacilitatorAdapter.CallOpts)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) FACTORY() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.FACTORY(&_BridgeFacilitatorAdapter.CallOpts)
}

// REQUESTWHITELIST is a free data retrieval call binding the contract method 0x894e6d61.
//
// Solidity: function REQUEST_WHITELIST() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) REQUESTWHITELIST(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "REQUEST_WHITELIST")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// REQUESTWHITELIST is a free data retrieval call binding the contract method 0x894e6d61.
//
// Solidity: function REQUEST_WHITELIST() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) REQUESTWHITELIST() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.REQUESTWHITELIST(&_BridgeFacilitatorAdapter.CallOpts)
}

// REQUESTWHITELIST is a free data retrieval call binding the contract method 0x894e6d61.
//
// Solidity: function REQUEST_WHITELIST() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) REQUESTWHITELIST() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.REQUESTWHITELIST(&_BridgeFacilitatorAdapter.CallOpts)
}

// ActiveRequests is a free data retrieval call binding the contract method 0x83cc915c.
//
// Solidity: function activeRequests() view returns(address[])
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) ActiveRequests(opts *bind.CallOpts) ([]common.Address, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "activeRequests")

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// ActiveRequests is a free data retrieval call binding the contract method 0x83cc915c.
//
// Solidity: function activeRequests() view returns(address[])
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) ActiveRequests() ([]common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.ActiveRequests(&_BridgeFacilitatorAdapter.CallOpts)
}

// ActiveRequests is a free data retrieval call binding the contract method 0x83cc915c.
//
// Solidity: function activeRequests() view returns(address[])
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) ActiveRequests() ([]common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.ActiveRequests(&_BridgeFacilitatorAdapter.CallOpts)
}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) Allocatable(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "allocatable")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Allocatable() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.Allocatable(&_BridgeFacilitatorAdapter.CallOpts)
}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) Allocatable() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.Allocatable(&_BridgeFacilitatorAdapter.CallOpts)
}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) FreeAssets(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "freeAssets")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) FreeAssets() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.FreeAssets(&_BridgeFacilitatorAdapter.CallOpts)
}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) FreeAssets() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.FreeAssets(&_BridgeFacilitatorAdapter.CallOpts)
}

// IsValidSignature is a free data retrieval call binding the contract method 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) IsValidSignature(opts *bind.CallOpts, hash [32]byte, signature []byte) ([4]byte, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "isValidSignature", hash, signature)

	if err != nil {
		return *new([4]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([4]byte)).(*[4]byte)

	return out0, err

}

// IsValidSignature is a free data retrieval call binding the contract method 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) IsValidSignature(hash [32]byte, signature []byte) ([4]byte, error) {
	return _BridgeFacilitatorAdapter.Contract.IsValidSignature(&_BridgeFacilitatorAdapter.CallOpts, hash, signature)
}

// IsValidSignature is a free data retrieval call binding the contract method 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) IsValidSignature(hash [32]byte, signature []byte) ([4]byte, error) {
	return _BridgeFacilitatorAdapter.Contract.IsValidSignature(&_BridgeFacilitatorAdapter.CallOpts, hash, signature)
}

// MaxConcurrentLoans is a free data retrieval call binding the contract method 0x0fa715c7.
//
// Solidity: function maxConcurrentLoans() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) MaxConcurrentLoans(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "maxConcurrentLoans")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MaxConcurrentLoans is a free data retrieval call binding the contract method 0x0fa715c7.
//
// Solidity: function maxConcurrentLoans() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) MaxConcurrentLoans() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.MaxConcurrentLoans(&_BridgeFacilitatorAdapter.CallOpts)
}

// MaxConcurrentLoans is a free data retrieval call binding the contract method 0x0fa715c7.
//
// Solidity: function maxConcurrentLoans() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) MaxConcurrentLoans() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.MaxConcurrentLoans(&_BridgeFacilitatorAdapter.CallOpts)
}

// MinRequestYieldBps is a free data retrieval call binding the contract method 0x6762571b.
//
// Solidity: function minRequestYieldBps() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) MinRequestYieldBps(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "minRequestYieldBps")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MinRequestYieldBps is a free data retrieval call binding the contract method 0x6762571b.
//
// Solidity: function minRequestYieldBps() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) MinRequestYieldBps() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.MinRequestYieldBps(&_BridgeFacilitatorAdapter.CallOpts)
}

// MinRequestYieldBps is a free data retrieval call binding the contract method 0x6762571b.
//
// Solidity: function minRequestYieldBps() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) MinRequestYieldBps() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.MinRequestYieldBps(&_BridgeFacilitatorAdapter.CallOpts)
}

// OfferSigner is a free data retrieval call binding the contract method 0x566bd6c3.
//
// Solidity: function offerSigner() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) OfferSigner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "offerSigner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// OfferSigner is a free data retrieval call binding the contract method 0x566bd6c3.
//
// Solidity: function offerSigner() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) OfferSigner() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.OfferSigner(&_BridgeFacilitatorAdapter.CallOpts)
}

// OfferSigner is a free data retrieval call binding the contract method 0x566bd6c3.
//
// Solidity: function offerSigner() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) OfferSigner() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.OfferSigner(&_BridgeFacilitatorAdapter.CallOpts)
}

// OutstandingPrincipal is a free data retrieval call binding the contract method 0x29b1829e.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) OutstandingPrincipal(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "outstandingPrincipal")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// OutstandingPrincipal is a free data retrieval call binding the contract method 0x29b1829e.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) OutstandingPrincipal() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.OutstandingPrincipal(&_BridgeFacilitatorAdapter.CallOpts)
}

// OutstandingPrincipal is a free data retrieval call binding the contract method 0x29b1829e.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) OutstandingPrincipal() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.OutstandingPrincipal(&_BridgeFacilitatorAdapter.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Owner() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.Owner(&_BridgeFacilitatorAdapter.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) Owner() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.Owner(&_BridgeFacilitatorAdapter.CallOpts)
}

// PerRequestMaxCollateral is a free data retrieval call binding the contract method 0xca1f1576.
//
// Solidity: function perRequestMaxCollateral() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) PerRequestMaxCollateral(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "perRequestMaxCollateral")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// PerRequestMaxCollateral is a free data retrieval call binding the contract method 0xca1f1576.
//
// Solidity: function perRequestMaxCollateral() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) PerRequestMaxCollateral() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.PerRequestMaxCollateral(&_BridgeFacilitatorAdapter.CallOpts)
}

// PerRequestMaxCollateral is a free data retrieval call binding the contract method 0xca1f1576.
//
// Solidity: function perRequestMaxCollateral() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) PerRequestMaxCollateral() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.PerRequestMaxCollateral(&_BridgeFacilitatorAdapter.CallOpts)
}

// Positions is a free data retrieval call binding the contract method 0x55f57510.
//
// Solidity: function positions(address request) view returns(uint128 principal, uint128 ytExpected, uint48 openedAt, bool redeemed)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) Positions(opts *bind.CallOpts, request common.Address) (struct {
	Principal  *big.Int
	YtExpected *big.Int
	OpenedAt   *big.Int
	Redeemed   bool
}, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "positions", request)

	outstruct := new(struct {
		Principal  *big.Int
		YtExpected *big.Int
		OpenedAt   *big.Int
		Redeemed   bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Principal = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.YtExpected = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.OpenedAt = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.Redeemed = *abi.ConvertType(out[3], new(bool)).(*bool)

	return *outstruct, err

}

// Positions is a free data retrieval call binding the contract method 0x55f57510.
//
// Solidity: function positions(address request) view returns(uint128 principal, uint128 ytExpected, uint48 openedAt, bool redeemed)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Positions(request common.Address) (struct {
	Principal  *big.Int
	YtExpected *big.Int
	OpenedAt   *big.Int
	Redeemed   bool
}, error) {
	return _BridgeFacilitatorAdapter.Contract.Positions(&_BridgeFacilitatorAdapter.CallOpts, request)
}

// Positions is a free data retrieval call binding the contract method 0x55f57510.
//
// Solidity: function positions(address request) view returns(uint128 principal, uint128 ytExpected, uint48 openedAt, bool redeemed)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) Positions(request common.Address) (struct {
	Principal  *big.Int
	YtExpected *big.Int
	OpenedAt   *big.Int
	Redeemed   bool
}, error) {
	return _BridgeFacilitatorAdapter.Contract.Positions(&_BridgeFacilitatorAdapter.CallOpts, request)
}

// RealizedPrincipal is a free data retrieval call binding the contract method 0x5b348b1f.
//
// Solidity: function realizedPrincipal() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) RealizedPrincipal(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "realizedPrincipal")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RealizedPrincipal is a free data retrieval call binding the contract method 0x5b348b1f.
//
// Solidity: function realizedPrincipal() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) RealizedPrincipal() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.RealizedPrincipal(&_BridgeFacilitatorAdapter.CallOpts)
}

// RealizedPrincipal is a free data retrieval call binding the contract method 0x5b348b1f.
//
// Solidity: function realizedPrincipal() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) RealizedPrincipal() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.RealizedPrincipal(&_BridgeFacilitatorAdapter.CallOpts)
}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) TotalAssets(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "totalAssets")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) TotalAssets() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.TotalAssets(&_BridgeFacilitatorAdapter.CallOpts)
}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) TotalAssets() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.TotalAssets(&_BridgeFacilitatorAdapter.CallOpts)
}

// TotalMaxCollateral is a free data retrieval call binding the contract method 0xe5a81bbc.
//
// Solidity: function totalMaxCollateral() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) TotalMaxCollateral(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "totalMaxCollateral")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalMaxCollateral is a free data retrieval call binding the contract method 0xe5a81bbc.
//
// Solidity: function totalMaxCollateral() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) TotalMaxCollateral() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.TotalMaxCollateral(&_BridgeFacilitatorAdapter.CallOpts)
}

// TotalMaxCollateral is a free data retrieval call binding the contract method 0xe5a81bbc.
//
// Solidity: function totalMaxCollateral() view returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) TotalMaxCollateral() (*big.Int, error) {
	return _BridgeFacilitatorAdapter.Contract.TotalMaxCollateral(&_BridgeFacilitatorAdapter.CallOpts)
}

// Vault is a free data retrieval call binding the contract method 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) Vault(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "vault")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Vault is a free data retrieval call binding the contract method 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Vault() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.Vault(&_BridgeFacilitatorAdapter.CallOpts)
}

// Vault is a free data retrieval call binding the contract method 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) Vault() (common.Address, error) {
	return _BridgeFacilitatorAdapter.Contract.Vault(&_BridgeFacilitatorAdapter.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCaller) Version(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _BridgeFacilitatorAdapter.contract.Call(opts, &out, "version")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Version() (uint64, error) {
	return _BridgeFacilitatorAdapter.Contract.Version(&_BridgeFacilitatorAdapter.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterCallerSession) Version() (uint64, error) {
	return _BridgeFacilitatorAdapter.Contract.Version(&_BridgeFacilitatorAdapter.CallOpts)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) Allocate(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "allocate", amount)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Allocate(amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Allocate(&_BridgeFacilitatorAdapter.TransactOpts, amount)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) Allocate(amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Allocate(&_BridgeFacilitatorAdapter.TransactOpts, amount)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256 deallocated)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) Deallocate(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "deallocate", amount)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256 deallocated)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Deallocate(amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Deallocate(&_BridgeFacilitatorAdapter.TransactOpts, amount)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256 deallocated)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) Deallocate(amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Deallocate(&_BridgeFacilitatorAdapter.TransactOpts, amount)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) Initialize(opts *bind.TransactOpts, initialVersion uint64, owner_ common.Address, data []byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "initialize", initialVersion, owner_, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Initialize(initialVersion uint64, owner_ common.Address, data []byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Initialize(&_BridgeFacilitatorAdapter.TransactOpts, initialVersion, owner_, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) Initialize(initialVersion uint64, owner_ common.Address, data []byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Initialize(&_BridgeFacilitatorAdapter.TransactOpts, initialVersion, owner_, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) Migrate(opts *bind.TransactOpts, newVersion uint64, data []byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "migrate", newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Migrate(&_BridgeFacilitatorAdapter.TransactOpts, newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Migrate(&_BridgeFacilitatorAdapter.TransactOpts, newVersion, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) Multicall(opts *bind.TransactOpts, data [][]byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "multicall", data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Multicall(&_BridgeFacilitatorAdapter.TransactOpts, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Multicall(&_BridgeFacilitatorAdapter.TransactOpts, data)
}

// OnRequestConsumed is a paid mutator transaction binding the contract method 0xf2fe1357.
//
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) , bytes , uint256 principal, uint256 yield) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) OnRequestConsumed(opts *bind.TransactOpts, arg0 Offer, arg1 []byte, principal *big.Int, yield *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "onRequestConsumed", arg0, arg1, principal, yield)
}

// OnRequestConsumed is a paid mutator transaction binding the contract method 0xf2fe1357.
//
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) , bytes , uint256 principal, uint256 yield) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) OnRequestConsumed(arg0 Offer, arg1 []byte, principal *big.Int, yield *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.OnRequestConsumed(&_BridgeFacilitatorAdapter.TransactOpts, arg0, arg1, principal, yield)
}

// OnRequestConsumed is a paid mutator transaction binding the contract method 0xf2fe1357.
//
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) , bytes , uint256 principal, uint256 yield) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) OnRequestConsumed(arg0 Offer, arg1 []byte, principal *big.Int, yield *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.OnRequestConsumed(&_BridgeFacilitatorAdapter.TransactOpts, arg0, arg1, principal, yield)
}

// Redeem is a paid mutator transaction binding the contract method 0x8730b205.
//
// Solidity: function redeem(address[] requests) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) Redeem(opts *bind.TransactOpts, requests []common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "redeem", requests)
}

// Redeem is a paid mutator transaction binding the contract method 0x8730b205.
//
// Solidity: function redeem(address[] requests) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) Redeem(requests []common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Redeem(&_BridgeFacilitatorAdapter.TransactOpts, requests)
}

// Redeem is a paid mutator transaction binding the contract method 0x8730b205.
//
// Solidity: function redeem(address[] requests) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) Redeem(requests []common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.Redeem(&_BridgeFacilitatorAdapter.TransactOpts, requests)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) RenounceOwnership() (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.RenounceOwnership(&_BridgeFacilitatorAdapter.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.RenounceOwnership(&_BridgeFacilitatorAdapter.TransactOpts)
}

// RequestDeallocate is a paid mutator transaction binding the contract method 0xf79f679d.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) RequestDeallocate(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "requestDeallocate", amount)
}

// RequestDeallocate is a paid mutator transaction binding the contract method 0xf79f679d.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) RequestDeallocate(amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.RequestDeallocate(&_BridgeFacilitatorAdapter.TransactOpts, amount)
}

// RequestDeallocate is a paid mutator transaction binding the contract method 0xf79f679d.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) RequestDeallocate(amount *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.RequestDeallocate(&_BridgeFacilitatorAdapter.TransactOpts, amount)
}

// SetExposureLimits is a paid mutator transaction binding the contract method 0xe05d0a0c.
//
// Solidity: function setExposureLimits(uint256 perRequestMaxCollateral_, uint256 totalMaxCollateral_, uint256 minRequestYieldBps_, uint256 maxConcurrentLoans_) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) SetExposureLimits(opts *bind.TransactOpts, perRequestMaxCollateral_ *big.Int, totalMaxCollateral_ *big.Int, minRequestYieldBps_ *big.Int, maxConcurrentLoans_ *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "setExposureLimits", perRequestMaxCollateral_, totalMaxCollateral_, minRequestYieldBps_, maxConcurrentLoans_)
}

// SetExposureLimits is a paid mutator transaction binding the contract method 0xe05d0a0c.
//
// Solidity: function setExposureLimits(uint256 perRequestMaxCollateral_, uint256 totalMaxCollateral_, uint256 minRequestYieldBps_, uint256 maxConcurrentLoans_) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) SetExposureLimits(perRequestMaxCollateral_ *big.Int, totalMaxCollateral_ *big.Int, minRequestYieldBps_ *big.Int, maxConcurrentLoans_ *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.SetExposureLimits(&_BridgeFacilitatorAdapter.TransactOpts, perRequestMaxCollateral_, totalMaxCollateral_, minRequestYieldBps_, maxConcurrentLoans_)
}

// SetExposureLimits is a paid mutator transaction binding the contract method 0xe05d0a0c.
//
// Solidity: function setExposureLimits(uint256 perRequestMaxCollateral_, uint256 totalMaxCollateral_, uint256 minRequestYieldBps_, uint256 maxConcurrentLoans_) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) SetExposureLimits(perRequestMaxCollateral_ *big.Int, totalMaxCollateral_ *big.Int, minRequestYieldBps_ *big.Int, maxConcurrentLoans_ *big.Int) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.SetExposureLimits(&_BridgeFacilitatorAdapter.TransactOpts, perRequestMaxCollateral_, totalMaxCollateral_, minRequestYieldBps_, maxConcurrentLoans_)
}

// SetOfferSigner is a paid mutator transaction binding the contract method 0x868adcae.
//
// Solidity: function setOfferSigner(address signer) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) SetOfferSigner(opts *bind.TransactOpts, signer common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "setOfferSigner", signer)
}

// SetOfferSigner is a paid mutator transaction binding the contract method 0x868adcae.
//
// Solidity: function setOfferSigner(address signer) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) SetOfferSigner(signer common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.SetOfferSigner(&_BridgeFacilitatorAdapter.TransactOpts, signer)
}

// SetOfferSigner is a paid mutator transaction binding the contract method 0x868adcae.
//
// Solidity: function setOfferSigner(address signer) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) SetOfferSigner(signer common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.SetOfferSigner(&_BridgeFacilitatorAdapter.TransactOpts, signer)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.TransferOwnership(&_BridgeFacilitatorAdapter.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _BridgeFacilitatorAdapter.Contract.TransferOwnership(&_BridgeFacilitatorAdapter.TransactOpts, newOwner)
}

// BridgeFacilitatorAdapterInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterInitializedIterator struct {
	Event *BridgeFacilitatorAdapterInitialized // Event containing the contract specifics and raw log

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
func (it *BridgeFacilitatorAdapterInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BridgeFacilitatorAdapterInitialized)
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
		it.Event = new(BridgeFacilitatorAdapterInitialized)
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
func (it *BridgeFacilitatorAdapterInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BridgeFacilitatorAdapterInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BridgeFacilitatorAdapterInitialized represents a Initialized event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterInitialized struct {
	Version uint64
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) FilterInitialized(opts *bind.FilterOpts) (*BridgeFacilitatorAdapterInitializedIterator, error) {

	logs, sub, err := _BridgeFacilitatorAdapter.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterInitializedIterator{contract: _BridgeFacilitatorAdapter.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *BridgeFacilitatorAdapterInitialized) (event.Subscription, error) {

	logs, sub, err := _BridgeFacilitatorAdapter.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BridgeFacilitatorAdapterInitialized)
				if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "Initialized", log); err != nil {
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

// ParseInitialized is a log parse operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) ParseInitialized(log types.Log) (*BridgeFacilitatorAdapterInitialized, error) {
	event := new(BridgeFacilitatorAdapterInitialized)
	if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BridgeFacilitatorAdapterOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterOwnershipTransferredIterator struct {
	Event *BridgeFacilitatorAdapterOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *BridgeFacilitatorAdapterOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BridgeFacilitatorAdapterOwnershipTransferred)
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
		it.Event = new(BridgeFacilitatorAdapterOwnershipTransferred)
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
func (it *BridgeFacilitatorAdapterOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BridgeFacilitatorAdapterOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BridgeFacilitatorAdapterOwnershipTransferred represents a OwnershipTransferred event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*BridgeFacilitatorAdapterOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterOwnershipTransferredIterator{contract: _BridgeFacilitatorAdapter.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *BridgeFacilitatorAdapterOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BridgeFacilitatorAdapterOwnershipTransferred)
				if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) ParseOwnershipTransferred(log types.Log) (*BridgeFacilitatorAdapterOwnershipTransferred, error) {
	event := new(BridgeFacilitatorAdapterOwnershipTransferred)
	if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BridgeFacilitatorAdapterPositionOpenedIterator is returned from FilterPositionOpened and is used to iterate over the raw logs and unpacked data for PositionOpened events raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterPositionOpenedIterator struct {
	Event *BridgeFacilitatorAdapterPositionOpened // Event containing the contract specifics and raw log

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
func (it *BridgeFacilitatorAdapterPositionOpenedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BridgeFacilitatorAdapterPositionOpened)
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
		it.Event = new(BridgeFacilitatorAdapterPositionOpened)
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
func (it *BridgeFacilitatorAdapterPositionOpenedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BridgeFacilitatorAdapterPositionOpenedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BridgeFacilitatorAdapterPositionOpened represents a PositionOpened event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterPositionOpened struct {
	Request    common.Address
	Principal  *big.Int
	YtExpected *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterPositionOpened is a free log retrieval operation binding the contract event 0xc491b8e6940cb48fcf9cf813c35e2d45fad221af7d5f4ce18d77dc576e5bf220.
//
// Solidity: event PositionOpened(address indexed request, uint256 principal, uint256 ytExpected)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) FilterPositionOpened(opts *bind.FilterOpts, request []common.Address) (*BridgeFacilitatorAdapterPositionOpenedIterator, error) {

	var requestRule []interface{}
	for _, requestItem := range request {
		requestRule = append(requestRule, requestItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.FilterLogs(opts, "PositionOpened", requestRule)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterPositionOpenedIterator{contract: _BridgeFacilitatorAdapter.contract, event: "PositionOpened", logs: logs, sub: sub}, nil
}

// WatchPositionOpened is a free log subscription operation binding the contract event 0xc491b8e6940cb48fcf9cf813c35e2d45fad221af7d5f4ce18d77dc576e5bf220.
//
// Solidity: event PositionOpened(address indexed request, uint256 principal, uint256 ytExpected)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) WatchPositionOpened(opts *bind.WatchOpts, sink chan<- *BridgeFacilitatorAdapterPositionOpened, request []common.Address) (event.Subscription, error) {

	var requestRule []interface{}
	for _, requestItem := range request {
		requestRule = append(requestRule, requestItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.WatchLogs(opts, "PositionOpened", requestRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BridgeFacilitatorAdapterPositionOpened)
				if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "PositionOpened", log); err != nil {
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

// ParsePositionOpened is a log parse operation binding the contract event 0xc491b8e6940cb48fcf9cf813c35e2d45fad221af7d5f4ce18d77dc576e5bf220.
//
// Solidity: event PositionOpened(address indexed request, uint256 principal, uint256 ytExpected)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) ParsePositionOpened(log types.Log) (*BridgeFacilitatorAdapterPositionOpened, error) {
	event := new(BridgeFacilitatorAdapterPositionOpened)
	if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "PositionOpened", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BridgeFacilitatorAdapterPositionRedeemedIterator is returned from FilterPositionRedeemed and is used to iterate over the raw logs and unpacked data for PositionRedeemed events raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterPositionRedeemedIterator struct {
	Event *BridgeFacilitatorAdapterPositionRedeemed // Event containing the contract specifics and raw log

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
func (it *BridgeFacilitatorAdapterPositionRedeemedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BridgeFacilitatorAdapterPositionRedeemed)
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
		it.Event = new(BridgeFacilitatorAdapterPositionRedeemed)
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
func (it *BridgeFacilitatorAdapterPositionRedeemedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BridgeFacilitatorAdapterPositionRedeemedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BridgeFacilitatorAdapterPositionRedeemed represents a PositionRedeemed event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterPositionRedeemed struct {
	Request   common.Address
	Principal *big.Int
	Yield     *big.Int
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterPositionRedeemed is a free log retrieval operation binding the contract event 0xcb189b8f48370aa830ed2ee24fea5ae0eaf681c011556ccdc4939fc3c5119906.
//
// Solidity: event PositionRedeemed(address indexed request, uint256 principal, uint256 yield)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) FilterPositionRedeemed(opts *bind.FilterOpts, request []common.Address) (*BridgeFacilitatorAdapterPositionRedeemedIterator, error) {

	var requestRule []interface{}
	for _, requestItem := range request {
		requestRule = append(requestRule, requestItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.FilterLogs(opts, "PositionRedeemed", requestRule)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterPositionRedeemedIterator{contract: _BridgeFacilitatorAdapter.contract, event: "PositionRedeemed", logs: logs, sub: sub}, nil
}

// WatchPositionRedeemed is a free log subscription operation binding the contract event 0xcb189b8f48370aa830ed2ee24fea5ae0eaf681c011556ccdc4939fc3c5119906.
//
// Solidity: event PositionRedeemed(address indexed request, uint256 principal, uint256 yield)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) WatchPositionRedeemed(opts *bind.WatchOpts, sink chan<- *BridgeFacilitatorAdapterPositionRedeemed, request []common.Address) (event.Subscription, error) {

	var requestRule []interface{}
	for _, requestItem := range request {
		requestRule = append(requestRule, requestItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.WatchLogs(opts, "PositionRedeemed", requestRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BridgeFacilitatorAdapterPositionRedeemed)
				if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "PositionRedeemed", log); err != nil {
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

// ParsePositionRedeemed is a log parse operation binding the contract event 0xcb189b8f48370aa830ed2ee24fea5ae0eaf681c011556ccdc4939fc3c5119906.
//
// Solidity: event PositionRedeemed(address indexed request, uint256 principal, uint256 yield)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) ParsePositionRedeemed(log types.Log) (*BridgeFacilitatorAdapterPositionRedeemed, error) {
	event := new(BridgeFacilitatorAdapterPositionRedeemed)
	if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "PositionRedeemed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BridgeFacilitatorAdapterSetExposureLimitsIterator is returned from FilterSetExposureLimits and is used to iterate over the raw logs and unpacked data for SetExposureLimits events raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetExposureLimitsIterator struct {
	Event *BridgeFacilitatorAdapterSetExposureLimits // Event containing the contract specifics and raw log

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
func (it *BridgeFacilitatorAdapterSetExposureLimitsIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BridgeFacilitatorAdapterSetExposureLimits)
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
		it.Event = new(BridgeFacilitatorAdapterSetExposureLimits)
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
func (it *BridgeFacilitatorAdapterSetExposureLimitsIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BridgeFacilitatorAdapterSetExposureLimitsIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BridgeFacilitatorAdapterSetExposureLimits represents a SetExposureLimits event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetExposureLimits struct {
	PerRequestMaxCollateral *big.Int
	TotalMaxCollateral      *big.Int
	MinRequestYieldBps      *big.Int
	MaxConcurrentLoans      *big.Int
	Raw                     types.Log // Blockchain specific contextual infos
}

// FilterSetExposureLimits is a free log retrieval operation binding the contract event 0x32619ec3660c3c554b2e041bb295e0f9b06d8213852d4f704dfa2bb6291272e6.
//
// Solidity: event SetExposureLimits(uint256 perRequestMaxCollateral, uint256 totalMaxCollateral, uint256 minRequestYieldBps, uint256 maxConcurrentLoans)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) FilterSetExposureLimits(opts *bind.FilterOpts) (*BridgeFacilitatorAdapterSetExposureLimitsIterator, error) {

	logs, sub, err := _BridgeFacilitatorAdapter.contract.FilterLogs(opts, "SetExposureLimits")
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterSetExposureLimitsIterator{contract: _BridgeFacilitatorAdapter.contract, event: "SetExposureLimits", logs: logs, sub: sub}, nil
}

// WatchSetExposureLimits is a free log subscription operation binding the contract event 0x32619ec3660c3c554b2e041bb295e0f9b06d8213852d4f704dfa2bb6291272e6.
//
// Solidity: event SetExposureLimits(uint256 perRequestMaxCollateral, uint256 totalMaxCollateral, uint256 minRequestYieldBps, uint256 maxConcurrentLoans)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) WatchSetExposureLimits(opts *bind.WatchOpts, sink chan<- *BridgeFacilitatorAdapterSetExposureLimits) (event.Subscription, error) {

	logs, sub, err := _BridgeFacilitatorAdapter.contract.WatchLogs(opts, "SetExposureLimits")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BridgeFacilitatorAdapterSetExposureLimits)
				if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "SetExposureLimits", log); err != nil {
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

// ParseSetExposureLimits is a log parse operation binding the contract event 0x32619ec3660c3c554b2e041bb295e0f9b06d8213852d4f704dfa2bb6291272e6.
//
// Solidity: event SetExposureLimits(uint256 perRequestMaxCollateral, uint256 totalMaxCollateral, uint256 minRequestYieldBps, uint256 maxConcurrentLoans)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) ParseSetExposureLimits(log types.Log) (*BridgeFacilitatorAdapterSetExposureLimits, error) {
	event := new(BridgeFacilitatorAdapterSetExposureLimits)
	if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "SetExposureLimits", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BridgeFacilitatorAdapterSetOfferSignerIterator is returned from FilterSetOfferSigner and is used to iterate over the raw logs and unpacked data for SetOfferSigner events raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetOfferSignerIterator struct {
	Event *BridgeFacilitatorAdapterSetOfferSigner // Event containing the contract specifics and raw log

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
func (it *BridgeFacilitatorAdapterSetOfferSignerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BridgeFacilitatorAdapterSetOfferSigner)
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
		it.Event = new(BridgeFacilitatorAdapterSetOfferSigner)
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
func (it *BridgeFacilitatorAdapterSetOfferSignerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BridgeFacilitatorAdapterSetOfferSignerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BridgeFacilitatorAdapterSetOfferSigner represents a SetOfferSigner event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetOfferSigner struct {
	Signer common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSetOfferSigner is a free log retrieval operation binding the contract event 0xa0988fc1a8aee9e42bf83cf2098a1ab27094bc6b063ec73c4a267d64eb017542.
//
// Solidity: event SetOfferSigner(address indexed signer)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) FilterSetOfferSigner(opts *bind.FilterOpts, signer []common.Address) (*BridgeFacilitatorAdapterSetOfferSignerIterator, error) {

	var signerRule []interface{}
	for _, signerItem := range signer {
		signerRule = append(signerRule, signerItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.FilterLogs(opts, "SetOfferSigner", signerRule)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterSetOfferSignerIterator{contract: _BridgeFacilitatorAdapter.contract, event: "SetOfferSigner", logs: logs, sub: sub}, nil
}

// WatchSetOfferSigner is a free log subscription operation binding the contract event 0xa0988fc1a8aee9e42bf83cf2098a1ab27094bc6b063ec73c4a267d64eb017542.
//
// Solidity: event SetOfferSigner(address indexed signer)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) WatchSetOfferSigner(opts *bind.WatchOpts, sink chan<- *BridgeFacilitatorAdapterSetOfferSigner, signer []common.Address) (event.Subscription, error) {

	var signerRule []interface{}
	for _, signerItem := range signer {
		signerRule = append(signerRule, signerItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.WatchLogs(opts, "SetOfferSigner", signerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BridgeFacilitatorAdapterSetOfferSigner)
				if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "SetOfferSigner", log); err != nil {
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

// ParseSetOfferSigner is a log parse operation binding the contract event 0xa0988fc1a8aee9e42bf83cf2098a1ab27094bc6b063ec73c4a267d64eb017542.
//
// Solidity: event SetOfferSigner(address indexed signer)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) ParseSetOfferSigner(log types.Log) (*BridgeFacilitatorAdapterSetOfferSigner, error) {
	event := new(BridgeFacilitatorAdapterSetOfferSigner)
	if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "SetOfferSigner", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BridgeFacilitatorAdapterSetVaultIterator is returned from FilterSetVault and is used to iterate over the raw logs and unpacked data for SetVault events raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetVaultIterator struct {
	Event *BridgeFacilitatorAdapterSetVault // Event containing the contract specifics and raw log

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
func (it *BridgeFacilitatorAdapterSetVaultIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BridgeFacilitatorAdapterSetVault)
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
		it.Event = new(BridgeFacilitatorAdapterSetVault)
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
func (it *BridgeFacilitatorAdapterSetVaultIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BridgeFacilitatorAdapterSetVaultIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BridgeFacilitatorAdapterSetVault represents a SetVault event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetVault struct {
	Vault common.Address
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterSetVault is a free log retrieval operation binding the contract event 0xd459c7242e23d490831b5676a611c4342d899d28f342d89ae80793e56a930f30.
//
// Solidity: event SetVault(address indexed vault)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) FilterSetVault(opts *bind.FilterOpts, vault []common.Address) (*BridgeFacilitatorAdapterSetVaultIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.FilterLogs(opts, "SetVault", vaultRule)
	if err != nil {
		return nil, err
	}
	return &BridgeFacilitatorAdapterSetVaultIterator{contract: _BridgeFacilitatorAdapter.contract, event: "SetVault", logs: logs, sub: sub}, nil
}

// WatchSetVault is a free log subscription operation binding the contract event 0xd459c7242e23d490831b5676a611c4342d899d28f342d89ae80793e56a930f30.
//
// Solidity: event SetVault(address indexed vault)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) WatchSetVault(opts *bind.WatchOpts, sink chan<- *BridgeFacilitatorAdapterSetVault, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _BridgeFacilitatorAdapter.contract.WatchLogs(opts, "SetVault", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BridgeFacilitatorAdapterSetVault)
				if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "SetVault", log); err != nil {
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

// ParseSetVault is a log parse operation binding the contract event 0xd459c7242e23d490831b5676a611c4342d899d28f342d89ae80793e56a930f30.
//
// Solidity: event SetVault(address indexed vault)
func (_BridgeFacilitatorAdapter *BridgeFacilitatorAdapterFilterer) ParseSetVault(log types.Log) (*BridgeFacilitatorAdapterSetVault, error) {
	event := new(BridgeFacilitatorAdapterSetVault)
	if err := _BridgeFacilitatorAdapter.contract.UnpackLog(event, "SetVault", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
