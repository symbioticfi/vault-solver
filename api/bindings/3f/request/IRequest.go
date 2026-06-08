// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package request

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

// IRequestMetaData contains all meta data concerning the IRequest contract.
var IRequestMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"asset\",\"inputs\":[],\"outputs\":[{\"name\":\"assetAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"authorizeMinting\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"ytAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"consume\",\"inputs\":[{\"name\":\"offer\",\"type\":\"tuple\",\"internalType\":\"structOffer\",\"components\":[{\"name\":\"maker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedReturn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expiration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"useCallback\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isRepaid\",\"inputs\":[],\"outputs\":[{\"name\":\"repaid\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastMintTimestamp\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"mint\",\"inputs\":[{\"name\":\"maxPt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"minYt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"mintAuthorization\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ptAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"ytAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"mintToRepaidDelay\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pullFunds\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"repaidAvailableAt\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"repay\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMintToRepaidDelay\",\"inputs\":[{\"name\":\"mintToRepaidDelay_\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setRepaid\",\"inputs\":[{\"name\":\"minBalance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxBalance\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"syncRepaidStatus\",\"inputs\":[],\"outputs\":[{\"name\":\"repaid\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AuthorizedMinting\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"FundsPulled\",\"inputs\":[{\"name\":\"puller\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"MintToRepaidDelaySet\",\"inputs\":[{\"name\":\"mintToRepaidDelay\",\"type\":\"uint40\",\"indexed\":false,\"internalType\":\"uint40\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Repaid\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false}]",
}

// IRequestABI is the input ABI used to generate the binding from.
// Deprecated: Use IRequestMetaData.ABI instead.
var IRequestABI = IRequestMetaData.ABI

// IRequest is an auto generated Go binding around an Ethereum contract.
type IRequest struct {
	IRequestCaller     // Read-only binding to the contract
	IRequestTransactor // Write-only binding to the contract
	IRequestFilterer   // Log filterer for contract events
}

