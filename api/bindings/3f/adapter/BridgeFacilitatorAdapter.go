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

// BridgeFacilitatorAdapterMetaData contains all meta data concerning the BridgeFacilitatorAdapter contract.
var BridgeFacilitatorAdapterMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"requestWhitelist\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vaultFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"adapterFactory\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"REQUEST_WHITELIST\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeRequests\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"deallocated\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isValidSignature\",\"inputs\":[{\"name\":\"hash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"maxConcurrentLoans\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minRequestYieldBps\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"offerSigner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"onRequestConsumed\",\"inputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structOffer\",\"components\":[{\"name\":\"maker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedReturn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expiration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"useCallback\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"principal\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yield\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"outstandingPrincipal\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"perRequestMaxCollateral\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"positions\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"principal\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"ytExpected\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"openedAt\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"redeemed\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"realizedPrincipal\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"redeem\",\"inputs\":[{\"name\":\"requests\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestDeallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setExposureLimits\",\"inputs\":[{\"name\":\"perRequestMaxCollateral_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"totalMaxCollateral_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minRequestYieldBps_\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxConcurrentLoans_\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setOfferSigner\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalMaxCollateral\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"vault\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PositionOpened\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"principal\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"ytExpected\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PositionRedeemed\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"principal\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"yield\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetExposureLimits\",\"inputs\":[{\"name\":\"perRequestMaxCollateral\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"totalMaxCollateral\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"minRequestYieldBps\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"maxConcurrentLoans\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetOfferSigner\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetVault\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AssetMismatch\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientLiquidity\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotAttested\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"PerRequestCapExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeCastOverflowedUintDowncast\",\"inputs\":[{\"name\":\"bits\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SleeveCapExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooManyConcurrentLoans\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"YieldTooLow\",\"inputs\":[]}]",
	ID:  "BridgeFacilitatorAdapter",
}

// BridgeFacilitatorAdapter is an auto generated Go binding around an Ethereum contract.
type BridgeFacilitatorAdapter struct {
	abi abi.ABI
}

// NewBridgeFacilitatorAdapter creates a new instance of BridgeFacilitatorAdapter.
func NewBridgeFacilitatorAdapter() *BridgeFacilitatorAdapter {
	parsed, err := BridgeFacilitatorAdapterMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &BridgeFacilitatorAdapter{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *BridgeFacilitatorAdapter) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address requestWhitelist, address vaultFactory, address adapterFactory) returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackConstructor(requestWhitelist common.Address, vaultFactory common.Address, adapterFactory common.Address) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("", requestWhitelist, vaultFactory, adapterFactory)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackFACTORY() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("FACTORY")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackFACTORY() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("FACTORY")
}

// UnpackFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackFACTORY(data []byte) (common.Address, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("FACTORY", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackREQUESTWHITELIST() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("REQUEST_WHITELIST")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackREQUESTWHITELIST() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("REQUEST_WHITELIST")
}

// UnpackREQUESTWHITELIST is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x894e6d61.
//
// Solidity: function REQUEST_WHITELIST() view returns(address)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackREQUESTWHITELIST(data []byte) (common.Address, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("REQUEST_WHITELIST", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackActiveRequests is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x83cc915c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function activeRequests() view returns(address[])
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackActiveRequests() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("activeRequests")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackActiveRequests() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("activeRequests")
}

// UnpackActiveRequests is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x83cc915c.
//
// Solidity: function activeRequests() view returns(address[])
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackActiveRequests(data []byte) ([]common.Address, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("activeRequests", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackAllocatable() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("allocatable")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackAllocatable() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("allocatable")
}

// UnpackAllocatable is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackAllocatable(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("allocatable", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackAllocate(amount *big.Int) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("allocate", amount)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackAllocate(amount *big.Int) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("allocate", amount)
}

// UnpackAllocate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackAllocate(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("allocate", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackDeallocate(amount *big.Int) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("deallocate", amount)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackDeallocate(amount *big.Int) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("deallocate", amount)
}

