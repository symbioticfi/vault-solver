// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package adapter

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// ILiquidLaneAdapterDiscount is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterDiscount struct {
	TokenToRedeem common.Address
	Discount      *big.Int
	Signer        common.Address
	Protocol      common.Address
	Nonce         *big.Int
	Deadline      *big.Int
}

// ILiquidLaneAdapterDiscountSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterDiscountSwap struct {
	Discount         ILiquidLaneAdapterDiscount
	SignerSignature  []byte
	ProtocolDeadline *big.Int
}

// ILiquidLaneAdapterInitParams is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterInitParams struct {
	Pauser   common.Address
	Unpauser common.Address
}

// ILiquidLaneAdapterSignedSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterSignedSwap struct {
	Recipient common.Address
	TokenIn   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
	Caller    common.Address
	Signer    common.Address
	Nonce     *big.Int
	Deadline  *big.Int
}

// ILiquidLaneAdapterSwap is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneAdapterSwap struct {
	Recipient common.Address
	TokenIn   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

// LiquidLaneAdapterMetaData contains all meta data concerning the LiquidLaneAdapter contract.
var LiquidLaneAdapterMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"vaultFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"adapterFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"accountRegistry\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"accounts\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"acquireBalance\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"marketMaker\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"addTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deallocate\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"deallocated\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"depositToAcquire\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAmountOut\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getMaxRate\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTokensToRedeemLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"invalidateNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isFiller\",\"inputs\":[{\"name\":\"marketMaker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isUsedNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"limit\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"marketMaker\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"marketMakerCanAcquire\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ppm\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pause\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"paused\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pauser\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"receiver\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"removeTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestDeallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setFiller\",\"inputs\":[{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setLimit\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMarketMaker\",\"inputs\":[{\"name\":\"newMarketMaker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newCanAcquire\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMinDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMinDiscount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPauser\",\"inputs\":[{\"name\":\"newPauser\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setReceiver\",\"inputs\":[{\"name\":\"newReceiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setUnpauser\",\"inputs\":[{\"name\":\"newUnpauser\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"signedSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.SignedSwap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"tokensToRedeem\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"unpause\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"unpauser\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"vault\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawToAcquire\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AddTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DepositToAcquire\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DoSwap\",\"inputs\":[{\"name\":\"swap\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialize\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structILiquidLaneAdapter.InitParams\",\"components\":[{\"name\":\"pauser\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"unpauser\",\"type\":\"address\",\"internalType\":\"address\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"InvalidateNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Paused\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RemoveTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetFiller\",\"inputs\":[{\"name\":\"marketMaker\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"isAuthorized\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetLimit\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newLimit\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMarketMaker\",\"inputs\":[{\"name\":\"newMarketMaker\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newCanAcquire\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMinDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newMinDiscount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetPauser\",\"inputs\":[{\"name\":\"newPauser\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetReceiver\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetUnpauser\",\"inputs\":[{\"name\":\"newUnpauser\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetVault\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Unpaused\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"WithdrawToAcquire\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AccountHasAssets\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyUsedNonce\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DepositNotAllowed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"EnforcedPause\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExpectedPause\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExpiredSwap\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientAllocate\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAccount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDiscount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOracle\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidReceiver\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidShortString\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSwapRate\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTokenToRedeem\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"LimitExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"StringTooLong\",\"inputs\":[{\"name\":\"str\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"TooManyTokensToRedeem\",\"inputs\":[]}]",
}

// LiquidLaneAdapterABI is the input ABI used to generate the binding from.
// Deprecated: Use LiquidLaneAdapterMetaData.ABI instead.
var LiquidLaneAdapterABI = LiquidLaneAdapterMetaData.ABI

// LiquidLaneAdapter is an auto generated Go binding around an Ethereum contract.
type LiquidLaneAdapter struct {
	LiquidLaneAdapterCaller     // Read-only binding to the contract
	LiquidLaneAdapterTransactor // Write-only binding to the contract
	LiquidLaneAdapterFilterer   // Log filterer for contract events
}

// LiquidLaneAdapterCaller is an auto generated read-only Go binding around an Ethereum contract.
type LiquidLaneAdapterCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LiquidLaneAdapterTransactor is an auto generated write-only Go binding around an Ethereum contract.
type LiquidLaneAdapterTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LiquidLaneAdapterFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type LiquidLaneAdapterFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LiquidLaneAdapterSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type LiquidLaneAdapterSession struct {
	Contract     *LiquidLaneAdapter // Generic contract binding to set the session for
	CallOpts     bind.CallOpts      // Call options to use throughout this session
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// LiquidLaneAdapterCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type LiquidLaneAdapterCallerSession struct {
	Contract *LiquidLaneAdapterCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts            // Call options to use throughout this session
}

// LiquidLaneAdapterTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type LiquidLaneAdapterTransactorSession struct {
	Contract     *LiquidLaneAdapterTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts            // Transaction auth options to use throughout this session
}

// LiquidLaneAdapterRaw is an auto generated low-level Go binding around an Ethereum contract.
type LiquidLaneAdapterRaw struct {
	Contract *LiquidLaneAdapter // Generic contract binding to access the raw methods on
}

// LiquidLaneAdapterCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type LiquidLaneAdapterCallerRaw struct {
	Contract *LiquidLaneAdapterCaller // Generic read-only contract binding to access the raw methods on
}

// LiquidLaneAdapterTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type LiquidLaneAdapterTransactorRaw struct {
	Contract *LiquidLaneAdapterTransactor // Generic write-only contract binding to access the raw methods on
}

// NewLiquidLaneAdapter creates a new instance of LiquidLaneAdapter, bound to a specific deployed contract.
func NewLiquidLaneAdapter(address common.Address, backend bind.ContractBackend) (*LiquidLaneAdapter, error) {
	contract, err := bindLiquidLaneAdapter(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapter{LiquidLaneAdapterCaller: LiquidLaneAdapterCaller{contract: contract}, LiquidLaneAdapterTransactor: LiquidLaneAdapterTransactor{contract: contract}, LiquidLaneAdapterFilterer: LiquidLaneAdapterFilterer{contract: contract}}, nil
}

// NewLiquidLaneAdapterCaller creates a new read-only instance of LiquidLaneAdapter, bound to a specific deployed contract.
func NewLiquidLaneAdapterCaller(address common.Address, caller bind.ContractCaller) (*LiquidLaneAdapterCaller, error) {
	contract, err := bindLiquidLaneAdapter(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterCaller{contract: contract}, nil
}

// NewLiquidLaneAdapterTransactor creates a new write-only instance of LiquidLaneAdapter, bound to a specific deployed contract.
func NewLiquidLaneAdapterTransactor(address common.Address, transactor bind.ContractTransactor) (*LiquidLaneAdapterTransactor, error) {
	contract, err := bindLiquidLaneAdapter(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterTransactor{contract: contract}, nil
}

// NewLiquidLaneAdapterFilterer creates a new log filterer instance of LiquidLaneAdapter, bound to a specific deployed contract.
func NewLiquidLaneAdapterFilterer(address common.Address, filterer bind.ContractFilterer) (*LiquidLaneAdapterFilterer, error) {
	contract, err := bindLiquidLaneAdapter(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterFilterer{contract: contract}, nil
}

// bindLiquidLaneAdapter binds a generic wrapper to an already deployed contract.
func bindLiquidLaneAdapter(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := LiquidLaneAdapterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_LiquidLaneAdapter *LiquidLaneAdapterRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _LiquidLaneAdapter.Contract.LiquidLaneAdapterCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_LiquidLaneAdapter *LiquidLaneAdapterRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.LiquidLaneAdapterTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_LiquidLaneAdapter *LiquidLaneAdapterRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.LiquidLaneAdapterTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _LiquidLaneAdapter.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.contract.Transact(opts, method, params...)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) FACTORY(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "FACTORY")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) FACTORY() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.FACTORY(&_LiquidLaneAdapter.CallOpts)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) FACTORY() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.FACTORY(&_LiquidLaneAdapter.CallOpts)
}

// Accounts is a free data retrieval call binding the contract method 0x5e5c06e2.
//
// Solidity: function accounts(address tokenToRedeem) view returns(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Accounts(opts *bind.CallOpts, tokenToRedeem common.Address) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "accounts", tokenToRedeem)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Accounts is a free data retrieval call binding the contract method 0x5e5c06e2.
//
// Solidity: function accounts(address tokenToRedeem) view returns(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Accounts(tokenToRedeem common.Address) (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Accounts(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// Accounts is a free data retrieval call binding the contract method 0x5e5c06e2.
//
// Solidity: function accounts(address tokenToRedeem) view returns(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Accounts(tokenToRedeem common.Address) (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Accounts(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// AcquireBalance is a free data retrieval call binding the contract method 0x5cf29c90.
//
// Solidity: function acquireBalance(address tokenToRedeem, address marketMaker) view returns(uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) AcquireBalance(opts *bind.CallOpts, tokenToRedeem common.Address, marketMaker common.Address) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "acquireBalance", tokenToRedeem, marketMaker)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AcquireBalance is a free data retrieval call binding the contract method 0x5cf29c90.
//
// Solidity: function acquireBalance(address tokenToRedeem, address marketMaker) view returns(uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) AcquireBalance(tokenToRedeem common.Address, marketMaker common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.AcquireBalance(&_LiquidLaneAdapter.CallOpts, tokenToRedeem, marketMaker)
}

