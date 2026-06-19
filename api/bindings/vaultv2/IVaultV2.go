// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package vaultv2

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

// IVaultV2InitParams is an auto generated low-level Go binding around an user-defined struct.
type IVaultV2InitParams struct {
	Name                          string
	Symbol                        string
	Asset                         common.Address
	DepositWhitelist              bool
	DepositorToWhitelist          common.Address
	DepositLimit                  *big.Int
	IsDepositLimit                bool
	DefaultAdminRoleHolder        common.Address
	ManagementFeeRoleHolder       common.Address
	PerformanceFeeRoleHolder      common.Address
	DepositLimitSetRoleHolder     common.Address
	DepositorWhitelistRoleHolder  common.Address
	IsDepositLimitSetRoleHolder   common.Address
	DepositWhitelistSetRoleHolder common.Address
}

// IVaultV2MetaData contains all meta data concerning the IVaultV2 contract.
var IVaultV2MetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"accrueInterest\",\"inputs\":[],\"outputs\":[{\"name\":\"managementFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"performanceFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocolFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"balanceOfAt\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"delegator\",\"inputs\":[],\"outputs\":[{\"name\":\"delegatorAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"depositLimit\",\"inputs\":[],\"outputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"depositWhitelist\",\"inputs\":[],\"outputs\":[{\"name\":\"enabled\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAccrueInterest\",\"inputs\":[],\"outputs\":[{\"name\":\"newTotalAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"managementFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"performanceFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"protocolFeeShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isDepositLimit\",\"inputs\":[],\"outputs\":[{\"name\":\"enabled\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isDepositorWhitelisted\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"whitelisted\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isInitialized\",\"inputs\":[],\"outputs\":[{\"name\":\"initialized\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastProtocolFeeReceiver\",\"inputs\":[],\"outputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastProtocolManagementFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastProtocolPerformanceFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastUpdate\",\"inputs\":[],\"outputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"managementFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"managementFeeReceiver\",\"inputs\":[],\"outputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"performanceFee\",\"inputs\":[],\"outputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"performanceFeeReceiver\",\"inputs\":[],\"outputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pull\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"push\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"redeemable\",\"inputs\":[],\"outputs\":[{\"name\":\"shares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDelegator\",\"inputs\":[{\"name\":\"delegator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositLimit\",\"inputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositWhitelist\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositorWhitelistStatus\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setIsDepositLimit\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setManagementFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPerformanceFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setSlasher\",\"inputs\":[{\"name\":\"slasher\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalSupplyAt\",\"inputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"supply\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawable\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdrawalQueue\",\"inputs\":[],\"outputs\":[{\"name\":\"withdrawalQueueAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"AccrueInterest\",\"inputs\":[{\"name\":\"newTotalAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"managementFeeShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"performanceFeeShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"protocolFeeShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Claim\",\"inputs\":[{\"name\":\"claimer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialize\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structIVaultV2.InitParams\",\"components\":[{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"symbol\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositWhitelist\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"depositorToWhitelist\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"isDepositLimit\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"defaultAdminRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"managementFeeRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"performanceFeeRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositLimitSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositorWhitelistRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"isDepositLimitSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositWhitelistSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Pull\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Push\",\"inputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"owner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDelegator\",\"inputs\":[{\"name\":\"delegator\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositLimit\",\"inputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositWhitelist\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositorWhitelistStatus\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetIsDepositLimit\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetManagementFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetPerformanceFee\",\"inputs\":[{\"name\":\"fee\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetWithdrawalQueue\",\"inputs\":[{\"name\":\"withdrawalQueue\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"UpdateProtocolFee\",\"inputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"managementFee\",\"type\":\"uint96\",\"indexed\":false,\"internalType\":\"uint96\"},{\"name\":\"performanceFee\",\"type\":\"uint96\",\"indexed\":false,\"internalType\":\"uint96\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DelegatorAlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DepositLimitReached\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"FeeTooHigh\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientFreeAssets\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAddress\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDelegator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDepositorToWhitelist\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotDelegator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotWhitelistedDepositor\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotWithdrawalQueue\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"PendingWithdrawalQueue\",\"inputs\":[]}]",
	ID:  "IVaultV2",
}

// IVaultV2 is an auto generated Go binding around an Ethereum contract.
type IVaultV2 struct {
	abi abi.ABI
}

