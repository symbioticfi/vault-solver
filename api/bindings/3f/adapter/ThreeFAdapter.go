// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package adapter

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

// Offer is an auto generated low-level Go binding around an user-defined struct.
type Offer struct {
	Maker          common.Address
	Amount         *big.Int
	ExpectedReturn *big.Int
	Nonce          *big.Int
	Expiration     *big.Int
	UseCallback    bool
}

// ThreeFAdapterMetaData contains all meta data concerning the ThreeFAdapter contract.
var ThreeFAdapterMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"requestWhitelist\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"adapterFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vaultFactory\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"REQUEST_WHITELIST\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeLoans\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeRequests\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"deallocated\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isRequest\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isValidSignature\",\"inputs\":[{\"name\":\"hash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"maxConcurrentLoans\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minRequestYield\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"offerSigner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"onRequestConsumed\",\"inputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structOffer\",\"components\":[{\"name\":\"maker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedReturn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expiration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"useCallback\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"principal\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yieldAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"outstandingPrincipal\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"perRequestMaxCollateral\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"positions\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"principal\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"ytExpected\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"openedAt\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"redeemed\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"realizedPrincipal\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"redeem\",\"inputs\":[{\"name\":\"requests\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestDeallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setExposureLimits\",\"inputs\":[{\"name\":\"perRequestMaxCollateral_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minRequestYield_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxConcurrentLoans_\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setOfferSigner\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"staticDelegateCall\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"vault\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PositionOpened\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"principal\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"ytExpected\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PositionRedeemed\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"principal\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"yieldAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetExposureLimits\",\"inputs\":[{\"name\":\"perRequestMaxCollateral\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"minRequestYield\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"maxConcurrentLoans\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetOfferSigner\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetVault\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AssetMismatch\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientLiquidity\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotAttested\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"PerRequestCapExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"TooManyLoans\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"YieldTooLow\",\"inputs\":[]}]",
	ID:  "ThreeFAdapter",
}

// ThreeFAdapter is an auto generated Go binding around an Ethereum contract.
type ThreeFAdapter struct {
	abi abi.ABI
}

// NewThreeFAdapter creates a new instance of ThreeFAdapter.
func NewThreeFAdapter() *ThreeFAdapter {
	parsed, err := ThreeFAdapterMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &ThreeFAdapter{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *ThreeFAdapter) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address requestWhitelist, address adapterFactory, address vaultFactory) returns()