// AcquireBalance is a free data retrieval call binding the contract method 0x5cf29c90.
//
// Solidity: function acquireBalance(address tokenToRedeem, address marketMaker) view returns(uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) AcquireBalance(tokenToRedeem common.Address, marketMaker common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.AcquireBalance(&_LiquidLaneAdapter.CallOpts, tokenToRedeem, marketMaker)
}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Allocatable(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "allocatable")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Allocatable() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.Allocatable(&_LiquidLaneAdapter.CallOpts)
}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Allocatable() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.Allocatable(&_LiquidLaneAdapter.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Eip712Domain(opts *bind.CallOpts) (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "eip712Domain")

	outstruct := new(struct {
		Fields            [1]byte
		Name              string
		Version           string
		ChainId           *big.Int
		VerifyingContract common.Address
		Salt              [32]byte
		Extensions        []*big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)

	return *outstruct, err

}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _LiquidLaneAdapter.Contract.Eip712Domain(&_LiquidLaneAdapter.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _LiquidLaneAdapter.Contract.Eip712Domain(&_LiquidLaneAdapter.CallOpts)
}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) FreeAssets(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "freeAssets")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) FreeAssets() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.FreeAssets(&_LiquidLaneAdapter.CallOpts)
}

// FreeAssets is a free data retrieval call binding the contract method 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) FreeAssets() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.FreeAssets(&_LiquidLaneAdapter.CallOpts)
}

// GetAmountOut is a free data retrieval call binding the contract method 0xca706bcf.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) GetAmountOut(opts *bind.CallOpts, tokenToRedeem common.Address, amountIn *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "getAmountOut", tokenToRedeem, amountIn)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetAmountOut is a free data retrieval call binding the contract method 0xca706bcf.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) GetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.GetAmountOut(&_LiquidLaneAdapter.CallOpts, tokenToRedeem, amountIn)
}

// GetAmountOut is a free data retrieval call binding the contract method 0xca706bcf.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) GetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.GetAmountOut(&_LiquidLaneAdapter.CallOpts, tokenToRedeem, amountIn)
}

// GetMaxRate is a free data retrieval call binding the contract method 0xa99c53b3.
//
// Solidity: function getMaxRate(address tokenToRedeem) view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) GetMaxRate(opts *bind.CallOpts, tokenToRedeem common.Address) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "getMaxRate", tokenToRedeem)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMaxRate is a free data retrieval call binding the contract method 0xa99c53b3.
//
// Solidity: function getMaxRate(address tokenToRedeem) view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) GetMaxRate(tokenToRedeem common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.GetMaxRate(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// GetMaxRate is a free data retrieval call binding the contract method 0xa99c53b3.
//
// Solidity: function getMaxRate(address tokenToRedeem) view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) GetMaxRate(tokenToRedeem common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.GetMaxRate(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// GetTokensToRedeemLength is a free data retrieval call binding the contract method 0x55d931bf.
//
// Solidity: function getTokensToRedeemLength() view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) GetTokensToRedeemLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "getTokensToRedeemLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetTokensToRedeemLength is a free data retrieval call binding the contract method 0x55d931bf.
//
// Solidity: function getTokensToRedeemLength() view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) GetTokensToRedeemLength() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.GetTokensToRedeemLength(&_LiquidLaneAdapter.CallOpts)
}

// GetTokensToRedeemLength is a free data retrieval call binding the contract method 0x55d931bf.
//
// Solidity: function getTokensToRedeemLength() view returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) GetTokensToRedeemLength() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.GetTokensToRedeemLength(&_LiquidLaneAdapter.CallOpts)
}

// IsFiller is a free data retrieval call binding the contract method 0xb0f9fe6d.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) IsFiller(opts *bind.CallOpts, marketMaker common.Address, filler common.Address) (bool, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "isFiller", marketMaker, filler)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsFiller is a free data retrieval call binding the contract method 0xb0f9fe6d.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) IsFiller(marketMaker common.Address, filler common.Address) (bool, error) {
	return _LiquidLaneAdapter.Contract.IsFiller(&_LiquidLaneAdapter.CallOpts, marketMaker, filler)
}

// IsFiller is a free data retrieval call binding the contract method 0xb0f9fe6d.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) IsFiller(marketMaker common.Address, filler common.Address) (bool, error) {
	return _LiquidLaneAdapter.Contract.IsFiller(&_LiquidLaneAdapter.CallOpts, marketMaker, filler)
}

// IsUsedNonce is a free data retrieval call binding the contract method 0x0ee60fa7.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) IsUsedNonce(opts *bind.CallOpts, tokenToRedeem common.Address, nonce *big.Int) (bool, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "isUsedNonce", tokenToRedeem, nonce)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsUsedNonce is a free data retrieval call binding the contract method 0x0ee60fa7.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) IsUsedNonce(tokenToRedeem common.Address, nonce *big.Int) (bool, error) {
	return _LiquidLaneAdapter.Contract.IsUsedNonce(&_LiquidLaneAdapter.CallOpts, tokenToRedeem, nonce)
}

// IsUsedNonce is a free data retrieval call binding the contract method 0x0ee60fa7.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) IsUsedNonce(tokenToRedeem common.Address, nonce *big.Int) (bool, error) {
	return _LiquidLaneAdapter.Contract.IsUsedNonce(&_LiquidLaneAdapter.CallOpts, tokenToRedeem, nonce)
}

// Limit is a free data retrieval call binding the contract method 0xd8797262.
//
// Solidity: function limit(address tokenToRedeem) view returns(uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Limit(opts *bind.CallOpts, tokenToRedeem common.Address) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "limit", tokenToRedeem)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Limit is a free data retrieval call binding the contract method 0xd8797262.
//
// Solidity: function limit(address tokenToRedeem) view returns(uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Limit(tokenToRedeem common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.Limit(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// Limit is a free data retrieval call binding the contract method 0xd8797262.
//
// Solidity: function limit(address tokenToRedeem) view returns(uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Limit(tokenToRedeem common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.Limit(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// MarketMaker is a free data retrieval call binding the contract method 0x1f21f9af.
//
// Solidity: function marketMaker() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) MarketMaker(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "marketMaker")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// MarketMaker is a free data retrieval call binding the contract method 0x1f21f9af.
//
// Solidity: function marketMaker() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) MarketMaker() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.MarketMaker(&_LiquidLaneAdapter.CallOpts)
}

// MarketMaker is a free data retrieval call binding the contract method 0x1f21f9af.
//
// Solidity: function marketMaker() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) MarketMaker() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.MarketMaker(&_LiquidLaneAdapter.CallOpts)
}

// MarketMakerCanAcquire is a free data retrieval call binding the contract method 0xe1d594cb.
//
// Solidity: function marketMakerCanAcquire() view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) MarketMakerCanAcquire(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "marketMakerCanAcquire")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// MarketMakerCanAcquire is a free data retrieval call binding the contract method 0xe1d594cb.
//
// Solidity: function marketMakerCanAcquire() view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) MarketMakerCanAcquire() (bool, error) {
	return _LiquidLaneAdapter.Contract.MarketMakerCanAcquire(&_LiquidLaneAdapter.CallOpts)
}

// MarketMakerCanAcquire is a free data retrieval call binding the contract method 0xe1d594cb.
//
// Solidity: function marketMakerCanAcquire() view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) MarketMakerCanAcquire() (bool, error) {
	return _LiquidLaneAdapter.Contract.MarketMakerCanAcquire(&_LiquidLaneAdapter.CallOpts)
}

// MinDiscount is a free data retrieval call binding the contract method 0xd9104c14.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 ppm)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) MinDiscount(opts *bind.CallOpts, tokenToRedeem common.Address) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "minDiscount", tokenToRedeem)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MinDiscount is a free data retrieval call binding the contract method 0xd9104c14.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 ppm)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) MinDiscount(tokenToRedeem common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.MinDiscount(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// MinDiscount is a free data retrieval call binding the contract method 0xd9104c14.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 ppm)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) MinDiscount(tokenToRedeem common.Address) (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.MinDiscount(&_LiquidLaneAdapter.CallOpts, tokenToRedeem)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Owner() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Owner(&_LiquidLaneAdapter.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Owner() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Owner(&_LiquidLaneAdapter.CallOpts)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Paused(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "paused")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Paused() (bool, error) {
	return _LiquidLaneAdapter.Contract.Paused(&_LiquidLaneAdapter.CallOpts)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Paused() (bool, error) {
	return _LiquidLaneAdapter.Contract.Paused(&_LiquidLaneAdapter.CallOpts)
}

// Pauser is a free data retrieval call binding the contract method 0x9fd0506d.
//
// Solidity: function pauser() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Pauser(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "pauser")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Pauser is a free data retrieval call binding the contract method 0x9fd0506d.
//
// Solidity: function pauser() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Pauser() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Pauser(&_LiquidLaneAdapter.CallOpts)
}

// Pauser is a free data retrieval call binding the contract method 0x9fd0506d.
//
// Solidity: function pauser() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Pauser() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Pauser(&_LiquidLaneAdapter.CallOpts)
}

// Receiver is a free data retrieval call binding the contract method 0x9da41802.
//
// Solidity: function receiver(address who) view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Receiver(opts *bind.CallOpts, who common.Address) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "receiver", who)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Receiver is a free data retrieval call binding the contract method 0x9da41802.
//
// Solidity: function receiver(address who) view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Receiver(who common.Address) (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Receiver(&_LiquidLaneAdapter.CallOpts, who)
}

