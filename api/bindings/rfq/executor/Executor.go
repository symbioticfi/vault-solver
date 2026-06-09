// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package executor

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

// ILiquidLaneAdapterDiscount is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterDiscount struct {
	TokenToRedeem common.Address
	Discount      *big.Int
	Signer        common.Address
	Protocol      common.Address
	Nonce         *big.Int
	Deadline      *big.Int
}

// ILiquidLaneAdapterDiscountSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterDiscountSwap struct {
	Discount         ILiquidLaneAdapterDiscount
	SignerSignature  []byte
	ProtocolDeadline *big.Int
}

// ILiquidLaneAdapterSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterSwap struct {
	Recipient common.Address
	TokenIn   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

// IReactorDiscountSwapInput is an auto generated low-level Go binding around an user-defined struct.
type IReactorDiscountSwapInput struct {
	Adapter           common.Address
	DiscountSwap      ILiquidLaneAdapterDiscountSwap
	ProtocolSignature []byte
	Recipient         common.Address
	AmountIn          *big.Int
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

// IReactorSwapInput is an auto generated low-level Go binding around an user-defined struct.
type IReactorSwapInput struct {
	Adapter common.Address
	Swap    ILiquidLaneAdapterSwap
}

// ExecutorMetaData contains all meta data concerning the Executor contract.
var ExecutorMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"reactor\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"receive\",\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"callers\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"execute\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"discountSwapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.DiscountSwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"discountSwapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.DiscountSwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInput\",\"type\":\"tuple\",\"internalType\":\"structIReactor.SwapInput\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"indexed\":false,\"internalType\":\"address[]\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AddressEmptyCode\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"FailedCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientBalance\",\"inputs\":[{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotReactor\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
}

// ExecutorABI is the input ABI used to generate the binding from.
// Deprecated: Use ExecutorMetaData.ABI instead.
var ExecutorABI = ExecutorMetaData.ABI

// Executor is an auto generated Go binding around an Ethereum contract.
type Executor struct {
	ExecutorCaller     // Read-only binding to the contract
	ExecutorTransactor // Write-only binding to the contract
	ExecutorFilterer   // Log filterer for contract events
}

// ExecutorCaller is an auto generated read-only Go binding around an Ethereum contract.
type ExecutorCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExecutorTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ExecutorTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExecutorFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ExecutorFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExecutorSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ExecutorSession struct {
	Contract     *Executor         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ExecutorCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ExecutorCallerSession struct {
	Contract *ExecutorCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// ExecutorTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ExecutorTransactorSession struct {
	Contract     *ExecutorTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// ExecutorRaw is an auto generated low-level Go binding around an Ethereum contract.
type ExecutorRaw struct {
	Contract *Executor // Generic contract binding to access the raw methods on
}

// ExecutorCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ExecutorCallerRaw struct {
	Contract *ExecutorCaller // Generic read-only contract binding to access the raw methods on
}

// ExecutorTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ExecutorTransactorRaw struct {
	Contract *ExecutorTransactor // Generic write-only contract binding to access the raw methods on
}