func (threeFAdapter *ThreeFAdapter) PackConstructor(requestWhitelist common.Address, adapterFactory common.Address, vaultFactory common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("", requestWhitelist, adapterFactory, vaultFactory)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackFACTORY is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dd31000.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function FACTORY() view returns(address)
func (threeFAdapter *ThreeFAdapter) PackFACTORY() []byte {
	enc, err := threeFAdapter.abi.Pack("FACTORY")
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
func (threeFAdapter *ThreeFAdapter) TryPackFACTORY() ([]byte, error) {
	return threeFAdapter.abi.Pack("FACTORY")
}

// UnpackFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (threeFAdapter *ThreeFAdapter) UnpackFACTORY(data []byte) (common.Address, error) {
	out, err := threeFAdapter.abi.Unpack("FACTORY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackREQUESTWHITELIST is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x894e6d61.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function REQUEST_WHITELIST() view returns(address)
func (threeFAdapter *ThreeFAdapter) PackREQUESTWHITELIST() []byte {
	enc, err := threeFAdapter.abi.Pack("REQUEST_WHITELIST")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackREQUESTWHITELIST is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x894e6d61.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function REQUEST_WHITELIST() view returns(address)
func (threeFAdapter *ThreeFAdapter) TryPackREQUESTWHITELIST() ([]byte, error) {
	return threeFAdapter.abi.Pack("REQUEST_WHITELIST")
}

// UnpackREQUESTWHITELIST is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x894e6d61.
//
// Solidity: function REQUEST_WHITELIST() view returns(address)
func (threeFAdapter *ThreeFAdapter) UnpackREQUESTWHITELIST(data []byte) (common.Address, error) {
	out, err := threeFAdapter.abi.Unpack("REQUEST_WHITELIST", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackActiveLoans is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x416b40c7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function activeLoans() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackActiveLoans() []byte {
	enc, err := threeFAdapter.abi.Pack("activeLoans")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackActiveLoans is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x416b40c7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function activeLoans() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackActiveLoans() ([]byte, error) {
	return threeFAdapter.abi.Pack("activeLoans")
}

// UnpackActiveLoans is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x416b40c7.
//
// Solidity: function activeLoans() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackActiveLoans(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("activeLoans", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackActiveRequests is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x83cc915c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function activeRequests() view returns(address[])
func (threeFAdapter *ThreeFAdapter) PackActiveRequests() []byte {
	enc, err := threeFAdapter.abi.Pack("activeRequests")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackActiveRequests is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x83cc915c.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function activeRequests() view returns(address[])
func (threeFAdapter *ThreeFAdapter) TryPackActiveRequests() ([]byte, error) {
	return threeFAdapter.abi.Pack("activeRequests")
}

// UnpackActiveRequests is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x83cc915c.
//
// Solidity: function activeRequests() view returns(address[])
func (threeFAdapter *ThreeFAdapter) UnpackActiveRequests(data []byte) ([]common.Address, error) {
	out, err := threeFAdapter.abi.Unpack("activeRequests", data)
	if err != nil {
		return *new([]common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)
	return out0, nil
}

// PackAllocatable is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1d3b809a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function allocatable() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackAllocatable() []byte {
	enc, err := threeFAdapter.abi.Pack("allocatable")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAllocatable is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1d3b809a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function allocatable() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackAllocatable() ([]byte, error) {
	return threeFAdapter.abi.Pack("allocatable")
}

// UnpackAllocatable is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackAllocatable(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("allocatable", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackAllocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x90ca796b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackAllocate(amount *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("allocate", amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAllocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x90ca796b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackAllocate(amount *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("allocate", amount)
}

// UnpackAllocate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackAllocate(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("allocate", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackDeallocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6f6c441f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function deallocate(uint256 amount) returns(uint256 deallocated)
func (threeFAdapter *ThreeFAdapter) PackDeallocate(amount *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("deallocate", amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDeallocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6f6c441f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function deallocate(uint256 amount) returns(uint256 deallocated)
func (threeFAdapter *ThreeFAdapter) TryPackDeallocate(amount *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("deallocate", amount)
}

// UnpackDeallocate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256 deallocated)
func (threeFAdapter *ThreeFAdapter) UnpackDeallocate(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("deallocate", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackFreeAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x11f240ac.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function freeAssets() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackFreeAssets() []byte {
	enc, err := threeFAdapter.abi.Pack("freeAssets")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFreeAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x11f240ac.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function freeAssets() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackFreeAssets() ([]byte, error) {
	return threeFAdapter.abi.Pack("freeAssets")
}

// UnpackFreeAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackFreeAssets(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("freeAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (threeFAdapter *ThreeFAdapter) PackInitialize(initialVersion uint64, owner common.Address, data []byte) []byte {
	enc, err := threeFAdapter.abi.Pack("initialize", initialVersion, owner, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x57ec83cc.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (threeFAdapter *ThreeFAdapter) TryPackInitialize(initialVersion uint64, owner common.Address, data []byte) ([]byte, error) {
	return threeFAdapter.abi.Pack("initialize", initialVersion, owner, data)
}

// PackIsRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe9adfc20.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isRequest(address request) view returns(bool)
func (threeFAdapter *ThreeFAdapter) PackIsRequest(request common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("isRequest", request)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe9adfc20.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isRequest(address request) view returns(bool)
func (threeFAdapter *ThreeFAdapter) TryPackIsRequest(request common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("isRequest", request)
}

// UnpackIsRequest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xe9adfc20.
//
// Solidity: function isRequest(address request) view returns(bool)
func (threeFAdapter *ThreeFAdapter) UnpackIsRequest(data []byte) (bool, error) {
	out, err := threeFAdapter.abi.Unpack("isRequest", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackIsValidSignature is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1626ba7e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (threeFAdapter *ThreeFAdapter) PackIsValidSignature(hash [32]byte, signature []byte) []byte {
	enc, err := threeFAdapter.abi.Pack("isValidSignature", hash, signature)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsValidSignature is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1626ba7e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (threeFAdapter *ThreeFAdapter) TryPackIsValidSignature(hash [32]byte, signature []byte) ([]byte, error) {
	return threeFAdapter.abi.Pack("isValidSignature", hash, signature)
}

// UnpackIsValidSignature is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (threeFAdapter *ThreeFAdapter) UnpackIsValidSignature(data []byte) ([4]byte, error) {
	out, err := threeFAdapter.abi.Unpack("isValidSignature", data)
	if err != nil {
		return *new([4]byte), err
	}
	out0 := *abi.ConvertType(out[0], new([4]byte)).(*[4]byte)
	return out0, nil
}

// PackMaxConcurrentLoans is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0fa715c7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function maxConcurrentLoans() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackMaxConcurrentLoans() []byte {
	enc, err := threeFAdapter.abi.Pack("maxConcurrentLoans")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMaxConcurrentLoans is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0fa715c7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function maxConcurrentLoans() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackMaxConcurrentLoans() ([]byte, error) {
	return threeFAdapter.abi.Pack("maxConcurrentLoans")
}

// UnpackMaxConcurrentLoans is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0fa715c7.
//
// Solidity: function maxConcurrentLoans() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackMaxConcurrentLoans(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("maxConcurrentLoans", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2abe3048.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (threeFAdapter *ThreeFAdapter) PackMigrate(newVersion uint64, data []byte) []byte {
	enc, err := threeFAdapter.abi.Pack("migrate", newVersion, data)
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
func (threeFAdapter *ThreeFAdapter) TryPackMigrate(newVersion uint64, data []byte) ([]byte, error) {
	return threeFAdapter.abi.Pack("migrate", newVersion, data)
}

// PackMinRequestYield is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbc0d16fd.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minRequestYield() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackMinRequestYield() []byte {
	enc, err := threeFAdapter.abi.Pack("minRequestYield")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinRequestYield is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbc0d16fd.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minRequestYield() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackMinRequestYield() ([]byte, error) {
	return threeFAdapter.abi.Pack("minRequestYield")
}

// UnpackMinRequestYield is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xbc0d16fd.
//
// Solidity: function minRequestYield() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackMinRequestYield(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("minRequestYield", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMulticall is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xac9650d8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function multicall(bytes[] data) returns()
func (threeFAdapter *ThreeFAdapter) PackMulticall(data [][]byte) []byte {
	enc, err := threeFAdapter.abi.Pack("multicall", data)
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
func (threeFAdapter *ThreeFAdapter) TryPackMulticall(data [][]byte) ([]byte, error) {
	return threeFAdapter.abi.Pack("multicall", data)
}

// PackOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x566bd6c3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function offerSigner() view returns(address)
func (threeFAdapter *ThreeFAdapter) PackOfferSigner() []byte {
	enc, err := threeFAdapter.abi.Pack("offerSigner")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x566bd6c3.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function offerSigner() view returns(address)
func (threeFAdapter *ThreeFAdapter) TryPackOfferSigner() ([]byte, error) {
	return threeFAdapter.abi.Pack("offerSigner")
}

// UnpackOfferSigner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x566bd6c3.
//
// Solidity: function offerSigner() view returns(address)
func (threeFAdapter *ThreeFAdapter) UnpackOfferSigner(data []byte) (common.Address, error) {
	out, err := threeFAdapter.abi.Unpack("offerSigner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackOnRequestConsumed is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fe1357.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) , bytes , uint256 principal, uint256 yieldAmount) returns()
func (threeFAdapter *ThreeFAdapter) PackOnRequestConsumed(arg0 Offer, arg1 []byte, principal *big.Int, yieldAmount *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("onRequestConsumed", arg0, arg1, principal, yieldAmount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOnRequestConsumed is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fe1357.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) , bytes , uint256 principal, uint256 yieldAmount) returns()
func (threeFAdapter *ThreeFAdapter) TryPackOnRequestConsumed(arg0 Offer, arg1 []byte, principal *big.Int, yieldAmount *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("onRequestConsumed", arg0, arg1, principal, yieldAmount)
}

// PackOutstandingPrincipal is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x29b1829e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackOutstandingPrincipal() []byte {
	enc, err := threeFAdapter.abi.Pack("outstandingPrincipal")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOutstandingPrincipal is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x29b1829e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackOutstandingPrincipal() ([]byte, error) {
	return threeFAdapter.abi.Pack("outstandingPrincipal")
}

// UnpackOutstandingPrincipal is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x29b1829e.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackOutstandingPrincipal(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("outstandingPrincipal", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address)
func (threeFAdapter *ThreeFAdapter) PackOwner() []byte {
	enc, err := threeFAdapter.abi.Pack("owner")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function owner() view returns(address)
func (threeFAdapter *ThreeFAdapter) TryPackOwner() ([]byte, error) {
	return threeFAdapter.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (threeFAdapter *ThreeFAdapter) UnpackOwner(data []byte) (common.Address, error) {
	out, err := threeFAdapter.abi.Unpack("owner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackPerRequestMaxCollateral is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca1f1576.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function perRequestMaxCollateral() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackPerRequestMaxCollateral() []byte {
	enc, err := threeFAdapter.abi.Pack("perRequestMaxCollateral")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPerRequestMaxCollateral is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca1f1576.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function perRequestMaxCollateral() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackPerRequestMaxCollateral() ([]byte, error) {
	return threeFAdapter.abi.Pack("perRequestMaxCollateral")
}

// UnpackPerRequestMaxCollateral is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xca1f1576.
//
// Solidity: function perRequestMaxCollateral() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackPerRequestMaxCollateral(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("perRequestMaxCollateral", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackPositions is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x55f57510.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function positions(address request) view returns(uint256 principal, uint256 ytExpected, uint48 openedAt, bool redeemed)
func (threeFAdapter *ThreeFAdapter) PackPositions(request common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("positions", request)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPositions is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x55f57510.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function positions(address request) view returns(uint256 principal, uint256 ytExpected, uint48 openedAt, bool redeemed)
func (threeFAdapter *ThreeFAdapter) TryPackPositions(request common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("positions", request)
}

// PositionsOutput serves as a container for the return parameters of contract
// method Positions.
type PositionsOutput struct {
	Principal  *big.Int
	YtExpected *big.Int
	OpenedAt   *big.Int
	Redeemed   bool
}

// UnpackPositions is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x55f57510.
//
// Solidity: function positions(address request) view returns(uint256 principal, uint256 ytExpected, uint48 openedAt, bool redeemed)
func (threeFAdapter *ThreeFAdapter) UnpackPositions(data []byte) (PositionsOutput, error) {
	out, err := threeFAdapter.abi.Unpack("positions", data)
	outstruct := new(PositionsOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.Principal = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.YtExpected = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.OpenedAt = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.Redeemed = *abi.ConvertType(out[3], new(bool)).(*bool)
	return *outstruct, nil
}

// PackRealizedPrincipal is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5b348b1f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function realizedPrincipal() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackRealizedPrincipal() []byte {
	enc, err := threeFAdapter.abi.Pack("realizedPrincipal")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRealizedPrincipal is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5b348b1f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function realizedPrincipal() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackRealizedPrincipal() ([]byte, error) {
	return threeFAdapter.abi.Pack("realizedPrincipal")
}

// UnpackRealizedPrincipal is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5b348b1f.
//
// Solidity: function realizedPrincipal() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackRealizedPrincipal(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("realizedPrincipal", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8730b205.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function redeem(address[] requests) returns()
func (threeFAdapter *ThreeFAdapter) PackRedeem(requests []common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("redeem", requests)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8730b205.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function redeem(address[] requests) returns()
func (threeFAdapter *ThreeFAdapter) TryPackRedeem(requests []common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("redeem", requests)
}

// PackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function renounceOwnership() returns()
func (threeFAdapter *ThreeFAdapter) PackRenounceOwnership() []byte {
	enc, err := threeFAdapter.abi.Pack("renounceOwnership")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function renounceOwnership() returns()
func (threeFAdapter *ThreeFAdapter) TryPackRenounceOwnership() ([]byte, error) {
	return threeFAdapter.abi.Pack("renounceOwnership")
}

// PackRequestDeallocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf79f679d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (threeFAdapter *ThreeFAdapter) PackRequestDeallocate(amount *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("requestDeallocate", amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRequestDeallocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf79f679d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (threeFAdapter *ThreeFAdapter) TryPackRequestDeallocate(amount *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("requestDeallocate", amount)
}

// PackSetExposureLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x30f0c89f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setExposureLimits(uint256 perRequestMaxCollateral_, uint256 minRequestYield_, uint256 maxConcurrentLoans_) returns()
func (threeFAdapter *ThreeFAdapter) PackSetExposureLimits(perRequestMaxCollateral *big.Int, minRequestYield *big.Int, maxConcurrentLoans *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("setExposureLimits", perRequestMaxCollateral, minRequestYield, maxConcurrentLoans)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetExposureLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x30f0c89f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setExposureLimits(uint256 perRequestMaxCollateral_, uint256 minRequestYield_, uint256 maxConcurrentLoans_) returns()
func (threeFAdapter *ThreeFAdapter) TryPackSetExposureLimits(perRequestMaxCollateral *big.Int, minRequestYield *big.Int, maxConcurrentLoans *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("setExposureLimits", perRequestMaxCollateral, minRequestYield, maxConcurrentLoans)
}

// PackSetOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x868adcae.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setOfferSigner(address signer) returns()
func (threeFAdapter *ThreeFAdapter) PackSetOfferSigner(signer common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("setOfferSigner", signer)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x868adcae.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setOfferSigner(address signer) returns()
func (threeFAdapter *ThreeFAdapter) TryPackSetOfferSigner(signer common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("setOfferSigner", signer)
}

// PackStaticDelegateCall is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9f86fd85.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function staticDelegateCall(address target, bytes data) returns()
func (threeFAdapter *ThreeFAdapter) PackStaticDelegateCall(target common.Address, data []byte) []byte {
	enc, err := threeFAdapter.abi.Pack("staticDelegateCall", target, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackStaticDelegateCall is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9f86fd85.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function staticDelegateCall(address target, bytes data) returns()
func (threeFAdapter *ThreeFAdapter) TryPackStaticDelegateCall(target common.Address, data []byte) ([]byte, error) {
	return threeFAdapter.abi.Pack("staticDelegateCall", target, data)
}

// PackTotalAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x01e1d114.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalAssets() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackTotalAssets() []byte {
	enc, err := threeFAdapter.abi.Pack("totalAssets")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTotalAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x01e1d114.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function totalAssets() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackTotalAssets() ([]byte, error) {
	return threeFAdapter.abi.Pack("totalAssets")
}

// UnpackTotalAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackTotalAssets(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("totalAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (threeFAdapter *ThreeFAdapter) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("transferOwnership", newOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (threeFAdapter *ThreeFAdapter) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("transferOwnership", newOwner)
}

// PackVault is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfbfa77cf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function vault() view returns(address)
func (threeFAdapter *ThreeFAdapter) PackVault() []byte {
	enc, err := threeFAdapter.abi.Pack("vault")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackVault is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfbfa77cf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function vault() view returns(address)
func (threeFAdapter *ThreeFAdapter) TryPackVault() ([]byte, error) {
	return threeFAdapter.abi.Pack("vault")
}

// UnpackVault is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (threeFAdapter *ThreeFAdapter) UnpackVault(data []byte) (common.Address, error) {
	out, err := threeFAdapter.abi.Unpack("vault", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function version() view returns(uint64)
func (threeFAdapter *ThreeFAdapter) PackVersion() []byte {
	enc, err := threeFAdapter.abi.Pack("version")
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
func (threeFAdapter *ThreeFAdapter) TryPackVersion() ([]byte, error) {
	return threeFAdapter.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (threeFAdapter *ThreeFAdapter) UnpackVersion(data []byte) (uint64, error) {
	out, err := threeFAdapter.abi.Unpack("version", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// ThreeFAdapterInitialized represents a Initialized event raised by the ThreeFAdapter contract.
type ThreeFAdapterInitialized struct {
	Version uint64
	Raw     *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterInitializedEventName = "Initialized"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterInitialized) ContractEventName() string {
	return ThreeFAdapterInitializedEventName
}

// UnpackInitializedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialized(uint64 version)
func (threeFAdapter *ThreeFAdapter) UnpackInitializedEvent(log *types.Log) (*ThreeFAdapterInitialized, error) {
	event := "Initialized"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterInitialized)
	if len(log.Data) > 0 {
		if err := threeFAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range threeFAdapter.abi.Events[event].Inputs {
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

// ThreeFAdapterOwnershipTransferred represents a OwnershipTransferred event raised by the ThreeFAdapter contract.
type ThreeFAdapterOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterOwnershipTransferred) ContractEventName() string {
	return ThreeFAdapterOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (threeFAdapter *ThreeFAdapter) UnpackOwnershipTransferredEvent(log *types.Log) (*ThreeFAdapterOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := threeFAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range threeFAdapter.abi.Events[event].Inputs {
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

// ThreeFAdapterPositionOpened represents a PositionOpened event raised by the ThreeFAdapter contract.
type ThreeFAdapterPositionOpened struct {
	Request    common.Address
	Principal  *big.Int
	YtExpected *big.Int
	Raw        *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterPositionOpenedEventName = "PositionOpened"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterPositionOpened) ContractEventName() string {
	return ThreeFAdapterPositionOpenedEventName
}

// UnpackPositionOpenedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event PositionOpened(address indexed request, uint256 principal, uint256 ytExpected)
func (threeFAdapter *ThreeFAdapter) UnpackPositionOpenedEvent(log *types.Log) (*ThreeFAdapterPositionOpened, error) {
	event := "PositionOpened"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterPositionOpened)
	if len(log.Data) > 0 {
		if err := threeFAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range threeFAdapter.abi.Events[event].Inputs {
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

// ThreeFAdapterPositionRedeemed represents a PositionRedeemed event raised by the ThreeFAdapter contract.
type ThreeFAdapterPositionRedeemed struct {
	Request     common.Address
	Principal   *big.Int
	YieldAmount *big.Int
	Raw         *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterPositionRedeemedEventName = "PositionRedeemed"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterPositionRedeemed) ContractEventName() string {
	return ThreeFAdapterPositionRedeemedEventName
}

// UnpackPositionRedeemedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event PositionRedeemed(address indexed request, uint256 principal, uint256 yieldAmount)
func (threeFAdapter *ThreeFAdapter) UnpackPositionRedeemedEvent(log *types.Log) (*ThreeFAdapterPositionRedeemed, error) {
	event := "PositionRedeemed"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterPositionRedeemed)
	if len(log.Data) > 0 {
		if err := threeFAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range threeFAdapter.abi.Events[event].Inputs {
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

// ThreeFAdapterSetExposureLimits represents a SetExposureLimits event raised by the ThreeFAdapter contract.
type ThreeFAdapterSetExposureLimits struct {
	PerRequestMaxCollateral *big.Int
	MinRequestYield         *big.Int
	MaxConcurrentLoans      *big.Int
	Raw                     *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterSetExposureLimitsEventName = "SetExposureLimits"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterSetExposureLimits) ContractEventName() string {
	return ThreeFAdapterSetExposureLimitsEventName
}

// UnpackSetExposureLimitsEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetExposureLimits(uint256 perRequestMaxCollateral, uint256 minRequestYield, uint256 maxConcurrentLoans)
func (threeFAdapter *ThreeFAdapter) UnpackSetExposureLimitsEvent(log *types.Log) (*ThreeFAdapterSetExposureLimits, error) {
	event := "SetExposureLimits"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterSetExposureLimits)
	if len(log.Data) > 0 {
		if err := threeFAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range threeFAdapter.abi.Events[event].Inputs {
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

// ThreeFAdapterSetOfferSigner represents a SetOfferSigner event raised by the ThreeFAdapter contract.
type ThreeFAdapterSetOfferSigner struct {
	Signer common.Address
	Raw    *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterSetOfferSignerEventName = "SetOfferSigner"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterSetOfferSigner) ContractEventName() string {
	return ThreeFAdapterSetOfferSignerEventName
}

// UnpackSetOfferSignerEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetOfferSigner(address indexed signer)
func (threeFAdapter *ThreeFAdapter) UnpackSetOfferSignerEvent(log *types.Log) (*ThreeFAdapterSetOfferSigner, error) {
	event := "SetOfferSigner"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterSetOfferSigner)
	if len(log.Data) > 0 {
		if err := threeFAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range threeFAdapter.abi.Events[event].Inputs {
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

// ThreeFAdapterSetVault represents a SetVault event raised by the ThreeFAdapter contract.
type ThreeFAdapterSetVault struct {
	Vault common.Address
	Raw   *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterSetVaultEventName = "SetVault"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterSetVault) ContractEventName() string {
	return ThreeFAdapterSetVaultEventName
}

// UnpackSetVaultEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetVault(address indexed vault)
func (threeFAdapter *ThreeFAdapter) UnpackSetVaultEvent(log *types.Log) (*ThreeFAdapterSetVault, error) {
	event := "SetVault"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterSetVault)
	if len(log.Data) > 0 {
		if err := threeFAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range threeFAdapter.abi.Events[event].Inputs {
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
func (threeFAdapter *ThreeFAdapter) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["AlreadyInitialized"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["AssetMismatch"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackAssetMismatchError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["InsufficientLiquidity"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackInsufficientLiquidityError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["InvalidInitialization"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackInvalidInitializationError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["InvalidVault"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackInvalidVaultError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["NotAttested"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackNotAttestedError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["NotFactory"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackNotFactoryError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["NotInitializing"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackNotInitializingError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["NotVault"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackNotVaultError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["OwnableInvalidOwner"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackOwnableInvalidOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["OwnableUnauthorizedAccount"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackOwnableUnauthorizedAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["PerRequestCapExceeded"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackPerRequestCapExceededError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["ReentrancyGuardReentrantCall"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackReentrancyGuardReentrantCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["TooManyLoans"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackTooManyLoansError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["YieldTooLow"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackYieldTooLowError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// ThreeFAdapterAlreadyInitialized represents a AlreadyInitialized error raised by the ThreeFAdapter contract.
type ThreeFAdapterAlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyInitialized()
func ThreeFAdapterAlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x0dc149f07762891dbcea3fe72770f3d63a1863fc54b2f084e8c59ec476996927")
}

// UnpackAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyInitialized()
func (threeFAdapter *ThreeFAdapter) UnpackAlreadyInitializedError(raw []byte) (*ThreeFAdapterAlreadyInitialized, error) {
	out := new(ThreeFAdapterAlreadyInitialized)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "AlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterAssetMismatch represents a AssetMismatch error raised by the ThreeFAdapter contract.
type ThreeFAdapterAssetMismatch struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AssetMismatch()
func ThreeFAdapterAssetMismatchErrorID() common.Hash {
	return common.HexToHash("0x83c1010ad7aa04f27fb612a82818ae1f4e183ffb2c2ce08a49b7b56cdd6dd4fb")
}

// UnpackAssetMismatchError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AssetMismatch()
func (threeFAdapter *ThreeFAdapter) UnpackAssetMismatchError(raw []byte) (*ThreeFAdapterAssetMismatch, error) {
	out := new(ThreeFAdapterAssetMismatch)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "AssetMismatch", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterInsufficientLiquidity represents a InsufficientLiquidity error raised by the ThreeFAdapter contract.
type ThreeFAdapterInsufficientLiquidity struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientLiquidity()
func ThreeFAdapterInsufficientLiquidityErrorID() common.Hash {
	return common.HexToHash("0xbb55fd27c46b5ba9f88ff2cb2222216afeb0f193423b26615497b3020ab61f8e")
}

// UnpackInsufficientLiquidityError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientLiquidity()
func (threeFAdapter *ThreeFAdapter) UnpackInsufficientLiquidityError(raw []byte) (*ThreeFAdapterInsufficientLiquidity, error) {
	out := new(ThreeFAdapterInsufficientLiquidity)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "InsufficientLiquidity", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterInvalidInitialization represents a InvalidInitialization error raised by the ThreeFAdapter contract.
type ThreeFAdapterInvalidInitialization struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidInitialization()
func ThreeFAdapterInvalidInitializationErrorID() common.Hash {
	return common.HexToHash("0xf92ee8a957075833165f68c320933b1a1294aafc84ee6e0dd3fb178008f9aaf5")
}

// UnpackInvalidInitializationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidInitialization()
func (threeFAdapter *ThreeFAdapter) UnpackInvalidInitializationError(raw []byte) (*ThreeFAdapterInvalidInitialization, error) {
	out := new(ThreeFAdapterInvalidInitialization)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "InvalidInitialization", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterInvalidVault represents a InvalidVault error raised by the ThreeFAdapter contract.
type ThreeFAdapterInvalidVault struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidVault()
func ThreeFAdapterInvalidVaultErrorID() common.Hash {
	return common.HexToHash("0xd03a63207f799c8b4a310cf73db481de483ce6543ef24d1f75f918a11e4eae1f")
}

// UnpackInvalidVaultError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidVault()
func (threeFAdapter *ThreeFAdapter) UnpackInvalidVaultError(raw []byte) (*ThreeFAdapterInvalidVault, error) {
	out := new(ThreeFAdapterInvalidVault)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "InvalidVault", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterNotAttested represents a NotAttested error raised by the ThreeFAdapter contract.
type ThreeFAdapterNotAttested struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotAttested()
func ThreeFAdapterNotAttestedErrorID() common.Hash {
	return common.HexToHash("0x99efb89078879e78f0f307145c3360fe4f6680762a21d87392e067610a80f73d")
}

// UnpackNotAttestedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotAttested()
func (threeFAdapter *ThreeFAdapter) UnpackNotAttestedError(raw []byte) (*ThreeFAdapterNotAttested, error) {
	out := new(ThreeFAdapterNotAttested)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "NotAttested", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterNotFactory represents a NotFactory error raised by the ThreeFAdapter contract.
type ThreeFAdapterNotFactory struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotFactory()
func ThreeFAdapterNotFactoryErrorID() common.Hash {
	return common.HexToHash("0x32cc723614e775fc4a8386492bc9a860c12fe98d5f5f28ec17e265818645b229")
}

// UnpackNotFactoryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotFactory()
func (threeFAdapter *ThreeFAdapter) UnpackNotFactoryError(raw []byte) (*ThreeFAdapterNotFactory, error) {
	out := new(ThreeFAdapterNotFactory)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "NotFactory", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterNotInitializing represents a NotInitializing error raised by the ThreeFAdapter contract.
type ThreeFAdapterNotInitializing struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitializing()
func ThreeFAdapterNotInitializingErrorID() common.Hash {
	return common.HexToHash("0xd7e6bcf8597daa127dc9f0048d2f08d5ef140a2cb659feabd700beff1f7a8302")
}

// UnpackNotInitializingError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitializing()
func (threeFAdapter *ThreeFAdapter) UnpackNotInitializingError(raw []byte) (*ThreeFAdapterNotInitializing, error) {
	out := new(ThreeFAdapterNotInitializing)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "NotInitializing", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterNotVault represents a NotVault error raised by the ThreeFAdapter contract.
type ThreeFAdapterNotVault struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotVault()
func ThreeFAdapterNotVaultErrorID() common.Hash {
	return common.HexToHash("0x62df0545b0e47f06f6a9990975121b8c49c83a96f18696393f66a69dd2ffe568")
}

// UnpackNotVaultError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotVault()
func (threeFAdapter *ThreeFAdapter) UnpackNotVaultError(raw []byte) (*ThreeFAdapterNotVault, error) {
	out := new(ThreeFAdapterNotVault)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "NotVault", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterOwnableInvalidOwner represents a OwnableInvalidOwner error raised by the ThreeFAdapter contract.
type ThreeFAdapterOwnableInvalidOwner struct {
	Owner common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableInvalidOwner(address owner)
func ThreeFAdapterOwnableInvalidOwnerErrorID() common.Hash {
	return common.HexToHash("0x1e4fbdf7f3ef8bcaa855599e3abf48b232380f183f08f6f813d9ffa5bd585188")
}

// UnpackOwnableInvalidOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableInvalidOwner(address owner)
func (threeFAdapter *ThreeFAdapter) UnpackOwnableInvalidOwnerError(raw []byte) (*ThreeFAdapterOwnableInvalidOwner, error) {
	out := new(ThreeFAdapterOwnableInvalidOwner)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "OwnableInvalidOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterOwnableUnauthorizedAccount represents a OwnableUnauthorizedAccount error raised by the ThreeFAdapter contract.
type ThreeFAdapterOwnableUnauthorizedAccount struct {
	Account common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func ThreeFAdapterOwnableUnauthorizedAccountErrorID() common.Hash {
	return common.HexToHash("0x118cdaa7a341953d1887a2245fd6665d741c67c8c50581daa59e1d03373fa188")
}

// UnpackOwnableUnauthorizedAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func (threeFAdapter *ThreeFAdapter) UnpackOwnableUnauthorizedAccountError(raw []byte) (*ThreeFAdapterOwnableUnauthorizedAccount, error) {
	out := new(ThreeFAdapterOwnableUnauthorizedAccount)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "OwnableUnauthorizedAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterPerRequestCapExceeded represents a PerRequestCapExceeded error raised by the ThreeFAdapter contract.
type ThreeFAdapterPerRequestCapExceeded struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error PerRequestCapExceeded()
func ThreeFAdapterPerRequestCapExceededErrorID() common.Hash {
	return common.HexToHash("0x71f1d368b03a6a05e70fa19b11e67ee48021141a925ec34bb569da61b20c54ba")
}

// UnpackPerRequestCapExceededError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error PerRequestCapExceeded()
func (threeFAdapter *ThreeFAdapter) UnpackPerRequestCapExceededError(raw []byte) (*ThreeFAdapterPerRequestCapExceeded, error) {
	out := new(ThreeFAdapterPerRequestCapExceeded)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "PerRequestCapExceeded", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterReentrancyGuardReentrantCall represents a ReentrancyGuardReentrantCall error raised by the ThreeFAdapter contract.
type ThreeFAdapterReentrancyGuardReentrantCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ReentrancyGuardReentrantCall()
func ThreeFAdapterReentrancyGuardReentrantCallErrorID() common.Hash {
	return common.HexToHash("0x3ee5aeb571de7fc460830b4d0017439a1ca56fb0bc39062227ade4fe4a24c1ca")
}

// UnpackReentrancyGuardReentrantCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ReentrancyGuardReentrantCall()
func (threeFAdapter *ThreeFAdapter) UnpackReentrancyGuardReentrantCallError(raw []byte) (*ThreeFAdapterReentrancyGuardReentrantCall, error) {
	out := new(ThreeFAdapterReentrancyGuardReentrantCall)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "ReentrancyGuardReentrantCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the ThreeFAdapter contract.
type ThreeFAdapterSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func ThreeFAdapterSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (threeFAdapter *ThreeFAdapter) UnpackSafeERC20FailedOperationError(raw []byte) (*ThreeFAdapterSafeERC20FailedOperation, error) {
	out := new(ThreeFAdapterSafeERC20FailedOperation)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterTooManyLoans represents a TooManyLoans error raised by the ThreeFAdapter contract.
type ThreeFAdapterTooManyLoans struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooManyLoans()
func ThreeFAdapterTooManyLoansErrorID() common.Hash {
	return common.HexToHash("0x79f076cbcc68c75c88e41b56e5c9c606891ae32cb5903197b2e48a3a10ade578")
}

// UnpackTooManyLoansError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooManyLoans()
func (threeFAdapter *ThreeFAdapter) UnpackTooManyLoansError(raw []byte) (*ThreeFAdapterTooManyLoans, error) {
	out := new(ThreeFAdapterTooManyLoans)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "TooManyLoans", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterYieldTooLow represents a YieldTooLow error raised by the ThreeFAdapter contract.
type ThreeFAdapterYieldTooLow struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error YieldTooLow()
func ThreeFAdapterYieldTooLowErrorID() common.Hash {
	return common.HexToHash("0x6f0b92522c675a3e71e7d7b1715735261c69f22c160af876cf34df7e201b542f")
}

// UnpackYieldTooLowError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error YieldTooLow()
func (threeFAdapter *ThreeFAdapter) UnpackYieldTooLowError(raw []byte) (*ThreeFAdapterYieldTooLow, error) {
	out := new(ThreeFAdapterYieldTooLow)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "YieldTooLow", raw); err != nil {
		return nil, err
	}
	return out, nil
}