// Receiver is a free data retrieval call binding the contract method 0x9da41802.
//
// Solidity: function receiver(address who) view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Receiver(who common.Address) (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Receiver(&_LiquidLaneAdapter.CallOpts, who)
}

// TokensToRedeem is a free data retrieval call binding the contract method 0x3d2a61ce.
//
// Solidity: function tokensToRedeem(uint256 ) view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) TokensToRedeem(opts *bind.CallOpts, arg0 *big.Int) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "tokensToRedeem", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// TokensToRedeem is a free data retrieval call binding the contract method 0x3d2a61ce.
//
// Solidity: function tokensToRedeem(uint256 ) view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) TokensToRedeem(arg0 *big.Int) (common.Address, error) {
	return _LiquidLaneAdapter.Contract.TokensToRedeem(&_LiquidLaneAdapter.CallOpts, arg0)
}

// TokensToRedeem is a free data retrieval call binding the contract method 0x3d2a61ce.
//
// Solidity: function tokensToRedeem(uint256 ) view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) TokensToRedeem(arg0 *big.Int) (common.Address, error) {
	return _LiquidLaneAdapter.Contract.TokensToRedeem(&_LiquidLaneAdapter.CallOpts, arg0)
}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) TotalAssets(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "totalAssets")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) TotalAssets() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.TotalAssets(&_LiquidLaneAdapter.CallOpts)
}

// TotalAssets is a free data retrieval call binding the contract method 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) TotalAssets() (*big.Int, error) {
	return _LiquidLaneAdapter.Contract.TotalAssets(&_LiquidLaneAdapter.CallOpts)
}

// Unpauser is a free data retrieval call binding the contract method 0xeab66d7a.
//
// Solidity: function unpauser() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Unpauser(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "unpauser")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Unpauser is a free data retrieval call binding the contract method 0xeab66d7a.
//
// Solidity: function unpauser() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Unpauser() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Unpauser(&_LiquidLaneAdapter.CallOpts)
}

// Unpauser is a free data retrieval call binding the contract method 0xeab66d7a.
//
// Solidity: function unpauser() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Unpauser() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Unpauser(&_LiquidLaneAdapter.CallOpts)
}

// Vault is a free data retrieval call binding the contract method 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Vault(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "vault")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Vault is a free data retrieval call binding the contract method 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Vault() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Vault(&_LiquidLaneAdapter.CallOpts)
}

// Vault is a free data retrieval call binding the contract method 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Vault() (common.Address, error) {
	return _LiquidLaneAdapter.Contract.Vault(&_LiquidLaneAdapter.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_LiquidLaneAdapter *LiquidLaneAdapterCaller) Version(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _LiquidLaneAdapter.contract.Call(opts, &out, "version")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Version() (uint64, error) {
	return _LiquidLaneAdapter.Contract.Version(&_LiquidLaneAdapter.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_LiquidLaneAdapter *LiquidLaneAdapterCallerSession) Version() (uint64, error) {
	return _LiquidLaneAdapter.Contract.Version(&_LiquidLaneAdapter.CallOpts)
}

// AddTokenToRedeem is a paid mutator transaction binding the contract method 0xc9b3c58d.
//
// Solidity: function addTokenToRedeem(address tokenToRedeem) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) AddTokenToRedeem(opts *bind.TransactOpts, tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "addTokenToRedeem", tokenToRedeem)
}

// AddTokenToRedeem is a paid mutator transaction binding the contract method 0xc9b3c58d.
//
// Solidity: function addTokenToRedeem(address tokenToRedeem) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) AddTokenToRedeem(tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.AddTokenToRedeem(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem)
}

// AddTokenToRedeem is a paid mutator transaction binding the contract method 0xc9b3c58d.
//
// Solidity: function addTokenToRedeem(address tokenToRedeem) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) AddTokenToRedeem(tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.AddTokenToRedeem(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Allocate(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "allocate", amount)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Allocate(amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Allocate(&_LiquidLaneAdapter.TransactOpts, amount)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Allocate(amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Allocate(&_LiquidLaneAdapter.TransactOpts, amount)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 ) returns(uint256 deallocated)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Deallocate(opts *bind.TransactOpts, arg0 *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "deallocate", arg0)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 ) returns(uint256 deallocated)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Deallocate(arg0 *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Deallocate(&_LiquidLaneAdapter.TransactOpts, arg0)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 ) returns(uint256 deallocated)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Deallocate(arg0 *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Deallocate(&_LiquidLaneAdapter.TransactOpts, arg0)
}

// DepositToAcquire is a paid mutator transaction binding the contract method 0xd6c83d24.
//
// Solidity: function depositToAcquire(address tokenToRedeem, uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) DepositToAcquire(opts *bind.TransactOpts, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "depositToAcquire", tokenToRedeem, amount)
}

// DepositToAcquire is a paid mutator transaction binding the contract method 0xd6c83d24.
//
// Solidity: function depositToAcquire(address tokenToRedeem, uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) DepositToAcquire(tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.DepositToAcquire(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, amount)
}

// DepositToAcquire is a paid mutator transaction binding the contract method 0xd6c83d24.
//
// Solidity: function depositToAcquire(address tokenToRedeem, uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) DepositToAcquire(tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.DepositToAcquire(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, amount)
}

// GetMaxAssets is a paid mutator transaction binding the contract method 0x22135549.
//
// Solidity: function getMaxAssets(address tokenToRedeem) returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) GetMaxAssets(opts *bind.TransactOpts, tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "getMaxAssets", tokenToRedeem)
}

// GetMaxAssets is a paid mutator transaction binding the contract method 0x22135549.
//
// Solidity: function getMaxAssets(address tokenToRedeem) returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) GetMaxAssets(tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.GetMaxAssets(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem)
}

// GetMaxAssets is a paid mutator transaction binding the contract method 0x22135549.
//
// Solidity: function getMaxAssets(address tokenToRedeem) returns(uint256 assets)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) GetMaxAssets(tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.GetMaxAssets(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Initialize(opts *bind.TransactOpts, initialVersion uint64, owner_ common.Address, data []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "initialize", initialVersion, owner_, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Initialize(initialVersion uint64, owner_ common.Address, data []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Initialize(&_LiquidLaneAdapter.TransactOpts, initialVersion, owner_, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner_, bytes data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Initialize(initialVersion uint64, owner_ common.Address, data []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Initialize(&_LiquidLaneAdapter.TransactOpts, initialVersion, owner_, data)
}

// InvalidateNonce is a paid mutator transaction binding the contract method 0x0d3762b5.
//
// Solidity: function invalidateNonce(address tokenToRedeem, uint256 nonce) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) InvalidateNonce(opts *bind.TransactOpts, tokenToRedeem common.Address, nonce *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "invalidateNonce", tokenToRedeem, nonce)
}

// InvalidateNonce is a paid mutator transaction binding the contract method 0x0d3762b5.
//
// Solidity: function invalidateNonce(address tokenToRedeem, uint256 nonce) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) InvalidateNonce(tokenToRedeem common.Address, nonce *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.InvalidateNonce(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, nonce)
}

// InvalidateNonce is a paid mutator transaction binding the contract method 0x0d3762b5.
//
// Solidity: function invalidateNonce(address tokenToRedeem, uint256 nonce) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) InvalidateNonce(tokenToRedeem common.Address, nonce *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.InvalidateNonce(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, nonce)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Migrate(opts *bind.TransactOpts, newVersion uint64, data []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "migrate", newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Migrate(&_LiquidLaneAdapter.TransactOpts, newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Migrate(&_LiquidLaneAdapter.TransactOpts, newVersion, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Multicall(opts *bind.TransactOpts, data [][]byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "multicall", data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Multicall(&_LiquidLaneAdapter.TransactOpts, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Multicall(&_LiquidLaneAdapter.TransactOpts, data)
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Pause(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "pause")
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Pause() (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Pause(&_LiquidLaneAdapter.TransactOpts)
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Pause() (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Pause(&_LiquidLaneAdapter.TransactOpts)
}

// RemoveTokenToRedeem is a paid mutator transaction binding the contract method 0xe250fef7.
//
// Solidity: function removeTokenToRedeem(address tokenToRedeem) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) RemoveTokenToRedeem(opts *bind.TransactOpts, tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "removeTokenToRedeem", tokenToRedeem)
}

// RemoveTokenToRedeem is a paid mutator transaction binding the contract method 0xe250fef7.
//
// Solidity: function removeTokenToRedeem(address tokenToRedeem) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) RemoveTokenToRedeem(tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.RemoveTokenToRedeem(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem)
}

// RemoveTokenToRedeem is a paid mutator transaction binding the contract method 0xe250fef7.
//
// Solidity: function removeTokenToRedeem(address tokenToRedeem) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) RemoveTokenToRedeem(tokenToRedeem common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.RemoveTokenToRedeem(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) RenounceOwnership() (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.RenounceOwnership(&_LiquidLaneAdapter.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.RenounceOwnership(&_LiquidLaneAdapter.TransactOpts)
}

// RequestDeallocate is a paid mutator transaction binding the contract method 0xf79f679d.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) RequestDeallocate(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "requestDeallocate", amount)
}

// RequestDeallocate is a paid mutator transaction binding the contract method 0xf79f679d.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) RequestDeallocate(amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.RequestDeallocate(&_LiquidLaneAdapter.TransactOpts, amount)
}

