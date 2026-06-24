// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package request

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

// IRequestMetaData contains all meta data concerning the IRequest contract.
var IRequestMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"asset\",\"inputs\":[],\"outputs\":[{\"name\":\"assetAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"authorizeMinting\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"ytAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"consume\",\"inputs\":[{\"name\":\"offer\",\"type\":\"tuple\",\"internalType\":\"structOffer\",\"components\":[{\"name\":\"maker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedReturn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expiration\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"useCallback\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"ytAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isRepaid\",\"inputs\":[],\"outputs\":[{\"name\":\"repaid\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastMintTimestamp\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"mint\",\"inputs\":[{\"name\":\"maxPt\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"minYt\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"mintAuthorization\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ptAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"},{\"name\":\"ytAmount\",\"type\":\"uint128\",\"internalType\":\"uint128\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"mintToRepaidDelay\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pullFunds\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"repaidAvailableAt\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"repay\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMintToRepaidDelay\",\"inputs\":[{\"name\":\"mintToRepaidDelay_\",\"type\":\"uint40\",\"internalType\":\"uint40\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setRepaid\",\"inputs\":[{\"name\":\"minBalance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxBalance\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"syncRepaidStatus\",\"inputs\":[],\"outputs\":[{\"name\":\"repaid\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AuthorizedMinting\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"ptAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"ytAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"FundsPulled\",\"inputs\":[{\"name\":\"puller\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"MintToRepaidDelaySet\",\"inputs\":[{\"name\":\"mintToRepaidDelay\",\"type\":\"uint40\",\"indexed\":false,\"internalType\":\"uint40\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Repaid\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false}]",
	ID:  "IRequest",
}

// IRequest is an auto generated Go binding around an Ethereum contract.
type IRequest struct {
	abi abi.ABI
}

