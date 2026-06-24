// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package reactor

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

// ReactorMetaData contains all meta data concerning the Reactor contract.
var ReactorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"liquidLaneAdapterFactory\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"receive\",\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"LIQUID_LANE_ADAPTER_FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"discountSwapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.DiscountSwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.SwapInput[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapInput\",\"type\":\"tuple\",\"internalType\":\"structIReactor.SwapInput\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]},{\"name\":\"executorData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"invalidateNonce\",\"inputs\":[{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isUsedNonce\",\"inputs\":[{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"used\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Fill\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structIReactor.Order\",\"components\":[{\"name\":\"request\",\"type\":\"tuple\",\"internalType\":\"structIReactor.Request\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structIReactor.Output[]\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"name\":\"swapperSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"swapper\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"InvalidateNonce\",\"inputs\":[{\"name\":\"swapper\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ExpiredRequest\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"FailedCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientBalance\",\"inputs\":[{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidAdapter\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAmountIn\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidFiller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOutput\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidProtocolSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidShortString\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTokenIn\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NonceUsed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"StringTooLong\",\"inputs\":[{\"name\":\"str\",\"type\":\"string\",\"internalType\":\"string\"}]}]",
	ID:  "Reactor",
}

// Reactor is an auto generated Go binding around an Ethereum contract.
type Reactor struct {
	abi abi.ABI
}

// NewReactor creates a new instance of Reactor.
func NewReactor() *Reactor {
	parsed, err := ReactorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &Reactor{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *Reactor) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address liquidLaneAdapterFactory) returns()