// RequestDeallocate is a paid mutator transaction binding the contract method 0xf79f679d.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) RequestDeallocate(amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.RequestDeallocate(&_LiquidLaneAdapter.TransactOpts, amount)
}

// SetFiller is a paid mutator transaction binding the contract method 0xc6af7897.
//
// Solidity: function setFiller(address filler, bool status) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) SetFiller(opts *bind.TransactOpts, filler common.Address, status bool) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "setFiller", filler, status)
}

// SetFiller is a paid mutator transaction binding the contract method 0xc6af7897.
//
// Solidity: function setFiller(address filler, bool status) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) SetFiller(filler common.Address, status bool) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetFiller(&_LiquidLaneAdapter.TransactOpts, filler, status)
}

// SetFiller is a paid mutator transaction binding the contract method 0xc6af7897.
//
// Solidity: function setFiller(address filler, bool status) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) SetFiller(filler common.Address, status bool) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetFiller(&_LiquidLaneAdapter.TransactOpts, filler, status)
}

// SetLimit is a paid mutator transaction binding the contract method 0x36db43b5.
//
// Solidity: function setLimit(address tokenToRedeem, uint256 newLimit) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) SetLimit(opts *bind.TransactOpts, tokenToRedeem common.Address, newLimit *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "setLimit", tokenToRedeem, newLimit)
}

// SetLimit is a paid mutator transaction binding the contract method 0x36db43b5.
//
// Solidity: function setLimit(address tokenToRedeem, uint256 newLimit) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) SetLimit(tokenToRedeem common.Address, newLimit *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetLimit(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, newLimit)
}

// SetLimit is a paid mutator transaction binding the contract method 0x36db43b5.
//
// Solidity: function setLimit(address tokenToRedeem, uint256 newLimit) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) SetLimit(tokenToRedeem common.Address, newLimit *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetLimit(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, newLimit)
}

// SetMarketMaker is a paid mutator transaction binding the contract method 0xd6e2df05.
//
// Solidity: function setMarketMaker(address newMarketMaker, bool newCanAcquire) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) SetMarketMaker(opts *bind.TransactOpts, newMarketMaker common.Address, newCanAcquire bool) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "setMarketMaker", newMarketMaker, newCanAcquire)
}

// SetMarketMaker is a paid mutator transaction binding the contract method 0xd6e2df05.
//
// Solidity: function setMarketMaker(address newMarketMaker, bool newCanAcquire) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) SetMarketMaker(newMarketMaker common.Address, newCanAcquire bool) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetMarketMaker(&_LiquidLaneAdapter.TransactOpts, newMarketMaker, newCanAcquire)
}

// SetMarketMaker is a paid mutator transaction binding the contract method 0xd6e2df05.
//
// Solidity: function setMarketMaker(address newMarketMaker, bool newCanAcquire) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) SetMarketMaker(newMarketMaker common.Address, newCanAcquire bool) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetMarketMaker(&_LiquidLaneAdapter.TransactOpts, newMarketMaker, newCanAcquire)
}

// SetMinDiscount is a paid mutator transaction binding the contract method 0x0517ef9a.
//
// Solidity: function setMinDiscount(address tokenToRedeem, uint256 newMinDiscount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) SetMinDiscount(opts *bind.TransactOpts, tokenToRedeem common.Address, newMinDiscount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "setMinDiscount", tokenToRedeem, newMinDiscount)
}

// SetMinDiscount is a paid mutator transaction binding the contract method 0x0517ef9a.
//
// Solidity: function setMinDiscount(address tokenToRedeem, uint256 newMinDiscount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) SetMinDiscount(tokenToRedeem common.Address, newMinDiscount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetMinDiscount(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, newMinDiscount)
}

// SetMinDiscount is a paid mutator transaction binding the contract method 0x0517ef9a.
//
// Solidity: function setMinDiscount(address tokenToRedeem, uint256 newMinDiscount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) SetMinDiscount(tokenToRedeem common.Address, newMinDiscount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetMinDiscount(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, newMinDiscount)
}

// SetPauser is a paid mutator transaction binding the contract method 0x2d88af4a.
//
// Solidity: function setPauser(address newPauser) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) SetPauser(opts *bind.TransactOpts, newPauser common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "setPauser", newPauser)
}

// SetPauser is a paid mutator transaction binding the contract method 0x2d88af4a.
//
// Solidity: function setPauser(address newPauser) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) SetPauser(newPauser common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetPauser(&_LiquidLaneAdapter.TransactOpts, newPauser)
}

// SetPauser is a paid mutator transaction binding the contract method 0x2d88af4a.
//
// Solidity: function setPauser(address newPauser) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) SetPauser(newPauser common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetPauser(&_LiquidLaneAdapter.TransactOpts, newPauser)
}

// SetReceiver is a paid mutator transaction binding the contract method 0x718da7ee.
//
// Solidity: function setReceiver(address newReceiver) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) SetReceiver(opts *bind.TransactOpts, newReceiver common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "setReceiver", newReceiver)
}

// SetReceiver is a paid mutator transaction binding the contract method 0x718da7ee.
//
// Solidity: function setReceiver(address newReceiver) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) SetReceiver(newReceiver common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetReceiver(&_LiquidLaneAdapter.TransactOpts, newReceiver)
}

// SetReceiver is a paid mutator transaction binding the contract method 0x718da7ee.
//
// Solidity: function setReceiver(address newReceiver) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) SetReceiver(newReceiver common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetReceiver(&_LiquidLaneAdapter.TransactOpts, newReceiver)
}

// SetUnpauser is a paid mutator transaction binding the contract method 0xce548428.
//
// Solidity: function setUnpauser(address newUnpauser) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) SetUnpauser(opts *bind.TransactOpts, newUnpauser common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "setUnpauser", newUnpauser)
}

// SetUnpauser is a paid mutator transaction binding the contract method 0xce548428.
//
// Solidity: function setUnpauser(address newUnpauser) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) SetUnpauser(newUnpauser common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetUnpauser(&_LiquidLaneAdapter.TransactOpts, newUnpauser)
}

// SetUnpauser is a paid mutator transaction binding the contract method 0xce548428.
//
// Solidity: function setUnpauser(address newUnpauser) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) SetUnpauser(newUnpauser common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.SetUnpauser(&_LiquidLaneAdapter.TransactOpts, newUnpauser)
}

// Swap is a paid mutator transaction binding the contract method 0x53f549d8.
//
// Solidity: function swap((address,address,uint256,uint256) swap) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Swap(opts *bind.TransactOpts, swap ILiquidLaneAdapterSwap) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "swap", swap)
}

// Swap is a paid mutator transaction binding the contract method 0x53f549d8.
//
// Solidity: function swap((address,address,uint256,uint256) swap) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Swap(swap ILiquidLaneAdapterSwap) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Swap(&_LiquidLaneAdapter.TransactOpts, swap)
}

// Swap is a paid mutator transaction binding the contract method 0x53f549d8.
//
// Solidity: function swap((address,address,uint256,uint256) swap) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Swap(swap ILiquidLaneAdapterSwap) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Swap(&_LiquidLaneAdapter.TransactOpts, swap)
}

// Swap0 is a paid mutator transaction binding the contract method 0x8fa5c671.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Swap0(opts *bind.TransactOpts, discountSwap ILiquidLaneAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "swap0", discountSwap, protocolSignature, recipient, amountIn)
}

// Swap0 is a paid mutator transaction binding the contract method 0x8fa5c671.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Swap0(discountSwap ILiquidLaneAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Swap0(&_LiquidLaneAdapter.TransactOpts, discountSwap, protocolSignature, recipient, amountIn)
}

// Swap0 is a paid mutator transaction binding the contract method 0x8fa5c671.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Swap0(discountSwap ILiquidLaneAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Swap0(&_LiquidLaneAdapter.TransactOpts, discountSwap, protocolSignature, recipient, amountIn)
}

// Swap1 is a paid mutator transaction binding the contract method 0x9a4568b6.
//
// Solidity: function swap((address,address,uint256,uint256,address,address,uint256,uint48) signedSwap, bytes signature) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Swap1(opts *bind.TransactOpts, signedSwap ILiquidLaneAdapterSignedSwap, signature []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "swap1", signedSwap, signature)
}

// Swap1 is a paid mutator transaction binding the contract method 0x9a4568b6.
//
// Solidity: function swap((address,address,uint256,uint256,address,address,uint256,uint48) signedSwap, bytes signature) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Swap1(signedSwap ILiquidLaneAdapterSignedSwap, signature []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Swap1(&_LiquidLaneAdapter.TransactOpts, signedSwap, signature)
}

// Swap1 is a paid mutator transaction binding the contract method 0x9a4568b6.
//
// Solidity: function swap((address,address,uint256,uint256,address,address,uint256,uint48) signedSwap, bytes signature) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Swap1(signedSwap ILiquidLaneAdapterSignedSwap, signature []byte) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Swap1(&_LiquidLaneAdapter.TransactOpts, signedSwap, signature)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.TransferOwnership(&_LiquidLaneAdapter.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.TransferOwnership(&_LiquidLaneAdapter.TransactOpts, newOwner)
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) Unpause(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "unpause")
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) Unpause() (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Unpause(&_LiquidLaneAdapter.TransactOpts)
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) Unpause() (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.Unpause(&_LiquidLaneAdapter.TransactOpts)
}

