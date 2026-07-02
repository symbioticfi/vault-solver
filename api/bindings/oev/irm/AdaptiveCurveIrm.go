// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package irm

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

// Struct0 is an auto generated low-level Go binding around an user-defined struct.
type Struct0 struct {
	LoanToken       common.Address
	CollateralToken common.Address
	Oracle          common.Address
	Irm             common.Address
	Lltv            *big.Int
}

// Struct1 is an auto generated low-level Go binding around an user-defined struct.
type Struct1 struct {
	TotalSupplyAssets *big.Int
	TotalSupplyShares *big.Int
	TotalBorrowAssets *big.Int
	TotalBorrowShares *big.Int
	LastUpdate        *big.Int
	Fee               *big.Int
}

// AdaptiveCurveIrmMetaData contains all meta data concerning the AdaptiveCurveIrm contract.
var AdaptiveCurveIrmMetaData = bind.MetaData{
	ABI: "[{\"inputs\":[{\"name\":\"marketParams\",\"type\":\"tuple\",\"components\":[{\"name\":\"loanToken\",\"type\":\"address\"},{\"name\":\"collateralToken\",\"type\":\"address\"},{\"name\":\"oracle\",\"type\":\"address\"},{\"name\":\"irm\",\"type\":\"address\"},{\"name\":\"lltv\",\"type\":\"uint256\"}]},{\"name\":\"market\",\"type\":\"tuple\",\"components\":[{\"name\":\"totalSupplyAssets\",\"type\":\"uint128\"},{\"name\":\"totalSupplyShares\",\"type\":\"uint128\"},{\"name\":\"totalBorrowAssets\",\"type\":\"uint128\"},{\"name\":\"totalBorrowShares\",\"type\":\"uint128\"},{\"name\":\"lastUpdate\",\"type\":\"uint128\"},{\"name\":\"fee\",\"type\":\"uint128\"}]}],\"name\":\"borrowRateView\",\"outputs\":[{\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	ID:  "AdaptiveCurveIrm",
}

// AdaptiveCurveIrm is an auto generated Go binding around an Ethereum contract.
type AdaptiveCurveIrm struct {
	abi abi.ABI
}

// NewAdaptiveCurveIrm creates a new instance of AdaptiveCurveIrm.
func NewAdaptiveCurveIrm() *AdaptiveCurveIrm {
	parsed, err := AdaptiveCurveIrmMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &AdaptiveCurveIrm{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *AdaptiveCurveIrm) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackBorrowRateView is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8c00bf6b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function borrowRateView((address,address,address,address,uint256) marketParams, (uint128,uint128,uint128,uint128,uint128,uint128) market) view returns(uint256)
func (adaptiveCurveIrm *AdaptiveCurveIrm) PackBorrowRateView(marketParams Struct0, market Struct1) []byte {
	enc, err := adaptiveCurveIrm.abi.Pack("borrowRateView", marketParams, market)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBorrowRateView is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8c00bf6b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function borrowRateView((address,address,address,address,uint256) marketParams, (uint128,uint128,uint128,uint128,uint128,uint128) market) view returns(uint256)
func (adaptiveCurveIrm *AdaptiveCurveIrm) TryPackBorrowRateView(marketParams Struct0, market Struct1) ([]byte, error) {
	return adaptiveCurveIrm.abi.Pack("borrowRateView", marketParams, market)
}

// UnpackBorrowRateView is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8c00bf6b.
//
// Solidity: function borrowRateView((address,address,address,address,uint256) marketParams, (uint128,uint128,uint128,uint128,uint128,uint128) market) view returns(uint256)
func (adaptiveCurveIrm *AdaptiveCurveIrm) UnpackBorrowRateView(data []byte) (*big.Int, error) {
	out, err := adaptiveCurveIrm.abi.Unpack("borrowRateView", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
