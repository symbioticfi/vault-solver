// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package positionmanager

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

// IThreeFPositionManagerMetaData contains all meta data concerning the IThreeFPositionManager contract.
var IThreeFPositionManagerMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"assets\",\"inputs\":[],\"outputs\":[{\"name\":\"collateralAsset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"debtAsset\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"}]",
	ID:  "IThreeFPositionManager",
}

// IThreeFPositionManager is an auto generated Go binding around an Ethereum contract.
type IThreeFPositionManager struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *IThreeFPositionManager) GetABI() abi.ABI {
	return c.abi
}

// NewIThreeFPositionManager creates a new instance of IThreeFPositionManager.
func NewIThreeFPositionManager() *IThreeFPositionManager {
	parsed, err := IThreeFPositionManagerMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IThreeFPositionManager{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IThreeFPositionManager) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x71a97305.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function assets() view returns(address collateralAsset, address debtAsset)
func (iThreeFPositionManager *IThreeFPositionManager) PackAssets() []byte {
	enc, err := iThreeFPositionManager.abi.Pack("assets")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x71a97305.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function assets() view returns(address collateralAsset, address debtAsset)
func (iThreeFPositionManager *IThreeFPositionManager) TryPackAssets() ([]byte, error) {
	return iThreeFPositionManager.abi.Pack("assets")
}

// AssetsOutput serves as a container for the return parameters of contract
// method Assets.
type AssetsOutput struct {
	CollateralAsset common.Address
	DebtAsset       common.Address
}

// UnpackAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x71a97305.
//
// Solidity: function assets() view returns(address collateralAsset, address debtAsset)
func (iThreeFPositionManager *IThreeFPositionManager) UnpackAssets(data []byte) (AssetsOutput, error) {
	out, err := iThreeFPositionManager.abi.Unpack("assets", data)
	outstruct := new(AssetsOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.CollateralAsset = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.DebtAsset = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	return *outstruct, nil
}