// WithdrawToAcquire is a paid mutator transaction binding the contract method 0x00812e34.
//
// Solidity: function withdrawToAcquire(address tokenToRedeem, uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactor) WithdrawToAcquire(opts *bind.TransactOpts, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.contract.Transact(opts, "withdrawToAcquire", tokenToRedeem, amount)
}

// WithdrawToAcquire is a paid mutator transaction binding the contract method 0x00812e34.
//
// Solidity: function withdrawToAcquire(address tokenToRedeem, uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterSession) WithdrawToAcquire(tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.WithdrawToAcquire(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, amount)
}

// WithdrawToAcquire is a paid mutator transaction binding the contract method 0x00812e34.
//
// Solidity: function withdrawToAcquire(address tokenToRedeem, uint256 amount) returns()
func (_LiquidLaneAdapter *LiquidLaneAdapterTransactorSession) WithdrawToAcquire(tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _LiquidLaneAdapter.Contract.WithdrawToAcquire(&_LiquidLaneAdapter.TransactOpts, tokenToRedeem, amount)
}

// LiquidLaneAdapterAddTokenToRedeemIterator is returned from FilterAddTokenToRedeem and is used to iterate over the raw logs and unpacked data for AddTokenToRedeem events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterAddTokenToRedeemIterator struct {
	Event *LiquidLaneAdapterAddTokenToRedeem // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterAddTokenToRedeemIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterAddTokenToRedeem)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterAddTokenToRedeem)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterAddTokenToRedeemIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterAddTokenToRedeemIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterAddTokenToRedeem represents a AddTokenToRedeem event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterAddTokenToRedeem struct {
	TokenToRedeem common.Address
	Account       common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterAddTokenToRedeem is a free log retrieval operation binding the contract event 0x5f32259d5292485fd235559f0cff88cb18ef3d09d44aea931403c5ded5fc9f2b.
//
// Solidity: event AddTokenToRedeem(address indexed tokenToRedeem, address indexed account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterAddTokenToRedeem(opts *bind.FilterOpts, tokenToRedeem []common.Address, account []common.Address) (*LiquidLaneAdapterAddTokenToRedeemIterator, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "AddTokenToRedeem", tokenToRedeemRule, accountRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterAddTokenToRedeemIterator{contract: _LiquidLaneAdapter.contract, event: "AddTokenToRedeem", logs: logs, sub: sub}, nil
}

