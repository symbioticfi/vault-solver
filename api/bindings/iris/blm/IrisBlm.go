// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package blm

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

// Quote is an auto generated low-level Go binding around an user-defined struct.
type Quote struct {
	Borrower        common.Address
	Solver          common.Address
	Receiver        common.Address
	Blm             common.Address
	CollateralToken common.Address
	DebtToken       common.Address
	Collateral      *big.Int
	Debt            *big.Int
	FixedRate       *big.Int
	Duration        *big.Int
	OverdueRate     *big.Int
	OverduePeriod   *big.Int
	Bond            *big.Int
	BondLltv        *big.Int
	VenueBitmap     *big.Int
	VenueId         *big.Int
	Deadline        *big.Int
	Nonce           *big.Int
	Data            []byte
}

// IrisBlmMetaData contains all meta data concerning the IrisBlm contract.
var IrisBlmMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"bondRequirement\",\"inputs\":[{\"name\":\"quote\",\"type\":\"tuple\",\"internalType\":\"structQuote\",\"components\":[{\"name\":\"borrower\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"solver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"blm\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"collateralToken\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"debtToken\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"collateral\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"debt\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"fixedRate\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"duration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"overdueRate\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"overduePeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"bond\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"bondLltv\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"venueBitmap\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"venueId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]}],\"outputs\":[{\"name\":\"bondRequirement\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"}]",
	ID:  "IrisBlm",
}

// IrisBlm is an auto generated Go binding around an Ethereum contract.
type IrisBlm struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *IrisBlm) GetABI() abi.ABI {
	return c.abi
}

// NewIrisBlm creates a new instance of IrisBlm.
func NewIrisBlm() *IrisBlm {
	parsed, err := IrisBlmMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IrisBlm{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IrisBlm) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackBondRequirement is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0eaaacbd.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function bondRequirement((address,address,address,address,address,address,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,bytes) quote) view returns(uint256 bondRequirement)
func (irisBlm *IrisBlm) PackBondRequirement(quote Quote) []byte {
	enc, err := irisBlm.abi.Pack("bondRequirement", quote)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBondRequirement is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0eaaacbd.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function bondRequirement((address,address,address,address,address,address,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,bytes) quote) view returns(uint256 bondRequirement)
func (irisBlm *IrisBlm) TryPackBondRequirement(quote Quote) ([]byte, error) {
	return irisBlm.abi.Pack("bondRequirement", quote)
}

// UnpackBondRequirement is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0eaaacbd.
//
// Solidity: function bondRequirement((address,address,address,address,address,address,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,bytes) quote) view returns(uint256 bondRequirement)
func (irisBlm *IrisBlm) UnpackBondRequirement(data []byte) (*big.Int, error) {
	out, err := irisBlm.abi.Unpack("bondRequirement", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
