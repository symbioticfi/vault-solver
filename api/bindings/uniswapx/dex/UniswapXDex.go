// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package dex

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

// IUniswapXV3QuoterQuoteExactInputSingleParams is an auto generated low-level Go binding around an user-defined struct.
type IUniswapXV3QuoterQuoteExactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	AmountIn          *big.Int
	Fee               *big.Int
	SqrtPriceLimitX96 *big.Int
}

// IUniswapXV3QuoterQuoteExactOutputSingleParams is an auto generated low-level Go binding around an user-defined struct.
type IUniswapXV3QuoterQuoteExactOutputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	Amount            *big.Int
	Fee               *big.Int
	SqrtPriceLimitX96 *big.Int
}

// LiquidLaneUniswapXDexExecutorDexLimits is an auto generated low-level Go binding around an user-defined struct.
type LiquidLaneUniswapXDexExecutorDexLimits struct {
	MaxBridgePayout   *big.Int
	MaxDexInput       *big.Int
	MinBridgeResidual *big.Int
	ValidUntil        *big.Int
}

// UniswapXSignedOrder is an auto generated low-level Go binding around an user-defined struct.
type UniswapXSignedOrder struct {
	Order []byte
	Sig   []byte
}

// UniswapXDexMetaData contains all meta data concerning the UniswapXDex contract.
var UniswapXDexMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"BRIDGE_TOKEN\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"FEE_0\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint24\",\"internalType\":\"uint24\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"FEE_1\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint24\",\"internalType\":\"uint24\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"INPUT_TOKEN\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"OUTPUT_0\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"OUTPUT_1\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"ROUTER\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"TREASURY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"executeWithDex\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structUniswapXSignedOrder\",\"components\":[{\"name\":\"order\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"sig\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"name\":\"fillCall\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneUniswapXExecutor.FillCall\",\"components\":[{\"name\":\"routes\",\"type\":\"tuple[]\",\"internalType\":\"structILiquidLaneUniswapXExecutor.FillRoute[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"discountRoutes\",\"type\":\"tuple[]\",\"internalType\":\"structILiquidLaneUniswapXExecutor.DiscountRoute[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]}]},{\"name\":\"limits\",\"type\":\"tuple\",\"internalType\":\"structLiquidLaneUniswapXDexExecutor.DexLimits\",\"components\":[{\"name\":\"maxBridgePayout\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxDexInput\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minBridgeResidual\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"validUntil\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"reactorAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"factory\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"quoteExactInputSingle\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"internalType\":\"structIUniswapXV3Quoter.QuoteExactInputSingleParams\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenOut\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"fee\",\"type\":\"uint24\",\"internalType\":\"uint24\"},{\"name\":\"sqrtPriceLimitX96\",\"type\":\"uint160\",\"internalType\":\"uint160\"}]}],\"outputs\":[{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"sqrtPriceX96After\",\"type\":\"uint160\",\"internalType\":\"uint160\"},{\"name\":\"initializedTicksCrossed\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"gasEstimate\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"quoteExactOutputSingle\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"internalType\":\"structIUniswapXV3Quoter.QuoteExactOutputSingleParams\",\"components\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenOut\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"fee\",\"type\":\"uint24\",\"internalType\":\"uint24\"},{\"name\":\"sqrtPriceLimitX96\",\"type\":\"uint160\",\"internalType\":\"uint160\"}]}],\"outputs\":[{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"sqrtPriceX96After\",\"type\":\"uint160\",\"internalType\":\"uint160\"},{\"name\":\"initializedTicksCrossed\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"gasEstimate\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getPool\",\"inputs\":[{\"name\":\"tokenA\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenB\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"fee\",\"type\":\"uint24\",\"internalType\":\"uint24\"}],\"outputs\":[{\"name\":\"pool\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"feeController\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"}]",
	ID:  "UniswapXDex",
}

// UniswapXDex is an auto generated Go binding around an Ethereum contract.
type UniswapXDex struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *UniswapXDex) GetABI() abi.ABI {
	return c.abi
}

