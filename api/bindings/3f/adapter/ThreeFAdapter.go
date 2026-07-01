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
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"vaultFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"adapterFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"requestWhitelist\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"REQUEST_WHITELIST\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"finalizeRequest\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isValidSignature\",\"inputs\":[{\"name\":\"hash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"maxAssetsPerRequest\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minAssetsPerRequest\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"minYieldPerRequest\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"offerSigner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"onRequestConsumed\",\"inputs\":[{\"name\":\"offer\",\"type\":\"tuple\",\"internalType\":\"structOffer\",\"components\":[{\"name\":\"maker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedReturn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expiration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"useCallback\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"principalAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yieldAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestDeallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestIndex\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"requests\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"requestsLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"setLimitsPerRequest\",\"inputs\":[{\"name\":\"newMinYieldPerRequest\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"newMinAssetsPerRequest\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"newMaxAssetsPerRequest\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setOfferSigner\",\"inputs\":[{\"name\":\"newOfferSigner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"staticDelegateCall\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"vault\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"FinalizeRequest\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OnRequestConsumed\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"offer\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structOffer\",\"components\":[{\"name\":\"maker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedReturn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expiration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"useCallback\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"principalAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"yieldAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetLimitsPerRequest\",\"inputs\":[{\"name\":\"minYieldPerRequest\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"minAssetsPerRequest\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"maxAssetsPerRequest\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetOfferSigner\",\"inputs\":[{\"name\":\"offerSigner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetVault\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyRequest\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientAllocate\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotRequest\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"TooLargeRequest\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooLowYield\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooManyRequests\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooSmallRequest\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongAsset\",\"inputs\":[]}]",
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
// Solidity: constructor(address vaultFactory, address adapterFactory, address requestWhitelist) returns()
func (threeFAdapter *ThreeFAdapter) PackConstructor(vaultFactory common.Address, adapterFactory common.Address, requestWhitelist common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("", vaultFactory, adapterFactory, requestWhitelist)
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
// Solidity: function deallocate(uint256 amount) returns(uint256)
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
// Solidity: function deallocate(uint256 amount) returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackDeallocate(amount *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("deallocate", amount)
}

