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
var ExecutorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"reactor\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"receive\",\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"callers\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"execute\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"discountSwapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.DiscountSwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"discountSwapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.DiscountSwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInput\",\"type\":\"tuple\",\"internalType\":\"structIReactor.SwapInput\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"indexed\":false,\"internalType\":\"address[]\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AddressEmptyCode\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"FailedCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientBalance\",\"inputs\":[{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotReactor\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
	ID:  "Executor",
}

// Executor is an auto generated Go binding around an Ethereum contract.
type Executor struct {
	abi abi.ABI
}

// NewExecutor creates a new instance of Executor.
func NewExecutor() *Executor {
	parsed, err := ExecutorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &Executor{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *Executor) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address reactor, address owner, address[] initCallers) returns()
func (executor *Executor) PackConstructor(reactor common.Address, owner common.Address, initCallers []common.Address) []byte {
	enc, err := executor.abi.Pack("", reactor, owner, initCallers)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaa03fa3d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function callers(uint256 ) view returns(address)
func (executor *Executor) PackCallers(arg0 *big.Int) []byte {
	enc, err := executor.abi.Pack("callers", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaa03fa3d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function callers(uint256 ) view returns(address)
func (executor *Executor) TryPackCallers(arg0 *big.Int) ([]byte, error) {
	return executor.abi.Pack("callers", arg0)
}

// UnpackCallers is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xaa03fa3d.
//
// Solidity: function callers(uint256 ) view returns(address)
func (executor *Executor) UnpackCallers(data []byte) (common.Address, error) {
	out, err := executor.abi.Unpack("callers", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackExecute is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa3b18964.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function execute(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (executor *Executor) PackExecute(order IReactorOrder, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) []byte {
	enc, err := executor.abi.Pack("execute", order, swapInputs, discountSwapInputs, executorData)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackExecute is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa3b18964.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function execute(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (executor *Executor) TryPackExecute(order IReactorOrder, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) ([]byte, error) {
	return executor.abi.Pack("execute", order, swapInputs, discountSwapInputs, executorData)
}

// PackFill is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2b137442.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (executor *Executor) PackFill(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) []byte {
	enc, err := executor.abi.Pack("fill", order, protocolSignature, swapInputs, discountSwapInputs, executorData)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFill is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2b137442.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (executor *Executor) TryPackFill(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) ([]byte, error) {
	return executor.abi.Pack("fill", order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// PackFill0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x33891b08.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, bytes executorData) returns()
func (executor *Executor) PackFill0(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, executorData []byte) []byte {
	enc, err := executor.abi.Pack("fill0", order, protocolSignature, swapInputs, executorData)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFill0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x33891b08.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, bytes executorData) returns()
func (executor *Executor) TryPackFill0(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, executorData []byte) ([]byte, error) {
	return executor.abi.Pack("fill0", order, protocolSignature, swapInputs, executorData)
}

// PackFill1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc1c2b99f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256)) swapInput, bytes executorData) returns()
func (executor *Executor) PackFill1(order IReactorOrder, protocolSignature []byte, swapInput IReactorSwapInput, executorData []byte) []byte {
	enc, err := executor.abi.Pack("fill1", order, protocolSignature, swapInput, executorData)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFill1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc1c2b99f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256)) swapInput, bytes executorData) returns()
func (executor *Executor) TryPackFill1(order IReactorOrder, protocolSignature []byte, swapInput IReactorSwapInput, executorData []byte) ([]byte, error) {
	return executor.abi.Pack("fill1", order, protocolSignature, swapInput, executorData)
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address)
func (executor *Executor) PackOwner() []byte {
	enc, err := executor.abi.Pack("owner")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function owner() view returns(address)
func (executor *Executor) TryPackOwner() ([]byte, error) {
	return executor.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (executor *Executor) UnpackOwner(data []byte) (common.Address, error) {
	out, err := executor.abi.Unpack("owner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function renounceOwnership() returns()
func (executor *Executor) PackRenounceOwnership() []byte {
	enc, err := executor.abi.Pack("renounceOwnership")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function renounceOwnership() returns()
func (executor *Executor) TryPackRenounceOwnership() ([]byte, error) {
	return executor.abi.Pack("renounceOwnership")
}

// PackSetCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x43ded848.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (executor *Executor) PackSetCallers(newCallers []common.Address) []byte {
	enc, err := executor.abi.Pack("setCallers", newCallers)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x43ded848.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (executor *Executor) TryPackSetCallers(newCallers []common.Address) ([]byte, error) {
	return executor.abi.Pack("setCallers", newCallers)
}

// PackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (executor *Executor) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := executor.abi.Pack("transferOwnership", newOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (executor *Executor) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return executor.abi.Pack("transferOwnership", newOwner)
}

// ExecutorOwnershipTransferred represents a OwnershipTransferred event raised by the Executor contract.
type ExecutorOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const ExecutorOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (ExecutorOwnershipTransferred) ContractEventName() string {
	return ExecutorOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (executor *Executor) UnpackOwnershipTransferredEvent(log *types.Log) (*ExecutorOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if log.Topics[0] != executor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ExecutorOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := executor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range executor.abi.Events[event].Inputs {
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

// ExecutorSetCallers represents a SetCallers event raised by the Executor contract.
type ExecutorSetCallers struct {
	NewCallers []common.Address
	Raw        *types.Log // Blockchain specific contextual infos
}

const ExecutorSetCallersEventName = "SetCallers"

// ContractEventName returns the user-defined event name.
func (ExecutorSetCallers) ContractEventName() string {
	return ExecutorSetCallersEventName
}

// UnpackSetCallersEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetCallers(address[] newCallers)
func (executor *Executor) UnpackSetCallersEvent(log *types.Log) (*ExecutorSetCallers, error) {
	event := "SetCallers"
	if log.Topics[0] != executor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ExecutorSetCallers)
	if len(log.Data) > 0 {
		if err := executor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range executor.abi.Events[event].Inputs {
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

// UnpackError attempts to decode the provided error data using user-defined
// error definitions.
func (executor *Executor) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], executor.abi.Errors["AddressEmptyCode"].ID.Bytes()[:4]) {
		return executor.UnpackAddressEmptyCodeError(raw[4:])
	}
	if bytes.Equal(raw[:4], executor.abi.Errors["FailedCall"].ID.Bytes()[:4]) {
		return executor.UnpackFailedCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], executor.abi.Errors["InsufficientBalance"].ID.Bytes()[:4]) {
		return executor.UnpackInsufficientBalanceError(raw[4:])
	}
	if bytes.Equal(raw[:4], executor.abi.Errors["NotCaller"].ID.Bytes()[:4]) {
		return executor.UnpackNotCallerError(raw[4:])
	}
	if bytes.Equal(raw[:4], executor.abi.Errors["NotReactor"].ID.Bytes()[:4]) {
		return executor.UnpackNotReactorError(raw[4:])
	}
	if bytes.Equal(raw[:4], executor.abi.Errors["OwnableInvalidOwner"].ID.Bytes()[:4]) {
		return executor.UnpackOwnableInvalidOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], executor.abi.Errors["OwnableUnauthorizedAccount"].ID.Bytes()[:4]) {
		return executor.UnpackOwnableUnauthorizedAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], executor.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return executor.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// ExecutorAddressEmptyCode represents a AddressEmptyCode error raised by the Executor contract.
type ExecutorAddressEmptyCode struct {
	Target common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AddressEmptyCode(address target)
func ExecutorAddressEmptyCodeErrorID() common.Hash {
	return common.HexToHash("0x9996b315c842ff135b8fc4a08ad5df1c344efbc03d2687aecc0678050d2aac89")
}

// UnpackAddressEmptyCodeError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AddressEmptyCode(address target)
func (executor *Executor) UnpackAddressEmptyCodeError(raw []byte) (*ExecutorAddressEmptyCode, error) {
	out := new(ExecutorAddressEmptyCode)
	if err := executor.abi.UnpackIntoInterface(out, "AddressEmptyCode", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorFailedCall represents a FailedCall error raised by the Executor contract.
type ExecutorFailedCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error FailedCall()
func ExecutorFailedCallErrorID() common.Hash {
	return common.HexToHash("0xd6bda27508c0fb6d8a39b4b122878dab26f731a7d4e4abe711dd3731899052a4")
}

// UnpackFailedCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error FailedCall()
func (executor *Executor) UnpackFailedCallError(raw []byte) (*ExecutorFailedCall, error) {
	out := new(ExecutorFailedCall)
	if err := executor.abi.UnpackIntoInterface(out, "FailedCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorInsufficientBalance represents a InsufficientBalance error raised by the Executor contract.
type ExecutorInsufficientBalance struct {
	Balance *big.Int
	Needed  *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientBalance(uint256 balance, uint256 needed)
func ExecutorInsufficientBalanceErrorID() common.Hash {
	return common.HexToHash("0xcf4791818fba6e019216eb4864093b4947f674afada5d305e57d598b641dad1d")
}

// UnpackInsufficientBalanceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientBalance(uint256 balance, uint256 needed)
func (executor *Executor) UnpackInsufficientBalanceError(raw []byte) (*ExecutorInsufficientBalance, error) {
	out := new(ExecutorInsufficientBalance)
	if err := executor.abi.UnpackIntoInterface(out, "InsufficientBalance", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorNotCaller represents a NotCaller error raised by the Executor contract.
type ExecutorNotCaller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotCaller()
func ExecutorNotCallerErrorID() common.Hash {
	return common.HexToHash("0x16c618d80989492b64dbf0ed90935e3959f670b9b9d57385b45d00c0d1cdedf9")
}

// UnpackNotCallerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotCaller()
func (executor *Executor) UnpackNotCallerError(raw []byte) (*ExecutorNotCaller, error) {
	out := new(ExecutorNotCaller)
	if err := executor.abi.UnpackIntoInterface(out, "NotCaller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorNotReactor represents a NotReactor error raised by the Executor contract.
type ExecutorNotReactor struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotReactor()
func ExecutorNotReactorErrorID() common.Hash {
	return common.HexToHash("0x73f7fe5f826382662fca59946d03ef1eeb4c9e934f9b5911fe3582eee63054f6")
}

// UnpackNotReactorError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotReactor()
func (executor *Executor) UnpackNotReactorError(raw []byte) (*ExecutorNotReactor, error) {
	out := new(ExecutorNotReactor)
	if err := executor.abi.UnpackIntoInterface(out, "NotReactor", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorOwnableInvalidOwner represents a OwnableInvalidOwner error raised by the Executor contract.
type ExecutorOwnableInvalidOwner struct {
	Owner common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableInvalidOwner(address owner)
func ExecutorOwnableInvalidOwnerErrorID() common.Hash {
	return common.HexToHash("0x1e4fbdf7f3ef8bcaa855599e3abf48b232380f183f08f6f813d9ffa5bd585188")
}

// UnpackOwnableInvalidOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableInvalidOwner(address owner)
func (executor *Executor) UnpackOwnableInvalidOwnerError(raw []byte) (*ExecutorOwnableInvalidOwner, error) {
	out := new(ExecutorOwnableInvalidOwner)
	if err := executor.abi.UnpackIntoInterface(out, "OwnableInvalidOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorOwnableUnauthorizedAccount represents a OwnableUnauthorizedAccount error raised by the Executor contract.
type ExecutorOwnableUnauthorizedAccount struct {
	Account common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func ExecutorOwnableUnauthorizedAccountErrorID() common.Hash {
	return common.HexToHash("0x118cdaa7a341953d1887a2245fd6665d741c67c8c50581daa59e1d03373fa188")
}

// UnpackOwnableUnauthorizedAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func (executor *Executor) UnpackOwnableUnauthorizedAccountError(raw []byte) (*ExecutorOwnableUnauthorizedAccount, error) {
	out := new(ExecutorOwnableUnauthorizedAccount)
	if err := executor.abi.UnpackIntoInterface(out, "OwnableUnauthorizedAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the Executor contract.
type ExecutorSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func ExecutorSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (executor *Executor) UnpackSafeERC20FailedOperationError(raw []byte) (*ExecutorSafeERC20FailedOperation, error) {
	out := new(ExecutorSafeERC20FailedOperation)
	if err := executor.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}