// UnpackDeallocate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256 deallocated)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackDeallocate(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("deallocate", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackFreeAssets() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("freeAssets")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackFreeAssets() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("freeAssets")
}

// UnpackFreeAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackFreeAssets(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("freeAssets", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackInitialize(initialVersion uint64, owner common.Address, data []byte) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("initialize", initialVersion, owner, data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackInitialize(initialVersion uint64, owner common.Address, data []byte) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("initialize", initialVersion, owner, data)
}

// PackIsValidSignature is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1626ba7e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackIsValidSignature(hash [32]byte, signature []byte) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("isValidSignature", hash, signature)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackIsValidSignature(hash [32]byte, signature []byte) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("isValidSignature", hash, signature)
}

// UnpackIsValidSignature is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackIsValidSignature(data []byte) ([4]byte, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("isValidSignature", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackMaxConcurrentLoans() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("maxConcurrentLoans")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackMaxConcurrentLoans() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("maxConcurrentLoans")
}

// UnpackMaxConcurrentLoans is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0fa715c7.
//
// Solidity: function maxConcurrentLoans() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackMaxConcurrentLoans(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("maxConcurrentLoans", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackMigrate(newVersion uint64, data []byte) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("migrate", newVersion, data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackMigrate(newVersion uint64, data []byte) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("migrate", newVersion, data)
}

// PackMinRequestYieldBps is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6762571b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minRequestYieldBps() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackMinRequestYieldBps() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("minRequestYieldBps")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinRequestYieldBps is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6762571b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minRequestYieldBps() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackMinRequestYieldBps() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("minRequestYieldBps")
}

// UnpackMinRequestYieldBps is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6762571b.
//
// Solidity: function minRequestYieldBps() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackMinRequestYieldBps(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("minRequestYieldBps", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackMulticall(data [][]byte) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("multicall", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackMulticall(data [][]byte) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("multicall", data)
}

// PackOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x566bd6c3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function offerSigner() view returns(address)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackOfferSigner() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("offerSigner")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackOfferSigner() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("offerSigner")
}

// UnpackOfferSigner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x566bd6c3.
//
// Solidity: function offerSigner() view returns(address)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackOfferSigner(data []byte) (common.Address, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("offerSigner", data)
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
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) , bytes , uint256 principal, uint256 yield) returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackOnRequestConsumed(arg0 Offer, arg1 []byte, principal *big.Int, yield *big.Int) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("onRequestConsumed", arg0, arg1, principal, yield)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOnRequestConsumed is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fe1357.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) , bytes , uint256 principal, uint256 yield) returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackOnRequestConsumed(arg0 Offer, arg1 []byte, principal *big.Int, yield *big.Int) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("onRequestConsumed", arg0, arg1, principal, yield)
}

// PackOutstandingPrincipal is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x29b1829e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackOutstandingPrincipal() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("outstandingPrincipal")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackOutstandingPrincipal() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("outstandingPrincipal")
}

// UnpackOutstandingPrincipal is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x29b1829e.
//
// Solidity: function outstandingPrincipal() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackOutstandingPrincipal(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("outstandingPrincipal", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackOwner() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("owner")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackOwner() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackOwner(data []byte) (common.Address, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("owner", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackPerRequestMaxCollateral() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("perRequestMaxCollateral")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackPerRequestMaxCollateral() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("perRequestMaxCollateral")
}

// UnpackPerRequestMaxCollateral is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xca1f1576.
//
// Solidity: function perRequestMaxCollateral() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackPerRequestMaxCollateral(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("perRequestMaxCollateral", data)
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
// Solidity: function positions(address request) view returns(uint128 principal, uint128 ytExpected, uint48 openedAt, bool redeemed)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackPositions(request common.Address) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("positions", request)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPositions is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x55f57510.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function positions(address request) view returns(uint128 principal, uint128 ytExpected, uint48 openedAt, bool redeemed)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackPositions(request common.Address) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("positions", request)
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
// Solidity: function positions(address request) view returns(uint128 principal, uint128 ytExpected, uint48 openedAt, bool redeemed)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackPositions(data []byte) (PositionsOutput, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("positions", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackRealizedPrincipal() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("realizedPrincipal")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackRealizedPrincipal() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("realizedPrincipal")
}

