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

// ILiquidLaneUniswapXExecutorDiscountRoute is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneUniswapXExecutorDiscountRoute struct {
	Adapter           common.Address
	AmountIn          *big.Int
	DiscountSwap      ILiquidLaneAdapterDiscountSwap
	ProtocolSignature []byte
}

// ILiquidLaneUniswapXExecutorFillCall is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneUniswapXExecutorFillCall struct {
	Routes         []ILiquidLaneUniswapXExecutorFillRoute
	DiscountRoutes []ILiquidLaneUniswapXExecutorDiscountRoute
}

// ILiquidLaneUniswapXExecutorFillRoute is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneUniswapXExecutorFillRoute struct {
	Adapter   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

// UniswapXInputToken is an auto generated low-level Go binding around an user-defined struct.
type UniswapXInputToken struct {
	Token     common.Address
	Amount    *big.Int
	MaxAmount *big.Int
}

// UniswapXOrderInfo is an auto generated low-level Go binding around an user-defined struct.
type UniswapXOrderInfo struct {
	Reactor                      common.Address
	Swapper                      common.Address
	Nonce                        *big.Int
	Deadline                     *big.Int
	AdditionalValidationContract common.Address
	AdditionalValidationData     []byte
}

// UniswapXOutputToken is an auto generated low-level Go binding around an user-defined struct.
type UniswapXOutputToken struct {
	Token     common.Address
	Amount    *big.Int
	Recipient common.Address
}

// UniswapXResolvedOrder is an auto generated low-level Go binding around an user-defined struct.
type UniswapXResolvedOrder struct {
	Info    UniswapXOrderInfo
	Input   UniswapXInputToken
	Outputs []UniswapXOutputToken
	Sig     []byte
	Hash    [32]byte
}

// UniswapXSignedOrder is an auto generated low-level Go binding around an user-defined struct.
type UniswapXSignedOrder struct {
	Order []byte
	Sig   []byte
}

// LiquidLaneUniswapXExecutorMetaData contains all meta data concerning the LiquidLaneUniswapXExecutor contract.
var LiquidLaneUniswapXExecutorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"reactor\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"receive\",\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"callers\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"execute\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structUniswapXSignedOrder\",\"components\":[{\"name\":\"order\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"sig\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"name\":\"fillCall\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneUniswapXExecutor.FillCall\",\"components\":[{\"name\":\"routes\",\"type\":\"tuple[]\",\"internalType\":\"structILiquidLaneUniswapXExecutor.FillRoute[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"discountRoutes\",\"type\":\"tuple[]\",\"internalType\":\"structILiquidLaneUniswapXExecutor.DiscountRoute[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"reactorCallback\",\"inputs\":[{\"name\":\"resolvedOrders\",\"type\":\"tuple[]\",\"internalType\":\"structUniswapXResolvedOrder[]\",\"components\":[{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structUniswapXOrderInfo\",\"components\":[{\"name\":\"reactor\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"additionalValidationContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"additionalValidationData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"name\":\"input\",\"type\":\"tuple\",\"internalType\":\"structUniswapXInputToken\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structUniswapXOutputToken[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"sig\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"hash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"name\":\"callbackData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"indexed\":false,\"internalType\":\"address[]\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"FailedCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientBalance\",\"inputs\":[{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotReactor\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
	ID:  "LiquidLaneUniswapXExecutor",
}

// LiquidLaneUniswapXExecutor is an auto generated Go binding around an Ethereum contract.
type LiquidLaneUniswapXExecutor struct {
	abi abi.ABI
}

// NewLiquidLaneUniswapXExecutor creates a new instance of LiquidLaneUniswapXExecutor.
func NewLiquidLaneUniswapXExecutor() *LiquidLaneUniswapXExecutor {
	parsed, err := LiquidLaneUniswapXExecutorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &LiquidLaneUniswapXExecutor{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *LiquidLaneUniswapXExecutor) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address reactor) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackConstructor(reactor common.Address) []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("", reactor)
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
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackCallers(arg0 *big.Int) []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("callers", arg0)
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
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackCallers(arg0 *big.Int) ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("callers", arg0)
}

// UnpackCallers is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xaa03fa3d.
//
// Solidity: function callers(uint256 ) view returns(address)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackCallers(data []byte) (common.Address, error) {
	out, err := liquidLaneUniswapXExecutor.abi.Unpack("callers", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackExecute is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf21abd0f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function execute((bytes,bytes) order, ((address,uint256,uint256)[],(address,uint256,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes)[]) fillCall) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackExecute(order UniswapXSignedOrder, fillCall ILiquidLaneUniswapXExecutorFillCall) []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("execute", order, fillCall)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackExecute is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf21abd0f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function execute((bytes,bytes) order, ((address,uint256,uint256)[],(address,uint256,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes)[]) fillCall) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackExecute(order UniswapXSignedOrder, fillCall ILiquidLaneUniswapXExecutorFillCall) ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("execute", order, fillCall)
}

// PackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x946d9204.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function initialize(address owner, address[] initCallers) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackInitialize(owner common.Address, initCallers []common.Address) []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("initialize", owner, initCallers)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x946d9204.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function initialize(address owner, address[] initCallers) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackInitialize(owner common.Address, initCallers []common.Address) ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("initialize", owner, initCallers)
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackOwner() []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("owner")
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
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackOwner() ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackOwner(data []byte) (common.Address, error) {
	out, err := liquidLaneUniswapXExecutor.abi.Unpack("owner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackReactorCallback is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x585da628.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function reactorCallback(((address,address,uint256,uint256,address,bytes),(address,uint256,uint256),(address,uint256,address)[],bytes,bytes32)[] resolvedOrders, bytes callbackData) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackReactorCallback(resolvedOrders []UniswapXResolvedOrder, callbackData []byte) []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("reactorCallback", resolvedOrders, callbackData)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackReactorCallback is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x585da628.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function reactorCallback(((address,address,uint256,uint256,address,bytes),(address,uint256,uint256),(address,uint256,address)[],bytes,bytes32)[] resolvedOrders, bytes callbackData) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackReactorCallback(resolvedOrders []UniswapXResolvedOrder, callbackData []byte) ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("reactorCallback", resolvedOrders, callbackData)
}

// PackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function renounceOwnership() returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackRenounceOwnership() []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("renounceOwnership")
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
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackRenounceOwnership() ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("renounceOwnership")
}

// PackSetCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x43ded848.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackSetCallers(newCallers []common.Address) []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("setCallers", newCallers)
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
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackSetCallers(newCallers []common.Address) ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("setCallers", newCallers)
}

// PackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := liquidLaneUniswapXExecutor.abi.Pack("transferOwnership", newOwner)
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
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return liquidLaneUniswapXExecutor.abi.Pack("transferOwnership", newOwner)
}

// LiquidLaneUniswapXExecutorInitialized represents a Initialized event raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorInitialized struct {
	Version uint64
	Raw     *types.Log // Blockchain specific contextual infos
}

const LiquidLaneUniswapXExecutorInitializedEventName = "Initialized"

// ContractEventName returns the user-defined event name.
func (LiquidLaneUniswapXExecutorInitialized) ContractEventName() string {
	return LiquidLaneUniswapXExecutorInitializedEventName
}

// UnpackInitializedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialized(uint64 version)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackInitializedEvent(log *types.Log) (*LiquidLaneUniswapXExecutorInitialized, error) {
	event := "Initialized"
	if log.Topics[0] != liquidLaneUniswapXExecutor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneUniswapXExecutorInitialized)
	if len(log.Data) > 0 {
		if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneUniswapXExecutor.abi.Events[event].Inputs {
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

// LiquidLaneUniswapXExecutorOwnershipTransferred represents a OwnershipTransferred event raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneUniswapXExecutorOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (LiquidLaneUniswapXExecutorOwnershipTransferred) ContractEventName() string {
	return LiquidLaneUniswapXExecutorOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackOwnershipTransferredEvent(log *types.Log) (*LiquidLaneUniswapXExecutorOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if log.Topics[0] != liquidLaneUniswapXExecutor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneUniswapXExecutorOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneUniswapXExecutor.abi.Events[event].Inputs {
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

// LiquidLaneUniswapXExecutorSetCallers represents a SetCallers event raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorSetCallers struct {
	NewCallers []common.Address
	Raw        *types.Log // Blockchain specific contextual infos
}

const LiquidLaneUniswapXExecutorSetCallersEventName = "SetCallers"

// ContractEventName returns the user-defined event name.
func (LiquidLaneUniswapXExecutorSetCallers) ContractEventName() string {
	return LiquidLaneUniswapXExecutorSetCallersEventName
}

// UnpackSetCallersEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetCallers(address[] newCallers)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackSetCallersEvent(log *types.Log) (*LiquidLaneUniswapXExecutorSetCallers, error) {
	event := "SetCallers"
	if log.Topics[0] != liquidLaneUniswapXExecutor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneUniswapXExecutorSetCallers)
	if len(log.Data) > 0 {
		if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneUniswapXExecutor.abi.Events[event].Inputs {
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
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["FailedCall"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackFailedCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["InsufficientBalance"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackInsufficientBalanceError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["InvalidInitialization"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackInvalidInitializationError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["NotCaller"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackNotCallerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["NotInitializing"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackNotInitializingError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["NotReactor"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackNotReactorError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["OwnableInvalidOwner"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackOwnableInvalidOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["OwnableUnauthorizedAccount"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackOwnableUnauthorizedAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneUniswapXExecutor.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return liquidLaneUniswapXExecutor.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// LiquidLaneUniswapXExecutorFailedCall represents a FailedCall error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorFailedCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error FailedCall()
func LiquidLaneUniswapXExecutorFailedCallErrorID() common.Hash {
	return common.HexToHash("0xd6bda27508c0fb6d8a39b4b122878dab26f731a7d4e4abe711dd3731899052a4")
}

// UnpackFailedCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error FailedCall()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackFailedCallError(raw []byte) (*LiquidLaneUniswapXExecutorFailedCall, error) {
	out := new(LiquidLaneUniswapXExecutorFailedCall)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "FailedCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorInsufficientBalance represents a InsufficientBalance error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorInsufficientBalance struct {
	Balance *big.Int
	Needed  *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientBalance(uint256 balance, uint256 needed)
func LiquidLaneUniswapXExecutorInsufficientBalanceErrorID() common.Hash {
	return common.HexToHash("0xcf4791818fba6e019216eb4864093b4947f674afada5d305e57d598b641dad1d")
}

// UnpackInsufficientBalanceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientBalance(uint256 balance, uint256 needed)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackInsufficientBalanceError(raw []byte) (*LiquidLaneUniswapXExecutorInsufficientBalance, error) {
	out := new(LiquidLaneUniswapXExecutorInsufficientBalance)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "InsufficientBalance", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorInvalidInitialization represents a InvalidInitialization error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorInvalidInitialization struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidInitialization()
func LiquidLaneUniswapXExecutorInvalidInitializationErrorID() common.Hash {
	return common.HexToHash("0xf92ee8a957075833165f68c320933b1a1294aafc84ee6e0dd3fb178008f9aaf5")
}

// UnpackInvalidInitializationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidInitialization()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackInvalidInitializationError(raw []byte) (*LiquidLaneUniswapXExecutorInvalidInitialization, error) {
	out := new(LiquidLaneUniswapXExecutorInvalidInitialization)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "InvalidInitialization", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorNotCaller represents a NotCaller error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorNotCaller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotCaller()
func LiquidLaneUniswapXExecutorNotCallerErrorID() common.Hash {
	return common.HexToHash("0x16c618d80989492b64dbf0ed90935e3959f670b9b9d57385b45d00c0d1cdedf9")
}

// UnpackNotCallerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotCaller()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackNotCallerError(raw []byte) (*LiquidLaneUniswapXExecutorNotCaller, error) {
	out := new(LiquidLaneUniswapXExecutorNotCaller)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "NotCaller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorNotInitializing represents a NotInitializing error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorNotInitializing struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitializing()
func LiquidLaneUniswapXExecutorNotInitializingErrorID() common.Hash {
	return common.HexToHash("0xd7e6bcf8597daa127dc9f0048d2f08d5ef140a2cb659feabd700beff1f7a8302")
}

// UnpackNotInitializingError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitializing()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackNotInitializingError(raw []byte) (*LiquidLaneUniswapXExecutorNotInitializing, error) {
	out := new(LiquidLaneUniswapXExecutorNotInitializing)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "NotInitializing", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorNotReactor represents a NotReactor error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorNotReactor struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotReactor()
func LiquidLaneUniswapXExecutorNotReactorErrorID() common.Hash {
	return common.HexToHash("0x73f7fe5f826382662fca59946d03ef1eeb4c9e934f9b5911fe3582eee63054f6")
}

// UnpackNotReactorError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotReactor()
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackNotReactorError(raw []byte) (*LiquidLaneUniswapXExecutorNotReactor, error) {
	out := new(LiquidLaneUniswapXExecutorNotReactor)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "NotReactor", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorOwnableInvalidOwner represents a OwnableInvalidOwner error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorOwnableInvalidOwner struct {
	Owner common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableInvalidOwner(address owner)
func LiquidLaneUniswapXExecutorOwnableInvalidOwnerErrorID() common.Hash {
	return common.HexToHash("0x1e4fbdf7f3ef8bcaa855599e3abf48b232380f183f08f6f813d9ffa5bd585188")
}

// UnpackOwnableInvalidOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableInvalidOwner(address owner)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackOwnableInvalidOwnerError(raw []byte) (*LiquidLaneUniswapXExecutorOwnableInvalidOwner, error) {
	out := new(LiquidLaneUniswapXExecutorOwnableInvalidOwner)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "OwnableInvalidOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorOwnableUnauthorizedAccount represents a OwnableUnauthorizedAccount error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorOwnableUnauthorizedAccount struct {
	Account common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func LiquidLaneUniswapXExecutorOwnableUnauthorizedAccountErrorID() common.Hash {
	return common.HexToHash("0x118cdaa7a341953d1887a2245fd6665d741c67c8c50581daa59e1d03373fa188")
}

// UnpackOwnableUnauthorizedAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackOwnableUnauthorizedAccountError(raw []byte) (*LiquidLaneUniswapXExecutorOwnableUnauthorizedAccount, error) {
	out := new(LiquidLaneUniswapXExecutorOwnableUnauthorizedAccount)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "OwnableUnauthorizedAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneUniswapXExecutorSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the LiquidLaneUniswapXExecutor contract.
type LiquidLaneUniswapXExecutorSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func LiquidLaneUniswapXExecutorSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (liquidLaneUniswapXExecutor *LiquidLaneUniswapXExecutor) UnpackSafeERC20FailedOperationError(raw []byte) (*LiquidLaneUniswapXExecutorSafeERC20FailedOperation, error) {
	out := new(LiquidLaneUniswapXExecutorSafeERC20FailedOperation)
	if err := liquidLaneUniswapXExecutor.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}
