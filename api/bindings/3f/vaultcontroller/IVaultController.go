// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package vaultcontroller

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

// IVaultControllerMetaData contains all meta data concerning the IVaultController contract.
var IVaultControllerMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"allowance\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"yt\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[{\"name\":\"result\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allowancesOf\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"pt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"yt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"approveBatch\",\"inputs\":[{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"asset\",\"inputs\":[],\"outputs\":[{\"name\":\"assetAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"balanceOf\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"yt\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"balancesOf\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"pt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"yt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"burnAll\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ptShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"canWithdraw\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"convertToAssets\",\"inputs\":[{\"name\":\"ptShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"convertToShares\",\"inputs\":[{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"ptShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"decimals\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"name\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"ptToken\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"symbol\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"pAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalSupplies\",\"inputs\":[],\"outputs\":[{\"name\":\"pt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"yt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalSupply\",\"inputs\":[{\"name\":\"yt\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferBatch\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferFromBatch\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"ytToken\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"}]",
	ID:  "IVaultController",
}

// IVaultController is an auto generated Go binding around an Ethereum contract.
type IVaultController struct {
	abi abi.ABI
}

// NewIVaultController creates a new instance of IVaultController.
func NewIVaultController() *IVaultController {
	parsed, err := IVaultControllerMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IVaultController{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IVaultController) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackAllowance is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfc091938.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function allowance(address owner, address spender, bool yt) view returns(uint256 result)
func (iVaultController *IVaultController) PackAllowance(owner common.Address, spender common.Address, yt bool) []byte {
	enc, err := iVaultController.abi.Pack("allowance", owner, spender, yt)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAllowance is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfc091938.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function allowance(address owner, address spender, bool yt) view returns(uint256 result)
func (iVaultController *IVaultController) TryPackAllowance(owner common.Address, spender common.Address, yt bool) ([]byte, error) {
	return iVaultController.abi.Pack("allowance", owner, spender, yt)
}

// UnpackAllowance is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfc091938.
//
// Solidity: function allowance(address owner, address spender, bool yt) view returns(uint256 result)
func (iVaultController *IVaultController) UnpackAllowance(data []byte) (*big.Int, error) {
	out, err := iVaultController.abi.Unpack("allowance", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackAllowancesOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc5d31bec.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function allowancesOf(address owner, address spender) view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) PackAllowancesOf(owner common.Address, spender common.Address) []byte {
	enc, err := iVaultController.abi.Pack("allowancesOf", owner, spender)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAllowancesOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc5d31bec.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function allowancesOf(address owner, address spender) view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) TryPackAllowancesOf(owner common.Address, spender common.Address) ([]byte, error) {
	return iVaultController.abi.Pack("allowancesOf", owner, spender)
}

// AllowancesOfOutput serves as a container for the return parameters of contract
// method AllowancesOf.
type AllowancesOfOutput struct {
	Pt *big.Int
	Yt *big.Int
}

// UnpackAllowancesOf is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc5d31bec.
//
// Solidity: function allowancesOf(address owner, address spender) view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) UnpackAllowancesOf(data []byte) (AllowancesOfOutput, error) {
	out, err := iVaultController.abi.Unpack("allowancesOf", data)
	outstruct := new(AllowancesOfOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.Pt = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.Yt = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackApproveBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x75ac8912.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function approveBatch(address spender, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) PackApproveBatch(spender common.Address, ptAmount *big.Int, ytAmount *big.Int) []byte {
	enc, err := iVaultController.abi.Pack("approveBatch", spender, ptAmount, ytAmount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackApproveBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x75ac8912.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function approveBatch(address spender, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) TryPackApproveBatch(spender common.Address, ptAmount *big.Int, ytAmount *big.Int) ([]byte, error) {
	return iVaultController.abi.Pack("approveBatch", spender, ptAmount, ytAmount)
}

// UnpackApproveBatch is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x75ac8912.
//
// Solidity: function approveBatch(address spender, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) UnpackApproveBatch(data []byte) (bool, error) {
	out, err := iVaultController.abi.Unpack("approveBatch", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x38d52e0f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function asset() view returns(address assetAddress)
func (iVaultController *IVaultController) PackAsset() []byte {
	enc, err := iVaultController.abi.Pack("asset")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x38d52e0f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function asset() view returns(address assetAddress)
func (iVaultController *IVaultController) TryPackAsset() ([]byte, error) {
	return iVaultController.abi.Pack("asset")
}

// UnpackAsset is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (iVaultController *IVaultController) UnpackAsset(data []byte) (common.Address, error) {
	out, err := iVaultController.abi.Unpack("asset", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackBalanceOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x772865e2.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function balanceOf(address account, bool yt) view returns(uint128)
func (iVaultController *IVaultController) PackBalanceOf(account common.Address, yt bool) []byte {
	enc, err := iVaultController.abi.Pack("balanceOf", account, yt)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBalanceOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x772865e2.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function balanceOf(address account, bool yt) view returns(uint128)
func (iVaultController *IVaultController) TryPackBalanceOf(account common.Address, yt bool) ([]byte, error) {
	return iVaultController.abi.Pack("balanceOf", account, yt)
}

// UnpackBalanceOf is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x772865e2.
//
// Solidity: function balanceOf(address account, bool yt) view returns(uint128)
func (iVaultController *IVaultController) UnpackBalanceOf(data []byte) (*big.Int, error) {
	out, err := iVaultController.abi.Unpack("balanceOf", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackBalancesOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6392a51f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function balancesOf(address account) view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) PackBalancesOf(account common.Address) []byte {
	enc, err := iVaultController.abi.Pack("balancesOf", account)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBalancesOf is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6392a51f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function balancesOf(address account) view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) TryPackBalancesOf(account common.Address) ([]byte, error) {
	return iVaultController.abi.Pack("balancesOf", account)
}

// BalancesOfOutput serves as a container for the return parameters of contract
// method BalancesOf.
type BalancesOfOutput struct {
	Pt *big.Int
	Yt *big.Int
}

// UnpackBalancesOf is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6392a51f.
//
// Solidity: function balancesOf(address account) view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) UnpackBalancesOf(data []byte) (BalancesOfOutput, error) {
	out, err := iVaultController.abi.Unpack("balancesOf", data)
	outstruct := new(BalancesOfOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.Pt = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.Yt = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackBurnAll is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdd19a0d8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function burnAll(address owner, address receiver) returns(uint256 ptShares, uint256 ytShares, uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) PackBurnAll(owner common.Address, receiver common.Address) []byte {
	enc, err := iVaultController.abi.Pack("burnAll", owner, receiver)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBurnAll is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdd19a0d8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function burnAll(address owner, address receiver) returns(uint256 ptShares, uint256 ytShares, uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) TryPackBurnAll(owner common.Address, receiver common.Address) ([]byte, error) {
	return iVaultController.abi.Pack("burnAll", owner, receiver)
}

// BurnAllOutput serves as a container for the return parameters of contract
// method BurnAll.
type BurnAllOutput struct {
	PtShares *big.Int
	YtShares *big.Int
	PAssets  *big.Int
	YAssets  *big.Int
}

// UnpackBurnAll is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xdd19a0d8.
//
// Solidity: function burnAll(address owner, address receiver) returns(uint256 ptShares, uint256 ytShares, uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) UnpackBurnAll(data []byte) (BurnAllOutput, error) {
	out, err := iVaultController.abi.Unpack("burnAll", data)
	outstruct := new(BurnAllOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.PtShares = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.YtShares = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.PAssets = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.YAssets = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackCanWithdraw is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb51459fe.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function canWithdraw() view returns(bool)
func (iVaultController *IVaultController) PackCanWithdraw() []byte {
	enc, err := iVaultController.abi.Pack("canWithdraw")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackCanWithdraw is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb51459fe.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function canWithdraw() view returns(bool)
func (iVaultController *IVaultController) TryPackCanWithdraw() ([]byte, error) {
	return iVaultController.abi.Pack("canWithdraw")
}

// UnpackCanWithdraw is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xb51459fe.
//
// Solidity: function canWithdraw() view returns(bool)
func (iVaultController *IVaultController) UnpackCanWithdraw(data []byte) (bool, error) {
	out, err := iVaultController.abi.Unpack("canWithdraw", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackConvertToAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x181e7b3b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function convertToAssets(uint256 ptShares, uint256 ytShares) view returns(uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) PackConvertToAssets(ptShares *big.Int, ytShares *big.Int) []byte {
	enc, err := iVaultController.abi.Pack("convertToAssets", ptShares, ytShares)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackConvertToAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x181e7b3b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function convertToAssets(uint256 ptShares, uint256 ytShares) view returns(uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) TryPackConvertToAssets(ptShares *big.Int, ytShares *big.Int) ([]byte, error) {
	return iVaultController.abi.Pack("convertToAssets", ptShares, ytShares)
}

// ConvertToAssetsOutput serves as a container for the return parameters of contract
// method ConvertToAssets.
type ConvertToAssetsOutput struct {
	PAssets *big.Int
	YAssets *big.Int
}

// UnpackConvertToAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x181e7b3b.
//
// Solidity: function convertToAssets(uint256 ptShares, uint256 ytShares) view returns(uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) UnpackConvertToAssets(data []byte) (ConvertToAssetsOutput, error) {
	out, err := iVaultController.abi.Unpack("convertToAssets", data)
	outstruct := new(ConvertToAssetsOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.PAssets = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.YAssets = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackConvertToShares is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdb2088f4.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function convertToShares(uint256 pAssets, uint256 yAssets) view returns(uint256 ptShares, uint256 ytShares)
func (iVaultController *IVaultController) PackConvertToShares(pAssets *big.Int, yAssets *big.Int) []byte {
	enc, err := iVaultController.abi.Pack("convertToShares", pAssets, yAssets)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackConvertToShares is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdb2088f4.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function convertToShares(uint256 pAssets, uint256 yAssets) view returns(uint256 ptShares, uint256 ytShares)
func (iVaultController *IVaultController) TryPackConvertToShares(pAssets *big.Int, yAssets *big.Int) ([]byte, error) {
	return iVaultController.abi.Pack("convertToShares", pAssets, yAssets)
}

// ConvertToSharesOutput serves as a container for the return parameters of contract
// method ConvertToShares.
type ConvertToSharesOutput struct {
	PtShares *big.Int
	YtShares *big.Int
}

// UnpackConvertToShares is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xdb2088f4.
//
// Solidity: function convertToShares(uint256 pAssets, uint256 yAssets) view returns(uint256 ptShares, uint256 ytShares)
func (iVaultController *IVaultController) UnpackConvertToShares(data []byte) (ConvertToSharesOutput, error) {
	out, err := iVaultController.abi.Unpack("convertToShares", data)
	outstruct := new(ConvertToSharesOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.PtShares = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.YtShares = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function decimals() view returns(uint8)
func (iVaultController *IVaultController) PackDecimals() []byte {
	enc, err := iVaultController.abi.Pack("decimals")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function decimals() view returns(uint8)
func (iVaultController *IVaultController) TryPackDecimals() ([]byte, error) {
	return iVaultController.abi.Pack("decimals")
}

// UnpackDecimals is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (iVaultController *IVaultController) UnpackDecimals(data []byte) (uint8, error) {
	out, err := iVaultController.abi.Unpack("decimals", data)
	if err != nil {
		return *new(uint8), err
	}
	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)
	return out0, nil
}

// PackName is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x06fdde03.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function name() view returns(string)
func (iVaultController *IVaultController) PackName() []byte {
	enc, err := iVaultController.abi.Pack("name")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackName is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x06fdde03.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function name() view returns(string)
func (iVaultController *IVaultController) TryPackName() ([]byte, error) {
	return iVaultController.abi.Pack("name")
}

// UnpackName is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (iVaultController *IVaultController) UnpackName(data []byte) (string, error) {
	out, err := iVaultController.abi.Unpack("name", data)
	if err != nil {
		return *new(string), err
	}
	out0 := *abi.ConvertType(out[0], new(string)).(*string)
	return out0, nil
}

// PackPtToken is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe018b0ef.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function ptToken() view returns(address)
func (iVaultController *IVaultController) PackPtToken() []byte {
	enc, err := iVaultController.abi.Pack("ptToken")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPtToken is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe018b0ef.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function ptToken() view returns(address)
func (iVaultController *IVaultController) TryPackPtToken() ([]byte, error) {
	return iVaultController.abi.Pack("ptToken")
}

// UnpackPtToken is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xe018b0ef.
//
// Solidity: function ptToken() view returns(address)
func (iVaultController *IVaultController) UnpackPtToken(data []byte) (common.Address, error) {
	out, err := iVaultController.abi.Unpack("ptToken", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackSymbol is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x95d89b41.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function symbol() view returns(string)
func (iVaultController *IVaultController) PackSymbol() []byte {
	enc, err := iVaultController.abi.Pack("symbol")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSymbol is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x95d89b41.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function symbol() view returns(string)
func (iVaultController *IVaultController) TryPackSymbol() ([]byte, error) {
	return iVaultController.abi.Pack("symbol")
}

// UnpackSymbol is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (iVaultController *IVaultController) UnpackSymbol(data []byte) (string, error) {
	out, err := iVaultController.abi.Unpack("symbol", data)
	if err != nil {
		return *new(string), err
	}
	out0 := *abi.ConvertType(out[0], new(string)).(*string)
	return out0, nil
}

// PackTotalAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x01e1d114.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalAssets() view returns(uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) PackTotalAssets() []byte {
	enc, err := iVaultController.abi.Pack("totalAssets")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTotalAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x01e1d114.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function totalAssets() view returns(uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) TryPackTotalAssets() ([]byte, error) {
	return iVaultController.abi.Pack("totalAssets")
}

// TotalAssetsOutput serves as a container for the return parameters of contract
// method TotalAssets.
type TotalAssetsOutput struct {
	PAssets *big.Int
	YAssets *big.Int
}

// UnpackTotalAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 pAssets, uint256 yAssets)
func (iVaultController *IVaultController) UnpackTotalAssets(data []byte) (TotalAssetsOutput, error) {
	out, err := iVaultController.abi.Unpack("totalAssets", data)
	outstruct := new(TotalAssetsOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.PAssets = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.YAssets = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackTotalSupplies is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd068cdc5.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalSupplies() view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) PackTotalSupplies() []byte {
	enc, err := iVaultController.abi.Pack("totalSupplies")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTotalSupplies is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd068cdc5.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function totalSupplies() view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) TryPackTotalSupplies() ([]byte, error) {
	return iVaultController.abi.Pack("totalSupplies")
}

// TotalSuppliesOutput serves as a container for the return parameters of contract
// method TotalSupplies.
type TotalSuppliesOutput struct {
	Pt *big.Int
	Yt *big.Int
}

// UnpackTotalSupplies is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd068cdc5.
//
// Solidity: function totalSupplies() view returns(uint128 pt, uint128 yt)
func (iVaultController *IVaultController) UnpackTotalSupplies(data []byte) (TotalSuppliesOutput, error) {
	out, err := iVaultController.abi.Unpack("totalSupplies", data)
	outstruct := new(TotalSuppliesOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.Pt = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.Yt = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackTotalSupply is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x89942649.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalSupply(bool yt) view returns(uint128)
func (iVaultController *IVaultController) PackTotalSupply(yt bool) []byte {
	enc, err := iVaultController.abi.Pack("totalSupply", yt)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTotalSupply is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x89942649.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function totalSupply(bool yt) view returns(uint128)
func (iVaultController *IVaultController) TryPackTotalSupply(yt bool) ([]byte, error) {
	return iVaultController.abi.Pack("totalSupply", yt)
}

// UnpackTotalSupply is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x89942649.
//
// Solidity: function totalSupply(bool yt) view returns(uint128)
func (iVaultController *IVaultController) UnpackTotalSupply(data []byte) (*big.Int, error) {
	out, err := iVaultController.abi.Unpack("totalSupply", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackTransferBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x161a5ef8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferBatch(address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) PackTransferBatch(to common.Address, ptAmount *big.Int, ytAmount *big.Int) []byte {
	enc, err := iVaultController.abi.Pack("transferBatch", to, ptAmount, ytAmount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTransferBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x161a5ef8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function transferBatch(address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) TryPackTransferBatch(to common.Address, ptAmount *big.Int, ytAmount *big.Int) ([]byte, error) {
	return iVaultController.abi.Pack("transferBatch", to, ptAmount, ytAmount)
}

// UnpackTransferBatch is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x161a5ef8.
//
// Solidity: function transferBatch(address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) UnpackTransferBatch(data []byte) (bool, error) {
	out, err := iVaultController.abi.Unpack("transferBatch", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackTransferFromBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x68a3b3de.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferFromBatch(address from, address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) PackTransferFromBatch(from common.Address, to common.Address, ptAmount *big.Int, ytAmount *big.Int) []byte {
	enc, err := iVaultController.abi.Pack("transferFromBatch", from, to, ptAmount, ytAmount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTransferFromBatch is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x68a3b3de.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function transferFromBatch(address from, address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) TryPackTransferFromBatch(from common.Address, to common.Address, ptAmount *big.Int, ytAmount *big.Int) ([]byte, error) {
	return iVaultController.abi.Pack("transferFromBatch", from, to, ptAmount, ytAmount)
}

// UnpackTransferFromBatch is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x68a3b3de.
//
// Solidity: function transferFromBatch(address from, address to, uint256 ptAmount, uint256 ytAmount) returns(bool)
func (iVaultController *IVaultController) UnpackTransferFromBatch(data []byte) (bool, error) {
	out, err := iVaultController.abi.Unpack("transferFromBatch", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackYtToken is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x42203015.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function ytToken() view returns(address)
func (iVaultController *IVaultController) PackYtToken() []byte {
	enc, err := iVaultController.abi.Pack("ytToken")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackYtToken is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x42203015.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function ytToken() view returns(address)
func (iVaultController *IVaultController) TryPackYtToken() ([]byte, error) {
	return iVaultController.abi.Pack("ytToken")
}

// UnpackYtToken is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x42203015.
//
// Solidity: function ytToken() view returns(address)
func (iVaultController *IVaultController) UnpackYtToken(data []byte) (common.Address, error) {
	out, err := iVaultController.abi.Unpack("ytToken", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}