// UnpackRealizedPrincipal is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5b348b1f.
//
// Solidity: function realizedPrincipal() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackRealizedPrincipal(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("realizedPrincipal", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackRedeem(requests []common.Address) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("redeem", requests)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackRedeem(requests []common.Address) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("redeem", requests)
}

// PackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function renounceOwnership() returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackRenounceOwnership() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("renounceOwnership")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackRenounceOwnership() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("renounceOwnership")
}

// PackRequestDeallocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf79f679d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackRequestDeallocate(amount *big.Int) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("requestDeallocate", amount)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackRequestDeallocate(amount *big.Int) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("requestDeallocate", amount)
}

// PackSetExposureLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe05d0a0c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setExposureLimits(uint256 perRequestMaxCollateral_, uint256 totalMaxCollateral_, uint256 minRequestYieldBps_, uint256 maxConcurrentLoans_) returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackSetExposureLimits(perRequestMaxCollateral *big.Int, totalMaxCollateral *big.Int, minRequestYieldBps *big.Int, maxConcurrentLoans *big.Int) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("setExposureLimits", perRequestMaxCollateral, totalMaxCollateral, minRequestYieldBps, maxConcurrentLoans)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetExposureLimits is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe05d0a0c.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setExposureLimits(uint256 perRequestMaxCollateral_, uint256 totalMaxCollateral_, uint256 minRequestYieldBps_, uint256 maxConcurrentLoans_) returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackSetExposureLimits(perRequestMaxCollateral *big.Int, totalMaxCollateral *big.Int, minRequestYieldBps *big.Int, maxConcurrentLoans *big.Int) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("setExposureLimits", perRequestMaxCollateral, totalMaxCollateral, minRequestYieldBps, maxConcurrentLoans)
}

// PackSetOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x868adcae.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setOfferSigner(address signer) returns()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackSetOfferSigner(signer common.Address) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("setOfferSigner", signer)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackSetOfferSigner(signer common.Address) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("setOfferSigner", signer)
}

// PackTotalAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x01e1d114.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalAssets() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackTotalAssets() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("totalAssets")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackTotalAssets() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("totalAssets")
}

// UnpackTotalAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackTotalAssets(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("totalAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackTotalMaxCollateral is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe5a81bbc.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalMaxCollateral() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackTotalMaxCollateral() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("totalMaxCollateral")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTotalMaxCollateral is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe5a81bbc.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function totalMaxCollateral() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackTotalMaxCollateral() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("totalMaxCollateral")
}

// UnpackTotalMaxCollateral is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xe5a81bbc.
//
// Solidity: function totalMaxCollateral() view returns(uint256)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackTotalMaxCollateral(data []byte) (*big.Int, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("totalMaxCollateral", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("transferOwnership", newOwner)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("transferOwnership", newOwner)
}

// PackVault is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfbfa77cf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function vault() view returns(address)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackVault() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("vault")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackVault() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("vault")
}