// NewIVaultV2 creates a new instance of IVaultV2.
func NewIVaultV2() *IVaultV2 {
	parsed, err := IVaultV2MetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IVaultV2{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IVaultV2) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dd31000.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function FACTORY() view returns(address)
func (iVaultV2 *IVaultV2) PackFACTORY() []byte {
	enc, err := iVaultV2.abi.Pack("FACTORY")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dd31000.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function FACTORY() view returns(address)
func (iVaultV2 *IVaultV2) TryPackFACTORY() ([]byte, error) {
	return iVaultV2.abi.Pack("FACTORY")
}

// UnpackFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (iVaultV2 *IVaultV2) UnpackFACTORY(data []byte) (common.Address, error) {
	out, err := iVaultV2.abi.Unpack("FACTORY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackAccrueInterest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa6afed95.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function accrueInterest() returns(uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (iVaultV2 *IVaultV2) PackAccrueInterest() []byte {
	enc, err := iVaultV2.abi.Pack("accrueInterest")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAccrueInterest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa6afed95.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function accrueInterest() returns(uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (iVaultV2 *IVaultV2) TryPackAccrueInterest() ([]byte, error) {
	return iVaultV2.abi.Pack("accrueInterest")
}

// AccrueInterestOutput serves as a container for the return parameters of contract
// method AccrueInterest.
type AccrueInterestOutput struct {
	ManagementFeeShares  *big.Int
	PerformanceFeeShares *big.Int
	ProtocolFeeShares    *big.Int
}

// UnpackAccrueInterest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xa6afed95.
//
// Solidity: function accrueInterest() returns(uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (iVaultV2 *IVaultV2) UnpackAccrueInterest(data []byte) (AccrueInterestOutput, error) {
	out, err := iVaultV2.abi.Unpack("accrueInterest", data)
	outstruct := new(AccrueInterestOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.ManagementFeeShares = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.PerformanceFeeShares = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.ProtocolFeeShares = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackBalanceOfAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x95c3a492.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function balanceOfAt(address account, uint48 timestamp) view returns(uint256 balance)
func (iVaultV2 *IVaultV2) PackBalanceOfAt(account common.Address, timestamp *big.Int) []byte {
	enc, err := iVaultV2.abi.Pack("balanceOfAt", account, timestamp)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBalanceOfAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x95c3a492.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function balanceOfAt(address account, uint48 timestamp) view returns(uint256 balance)
func (iVaultV2 *IVaultV2) TryPackBalanceOfAt(account common.Address, timestamp *big.Int) ([]byte, error) {
	return iVaultV2.abi.Pack("balanceOfAt", account, timestamp)
}

// UnpackBalanceOfAt is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x95c3a492.
//
// Solidity: function balanceOfAt(address account, uint48 timestamp) view returns(uint256 balance)
func (iVaultV2 *IVaultV2) UnpackBalanceOfAt(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("balanceOfAt", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackDelegator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xce9b7930.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function delegator() view returns(address delegatorAddress)
func (iVaultV2 *IVaultV2) PackDelegator() []byte {
	enc, err := iVaultV2.abi.Pack("delegator")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDelegator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xce9b7930.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function delegator() view returns(address delegatorAddress)
func (iVaultV2 *IVaultV2) TryPackDelegator() ([]byte, error) {
	return iVaultV2.abi.Pack("delegator")
}

// UnpackDelegator is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xce9b7930.
//
// Solidity: function delegator() view returns(address delegatorAddress)
func (iVaultV2 *IVaultV2) UnpackDelegator(data []byte) (common.Address, error) {
	out, err := iVaultV2.abi.Unpack("delegator", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xecf70858.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function depositLimit() view returns(uint256 limit)
func (iVaultV2 *IVaultV2) PackDepositLimit() []byte {
	enc, err := iVaultV2.abi.Pack("depositLimit")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xecf70858.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function depositLimit() view returns(uint256 limit)
func (iVaultV2 *IVaultV2) TryPackDepositLimit() ([]byte, error) {
	return iVaultV2.abi.Pack("depositLimit")
}

// UnpackDepositLimit is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xecf70858.
//
// Solidity: function depositLimit() view returns(uint256 limit)
func (iVaultV2 *IVaultV2) UnpackDepositLimit(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("depositLimit", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackDepositWhitelist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x48d3b775.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function depositWhitelist() view returns(bool enabled)
func (iVaultV2 *IVaultV2) PackDepositWhitelist() []byte {
	enc, err := iVaultV2.abi.Pack("depositWhitelist")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDepositWhitelist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x48d3b775.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function depositWhitelist() view returns(bool enabled)
func (iVaultV2 *IVaultV2) TryPackDepositWhitelist() ([]byte, error) {
	return iVaultV2.abi.Pack("depositWhitelist")
}

// UnpackDepositWhitelist is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x48d3b775.
//
// Solidity: function depositWhitelist() view returns(bool enabled)
func (iVaultV2 *IVaultV2) UnpackDepositWhitelist(data []byte) (bool, error) {
	out, err := iVaultV2.abi.Unpack("depositWhitelist", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackFreeAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x11f240ac.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (iVaultV2 *IVaultV2) PackFreeAssets() []byte {
	enc, err := iVaultV2.abi.Pack("freeAssets")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFreeAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x11f240ac.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (iVaultV2 *IVaultV2) TryPackFreeAssets() ([]byte, error) {
	return iVaultV2.abi.Pack("freeAssets")
}

// UnpackFreeAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (iVaultV2 *IVaultV2) UnpackFreeAssets(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("freeAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackGetAccrueInterest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0445a611.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getAccrueInterest() view returns(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (iVaultV2 *IVaultV2) PackGetAccrueInterest() []byte {
	enc, err := iVaultV2.abi.Pack("getAccrueInterest")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetAccrueInterest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0445a611.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getAccrueInterest() view returns(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (iVaultV2 *IVaultV2) TryPackGetAccrueInterest() ([]byte, error) {
	return iVaultV2.abi.Pack("getAccrueInterest")
}

// GetAccrueInterestOutput serves as a container for the return parameters of contract
// method GetAccrueInterest.
type GetAccrueInterestOutput struct {
	NewTotalAssets       *big.Int
	ManagementFeeShares  *big.Int
	PerformanceFeeShares *big.Int
	ProtocolFeeShares    *big.Int
}

// UnpackGetAccrueInterest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0445a611.
//
// Solidity: function getAccrueInterest() view returns(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (iVaultV2 *IVaultV2) UnpackGetAccrueInterest(data []byte) (GetAccrueInterestOutput, error) {
	out, err := iVaultV2.abi.Unpack("getAccrueInterest", data)
	outstruct := new(GetAccrueInterestOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.NewTotalAssets = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.ManagementFeeShares = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.PerformanceFeeShares = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.ProtocolFeeShares = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (iVaultV2 *IVaultV2) PackInitialize(initialVersion uint64, owner common.Address, data []byte) []byte {
	enc, err := iVaultV2.abi.Pack("initialize", initialVersion, owner, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (iVaultV2 *IVaultV2) TryPackInitialize(initialVersion uint64, owner common.Address, data []byte) ([]byte, error) {
	return iVaultV2.abi.Pack("initialize", initialVersion, owner, data)
}

// PackIsDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa1b12202.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isDepositLimit() view returns(bool enabled)
func (iVaultV2 *IVaultV2) PackIsDepositLimit() []byte {
	enc, err := iVaultV2.abi.Pack("isDepositLimit")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa1b12202.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isDepositLimit() view returns(bool enabled)
func (iVaultV2 *IVaultV2) TryPackIsDepositLimit() ([]byte, error) {
	return iVaultV2.abi.Pack("isDepositLimit")
}

// UnpackIsDepositLimit is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xa1b12202.
//
// Solidity: function isDepositLimit() view returns(bool enabled)
func (iVaultV2 *IVaultV2) UnpackIsDepositLimit(data []byte) (bool, error) {
	out, err := iVaultV2.abi.Unpack("isDepositLimit", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackIsDepositorWhitelisted is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x794b15b7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool whitelisted)
func (iVaultV2 *IVaultV2) PackIsDepositorWhitelisted(account common.Address) []byte {
	enc, err := iVaultV2.abi.Pack("isDepositorWhitelisted", account)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsDepositorWhitelisted is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x794b15b7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool whitelisted)
func (iVaultV2 *IVaultV2) TryPackIsDepositorWhitelisted(account common.Address) ([]byte, error) {
	return iVaultV2.abi.Pack("isDepositorWhitelisted", account)
}

// UnpackIsDepositorWhitelisted is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x794b15b7.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool whitelisted)
func (iVaultV2 *IVaultV2) UnpackIsDepositorWhitelisted(data []byte) (bool, error) {
	out, err := iVaultV2.abi.Unpack("isDepositorWhitelisted", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackIsInitialized is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x392e53cd.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isInitialized() view returns(bool initialized)
func (iVaultV2 *IVaultV2) PackIsInitialized() []byte {
	enc, err := iVaultV2.abi.Pack("isInitialized")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsInitialized is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x392e53cd.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isInitialized() view returns(bool initialized)
func (iVaultV2 *IVaultV2) TryPackIsInitialized() ([]byte, error) {
	return iVaultV2.abi.Pack("isInitialized")
}

// UnpackIsInitialized is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x392e53cd.
//
// Solidity: function isInitialized() view returns(bool initialized)
func (iVaultV2 *IVaultV2) UnpackIsInitialized(data []byte) (bool, error) {
	out, err := iVaultV2.abi.Unpack("isInitialized", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackLastProtocolFeeReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xef9c691f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function lastProtocolFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) PackLastProtocolFeeReceiver() []byte {
	enc, err := iVaultV2.abi.Pack("lastProtocolFeeReceiver")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLastProtocolFeeReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xef9c691f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function lastProtocolFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) TryPackLastProtocolFeeReceiver() ([]byte, error) {
	return iVaultV2.abi.Pack("lastProtocolFeeReceiver")
}

// UnpackLastProtocolFeeReceiver is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xef9c691f.
//
// Solidity: function lastProtocolFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) UnpackLastProtocolFeeReceiver(data []byte) (common.Address, error) {
	out, err := iVaultV2.abi.Unpack("lastProtocolFeeReceiver", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackLastProtocolManagementFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x192fb170.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function lastProtocolManagementFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) PackLastProtocolManagementFee() []byte {
	enc, err := iVaultV2.abi.Pack("lastProtocolManagementFee")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLastProtocolManagementFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x192fb170.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function lastProtocolManagementFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) TryPackLastProtocolManagementFee() ([]byte, error) {
	return iVaultV2.abi.Pack("lastProtocolManagementFee")
}

// UnpackLastProtocolManagementFee is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x192fb170.
//
// Solidity: function lastProtocolManagementFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) UnpackLastProtocolManagementFee(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("lastProtocolManagementFee", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackLastProtocolPerformanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf56f1926.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function lastProtocolPerformanceFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) PackLastProtocolPerformanceFee() []byte {
	enc, err := iVaultV2.abi.Pack("lastProtocolPerformanceFee")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLastProtocolPerformanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf56f1926.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function lastProtocolPerformanceFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) TryPackLastProtocolPerformanceFee() ([]byte, error) {
	return iVaultV2.abi.Pack("lastProtocolPerformanceFee")
}

// UnpackLastProtocolPerformanceFee is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xf56f1926.
//
// Solidity: function lastProtocolPerformanceFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) UnpackLastProtocolPerformanceFee(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("lastProtocolPerformanceFee", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackLastUpdate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc0463711.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function lastUpdate() view returns(uint48 timestamp)
func (iVaultV2 *IVaultV2) PackLastUpdate() []byte {
	enc, err := iVaultV2.abi.Pack("lastUpdate")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLastUpdate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc0463711.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function lastUpdate() view returns(uint48 timestamp)
func (iVaultV2 *IVaultV2) TryPackLastUpdate() ([]byte, error) {
	return iVaultV2.abi.Pack("lastUpdate")
}

// UnpackLastUpdate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc0463711.
//
// Solidity: function lastUpdate() view returns(uint48 timestamp)
func (iVaultV2 *IVaultV2) UnpackLastUpdate(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("lastUpdate", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackManagementFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa6f7f5d6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function managementFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) PackManagementFee() []byte {
	enc, err := iVaultV2.abi.Pack("managementFee")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackManagementFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa6f7f5d6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function managementFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) TryPackManagementFee() ([]byte, error) {
	return iVaultV2.abi.Pack("managementFee")
}

// UnpackManagementFee is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xa6f7f5d6.
//
// Solidity: function managementFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) UnpackManagementFee(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("managementFee", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackManagementFeeReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x43039947.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function managementFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) PackManagementFeeReceiver() []byte {
	enc, err := iVaultV2.abi.Pack("managementFeeReceiver")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackManagementFeeReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x43039947.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function managementFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) TryPackManagementFeeReceiver() ([]byte, error) {
	return iVaultV2.abi.Pack("managementFeeReceiver")
}

// UnpackManagementFeeReceiver is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x43039947.
//
// Solidity: function managementFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) UnpackManagementFeeReceiver(data []byte) (common.Address, error) {
	out, err := iVaultV2.abi.Unpack("managementFeeReceiver", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2abe3048.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (iVaultV2 *IVaultV2) PackMigrate(newVersion uint64, data []byte) []byte {
	enc, err := iVaultV2.abi.Pack("migrate", newVersion, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2abe3048.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (iVaultV2 *IVaultV2) TryPackMigrate(newVersion uint64, data []byte) ([]byte, error) {
	return iVaultV2.abi.Pack("migrate", newVersion, data)
}

// PackMulticall is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xac9650d8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function multicall(bytes[] data) returns()
func (iVaultV2 *IVaultV2) PackMulticall(data [][]byte) []byte {
	enc, err := iVaultV2.abi.Pack("multicall", data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMulticall is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xac9650d8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function multicall(bytes[] data) returns()
func (iVaultV2 *IVaultV2) TryPackMulticall(data [][]byte) ([]byte, error) {
	return iVaultV2.abi.Pack("multicall", data)
}

// PackPerformanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x87788782.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function performanceFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) PackPerformanceFee() []byte {
	enc, err := iVaultV2.abi.Pack("performanceFee")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPerformanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x87788782.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function performanceFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) TryPackPerformanceFee() ([]byte, error) {
	return iVaultV2.abi.Pack("performanceFee")
}

// UnpackPerformanceFee is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x87788782.
//
// Solidity: function performanceFee() view returns(uint96 fee)
func (iVaultV2 *IVaultV2) UnpackPerformanceFee(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("performanceFee", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackPerformanceFeeReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x82cf16df.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function performanceFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) PackPerformanceFeeReceiver() []byte {
	enc, err := iVaultV2.abi.Pack("performanceFeeReceiver")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPerformanceFeeReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x82cf16df.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function performanceFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) TryPackPerformanceFeeReceiver() ([]byte, error) {
	return iVaultV2.abi.Pack("performanceFeeReceiver")
}

// UnpackPerformanceFeeReceiver is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x82cf16df.
//
// Solidity: function performanceFeeReceiver() view returns(address receiver)
func (iVaultV2 *IVaultV2) UnpackPerformanceFeeReceiver(data []byte) (common.Address, error) {
	out, err := iVaultV2.abi.Unpack("performanceFeeReceiver", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackPull is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x97b41a12.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function pull(uint256 assets, address receiver) returns()
func (iVaultV2 *IVaultV2) PackPull(assets *big.Int, receiver common.Address) []byte {
	enc, err := iVaultV2.abi.Pack("pull", assets, receiver)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPull is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x97b41a12.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function pull(uint256 assets, address receiver) returns()
func (iVaultV2 *IVaultV2) TryPackPull(assets *big.Int, receiver common.Address) ([]byte, error) {
	return iVaultV2.abi.Pack("pull", assets, receiver)
}

// PackPush is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc80fbe4e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function push(uint256 assets, address owner) returns()
func (iVaultV2 *IVaultV2) PackPush(assets *big.Int, owner common.Address) []byte {
	enc, err := iVaultV2.abi.Pack("push", assets, owner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPush is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc80fbe4e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function push(uint256 assets, address owner) returns()
func (iVaultV2 *IVaultV2) TryPackPush(assets *big.Int, owner common.Address) ([]byte, error) {
	return iVaultV2.abi.Pack("push", assets, owner)
}

// PackRedeemable is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2d7ecd11.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function redeemable() returns(uint256 shares)
func (iVaultV2 *IVaultV2) PackRedeemable() []byte {
	enc, err := iVaultV2.abi.Pack("redeemable")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRedeemable is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2d7ecd11.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function redeemable() returns(uint256 shares)
func (iVaultV2 *IVaultV2) TryPackRedeemable() ([]byte, error) {
	return iVaultV2.abi.Pack("redeemable")
}

// UnpackRedeemable is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2d7ecd11.
//
// Solidity: function redeemable() returns(uint256 shares)
func (iVaultV2 *IVaultV2) UnpackRedeemable(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("redeemable", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackSetDelegator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x83cd9cc3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setDelegator(address delegator) returns()
func (iVaultV2 *IVaultV2) PackSetDelegator(delegator common.Address) []byte {
	enc, err := iVaultV2.abi.Pack("setDelegator", delegator)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetDelegator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x83cd9cc3.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setDelegator(address delegator) returns()
func (iVaultV2 *IVaultV2) TryPackSetDelegator(delegator common.Address) ([]byte, error) {
	return iVaultV2.abi.Pack("setDelegator", delegator)
}

// PackSetDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbdc8144b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (iVaultV2 *IVaultV2) PackSetDepositLimit(limit *big.Int) []byte {
	enc, err := iVaultV2.abi.Pack("setDepositLimit", limit)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbdc8144b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (iVaultV2 *IVaultV2) TryPackSetDepositLimit(limit *big.Int) ([]byte, error) {
	return iVaultV2.abi.Pack("setDepositLimit", limit)
}

// PackSetDepositWhitelist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4105a7dd.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (iVaultV2 *IVaultV2) PackSetDepositWhitelist(status bool) []byte {
	enc, err := iVaultV2.abi.Pack("setDepositWhitelist", status)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetDepositWhitelist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4105a7dd.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (iVaultV2 *IVaultV2) TryPackSetDepositWhitelist(status bool) ([]byte, error) {
	return iVaultV2.abi.Pack("setDepositWhitelist", status)
}

// PackSetDepositorWhitelistStatus is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa2861466.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (iVaultV2 *IVaultV2) PackSetDepositorWhitelistStatus(account common.Address, status bool) []byte {
	enc, err := iVaultV2.abi.Pack("setDepositorWhitelistStatus", account, status)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetDepositorWhitelistStatus is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa2861466.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (iVaultV2 *IVaultV2) TryPackSetDepositorWhitelistStatus(account common.Address, status bool) ([]byte, error) {
	return iVaultV2.abi.Pack("setDepositorWhitelistStatus", account, status)
}

// PackSetIsDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5346e34f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (iVaultV2 *IVaultV2) PackSetIsDepositLimit(status bool) []byte {
	enc, err := iVaultV2.abi.Pack("setIsDepositLimit", status)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetIsDepositLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5346e34f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (iVaultV2 *IVaultV2) TryPackSetIsDepositLimit(status bool) ([]byte, error) {
	return iVaultV2.abi.Pack("setIsDepositLimit", status)
}

// PackSetManagementFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf5cd33d2.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setManagementFee(uint96 fee, address receiver) returns()
func (iVaultV2 *IVaultV2) PackSetManagementFee(fee *big.Int, receiver common.Address) []byte {
	enc, err := iVaultV2.abi.Pack("setManagementFee", fee, receiver)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetManagementFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf5cd33d2.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setManagementFee(uint96 fee, address receiver) returns()
func (iVaultV2 *IVaultV2) TryPackSetManagementFee(fee *big.Int, receiver common.Address) ([]byte, error) {
	return iVaultV2.abi.Pack("setManagementFee", fee, receiver)
}

// PackSetPerformanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc4fef03e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setPerformanceFee(uint96 fee, address receiver) returns()
func (iVaultV2 *IVaultV2) PackSetPerformanceFee(fee *big.Int, receiver common.Address) []byte {
	enc, err := iVaultV2.abi.Pack("setPerformanceFee", fee, receiver)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetPerformanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc4fef03e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setPerformanceFee(uint96 fee, address receiver) returns()
func (iVaultV2 *IVaultV2) TryPackSetPerformanceFee(fee *big.Int, receiver common.Address) ([]byte, error) {
	return iVaultV2.abi.Pack("setPerformanceFee", fee, receiver)
}

// PackSetSlasher is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaabc2496.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setSlasher(address slasher) returns()
func (iVaultV2 *IVaultV2) PackSetSlasher(slasher common.Address) []byte {
	enc, err := iVaultV2.abi.Pack("setSlasher", slasher)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetSlasher is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaabc2496.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setSlasher(address slasher) returns()
func (iVaultV2 *IVaultV2) TryPackSetSlasher(slasher common.Address) ([]byte, error) {
	return iVaultV2.abi.Pack("setSlasher", slasher)
}

// PackTotalSupplyAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe9efd4cf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalSupplyAt(uint48 timestamp) view returns(uint256 supply)
func (iVaultV2 *IVaultV2) PackTotalSupplyAt(timestamp *big.Int) []byte {
	enc, err := iVaultV2.abi.Pack("totalSupplyAt", timestamp)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTotalSupplyAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe9efd4cf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function totalSupplyAt(uint48 timestamp) view returns(uint256 supply)
func (iVaultV2 *IVaultV2) TryPackTotalSupplyAt(timestamp *big.Int) ([]byte, error) {
	return iVaultV2.abi.Pack("totalSupplyAt", timestamp)
}

// UnpackTotalSupplyAt is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xe9efd4cf.
//
// Solidity: function totalSupplyAt(uint48 timestamp) view returns(uint256 supply)
func (iVaultV2 *IVaultV2) UnpackTotalSupplyAt(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("totalSupplyAt", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function version() view returns(uint64)
func (iVaultV2 *IVaultV2) PackVersion() []byte {
	enc, err := iVaultV2.abi.Pack("version")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function version() view returns(uint64)
func (iVaultV2 *IVaultV2) TryPackVersion() ([]byte, error) {
	return iVaultV2.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (iVaultV2 *IVaultV2) UnpackVersion(data []byte) (uint64, error) {
	out, err := iVaultV2.abi.Unpack("version", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// PackWithdrawable is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x50188301.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function withdrawable() returns(uint256 assets)
func (iVaultV2 *IVaultV2) PackWithdrawable() []byte {
	enc, err := iVaultV2.abi.Pack("withdrawable")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackWithdrawable is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x50188301.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function withdrawable() returns(uint256 assets)
func (iVaultV2 *IVaultV2) TryPackWithdrawable() ([]byte, error) {
	return iVaultV2.abi.Pack("withdrawable")
}

// UnpackWithdrawable is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x50188301.
//
// Solidity: function withdrawable() returns(uint256 assets)
func (iVaultV2 *IVaultV2) UnpackWithdrawable(data []byte) (*big.Int, error) {
	out, err := iVaultV2.abi.Unpack("withdrawable", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackWithdrawalQueue is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x37d5fe99.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function withdrawalQueue() view returns(address withdrawalQueueAddress)
func (iVaultV2 *IVaultV2) PackWithdrawalQueue() []byte {
	enc, err := iVaultV2.abi.Pack("withdrawalQueue")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackWithdrawalQueue is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x37d5fe99.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function withdrawalQueue() view returns(address withdrawalQueueAddress)
func (iVaultV2 *IVaultV2) TryPackWithdrawalQueue() ([]byte, error) {
	return iVaultV2.abi.Pack("withdrawalQueue")
}

// UnpackWithdrawalQueue is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x37d5fe99.
//
// Solidity: function withdrawalQueue() view returns(address withdrawalQueueAddress)
func (iVaultV2 *IVaultV2) UnpackWithdrawalQueue(data []byte) (common.Address, error) {
	out, err := iVaultV2.abi.Unpack("withdrawalQueue", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// IVaultV2AccrueInterest represents a AccrueInterest event raised by the IVaultV2 contract.
type IVaultV2AccrueInterest struct {
	NewTotalAssets       *big.Int
	ManagementFeeShares  *big.Int
	PerformanceFeeShares *big.Int
	ProtocolFeeShares    *big.Int
	Raw                  *types.Log // Blockchain specific contextual infos
}

const IVaultV2AccrueInterestEventName = "AccrueInterest"

// ContractEventName returns the user-defined event name.
func (IVaultV2AccrueInterest) ContractEventName() string {
	return IVaultV2AccrueInterestEventName
}

// UnpackAccrueInterestEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event AccrueInterest(uint256 newTotalAssets, uint256 managementFeeShares, uint256 performanceFeeShares, uint256 protocolFeeShares)
func (iVaultV2 *IVaultV2) UnpackAccrueInterestEvent(log *types.Log) (*IVaultV2AccrueInterest, error) {
	event := "AccrueInterest"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2AccrueInterest)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2Claim represents a Claim event raised by the IVaultV2 contract.
type IVaultV2Claim struct {
	Claimer  common.Address
	Receiver common.Address
	TokenId  *big.Int
	Assets   *big.Int
	Raw      *types.Log // Blockchain specific contextual infos
}

const IVaultV2ClaimEventName = "Claim"

// ContractEventName returns the user-defined event name.
func (IVaultV2Claim) ContractEventName() string {
	return IVaultV2ClaimEventName
}

// UnpackClaimEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Claim(address indexed claimer, address indexed receiver, uint256 tokenId, uint256 assets)
func (iVaultV2 *IVaultV2) UnpackClaimEvent(log *types.Log) (*IVaultV2Claim, error) {
	event := "Claim"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2Claim)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2Initialize represents a Initialize event raised by the IVaultV2 contract.
type IVaultV2Initialize struct {
	Params IVaultV2InitParams
	Raw    *types.Log // Blockchain specific contextual infos
}

const IVaultV2InitializeEventName = "Initialize"

// ContractEventName returns the user-defined event name.
func (IVaultV2Initialize) ContractEventName() string {
	return IVaultV2InitializeEventName
}

// UnpackInitializeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialize((string,string,address,bool,address,uint256,bool,address,address,address,address,address,address,address) params)
func (iVaultV2 *IVaultV2) UnpackInitializeEvent(log *types.Log) (*IVaultV2Initialize, error) {
	event := "Initialize"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2Initialize)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2Pull represents a Pull event raised by the IVaultV2 contract.
type IVaultV2Pull struct {
	Assets   *big.Int
	Receiver common.Address
	Raw      *types.Log // Blockchain specific contextual infos
}

const IVaultV2PullEventName = "Pull"

// ContractEventName returns the user-defined event name.
func (IVaultV2Pull) ContractEventName() string {
	return IVaultV2PullEventName
}

// UnpackPullEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Pull(uint256 assets, address indexed receiver)
func (iVaultV2 *IVaultV2) UnpackPullEvent(log *types.Log) (*IVaultV2Pull, error) {
	event := "Pull"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2Pull)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2Push represents a Push event raised by the IVaultV2 contract.
type IVaultV2Push struct {
	Assets *big.Int
	Owner  common.Address
	Raw    *types.Log // Blockchain specific contextual infos
}

const IVaultV2PushEventName = "Push"

// ContractEventName returns the user-defined event name.
func (IVaultV2Push) ContractEventName() string {
	return IVaultV2PushEventName
}

// UnpackPushEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Push(uint256 assets, address indexed owner)
func (iVaultV2 *IVaultV2) UnpackPushEvent(log *types.Log) (*IVaultV2Push, error) {
	event := "Push"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2Push)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetDelegator represents a SetDelegator event raised by the IVaultV2 contract.
type IVaultV2SetDelegator struct {
	Delegator common.Address
	Raw       *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetDelegatorEventName = "SetDelegator"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetDelegator) ContractEventName() string {
	return IVaultV2SetDelegatorEventName
}

// UnpackSetDelegatorEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetDelegator(address indexed delegator)
func (iVaultV2 *IVaultV2) UnpackSetDelegatorEvent(log *types.Log) (*IVaultV2SetDelegator, error) {
	event := "SetDelegator"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetDelegator)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetDepositLimit represents a SetDepositLimit event raised by the IVaultV2 contract.
type IVaultV2SetDepositLimit struct {
	Limit *big.Int
	Raw   *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetDepositLimitEventName = "SetDepositLimit"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetDepositLimit) ContractEventName() string {
	return IVaultV2SetDepositLimitEventName
}

// UnpackSetDepositLimitEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetDepositLimit(uint256 limit)
func (iVaultV2 *IVaultV2) UnpackSetDepositLimitEvent(log *types.Log) (*IVaultV2SetDepositLimit, error) {
	event := "SetDepositLimit"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetDepositLimit)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetDepositWhitelist represents a SetDepositWhitelist event raised by the IVaultV2 contract.
type IVaultV2SetDepositWhitelist struct {
	Status bool
	Raw    *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetDepositWhitelistEventName = "SetDepositWhitelist"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetDepositWhitelist) ContractEventName() string {
	return IVaultV2SetDepositWhitelistEventName
}

// UnpackSetDepositWhitelistEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetDepositWhitelist(bool status)
func (iVaultV2 *IVaultV2) UnpackSetDepositWhitelistEvent(log *types.Log) (*IVaultV2SetDepositWhitelist, error) {
	event := "SetDepositWhitelist"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetDepositWhitelist)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetDepositorWhitelistStatus represents a SetDepositorWhitelistStatus event raised by the IVaultV2 contract.
type IVaultV2SetDepositorWhitelistStatus struct {
	Account common.Address
	Status  bool
	Raw     *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetDepositorWhitelistStatusEventName = "SetDepositorWhitelistStatus"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetDepositorWhitelistStatus) ContractEventName() string {
	return IVaultV2SetDepositorWhitelistStatusEventName
}

// UnpackSetDepositorWhitelistStatusEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetDepositorWhitelistStatus(address indexed account, bool status)
func (iVaultV2 *IVaultV2) UnpackSetDepositorWhitelistStatusEvent(log *types.Log) (*IVaultV2SetDepositorWhitelistStatus, error) {
	event := "SetDepositorWhitelistStatus"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetDepositorWhitelistStatus)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetIsDepositLimit represents a SetIsDepositLimit event raised by the IVaultV2 contract.
type IVaultV2SetIsDepositLimit struct {
	Status bool
	Raw    *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetIsDepositLimitEventName = "SetIsDepositLimit"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetIsDepositLimit) ContractEventName() string {
	return IVaultV2SetIsDepositLimitEventName
}

// UnpackSetIsDepositLimitEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetIsDepositLimit(bool status)
func (iVaultV2 *IVaultV2) UnpackSetIsDepositLimitEvent(log *types.Log) (*IVaultV2SetIsDepositLimit, error) {
	event := "SetIsDepositLimit"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetIsDepositLimit)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetManagementFee represents a SetManagementFee event raised by the IVaultV2 contract.
type IVaultV2SetManagementFee struct {
	Fee      *big.Int
	Receiver common.Address
	Raw      *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetManagementFeeEventName = "SetManagementFee"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetManagementFee) ContractEventName() string {
	return IVaultV2SetManagementFeeEventName
}

// UnpackSetManagementFeeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetManagementFee(uint256 fee, address indexed receiver)
func (iVaultV2 *IVaultV2) UnpackSetManagementFeeEvent(log *types.Log) (*IVaultV2SetManagementFee, error) {
	event := "SetManagementFee"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetManagementFee)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetPerformanceFee represents a SetPerformanceFee event raised by the IVaultV2 contract.
type IVaultV2SetPerformanceFee struct {
	Fee      *big.Int
	Receiver common.Address
	Raw      *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetPerformanceFeeEventName = "SetPerformanceFee"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetPerformanceFee) ContractEventName() string {
	return IVaultV2SetPerformanceFeeEventName
}

// UnpackSetPerformanceFeeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetPerformanceFee(uint256 fee, address indexed receiver)
func (iVaultV2 *IVaultV2) UnpackSetPerformanceFeeEvent(log *types.Log) (*IVaultV2SetPerformanceFee, error) {
	event := "SetPerformanceFee"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetPerformanceFee)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2SetWithdrawalQueue represents a SetWithdrawalQueue event raised by the IVaultV2 contract.
type IVaultV2SetWithdrawalQueue struct {
	WithdrawalQueue common.Address
	Raw             *types.Log // Blockchain specific contextual infos
}

const IVaultV2SetWithdrawalQueueEventName = "SetWithdrawalQueue"

// ContractEventName returns the user-defined event name.
func (IVaultV2SetWithdrawalQueue) ContractEventName() string {
	return IVaultV2SetWithdrawalQueueEventName
}

// UnpackSetWithdrawalQueueEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetWithdrawalQueue(address indexed withdrawalQueue)
func (iVaultV2 *IVaultV2) UnpackSetWithdrawalQueueEvent(log *types.Log) (*IVaultV2SetWithdrawalQueue, error) {
	event := "SetWithdrawalQueue"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2SetWithdrawalQueue)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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

// IVaultV2UpdateProtocolFee represents a UpdateProtocolFee event raised by the IVaultV2 contract.
type IVaultV2UpdateProtocolFee struct {
	Receiver       common.Address
	ManagementFee  *big.Int
	PerformanceFee *big.Int
	Raw            *types.Log // Blockchain specific contextual infos
}

const IVaultV2UpdateProtocolFeeEventName = "UpdateProtocolFee"

// ContractEventName returns the user-defined event name.
func (IVaultV2UpdateProtocolFee) ContractEventName() string {
	return IVaultV2UpdateProtocolFeeEventName
}

// UnpackUpdateProtocolFeeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event UpdateProtocolFee(address indexed receiver, uint96 managementFee, uint96 performanceFee)
func (iVaultV2 *IVaultV2) UnpackUpdateProtocolFeeEvent(log *types.Log) (*IVaultV2UpdateProtocolFee, error) {
	event := "UpdateProtocolFee"
	if log.Topics[0] != iVaultV2.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IVaultV2UpdateProtocolFee)
	if len(log.Data) > 0 {
		if err := iVaultV2.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iVaultV2.abi.Events[event].Inputs {
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
func (iVaultV2 *IVaultV2) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["AlreadyInitialized"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["DelegatorAlreadyInitialized"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackDelegatorAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["DepositLimitReached"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackDepositLimitReachedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["FeeTooHigh"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackFeeTooHighError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["InsufficientFreeAssets"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackInsufficientFreeAssetsError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["InvalidAddress"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackInvalidAddressError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["InvalidDelegator"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackInvalidDelegatorError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["InvalidDepositorToWhitelist"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackInvalidDepositorToWhitelistError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["NotDelegator"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackNotDelegatorError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["NotFactory"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackNotFactoryError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["NotInitialized"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackNotInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["NotWhitelistedDepositor"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackNotWhitelistedDepositorError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["NotWithdrawalQueue"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackNotWithdrawalQueueError(raw[4:])
	}
	if bytes.Equal(raw[:4], iVaultV2.abi.Errors["PendingWithdrawalQueue"].ID.Bytes()[:4]) {
		return iVaultV2.UnpackPendingWithdrawalQueueError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// IVaultV2AlreadyInitialized represents a AlreadyInitialized error raised by the IVaultV2 contract.
type IVaultV2AlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyInitialized()
func IVaultV2AlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x0dc149f07762891dbcea3fe72770f3d63a1863fc54b2f084e8c59ec476996927")
}

// UnpackAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyInitialized()
func (iVaultV2 *IVaultV2) UnpackAlreadyInitializedError(raw []byte) (*IVaultV2AlreadyInitialized, error) {
	out := new(IVaultV2AlreadyInitialized)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "AlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2DelegatorAlreadyInitialized represents a DelegatorAlreadyInitialized error raised by the IVaultV2 contract.
type IVaultV2DelegatorAlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error DelegatorAlreadyInitialized()
func IVaultV2DelegatorAlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x1380833b91eb492e6b5aa0558eb8adb3a39404629f16e0e8a18a5c798fc1a30a")
}

// UnpackDelegatorAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error DelegatorAlreadyInitialized()
func (iVaultV2 *IVaultV2) UnpackDelegatorAlreadyInitializedError(raw []byte) (*IVaultV2DelegatorAlreadyInitialized, error) {
	out := new(IVaultV2DelegatorAlreadyInitialized)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "DelegatorAlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2DepositLimitReached represents a DepositLimitReached error raised by the IVaultV2 contract.
type IVaultV2DepositLimitReached struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error DepositLimitReached()
func IVaultV2DepositLimitReachedErrorID() common.Hash {
	return common.HexToHash("0x248455798e33b4871de4258bfab3fb4b1bc826e576369d72ee7c613e411d262f")
}

// UnpackDepositLimitReachedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error DepositLimitReached()
func (iVaultV2 *IVaultV2) UnpackDepositLimitReachedError(raw []byte) (*IVaultV2DepositLimitReached, error) {
	out := new(IVaultV2DepositLimitReached)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "DepositLimitReached", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2FeeTooHigh represents a FeeTooHigh error raised by the IVaultV2 contract.
type IVaultV2FeeTooHigh struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error FeeTooHigh()
func IVaultV2FeeTooHighErrorID() common.Hash {
	return common.HexToHash("0xcd4e6167a0147beade9e7daca0e52cd42e992cd9c3dc1dd3ce8a2b6956f53601")
}

// UnpackFeeTooHighError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error FeeTooHigh()
func (iVaultV2 *IVaultV2) UnpackFeeTooHighError(raw []byte) (*IVaultV2FeeTooHigh, error) {
	out := new(IVaultV2FeeTooHigh)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "FeeTooHigh", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2InsufficientFreeAssets represents a InsufficientFreeAssets error raised by the IVaultV2 contract.
type IVaultV2InsufficientFreeAssets struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientFreeAssets()
func IVaultV2InsufficientFreeAssetsErrorID() common.Hash {
	return common.HexToHash("0xaba05e537886d7e0b409f4783c3efa2a53464cb042b0a651f2591b08a2c54f37")
}

// UnpackInsufficientFreeAssetsError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientFreeAssets()
func (iVaultV2 *IVaultV2) UnpackInsufficientFreeAssetsError(raw []byte) (*IVaultV2InsufficientFreeAssets, error) {
	out := new(IVaultV2InsufficientFreeAssets)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "InsufficientFreeAssets", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2InvalidAddress represents a InvalidAddress error raised by the IVaultV2 contract.
type IVaultV2InvalidAddress struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAddress()
func IVaultV2InvalidAddressErrorID() common.Hash {
	return common.HexToHash("0xe6c4247b90bd06996a32d386bb770af9c0018dd1b0ebbb2df2c4499f1eda7b16")
}

// UnpackInvalidAddressError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAddress()
func (iVaultV2 *IVaultV2) UnpackInvalidAddressError(raw []byte) (*IVaultV2InvalidAddress, error) {
	out := new(IVaultV2InvalidAddress)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "InvalidAddress", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2InvalidDelegator represents a InvalidDelegator error raised by the IVaultV2 contract.
type IVaultV2InvalidDelegator struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidDelegator()
func IVaultV2InvalidDelegatorErrorID() common.Hash {
	return common.HexToHash("0xb9f0f171cd404c8c5cc9943d60966d64de0cd7b803c221dea8d1f1b85443edad")
}

// UnpackInvalidDelegatorError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidDelegator()
func (iVaultV2 *IVaultV2) UnpackInvalidDelegatorError(raw []byte) (*IVaultV2InvalidDelegator, error) {
	out := new(IVaultV2InvalidDelegator)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "InvalidDelegator", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2InvalidDepositorToWhitelist represents a InvalidDepositorToWhitelist error raised by the IVaultV2 contract.
type IVaultV2InvalidDepositorToWhitelist struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidDepositorToWhitelist()
func IVaultV2InvalidDepositorToWhitelistErrorID() common.Hash {
	return common.HexToHash("0xd557010b8f00d05b95a691b53400d8e83d97be2263cd0f96c30204ebaf946fa2")
}

// UnpackInvalidDepositorToWhitelistError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidDepositorToWhitelist()
func (iVaultV2 *IVaultV2) UnpackInvalidDepositorToWhitelistError(raw []byte) (*IVaultV2InvalidDepositorToWhitelist, error) {
	out := new(IVaultV2InvalidDepositorToWhitelist)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "InvalidDepositorToWhitelist", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2NotDelegator represents a NotDelegator error raised by the IVaultV2 contract.
type IVaultV2NotDelegator struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotDelegator()
func IVaultV2NotDelegatorErrorID() common.Hash {
	return common.HexToHash("0x9396be343041469692e41894a3e201bf6e4c39752d91f0bb319a72474aee37e6")
}

// UnpackNotDelegatorError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotDelegator()
func (iVaultV2 *IVaultV2) UnpackNotDelegatorError(raw []byte) (*IVaultV2NotDelegator, error) {
	out := new(IVaultV2NotDelegator)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "NotDelegator", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2NotFactory represents a NotFactory error raised by the IVaultV2 contract.
type IVaultV2NotFactory struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotFactory()
func IVaultV2NotFactoryErrorID() common.Hash {
	return common.HexToHash("0x32cc723614e775fc4a8386492bc9a860c12fe98d5f5f28ec17e265818645b229")
}

// UnpackNotFactoryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotFactory()
func (iVaultV2 *IVaultV2) UnpackNotFactoryError(raw []byte) (*IVaultV2NotFactory, error) {
	out := new(IVaultV2NotFactory)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "NotFactory", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2NotInitialized represents a NotInitialized error raised by the IVaultV2 contract.
type IVaultV2NotInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitialized()
func IVaultV2NotInitializedErrorID() common.Hash {
	return common.HexToHash("0x87138d5c8c2e77cb9f25c07b03277aad63d22f6a05255580ec55d2c21666e734")
}

// UnpackNotInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitialized()
func (iVaultV2 *IVaultV2) UnpackNotInitializedError(raw []byte) (*IVaultV2NotInitialized, error) {
	out := new(IVaultV2NotInitialized)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "NotInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2NotWhitelistedDepositor represents a NotWhitelistedDepositor error raised by the IVaultV2 contract.
type IVaultV2NotWhitelistedDepositor struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotWhitelistedDepositor()
func IVaultV2NotWhitelistedDepositorErrorID() common.Hash {
	return common.HexToHash("0x04f63b859fbe057953731e40d2f69e085f6b681f5eb8e6be9f6bd18ecf911e5a")
}

// UnpackNotWhitelistedDepositorError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotWhitelistedDepositor()
func (iVaultV2 *IVaultV2) UnpackNotWhitelistedDepositorError(raw []byte) (*IVaultV2NotWhitelistedDepositor, error) {
	out := new(IVaultV2NotWhitelistedDepositor)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "NotWhitelistedDepositor", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2NotWithdrawalQueue represents a NotWithdrawalQueue error raised by the IVaultV2 contract.
type IVaultV2NotWithdrawalQueue struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotWithdrawalQueue()
func IVaultV2NotWithdrawalQueueErrorID() common.Hash {
	return common.HexToHash("0x915d3c9750f2be5afdfe0b35297484b6c420d6c9aa6da35ec431420bd7311ee8")
}

// UnpackNotWithdrawalQueueError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotWithdrawalQueue()
func (iVaultV2 *IVaultV2) UnpackNotWithdrawalQueueError(raw []byte) (*IVaultV2NotWithdrawalQueue, error) {
	out := new(IVaultV2NotWithdrawalQueue)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "NotWithdrawalQueue", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IVaultV2PendingWithdrawalQueue represents a PendingWithdrawalQueue error raised by the IVaultV2 contract.
type IVaultV2PendingWithdrawalQueue struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error PendingWithdrawalQueue()
func IVaultV2PendingWithdrawalQueueErrorID() common.Hash {
	return common.HexToHash("0xe7eeef015b5c57f1cbb7ab6ca8574d1b5aa7489fb197a2ebae848e67e185ffdc")
}

// UnpackPendingWithdrawalQueueError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error PendingWithdrawalQueue()
func (iVaultV2 *IVaultV2) UnpackPendingWithdrawalQueueError(raw []byte) (*IVaultV2PendingWithdrawalQueue, error) {
	out := new(IVaultV2PendingWithdrawalQueue)
	if err := iVaultV2.abi.UnpackIntoInterface(out, "PendingWithdrawalQueue", raw); err != nil {
		return nil, err
	}
	return out, nil
}