// NewExecutor creates a new instance of Executor, bound to a specific deployed contract.
func NewExecutor(address common.Address, backend bind.ContractBackend) (*Executor, error) {
	contract, err := bindExecutor(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Executor{ExecutorCaller: ExecutorCaller{contract: contract}, ExecutorTransactor: ExecutorTransactor{contract: contract}, ExecutorFilterer: ExecutorFilterer{contract: contract}}, nil
}

// NewExecutorCaller creates a new read-only instance of Executor, bound to a specific deployed contract.
func NewExecutorCaller(address common.Address, caller bind.ContractCaller) (*ExecutorCaller, error) {
	contract, err := bindExecutor(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ExecutorCaller{contract: contract}, nil
}

// NewExecutorTransactor creates a new write-only instance of Executor, bound to a specific deployed contract.
func NewExecutorTransactor(address common.Address, transactor bind.ContractTransactor) (*ExecutorTransactor, error) {
	contract, err := bindExecutor(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ExecutorTransactor{contract: contract}, nil
}

// NewExecutorFilterer creates a new log filterer instance of Executor, bound to a specific deployed contract.
func NewExecutorFilterer(address common.Address, filterer bind.ContractFilterer) (*ExecutorFilterer, error) {
	contract, err := bindExecutor(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ExecutorFilterer{contract: contract}, nil
}

// bindExecutor binds a generic wrapper to an already deployed contract.
func bindExecutor(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ExecutorMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Executor *ExecutorRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Executor.Contract.ExecutorCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Executor *ExecutorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Executor.Contract.ExecutorTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Executor *ExecutorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Executor.Contract.ExecutorTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Executor *ExecutorCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Executor.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Executor *ExecutorTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Executor.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Executor *ExecutorTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Executor.Contract.contract.Transact(opts, method, params...)
}

// Callers is a free data retrieval call binding the contract method 0xaa03fa3d.
//
// Solidity: function callers(uint256 ) view returns(address)
func (_Executor *ExecutorCaller) Callers(opts *bind.CallOpts, arg0 *big.Int) (common.Address, error) {
	var out []interface{}
	err := _Executor.contract.Call(opts, &out, "callers", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Callers is a free data retrieval call binding the contract method 0xaa03fa3d.
//
// Solidity: function callers(uint256 ) view returns(address)
func (_Executor *ExecutorSession) Callers(arg0 *big.Int) (common.Address, error) {
	return _Executor.Contract.Callers(&_Executor.CallOpts, arg0)
}

// Callers is a free data retrieval call binding the contract method 0xaa03fa3d.
//
// Solidity: function callers(uint256 ) view returns(address)
func (_Executor *ExecutorCallerSession) Callers(arg0 *big.Int) (common.Address, error) {
	return _Executor.Contract.Callers(&_Executor.CallOpts, arg0)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Executor *ExecutorCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Executor.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Executor *ExecutorSession) Owner() (common.Address, error) {
	return _Executor.Contract.Owner(&_Executor.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Executor *ExecutorCallerSession) Owner() (common.Address, error) {
	return _Executor.Contract.Owner(&_Executor.CallOpts)
}

// Execute is a paid mutator transaction binding the contract method 0xa3b18964.
//
// Solidity: function execute(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Executor *ExecutorTransactor) Execute(opts *bind.TransactOpts, order IReactorOrder, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.contract.Transact(opts, "execute", order, swapInputs, discountSwapInputs, executorData)
}

// Execute is a paid mutator transaction binding the contract method 0xa3b18964.
//
// Solidity: function execute(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Executor *ExecutorSession) Execute(order IReactorOrder, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Execute(&_Executor.TransactOpts, order, swapInputs, discountSwapInputs, executorData)
}

// Execute is a paid mutator transaction binding the contract method 0xa3b18964.
//
// Solidity: function execute(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Executor *ExecutorTransactorSession) Execute(order IReactorOrder, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Execute(&_Executor.TransactOpts, order, swapInputs, discountSwapInputs, executorData)
}

// Fill is a paid mutator transaction binding the contract method 0x2b137442.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Executor *ExecutorTransactor) Fill(opts *bind.TransactOpts, order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.contract.Transact(opts, "fill", order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// Fill is a paid mutator transaction binding the contract method 0x2b137442.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Executor *ExecutorSession) Fill(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Fill(&_Executor.TransactOpts, order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// Fill is a paid mutator transaction binding the contract method 0x2b137442.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (_Executor *ExecutorTransactorSession) Fill(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Fill(&_Executor.TransactOpts, order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// Fill0 is a paid mutator transaction binding the contract method 0x33891b08.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, bytes executorData) returns()
func (_Executor *ExecutorTransactor) Fill0(opts *bind.TransactOpts, order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.contract.Transact(opts, "fill0", order, protocolSignature, swapInputs, executorData)
}

// Fill0 is a paid mutator transaction binding the contract method 0x33891b08.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, bytes executorData) returns()
func (_Executor *ExecutorSession) Fill0(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Fill0(&_Executor.TransactOpts, order, protocolSignature, swapInputs, executorData)
}

// Fill0 is a paid mutator transaction binding the contract method 0x33891b08.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, bytes executorData) returns()
func (_Executor *ExecutorTransactorSession) Fill0(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Fill0(&_Executor.TransactOpts, order, protocolSignature, swapInputs, executorData)
}

// Fill1 is a paid mutator transaction binding the contract method 0xc1c2b99f.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256)) swapInput, bytes executorData) returns()
func (_Executor *ExecutorTransactor) Fill1(opts *bind.TransactOpts, order IReactorOrder, protocolSignature []byte, swapInput IReactorSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.contract.Transact(opts, "fill1", order, protocolSignature, swapInput, executorData)
}

// Fill1 is a paid mutator transaction binding the contract method 0xc1c2b99f.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256)) swapInput, bytes executorData) returns()
func (_Executor *ExecutorSession) Fill1(order IReactorOrder, protocolSignature []byte, swapInput IReactorSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Fill1(&_Executor.TransactOpts, order, protocolSignature, swapInput, executorData)
}

// Fill1 is a paid mutator transaction binding the contract method 0xc1c2b99f.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256)) swapInput, bytes executorData) returns()
func (_Executor *ExecutorTransactorSession) Fill1(order IReactorOrder, protocolSignature []byte, swapInput IReactorSwapInput, executorData []byte) (*types.Transaction, error) {
	return _Executor.Contract.Fill1(&_Executor.TransactOpts, order, protocolSignature, swapInput, executorData)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Executor *ExecutorTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Executor.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Executor *ExecutorSession) RenounceOwnership() (*types.Transaction, error) {
	return _Executor.Contract.RenounceOwnership(&_Executor.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Executor *ExecutorTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _Executor.Contract.RenounceOwnership(&_Executor.TransactOpts)
}

// SetCallers is a paid mutator transaction binding the contract method 0x43ded848.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (_Executor *ExecutorTransactor) SetCallers(opts *bind.TransactOpts, newCallers []common.Address) (*types.Transaction, error) {
	return _Executor.contract.Transact(opts, "setCallers", newCallers)
}

// SetCallers is a paid mutator transaction binding the contract method 0x43ded848.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (_Executor *ExecutorSession) SetCallers(newCallers []common.Address) (*types.Transaction, error) {
	return _Executor.Contract.SetCallers(&_Executor.TransactOpts, newCallers)
}

// SetCallers is a paid mutator transaction binding the contract method 0x43ded848.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (_Executor *ExecutorTransactorSession) SetCallers(newCallers []common.Address) (*types.Transaction, error) {
	return _Executor.Contract.SetCallers(&_Executor.TransactOpts, newCallers)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Executor *ExecutorTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _Executor.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Executor *ExecutorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _Executor.Contract.TransferOwnership(&_Executor.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Executor *ExecutorTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _Executor.Contract.TransferOwnership(&_Executor.TransactOpts, newOwner)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Executor *ExecutorTransactor) Receive(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Executor.contract.RawTransact(opts, nil) // calldata is disallowed for receive function
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Executor *ExecutorSession) Receive() (*types.Transaction, error) {
	return _Executor.Contract.Receive(&_Executor.TransactOpts)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Executor *ExecutorTransactorSession) Receive() (*types.Transaction, error) {
	return _Executor.Contract.Receive(&_Executor.TransactOpts)
}

// ExecutorOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the Executor contract.
type ExecutorOwnershipTransferredIterator struct {
	Event *ExecutorOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *ExecutorOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ExecutorOwnershipTransferred)
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
		it.Event = new(ExecutorOwnershipTransferred)
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
func (it *ExecutorOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ExecutorOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ExecutorOwnershipTransferred represents a OwnershipTransferred event raised by the Executor contract.
type ExecutorOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_Executor *ExecutorFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*ExecutorOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _Executor.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &ExecutorOwnershipTransferredIterator{contract: _Executor.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_Executor *ExecutorFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *ExecutorOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _Executor.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ExecutorOwnershipTransferred)
				if err := _Executor.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_Executor *ExecutorFilterer) ParseOwnershipTransferred(log types.Log) (*ExecutorOwnershipTransferred, error) {
	event := new(ExecutorOwnershipTransferred)
	if err := _Executor.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ExecutorSetCallersIterator is returned from FilterSetCallers and is used to iterate over the raw logs and unpacked data for SetCallers events raised by the Executor contract.
type ExecutorSetCallersIterator struct {
	Event *ExecutorSetCallers // Event containing the contract specifics and raw log

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
func (it *ExecutorSetCallersIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ExecutorSetCallers)
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
		it.Event = new(ExecutorSetCallers)
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
func (it *ExecutorSetCallersIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ExecutorSetCallersIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ExecutorSetCallers represents a SetCallers event raised by the Executor contract.
type ExecutorSetCallers struct {
	NewCallers []common.Address
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterSetCallers is a free log retrieval operation binding the contract event 0x0e123de96057b6051041e23e21f5c45be29329e5dde6f3a360901e20395ad0fd.
//
// Solidity: event SetCallers(address[] newCallers)
func (_Executor *ExecutorFilterer) FilterSetCallers(opts *bind.FilterOpts) (*ExecutorSetCallersIterator, error) {

	logs, sub, err := _Executor.contract.FilterLogs(opts, "SetCallers")
	if err != nil {
		return nil, err
	}
	return &ExecutorSetCallersIterator{contract: _Executor.contract, event: "SetCallers", logs: logs, sub: sub}, nil
}

// WatchSetCallers is a free log subscription operation binding the contract event 0x0e123de96057b6051041e23e21f5c45be29329e5dde6f3a360901e20395ad0fd.
//
// Solidity: event SetCallers(address[] newCallers)
func (_Executor *ExecutorFilterer) WatchSetCallers(opts *bind.WatchOpts, sink chan<- *ExecutorSetCallers) (event.Subscription, error) {

	logs, sub, err := _Executor.contract.WatchLogs(opts, "SetCallers")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ExecutorSetCallers)
				if err := _Executor.contract.UnpackLog(event, "SetCallers", log); err != nil {
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

// ParseSetCallers is a log parse operation binding the contract event 0x0e123de96057b6051041e23e21f5c45be29329e5dde6f3a360901e20395ad0fd.
//
// Solidity: event SetCallers(address[] newCallers)
func (_Executor *ExecutorFilterer) ParseSetCallers(log types.Log) (*ExecutorSetCallers, error) {
	event := new(ExecutorSetCallers)
	if err := _Executor.contract.UnpackLog(event, "SetCallers", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