func (reactor *Reactor) PackConstructor(liquidLaneAdapterFactory common.Address) []byte {
	enc, err := reactor.abi.Pack("", liquidLaneAdapterFactory)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackLIQUIDLANEADAPTERFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2f1b87ff.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function LIQUID_LANE_ADAPTER_FACTORY() view returns(address)
func (reactor *Reactor) PackLIQUIDLANEADAPTERFACTORY() []byte {
	enc, err := reactor.abi.Pack("LIQUID_LANE_ADAPTER_FACTORY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLIQUIDLANEADAPTERFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2f1b87ff.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function LIQUID_LANE_ADAPTER_FACTORY() view returns(address)
func (reactor *Reactor) TryPackLIQUIDLANEADAPTERFACTORY() ([]byte, error) {
	return reactor.abi.Pack("LIQUID_LANE_ADAPTER_FACTORY")
}

// UnpackLIQUIDLANEADAPTERFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2f1b87ff.
//
// Solidity: function LIQUID_LANE_ADAPTER_FACTORY() view returns(address)
func (reactor *Reactor) UnpackLIQUIDLANEADAPTERFACTORY(data []byte) (common.Address, error) {
	out, err := reactor.abi.Unpack("LIQUID_LANE_ADAPTER_FACTORY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (reactor *Reactor) PackEip712Domain() []byte {
	enc, err := reactor.abi.Pack("eip712Domain")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (reactor *Reactor) TryPackEip712Domain() ([]byte, error) {
	return reactor.abi.Pack("eip712Domain")
}

// Eip712DomainOutput serves as a container for the return parameters of contract
// method Eip712Domain.
type Eip712DomainOutput struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}

// UnpackEip712Domain is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (reactor *Reactor) UnpackEip712Domain(data []byte) (Eip712DomainOutput, error) {
	out, err := reactor.abi.Unpack("eip712Domain", data)
	outstruct := new(Eip712DomainOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)
	return *outstruct, nil
}

// PackFill is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2b137442.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, (address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[] discountSwapInputs, bytes executorData) returns()
func (reactor *Reactor) PackFill(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) []byte {
	enc, err := reactor.abi.Pack("fill", order, protocolSignature, swapInputs, discountSwapInputs, executorData)
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
func (reactor *Reactor) TryPackFill(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, discountSwapInputs []IReactorDiscountSwapInput, executorData []byte) ([]byte, error) {
	return reactor.abi.Pack("fill", order, protocolSignature, swapInputs, discountSwapInputs, executorData)
}

// PackFill0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x33891b08.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256))[] swapInputs, bytes executorData) returns()
func (reactor *Reactor) PackFill0(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, executorData []byte) []byte {
	enc, err := reactor.abi.Pack("fill0", order, protocolSignature, swapInputs, executorData)
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
func (reactor *Reactor) TryPackFill0(order IReactorOrder, protocolSignature []byte, swapInputs []IReactorSwapInput, executorData []byte) ([]byte, error) {
	return reactor.abi.Pack("fill0", order, protocolSignature, swapInputs, executorData)
}

// PackFill1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc1c2b99f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order, bytes protocolSignature, (address,(address,address,uint256,uint256)) swapInput, bytes executorData) returns()
func (reactor *Reactor) PackFill1(order IReactorOrder, protocolSignature []byte, swapInput IReactorSwapInput, executorData []byte) []byte {
	enc, err := reactor.abi.Pack("fill1", order, protocolSignature, swapInput, executorData)
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
func (reactor *Reactor) TryPackFill1(order IReactorOrder, protocolSignature []byte, swapInput IReactorSwapInput, executorData []byte) ([]byte, error) {
	return reactor.abi.Pack("fill1", order, protocolSignature, swapInput, executorData)
}

// PackInvalidateNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb70e36f0.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function invalidateNonce(uint256 nonce) returns()
func (reactor *Reactor) PackInvalidateNonce(nonce *big.Int) []byte {
	enc, err := reactor.abi.Pack("invalidateNonce", nonce)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInvalidateNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb70e36f0.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function invalidateNonce(uint256 nonce) returns()
func (reactor *Reactor) TryPackInvalidateNonce(nonce *big.Int) ([]byte, error) {
	return reactor.abi.Pack("invalidateNonce", nonce)
}

// PackIsUsedNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ee60fa7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isUsedNonce(address swapper, uint256 nonce) view returns(bool used)
func (reactor *Reactor) PackIsUsedNonce(swapper common.Address, nonce *big.Int) []byte {
	enc, err := reactor.abi.Pack("isUsedNonce", swapper, nonce)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsUsedNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ee60fa7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isUsedNonce(address swapper, uint256 nonce) view returns(bool used)
func (reactor *Reactor) TryPackIsUsedNonce(swapper common.Address, nonce *big.Int) ([]byte, error) {
	return reactor.abi.Pack("isUsedNonce", swapper, nonce)
}

// UnpackIsUsedNonce is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0ee60fa7.
//
// Solidity: function isUsedNonce(address swapper, uint256 nonce) view returns(bool used)
func (reactor *Reactor) UnpackIsUsedNonce(data []byte) (bool, error) {
	out, err := reactor.abi.Unpack("isUsedNonce", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// ReactorEIP712DomainChanged represents a EIP712DomainChanged event raised by the Reactor contract.
type ReactorEIP712DomainChanged struct {
	Raw *types.Log // Blockchain specific contextual infos
}

const ReactorEIP712DomainChangedEventName = "EIP712DomainChanged"

// ContractEventName returns the user-defined event name.
func (ReactorEIP712DomainChanged) ContractEventName() string {
	return ReactorEIP712DomainChangedEventName
}

// UnpackEIP712DomainChangedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event EIP712DomainChanged()
func (reactor *Reactor) UnpackEIP712DomainChangedEvent(log *types.Log) (*ReactorEIP712DomainChanged, error) {
	event := "EIP712DomainChanged"
	if log.Topics[0] != reactor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ReactorEIP712DomainChanged)
	if len(log.Data) > 0 {
		if err := reactor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range reactor.abi.Events[event].Inputs {
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

// ReactorFill represents a Fill event raised by the Reactor contract.
type ReactorFill struct {
	Order IReactorOrder
	Raw   *types.Log // Blockchain specific contextual infos
}

const ReactorFillEventName = "Fill"

// ContractEventName returns the user-defined event name.
func (ReactorFill) ContractEventName() string {
	return ReactorFillEventName
}

// UnpackFillEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address) order)
func (reactor *Reactor) UnpackFillEvent(log *types.Log) (*ReactorFill, error) {
	event := "Fill"
	if log.Topics[0] != reactor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ReactorFill)
	if len(log.Data) > 0 {
		if err := reactor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range reactor.abi.Events[event].Inputs {
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

// ReactorInvalidateNonce represents a InvalidateNonce event raised by the Reactor contract.
type ReactorInvalidateNonce struct {
	Swapper common.Address
	Nonce   *big.Int
	Raw     *types.Log // Blockchain specific contextual infos
}

const ReactorInvalidateNonceEventName = "InvalidateNonce"

// ContractEventName returns the user-defined event name.
func (ReactorInvalidateNonce) ContractEventName() string {
	return ReactorInvalidateNonceEventName
}

// UnpackInvalidateNonceEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event InvalidateNonce(address indexed swapper, uint256 nonce)
func (reactor *Reactor) UnpackInvalidateNonceEvent(log *types.Log) (*ReactorInvalidateNonce, error) {
	event := "InvalidateNonce"
	if log.Topics[0] != reactor.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ReactorInvalidateNonce)
	if len(log.Data) > 0 {
		if err := reactor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range reactor.abi.Events[event].Inputs {
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
func (reactor *Reactor) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], reactor.abi.Errors["ExpiredRequest"].ID.Bytes()[:4]) {
		return reactor.UnpackExpiredRequestError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["FailedCall"].ID.Bytes()[:4]) {
		return reactor.UnpackFailedCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InsufficientBalance"].ID.Bytes()[:4]) {
		return reactor.UnpackInsufficientBalanceError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InvalidAdapter"].ID.Bytes()[:4]) {
		return reactor.UnpackInvalidAdapterError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InvalidAmountIn"].ID.Bytes()[:4]) {
		return reactor.UnpackInvalidAmountInError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InvalidFiller"].ID.Bytes()[:4]) {
		return reactor.UnpackInvalidFillerError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InvalidOutput"].ID.Bytes()[:4]) {
		return reactor.UnpackInvalidOutputError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InvalidProtocolSignature"].ID.Bytes()[:4]) {
		return reactor.UnpackInvalidProtocolSignatureError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InvalidShortString"].ID.Bytes()[:4]) {
		return reactor.UnpackInvalidShortStringError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["InvalidTokenIn"].ID.Bytes()[:4]) {
		return reactor.UnpackInvalidTokenInError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["NonceUsed"].ID.Bytes()[:4]) {
		return reactor.UnpackNonceUsedError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return reactor.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], reactor.abi.Errors["StringTooLong"].ID.Bytes()[:4]) {
		return reactor.UnpackStringTooLongError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// ReactorExpiredRequest represents a ExpiredRequest error raised by the Reactor contract.
type ReactorExpiredRequest struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ExpiredRequest()
func ReactorExpiredRequestErrorID() common.Hash {
	return common.HexToHash("0xdd9cfdb60cb8705266dc8284ec4136f2d1152221e05b1206f6e260f4b73b6721")
}

// UnpackExpiredRequestError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ExpiredRequest()
func (reactor *Reactor) UnpackExpiredRequestError(raw []byte) (*ReactorExpiredRequest, error) {
	out := new(ReactorExpiredRequest)
	if err := reactor.abi.UnpackIntoInterface(out, "ExpiredRequest", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorFailedCall represents a FailedCall error raised by the Reactor contract.
type ReactorFailedCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error FailedCall()
func ReactorFailedCallErrorID() common.Hash {
	return common.HexToHash("0xd6bda27508c0fb6d8a39b4b122878dab26f731a7d4e4abe711dd3731899052a4")
}

// UnpackFailedCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error FailedCall()
func (reactor *Reactor) UnpackFailedCallError(raw []byte) (*ReactorFailedCall, error) {
	out := new(ReactorFailedCall)
	if err := reactor.abi.UnpackIntoInterface(out, "FailedCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInsufficientBalance represents a InsufficientBalance error raised by the Reactor contract.
type ReactorInsufficientBalance struct {
	Balance *big.Int
	Needed  *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientBalance(uint256 balance, uint256 needed)
func ReactorInsufficientBalanceErrorID() common.Hash {
	return common.HexToHash("0xcf4791818fba6e019216eb4864093b4947f674afada5d305e57d598b641dad1d")
}

// UnpackInsufficientBalanceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientBalance(uint256 balance, uint256 needed)
func (reactor *Reactor) UnpackInsufficientBalanceError(raw []byte) (*ReactorInsufficientBalance, error) {
	out := new(ReactorInsufficientBalance)
	if err := reactor.abi.UnpackIntoInterface(out, "InsufficientBalance", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInvalidAdapter represents a InvalidAdapter error raised by the Reactor contract.
type ReactorInvalidAdapter struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAdapter()
func ReactorInvalidAdapterErrorID() common.Hash {
	return common.HexToHash("0xfbf66df16c4e0cc635846a65caa4e7a61b59d2693cbd9bb842a271cee3fb9ae1")
}

// UnpackInvalidAdapterError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAdapter()
func (reactor *Reactor) UnpackInvalidAdapterError(raw []byte) (*ReactorInvalidAdapter, error) {
	out := new(ReactorInvalidAdapter)
	if err := reactor.abi.UnpackIntoInterface(out, "InvalidAdapter", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInvalidAmountIn represents a InvalidAmountIn error raised by the Reactor contract.
type ReactorInvalidAmountIn struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAmountIn()
func ReactorInvalidAmountInErrorID() common.Hash {
	return common.HexToHash("0xcae33fc2ec7b8b95ca9448fef6e70e7edb40b568942fd281c3dc01b6327b455f")
}

// UnpackInvalidAmountInError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAmountIn()
func (reactor *Reactor) UnpackInvalidAmountInError(raw []byte) (*ReactorInvalidAmountIn, error) {
	out := new(ReactorInvalidAmountIn)
	if err := reactor.abi.UnpackIntoInterface(out, "InvalidAmountIn", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInvalidFiller represents a InvalidFiller error raised by the Reactor contract.
type ReactorInvalidFiller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidFiller()
func ReactorInvalidFillerErrorID() common.Hash {
	return common.HexToHash("0x02f5c732c4ca7b6e083c7794b98dc7ea41e7b5cee2e041570e9629c0e99b44fc")
}

// UnpackInvalidFillerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidFiller()
func (reactor *Reactor) UnpackInvalidFillerError(raw []byte) (*ReactorInvalidFiller, error) {
	out := new(ReactorInvalidFiller)
	if err := reactor.abi.UnpackIntoInterface(out, "InvalidFiller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInvalidOutput represents a InvalidOutput error raised by the Reactor contract.
type ReactorInvalidOutput struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidOutput()
func ReactorInvalidOutputErrorID() common.Hash {
	return common.HexToHash("0x98f73609786fa23b15fe8edbfa60d3c30ec79f636b4d15060098c3ca66510536")
}

// UnpackInvalidOutputError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidOutput()
func (reactor *Reactor) UnpackInvalidOutputError(raw []byte) (*ReactorInvalidOutput, error) {
	out := new(ReactorInvalidOutput)
	if err := reactor.abi.UnpackIntoInterface(out, "InvalidOutput", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInvalidProtocolSignature represents a InvalidProtocolSignature error raised by the Reactor contract.
type ReactorInvalidProtocolSignature struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidProtocolSignature()
func ReactorInvalidProtocolSignatureErrorID() common.Hash {
	return common.HexToHash("0x2e90f0600e9f663ed58eb7c47214769f496f9a68e123e1662184318743452ec0")
}

// UnpackInvalidProtocolSignatureError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidProtocolSignature()
func (reactor *Reactor) UnpackInvalidProtocolSignatureError(raw []byte) (*ReactorInvalidProtocolSignature, error) {
	out := new(ReactorInvalidProtocolSignature)
	if err := reactor.abi.UnpackIntoInterface(out, "InvalidProtocolSignature", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInvalidShortString represents a InvalidShortString error raised by the Reactor contract.
type ReactorInvalidShortString struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidShortString()
func ReactorInvalidShortStringErrorID() common.Hash {
	return common.HexToHash("0xb3512b0c6163e5f0bafab72bb631b9d58cd7a731b082f910338aa21c83d5c274")
}

// UnpackInvalidShortStringError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidShortString()
func (reactor *Reactor) UnpackInvalidShortStringError(raw []byte) (*ReactorInvalidShortString, error) {
	out := new(ReactorInvalidShortString)
	if err := reactor.abi.UnpackIntoInterface(out, "InvalidShortString", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorInvalidTokenIn represents a InvalidTokenIn error raised by the Reactor contract.
type ReactorInvalidTokenIn struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidTokenIn()
func ReactorInvalidTokenInErrorID() common.Hash {
	return common.HexToHash("0xd70f29d2582fa167b7dcef93921128bd80331d86626bc6475dfb97c149c81845")
}

// UnpackInvalidTokenInError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidTokenIn()
func (reactor *Reactor) UnpackInvalidTokenInError(raw []byte) (*ReactorInvalidTokenIn, error) {
	out := new(ReactorInvalidTokenIn)
	if err := reactor.abi.UnpackIntoInterface(out, "InvalidTokenIn", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorNonceUsed represents a NonceUsed error raised by the Reactor contract.
type ReactorNonceUsed struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NonceUsed()
func ReactorNonceUsedErrorID() common.Hash {
	return common.HexToHash("0x1f6d5aef5a4e50674e57b82f3fc08dc6ad8892bdf4aadafb3bc99cc8cf7a4706")
}

// UnpackNonceUsedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NonceUsed()
func (reactor *Reactor) UnpackNonceUsedError(raw []byte) (*ReactorNonceUsed, error) {
	out := new(ReactorNonceUsed)
	if err := reactor.abi.UnpackIntoInterface(out, "NonceUsed", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the Reactor contract.
type ReactorSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func ReactorSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (reactor *Reactor) UnpackSafeERC20FailedOperationError(raw []byte) (*ReactorSafeERC20FailedOperation, error) {
	out := new(ReactorSafeERC20FailedOperation)
	if err := reactor.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ReactorStringTooLong represents a StringTooLong error raised by the Reactor contract.
type ReactorStringTooLong struct {
	Str string
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error StringTooLong(string str)
func ReactorStringTooLongErrorID() common.Hash {
	return common.HexToHash("0x305a27a93f8e33b7392df0a0f91d6fc63847395853c45991eec52dbf24d72381")
}

// UnpackStringTooLongError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error StringTooLong(string str)
func (reactor *Reactor) UnpackStringTooLongError(raw []byte) (*ReactorStringTooLong, error) {
	out := new(ReactorStringTooLong)
	if err := reactor.abi.UnpackIntoInterface(out, "StringTooLong", raw); err != nil {
		return nil, err
	}
	return out, nil
}