// UnpackDeallocate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackDeallocate(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("deallocate", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackFinalizeRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1d280eb9.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function finalizeRequest(address request) returns()
func (threeFAdapter *ThreeFAdapter) PackFinalizeRequest(request common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("finalizeRequest", request)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFinalizeRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1d280eb9.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function finalizeRequest(address request) returns()
func (threeFAdapter *ThreeFAdapter) TryPackFinalizeRequest(request common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("finalizeRequest", request)
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

// PackGetMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1755da83.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getMaxAssets() returns(uint256 assets)
func (threeFAdapter *ThreeFAdapter) PackGetMaxAssets() []byte {
	enc, err := threeFAdapter.abi.Pack("getMaxAssets")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1755da83.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getMaxAssets() returns(uint256 assets)
func (threeFAdapter *ThreeFAdapter) TryPackGetMaxAssets() ([]byte, error) {
	return threeFAdapter.abi.Pack("getMaxAssets")
}

// UnpackGetMaxAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1755da83.
//
// Solidity: function getMaxAssets() returns(uint256 assets)
func (threeFAdapter *ThreeFAdapter) UnpackGetMaxAssets(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("getMaxAssets", data)
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

// PackMaxAssetsPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe84fb141.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function maxAssetsPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackMaxAssetsPerRequest() []byte {
	enc, err := threeFAdapter.abi.Pack("maxAssetsPerRequest")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMaxAssetsPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe84fb141.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function maxAssetsPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackMaxAssetsPerRequest() ([]byte, error) {
	return threeFAdapter.abi.Pack("maxAssetsPerRequest")
}

// UnpackMaxAssetsPerRequest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xe84fb141.
//
// Solidity: function maxAssetsPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackMaxAssetsPerRequest(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("maxAssetsPerRequest", data)
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

// PackMinAssetsPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5b0a8440.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minAssetsPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackMinAssetsPerRequest() []byte {
	enc, err := threeFAdapter.abi.Pack("minAssetsPerRequest")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinAssetsPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5b0a8440.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minAssetsPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackMinAssetsPerRequest() ([]byte, error) {
	return threeFAdapter.abi.Pack("minAssetsPerRequest")
}

// UnpackMinAssetsPerRequest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5b0a8440.
//
// Solidity: function minAssetsPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackMinAssetsPerRequest(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("minAssetsPerRequest", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa9c6b425.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minYieldPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackMinYieldPerRequest() []byte {
	enc, err := threeFAdapter.abi.Pack("minYieldPerRequest")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa9c6b425.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minYieldPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackMinYieldPerRequest() ([]byte, error) {
	return threeFAdapter.abi.Pack("minYieldPerRequest")
}

// UnpackMinYieldPerRequest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xa9c6b425.
//
// Solidity: function minYieldPerRequest() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackMinYieldPerRequest(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("minYieldPerRequest", data)
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
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) offer, bytes , uint256 principalAssets, uint256 yieldAssets) returns()
func (threeFAdapter *ThreeFAdapter) PackOnRequestConsumed(offer Offer, arg1 []byte, principalAssets *big.Int, yieldAssets *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("onRequestConsumed", offer, arg1, principalAssets, yieldAssets)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOnRequestConsumed is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fe1357.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function onRequestConsumed((address,uint256,uint256,uint256,uint256,bool) offer, bytes , uint256 principalAssets, uint256 yieldAssets) returns()
func (threeFAdapter *ThreeFAdapter) TryPackOnRequestConsumed(offer Offer, arg1 []byte, principalAssets *big.Int, yieldAssets *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("onRequestConsumed", offer, arg1, principalAssets, yieldAssets)
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

// PackRequestIndex is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8163ade3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function requestIndex(address request) view returns(uint256 index)
func (threeFAdapter *ThreeFAdapter) PackRequestIndex(request common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("requestIndex", request)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRequestIndex is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8163ade3.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function requestIndex(address request) view returns(uint256 index)
func (threeFAdapter *ThreeFAdapter) TryPackRequestIndex(request common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("requestIndex", request)
}

// UnpackRequestIndex is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8163ade3.
//
// Solidity: function requestIndex(address request) view returns(uint256 index)
func (threeFAdapter *ThreeFAdapter) UnpackRequestIndex(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("requestIndex", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackRequests is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x81d12c58.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function requests(uint256 ) view returns(address)
func (threeFAdapter *ThreeFAdapter) PackRequests(arg0 *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("requests", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRequests is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x81d12c58.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function requests(uint256 ) view returns(address)
func (threeFAdapter *ThreeFAdapter) TryPackRequests(arg0 *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("requests", arg0)
}

// UnpackRequests is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x81d12c58.
//
// Solidity: function requests(uint256 ) view returns(address)
func (threeFAdapter *ThreeFAdapter) UnpackRequests(data []byte) (common.Address, error) {
	out, err := threeFAdapter.abi.Unpack("requests", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackRequestsLength is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xffbbfcb0.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function requestsLength() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) PackRequestsLength() []byte {
	enc, err := threeFAdapter.abi.Pack("requestsLength")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRequestsLength is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xffbbfcb0.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function requestsLength() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) TryPackRequestsLength() ([]byte, error) {
	return threeFAdapter.abi.Pack("requestsLength")
}

// UnpackRequestsLength is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xffbbfcb0.
//
// Solidity: function requestsLength() view returns(uint256)
func (threeFAdapter *ThreeFAdapter) UnpackRequestsLength(data []byte) (*big.Int, error) {
	out, err := threeFAdapter.abi.Unpack("requestsLength", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackSetLimitsPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x719b949f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setLimitsPerRequest(uint256 newMinYieldPerRequest, uint256 newMinAssetsPerRequest, uint256 newMaxAssetsPerRequest) returns()
func (threeFAdapter *ThreeFAdapter) PackSetLimitsPerRequest(newMinYieldPerRequest *big.Int, newMinAssetsPerRequest *big.Int, newMaxAssetsPerRequest *big.Int) []byte {
	enc, err := threeFAdapter.abi.Pack("setLimitsPerRequest", newMinYieldPerRequest, newMinAssetsPerRequest, newMaxAssetsPerRequest)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetLimitsPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x719b949f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setLimitsPerRequest(uint256 newMinYieldPerRequest, uint256 newMinAssetsPerRequest, uint256 newMaxAssetsPerRequest) returns()
func (threeFAdapter *ThreeFAdapter) TryPackSetLimitsPerRequest(newMinYieldPerRequest *big.Int, newMinAssetsPerRequest *big.Int, newMaxAssetsPerRequest *big.Int) ([]byte, error) {
	return threeFAdapter.abi.Pack("setLimitsPerRequest", newMinYieldPerRequest, newMinAssetsPerRequest, newMaxAssetsPerRequest)
}

// PackSetOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x868adcae.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setOfferSigner(address newOfferSigner) returns()
func (threeFAdapter *ThreeFAdapter) PackSetOfferSigner(newOfferSigner common.Address) []byte {
	enc, err := threeFAdapter.abi.Pack("setOfferSigner", newOfferSigner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetOfferSigner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x868adcae.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setOfferSigner(address newOfferSigner) returns()
func (threeFAdapter *ThreeFAdapter) TryPackSetOfferSigner(newOfferSigner common.Address) ([]byte, error) {
	return threeFAdapter.abi.Pack("setOfferSigner", newOfferSigner)
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
// Solidity: function totalAssets() view returns(uint256 assets)
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
// Solidity: function totalAssets() view returns(uint256 assets)
func (threeFAdapter *ThreeFAdapter) TryPackTotalAssets() ([]byte, error) {
	return threeFAdapter.abi.Pack("totalAssets")
}

// UnpackTotalAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 assets)
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

// ThreeFAdapterFinalizeRequest represents a FinalizeRequest event raised by the ThreeFAdapter contract.
type ThreeFAdapterFinalizeRequest struct {
	Request common.Address
	Raw     *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterFinalizeRequestEventName = "FinalizeRequest"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterFinalizeRequest) ContractEventName() string {
	return ThreeFAdapterFinalizeRequestEventName
}

// UnpackFinalizeRequestEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event FinalizeRequest(address indexed request)
func (threeFAdapter *ThreeFAdapter) UnpackFinalizeRequestEvent(log *types.Log) (*ThreeFAdapterFinalizeRequest, error) {
	event := "FinalizeRequest"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterFinalizeRequest)
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

// ThreeFAdapterOnRequestConsumed represents a OnRequestConsumed event raised by the ThreeFAdapter contract.
type ThreeFAdapterOnRequestConsumed struct {
	Request         common.Address
	Offer           Offer
	PrincipalAssets *big.Int
	YieldAssets     *big.Int
	Raw             *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterOnRequestConsumedEventName = "OnRequestConsumed"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterOnRequestConsumed) ContractEventName() string {
	return ThreeFAdapterOnRequestConsumedEventName
}

// UnpackOnRequestConsumedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OnRequestConsumed(address indexed request, (address,uint256,uint256,uint256,uint256,bool) offer, uint256 principalAssets, uint256 yieldAssets)
func (threeFAdapter *ThreeFAdapter) UnpackOnRequestConsumedEvent(log *types.Log) (*ThreeFAdapterOnRequestConsumed, error) {
	event := "OnRequestConsumed"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterOnRequestConsumed)
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

// ThreeFAdapterSetLimitsPerRequest represents a SetLimitsPerRequest event raised by the ThreeFAdapter contract.
type ThreeFAdapterSetLimitsPerRequest struct {
	MinYieldPerRequest  *big.Int
	MinAssetsPerRequest *big.Int
	MaxAssetsPerRequest *big.Int
	Raw                 *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterSetLimitsPerRequestEventName = "SetLimitsPerRequest"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterSetLimitsPerRequest) ContractEventName() string {
	return ThreeFAdapterSetLimitsPerRequestEventName
}

// UnpackSetLimitsPerRequestEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetLimitsPerRequest(uint256 minYieldPerRequest, uint256 minAssetsPerRequest, uint256 maxAssetsPerRequest)
func (threeFAdapter *ThreeFAdapter) UnpackSetLimitsPerRequestEvent(log *types.Log) (*ThreeFAdapterSetLimitsPerRequest, error) {
	event := "SetLimitsPerRequest"
	if log.Topics[0] != threeFAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ThreeFAdapterSetLimitsPerRequest)
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
	OfferSigner common.Address
	Raw         *types.Log // Blockchain specific contextual infos
}

const ThreeFAdapterSetOfferSignerEventName = "SetOfferSigner"

// ContractEventName returns the user-defined event name.
func (ThreeFAdapterSetOfferSigner) ContractEventName() string {
	return ThreeFAdapterSetOfferSignerEventName
}

// UnpackSetOfferSignerEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetOfferSigner(address indexed offerSigner)
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
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["AlreadyRequest"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackAlreadyRequestError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["InsufficientAllocate"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackInsufficientAllocateError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["InvalidInitialization"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackInvalidInitializationError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["InvalidVault"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackInvalidVaultError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["NotFactory"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackNotFactoryError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["NotInitializing"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackNotInitializingError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["NotRequest"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackNotRequestError(raw[4:])
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
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["ReentrancyGuardReentrantCall"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackReentrancyGuardReentrantCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["TooLargeRequest"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackTooLargeRequestError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["TooLowYield"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackTooLowYieldError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["TooManyRequests"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackTooManyRequestsError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["TooSmallRequest"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackTooSmallRequestError(raw[4:])
	}
	if bytes.Equal(raw[:4], threeFAdapter.abi.Errors["WrongAsset"].ID.Bytes()[:4]) {
		return threeFAdapter.UnpackWrongAssetError(raw[4:])
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

// ThreeFAdapterAlreadyRequest represents a AlreadyRequest error raised by the ThreeFAdapter contract.
type ThreeFAdapterAlreadyRequest struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyRequest()
func ThreeFAdapterAlreadyRequestErrorID() common.Hash {
	return common.HexToHash("0x8d93e31a683f438f3632655955c279b3597d3f204f2f379582403ed351370f82")
}

// UnpackAlreadyRequestError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyRequest()
func (threeFAdapter *ThreeFAdapter) UnpackAlreadyRequestError(raw []byte) (*ThreeFAdapterAlreadyRequest, error) {
	out := new(ThreeFAdapterAlreadyRequest)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "AlreadyRequest", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterInsufficientAllocate represents a InsufficientAllocate error raised by the ThreeFAdapter contract.
type ThreeFAdapterInsufficientAllocate struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientAllocate()
func ThreeFAdapterInsufficientAllocateErrorID() common.Hash {
	return common.HexToHash("0xb128897f3cb0ff1be99d96c4772ed6c60ee2a8e88745c65f2a907980f83cad61")
}

// UnpackInsufficientAllocateError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientAllocate()
func (threeFAdapter *ThreeFAdapter) UnpackInsufficientAllocateError(raw []byte) (*ThreeFAdapterInsufficientAllocate, error) {
	out := new(ThreeFAdapterInsufficientAllocate)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "InsufficientAllocate", raw); err != nil {
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

// ThreeFAdapterNotRequest represents a NotRequest error raised by the ThreeFAdapter contract.
type ThreeFAdapterNotRequest struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotRequest()
func ThreeFAdapterNotRequestErrorID() common.Hash {
	return common.HexToHash("0x2b1697af70eb58a1fa466030f88f2dd8bedf01f69018be1b04b7747be7c762c7")
}

// UnpackNotRequestError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotRequest()
func (threeFAdapter *ThreeFAdapter) UnpackNotRequestError(raw []byte) (*ThreeFAdapterNotRequest, error) {
	out := new(ThreeFAdapterNotRequest)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "NotRequest", raw); err != nil {
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

// ThreeFAdapterTooLargeRequest represents a TooLargeRequest error raised by the ThreeFAdapter contract.
type ThreeFAdapterTooLargeRequest struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooLargeRequest()
func ThreeFAdapterTooLargeRequestErrorID() common.Hash {
	return common.HexToHash("0xd67cf587430cfcee33da8a888aff2db6f5f4b9a07d9bbd7bd784b7f9c8c59ab2")
}

// UnpackTooLargeRequestError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooLargeRequest()
func (threeFAdapter *ThreeFAdapter) UnpackTooLargeRequestError(raw []byte) (*ThreeFAdapterTooLargeRequest, error) {
	out := new(ThreeFAdapterTooLargeRequest)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "TooLargeRequest", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterTooLowYield represents a TooLowYield error raised by the ThreeFAdapter contract.
type ThreeFAdapterTooLowYield struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooLowYield()
func ThreeFAdapterTooLowYieldErrorID() common.Hash {
	return common.HexToHash("0xec84af7bf6cfbe9482148973e3ddd1942a9f2808b5f046694f7cfe46aa2ce953")
}

// UnpackTooLowYieldError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooLowYield()
func (threeFAdapter *ThreeFAdapter) UnpackTooLowYieldError(raw []byte) (*ThreeFAdapterTooLowYield, error) {
	out := new(ThreeFAdapterTooLowYield)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "TooLowYield", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterTooManyRequests represents a TooManyRequests error raised by the ThreeFAdapter contract.
type ThreeFAdapterTooManyRequests struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooManyRequests()
func ThreeFAdapterTooManyRequestsErrorID() common.Hash {
	return common.HexToHash("0x056d63471330a57f6c0d5cc835e9e9c3948af33484f6a6e592a2e3b11a42f713")
}

// UnpackTooManyRequestsError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooManyRequests()
func (threeFAdapter *ThreeFAdapter) UnpackTooManyRequestsError(raw []byte) (*ThreeFAdapterTooManyRequests, error) {
	out := new(ThreeFAdapterTooManyRequests)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "TooManyRequests", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterTooSmallRequest represents a TooSmallRequest error raised by the ThreeFAdapter contract.
type ThreeFAdapterTooSmallRequest struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooSmallRequest()
func ThreeFAdapterTooSmallRequestErrorID() common.Hash {
	return common.HexToHash("0x81b8a5cdb66b9b21248015d7ceda95a251199c135eabf6392fb671dcfa81ea3f")
}

// UnpackTooSmallRequestError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooSmallRequest()
func (threeFAdapter *ThreeFAdapter) UnpackTooSmallRequestError(raw []byte) (*ThreeFAdapterTooSmallRequest, error) {
	out := new(ThreeFAdapterTooSmallRequest)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "TooSmallRequest", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ThreeFAdapterWrongAsset represents a WrongAsset error raised by the ThreeFAdapter contract.
type ThreeFAdapterWrongAsset struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error WrongAsset()
func ThreeFAdapterWrongAssetErrorID() common.Hash {
	return common.HexToHash("0xf170c67fbef37d60daa2c8494fe22631cd135dc228bcc58a8c645c15992ea504")
}

// UnpackWrongAssetError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error WrongAsset()
func (threeFAdapter *ThreeFAdapter) UnpackWrongAssetError(raw []byte) (*ThreeFAdapterWrongAsset, error) {
	out := new(ThreeFAdapterWrongAsset)
	if err := threeFAdapter.abi.UnpackIntoInterface(out, "WrongAsset", raw); err != nil {
		return nil, err
	}
	return out, nil
}
