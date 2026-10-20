// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package connector

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

// ILiquidLaneConnectorDiscount is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneConnectorDiscount struct {
	TokenToRedeem common.Address
	Discount      *big.Int
	Signer        common.Address
	Protocol      common.Address
	Nonce         *big.Int
	Deadline      *big.Int
}

// ILiquidLaneConnectorDiscountSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneConnectorDiscountSwap struct {
	Discount         ILiquidLaneConnectorDiscount
	SignerSignature  []byte
	ProtocolDeadline *big.Int
}

// LiquidLaneConnectorMetaData contains all meta data concerning the LiquidLaneConnector contract.
var LiquidLaneConnectorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"account\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAmountOut\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"minDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"outputAsset\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"remainingAssets\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneConnector.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneConnector.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"}]",
	ID:  "LiquidLaneConnector",
}

// LiquidLaneConnector is an auto generated Go binding around an Ethereum contract.
type LiquidLaneConnector struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *LiquidLaneConnector) GetABI() abi.ABI {
	return c.abi
}

// NewLiquidLaneConnector creates a new instance of LiquidLaneConnector.
func NewLiquidLaneConnector() *LiquidLaneConnector {
	parsed, err := LiquidLaneConnectorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &LiquidLaneConnector{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *LiquidLaneConnector) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackAccount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5dab2420.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function account() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackAccount() []byte {
	enc, err := liquidLaneConnector.abi.Pack("account")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAccount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5dab2420.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function account() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackAccount() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("account")
}

// UnpackAccount is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5dab2420.
//
// Solidity: function account() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackAccount(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("account", data)
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
func (liquidLaneConnector *LiquidLaneConnector) PackEip712Domain() []byte {
	enc, err := liquidLaneConnector.abi.Pack("eip712Domain")
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
func (liquidLaneConnector *LiquidLaneConnector) TryPackEip712Domain() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("eip712Domain")
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
func (liquidLaneConnector *LiquidLaneConnector) UnpackEip712Domain(data []byte) (Eip712DomainOutput, error) {
	out, err := liquidLaneConnector.abi.Unpack("eip712Domain", data)
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

// PackGetAmountOut is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca706bcf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) PackGetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("getAmountOut", tokenToRedeem, amountIn)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetAmountOut is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca706bcf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) TryPackGetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("getAmountOut", tokenToRedeem, amountIn)
}

// UnpackGetAmountOut is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xca706bcf.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) UnpackGetAmountOut(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("getAmountOut", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd9104c14.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 discount)
func (liquidLaneConnector *LiquidLaneConnector) PackMinDiscount(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("minDiscount", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd9104c14.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 discount)
func (liquidLaneConnector *LiquidLaneConnector) TryPackMinDiscount(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("minDiscount", tokenToRedeem)
}

// UnpackMinDiscount is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd9104c14.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 discount)
func (liquidLaneConnector *LiquidLaneConnector) UnpackMinDiscount(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("minDiscount", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackOutputAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x69cb2a8b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function outputAsset() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) PackOutputAsset() []byte {
	enc, err := liquidLaneConnector.abi.Pack("outputAsset")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOutputAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x69cb2a8b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function outputAsset() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) TryPackOutputAsset() ([]byte, error) {
	return liquidLaneConnector.abi.Pack("outputAsset")
}

// UnpackOutputAsset is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x69cb2a8b.
//
// Solidity: function outputAsset() view returns(address)
func (liquidLaneConnector *LiquidLaneConnector) UnpackOutputAsset(data []byte) (common.Address, error) {
	out, err := liquidLaneConnector.abi.Unpack("outputAsset", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd91bbc69.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function remainingAssets(address tokenToRedeem) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) PackRemainingAssets(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneConnector.abi.Pack("remainingAssets", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd91bbc69.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function remainingAssets(address tokenToRedeem) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) TryPackRemainingAssets(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("remainingAssets", tokenToRedeem)
}

// UnpackRemainingAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd91bbc69.
//
// Solidity: function remainingAssets(address tokenToRedeem) view returns(uint256)
func (liquidLaneConnector *LiquidLaneConnector) UnpackRemainingAssets(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("remainingAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackSwap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8fa5c671.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneConnector *LiquidLaneConnector) PackSwap(discountSwap ILiquidLaneConnectorDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) []byte {
	enc, err := liquidLaneConnector.abi.Pack("swap", discountSwap, protocolSignature, recipient, amountIn)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSwap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8fa5c671.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneConnector *LiquidLaneConnector) TryPackSwap(discountSwap ILiquidLaneConnectorDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) ([]byte, error) {
	return liquidLaneConnector.abi.Pack("swap", discountSwap, protocolSignature, recipient, amountIn)
}

// UnpackSwap is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8fa5c671.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneConnector *LiquidLaneConnector) UnpackSwap(data []byte) (*big.Int, error) {
	out, err := liquidLaneConnector.abi.Unpack("swap", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