// NewUniswapXDex creates a new instance of UniswapXDex.
func NewUniswapXDex() *UniswapXDex {
	parsed, err := UniswapXDexMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &UniswapXDex{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *UniswapXDex) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackBRIDGETOKEN is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0d1ec495.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function BRIDGE_TOKEN() view returns(address)
func (uniswapXDex *UniswapXDex) PackBRIDGETOKEN() []byte {
	enc, err := uniswapXDex.abi.Pack("BRIDGE_TOKEN")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBRIDGETOKEN is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0d1ec495.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function BRIDGE_TOKEN() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackBRIDGETOKEN() ([]byte, error) {
	return uniswapXDex.abi.Pack("BRIDGE_TOKEN")
}

// UnpackBRIDGETOKEN is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0d1ec495.
//
// Solidity: function BRIDGE_TOKEN() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackBRIDGETOKEN(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("BRIDGE_TOKEN", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackFEE0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd0487e9c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function FEE_0() view returns(uint24)
func (uniswapXDex *UniswapXDex) PackFEE0() []byte {
	enc, err := uniswapXDex.abi.Pack("FEE_0")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFEE0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd0487e9c.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function FEE_0() view returns(uint24)
func (uniswapXDex *UniswapXDex) TryPackFEE0() ([]byte, error) {
	return uniswapXDex.abi.Pack("FEE_0")
}

// UnpackFEE0 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd0487e9c.
//
// Solidity: function FEE_0() view returns(uint24)
func (uniswapXDex *UniswapXDex) UnpackFEE0(data []byte) (*big.Int, error) {
	out, err := uniswapXDex.abi.Unpack("FEE_0", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackFEE1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5f056b49.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function FEE_1() view returns(uint24)
func (uniswapXDex *UniswapXDex) PackFEE1() []byte {
	enc, err := uniswapXDex.abi.Pack("FEE_1")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFEE1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5f056b49.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function FEE_1() view returns(uint24)
func (uniswapXDex *UniswapXDex) TryPackFEE1() ([]byte, error) {
	return uniswapXDex.abi.Pack("FEE_1")
}

// UnpackFEE1 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5f056b49.
//
// Solidity: function FEE_1() view returns(uint24)
func (uniswapXDex *UniswapXDex) UnpackFEE1(data []byte) (*big.Int, error) {
	out, err := uniswapXDex.abi.Unpack("FEE_1", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackINPUTTOKEN is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf3835afb.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function INPUT_TOKEN() view returns(address)
func (uniswapXDex *UniswapXDex) PackINPUTTOKEN() []byte {
	enc, err := uniswapXDex.abi.Pack("INPUT_TOKEN")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackINPUTTOKEN is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf3835afb.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function INPUT_TOKEN() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackINPUTTOKEN() ([]byte, error) {
	return uniswapXDex.abi.Pack("INPUT_TOKEN")
}

// UnpackINPUTTOKEN is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xf3835afb.
//
// Solidity: function INPUT_TOKEN() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackINPUTTOKEN(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("INPUT_TOKEN", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackOUTPUT0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4fa51a69.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function OUTPUT_0() view returns(address)
func (uniswapXDex *UniswapXDex) PackOUTPUT0() []byte {
	enc, err := uniswapXDex.abi.Pack("OUTPUT_0")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOUTPUT0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4fa51a69.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function OUTPUT_0() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackOUTPUT0() ([]byte, error) {
	return uniswapXDex.abi.Pack("OUTPUT_0")
}

// UnpackOUTPUT0 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x4fa51a69.
//
// Solidity: function OUTPUT_0() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackOUTPUT0(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("OUTPUT_0", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackOUTPUT1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe077d86a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function OUTPUT_1() view returns(address)
func (uniswapXDex *UniswapXDex) PackOUTPUT1() []byte {
	enc, err := uniswapXDex.abi.Pack("OUTPUT_1")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOUTPUT1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe077d86a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function OUTPUT_1() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackOUTPUT1() ([]byte, error) {
	return uniswapXDex.abi.Pack("OUTPUT_1")
}

// UnpackOUTPUT1 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xe077d86a.
//
// Solidity: function OUTPUT_1() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackOUTPUT1(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("OUTPUT_1", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackROUTER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x32fe7b26.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function ROUTER() view returns(address)
func (uniswapXDex *UniswapXDex) PackROUTER() []byte {
	enc, err := uniswapXDex.abi.Pack("ROUTER")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackROUTER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x32fe7b26.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function ROUTER() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackROUTER() ([]byte, error) {
	return uniswapXDex.abi.Pack("ROUTER")
}

// UnpackROUTER is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x32fe7b26.
//
// Solidity: function ROUTER() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackROUTER(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("ROUTER", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackTREASURY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2d2c5565.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function TREASURY() view returns(address)
func (uniswapXDex *UniswapXDex) PackTREASURY() []byte {
	enc, err := uniswapXDex.abi.Pack("TREASURY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTREASURY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2d2c5565.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function TREASURY() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackTREASURY() ([]byte, error) {
	return uniswapXDex.abi.Pack("TREASURY")
}

// UnpackTREASURY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2d2c5565.
//
// Solidity: function TREASURY() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackTREASURY(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("TREASURY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackExecuteWithDex is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa0cb0d2f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function executeWithDex((bytes,bytes) order, ((address,uint256,uint256)[],(address,uint256,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes)[]) fillCall, (uint256,uint256,uint256,uint256) limits) returns()
func (uniswapXDex *UniswapXDex) PackExecuteWithDex(order UniswapXSignedOrder, fillCall ILiquidLaneUniswapXExecutorFillCall, limits LiquidLaneUniswapXDexExecutorDexLimits) []byte {
	enc, err := uniswapXDex.abi.Pack("executeWithDex", order, fillCall, limits)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackExecuteWithDex is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa0cb0d2f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function executeWithDex((bytes,bytes) order, ((address,uint256,uint256)[],(address,uint256,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes)[]) fillCall, (uint256,uint256,uint256,uint256) limits) returns()
func (uniswapXDex *UniswapXDex) TryPackExecuteWithDex(order UniswapXSignedOrder, fillCall ILiquidLaneUniswapXExecutorFillCall, limits LiquidLaneUniswapXDexExecutorDexLimits) ([]byte, error) {
	return uniswapXDex.abi.Pack("executeWithDex", order, fillCall, limits)
}

// PackFactory is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc45a0155.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function factory() view returns(address)
func (uniswapXDex *UniswapXDex) PackFactory() []byte {
	enc, err := uniswapXDex.abi.Pack("factory")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFactory is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc45a0155.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function factory() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackFactory() ([]byte, error) {
	return uniswapXDex.abi.Pack("factory")
}

// UnpackFactory is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc45a0155.
//
// Solidity: function factory() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackFactory(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("factory", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackFeeController is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6999b377.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function feeController() view returns(address)
func (uniswapXDex *UniswapXDex) PackFeeController() []byte {
	enc, err := uniswapXDex.abi.Pack("feeController")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFeeController is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6999b377.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function feeController() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackFeeController() ([]byte, error) {
	return uniswapXDex.abi.Pack("feeController")
}

// UnpackFeeController is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6999b377.
//
// Solidity: function feeController() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackFeeController(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("feeController", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackGetPool is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1698ee82.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getPool(address tokenA, address tokenB, uint24 fee) view returns(address pool)
func (uniswapXDex *UniswapXDex) PackGetPool(tokenA common.Address, tokenB common.Address, fee *big.Int) []byte {
	enc, err := uniswapXDex.abi.Pack("getPool", tokenA, tokenB, fee)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetPool is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1698ee82.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getPool(address tokenA, address tokenB, uint24 fee) view returns(address pool)
func (uniswapXDex *UniswapXDex) TryPackGetPool(tokenA common.Address, tokenB common.Address, fee *big.Int) ([]byte, error) {
	return uniswapXDex.abi.Pack("getPool", tokenA, tokenB, fee)
}

// UnpackGetPool is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1698ee82.
//
// Solidity: function getPool(address tokenA, address tokenB, uint24 fee) view returns(address pool)
func (uniswapXDex *UniswapXDex) UnpackGetPool(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("getPool", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackQuoteExactInputSingle is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6a5026a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function quoteExactInputSingle((address,address,uint256,uint24,uint160) params) returns(uint256 amountOut, uint160 sqrtPriceX96After, uint32 initializedTicksCrossed, uint256 gasEstimate)
func (uniswapXDex *UniswapXDex) PackQuoteExactInputSingle(params IUniswapXV3QuoterQuoteExactInputSingleParams) []byte {
	enc, err := uniswapXDex.abi.Pack("quoteExactInputSingle", params)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackQuoteExactInputSingle is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6a5026a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function quoteExactInputSingle((address,address,uint256,uint24,uint160) params) returns(uint256 amountOut, uint160 sqrtPriceX96After, uint32 initializedTicksCrossed, uint256 gasEstimate)
func (uniswapXDex *UniswapXDex) TryPackQuoteExactInputSingle(params IUniswapXV3QuoterQuoteExactInputSingleParams) ([]byte, error) {
	return uniswapXDex.abi.Pack("quoteExactInputSingle", params)
}

// QuoteExactInputSingleOutput serves as a container for the return parameters of contract
// method QuoteExactInputSingle.
type QuoteExactInputSingleOutput struct {
	AmountOut               *big.Int
	SqrtPriceX96After       *big.Int
	InitializedTicksCrossed uint32
	GasEstimate             *big.Int
}

// UnpackQuoteExactInputSingle is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc6a5026a.
//
// Solidity: function quoteExactInputSingle((address,address,uint256,uint24,uint160) params) returns(uint256 amountOut, uint160 sqrtPriceX96After, uint32 initializedTicksCrossed, uint256 gasEstimate)
func (uniswapXDex *UniswapXDex) UnpackQuoteExactInputSingle(data []byte) (QuoteExactInputSingleOutput, error) {
	out, err := uniswapXDex.abi.Unpack("quoteExactInputSingle", data)
	outstruct := new(QuoteExactInputSingleOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.AmountOut = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.SqrtPriceX96After = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.InitializedTicksCrossed = *abi.ConvertType(out[2], new(uint32)).(*uint32)
	outstruct.GasEstimate = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackQuoteExactOutputSingle is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbd21704a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function quoteExactOutputSingle((address,address,uint256,uint24,uint160) params) returns(uint256 amountIn, uint160 sqrtPriceX96After, uint32 initializedTicksCrossed, uint256 gasEstimate)
func (uniswapXDex *UniswapXDex) PackQuoteExactOutputSingle(params IUniswapXV3QuoterQuoteExactOutputSingleParams) []byte {
	enc, err := uniswapXDex.abi.Pack("quoteExactOutputSingle", params)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackQuoteExactOutputSingle is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbd21704a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function quoteExactOutputSingle((address,address,uint256,uint24,uint160) params) returns(uint256 amountIn, uint160 sqrtPriceX96After, uint32 initializedTicksCrossed, uint256 gasEstimate)
func (uniswapXDex *UniswapXDex) TryPackQuoteExactOutputSingle(params IUniswapXV3QuoterQuoteExactOutputSingleParams) ([]byte, error) {
	return uniswapXDex.abi.Pack("quoteExactOutputSingle", params)
}

// QuoteExactOutputSingleOutput serves as a container for the return parameters of contract
// method QuoteExactOutputSingle.
type QuoteExactOutputSingleOutput struct {
	AmountIn                *big.Int
	SqrtPriceX96After       *big.Int
	InitializedTicksCrossed uint32
	GasEstimate             *big.Int
}

// UnpackQuoteExactOutputSingle is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xbd21704a.
//
// Solidity: function quoteExactOutputSingle((address,address,uint256,uint24,uint160) params) returns(uint256 amountIn, uint160 sqrtPriceX96After, uint32 initializedTicksCrossed, uint256 gasEstimate)
func (uniswapXDex *UniswapXDex) UnpackQuoteExactOutputSingle(data []byte) (QuoteExactOutputSingleOutput, error) {
	out, err := uniswapXDex.abi.Unpack("quoteExactOutputSingle", data)
	outstruct := new(QuoteExactOutputSingleOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.AmountIn = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.SqrtPriceX96After = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.InitializedTicksCrossed = *abi.ConvertType(out[2], new(uint32)).(*uint32)
	outstruct.GasEstimate = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackReactorAddress is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd768a111.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function reactorAddress() view returns(address)
func (uniswapXDex *UniswapXDex) PackReactorAddress() []byte {
	enc, err := uniswapXDex.abi.Pack("reactorAddress")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackReactorAddress is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd768a111.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function reactorAddress() view returns(address)
func (uniswapXDex *UniswapXDex) TryPackReactorAddress() ([]byte, error) {
	return uniswapXDex.abi.Pack("reactorAddress")
}

// UnpackReactorAddress is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd768a111.
//
// Solidity: function reactorAddress() view returns(address)
func (uniswapXDex *UniswapXDex) UnpackReactorAddress(data []byte) (common.Address, error) {
	out, err := uniswapXDex.abi.Unpack("reactorAddress", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}