// IRequestCaller is an auto generated read-only Go binding around an Ethereum contract.
type IRequestCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IRequestTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IRequestTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IRequestFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IRequestFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IRequestSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IRequestSession struct {
	Contract     *IRequest         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IRequestCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IRequestCallerSession struct {
	Contract *IRequestCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// IRequestTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IRequestTransactorSession struct {
	Contract     *IRequestTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// IRequestRaw is an auto generated low-level Go binding around an Ethereum contract.
type IRequestRaw struct {
	Contract *IRequest // Generic contract binding to access the raw methods on
}

// IRequestCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IRequestCallerRaw struct {
	Contract *IRequestCaller // Generic read-only contract binding to access the raw methods on
}

// IRequestTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IRequestTransactorRaw struct {
	Contract *IRequestTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIRequest creates a new instance of IRequest, bound to a specific deployed contract.
func NewIRequest(address common.Address, backend bind.ContractBackend) (*IRequest, error) {
	contract, err := bindIRequest(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IRequest{IRequestCaller: IRequestCaller{contract: contract}, IRequestTransactor: IRequestTransactor{contract: contract}, IRequestFilterer: IRequestFilterer{contract: contract}}, nil
}

// NewIRequestCaller creates a new read-only instance of IRequest, bound to a specific deployed contract.
func NewIRequestCaller(address common.Address, caller bind.ContractCaller) (*IRequestCaller, error) {
	contract, err := bindIRequest(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IRequestCaller{contract: contract}, nil
}

// NewIRequestTransactor creates a new write-only instance of IRequest, bound to a specific deployed contract.
func NewIRequestTransactor(address common.Address, transactor bind.ContractTransactor) (*IRequestTransactor, error) {
	contract, err := bindIRequest(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IRequestTransactor{contract: contract}, nil
}

// NewIRequestFilterer creates a new log filterer instance of IRequest, bound to a specific deployed contract.
func NewIRequestFilterer(address common.Address, filterer bind.ContractFilterer) (*IRequestFilterer, error) {
	contract, err := bindIRequest(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IRequestFilterer{contract: contract}, nil
}

// bindIRequest binds a generic wrapper to an already deployed contract.
func bindIRequest(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IRequestMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IRequest *IRequestRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IRequest.Contract.IRequestCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IRequest *IRequestRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IRequest.Contract.IRequestTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IRequest *IRequestRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IRequest.Contract.IRequestTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IRequest *IRequestCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IRequest.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IRequest *IRequestTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IRequest.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IRequest *IRequestTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IRequest.Contract.contract.Transact(opts, method, params...)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (_IRequest *IRequestCaller) Asset(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IRequest.contract.Call(opts, &out, "asset")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (_IRequest *IRequestSession) Asset() (common.Address, error) {
	return _IRequest.Contract.Asset(&_IRequest.CallOpts)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (_IRequest *IRequestCallerSession) Asset() (common.Address, error) {
	return _IRequest.Contract.Asset(&_IRequest.CallOpts)
}

// IsRepaid is a free data retrieval call binding the contract method 0x6164051a.
//
// Solidity: function isRepaid() view returns(bool repaid)
func (_IRequest *IRequestCaller) IsRepaid(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IRequest.contract.Call(opts, &out, "isRepaid")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsRepaid is a free data retrieval call binding the contract method 0x6164051a.
//
// Solidity: function isRepaid() view returns(bool repaid)
func (_IRequest *IRequestSession) IsRepaid() (bool, error) {
	return _IRequest.Contract.IsRepaid(&_IRequest.CallOpts)
}

// IsRepaid is a free data retrieval call binding the contract method 0x6164051a.
//
// Solidity: function isRepaid() view returns(bool repaid)
func (_IRequest *IRequestCallerSession) IsRepaid() (bool, error) {
	return _IRequest.Contract.IsRepaid(&_IRequest.CallOpts)
}

// LastMintTimestamp is a free data retrieval call binding the contract method 0x8e80ff5d.
//
// Solidity: function lastMintTimestamp() view returns(uint40)
func (_IRequest *IRequestCaller) LastMintTimestamp(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IRequest.contract.Call(opts, &out, "lastMintTimestamp")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// LastMintTimestamp is a free data retrieval call binding the contract method 0x8e80ff5d.
//
// Solidity: function lastMintTimestamp() view returns(uint40)
func (_IRequest *IRequestSession) LastMintTimestamp() (*big.Int, error) {
	return _IRequest.Contract.LastMintTimestamp(&_IRequest.CallOpts)
}

// LastMintTimestamp is a free data retrieval call binding the contract method 0x8e80ff5d.
//
// Solidity: function lastMintTimestamp() view returns(uint40)
func (_IRequest *IRequestCallerSession) LastMintTimestamp() (*big.Int, error) {
	return _IRequest.Contract.LastMintTimestamp(&_IRequest.CallOpts)
}

// MintAuthorization is a free data retrieval call binding the contract method 0xdc6c1d71.
//
// Solidity: function mintAuthorization(address account) view returns(uint128 ptAmount, uint128 ytAmount)
func (_IRequest *IRequestCaller) MintAuthorization(opts *bind.CallOpts, account common.Address) (struct {
	PtAmount *big.Int
	YtAmount *big.Int
}, error) {
	var out []interface{}
	err := _IRequest.contract.Call(opts, &out, "mintAuthorization", account)

	outstruct := new(struct {
		PtAmount *big.Int
		YtAmount *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.PtAmount = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.YtAmount = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// MintAuthorization is a free data retrieval call binding the contract method 0xdc6c1d71.
//
// Solidity: function mintAuthorization(address account) view returns(uint128 ptAmount, uint128 ytAmount)
func (_IRequest *IRequestSession) MintAuthorization(account common.Address) (struct {
	PtAmount *big.Int
	YtAmount *big.Int
}, error) {
	return _IRequest.Contract.MintAuthorization(&_IRequest.CallOpts, account)
}

// MintAuthorization is a free data retrieval call binding the contract method 0xdc6c1d71.
//
// Solidity: function mintAuthorization(address account) view returns(uint128 ptAmount, uint128 ytAmount)
func (_IRequest *IRequestCallerSession) MintAuthorization(account common.Address) (struct {
	PtAmount *big.Int
	YtAmount *big.Int
}, error) {
	return _IRequest.Contract.MintAuthorization(&_IRequest.CallOpts, account)
}

// MintToRepaidDelay is a free data retrieval call binding the contract method 0x80aed3e4.
//
// Solidity: function mintToRepaidDelay() view returns(uint40)
func (_IRequest *IRequestCaller) MintToRepaidDelay(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IRequest.contract.Call(opts, &out, "mintToRepaidDelay")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MintToRepaidDelay is a free data retrieval call binding the contract method 0x80aed3e4.
//
// Solidity: function mintToRepaidDelay() view returns(uint40)
func (_IRequest *IRequestSession) MintToRepaidDelay() (*big.Int, error) {
	return _IRequest.Contract.MintToRepaidDelay(&_IRequest.CallOpts)
}

// MintToRepaidDelay is a free data retrieval call binding the contract method 0x80aed3e4.
//
// Solidity: function mintToRepaidDelay() view returns(uint40)
func (_IRequest *IRequestCallerSession) MintToRepaidDelay() (*big.Int, error) {
	return _IRequest.Contract.MintToRepaidDelay(&_IRequest.CallOpts)
}

// RepaidAvailableAt is a free data retrieval call binding the contract method 0xfe4b6faa.
//
// Solidity: function repaidAvailableAt() view returns(uint40)
func (_IRequest *IRequestCaller) RepaidAvailableAt(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IRequest.contract.Call(opts, &out, "repaidAvailableAt")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RepaidAvailableAt is a free data retrieval call binding the contract method 0xfe4b6faa.
//
// Solidity: function repaidAvailableAt() view returns(uint40)
func (_IRequest *IRequestSession) RepaidAvailableAt() (*big.Int, error) {
	return _IRequest.Contract.RepaidAvailableAt(&_IRequest.CallOpts)
}

// RepaidAvailableAt is a free data retrieval call binding the contract method 0xfe4b6faa.
//
// Solidity: function repaidAvailableAt() view returns(uint40)
func (_IRequest *IRequestCallerSession) RepaidAvailableAt() (*big.Int, error) {
	return _IRequest.Contract.RepaidAvailableAt(&_IRequest.CallOpts)
}

// AuthorizeMinting is a paid mutator transaction binding the contract method 0xb1f88261.
//
// Solidity: function authorizeMinting(address to, uint128 ptAmount, uint128 ytAmount) returns()
func (_IRequest *IRequestTransactor) AuthorizeMinting(opts *bind.TransactOpts, to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "authorizeMinting", to, ptAmount, ytAmount)
}

// AuthorizeMinting is a paid mutator transaction binding the contract method 0xb1f88261.
//
// Solidity: function authorizeMinting(address to, uint128 ptAmount, uint128 ytAmount) returns()
func (_IRequest *IRequestSession) AuthorizeMinting(to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.AuthorizeMinting(&_IRequest.TransactOpts, to, ptAmount, ytAmount)
}

// AuthorizeMinting is a paid mutator transaction binding the contract method 0xb1f88261.
//
// Solidity: function authorizeMinting(address to, uint128 ptAmount, uint128 ytAmount) returns()
func (_IRequest *IRequestTransactorSession) AuthorizeMinting(to common.Address, ptAmount *big.Int, ytAmount *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.AuthorizeMinting(&_IRequest.TransactOpts, to, ptAmount, ytAmount)
}

// Consume is a paid mutator transaction binding the contract method 0x13bcaf67.
//
// Solidity: function consume((address,uint256,uint256,uint256,uint256,bool) offer, bytes signature, uint256 ptAmount) returns(uint256 ytAmount)
func (_IRequest *IRequestTransactor) Consume(opts *bind.TransactOpts, offer Offer, signature []byte, ptAmount *big.Int) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "consume", offer, signature, ptAmount)
}

// Consume is a paid mutator transaction binding the contract method 0x13bcaf67.
//
// Solidity: function consume((address,uint256,uint256,uint256,uint256,bool) offer, bytes signature, uint256 ptAmount) returns(uint256 ytAmount)
func (_IRequest *IRequestSession) Consume(offer Offer, signature []byte, ptAmount *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.Consume(&_IRequest.TransactOpts, offer, signature, ptAmount)
}

// Consume is a paid mutator transaction binding the contract method 0x13bcaf67.
//
// Solidity: function consume((address,uint256,uint256,uint256,uint256,bool) offer, bytes signature, uint256 ptAmount) returns(uint256 ytAmount)
func (_IRequest *IRequestTransactorSession) Consume(offer Offer, signature []byte, ptAmount *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.Consume(&_IRequest.TransactOpts, offer, signature, ptAmount)
}

// Mint is a paid mutator transaction binding the contract method 0xdfe7a8e5.
//
// Solidity: function mint(uint128 maxPt, uint128 minYt) returns()
func (_IRequest *IRequestTransactor) Mint(opts *bind.TransactOpts, maxPt *big.Int, minYt *big.Int) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "mint", maxPt, minYt)
}

// Mint is a paid mutator transaction binding the contract method 0xdfe7a8e5.
//
// Solidity: function mint(uint128 maxPt, uint128 minYt) returns()
func (_IRequest *IRequestSession) Mint(maxPt *big.Int, minYt *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.Mint(&_IRequest.TransactOpts, maxPt, minYt)
}

// Mint is a paid mutator transaction binding the contract method 0xdfe7a8e5.
//
// Solidity: function mint(uint128 maxPt, uint128 minYt) returns()
func (_IRequest *IRequestTransactorSession) Mint(maxPt *big.Int, minYt *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.Mint(&_IRequest.TransactOpts, maxPt, minYt)
}

// PullFunds is a paid mutator transaction binding the contract method 0x5cb5727a.
//
// Solidity: function pullFunds(uint256 amount, bytes data) returns()
func (_IRequest *IRequestTransactor) PullFunds(opts *bind.TransactOpts, amount *big.Int, data []byte) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "pullFunds", amount, data)
}

// PullFunds is a paid mutator transaction binding the contract method 0x5cb5727a.
//
// Solidity: function pullFunds(uint256 amount, bytes data) returns()
func (_IRequest *IRequestSession) PullFunds(amount *big.Int, data []byte) (*types.Transaction, error) {
	return _IRequest.Contract.PullFunds(&_IRequest.TransactOpts, amount, data)
}

// PullFunds is a paid mutator transaction binding the contract method 0x5cb5727a.
//
// Solidity: function pullFunds(uint256 amount, bytes data) returns()
func (_IRequest *IRequestTransactorSession) PullFunds(amount *big.Int, data []byte) (*types.Transaction, error) {
	return _IRequest.Contract.PullFunds(&_IRequest.TransactOpts, amount, data)
}

// Repay is a paid mutator transaction binding the contract method 0x371fd8e6.
//
// Solidity: function repay(uint256 amount) returns()
func (_IRequest *IRequestTransactor) Repay(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "repay", amount)
}

// Repay is a paid mutator transaction binding the contract method 0x371fd8e6.
//
// Solidity: function repay(uint256 amount) returns()
func (_IRequest *IRequestSession) Repay(amount *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.Repay(&_IRequest.TransactOpts, amount)
}

// Repay is a paid mutator transaction binding the contract method 0x371fd8e6.
//
// Solidity: function repay(uint256 amount) returns()
func (_IRequest *IRequestTransactorSession) Repay(amount *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.Repay(&_IRequest.TransactOpts, amount)
}

// SetMintToRepaidDelay is a paid mutator transaction binding the contract method 0x4e3f7fdb.
//
// Solidity: function setMintToRepaidDelay(uint40 mintToRepaidDelay_) returns()
func (_IRequest *IRequestTransactor) SetMintToRepaidDelay(opts *bind.TransactOpts, mintToRepaidDelay_ *big.Int) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "setMintToRepaidDelay", mintToRepaidDelay_)
}

// SetMintToRepaidDelay is a paid mutator transaction binding the contract method 0x4e3f7fdb.
//
// Solidity: function setMintToRepaidDelay(uint40 mintToRepaidDelay_) returns()
func (_IRequest *IRequestSession) SetMintToRepaidDelay(mintToRepaidDelay_ *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.SetMintToRepaidDelay(&_IRequest.TransactOpts, mintToRepaidDelay_)
}

// SetMintToRepaidDelay is a paid mutator transaction binding the contract method 0x4e3f7fdb.
//
// Solidity: function setMintToRepaidDelay(uint40 mintToRepaidDelay_) returns()
func (_IRequest *IRequestTransactorSession) SetMintToRepaidDelay(mintToRepaidDelay_ *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.SetMintToRepaidDelay(&_IRequest.TransactOpts, mintToRepaidDelay_)
}

// SetRepaid is a paid mutator transaction binding the contract method 0x512acc56.
//
// Solidity: function setRepaid(uint256 minBalance, uint256 maxBalance) returns()
func (_IRequest *IRequestTransactor) SetRepaid(opts *bind.TransactOpts, minBalance *big.Int, maxBalance *big.Int) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "setRepaid", minBalance, maxBalance)
}

// SetRepaid is a paid mutator transaction binding the contract method 0x512acc56.
//
// Solidity: function setRepaid(uint256 minBalance, uint256 maxBalance) returns()
func (_IRequest *IRequestSession) SetRepaid(minBalance *big.Int, maxBalance *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.SetRepaid(&_IRequest.TransactOpts, minBalance, maxBalance)
}

// SetRepaid is a paid mutator transaction binding the contract method 0x512acc56.
//
// Solidity: function setRepaid(uint256 minBalance, uint256 maxBalance) returns()
func (_IRequest *IRequestTransactorSession) SetRepaid(minBalance *big.Int, maxBalance *big.Int) (*types.Transaction, error) {
	return _IRequest.Contract.SetRepaid(&_IRequest.TransactOpts, minBalance, maxBalance)
}

// SyncRepaidStatus is a paid mutator transaction binding the contract method 0xf0d38777.
//
// Solidity: function syncRepaidStatus() returns(bool repaid)
func (_IRequest *IRequestTransactor) SyncRepaidStatus(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IRequest.contract.Transact(opts, "syncRepaidStatus")
}

// SyncRepaidStatus is a paid mutator transaction binding the contract method 0xf0d38777.
//
// Solidity: function syncRepaidStatus() returns(bool repaid)
func (_IRequest *IRequestSession) SyncRepaidStatus() (*types.Transaction, error) {
	return _IRequest.Contract.SyncRepaidStatus(&_IRequest.TransactOpts)
}

// SyncRepaidStatus is a paid mutator transaction binding the contract method 0xf0d38777.
//
// Solidity: function syncRepaidStatus() returns(bool repaid)
func (_IRequest *IRequestTransactorSession) SyncRepaidStatus() (*types.Transaction, error) {
	return _IRequest.Contract.SyncRepaidStatus(&_IRequest.TransactOpts)
}

// IRequestAuthorizedMintingIterator is returned from FilterAuthorizedMinting and is used to iterate over the raw logs and unpacked data for AuthorizedMinting events raised by the IRequest contract.
type IRequestAuthorizedMintingIterator struct {
	Event *IRequestAuthorizedMinting // Event containing the contract specifics and raw log

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
func (it *IRequestAuthorizedMintingIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IRequestAuthorizedMinting)
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
		it.Event = new(IRequestAuthorizedMinting)
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
func (it *IRequestAuthorizedMintingIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IRequestAuthorizedMintingIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IRequestAuthorizedMinting represents a AuthorizedMinting event raised by the IRequest contract.
type IRequestAuthorizedMinting struct {
	To       common.Address
	PtAmount *big.Int
	YtAmount *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterAuthorizedMinting is a free log retrieval operation binding the contract event 0xbd5e3b7f98154dc8659767673a8d300d69bf09e88b2c6d69ba770fd17900a5f8.
//
// Solidity: event AuthorizedMinting(address indexed to, uint256 ptAmount, uint256 ytAmount)
func (_IRequest *IRequestFilterer) FilterAuthorizedMinting(opts *bind.FilterOpts, to []common.Address) (*IRequestAuthorizedMintingIterator, error) {

	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _IRequest.contract.FilterLogs(opts, "AuthorizedMinting", toRule)
	if err != nil {
		return nil, err
	}
	return &IRequestAuthorizedMintingIterator{contract: _IRequest.contract, event: "AuthorizedMinting", logs: logs, sub: sub}, nil
}

// WatchAuthorizedMinting is a free log subscription operation binding the contract event 0xbd5e3b7f98154dc8659767673a8d300d69bf09e88b2c6d69ba770fd17900a5f8.
//
// Solidity: event AuthorizedMinting(address indexed to, uint256 ptAmount, uint256 ytAmount)
func (_IRequest *IRequestFilterer) WatchAuthorizedMinting(opts *bind.WatchOpts, sink chan<- *IRequestAuthorizedMinting, to []common.Address) (event.Subscription, error) {

	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _IRequest.contract.WatchLogs(opts, "AuthorizedMinting", toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IRequestAuthorizedMinting)
				if err := _IRequest.contract.UnpackLog(event, "AuthorizedMinting", log); err != nil {
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

// ParseAuthorizedMinting is a log parse operation binding the contract event 0xbd5e3b7f98154dc8659767673a8d300d69bf09e88b2c6d69ba770fd17900a5f8.
//
// Solidity: event AuthorizedMinting(address indexed to, uint256 ptAmount, uint256 ytAmount)
func (_IRequest *IRequestFilterer) ParseAuthorizedMinting(log types.Log) (*IRequestAuthorizedMinting, error) {
	event := new(IRequestAuthorizedMinting)
	if err := _IRequest.contract.UnpackLog(event, "AuthorizedMinting", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IRequestFundsPulledIterator is returned from FilterFundsPulled and is used to iterate over the raw logs and unpacked data for FundsPulled events raised by the IRequest contract.
type IRequestFundsPulledIterator struct {
	Event *IRequestFundsPulled // Event containing the contract specifics and raw log

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
func (it *IRequestFundsPulledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IRequestFundsPulled)
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
		it.Event = new(IRequestFundsPulled)
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
func (it *IRequestFundsPulledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IRequestFundsPulledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IRequestFundsPulled represents a FundsPulled event raised by the IRequest contract.
type IRequestFundsPulled struct {
	Puller common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterFundsPulled is a free log retrieval operation binding the contract event 0x1a6b3ffc38e569eb77b736c56f6d826b44ad9f3df5f63ec4eac3dcb444668ea5.
//
// Solidity: event FundsPulled(address indexed puller, uint256 amount)
func (_IRequest *IRequestFilterer) FilterFundsPulled(opts *bind.FilterOpts, puller []common.Address) (*IRequestFundsPulledIterator, error) {

	var pullerRule []interface{}
	for _, pullerItem := range puller {
		pullerRule = append(pullerRule, pullerItem)
	}

	logs, sub, err := _IRequest.contract.FilterLogs(opts, "FundsPulled", pullerRule)
	if err != nil {
		return nil, err
	}
	return &IRequestFundsPulledIterator{contract: _IRequest.contract, event: "FundsPulled", logs: logs, sub: sub}, nil
}

// WatchFundsPulled is a free log subscription operation binding the contract event 0x1a6b3ffc38e569eb77b736c56f6d826b44ad9f3df5f63ec4eac3dcb444668ea5.
//
// Solidity: event FundsPulled(address indexed puller, uint256 amount)
func (_IRequest *IRequestFilterer) WatchFundsPulled(opts *bind.WatchOpts, sink chan<- *IRequestFundsPulled, puller []common.Address) (event.Subscription, error) {

	var pullerRule []interface{}
	for _, pullerItem := range puller {
		pullerRule = append(pullerRule, pullerItem)
	}

	logs, sub, err := _IRequest.contract.WatchLogs(opts, "FundsPulled", pullerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IRequestFundsPulled)
				if err := _IRequest.contract.UnpackLog(event, "FundsPulled", log); err != nil {
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

// ParseFundsPulled is a log parse operation binding the contract event 0x1a6b3ffc38e569eb77b736c56f6d826b44ad9f3df5f63ec4eac3dcb444668ea5.
//
// Solidity: event FundsPulled(address indexed puller, uint256 amount)
func (_IRequest *IRequestFilterer) ParseFundsPulled(log types.Log) (*IRequestFundsPulled, error) {
	event := new(IRequestFundsPulled)
	if err := _IRequest.contract.UnpackLog(event, "FundsPulled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IRequestMintToRepaidDelaySetIterator is returned from FilterMintToRepaidDelaySet and is used to iterate over the raw logs and unpacked data for MintToRepaidDelaySet events raised by the IRequest contract.
type IRequestMintToRepaidDelaySetIterator struct {
	Event *IRequestMintToRepaidDelaySet // Event containing the contract specifics and raw log

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
func (it *IRequestMintToRepaidDelaySetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IRequestMintToRepaidDelaySet)
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
		it.Event = new(IRequestMintToRepaidDelaySet)
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
func (it *IRequestMintToRepaidDelaySetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IRequestMintToRepaidDelaySetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IRequestMintToRepaidDelaySet represents a MintToRepaidDelaySet event raised by the IRequest contract.
type IRequestMintToRepaidDelaySet struct {
	MintToRepaidDelay *big.Int
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterMintToRepaidDelaySet is a free log retrieval operation binding the contract event 0x1ea66a36a1751438916d12ac1a03312e5aaff0f5838648df63dbe844407c0f9a.
//
// Solidity: event MintToRepaidDelaySet(uint40 mintToRepaidDelay)
func (_IRequest *IRequestFilterer) FilterMintToRepaidDelaySet(opts *bind.FilterOpts) (*IRequestMintToRepaidDelaySetIterator, error) {

	logs, sub, err := _IRequest.contract.FilterLogs(opts, "MintToRepaidDelaySet")
	if err != nil {
		return nil, err
	}
	return &IRequestMintToRepaidDelaySetIterator{contract: _IRequest.contract, event: "MintToRepaidDelaySet", logs: logs, sub: sub}, nil
}

// WatchMintToRepaidDelaySet is a free log subscription operation binding the contract event 0x1ea66a36a1751438916d12ac1a03312e5aaff0f5838648df63dbe844407c0f9a.
//
// Solidity: event MintToRepaidDelaySet(uint40 mintToRepaidDelay)
func (_IRequest *IRequestFilterer) WatchMintToRepaidDelaySet(opts *bind.WatchOpts, sink chan<- *IRequestMintToRepaidDelaySet) (event.Subscription, error) {

	logs, sub, err := _IRequest.contract.WatchLogs(opts, "MintToRepaidDelaySet")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IRequestMintToRepaidDelaySet)
				if err := _IRequest.contract.UnpackLog(event, "MintToRepaidDelaySet", log); err != nil {
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

// ParseMintToRepaidDelaySet is a log parse operation binding the contract event 0x1ea66a36a1751438916d12ac1a03312e5aaff0f5838648df63dbe844407c0f9a.
//
// Solidity: event MintToRepaidDelaySet(uint40 mintToRepaidDelay)
func (_IRequest *IRequestFilterer) ParseMintToRepaidDelaySet(log types.Log) (*IRequestMintToRepaidDelaySet, error) {
	event := new(IRequestMintToRepaidDelaySet)
	if err := _IRequest.contract.UnpackLog(event, "MintToRepaidDelaySet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IRequestRepaidIterator is returned from FilterRepaid and is used to iterate over the raw logs and unpacked data for Repaid events raised by the IRequest contract.
type IRequestRepaidIterator struct {
	Event *IRequestRepaid // Event containing the contract specifics and raw log

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
func (it *IRequestRepaidIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IRequestRepaid)
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
		it.Event = new(IRequestRepaid)
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
func (it *IRequestRepaidIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IRequestRepaidIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IRequestRepaid represents a Repaid event raised by the IRequest contract.
type IRequestRepaid struct {
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRepaid is a free log retrieval operation binding the contract event 0x33a382daad6aace935340a474d09fec82af4bec7e2b69518d283231b03a65f24.
//
// Solidity: event Repaid(uint256 amount)
func (_IRequest *IRequestFilterer) FilterRepaid(opts *bind.FilterOpts) (*IRequestRepaidIterator, error) {

	logs, sub, err := _IRequest.contract.FilterLogs(opts, "Repaid")
	if err != nil {
		return nil, err
	}
	return &IRequestRepaidIterator{contract: _IRequest.contract, event: "Repaid", logs: logs, sub: sub}, nil
}

// WatchRepaid is a free log subscription operation binding the contract event 0x33a382daad6aace935340a474d09fec82af4bec7e2b69518d283231b03a65f24.
//
// Solidity: event Repaid(uint256 amount)
func (_IRequest *IRequestFilterer) WatchRepaid(opts *bind.WatchOpts, sink chan<- *IRequestRepaid) (event.Subscription, error) {

	logs, sub, err := _IRequest.contract.WatchLogs(opts, "Repaid")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IRequestRepaid)
				if err := _IRequest.contract.UnpackLog(event, "Repaid", log); err != nil {
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

// ParseRepaid is a log parse operation binding the contract event 0x33a382daad6aace935340a474d09fec82af4bec7e2b69518d283231b03a65f24.
//
// Solidity: event Repaid(uint256 amount)
func (_IRequest *IRequestFilterer) ParseRepaid(log types.Log) (*IRequestRepaid, error) {
	event := new(IRequestRepaid)
	if err := _IRequest.contract.UnpackLog(event, "Repaid", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
