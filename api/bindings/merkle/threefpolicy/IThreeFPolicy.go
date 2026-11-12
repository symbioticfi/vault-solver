// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package threefpolicy

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

// IThreeFPolicyMetaData contains all meta data concerning the IThreeFPolicy contract.
var IThreeFPolicyMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"account\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"consume\",\"inputs\":[{\"name\":\"request\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"yieldAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minYieldPerRequest\",\"inputs\":[{\"name\":\"underlyingAsset\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"remainingAssets\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"underlyingAsset\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"Consume\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"underlyingAsset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"assets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false}]",
	ID:  "IThreeFPolicy",
}

// IThreeFPolicy is an auto generated Go binding around an Ethereum contract.
type IThreeFPolicy struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *IThreeFPolicy) GetABI() abi.ABI {
	return c.abi
}

// NewIThreeFPolicy creates a new instance of IThreeFPolicy.
func NewIThreeFPolicy() *IThreeFPolicy {
	parsed, err := IThreeFPolicyMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IThreeFPolicy{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IThreeFPolicy) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackAccount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5dab2420.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function account() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) PackAccount() []byte {
	enc, err := iThreeFPolicy.abi.Pack("account")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAccount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5dab2420.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function account() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) TryPackAccount() ([]byte, error) {
	return iThreeFPolicy.abi.Pack("account")
}

// UnpackAccount is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5dab2420.
//
// Solidity: function account() view returns(address)
func (iThreeFPolicy *IThreeFPolicy) UnpackAccount(data []byte) (common.Address, error) {
	out, err := iThreeFPolicy.abi.Unpack("account", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackConsume is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7edab8a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function consume(address request, uint256 assets, uint256 yieldAssets) returns()
func (iThreeFPolicy *IThreeFPolicy) PackConsume(request common.Address, assets *big.Int, yieldAssets *big.Int) []byte {
	enc, err := iThreeFPolicy.abi.Pack("consume", request, assets, yieldAssets)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackConsume is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7edab8a6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function consume(address request, uint256 assets, uint256 yieldAssets) returns()
func (iThreeFPolicy *IThreeFPolicy) TryPackConsume(request common.Address, assets *big.Int, yieldAssets *big.Int) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("consume", request, assets, yieldAssets)
}

// PackMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1fd317e3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minYieldPerRequest(address underlyingAsset) view returns(uint256)
func (iThreeFPolicy *IThreeFPolicy) PackMinYieldPerRequest(underlyingAsset common.Address) []byte {
	enc, err := iThreeFPolicy.abi.Pack("minYieldPerRequest", underlyingAsset)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinYieldPerRequest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1fd317e3.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minYieldPerRequest(address underlyingAsset) view returns(uint256)
func (iThreeFPolicy *IThreeFPolicy) TryPackMinYieldPerRequest(underlyingAsset common.Address) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("minYieldPerRequest", underlyingAsset)
}

// UnpackMinYieldPerRequest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1fd317e3.
//
// Solidity: function minYieldPerRequest(address underlyingAsset) view returns(uint256)
func (iThreeFPolicy *IThreeFPolicy) UnpackMinYieldPerRequest(data []byte) (*big.Int, error) {
	out, err := iThreeFPolicy.abi.Unpack("minYieldPerRequest", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x764ef286.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function remainingAssets(address asset, address underlyingAsset) view returns(uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) PackRemainingAssets(asset common.Address, underlyingAsset common.Address) []byte {
	enc, err := iThreeFPolicy.abi.Pack("remainingAssets", asset, underlyingAsset)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRemainingAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x764ef286.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function remainingAssets(address asset, address underlyingAsset) view returns(uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) TryPackRemainingAssets(asset common.Address, underlyingAsset common.Address) ([]byte, error) {
	return iThreeFPolicy.abi.Pack("remainingAssets", asset, underlyingAsset)
}

// UnpackRemainingAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x764ef286.
//
// Solidity: function remainingAssets(address asset, address underlyingAsset) view returns(uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) UnpackRemainingAssets(data []byte) (*big.Int, error) {
	out, err := iThreeFPolicy.abi.Unpack("remainingAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// IThreeFPolicyConsume represents a Consume event raised by the IThreeFPolicy contract.
type IThreeFPolicyConsume struct {
	Asset           common.Address
	UnderlyingAsset common.Address
	Assets          *big.Int
	Raw             *types.Log // Blockchain specific contextual infos
}

const IThreeFPolicyConsumeEventName = "Consume"

// ContractEventName returns the user-defined event name.
func (IThreeFPolicyConsume) ContractEventName() string {
	return IThreeFPolicyConsumeEventName
}

// UnpackConsumeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Consume(address indexed asset, address indexed underlyingAsset, uint256 assets)
func (iThreeFPolicy *IThreeFPolicy) UnpackConsumeEvent(log *types.Log) (*IThreeFPolicyConsume, error) {
	event := "Consume"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != iThreeFPolicy.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(IThreeFPolicyConsume)
	if len(log.Data) > 0 {
		if err := iThreeFPolicy.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iThreeFPolicy.abi.Events[event].Inputs {
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
