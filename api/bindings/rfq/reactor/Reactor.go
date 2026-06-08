// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package reactor

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

// IInstantRedemptionAdapterDiscount is an auto generated low-level Go binding around an user-defined struct.
type IInstantRedemptionAdapterDiscount struct {
	Vault         common.Address
	TokenToRedeem common.Address
	Discount      *big.Int
	Signer        common.Address
	Protocol      common.Address
	Nonce         *big.Int
	Deadline      *big.Int
}

// IInstantRedemptionAdapterDiscountSwap is an auto generated low-level Go binding around an user-defined struct.
type IInstantRedemptionAdapterDiscountSwap struct {
	Discount         IInstantRedemptionAdapterDiscount
	SignerSignature  []byte
	ProtocolDeadline *big.Int
}

// IInstantRedemptionAdapterSwap is an auto generated low-level Go binding around an user-defined struct.
type IInstantRedemptionAdapterSwap struct {
	Recipient common.Address
	Vault     common.Address
	TokenIn   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

// IReactorDiscountSwapInput is an auto generated low-level Go binding around an user-defined struct.
type IReactorDiscountSwapInput struct {
	DiscountSwap      IInstantRedemptionAdapterDiscountSwap
	ProtocolSignature []byte
	Recipient         common.Address
	AmountIn          *big.Int
	AmountOut         *big.Int
}

// IReactorOrder is an auto generated low-level Go binding around an user-defined struct.
type IReactorOrder struct {
	Request          IReactorRequest
	SwapperSignature []byte
	Swapper          common.Address
	Filler           common.Address
}

// IReactorOutput is an auto generated low-level Go binding around an user-defined struct.
type IReactorOutput struct {
	Token     common.Address
	Amount    *big.Int
	Recipient common.Address
}

// IReactorRequest is an auto generated low-level Go binding around an user-defined struct.
type IReactorRequest struct {
	TokenIn  common.Address
	AmountIn *big.Int
	Outputs  []IReactorOutput
	Deadline *big.Int
	Nonce    *big.Int
	Protocol common.Address
}

// ReactorMetaData contains all meta data concerning the Reactor contract.
var ReactorMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"permit2\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"receive\",\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIInstantRedemptionAdapter.Swap[]\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"discountSwapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.DiscountSwapInput[]\",\"components\":[{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structIInstantRedemptionAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structIInstantRedemptionAdapter.Discount\",\"components\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIInstantRedemptionAdapter.Swap[]\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structIInstantRedemptionAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"Fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"InvalidAmountIn\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidFiller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOutput\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidProtocolSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTokenIn\",\"inputs\":[]}]",
}

// ReactorABI is the input ABI used to generate the binding from.
// Deprecated: Use ReactorMetaData.ABI instead.
var ReactorABI = ReactorMetaData.ABI

// Reactor is an auto generated Go binding around an Ethereum contract.
type Reactor struct {
	ReactorCaller     // Read-only binding to the contract
	ReactorTransactor // Write-only binding to the contract
	ReactorFilterer   // Log filterer for contract events
}

// ReactorCaller is an auto generated read-only Go binding around an Ethereum contract.
type ReactorCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ReactorTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ReactorTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ReactorFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ReactorFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ReactorSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ReactorSession struct {
	Contract     *Reactor          // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ReactorCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ReactorCallerSession struct {
	Contract *ReactorCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts  // Call options to use throughout this session
}

// ReactorTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ReactorTransactorSession struct {
	Contract     *ReactorTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// ReactorRaw is an auto generated low-level Go binding around an Ethereum contract.
type ReactorRaw struct {
	Contract *Reactor // Generic contract binding to access the raw methods on
}

// ReactorCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ReactorCallerRaw struct {
	Contract *ReactorCaller // Generic read-only contract binding to access the raw methods on
}

// ReactorTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ReactorTransactorRaw struct {
	Contract *ReactorTransactor // Generic write-only contract binding to access the raw methods on
}