// NewIRequest creates a new instance of IRequest.
func NewIRequest() *IRequest {
	parsed, err := IRequestMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IRequest{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IRequest) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x38d52e0f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function asset() view returns(address assetAddress)
func (iRequest *IRequest) PackAsset() []byte {
	enc, err := iRequest.abi.Pack("asset")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAsset is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x38d52e0f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function asset() view returns(address assetAddress)
func (iRequest *IRequest) TryPackAsset() ([]byte, error) {
	return iRequest.abi.Pack("asset")
}

// UnpackAsset is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x38d52e0f.
//
// Solidity: function asset() view returns(address assetAddress)
func (iRequest *IRequest) UnpackAsset(data []byte) (common.Address, error) {
	out, err := iRequest.abi.Unpack("asset", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackAuthorizeMinting is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb1f88261.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function authorizeMinting(address to, uint128 ptAmount, uint128 ytAmount) returns()
func (iRequest *IRequest) PackAuthorizeMinting(to common.Address, ptAmount *big.Int, ytAmount *big.Int) []byte {
	enc, err := iRequest.abi.Pack("authorizeMinting", to, ptAmount, ytAmount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAuthorizeMinting is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb1f88261.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function authorizeMinting(address to, uint128 ptAmount, uint128 ytAmount) returns()
func (iRequest *IRequest) TryPackAuthorizeMinting(to common.Address, ptAmount *big.Int, ytAmount *big.Int) ([]byte, error) {
	return iRequest.abi.Pack("authorizeMinting", to, ptAmount, ytAmount)
}

// PackConsume is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x13bcaf67.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function consume((address,uint256,uint256,uint256,uint256,bool) offer, bytes signature, uint256 ptAmount) returns(uint256 ytAmount)
func (iRequest *IRequest) PackConsume(offer Offer, signature []byte, ptAmount *big.Int) []byte {
	enc, err := iRequest.abi.Pack("consume", offer, signature, ptAmount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackConsume is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x13bcaf67.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function consume((address,uint256,uint256,uint256,uint256,bool) offer, bytes signature, uint256 ptAmount) returns(uint256 ytAmount)
func (iRequest *IRequest) TryPackConsume(offer Offer, signature []byte, ptAmount *big.Int) ([]byte, error) {
	return iRequest.abi.Pack("consume", offer, signature, ptAmount)
}

// UnpackConsume is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x13bcaf67.
//
// Solidity: function consume((address,uint256,uint256,uint256,uint256,bool) offer, bytes signature, uint256 ptAmount) returns(uint256 ytAmount)
func (iRequest *IRequest) UnpackConsume(data []byte) (*big.Int, error) {
	out, err := iRequest.abi.Unpack("consume", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackIsRepaid is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6164051a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isRepaid() view returns(bool repaid)
func (iRequest *IRequest) PackIsRepaid() []byte {
	enc, err := iRequest.abi.Pack("isRepaid")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsRepaid is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6164051a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isRepaid() view returns(bool repaid)
func (iRequest *IRequest) TryPackIsRepaid() ([]byte, error) {
	return iRequest.abi.Pack("isRepaid")
}

// UnpackIsRepaid is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6164051a.
//
// Solidity: function isRepaid() view returns(bool repaid)
func (iRequest *IRequest) UnpackIsRepaid(data []byte) (bool, error) {
	out, err := iRequest.abi.Unpack("isRepaid", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackLastMintTimestamp is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8e80ff5d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function lastMintTimestamp() view returns(uint40)
func (iRequest *IRequest) PackLastMintTimestamp() []byte {
	enc, err := iRequest.abi.Pack("lastMintTimestamp")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLastMintTimestamp is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8e80ff5d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function lastMintTimestamp() view returns(uint40)
func (iRequest *IRequest) TryPackLastMintTimestamp() ([]byte, error) {
	return iRequest.abi.Pack("lastMintTimestamp")
}

// UnpackLastMintTimestamp is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8e80ff5d.
//
// Solidity: function lastMintTimestamp() view returns(uint40)
func (iRequest *IRequest) UnpackLastMintTimestamp(data []byte) (*big.Int, error) {
	out, err := iRequest.abi.Unpack("lastMintTimestamp", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMint is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdfe7a8e5.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function mint(uint128 maxPt, uint128 minYt) returns()
func (iRequest *IRequest) PackMint(maxPt *big.Int, minYt *big.Int) []byte {
	enc, err := iRequest.abi.Pack("mint", maxPt, minYt)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMint is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdfe7a8e5.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function mint(uint128 maxPt, uint128 minYt) returns()
func (iRequest *IRequest) TryPackMint(maxPt *big.Int, minYt *big.Int) ([]byte, error) {
	return iRequest.abi.Pack("mint", maxPt, minYt)
}

// PackMintAuthorization is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdc6c1d71.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function mintAuthorization(address account) view returns(uint128 ptAmount, uint128 ytAmount)
func (iRequest *IRequest) PackMintAuthorization(account common.Address) []byte {
	enc, err := iRequest.abi.Pack("mintAuthorization", account)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMintAuthorization is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xdc6c1d71.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function mintAuthorization(address account) view returns(uint128 ptAmount, uint128 ytAmount)
func (iRequest *IRequest) TryPackMintAuthorization(account common.Address) ([]byte, error) {
	return iRequest.abi.Pack("mintAuthorization", account)
}

// MintAuthorizationOutput serves as a container for the return parameters of contract
// method MintAuthorization.
type MintAuthorizationOutput struct {
	PtAmount *big.Int
	YtAmount *big.Int
}

// UnpackMintAuthorization is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xdc6c1d71.
//
// Solidity: function mintAuthorization(address account) view returns(uint128 ptAmount, uint128 ytAmount)
func (iRequest *IRequest) UnpackMintAuthorization(data []byte) (MintAuthorizationOutput, error) {
	out, err := iRequest.abi.Unpack("mintAuthorization", data)
	outstruct := new(MintAuthorizationOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.PtAmount = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.YtAmount = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackMintToRepaidDelay is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x80aed3e4.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function mintToRepaidDelay() view returns(uint40)
func (iRequest *IRequest) PackMintToRepaidDelay() []byte {
	enc, err := iRequest.abi.Pack("mintToRepaidDelay")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMintToRepaidDelay is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x80aed3e4.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function mintToRepaidDelay() view returns(uint40)
func (iRequest *IRequest) TryPackMintToRepaidDelay() ([]byte, error) {
	return iRequest.abi.Pack("mintToRepaidDelay")
}

// UnpackMintToRepaidDelay is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x80aed3e4.
//
// Solidity: function mintToRepaidDelay() view returns(uint40)
func (iRequest *IRequest) UnpackMintToRepaidDelay(data []byte) (*big.Int, error) {
	out, err := iRequest.abi.Unpack("mintToRepaidDelay", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackPullFunds is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5cb5727a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function pullFunds(uint256 amount, bytes data) returns()
func (iRequest *IRequest) PackPullFunds(amount *big.Int, data []byte) []byte {
	enc, err := iRequest.abi.Pack("pullFunds", amount, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPullFunds is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5cb5727a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function pullFunds(uint256 amount, bytes data) returns()
func (iRequest *IRequest) TryPackPullFunds(amount *big.Int, data []byte) ([]byte, error) {
	return iRequest.abi.Pack("pullFunds", amount, data)
}

// PackRepaidAvailableAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfe4b6faa.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function repaidAvailableAt() view returns(uint40)
func (iRequest *IRequest) PackRepaidAvailableAt() []byte {
	enc, err := iRequest.abi.Pack("repaidAvailableAt")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRepaidAvailableAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfe4b6faa.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function repaidAvailableAt() view returns(uint40)
func (iRequest *IRequest) TryPackRepaidAvailableAt() ([]byte, error) {
	return iRequest.abi.Pack("repaidAvailableAt")
}

// UnpackRepaidAvailableAt is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfe4b6faa.
//
// Solidity: function repaidAvailableAt() view returns(uint40)
func (iRequest *IRequest) UnpackRepaidAvailableAt(data []byte) (*big.Int, error) {
	out, err := iRequest.abi.Unpack("repaidAvailableAt", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackRepay is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x371fd8e6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function repay(uint256 amount) returns()
func (iRequest *IRequest) PackRepay(amount *big.Int) []byte {
	enc, err := iRequest.abi.Pack("repay", amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRepay is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x371fd8e6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function repay(uint256 amount) returns()
func (iRequest *IRequest) TryPackRepay(amount *big.Int) ([]byte, error) {
	return iRequest.abi.Pack("repay", amount)
}

// PackSetMintToRepaidDelay is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4e3f7fdb.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setMintToRepaidDelay(uint40 mintToRepaidDelay_) returns()
func (iRequest *IRequest) PackSetMintToRepaidDelay(mintToRepaidDelay *big.Int) []byte {
	enc, err := iRequest.abi.Pack("setMintToRepaidDelay", mintToRepaidDelay)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetMintToRepaidDelay is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x4e3f7fdb.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setMintToRepaidDelay(uint40 mintToRepaidDelay_) returns()
func (iRequest *IRequest) TryPackSetMintToRepaidDelay(mintToRepaidDelay *big.Int) ([]byte, error) {
	return iRequest.abi.Pack("setMintToRepaidDelay", mintToRepaidDelay)
}

// PackSetRepaid is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x512acc56.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setRepaid(uint256 minBalance, uint256 maxBalance) returns()
func (iRequest *IRequest) PackSetRepaid(minBalance *big.Int, maxBalance *big.Int) []byte {
	enc, err := iRequest.abi.Pack("setRepaid", minBalance, maxBalance)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetRepaid is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x512acc56.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setRepaid(uint256 minBalance, uint256 maxBalance) returns()
func (iRequest *IRequest) TryPackSetRepaid(minBalance *big.Int, maxBalance *big.Int) ([]byte, error) {
	return iRequest.abi.Pack("setRepaid", minBalance, maxBalance)
}

// PackSyncRepaidStatus is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf0d38777.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function syncRepaidStatus() returns(bool repaid)
func (iRequest *IRequest) PackSyncRepaidStatus() []byte {
	enc, err := iRequest.abi.Pack("syncRepaidStatus")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSyncRepaidStatus is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf0d38777.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function syncRepaidStatus() returns(bool repaid)
func (iRequest *IRequest) TryPackSyncRepaidStatus() ([]byte, error) {
	return iRequest.abi.Pack("syncRepaidStatus")
}

// UnpackSyncRepaidStatus is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xf0d38777.
//
// Solidity: function syncRepaidStatus() returns(bool repaid)
func (iRequest *IRequest) UnpackSyncRepaidStatus(data []byte) (bool, error) {
	out, err := iRequest.abi.Unpack("syncRepaidStatus", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// IRequestAuthorizedMinting represents a AuthorizedMinting event raised by the IRequest contract.
type IRequestAuthorizedMinting struct {
	To       common.Address
	PtAmount *big.Int
	YtAmount *big.Int
	Raw      *types.Log // Blockchain specific contextual infos
}

const IRequestAuthorizedMintingEventName = "AuthorizedMinting"

// ContractEventName returns the user-defined event name.
func (IRequestAuthorizedMinting) ContractEventName() string {
	return IRequestAuthorizedMintingEventName
}

// UnpackAuthorizedMintingEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event AuthorizedMinting(address indexed to, uint256 ptAmount, uint256 ytAmount)
func (iRequest *IRequest) UnpackAuthorizedMintingEvent(log *types.Log) (*IRequestAuthorizedMinting, error) {
	event := "AuthorizedMinting"
	if log.Topics[0] != iRequest.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IRequestAuthorizedMinting)
	if len(log.Data) > 0 {
		if err := iRequest.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iRequest.abi.Events[event].Inputs {
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

// IRequestFundsPulled represents a FundsPulled event raised by the IRequest contract.
type IRequestFundsPulled struct {
	Puller common.Address
	Amount *big.Int
	Raw    *types.Log // Blockchain specific contextual infos
}

const IRequestFundsPulledEventName = "FundsPulled"

// ContractEventName returns the user-defined event name.
func (IRequestFundsPulled) ContractEventName() string {
	return IRequestFundsPulledEventName
}

// UnpackFundsPulledEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event FundsPulled(address indexed puller, uint256 amount)
func (iRequest *IRequest) UnpackFundsPulledEvent(log *types.Log) (*IRequestFundsPulled, error) {
	event := "FundsPulled"
	if log.Topics[0] != iRequest.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IRequestFundsPulled)
	if len(log.Data) > 0 {
		if err := iRequest.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iRequest.abi.Events[event].Inputs {
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

// IRequestMintToRepaidDelaySet represents a MintToRepaidDelaySet event raised by the IRequest contract.
type IRequestMintToRepaidDelaySet struct {
	MintToRepaidDelay *big.Int
	Raw               *types.Log // Blockchain specific contextual infos
}

const IRequestMintToRepaidDelaySetEventName = "MintToRepaidDelaySet"

// ContractEventName returns the user-defined event name.
func (IRequestMintToRepaidDelaySet) ContractEventName() string {
	return IRequestMintToRepaidDelaySetEventName
}

// UnpackMintToRepaidDelaySetEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event MintToRepaidDelaySet(uint40 mintToRepaidDelay)
func (iRequest *IRequest) UnpackMintToRepaidDelaySetEvent(log *types.Log) (*IRequestMintToRepaidDelaySet, error) {
	event := "MintToRepaidDelaySet"
	if log.Topics[0] != iRequest.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IRequestMintToRepaidDelaySet)
	if len(log.Data) > 0 {
		if err := iRequest.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iRequest.abi.Events[event].Inputs {
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

// IRequestRepaid represents a Repaid event raised by the IRequest contract.
type IRequestRepaid struct {
	Amount *big.Int
	Raw    *types.Log // Blockchain specific contextual infos
}

const IRequestRepaidEventName = "Repaid"

// ContractEventName returns the user-defined event name.
func (IRequestRepaid) ContractEventName() string {
	return IRequestRepaidEventName
}

// UnpackRepaidEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Repaid(uint256 amount)
func (iRequest *IRequest) UnpackRepaidEvent(log *types.Log) (*IRequestRepaid, error) {
	event := "Repaid"
	if log.Topics[0] != iRequest.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IRequestRepaid)
	if len(log.Data) > 0 {
		if err := iRequest.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iRequest.abi.Events[event].Inputs {
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