// UnpackVault is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackVault(data []byte) (common.Address, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("vault", data)
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) PackVersion() []byte {
	enc, err := bridgeFacilitatorAdapter.abi.Pack("version")
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) TryPackVersion() ([]byte, error) {
	return bridgeFacilitatorAdapter.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackVersion(data []byte) (uint64, error) {
	out, err := bridgeFacilitatorAdapter.abi.Unpack("version", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// BridgeFacilitatorAdapterInitialized represents a Initialized event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterInitialized struct {
	Version uint64
	Raw     *types.Log // Blockchain specific contextual infos
}

const BridgeFacilitatorAdapterInitializedEventName = "Initialized"

// ContractEventName returns the user-defined event name.
func (BridgeFacilitatorAdapterInitialized) ContractEventName() string {
	return BridgeFacilitatorAdapterInitializedEventName
}

// UnpackInitializedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialized(uint64 version)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackInitializedEvent(log *types.Log) (*BridgeFacilitatorAdapterInitialized, error) {
	event := "Initialized"
	if log.Topics[0] != bridgeFacilitatorAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(BridgeFacilitatorAdapterInitialized)
	if len(log.Data) > 0 {
		if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range bridgeFacilitatorAdapter.abi.Events[event].Inputs {
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

// BridgeFacilitatorAdapterOwnershipTransferred represents a OwnershipTransferred event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const BridgeFacilitatorAdapterOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (BridgeFacilitatorAdapterOwnershipTransferred) ContractEventName() string {
	return BridgeFacilitatorAdapterOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackOwnershipTransferredEvent(log *types.Log) (*BridgeFacilitatorAdapterOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if log.Topics[0] != bridgeFacilitatorAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(BridgeFacilitatorAdapterOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range bridgeFacilitatorAdapter.abi.Events[event].Inputs {
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

// BridgeFacilitatorAdapterPositionOpened represents a PositionOpened event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterPositionOpened struct {
	Request    common.Address
	Principal  *big.Int
	YtExpected *big.Int
	Raw        *types.Log // Blockchain specific contextual infos
}

const BridgeFacilitatorAdapterPositionOpenedEventName = "PositionOpened"

// ContractEventName returns the user-defined event name.
func (BridgeFacilitatorAdapterPositionOpened) ContractEventName() string {
	return BridgeFacilitatorAdapterPositionOpenedEventName
}

// UnpackPositionOpenedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event PositionOpened(address indexed request, uint256 principal, uint256 ytExpected)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackPositionOpenedEvent(log *types.Log) (*BridgeFacilitatorAdapterPositionOpened, error) {
	event := "PositionOpened"
	if log.Topics[0] != bridgeFacilitatorAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(BridgeFacilitatorAdapterPositionOpened)
	if len(log.Data) > 0 {
		if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range bridgeFacilitatorAdapter.abi.Events[event].Inputs {
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

// BridgeFacilitatorAdapterPositionRedeemed represents a PositionRedeemed event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterPositionRedeemed struct {
	Request   common.Address
	Principal *big.Int
	Yield     *big.Int
	Raw       *types.Log // Blockchain specific contextual infos
}

const BridgeFacilitatorAdapterPositionRedeemedEventName = "PositionRedeemed"

// ContractEventName returns the user-defined event name.
func (BridgeFacilitatorAdapterPositionRedeemed) ContractEventName() string {
	return BridgeFacilitatorAdapterPositionRedeemedEventName
}

// UnpackPositionRedeemedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event PositionRedeemed(address indexed request, uint256 principal, uint256 yield)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackPositionRedeemedEvent(log *types.Log) (*BridgeFacilitatorAdapterPositionRedeemed, error) {
	event := "PositionRedeemed"
	if log.Topics[0] != bridgeFacilitatorAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(BridgeFacilitatorAdapterPositionRedeemed)
	if len(log.Data) > 0 {
		if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range bridgeFacilitatorAdapter.abi.Events[event].Inputs {
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

// BridgeFacilitatorAdapterSetExposureLimits represents a SetExposureLimits event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetExposureLimits struct {
	PerRequestMaxCollateral *big.Int
	TotalMaxCollateral      *big.Int
	MinRequestYieldBps      *big.Int
	MaxConcurrentLoans      *big.Int
	Raw                     *types.Log // Blockchain specific contextual infos
}

const BridgeFacilitatorAdapterSetExposureLimitsEventName = "SetExposureLimits"

// ContractEventName returns the user-defined event name.
func (BridgeFacilitatorAdapterSetExposureLimits) ContractEventName() string {
	return BridgeFacilitatorAdapterSetExposureLimitsEventName
}

// UnpackSetExposureLimitsEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetExposureLimits(uint256 perRequestMaxCollateral, uint256 totalMaxCollateral, uint256 minRequestYieldBps, uint256 maxConcurrentLoans)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackSetExposureLimitsEvent(log *types.Log) (*BridgeFacilitatorAdapterSetExposureLimits, error) {
	event := "SetExposureLimits"
	if log.Topics[0] != bridgeFacilitatorAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(BridgeFacilitatorAdapterSetExposureLimits)
	if len(log.Data) > 0 {
		if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range bridgeFacilitatorAdapter.abi.Events[event].Inputs {
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

// BridgeFacilitatorAdapterSetOfferSigner represents a SetOfferSigner event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetOfferSigner struct {
	Signer common.Address
	Raw    *types.Log // Blockchain specific contextual infos
}

const BridgeFacilitatorAdapterSetOfferSignerEventName = "SetOfferSigner"

// ContractEventName returns the user-defined event name.
func (BridgeFacilitatorAdapterSetOfferSigner) ContractEventName() string {
	return BridgeFacilitatorAdapterSetOfferSignerEventName
}

// UnpackSetOfferSignerEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetOfferSigner(address indexed signer)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackSetOfferSignerEvent(log *types.Log) (*BridgeFacilitatorAdapterSetOfferSigner, error) {
	event := "SetOfferSigner"
	if log.Topics[0] != bridgeFacilitatorAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(BridgeFacilitatorAdapterSetOfferSigner)
	if len(log.Data) > 0 {
		if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range bridgeFacilitatorAdapter.abi.Events[event].Inputs {
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

// BridgeFacilitatorAdapterSetVault represents a SetVault event raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSetVault struct {
	Vault common.Address
	Raw   *types.Log // Blockchain specific contextual infos
}

const BridgeFacilitatorAdapterSetVaultEventName = "SetVault"

// ContractEventName returns the user-defined event name.
func (BridgeFacilitatorAdapterSetVault) ContractEventName() string {
	return BridgeFacilitatorAdapterSetVaultEventName
}

// UnpackSetVaultEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetVault(address indexed vault)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackSetVaultEvent(log *types.Log) (*BridgeFacilitatorAdapterSetVault, error) {
	event := "SetVault"
	if log.Topics[0] != bridgeFacilitatorAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(BridgeFacilitatorAdapterSetVault)
	if len(log.Data) > 0 {
		if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range bridgeFacilitatorAdapter.abi.Events[event].Inputs {
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
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["AlreadyInitialized"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["AssetMismatch"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackAssetMismatchError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["InsufficientLiquidity"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackInsufficientLiquidityError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["InvalidInitialization"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackInvalidInitializationError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["InvalidVault"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackInvalidVaultError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["NotAttested"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackNotAttestedError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["NotFactory"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackNotFactoryError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["NotInitialized"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackNotInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["NotInitializing"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackNotInitializingError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["NotVault"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackNotVaultError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["OwnableInvalidOwner"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackOwnableInvalidOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["OwnableUnauthorizedAccount"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackOwnableUnauthorizedAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["PerRequestCapExceeded"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackPerRequestCapExceededError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["ReentrancyGuardReentrantCall"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackReentrancyGuardReentrantCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["SafeCastOverflowedUintDowncast"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackSafeCastOverflowedUintDowncastError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["SleeveCapExceeded"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackSleeveCapExceededError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["TooManyConcurrentLoans"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackTooManyConcurrentLoansError(raw[4:])
	}
	if bytes.Equal(raw[:4], bridgeFacilitatorAdapter.abi.Errors["YieldTooLow"].ID.Bytes()[:4]) {
		return bridgeFacilitatorAdapter.UnpackYieldTooLowError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// BridgeFacilitatorAdapterAlreadyInitialized represents a AlreadyInitialized error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterAlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyInitialized()
func BridgeFacilitatorAdapterAlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x0dc149f07762891dbcea3fe72770f3d63a1863fc54b2f084e8c59ec476996927")
}

// UnpackAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyInitialized()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackAlreadyInitializedError(raw []byte) (*BridgeFacilitatorAdapterAlreadyInitialized, error) {
	out := new(BridgeFacilitatorAdapterAlreadyInitialized)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "AlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterAssetMismatch represents a AssetMismatch error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterAssetMismatch struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AssetMismatch()
func BridgeFacilitatorAdapterAssetMismatchErrorID() common.Hash {
	return common.HexToHash("0x83c1010ad7aa04f27fb612a82818ae1f4e183ffb2c2ce08a49b7b56cdd6dd4fb")
}

// UnpackAssetMismatchError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AssetMismatch()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackAssetMismatchError(raw []byte) (*BridgeFacilitatorAdapterAssetMismatch, error) {
	out := new(BridgeFacilitatorAdapterAssetMismatch)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "AssetMismatch", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterInsufficientLiquidity represents a InsufficientLiquidity error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterInsufficientLiquidity struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientLiquidity()
func BridgeFacilitatorAdapterInsufficientLiquidityErrorID() common.Hash {
	return common.HexToHash("0xbb55fd27c46b5ba9f88ff2cb2222216afeb0f193423b26615497b3020ab61f8e")
}

// UnpackInsufficientLiquidityError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientLiquidity()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackInsufficientLiquidityError(raw []byte) (*BridgeFacilitatorAdapterInsufficientLiquidity, error) {
	out := new(BridgeFacilitatorAdapterInsufficientLiquidity)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "InsufficientLiquidity", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterInvalidInitialization represents a InvalidInitialization error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterInvalidInitialization struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidInitialization()
func BridgeFacilitatorAdapterInvalidInitializationErrorID() common.Hash {
	return common.HexToHash("0xf92ee8a957075833165f68c320933b1a1294aafc84ee6e0dd3fb178008f9aaf5")
}

// UnpackInvalidInitializationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidInitialization()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackInvalidInitializationError(raw []byte) (*BridgeFacilitatorAdapterInvalidInitialization, error) {
	out := new(BridgeFacilitatorAdapterInvalidInitialization)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "InvalidInitialization", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterInvalidVault represents a InvalidVault error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterInvalidVault struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidVault()
func BridgeFacilitatorAdapterInvalidVaultErrorID() common.Hash {
	return common.HexToHash("0xd03a63207f799c8b4a310cf73db481de483ce6543ef24d1f75f918a11e4eae1f")
}

// UnpackInvalidVaultError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidVault()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackInvalidVaultError(raw []byte) (*BridgeFacilitatorAdapterInvalidVault, error) {
	out := new(BridgeFacilitatorAdapterInvalidVault)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "InvalidVault", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterNotAttested represents a NotAttested error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterNotAttested struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotAttested()
func BridgeFacilitatorAdapterNotAttestedErrorID() common.Hash {
	return common.HexToHash("0x99efb89078879e78f0f307145c3360fe4f6680762a21d87392e067610a80f73d")
}

// UnpackNotAttestedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotAttested()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackNotAttestedError(raw []byte) (*BridgeFacilitatorAdapterNotAttested, error) {
	out := new(BridgeFacilitatorAdapterNotAttested)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "NotAttested", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterNotFactory represents a NotFactory error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterNotFactory struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotFactory()
func BridgeFacilitatorAdapterNotFactoryErrorID() common.Hash {
	return common.HexToHash("0x32cc723614e775fc4a8386492bc9a860c12fe98d5f5f28ec17e265818645b229")
}

// UnpackNotFactoryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotFactory()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackNotFactoryError(raw []byte) (*BridgeFacilitatorAdapterNotFactory, error) {
	out := new(BridgeFacilitatorAdapterNotFactory)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "NotFactory", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterNotInitialized represents a NotInitialized error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterNotInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitialized()
func BridgeFacilitatorAdapterNotInitializedErrorID() common.Hash {
	return common.HexToHash("0x87138d5c8c2e77cb9f25c07b03277aad63d22f6a05255580ec55d2c21666e734")
}

// UnpackNotInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitialized()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackNotInitializedError(raw []byte) (*BridgeFacilitatorAdapterNotInitialized, error) {
	out := new(BridgeFacilitatorAdapterNotInitialized)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "NotInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterNotInitializing represents a NotInitializing error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterNotInitializing struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitializing()
func BridgeFacilitatorAdapterNotInitializingErrorID() common.Hash {
	return common.HexToHash("0xd7e6bcf8597daa127dc9f0048d2f08d5ef140a2cb659feabd700beff1f7a8302")
}

// UnpackNotInitializingError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitializing()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackNotInitializingError(raw []byte) (*BridgeFacilitatorAdapterNotInitializing, error) {
	out := new(BridgeFacilitatorAdapterNotInitializing)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "NotInitializing", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterNotVault represents a NotVault error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterNotVault struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotVault()
func BridgeFacilitatorAdapterNotVaultErrorID() common.Hash {
	return common.HexToHash("0x62df0545b0e47f06f6a9990975121b8c49c83a96f18696393f66a69dd2ffe568")
}

// UnpackNotVaultError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotVault()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackNotVaultError(raw []byte) (*BridgeFacilitatorAdapterNotVault, error) {
	out := new(BridgeFacilitatorAdapterNotVault)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "NotVault", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterOwnableInvalidOwner represents a OwnableInvalidOwner error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterOwnableInvalidOwner struct {
	Owner common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableInvalidOwner(address owner)
func BridgeFacilitatorAdapterOwnableInvalidOwnerErrorID() common.Hash {
	return common.HexToHash("0x1e4fbdf7f3ef8bcaa855599e3abf48b232380f183f08f6f813d9ffa5bd585188")
}

// UnpackOwnableInvalidOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableInvalidOwner(address owner)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackOwnableInvalidOwnerError(raw []byte) (*BridgeFacilitatorAdapterOwnableInvalidOwner, error) {
	out := new(BridgeFacilitatorAdapterOwnableInvalidOwner)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "OwnableInvalidOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterOwnableUnauthorizedAccount represents a OwnableUnauthorizedAccount error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterOwnableUnauthorizedAccount struct {
	Account common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func BridgeFacilitatorAdapterOwnableUnauthorizedAccountErrorID() common.Hash {
	return common.HexToHash("0x118cdaa7a341953d1887a2245fd6665d741c67c8c50581daa59e1d03373fa188")
}

// UnpackOwnableUnauthorizedAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackOwnableUnauthorizedAccountError(raw []byte) (*BridgeFacilitatorAdapterOwnableUnauthorizedAccount, error) {
	out := new(BridgeFacilitatorAdapterOwnableUnauthorizedAccount)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "OwnableUnauthorizedAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterPerRequestCapExceeded represents a PerRequestCapExceeded error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterPerRequestCapExceeded struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error PerRequestCapExceeded()
func BridgeFacilitatorAdapterPerRequestCapExceededErrorID() common.Hash {
	return common.HexToHash("0x71f1d368b03a6a05e70fa19b11e67ee48021141a925ec34bb569da61b20c54ba")
}

// UnpackPerRequestCapExceededError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error PerRequestCapExceeded()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackPerRequestCapExceededError(raw []byte) (*BridgeFacilitatorAdapterPerRequestCapExceeded, error) {
	out := new(BridgeFacilitatorAdapterPerRequestCapExceeded)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "PerRequestCapExceeded", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterReentrancyGuardReentrantCall represents a ReentrancyGuardReentrantCall error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterReentrancyGuardReentrantCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ReentrancyGuardReentrantCall()
func BridgeFacilitatorAdapterReentrancyGuardReentrantCallErrorID() common.Hash {
	return common.HexToHash("0x3ee5aeb571de7fc460830b4d0017439a1ca56fb0bc39062227ade4fe4a24c1ca")
}

// UnpackReentrancyGuardReentrantCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ReentrancyGuardReentrantCall()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackReentrancyGuardReentrantCallError(raw []byte) (*BridgeFacilitatorAdapterReentrancyGuardReentrantCall, error) {
	out := new(BridgeFacilitatorAdapterReentrancyGuardReentrantCall)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "ReentrancyGuardReentrantCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterSafeCastOverflowedUintDowncast represents a SafeCastOverflowedUintDowncast error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSafeCastOverflowedUintDowncast struct {
	Bits  uint8
	Value *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeCastOverflowedUintDowncast(uint8 bits, uint256 value)
func BridgeFacilitatorAdapterSafeCastOverflowedUintDowncastErrorID() common.Hash {
	return common.HexToHash("0x6dfcc6503a32754ce7a89698e18201fc5294fd4aad43edefee786f88423b1a12")
}

// UnpackSafeCastOverflowedUintDowncastError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeCastOverflowedUintDowncast(uint8 bits, uint256 value)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackSafeCastOverflowedUintDowncastError(raw []byte) (*BridgeFacilitatorAdapterSafeCastOverflowedUintDowncast, error) {
	out := new(BridgeFacilitatorAdapterSafeCastOverflowedUintDowncast)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "SafeCastOverflowedUintDowncast", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func BridgeFacilitatorAdapterSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackSafeERC20FailedOperationError(raw []byte) (*BridgeFacilitatorAdapterSafeERC20FailedOperation, error) {
	out := new(BridgeFacilitatorAdapterSafeERC20FailedOperation)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterSleeveCapExceeded represents a SleeveCapExceeded error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterSleeveCapExceeded struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SleeveCapExceeded()
func BridgeFacilitatorAdapterSleeveCapExceededErrorID() common.Hash {
	return common.HexToHash("0x8d3a6f3e593b3d68ab893b7400dc61f24661274e31ea7e1a15c542fef17b22de")
}

// UnpackSleeveCapExceededError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SleeveCapExceeded()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackSleeveCapExceededError(raw []byte) (*BridgeFacilitatorAdapterSleeveCapExceeded, error) {
	out := new(BridgeFacilitatorAdapterSleeveCapExceeded)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "SleeveCapExceeded", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterTooManyConcurrentLoans represents a TooManyConcurrentLoans error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterTooManyConcurrentLoans struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooManyConcurrentLoans()
func BridgeFacilitatorAdapterTooManyConcurrentLoansErrorID() common.Hash {
	return common.HexToHash("0x300b297baedcb5c69bc8e4b45ba3f071ba8caa59a4b3e446905bac4dc7450498")
}

// UnpackTooManyConcurrentLoansError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooManyConcurrentLoans()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackTooManyConcurrentLoansError(raw []byte) (*BridgeFacilitatorAdapterTooManyConcurrentLoans, error) {
	out := new(BridgeFacilitatorAdapterTooManyConcurrentLoans)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "TooManyConcurrentLoans", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// BridgeFacilitatorAdapterYieldTooLow represents a YieldTooLow error raised by the BridgeFacilitatorAdapter contract.
type BridgeFacilitatorAdapterYieldTooLow struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error YieldTooLow()
func BridgeFacilitatorAdapterYieldTooLowErrorID() common.Hash {
	return common.HexToHash("0x6f0b92522c675a3e71e7d7b1715735261c69f22c160af876cf34df7e201b542f")
}

// UnpackYieldTooLowError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error YieldTooLow()
func (bridgeFacilitatorAdapter *BridgeFacilitatorAdapter) UnpackYieldTooLowError(raw []byte) (*BridgeFacilitatorAdapterYieldTooLow, error) {
	out := new(BridgeFacilitatorAdapterYieldTooLow)
	if err := bridgeFacilitatorAdapter.abi.UnpackIntoInterface(out, "YieldTooLow", raw); err != nil {
		return nil, err
	}
	return out, nil
}
