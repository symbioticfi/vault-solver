// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package aggregator

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

// AggregatorV3MetaData contains all meta data concerning the AggregatorV3 contract.
var AggregatorV3MetaData = bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"latestRoundData\",\"outputs\":[{\"name\":\"roundId\",\"type\":\"uint80\"},{\"name\":\"answer\",\"type\":\"int256\"},{\"name\":\"startedAt\",\"type\":\"uint256\"},{\"name\":\"updatedAt\",\"type\":\"uint256\"},{\"name\":\"answeredInRound\",\"type\":\"uint80\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	ID:  "AggregatorV3",
}

// AggregatorV3 is an auto generated Go binding around an Ethereum contract.
type AggregatorV3 struct {
	abi abi.ABI
}

// NewAggregatorV3 creates a new instance of AggregatorV3.
func NewAggregatorV3() *AggregatorV3 {
	parsed, err := AggregatorV3MetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &AggregatorV3{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *AggregatorV3) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function decimals() view returns(uint8)
func (aggregatorV3 *AggregatorV3) PackDecimals() []byte {
	enc, err := aggregatorV3.abi.Pack("decimals")
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
func (aggregatorV3 *AggregatorV3) TryPackDecimals() ([]byte, error) {
	return aggregatorV3.abi.Pack("decimals")
}

// UnpackDecimals is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (aggregatorV3 *AggregatorV3) UnpackDecimals(data []byte) (uint8, error) {
	out, err := aggregatorV3.abi.Unpack("decimals", data)
	if err != nil {
		return *new(uint8), err
	}
	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)
	return out0, nil
}

// PackLatestRoundData is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfeaf968c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function latestRoundData() view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (aggregatorV3 *AggregatorV3) PackLatestRoundData() []byte {
	enc, err := aggregatorV3.abi.Pack("latestRoundData")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLatestRoundData is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfeaf968c.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function latestRoundData() view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (aggregatorV3 *AggregatorV3) TryPackLatestRoundData() ([]byte, error) {
	return aggregatorV3.abi.Pack("latestRoundData")
}

// LatestRoundDataOutput serves as a container for the return parameters of contract
// method LatestRoundData.
type LatestRoundDataOutput struct {
	RoundId         *big.Int
	Answer          *big.Int
	StartedAt       *big.Int
	UpdatedAt       *big.Int
	AnsweredInRound *big.Int
}

// UnpackLatestRoundData is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfeaf968c.
//
// Solidity: function latestRoundData() view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (aggregatorV3 *AggregatorV3) UnpackLatestRoundData(data []byte) (LatestRoundDataOutput, error) {
	out, err := aggregatorV3.abi.Unpack("latestRoundData", data)
	outstruct := new(LatestRoundDataOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.RoundId = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.Answer = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.StartedAt = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.UpdatedAt = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	outstruct.AnsweredInRound = abi.ConvertType(out[4], new(big.Int)).(*big.Int)
	return *outstruct, nil
}