// WatchAddTokenToRedeem is a free log subscription operation binding the contract event 0x5f32259d5292485fd235559f0cff88cb18ef3d09d44aea931403c5ded5fc9f2b.
//
// Solidity: event AddTokenToRedeem(address indexed tokenToRedeem, address indexed account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchAddTokenToRedeem(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterAddTokenToRedeem, tokenToRedeem []common.Address, account []common.Address) (event.Subscription, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "AddTokenToRedeem", tokenToRedeemRule, accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterAddTokenToRedeem)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "AddTokenToRedeem", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseAddTokenToRedeem is a log parse operation binding the contract event 0x5f32259d5292485fd235559f0cff88cb18ef3d09d44aea931403c5ded5fc9f2b.
//
// Solidity: event AddTokenToRedeem(address indexed tokenToRedeem, address indexed account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseAddTokenToRedeem(log types.Log) (*LiquidLaneAdapterAddTokenToRedeem, error) {
	event := new(LiquidLaneAdapterAddTokenToRedeem)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "AddTokenToRedeem", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterDepositToAcquireIterator is returned from FilterDepositToAcquire and is used to iterate over the raw logs and unpacked data for DepositToAcquire events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterDepositToAcquireIterator struct {
	Event *LiquidLaneAdapterDepositToAcquire // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterDepositToAcquireIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterDepositToAcquire)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterDepositToAcquire)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterDepositToAcquireIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterDepositToAcquireIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterDepositToAcquire represents a DepositToAcquire event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterDepositToAcquire struct {
	Who           common.Address
	TokenToRedeem common.Address
	Amount        *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterDepositToAcquire is a free log retrieval operation binding the contract event 0x43b3136ab574340cd0ab639ab932b5dfbfd94da466986b1025f84bd359d2f445.
//
// Solidity: event DepositToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterDepositToAcquire(opts *bind.FilterOpts, who []common.Address, tokenToRedeem []common.Address) (*LiquidLaneAdapterDepositToAcquireIterator, error) {

	var whoRule []interface{}
	for _, whoItem := range who {
		whoRule = append(whoRule, whoItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "DepositToAcquire", whoRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterDepositToAcquireIterator{contract: _LiquidLaneAdapter.contract, event: "DepositToAcquire", logs: logs, sub: sub}, nil
}

// WatchDepositToAcquire is a free log subscription operation binding the contract event 0x43b3136ab574340cd0ab639ab932b5dfbfd94da466986b1025f84bd359d2f445.
//
// Solidity: event DepositToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchDepositToAcquire(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterDepositToAcquire, who []common.Address, tokenToRedeem []common.Address) (event.Subscription, error) {

	var whoRule []interface{}
	for _, whoItem := range who {
		whoRule = append(whoRule, whoItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "DepositToAcquire", whoRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterDepositToAcquire)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "DepositToAcquire", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseDepositToAcquire is a log parse operation binding the contract event 0x43b3136ab574340cd0ab639ab932b5dfbfd94da466986b1025f84bd359d2f445.
//
// Solidity: event DepositToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseDepositToAcquire(log types.Log) (*LiquidLaneAdapterDepositToAcquire, error) {
	event := new(LiquidLaneAdapterDepositToAcquire)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "DepositToAcquire", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterDoSwapIterator is returned from FilterDoSwap and is used to iterate over the raw logs and unpacked data for DoSwap events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterDoSwapIterator struct {
	Event *LiquidLaneAdapterDoSwap // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterDoSwapIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterDoSwap)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterDoSwap)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterDoSwapIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterDoSwapIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterDoSwap represents a DoSwap event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterDoSwap struct {
	Swap ILiquidLaneAdapterSwap
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterDoSwap is a free log retrieval operation binding the contract event 0x4152adf3390c14101d81fa2579ca39f6cddb604f4245a525490a777afafcaa1e.
//
// Solidity: event DoSwap((address,address,uint256,uint256) swap)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterDoSwap(opts *bind.FilterOpts) (*LiquidLaneAdapterDoSwapIterator, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "DoSwap")
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterDoSwapIterator{contract: _LiquidLaneAdapter.contract, event: "DoSwap", logs: logs, sub: sub}, nil
}

// WatchDoSwap is a free log subscription operation binding the contract event 0x4152adf3390c14101d81fa2579ca39f6cddb604f4245a525490a777afafcaa1e.
//
// Solidity: event DoSwap((address,address,uint256,uint256) swap)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchDoSwap(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterDoSwap) (event.Subscription, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "DoSwap")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterDoSwap)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "DoSwap", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseDoSwap is a log parse operation binding the contract event 0x4152adf3390c14101d81fa2579ca39f6cddb604f4245a525490a777afafcaa1e.
//
// Solidity: event DoSwap((address,address,uint256,uint256) swap)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseDoSwap(log types.Log) (*LiquidLaneAdapterDoSwap, error) {
	event := new(LiquidLaneAdapterDoSwap)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "DoSwap", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterEIP712DomainChangedIterator is returned from FilterEIP712DomainChanged and is used to iterate over the raw logs and unpacked data for EIP712DomainChanged events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterEIP712DomainChangedIterator struct {
	Event *LiquidLaneAdapterEIP712DomainChanged // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterEIP712DomainChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterEIP712DomainChanged)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterEIP712DomainChanged)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterEIP712DomainChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterEIP712DomainChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterEIP712DomainChanged represents a EIP712DomainChanged event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterEIP712DomainChanged struct {
	Raw types.Log // Blockchain specific contextual infos
}

// FilterEIP712DomainChanged is a free log retrieval operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterEIP712DomainChanged(opts *bind.FilterOpts) (*LiquidLaneAdapterEIP712DomainChangedIterator, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterEIP712DomainChangedIterator{contract: _LiquidLaneAdapter.contract, event: "EIP712DomainChanged", logs: logs, sub: sub}, nil
}

// WatchEIP712DomainChanged is a free log subscription operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchEIP712DomainChanged(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterEIP712DomainChanged) (event.Subscription, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterEIP712DomainChanged)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseEIP712DomainChanged is a log parse operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseEIP712DomainChanged(log types.Log) (*LiquidLaneAdapterEIP712DomainChanged, error) {
	event := new(LiquidLaneAdapterEIP712DomainChanged)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterInitializeIterator is returned from FilterInitialize and is used to iterate over the raw logs and unpacked data for Initialize events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInitializeIterator struct {
	Event *LiquidLaneAdapterInitialize // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterInitializeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterInitialize)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterInitialize)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterInitializeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterInitializeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterInitialize represents a Initialize event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInitialize struct {
	Params ILiquidLaneAdapterInitParams
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterInitialize is a free log retrieval operation binding the contract event 0xb99039aac150084151e681a836d6d9631c0d78625c6c09a33f2157c74f4af539.
//
// Solidity: event Initialize((address,address) params)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterInitialize(opts *bind.FilterOpts) (*LiquidLaneAdapterInitializeIterator, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterInitializeIterator{contract: _LiquidLaneAdapter.contract, event: "Initialize", logs: logs, sub: sub}, nil
}

// WatchInitialize is a free log subscription operation binding the contract event 0xb99039aac150084151e681a836d6d9631c0d78625c6c09a33f2157c74f4af539.
//
// Solidity: event Initialize((address,address) params)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchInitialize(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterInitialize) (event.Subscription, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterInitialize)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Initialize", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInitialize is a log parse operation binding the contract event 0xb99039aac150084151e681a836d6d9631c0d78625c6c09a33f2157c74f4af539.
//
// Solidity: event Initialize((address,address) params)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseInitialize(log types.Log) (*LiquidLaneAdapterInitialize, error) {
	event := new(LiquidLaneAdapterInitialize)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Initialize", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInitializedIterator struct {
	Event *LiquidLaneAdapterInitialized // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterInitialized)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterInitialized)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterInitialized represents a Initialized event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInitialized struct {
	Version uint64
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterInitialized(opts *bind.FilterOpts) (*LiquidLaneAdapterInitializedIterator, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterInitializedIterator{contract: _LiquidLaneAdapter.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterInitialized) (event.Subscription, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterInitialized)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Initialized", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInitialized is a log parse operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseInitialized(log types.Log) (*LiquidLaneAdapterInitialized, error) {
	event := new(LiquidLaneAdapterInitialized)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterInvalidateNonceIterator is returned from FilterInvalidateNonce and is used to iterate over the raw logs and unpacked data for InvalidateNonce events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidateNonceIterator struct {
	Event *LiquidLaneAdapterInvalidateNonce // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterInvalidateNonceIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterInvalidateNonce)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterInvalidateNonce)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterInvalidateNonceIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterInvalidateNonceIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterInvalidateNonce represents a InvalidateNonce event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidateNonce struct {
	TokenToRedeem common.Address
	Nonce         *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterInvalidateNonce is a free log retrieval operation binding the contract event 0x294baeb3162c5caef603a11b80be3b7422473c4380865fecc65e3422f1f8b4d6.
//
// Solidity: event InvalidateNonce(address indexed tokenToRedeem, uint256 indexed nonce)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterInvalidateNonce(opts *bind.FilterOpts, tokenToRedeem []common.Address, nonce []*big.Int) (*LiquidLaneAdapterInvalidateNonceIterator, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "InvalidateNonce", tokenToRedeemRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterInvalidateNonceIterator{contract: _LiquidLaneAdapter.contract, event: "InvalidateNonce", logs: logs, sub: sub}, nil
}

// WatchInvalidateNonce is a free log subscription operation binding the contract event 0x294baeb3162c5caef603a11b80be3b7422473c4380865fecc65e3422f1f8b4d6.
//
// Solidity: event InvalidateNonce(address indexed tokenToRedeem, uint256 indexed nonce)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchInvalidateNonce(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterInvalidateNonce, tokenToRedeem []common.Address, nonce []*big.Int) (event.Subscription, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "InvalidateNonce", tokenToRedeemRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterInvalidateNonce)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "InvalidateNonce", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInvalidateNonce is a log parse operation binding the contract event 0x294baeb3162c5caef603a11b80be3b7422473c4380865fecc65e3422f1f8b4d6.
//
// Solidity: event InvalidateNonce(address indexed tokenToRedeem, uint256 indexed nonce)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseInvalidateNonce(log types.Log) (*LiquidLaneAdapterInvalidateNonce, error) {
	event := new(LiquidLaneAdapterInvalidateNonce)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "InvalidateNonce", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterOwnershipTransferredIterator struct {
	Event *LiquidLaneAdapterOwnershipTransferred // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterOwnershipTransferred)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterOwnershipTransferred)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterOwnershipTransferred represents a OwnershipTransferred event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*LiquidLaneAdapterOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterOwnershipTransferredIterator{contract: _LiquidLaneAdapter.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterOwnershipTransferred)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseOwnershipTransferred(log types.Log) (*LiquidLaneAdapterOwnershipTransferred, error) {
	event := new(LiquidLaneAdapterOwnershipTransferred)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterPausedIterator is returned from FilterPaused and is used to iterate over the raw logs and unpacked data for Paused events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterPausedIterator struct {
	Event *LiquidLaneAdapterPaused // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterPausedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterPaused)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterPaused)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterPausedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterPausedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterPaused represents a Paused event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterPaused struct {
	Account common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterPaused is a free log retrieval operation binding the contract event 0x62e78cea01bee320cd4e420270b5ea74000d11b0c9f74754ebdbfc544b05a258.
//
// Solidity: event Paused(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterPaused(opts *bind.FilterOpts) (*LiquidLaneAdapterPausedIterator, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "Paused")
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterPausedIterator{contract: _LiquidLaneAdapter.contract, event: "Paused", logs: logs, sub: sub}, nil
}

// WatchPaused is a free log subscription operation binding the contract event 0x62e78cea01bee320cd4e420270b5ea74000d11b0c9f74754ebdbfc544b05a258.
//
// Solidity: event Paused(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchPaused(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterPaused) (event.Subscription, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "Paused")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterPaused)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Paused", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParsePaused is a log parse operation binding the contract event 0x62e78cea01bee320cd4e420270b5ea74000d11b0c9f74754ebdbfc544b05a258.
//
// Solidity: event Paused(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParsePaused(log types.Log) (*LiquidLaneAdapterPaused, error) {
	event := new(LiquidLaneAdapterPaused)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Paused", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterRemoveTokenToRedeemIterator is returned from FilterRemoveTokenToRedeem and is used to iterate over the raw logs and unpacked data for RemoveTokenToRedeem events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterRemoveTokenToRedeemIterator struct {
	Event *LiquidLaneAdapterRemoveTokenToRedeem // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterRemoveTokenToRedeemIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterRemoveTokenToRedeem)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterRemoveTokenToRedeem)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterRemoveTokenToRedeemIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterRemoveTokenToRedeemIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterRemoveTokenToRedeem represents a RemoveTokenToRedeem event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterRemoveTokenToRedeem struct {
	TokenToRedeem common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterRemoveTokenToRedeem is a free log retrieval operation binding the contract event 0x8998e0e27970c7d27f4ba379de5da28b62def3add77fd6329c021fedde2263c1.
//
// Solidity: event RemoveTokenToRedeem(address indexed tokenToRedeem)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterRemoveTokenToRedeem(opts *bind.FilterOpts, tokenToRedeem []common.Address) (*LiquidLaneAdapterRemoveTokenToRedeemIterator, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "RemoveTokenToRedeem", tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterRemoveTokenToRedeemIterator{contract: _LiquidLaneAdapter.contract, event: "RemoveTokenToRedeem", logs: logs, sub: sub}, nil
}

// WatchRemoveTokenToRedeem is a free log subscription operation binding the contract event 0x8998e0e27970c7d27f4ba379de5da28b62def3add77fd6329c021fedde2263c1.
//
// Solidity: event RemoveTokenToRedeem(address indexed tokenToRedeem)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchRemoveTokenToRedeem(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterRemoveTokenToRedeem, tokenToRedeem []common.Address) (event.Subscription, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "RemoveTokenToRedeem", tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterRemoveTokenToRedeem)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "RemoveTokenToRedeem", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRemoveTokenToRedeem is a log parse operation binding the contract event 0x8998e0e27970c7d27f4ba379de5da28b62def3add77fd6329c021fedde2263c1.
//
// Solidity: event RemoveTokenToRedeem(address indexed tokenToRedeem)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseRemoveTokenToRedeem(log types.Log) (*LiquidLaneAdapterRemoveTokenToRedeem, error) {
	event := new(LiquidLaneAdapterRemoveTokenToRedeem)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "RemoveTokenToRedeem", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetFillerIterator is returned from FilterSetFiller and is used to iterate over the raw logs and unpacked data for SetFiller events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetFillerIterator struct {
	Event *LiquidLaneAdapterSetFiller // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetFillerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetFiller)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetFiller)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetFillerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetFillerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetFiller represents a SetFiller event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetFiller struct {
	MarketMaker  common.Address
	Filler       common.Address
	IsAuthorized bool
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterSetFiller is a free log retrieval operation binding the contract event 0x3d800b49f684e2981f26aa387e017232b4c9f220a9058b90639bb30b9fafeb84.
//
// Solidity: event SetFiller(address indexed marketMaker, address indexed filler, bool isAuthorized)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetFiller(opts *bind.FilterOpts, marketMaker []common.Address, filler []common.Address) (*LiquidLaneAdapterSetFillerIterator, error) {

	var marketMakerRule []interface{}
	for _, marketMakerItem := range marketMaker {
		marketMakerRule = append(marketMakerRule, marketMakerItem)
	}
	var fillerRule []interface{}
	for _, fillerItem := range filler {
		fillerRule = append(fillerRule, fillerItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetFiller", marketMakerRule, fillerRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetFillerIterator{contract: _LiquidLaneAdapter.contract, event: "SetFiller", logs: logs, sub: sub}, nil
}

// WatchSetFiller is a free log subscription operation binding the contract event 0x3d800b49f684e2981f26aa387e017232b4c9f220a9058b90639bb30b9fafeb84.
//
// Solidity: event SetFiller(address indexed marketMaker, address indexed filler, bool isAuthorized)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetFiller(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetFiller, marketMaker []common.Address, filler []common.Address) (event.Subscription, error) {

	var marketMakerRule []interface{}
	for _, marketMakerItem := range marketMaker {
		marketMakerRule = append(marketMakerRule, marketMakerItem)
	}
	var fillerRule []interface{}
	for _, fillerItem := range filler {
		fillerRule = append(fillerRule, fillerItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetFiller", marketMakerRule, fillerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetFiller)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetFiller", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetFiller is a log parse operation binding the contract event 0x3d800b49f684e2981f26aa387e017232b4c9f220a9058b90639bb30b9fafeb84.
//
// Solidity: event SetFiller(address indexed marketMaker, address indexed filler, bool isAuthorized)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetFiller(log types.Log) (*LiquidLaneAdapterSetFiller, error) {
	event := new(LiquidLaneAdapterSetFiller)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetFiller", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetLimitIterator is returned from FilterSetLimit and is used to iterate over the raw logs and unpacked data for SetLimit events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetLimitIterator struct {
	Event *LiquidLaneAdapterSetLimit // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetLimit)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetLimit)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetLimit represents a SetLimit event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetLimit struct {
	TokenToRedeem common.Address
	NewLimit      *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterSetLimit is a free log retrieval operation binding the contract event 0x195f01c216465063f9e6a5cc900b834b284e36961065b3d5f63b115ecd88582a.
//
// Solidity: event SetLimit(address indexed tokenToRedeem, uint256 newLimit)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetLimit(opts *bind.FilterOpts, tokenToRedeem []common.Address) (*LiquidLaneAdapterSetLimitIterator, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetLimit", tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetLimitIterator{contract: _LiquidLaneAdapter.contract, event: "SetLimit", logs: logs, sub: sub}, nil
}

// WatchSetLimit is a free log subscription operation binding the contract event 0x195f01c216465063f9e6a5cc900b834b284e36961065b3d5f63b115ecd88582a.
//
// Solidity: event SetLimit(address indexed tokenToRedeem, uint256 newLimit)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetLimit(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetLimit, tokenToRedeem []common.Address) (event.Subscription, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetLimit", tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetLimit)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetLimit", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetLimit is a log parse operation binding the contract event 0x195f01c216465063f9e6a5cc900b834b284e36961065b3d5f63b115ecd88582a.
//
// Solidity: event SetLimit(address indexed tokenToRedeem, uint256 newLimit)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetLimit(log types.Log) (*LiquidLaneAdapterSetLimit, error) {
	event := new(LiquidLaneAdapterSetLimit)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetMarketMakerIterator is returned from FilterSetMarketMaker and is used to iterate over the raw logs and unpacked data for SetMarketMaker events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetMarketMakerIterator struct {
	Event *LiquidLaneAdapterSetMarketMaker // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetMarketMakerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetMarketMaker)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetMarketMaker)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetMarketMakerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetMarketMakerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetMarketMaker represents a SetMarketMaker event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetMarketMaker struct {
	NewMarketMaker common.Address
	NewCanAcquire  bool
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterSetMarketMaker is a free log retrieval operation binding the contract event 0x91c7c4816685f676b7f12953bffd031e09ee4e1ab280ea728f0878b9e6169d27.
//
// Solidity: event SetMarketMaker(address indexed newMarketMaker, bool newCanAcquire)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetMarketMaker(opts *bind.FilterOpts, newMarketMaker []common.Address) (*LiquidLaneAdapterSetMarketMakerIterator, error) {

	var newMarketMakerRule []interface{}
	for _, newMarketMakerItem := range newMarketMaker {
		newMarketMakerRule = append(newMarketMakerRule, newMarketMakerItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetMarketMaker", newMarketMakerRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetMarketMakerIterator{contract: _LiquidLaneAdapter.contract, event: "SetMarketMaker", logs: logs, sub: sub}, nil
}

// WatchSetMarketMaker is a free log subscription operation binding the contract event 0x91c7c4816685f676b7f12953bffd031e09ee4e1ab280ea728f0878b9e6169d27.
//
// Solidity: event SetMarketMaker(address indexed newMarketMaker, bool newCanAcquire)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetMarketMaker(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetMarketMaker, newMarketMaker []common.Address) (event.Subscription, error) {

	var newMarketMakerRule []interface{}
	for _, newMarketMakerItem := range newMarketMaker {
		newMarketMakerRule = append(newMarketMakerRule, newMarketMakerItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetMarketMaker", newMarketMakerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetMarketMaker)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetMarketMaker", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetMarketMaker is a log parse operation binding the contract event 0x91c7c4816685f676b7f12953bffd031e09ee4e1ab280ea728f0878b9e6169d27.
//
// Solidity: event SetMarketMaker(address indexed newMarketMaker, bool newCanAcquire)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetMarketMaker(log types.Log) (*LiquidLaneAdapterSetMarketMaker, error) {
	event := new(LiquidLaneAdapterSetMarketMaker)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetMarketMaker", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetMinDiscountIterator is returned from FilterSetMinDiscount and is used to iterate over the raw logs and unpacked data for SetMinDiscount events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetMinDiscountIterator struct {
	Event *LiquidLaneAdapterSetMinDiscount // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetMinDiscountIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetMinDiscount)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetMinDiscount)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetMinDiscountIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetMinDiscountIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetMinDiscount represents a SetMinDiscount event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetMinDiscount struct {
	TokenToRedeem  common.Address
	NewMinDiscount *big.Int
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterSetMinDiscount is a free log retrieval operation binding the contract event 0xd842ee806832c3e5f5e97d22f096a87f9042e992a2d37a2b14077a81804d5ff7.
//
// Solidity: event SetMinDiscount(address indexed tokenToRedeem, uint256 newMinDiscount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetMinDiscount(opts *bind.FilterOpts, tokenToRedeem []common.Address) (*LiquidLaneAdapterSetMinDiscountIterator, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetMinDiscount", tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetMinDiscountIterator{contract: _LiquidLaneAdapter.contract, event: "SetMinDiscount", logs: logs, sub: sub}, nil
}

// WatchSetMinDiscount is a free log subscription operation binding the contract event 0xd842ee806832c3e5f5e97d22f096a87f9042e992a2d37a2b14077a81804d5ff7.
//
// Solidity: event SetMinDiscount(address indexed tokenToRedeem, uint256 newMinDiscount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetMinDiscount(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetMinDiscount, tokenToRedeem []common.Address) (event.Subscription, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetMinDiscount", tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetMinDiscount)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetMinDiscount", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetMinDiscount is a log parse operation binding the contract event 0xd842ee806832c3e5f5e97d22f096a87f9042e992a2d37a2b14077a81804d5ff7.
//
// Solidity: event SetMinDiscount(address indexed tokenToRedeem, uint256 newMinDiscount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetMinDiscount(log types.Log) (*LiquidLaneAdapterSetMinDiscount, error) {
	event := new(LiquidLaneAdapterSetMinDiscount)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetMinDiscount", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetPauserIterator is returned from FilterSetPauser and is used to iterate over the raw logs and unpacked data for SetPauser events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetPauserIterator struct {
	Event *LiquidLaneAdapterSetPauser // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetPauserIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetPauser)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetPauser)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetPauserIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetPauserIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetPauser represents a SetPauser event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetPauser struct {
	NewPauser common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterSetPauser is a free log retrieval operation binding the contract event 0xe02efb9e8f0fc21546730ab32d594f62d586e1bbb15bb5045edd0b1878a77b35.
//
// Solidity: event SetPauser(address indexed newPauser)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetPauser(opts *bind.FilterOpts, newPauser []common.Address) (*LiquidLaneAdapterSetPauserIterator, error) {

	var newPauserRule []interface{}
	for _, newPauserItem := range newPauser {
		newPauserRule = append(newPauserRule, newPauserItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetPauser", newPauserRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetPauserIterator{contract: _LiquidLaneAdapter.contract, event: "SetPauser", logs: logs, sub: sub}, nil
}

// WatchSetPauser is a free log subscription operation binding the contract event 0xe02efb9e8f0fc21546730ab32d594f62d586e1bbb15bb5045edd0b1878a77b35.
//
// Solidity: event SetPauser(address indexed newPauser)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetPauser(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetPauser, newPauser []common.Address) (event.Subscription, error) {

	var newPauserRule []interface{}
	for _, newPauserItem := range newPauser {
		newPauserRule = append(newPauserRule, newPauserItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetPauser", newPauserRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetPauser)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetPauser", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetPauser is a log parse operation binding the contract event 0xe02efb9e8f0fc21546730ab32d594f62d586e1bbb15bb5045edd0b1878a77b35.
//
// Solidity: event SetPauser(address indexed newPauser)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetPauser(log types.Log) (*LiquidLaneAdapterSetPauser, error) {
	event := new(LiquidLaneAdapterSetPauser)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetPauser", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetReceiverIterator is returned from FilterSetReceiver and is used to iterate over the raw logs and unpacked data for SetReceiver events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetReceiverIterator struct {
	Event *LiquidLaneAdapterSetReceiver // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetReceiverIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetReceiver)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetReceiver)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetReceiverIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetReceiverIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetReceiver represents a SetReceiver event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetReceiver struct {
	Who      common.Address
	Receiver common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterSetReceiver is a free log retrieval operation binding the contract event 0xff26e1bf0ab0473866e896a80844fef5835a5aac787e3094dc8737788c450d3e.
//
// Solidity: event SetReceiver(address indexed who, address indexed receiver)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetReceiver(opts *bind.FilterOpts, who []common.Address, receiver []common.Address) (*LiquidLaneAdapterSetReceiverIterator, error) {

	var whoRule []interface{}
	for _, whoItem := range who {
		whoRule = append(whoRule, whoItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetReceiver", whoRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetReceiverIterator{contract: _LiquidLaneAdapter.contract, event: "SetReceiver", logs: logs, sub: sub}, nil
}

// WatchSetReceiver is a free log subscription operation binding the contract event 0xff26e1bf0ab0473866e896a80844fef5835a5aac787e3094dc8737788c450d3e.
//
// Solidity: event SetReceiver(address indexed who, address indexed receiver)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetReceiver(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetReceiver, who []common.Address, receiver []common.Address) (event.Subscription, error) {

	var whoRule []interface{}
	for _, whoItem := range who {
		whoRule = append(whoRule, whoItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetReceiver", whoRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetReceiver)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetReceiver", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetReceiver is a log parse operation binding the contract event 0xff26e1bf0ab0473866e896a80844fef5835a5aac787e3094dc8737788c450d3e.
//
// Solidity: event SetReceiver(address indexed who, address indexed receiver)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetReceiver(log types.Log) (*LiquidLaneAdapterSetReceiver, error) {
	event := new(LiquidLaneAdapterSetReceiver)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetReceiver", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetUnpauserIterator is returned from FilterSetUnpauser and is used to iterate over the raw logs and unpacked data for SetUnpauser events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetUnpauserIterator struct {
	Event *LiquidLaneAdapterSetUnpauser // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetUnpauserIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetUnpauser)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetUnpauser)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetUnpauserIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetUnpauserIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetUnpauser represents a SetUnpauser event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetUnpauser struct {
	NewUnpauser common.Address
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterSetUnpauser is a free log retrieval operation binding the contract event 0x96440fd41a54d00eb948fd0859c1032db61858c87096ac9b38620453b5078557.
//
// Solidity: event SetUnpauser(address indexed newUnpauser)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetUnpauser(opts *bind.FilterOpts, newUnpauser []common.Address) (*LiquidLaneAdapterSetUnpauserIterator, error) {

	var newUnpauserRule []interface{}
	for _, newUnpauserItem := range newUnpauser {
		newUnpauserRule = append(newUnpauserRule, newUnpauserItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetUnpauser", newUnpauserRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetUnpauserIterator{contract: _LiquidLaneAdapter.contract, event: "SetUnpauser", logs: logs, sub: sub}, nil
}

// WatchSetUnpauser is a free log subscription operation binding the contract event 0x96440fd41a54d00eb948fd0859c1032db61858c87096ac9b38620453b5078557.
//
// Solidity: event SetUnpauser(address indexed newUnpauser)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetUnpauser(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetUnpauser, newUnpauser []common.Address) (event.Subscription, error) {

	var newUnpauserRule []interface{}
	for _, newUnpauserItem := range newUnpauser {
		newUnpauserRule = append(newUnpauserRule, newUnpauserItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetUnpauser", newUnpauserRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetUnpauser)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetUnpauser", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetUnpauser is a log parse operation binding the contract event 0x96440fd41a54d00eb948fd0859c1032db61858c87096ac9b38620453b5078557.
//
// Solidity: event SetUnpauser(address indexed newUnpauser)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetUnpauser(log types.Log) (*LiquidLaneAdapterSetUnpauser, error) {
	event := new(LiquidLaneAdapterSetUnpauser)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetUnpauser", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterSetVaultIterator is returned from FilterSetVault and is used to iterate over the raw logs and unpacked data for SetVault events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetVaultIterator struct {
	Event *LiquidLaneAdapterSetVault // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterSetVaultIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterSetVault)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterSetVault)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterSetVaultIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterSetVaultIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterSetVault represents a SetVault event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetVault struct {
	Vault common.Address
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterSetVault is a free log retrieval operation binding the contract event 0xd459c7242e23d490831b5676a611c4342d899d28f342d89ae80793e56a930f30.
//
// Solidity: event SetVault(address indexed vault)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterSetVault(opts *bind.FilterOpts, vault []common.Address) (*LiquidLaneAdapterSetVaultIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "SetVault", vaultRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterSetVaultIterator{contract: _LiquidLaneAdapter.contract, event: "SetVault", logs: logs, sub: sub}, nil
}

// WatchSetVault is a free log subscription operation binding the contract event 0xd459c7242e23d490831b5676a611c4342d899d28f342d89ae80793e56a930f30.
//
// Solidity: event SetVault(address indexed vault)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchSetVault(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterSetVault, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "SetVault", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterSetVault)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetVault", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetVault is a log parse operation binding the contract event 0xd459c7242e23d490831b5676a611c4342d899d28f342d89ae80793e56a930f30.
//
// Solidity: event SetVault(address indexed vault)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseSetVault(log types.Log) (*LiquidLaneAdapterSetVault, error) {
	event := new(LiquidLaneAdapterSetVault)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "SetVault", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterUnpausedIterator is returned from FilterUnpaused and is used to iterate over the raw logs and unpacked data for Unpaused events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterUnpausedIterator struct {
	Event *LiquidLaneAdapterUnpaused // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterUnpausedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterUnpaused)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterUnpaused)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterUnpausedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterUnpausedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterUnpaused represents a Unpaused event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterUnpaused struct {
	Account common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterUnpaused is a free log retrieval operation binding the contract event 0x5db9ee0a495bf2e6ff9c91a7834c1ba4fdd244a5e8aa4e537bd38aeae4b073aa.
//
// Solidity: event Unpaused(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterUnpaused(opts *bind.FilterOpts) (*LiquidLaneAdapterUnpausedIterator, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "Unpaused")
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterUnpausedIterator{contract: _LiquidLaneAdapter.contract, event: "Unpaused", logs: logs, sub: sub}, nil
}

// WatchUnpaused is a free log subscription operation binding the contract event 0x5db9ee0a495bf2e6ff9c91a7834c1ba4fdd244a5e8aa4e537bd38aeae4b073aa.
//
// Solidity: event Unpaused(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchUnpaused(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterUnpaused) (event.Subscription, error) {

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "Unpaused")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterUnpaused)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Unpaused", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUnpaused is a log parse operation binding the contract event 0x5db9ee0a495bf2e6ff9c91a7834c1ba4fdd244a5e8aa4e537bd38aeae4b073aa.
//
// Solidity: event Unpaused(address account)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseUnpaused(log types.Log) (*LiquidLaneAdapterUnpaused, error) {
	event := new(LiquidLaneAdapterUnpaused)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "Unpaused", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LiquidLaneAdapterWithdrawToAcquireIterator is returned from FilterWithdrawToAcquire and is used to iterate over the raw logs and unpacked data for WithdrawToAcquire events raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterWithdrawToAcquireIterator struct {
	Event *LiquidLaneAdapterWithdrawToAcquire // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *LiquidLaneAdapterWithdrawToAcquireIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LiquidLaneAdapterWithdrawToAcquire)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(LiquidLaneAdapterWithdrawToAcquire)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *LiquidLaneAdapterWithdrawToAcquireIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LiquidLaneAdapterWithdrawToAcquireIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LiquidLaneAdapterWithdrawToAcquire represents a WithdrawToAcquire event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterWithdrawToAcquire struct {
	Who           common.Address
	TokenToRedeem common.Address
	Amount        *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterWithdrawToAcquire is a free log retrieval operation binding the contract event 0xc9f36ee08fa818d79e973b718349a2d483dc5c6058b82931c2611e9d7dd3b664.
//
// Solidity: event WithdrawToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) FilterWithdrawToAcquire(opts *bind.FilterOpts, who []common.Address, tokenToRedeem []common.Address) (*LiquidLaneAdapterWithdrawToAcquireIterator, error) {

	var whoRule []interface{}
	for _, whoItem := range who {
		whoRule = append(whoRule, whoItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.FilterLogs(opts, "WithdrawToAcquire", whoRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &LiquidLaneAdapterWithdrawToAcquireIterator{contract: _LiquidLaneAdapter.contract, event: "WithdrawToAcquire", logs: logs, sub: sub}, nil
}

// WatchWithdrawToAcquire is a free log subscription operation binding the contract event 0xc9f36ee08fa818d79e973b718349a2d483dc5c6058b82931c2611e9d7dd3b664.
//
// Solidity: event WithdrawToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) WatchWithdrawToAcquire(opts *bind.WatchOpts, sink chan<- *LiquidLaneAdapterWithdrawToAcquire, who []common.Address, tokenToRedeem []common.Address) (event.Subscription, error) {

	var whoRule []interface{}
	for _, whoItem := range who {
		whoRule = append(whoRule, whoItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _LiquidLaneAdapter.contract.WatchLogs(opts, "WithdrawToAcquire", whoRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LiquidLaneAdapterWithdrawToAcquire)
				if err := _LiquidLaneAdapter.contract.UnpackLog(event, "WithdrawToAcquire", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseWithdrawToAcquire is a log parse operation binding the contract event 0xc9f36ee08fa818d79e973b718349a2d483dc5c6058b82931c2611e9d7dd3b664.
//
// Solidity: event WithdrawToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (_LiquidLaneAdapter *LiquidLaneAdapterFilterer) ParseWithdrawToAcquire(log types.Log) (*LiquidLaneAdapterWithdrawToAcquire, error) {
	event := new(LiquidLaneAdapterWithdrawToAcquire)
	if err := _LiquidLaneAdapter.contract.UnpackLog(event, "WithdrawToAcquire", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
