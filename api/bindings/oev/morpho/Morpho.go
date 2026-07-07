// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package morpho

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

// MorphoMetaData contains all meta data concerning the Morpho contract.
var MorphoMetaData = bind.MetaData{
	ABI: "[{\"inputs\":[{\"type\":\"bytes32\"}],\"name\":\"market\",\"outputs\":[{\"name\":\"totalSupplyAssets\",\"type\":\"uint128\"},{\"name\":\"totalSupplyShares\",\"type\":\"uint128\"},{\"name\":\"totalBorrowAssets\",\"type\":\"uint128\"},{\"name\":\"totalBorrowShares\",\"type\":\"uint128\"},{\"name\":\"lastUpdate\",\"type\":\"uint128\"},{\"name\":\"fee\",\"type\":\"uint128\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"type\":\"bytes32\"},{\"type\":\"address\"}],\"name\":\"position\",\"outputs\":[{\"name\":\"supplyShares\",\"type\":\"uint256\"},{\"name\":\"borrowShares\",\"type\":\"uint128\"},{\"name\":\"collateral\",\"type\":\"uint128\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"type\":\"bytes32\"}],\"name\":\"idToMarketParams\",\"outputs\":[{\"name\":\"loanToken\",\"type\":\"address\"},{\"name\":\"collateralToken\",\"type\":\"address\"},{\"name\":\"oracle\",\"type\":\"address\"},{\"name\":\"irm\",\"type\":\"address\"},{\"name\":\"lltv\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	ID:  "Morpho",
}

// Morpho is an auto generated Go binding around an Ethereum contract.
type Morpho struct {
	abi abi.ABI
}

// NewMorpho creates a new instance of Morpho.
func NewMorpho() *Morpho {
	parsed, err := MorphoMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &Morpho{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *Morpho) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackIdToMarketParams is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2c3c9157.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function idToMarketParams(bytes32 ) view returns(address loanToken, address collateralToken, address oracle, address irm, uint256 lltv)
func (morpho *Morpho) PackIdToMarketParams(arg0 [32]byte) []byte {
	enc, err := morpho.abi.Pack("idToMarketParams", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIdToMarketParams is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2c3c9157.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function idToMarketParams(bytes32 ) view returns(address loanToken, address collateralToken, address oracle, address irm, uint256 lltv)
func (morpho *Morpho) TryPackIdToMarketParams(arg0 [32]byte) ([]byte, error) {
	return morpho.abi.Pack("idToMarketParams", arg0)
}

// IdToMarketParamsOutput serves as a container for the return parameters of contract
// method IdToMarketParams.
type IdToMarketParamsOutput struct {
	LoanToken       common.Address
	CollateralToken common.Address
	Oracle          common.Address
	Irm             common.Address
	Lltv            *big.Int
}

// UnpackIdToMarketParams is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2c3c9157.
//
// Solidity: function idToMarketParams(bytes32 ) view returns(address loanToken, address collateralToken, address oracle, address irm, uint256 lltv)
func (morpho *Morpho) UnpackIdToMarketParams(data []byte) (IdToMarketParamsOutput, error) {
	out, err := morpho.abi.Unpack("idToMarketParams", data)
	outstruct := new(IdToMarketParamsOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.LoanToken = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.CollateralToken = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.Oracle = *abi.ConvertType(out[2], new(common.Address)).(*common.Address)
	outstruct.Irm = *abi.ConvertType(out[3], new(common.Address)).(*common.Address)
	outstruct.Lltv = abi.ConvertType(out[4], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackMarket is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5c60e39a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function market(bytes32 ) view returns(uint128 totalSupplyAssets, uint128 totalSupplyShares, uint128 totalBorrowAssets, uint128 totalBorrowShares, uint128 lastUpdate, uint128 fee)
func (morpho *Morpho) PackMarket(arg0 [32]byte) []byte {
	enc, err := morpho.abi.Pack("market", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMarket is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5c60e39a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function market(bytes32 ) view returns(uint128 totalSupplyAssets, uint128 totalSupplyShares, uint128 totalBorrowAssets, uint128 totalBorrowShares, uint128 lastUpdate, uint128 fee)
func (morpho *Morpho) TryPackMarket(arg0 [32]byte) ([]byte, error) {
	return morpho.abi.Pack("market", arg0)
}

// MarketOutput serves as a container for the return parameters of contract
// method Market.
type MarketOutput struct {
	TotalSupplyAssets *big.Int
	TotalSupplyShares *big.Int
	TotalBorrowAssets *big.Int
	TotalBorrowShares *big.Int
	LastUpdate        *big.Int
	Fee               *big.Int
}

// UnpackMarket is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5c60e39a.
//
// Solidity: function market(bytes32 ) view returns(uint128 totalSupplyAssets, uint128 totalSupplyShares, uint128 totalBorrowAssets, uint128 totalBorrowShares, uint128 lastUpdate, uint128 fee)
func (morpho *Morpho) UnpackMarket(data []byte) (MarketOutput, error) {
	out, err := morpho.abi.Unpack("market", data)
	outstruct := new(MarketOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.TotalSupplyAssets = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.TotalSupplyShares = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.TotalBorrowAssets = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.TotalBorrowShares = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	outstruct.LastUpdate = abi.ConvertType(out[4], new(big.Int)).(*big.Int)
	outstruct.Fee = abi.ConvertType(out[5], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackPosition is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x93c52062.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function position(bytes32 , address ) view returns(uint256 supplyShares, uint128 borrowShares, uint128 collateral)
func (morpho *Morpho) PackPosition(arg0 [32]byte, arg1 common.Address) []byte {
	enc, err := morpho.abi.Pack("position", arg0, arg1)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPosition is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x93c52062.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function position(bytes32 , address ) view returns(uint256 supplyShares, uint128 borrowShares, uint128 collateral)
func (morpho *Morpho) TryPackPosition(arg0 [32]byte, arg1 common.Address) ([]byte, error) {
	return morpho.abi.Pack("position", arg0, arg1)
}

// PositionOutput serves as a container for the return parameters of contract
// method Position.
type PositionOutput struct {
	SupplyShares *big.Int
	BorrowShares *big.Int
	Collateral   *big.Int
}

// UnpackPosition is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x93c52062.
//
// Solidity: function position(bytes32 , address ) view returns(uint256 supplyShares, uint128 borrowShares, uint128 collateral)
func (morpho *Morpho) UnpackPosition(data []byte) (PositionOutput, error) {
	out, err := morpho.abi.Unpack("position", data)
	outstruct := new(PositionOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.SupplyShares = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.BorrowShares = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.Collateral = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	return *outstruct, nil
}