// NewReactor creates a new instance of Reactor, bound to a specific deployed contract.
func NewReactor(address common.Address, backend bind.ContractBackend) (*Reactor, error) {
	contract, err := bindReactor(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Reactor{ReactorCaller: ReactorCaller{contract: contract}, ReactorTransactor: ReactorTransactor{contract: contract}, ReactorFilterer: ReactorFilterer{contract: contract}}, nil
}

// NewReactorCaller creates a new read-only instance of Reactor, bound to a specific deployed contract.
func NewReactorCaller(address common.Address, caller bind.ContractCaller) (*ReactorCaller, error) {
	contract, err := bindReactor(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ReactorCaller{contract: contract}, nil
}

// NewReactorTransactor creates a new write-only instance of Reactor, bound to a specific deployed contract.
func NewReactorTransactor(address common.Address, transactor bind.ContractTransactor) (*ReactorTransactor, error) {
	contract, err := bindReactor(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ReactorTransactor{contract: contract}, nil
}

// NewReactorFilterer creates a new log filterer instance of Reactor, bound to a specific deployed contract.
func NewReactorFilterer(address common.Address, filterer bind.ContractFilterer) (*ReactorFilterer, error) {
	contract, err := bindReactor(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ReactorFilterer{contract: contract}, nil
}

// bindReactor binds a generic wrapper to an already deployed contract.
func bindReactor(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ReactorMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Reactor *ReactorRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Reactor.Contract.ReactorCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Reactor *ReactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Reactor.Contract.ReactorTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Reactor *ReactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Reactor.Contract.ReactorTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Reactor *ReactorCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Reactor.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Reactor *ReactorTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Reactor.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Reactor *ReactorTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Reactor.Contract.contract.Transact(opts, method, params...)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_Reactor *ReactorCaller) Eip712Domain(opts *bind.CallOpts) (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	var out []interface{}
	err := _Reactor.contract.Call(opts, &out, "eip712Domain")

	outstruct := new(struct {
		Fields            [1]byte
		Name              string
		Version           string
		ChainId           *big.Int
		VerifyingContract common.Address
		Salt              [32]byte
		Extensions        []*big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)

	return *outstruct, err

}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_Reactor *ReactorSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _Reactor.Contract.Eip712Domain(&_Reactor.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_Reactor *ReactorCallerSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _Reactor.Contract.Eip712Domain(&_Reactor.CallOpts)
}

// Fill is a paid mutator transaction binding the contract method 0x0a92fc9d.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256)[] swapInputs, (((address,address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Reactor *ReactorTransactor) Fill(opts *bind.TransactOpts, order IReactorOrder, protocolSignature []byte, swapInputs []IInstantRedemptionAdapterSwap, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Reactor.contract.Transact(opts, "fill", order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// Fill is a paid mutator transaction binding the contract method 0x0a92fc9d.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256)[] swapInputs, (((address,address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Reactor *ReactorSession) Fill(order IReactorOrder, protocolSignature []byte, swapInputs []IInstantRedemptionAdapterSwap, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Reactor.Contract.Fill(&_Reactor.TransactOpts, order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// Fill is a paid mutator transaction binding the contract method 0x0a92fc9d.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256)[] swapInputs, (((address,address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Reactor *ReactorTransactorSession) Fill(order IReactorOrder, protocolSignature []byte, swapInputs []IInstantRedemptionAdapterSwap, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Reactor.Contract.Fill(&_Reactor.TransactOpts, order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// Fill0 is a paid mutator transaction binding the contract method 0x3240cc69.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256)[] swapInputs, bytes executorData) returns()
func (_Reactor *ReactorTransactor) Fill0(opts *bind.TransactOpts, order IReactorOrder, protocolSignature []byte, swapInputs []IInstantRedemptionAdapterSwap, executorData []byte) (*types.Transaction, error) {
	return _Reactor.contract.Transact(opts, "fill0", order, protocolSignature, swapInputs, executorData)
}

// Fill0 is a paid mutator transaction binding the contract method 0x3240cc69.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256)[] swapInputs, bytes executorData) returns()
func (_Reactor *ReactorSession) Fill0(order IReactorOrder, protocolSignature []byte, swapInputs []IInstantRedemptionAdapterSwap, executorData []byte) (*types.Transaction, error) {
	return _Reactor.Contract.Fill0(&_Reactor.TransactOpts, order, protocolSignature, swapInputs, executorData)
}

// Fill0 is a paid mutator transaction binding the contract method 0x3240cc69.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256)[] swapInputs, bytes executorData) returns()
func (_Reactor *ReactorTransactorSession) Fill0(order IReactorOrder, protocolSignature []byte, swapInputs []IInstantRedemptionAdapterSwap, executorData []byte) (*types.Transaction, error) {
	return _Reactor.Contract.Fill0(&_Reactor.TransactOpts, order, protocolSignature, swapInputs, executorData)
}

// Fill1 is a paid mutator transaction binding the contract method 0xf1a7b5bb.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256) swap, bytes executorData) returns()
func (_Reactor *ReactorTransactor) Fill1(opts *bind.TransactOpts, order IReactorOrder, protocolSignature []byte, swap IInstantRedemptionAdapterSwap, executorData []byte) (*types.Transaction, error) {
	return _Reactor.contract.Transact(opts, "fill1", order, protocolSignature, swap, executorData)
}

// Fill1 is a paid mutator transaction binding the contract method 0xf1a7b5bb.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256) swap, bytes executorData) returns()
func (_Reactor *ReactorSession) Fill1(order IReactorOrder, protocolSignature []byte, swap IInstantRedemptionAdapterSwap, executorData []byte) (*types.Transaction, error) {
	return _Reactor.Contract.Fill1(&_Reactor.TransactOpts, order, protocolSignature, swap, executorData)
}

// Fill1 is a paid mutator transaction binding the contract method 0xf1a7b5bb.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,address,address,uint256,uint256) swap, bytes executorData) returns()
func (_Reactor *ReactorTransactorSession) Fill1(order IReactorOrder, protocolSignature []byte, swap IInstantRedemptionAdapterSwap, executorData []byte) (*types.Transaction, error) {
	return _Reactor.Contract.Fill1(&_Reactor.TransactOpts, order, protocolSignature, swap, executorData)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Reactor *ReactorTransactor) Receive(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Reactor.contract.RawTransact(opts, nil) // calldata is disallowed for receive function
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Reactor *ReactorSession) Receive() (*types.Transaction, error) {
	return _Reactor.Contract.Receive(&_Reactor.TransactOpts)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Reactor *ReactorTransactorSession) Receive() (*types.Transaction, error) {
	return _Reactor.Contract.Receive(&_Reactor.TransactOpts)
}

// ReactorFillIterator is returned from FilterFill and is used to iterate over the raw logs and unpacked data for Fill events raised by the Reactor contract.
type ReactorFillIterator struct {
	Event *ReactorFill // Event containing the contract specifics and raw log

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
func (it *ReactorFillIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ReactorFill)
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
		it.Event = new(ReactorFill)
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
func (it *ReactorFillIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ReactorFillIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ReactorFill represents a Fill event raised by the Reactor contract.
type ReactorFill struct {
	Order IReactorOrder
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterFill is a free log retrieval operation binding the contract event 0x74d0d4b8c4779bc4fd2992b6d1aca0e079606e557bf920ac20ff77f3b9ddb3f5.
//
// Solidity: event Fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order)
func (_Reactor *ReactorFilterer) FilterFill(opts *bind.FilterOpts) (*ReactorFillIterator, error) {

	logs, sub, err := _Reactor.contract.FilterLogs(opts, "Fill")
	if err != nil {
		return nil, err
	}
	return &ReactorFillIterator{contract: _Reactor.contract, event: "Fill", logs: logs, sub: sub}, nil
}

// WatchFill is a free log subscription operation binding the contract event 0x74d0d4b8c4779bc4fd2992b6d1aca0e079606e557bf920ac20ff77f3b9ddb3f5.
//
// Solidity: event Fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order)
func (_Reactor *ReactorFilterer) WatchFill(opts *bind.WatchOpts, sink chan<- *ReactorFill) (event.Subscription, error) {

	logs, sub, err := _Reactor.contract.WatchLogs(opts, "Fill")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ReactorFill)
				if err := _Reactor.contract.UnpackLog(event, "Fill", log); err != nil {
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

// ParseFill is a log parse operation binding the contract event 0x74d0d4b8c4779bc4fd2992b6d1aca0e079606e557bf920ac20ff77f3b9ddb3f5.
//
// Solidity: event Fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order)
func (_Reactor *ReactorFilterer) ParseFill(log types.Log) (*ReactorFill, error) {
	event := new(ReactorFill)
	if err := _Reactor.contract.UnpackLog(event, "Fill", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
