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
var LiquidLaneAdapterMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"vaultFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"adapterFactory\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"accountRegistry\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"accounts\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"acquireBalance\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"marketMaker\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"addTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deallocate\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"deallocated\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"depositToAcquire\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"freeAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAmountOut\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getMaxRate\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTokensToRedeemLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"invalidateNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isFiller\",\"inputs\":[{\"name\":\"marketMaker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isUsedNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"limit\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"marketMaker\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"marketMakerCanAcquire\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ppm\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pause\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"paused\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pauser\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"receiver\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"removeTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"requestDeallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setFiller\",\"inputs\":[{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setLimit\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMarketMaker\",\"inputs\":[{\"name\":\"newMarketMaker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newCanAcquire\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMinDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMinDiscount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPauser\",\"inputs\":[{\"name\":\"newPauser\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setReceiver\",\"inputs\":[{\"name\":\"newReceiver\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setUnpauser\",\"inputs\":[{\"name\":\"newUnpauser\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"signedSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.SignedSwap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"tokensToRedeem\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalAssets\",\"inputs\":[],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"unpause\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"unpauser\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"vault\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawToAcquire\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AddTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DepositToAcquire\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DoSwap\",\"inputs\":[{\"name\":\"swap\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structILiquidLaneAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialize\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structILiquidLaneAdapter.InitParams\",\"components\":[{\"name\":\"pauser\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"unpauser\",\"type\":\"address\",\"internalType\":\"address\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"InvalidateNonce\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Paused\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RemoveTokenToRedeem\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetFiller\",\"inputs\":[{\"name\":\"marketMaker\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"isAuthorized\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetLimit\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newLimit\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMarketMaker\",\"inputs\":[{\"name\":\"newMarketMaker\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newCanAcquire\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMinDiscount\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newMinDiscount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetPauser\",\"inputs\":[{\"name\":\"newPauser\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetReceiver\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetUnpauser\",\"inputs\":[{\"name\":\"newUnpauser\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetVault\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Unpaused\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"WithdrawToAcquire\",\"inputs\":[{\"name\":\"who\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AccountHasAssets\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyUsedNonce\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DepositNotAllowed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"EnforcedPause\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExpectedPause\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExpiredSwap\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientAllocate\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAccount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDiscount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOracle\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidReceiver\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidShortString\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSwapRate\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTokenToRedeem\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"LimitExceeded\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"StringTooLong\",\"inputs\":[{\"name\":\"str\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"TooManyTokensToRedeem\",\"inputs\":[]}]",
	ID:  "LiquidLaneAdapter",
}

// LiquidLaneAdapter is an auto generated Go binding around an Ethereum contract.
type LiquidLaneAdapter struct {
	abi abi.ABI
}

// NewLiquidLaneAdapter creates a new instance of LiquidLaneAdapter.
func NewLiquidLaneAdapter() *LiquidLaneAdapter {
	parsed, err := LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &LiquidLaneAdapter{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *LiquidLaneAdapter) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address vaultFactory, address adapterFactory, address accountRegistry) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackConstructor(vaultFactory common.Address, adapterFactory common.Address, accountRegistry common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("", vaultFactory, adapterFactory, accountRegistry)
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
func (liquidLaneAdapter *LiquidLaneAdapter) PackFACTORY() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("FACTORY")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackFACTORY() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("FACTORY")
}

// UnpackFACTORY is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackFACTORY(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("FACTORY", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackAccounts is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5e5c06e2.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function accounts(address tokenToRedeem) view returns(address account)
func (liquidLaneAdapter *LiquidLaneAdapter) PackAccounts(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("accounts", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAccounts is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5e5c06e2.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function accounts(address tokenToRedeem) view returns(address account)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackAccounts(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("accounts", tokenToRedeem)
}

// UnpackAccounts is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5e5c06e2.
//
// Solidity: function accounts(address tokenToRedeem) view returns(address account)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAccounts(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("accounts", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackAcquireBalance is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5cf29c90.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function acquireBalance(address tokenToRedeem, address marketMaker) view returns(uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) PackAcquireBalance(tokenToRedeem common.Address, marketMaker common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("acquireBalance", tokenToRedeem, marketMaker)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAcquireBalance is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5cf29c90.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function acquireBalance(address tokenToRedeem, address marketMaker) view returns(uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackAcquireBalance(tokenToRedeem common.Address, marketMaker common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("acquireBalance", tokenToRedeem, marketMaker)
}

// UnpackAcquireBalance is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5cf29c90.
//
// Solidity: function acquireBalance(address tokenToRedeem, address marketMaker) view returns(uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAcquireBalance(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("acquireBalance", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackAddTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc9b3c58d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function addTokenToRedeem(address tokenToRedeem) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackAddTokenToRedeem(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("addTokenToRedeem", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAddTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc9b3c58d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function addTokenToRedeem(address tokenToRedeem) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackAddTokenToRedeem(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("addTokenToRedeem", tokenToRedeem)
}

// PackAllocatable is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1d3b809a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function allocatable() view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) PackAllocatable() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("allocatable")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackAllocatable() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("allocatable")
}

// UnpackAllocatable is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAllocatable(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("allocatable", data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) PackAllocate(amount *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("allocate", amount)
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackAllocate(amount *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("allocate", amount)
}

// UnpackAllocate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAllocate(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("allocate", data)
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
// Solidity: function deallocate(uint256 ) returns(uint256 deallocated)
func (liquidLaneAdapter *LiquidLaneAdapter) PackDeallocate(arg0 *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("deallocate", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDeallocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x6f6c441f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function deallocate(uint256 ) returns(uint256 deallocated)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackDeallocate(arg0 *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("deallocate", arg0)
}

// UnpackDeallocate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x6f6c441f.
//
// Solidity: function deallocate(uint256 ) returns(uint256 deallocated)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackDeallocate(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("deallocate", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackDepositToAcquire is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd6c83d24.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function depositToAcquire(address tokenToRedeem, uint256 amount) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackDepositToAcquire(tokenToRedeem common.Address, amount *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("depositToAcquire", tokenToRedeem, amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDepositToAcquire is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd6c83d24.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function depositToAcquire(address tokenToRedeem, uint256 amount) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackDepositToAcquire(tokenToRedeem common.Address, amount *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("depositToAcquire", tokenToRedeem, amount)
}

// PackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (liquidLaneAdapter *LiquidLaneAdapter) PackEip712Domain() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("eip712Domain")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackEip712Domain() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("eip712Domain")
}

// Eip712DomainOutput serves as a container for the return parameters of contract
// method Eip712Domain.
type Eip712DomainOutput struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}

// UnpackEip712Domain is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackEip712Domain(data []byte) (Eip712DomainOutput, error) {
	out, err := liquidLaneAdapter.abi.Unpack("eip712Domain", data)
	outstruct := new(Eip712DomainOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)
	return *outstruct, nil
}

// PackFreeAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x11f240ac.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (liquidLaneAdapter *LiquidLaneAdapter) PackFreeAssets() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("freeAssets")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackFreeAssets() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("freeAssets")
}

// UnpackFreeAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x11f240ac.
//
// Solidity: function freeAssets() view returns(uint256 assets)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackFreeAssets(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("freeAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackGetAmountOut is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca706bcf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) PackGetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("getAmountOut", tokenToRedeem, amountIn)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetAmountOut is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xca706bcf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackGetAmountOut(tokenToRedeem common.Address, amountIn *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("getAmountOut", tokenToRedeem, amountIn)
}

// UnpackGetAmountOut is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xca706bcf.
//
// Solidity: function getAmountOut(address tokenToRedeem, uint256 amountIn) view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackGetAmountOut(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("getAmountOut", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackGetMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x22135549.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getMaxAssets(address tokenToRedeem) returns(uint256 assets)
func (liquidLaneAdapter *LiquidLaneAdapter) PackGetMaxAssets(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("getMaxAssets", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetMaxAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x22135549.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getMaxAssets(address tokenToRedeem) returns(uint256 assets)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackGetMaxAssets(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("getMaxAssets", tokenToRedeem)
}

// UnpackGetMaxAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x22135549.
//
// Solidity: function getMaxAssets(address tokenToRedeem) returns(uint256 assets)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackGetMaxAssets(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("getMaxAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackGetMaxRate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa99c53b3.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getMaxRate(address tokenToRedeem) view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) PackGetMaxRate(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("getMaxRate", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetMaxRate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xa99c53b3.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getMaxRate(address tokenToRedeem) view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackGetMaxRate(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("getMaxRate", tokenToRedeem)
}

// UnpackGetMaxRate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xa99c53b3.
//
// Solidity: function getMaxRate(address tokenToRedeem) view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackGetMaxRate(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("getMaxRate", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackGetTokensToRedeemLength is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x55d931bf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getTokensToRedeemLength() view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) PackGetTokensToRedeemLength() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("getTokensToRedeemLength")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetTokensToRedeemLength is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x55d931bf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getTokensToRedeemLength() view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackGetTokensToRedeemLength() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("getTokensToRedeemLength")
}

// UnpackGetTokensToRedeemLength is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x55d931bf.
//
// Solidity: function getTokensToRedeemLength() view returns(uint256)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackGetTokensToRedeemLength(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("getTokensToRedeemLength", data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) PackInitialize(initialVersion uint64, owner common.Address, data []byte) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("initialize", initialVersion, owner, data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackInitialize(initialVersion uint64, owner common.Address, data []byte) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("initialize", initialVersion, owner, data)
}

// PackInvalidateNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0d3762b5.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function invalidateNonce(address tokenToRedeem, uint256 nonce) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackInvalidateNonce(tokenToRedeem common.Address, nonce *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("invalidateNonce", tokenToRedeem, nonce)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInvalidateNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0d3762b5.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function invalidateNonce(address tokenToRedeem, uint256 nonce) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackInvalidateNonce(tokenToRedeem common.Address, nonce *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("invalidateNonce", tokenToRedeem, nonce)
}

// PackIsFiller is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb0f9fe6d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) PackIsFiller(marketMaker common.Address, filler common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("isFiller", marketMaker, filler)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsFiller is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb0f9fe6d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackIsFiller(marketMaker common.Address, filler common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("isFiller", marketMaker, filler)
}

// UnpackIsFiller is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xb0f9fe6d.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackIsFiller(data []byte) (bool, error) {
	out, err := liquidLaneAdapter.abi.Unpack("isFiller", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackIsUsedNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ee60fa7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) PackIsUsedNonce(tokenToRedeem common.Address, nonce *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("isUsedNonce", tokenToRedeem, nonce)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsUsedNonce is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ee60fa7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackIsUsedNonce(tokenToRedeem common.Address, nonce *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("isUsedNonce", tokenToRedeem, nonce)
}

// UnpackIsUsedNonce is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0ee60fa7.
//
// Solidity: function isUsedNonce(address tokenToRedeem, uint256 nonce) view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackIsUsedNonce(data []byte) (bool, error) {
	out, err := liquidLaneAdapter.abi.Unpack("isUsedNonce", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd8797262.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function limit(address tokenToRedeem) view returns(uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) PackLimit(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("limit", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd8797262.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function limit(address tokenToRedeem) view returns(uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackLimit(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("limit", tokenToRedeem)
}

// UnpackLimit is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd8797262.
//
// Solidity: function limit(address tokenToRedeem) view returns(uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackLimit(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("limit", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMarketMaker is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1f21f9af.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function marketMaker() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) PackMarketMaker() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("marketMaker")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMarketMaker is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1f21f9af.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function marketMaker() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackMarketMaker() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("marketMaker")
}

// UnpackMarketMaker is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1f21f9af.
//
// Solidity: function marketMaker() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackMarketMaker(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("marketMaker", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackMarketMakerCanAcquire is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe1d594cb.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function marketMakerCanAcquire() view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) PackMarketMakerCanAcquire() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("marketMakerCanAcquire")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMarketMakerCanAcquire is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe1d594cb.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function marketMakerCanAcquire() view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackMarketMakerCanAcquire() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("marketMakerCanAcquire")
}

// UnpackMarketMakerCanAcquire is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xe1d594cb.
//
// Solidity: function marketMakerCanAcquire() view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackMarketMakerCanAcquire(data []byte) (bool, error) {
	out, err := liquidLaneAdapter.abi.Unpack("marketMakerCanAcquire", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2abe3048.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackMigrate(newVersion uint64, data []byte) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("migrate", newVersion, data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackMigrate(newVersion uint64, data []byte) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("migrate", newVersion, data)
}

// PackMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd9104c14.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 ppm)
func (liquidLaneAdapter *LiquidLaneAdapter) PackMinDiscount(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("minDiscount", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd9104c14.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 ppm)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackMinDiscount(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("minDiscount", tokenToRedeem)
}

// UnpackMinDiscount is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd9104c14.
//
// Solidity: function minDiscount(address tokenToRedeem) view returns(uint256 ppm)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackMinDiscount(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("minDiscount", data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) PackMulticall(data [][]byte) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("multicall", data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackMulticall(data [][]byte) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("multicall", data)
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) PackOwner() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("owner")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackOwner() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackOwner(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("owner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackPause is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8456cb59.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function pause() returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackPause() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("pause")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPause is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8456cb59.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function pause() returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackPause() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("pause")
}

// PackPaused is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5c975abb.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function paused() view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) PackPaused() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("paused")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPaused is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5c975abb.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function paused() view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackPaused() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("paused")
}

// UnpackPaused is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackPaused(data []byte) (bool, error) {
	out, err := liquidLaneAdapter.abi.Unpack("paused", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackPauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9fd0506d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function pauser() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) PackPauser() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("pauser")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9fd0506d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function pauser() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackPauser() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("pauser")
}

// UnpackPauser is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x9fd0506d.
//
// Solidity: function pauser() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackPauser(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("pauser", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9da41802.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function receiver(address who) view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) PackReceiver(who common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("receiver", who)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9da41802.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function receiver(address who) view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackReceiver(who common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("receiver", who)
}

// UnpackReceiver is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x9da41802.
//
// Solidity: function receiver(address who) view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackReceiver(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("receiver", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackRemoveTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe250fef7.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function removeTokenToRedeem(address tokenToRedeem) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackRemoveTokenToRedeem(tokenToRedeem common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("removeTokenToRedeem", tokenToRedeem)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRemoveTokenToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xe250fef7.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function removeTokenToRedeem(address tokenToRedeem) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackRemoveTokenToRedeem(tokenToRedeem common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("removeTokenToRedeem", tokenToRedeem)
}

// PackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function renounceOwnership() returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackRenounceOwnership() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("renounceOwnership")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackRenounceOwnership() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("renounceOwnership")
}

// PackRequestDeallocate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf79f679d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function requestDeallocate(uint256 amount) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackRequestDeallocate(amount *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("requestDeallocate", amount)
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackRequestDeallocate(amount *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("requestDeallocate", amount)
}

// PackSetFiller is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6af7897.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setFiller(address filler, bool status) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSetFiller(filler common.Address, status bool) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("setFiller", filler, status)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetFiller is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6af7897.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setFiller(address filler, bool status) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSetFiller(filler common.Address, status bool) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("setFiller", filler, status)
}

// PackSetLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x36db43b5.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setLimit(address tokenToRedeem, uint256 newLimit) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSetLimit(tokenToRedeem common.Address, newLimit *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("setLimit", tokenToRedeem, newLimit)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetLimit is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x36db43b5.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setLimit(address tokenToRedeem, uint256 newLimit) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSetLimit(tokenToRedeem common.Address, newLimit *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("setLimit", tokenToRedeem, newLimit)
}

// PackSetMarketMaker is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd6e2df05.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setMarketMaker(address newMarketMaker, bool newCanAcquire) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSetMarketMaker(newMarketMaker common.Address, newCanAcquire bool) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("setMarketMaker", newMarketMaker, newCanAcquire)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetMarketMaker is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd6e2df05.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setMarketMaker(address newMarketMaker, bool newCanAcquire) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSetMarketMaker(newMarketMaker common.Address, newCanAcquire bool) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("setMarketMaker", newMarketMaker, newCanAcquire)
}

// PackSetMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0517ef9a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setMinDiscount(address tokenToRedeem, uint256 newMinDiscount) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSetMinDiscount(tokenToRedeem common.Address, newMinDiscount *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("setMinDiscount", tokenToRedeem, newMinDiscount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetMinDiscount is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0517ef9a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setMinDiscount(address tokenToRedeem, uint256 newMinDiscount) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSetMinDiscount(tokenToRedeem common.Address, newMinDiscount *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("setMinDiscount", tokenToRedeem, newMinDiscount)
}

// PackSetPauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2d88af4a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setPauser(address newPauser) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSetPauser(newPauser common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("setPauser", newPauser)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetPauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2d88af4a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setPauser(address newPauser) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSetPauser(newPauser common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("setPauser", newPauser)
}

// PackSetReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x718da7ee.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setReceiver(address newReceiver) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSetReceiver(newReceiver common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("setReceiver", newReceiver)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetReceiver is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x718da7ee.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setReceiver(address newReceiver) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSetReceiver(newReceiver common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("setReceiver", newReceiver)
}

// PackSetUnpauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xce548428.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setUnpauser(address newUnpauser) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSetUnpauser(newUnpauser common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("setUnpauser", newUnpauser)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetUnpauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xce548428.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setUnpauser(address newUnpauser) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSetUnpauser(newUnpauser common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("setUnpauser", newUnpauser)
}

// PackSwap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x53f549d8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function swap((address,address,uint256,uint256) swap) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSwap(swap ILiquidLaneAdapterSwap) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("swap", swap)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSwap is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x53f549d8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function swap((address,address,uint256,uint256) swap) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSwap(swap ILiquidLaneAdapterSwap) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("swap", swap)
}

// PackSwap0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8fa5c671.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneAdapter *LiquidLaneAdapter) PackSwap0(discountSwap ILiquidLaneAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("swap0", discountSwap, protocolSignature, recipient, amountIn)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSwap0 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8fa5c671.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSwap0(discountSwap ILiquidLaneAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("swap0", discountSwap, protocolSignature, recipient, amountIn)
}

// UnpackSwap0 is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8fa5c671.
//
// Solidity: function swap(((address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn) returns(uint256 amountOut)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSwap0(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("swap0", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackSwap1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9a4568b6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function swap((address,address,uint256,uint256,address,address,uint256,uint48) signedSwap, bytes signature) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackSwap1(signedSwap ILiquidLaneAdapterSignedSwap, signature []byte) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("swap1", signedSwap, signature)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSwap1 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9a4568b6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function swap((address,address,uint256,uint256,address,address,uint256,uint48) signedSwap, bytes signature) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackSwap1(signedSwap ILiquidLaneAdapterSignedSwap, signature []byte) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("swap1", signedSwap, signature)
}

// PackTokensToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3d2a61ce.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function tokensToRedeem(uint256 ) view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) PackTokensToRedeem(arg0 *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("tokensToRedeem", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTokensToRedeem is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3d2a61ce.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function tokensToRedeem(uint256 ) view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackTokensToRedeem(arg0 *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("tokensToRedeem", arg0)
}

// UnpackTokensToRedeem is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x3d2a61ce.
//
// Solidity: function tokensToRedeem(uint256 ) view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackTokensToRedeem(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("tokensToRedeem", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackTotalAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x01e1d114.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalAssets() view returns(uint256 assets)
func (liquidLaneAdapter *LiquidLaneAdapter) PackTotalAssets() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("totalAssets")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackTotalAssets() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("totalAssets")
}

// UnpackTotalAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x01e1d114.
//
// Solidity: function totalAssets() view returns(uint256 assets)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackTotalAssets(data []byte) (*big.Int, error) {
	out, err := liquidLaneAdapter.abi.Unpack("totalAssets", data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("transferOwnership", newOwner)
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("transferOwnership", newOwner)
}

// PackUnpause is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3f4ba83a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function unpause() returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackUnpause() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("unpause")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackUnpause is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3f4ba83a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function unpause() returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackUnpause() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("unpause")
}

// PackUnpauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xeab66d7a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function unpauser() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) PackUnpauser() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("unpauser")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackUnpauser is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xeab66d7a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function unpauser() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackUnpauser() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("unpauser")
}

// UnpackUnpauser is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xeab66d7a.
//
// Solidity: function unpauser() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackUnpauser(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("unpauser", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackVault is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfbfa77cf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function vault() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) PackVault() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("vault")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackVault() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("vault")
}

// UnpackVault is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfbfa77cf.
//
// Solidity: function vault() view returns(address)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackVault(data []byte) (common.Address, error) {
	out, err := liquidLaneAdapter.abi.Unpack("vault", data)
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
func (liquidLaneAdapter *LiquidLaneAdapter) PackVersion() []byte {
	enc, err := liquidLaneAdapter.abi.Pack("version")
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
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackVersion() ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackVersion(data []byte) (uint64, error) {
	out, err := liquidLaneAdapter.abi.Unpack("version", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// PackWithdrawToAcquire is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x00812e34.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function withdrawToAcquire(address tokenToRedeem, uint256 amount) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) PackWithdrawToAcquire(tokenToRedeem common.Address, amount *big.Int) []byte {
	enc, err := liquidLaneAdapter.abi.Pack("withdrawToAcquire", tokenToRedeem, amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackWithdrawToAcquire is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x00812e34.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function withdrawToAcquire(address tokenToRedeem, uint256 amount) returns()
func (liquidLaneAdapter *LiquidLaneAdapter) TryPackWithdrawToAcquire(tokenToRedeem common.Address, amount *big.Int) ([]byte, error) {
	return liquidLaneAdapter.abi.Pack("withdrawToAcquire", tokenToRedeem, amount)
}

// LiquidLaneAdapterAddTokenToRedeem represents a AddTokenToRedeem event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterAddTokenToRedeem struct {
	TokenToRedeem common.Address
	Account       common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterAddTokenToRedeemEventName = "AddTokenToRedeem"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterAddTokenToRedeem) ContractEventName() string {
	return LiquidLaneAdapterAddTokenToRedeemEventName
}

// UnpackAddTokenToRedeemEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event AddTokenToRedeem(address indexed tokenToRedeem, address indexed account)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAddTokenToRedeemEvent(log *types.Log) (*LiquidLaneAdapterAddTokenToRedeem, error) {
	event := "AddTokenToRedeem"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterAddTokenToRedeem)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterDepositToAcquire represents a DepositToAcquire event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterDepositToAcquire struct {
	Who           common.Address
	TokenToRedeem common.Address
	Amount        *big.Int
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterDepositToAcquireEventName = "DepositToAcquire"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterDepositToAcquire) ContractEventName() string {
	return LiquidLaneAdapterDepositToAcquireEventName
}

// UnpackDepositToAcquireEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event DepositToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackDepositToAcquireEvent(log *types.Log) (*LiquidLaneAdapterDepositToAcquire, error) {
	event := "DepositToAcquire"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterDepositToAcquire)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterDoSwap represents a DoSwap event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterDoSwap struct {
	Swap ILiquidLaneAdapterSwap
	Raw  *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterDoSwapEventName = "DoSwap"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterDoSwap) ContractEventName() string {
	return LiquidLaneAdapterDoSwapEventName
}

// UnpackDoSwapEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event DoSwap((address,address,uint256,uint256) swap)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackDoSwapEvent(log *types.Log) (*LiquidLaneAdapterDoSwap, error) {
	event := "DoSwap"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterDoSwap)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterEIP712DomainChanged represents a EIP712DomainChanged event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterEIP712DomainChanged struct {
	Raw *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterEIP712DomainChangedEventName = "EIP712DomainChanged"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterEIP712DomainChanged) ContractEventName() string {
	return LiquidLaneAdapterEIP712DomainChangedEventName
}

// UnpackEIP712DomainChangedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event EIP712DomainChanged()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackEIP712DomainChangedEvent(log *types.Log) (*LiquidLaneAdapterEIP712DomainChanged, error) {
	event := "EIP712DomainChanged"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterEIP712DomainChanged)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterInitialize represents a Initialize event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInitialize struct {
	Params ILiquidLaneAdapterInitParams
	Raw    *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterInitializeEventName = "Initialize"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterInitialize) ContractEventName() string {
	return LiquidLaneAdapterInitializeEventName
}

// UnpackInitializeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialize((address,address) params)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInitializeEvent(log *types.Log) (*LiquidLaneAdapterInitialize, error) {
	event := "Initialize"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterInitialize)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterInitialized represents a Initialized event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInitialized struct {
	Version uint64
	Raw     *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterInitializedEventName = "Initialized"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterInitialized) ContractEventName() string {
	return LiquidLaneAdapterInitializedEventName
}

// UnpackInitializedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialized(uint64 version)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInitializedEvent(log *types.Log) (*LiquidLaneAdapterInitialized, error) {
	event := "Initialized"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterInitialized)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterInvalidateNonce represents a InvalidateNonce event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidateNonce struct {
	TokenToRedeem common.Address
	Nonce         *big.Int
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterInvalidateNonceEventName = "InvalidateNonce"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterInvalidateNonce) ContractEventName() string {
	return LiquidLaneAdapterInvalidateNonceEventName
}

// UnpackInvalidateNonceEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event InvalidateNonce(address indexed tokenToRedeem, uint256 indexed nonce)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidateNonceEvent(log *types.Log) (*LiquidLaneAdapterInvalidateNonce, error) {
	event := "InvalidateNonce"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterInvalidateNonce)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterOwnershipTransferred represents a OwnershipTransferred event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterOwnershipTransferred) ContractEventName() string {
	return LiquidLaneAdapterOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackOwnershipTransferredEvent(log *types.Log) (*LiquidLaneAdapterOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterPaused represents a Paused event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterPaused struct {
	Account common.Address
	Raw     *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterPausedEventName = "Paused"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterPaused) ContractEventName() string {
	return LiquidLaneAdapterPausedEventName
}

// UnpackPausedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Paused(address account)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackPausedEvent(log *types.Log) (*LiquidLaneAdapterPaused, error) {
	event := "Paused"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterPaused)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterRemoveTokenToRedeem represents a RemoveTokenToRedeem event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterRemoveTokenToRedeem struct {
	TokenToRedeem common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterRemoveTokenToRedeemEventName = "RemoveTokenToRedeem"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterRemoveTokenToRedeem) ContractEventName() string {
	return LiquidLaneAdapterRemoveTokenToRedeemEventName
}

// UnpackRemoveTokenToRedeemEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event RemoveTokenToRedeem(address indexed tokenToRedeem)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackRemoveTokenToRedeemEvent(log *types.Log) (*LiquidLaneAdapterRemoveTokenToRedeem, error) {
	event := "RemoveTokenToRedeem"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterRemoveTokenToRedeem)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetFiller represents a SetFiller event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetFiller struct {
	MarketMaker  common.Address
	Filler       common.Address
	IsAuthorized bool
	Raw          *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetFillerEventName = "SetFiller"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetFiller) ContractEventName() string {
	return LiquidLaneAdapterSetFillerEventName
}

// UnpackSetFillerEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetFiller(address indexed marketMaker, address indexed filler, bool isAuthorized)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetFillerEvent(log *types.Log) (*LiquidLaneAdapterSetFiller, error) {
	event := "SetFiller"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetFiller)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetLimit represents a SetLimit event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetLimit struct {
	TokenToRedeem common.Address
	NewLimit      *big.Int
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetLimitEventName = "SetLimit"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetLimit) ContractEventName() string {
	return LiquidLaneAdapterSetLimitEventName
}

// UnpackSetLimitEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetLimit(address indexed tokenToRedeem, uint256 newLimit)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetLimitEvent(log *types.Log) (*LiquidLaneAdapterSetLimit, error) {
	event := "SetLimit"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetLimit)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetMarketMaker represents a SetMarketMaker event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetMarketMaker struct {
	NewMarketMaker common.Address
	NewCanAcquire  bool
	Raw            *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetMarketMakerEventName = "SetMarketMaker"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetMarketMaker) ContractEventName() string {
	return LiquidLaneAdapterSetMarketMakerEventName
}

// UnpackSetMarketMakerEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetMarketMaker(address indexed newMarketMaker, bool newCanAcquire)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetMarketMakerEvent(log *types.Log) (*LiquidLaneAdapterSetMarketMaker, error) {
	event := "SetMarketMaker"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetMarketMaker)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetMinDiscount represents a SetMinDiscount event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetMinDiscount struct {
	TokenToRedeem  common.Address
	NewMinDiscount *big.Int
	Raw            *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetMinDiscountEventName = "SetMinDiscount"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetMinDiscount) ContractEventName() string {
	return LiquidLaneAdapterSetMinDiscountEventName
}

// UnpackSetMinDiscountEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetMinDiscount(address indexed tokenToRedeem, uint256 newMinDiscount)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetMinDiscountEvent(log *types.Log) (*LiquidLaneAdapterSetMinDiscount, error) {
	event := "SetMinDiscount"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetMinDiscount)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetPauser represents a SetPauser event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetPauser struct {
	NewPauser common.Address
	Raw       *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetPauserEventName = "SetPauser"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetPauser) ContractEventName() string {
	return LiquidLaneAdapterSetPauserEventName
}

// UnpackSetPauserEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetPauser(address indexed newPauser)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetPauserEvent(log *types.Log) (*LiquidLaneAdapterSetPauser, error) {
	event := "SetPauser"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetPauser)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetReceiver represents a SetReceiver event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetReceiver struct {
	Who      common.Address
	Receiver common.Address
	Raw      *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetReceiverEventName = "SetReceiver"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetReceiver) ContractEventName() string {
	return LiquidLaneAdapterSetReceiverEventName
}

// UnpackSetReceiverEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetReceiver(address indexed who, address indexed receiver)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetReceiverEvent(log *types.Log) (*LiquidLaneAdapterSetReceiver, error) {
	event := "SetReceiver"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetReceiver)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetUnpauser represents a SetUnpauser event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetUnpauser struct {
	NewUnpauser common.Address
	Raw         *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetUnpauserEventName = "SetUnpauser"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetUnpauser) ContractEventName() string {
	return LiquidLaneAdapterSetUnpauserEventName
}

// UnpackSetUnpauserEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetUnpauser(address indexed newUnpauser)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetUnpauserEvent(log *types.Log) (*LiquidLaneAdapterSetUnpauser, error) {
	event := "SetUnpauser"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetUnpauser)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterSetVault represents a SetVault event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSetVault struct {
	Vault common.Address
	Raw   *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterSetVaultEventName = "SetVault"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterSetVault) ContractEventName() string {
	return LiquidLaneAdapterSetVaultEventName
}

// UnpackSetVaultEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetVault(address indexed vault)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSetVaultEvent(log *types.Log) (*LiquidLaneAdapterSetVault, error) {
	event := "SetVault"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterSetVault)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterUnpaused represents a Unpaused event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterUnpaused struct {
	Account common.Address
	Raw     *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterUnpausedEventName = "Unpaused"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterUnpaused) ContractEventName() string {
	return LiquidLaneAdapterUnpausedEventName
}

// UnpackUnpausedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Unpaused(address account)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackUnpausedEvent(log *types.Log) (*LiquidLaneAdapterUnpaused, error) {
	event := "Unpaused"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterUnpaused)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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

// LiquidLaneAdapterWithdrawToAcquire represents a WithdrawToAcquire event raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterWithdrawToAcquire struct {
	Who           common.Address
	TokenToRedeem common.Address
	Amount        *big.Int
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneAdapterWithdrawToAcquireEventName = "WithdrawToAcquire"

// ContractEventName returns the user-defined event name.
func (LiquidLaneAdapterWithdrawToAcquire) ContractEventName() string {
	return LiquidLaneAdapterWithdrawToAcquireEventName
}

// UnpackWithdrawToAcquireEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event WithdrawToAcquire(address indexed who, address indexed tokenToRedeem, uint256 amount)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackWithdrawToAcquireEvent(log *types.Log) (*LiquidLaneAdapterWithdrawToAcquire, error) {
	event := "WithdrawToAcquire"
	if log.Topics[0] != liquidLaneAdapter.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(LiquidLaneAdapterWithdrawToAcquire)
	if len(log.Data) > 0 {
		if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneAdapter.abi.Events[event].Inputs {
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
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["AccountHasAssets"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackAccountHasAssetsError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["AlreadyInitialized"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["AlreadyUsedNonce"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackAlreadyUsedNonceError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["DepositNotAllowed"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackDepositNotAllowedError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["EnforcedPause"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackEnforcedPauseError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["ExpectedPause"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackExpectedPauseError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["ExpiredSwap"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackExpiredSwapError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InsufficientAllocate"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInsufficientAllocateError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidAccount"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidCaller"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidCallerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidDiscount"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidDiscountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidInitialization"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidInitializationError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidOracle"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidOracleError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidReceiver"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidReceiverError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidShortString"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidShortStringError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidSignature"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidSignatureError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidSwapRate"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidSwapRateError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidTokenToRedeem"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidTokenToRedeemError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["InvalidVault"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackInvalidVaultError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["LimitExceeded"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackLimitExceededError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["NotFactory"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackNotFactoryError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["NotInitialized"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackNotInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["NotInitializing"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackNotInitializingError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["NotVault"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackNotVaultError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["OwnableInvalidOwner"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackOwnableInvalidOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["OwnableUnauthorizedAccount"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackOwnableUnauthorizedAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["ReentrancyGuardReentrantCall"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackReentrancyGuardReentrantCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["StringTooLong"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackStringTooLongError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneAdapter.abi.Errors["TooManyTokensToRedeem"].ID.Bytes()[:4]) {
		return liquidLaneAdapter.UnpackTooManyTokensToRedeemError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// LiquidLaneAdapterAccountHasAssets represents a AccountHasAssets error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterAccountHasAssets struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AccountHasAssets()
func LiquidLaneAdapterAccountHasAssetsErrorID() common.Hash {
	return common.HexToHash("0x4e6ae97885cebdf3331628d3d6398e3265838ddb78f4b379444ca5d6347d5948")
}

// UnpackAccountHasAssetsError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AccountHasAssets()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAccountHasAssetsError(raw []byte) (*LiquidLaneAdapterAccountHasAssets, error) {
	out := new(LiquidLaneAdapterAccountHasAssets)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "AccountHasAssets", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterAlreadyInitialized represents a AlreadyInitialized error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterAlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyInitialized()
func LiquidLaneAdapterAlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x0dc149f07762891dbcea3fe72770f3d63a1863fc54b2f084e8c59ec476996927")
}

// UnpackAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyInitialized()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAlreadyInitializedError(raw []byte) (*LiquidLaneAdapterAlreadyInitialized, error) {
	out := new(LiquidLaneAdapterAlreadyInitialized)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "AlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterAlreadyUsedNonce represents a AlreadyUsedNonce error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterAlreadyUsedNonce struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyUsedNonce()
func LiquidLaneAdapterAlreadyUsedNonceErrorID() common.Hash {
	return common.HexToHash("0x5247cb318fe1d375b7264beff576b70703d01bb4ac69fa1e1530f58741bcaf65")
}

// UnpackAlreadyUsedNonceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyUsedNonce()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackAlreadyUsedNonceError(raw []byte) (*LiquidLaneAdapterAlreadyUsedNonce, error) {
	out := new(LiquidLaneAdapterAlreadyUsedNonce)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "AlreadyUsedNonce", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterDepositNotAllowed represents a DepositNotAllowed error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterDepositNotAllowed struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error DepositNotAllowed()
func LiquidLaneAdapterDepositNotAllowedErrorID() common.Hash {
	return common.HexToHash("0x3d90e2a0405023b53b5e49483fcee2800f14df58d4aebaccd7becbbe7d2ac6b0")
}

// UnpackDepositNotAllowedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error DepositNotAllowed()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackDepositNotAllowedError(raw []byte) (*LiquidLaneAdapterDepositNotAllowed, error) {
	out := new(LiquidLaneAdapterDepositNotAllowed)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "DepositNotAllowed", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterEnforcedPause represents a EnforcedPause error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterEnforcedPause struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error EnforcedPause()
func LiquidLaneAdapterEnforcedPauseErrorID() common.Hash {
	return common.HexToHash("0xd93c0665d6c96d04a8f174024fc4ddd66c250604aff22bbec808de86dd3637e3")
}

// UnpackEnforcedPauseError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error EnforcedPause()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackEnforcedPauseError(raw []byte) (*LiquidLaneAdapterEnforcedPause, error) {
	out := new(LiquidLaneAdapterEnforcedPause)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "EnforcedPause", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterExpectedPause represents a ExpectedPause error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterExpectedPause struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ExpectedPause()
func LiquidLaneAdapterExpectedPauseErrorID() common.Hash {
	return common.HexToHash("0x8dfc202bcfe9a735b559bee70674422512bc5c30f687046ae8778315fb81da44")
}

// UnpackExpectedPauseError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ExpectedPause()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackExpectedPauseError(raw []byte) (*LiquidLaneAdapterExpectedPause, error) {
	out := new(LiquidLaneAdapterExpectedPause)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "ExpectedPause", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterExpiredSwap represents a ExpiredSwap error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterExpiredSwap struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ExpiredSwap()
func LiquidLaneAdapterExpiredSwapErrorID() common.Hash {
	return common.HexToHash("0x1c74180ce4ab063a7fe469e2a3b2a873fe947c3b7649f1a95af498681712959d")
}

// UnpackExpiredSwapError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ExpiredSwap()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackExpiredSwapError(raw []byte) (*LiquidLaneAdapterExpiredSwap, error) {
	out := new(LiquidLaneAdapterExpiredSwap)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "ExpiredSwap", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInsufficientAllocate represents a InsufficientAllocate error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInsufficientAllocate struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientAllocate()
func LiquidLaneAdapterInsufficientAllocateErrorID() common.Hash {
	return common.HexToHash("0xb128897f3cb0ff1be99d96c4772ed6c60ee2a8e88745c65f2a907980f83cad61")
}

// UnpackInsufficientAllocateError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientAllocate()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInsufficientAllocateError(raw []byte) (*LiquidLaneAdapterInsufficientAllocate, error) {
	out := new(LiquidLaneAdapterInsufficientAllocate)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InsufficientAllocate", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidAccount represents a InvalidAccount error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidAccount struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAccount()
func LiquidLaneAdapterInvalidAccountErrorID() common.Hash {
	return common.HexToHash("0x6d187b28dc0ac7ec3747dcca312a7f01229ba51941237588cf813e48090f2a50")
}

// UnpackInvalidAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAccount()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidAccountError(raw []byte) (*LiquidLaneAdapterInvalidAccount, error) {
	out := new(LiquidLaneAdapterInvalidAccount)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidCaller represents a InvalidCaller error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidCaller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidCaller()
func LiquidLaneAdapterInvalidCallerErrorID() common.Hash {
	return common.HexToHash("0x48f5c3ed50241a1b6c87d204a25d9d01339cd768de9a714ffbb53a5bb6ad572a")
}

// UnpackInvalidCallerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidCaller()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidCallerError(raw []byte) (*LiquidLaneAdapterInvalidCaller, error) {
	out := new(LiquidLaneAdapterInvalidCaller)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidCaller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidDiscount represents a InvalidDiscount error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidDiscount struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidDiscount()
func LiquidLaneAdapterInvalidDiscountErrorID() common.Hash {
	return common.HexToHash("0x997ea3601b175042a225bcd305c8e0434449bb795e78e4cb3f93a718c1a88caf")
}

// UnpackInvalidDiscountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidDiscount()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidDiscountError(raw []byte) (*LiquidLaneAdapterInvalidDiscount, error) {
	out := new(LiquidLaneAdapterInvalidDiscount)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidDiscount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidInitialization represents a InvalidInitialization error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidInitialization struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidInitialization()
func LiquidLaneAdapterInvalidInitializationErrorID() common.Hash {
	return common.HexToHash("0xf92ee8a957075833165f68c320933b1a1294aafc84ee6e0dd3fb178008f9aaf5")
}

// UnpackInvalidInitializationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidInitialization()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidInitializationError(raw []byte) (*LiquidLaneAdapterInvalidInitialization, error) {
	out := new(LiquidLaneAdapterInvalidInitialization)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidInitialization", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidOracle represents a InvalidOracle error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidOracle struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidOracle()
func LiquidLaneAdapterInvalidOracleErrorID() common.Hash {
	return common.HexToHash("0x9589a27d464cce309224596a505cbfd22e5fde1f0f420cecf8a6b6c1d65791b6")
}

// UnpackInvalidOracleError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidOracle()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidOracleError(raw []byte) (*LiquidLaneAdapterInvalidOracle, error) {
	out := new(LiquidLaneAdapterInvalidOracle)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidOracle", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidReceiver represents a InvalidReceiver error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidReceiver struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidReceiver()
func LiquidLaneAdapterInvalidReceiverErrorID() common.Hash {
	return common.HexToHash("0x1e4ec46ba639431522ed9b9ce6d5e4d79b6e4cb2c689f14af956db4edf067a7d")
}

// UnpackInvalidReceiverError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidReceiver()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidReceiverError(raw []byte) (*LiquidLaneAdapterInvalidReceiver, error) {
	out := new(LiquidLaneAdapterInvalidReceiver)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidReceiver", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidShortString represents a InvalidShortString error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidShortString struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidShortString()
func LiquidLaneAdapterInvalidShortStringErrorID() common.Hash {
	return common.HexToHash("0xb3512b0c6163e5f0bafab72bb631b9d58cd7a731b082f910338aa21c83d5c274")
}

// UnpackInvalidShortStringError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidShortString()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidShortStringError(raw []byte) (*LiquidLaneAdapterInvalidShortString, error) {
	out := new(LiquidLaneAdapterInvalidShortString)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidShortString", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidSignature represents a InvalidSignature error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidSignature struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidSignature()
func LiquidLaneAdapterInvalidSignatureErrorID() common.Hash {
	return common.HexToHash("0x8baa579fce362245063d36f11747a89dd489c54795634fc673cc0e0db51fedc5")
}

// UnpackInvalidSignatureError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidSignature()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidSignatureError(raw []byte) (*LiquidLaneAdapterInvalidSignature, error) {
	out := new(LiquidLaneAdapterInvalidSignature)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidSignature", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidSwapRate represents a InvalidSwapRate error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidSwapRate struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidSwapRate()
func LiquidLaneAdapterInvalidSwapRateErrorID() common.Hash {
	return common.HexToHash("0x285217814127ad6ebb3698d6e77d442a1bfc2ce040377b7a098f217e85a0789e")
}

// UnpackInvalidSwapRateError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidSwapRate()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidSwapRateError(raw []byte) (*LiquidLaneAdapterInvalidSwapRate, error) {
	out := new(LiquidLaneAdapterInvalidSwapRate)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidSwapRate", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidTokenToRedeem represents a InvalidTokenToRedeem error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidTokenToRedeem struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidTokenToRedeem()
func LiquidLaneAdapterInvalidTokenToRedeemErrorID() common.Hash {
	return common.HexToHash("0x997c4c8864a0026f8a0f15ff73d73b124e940d19db898a179d4983158f41cdf5")
}

// UnpackInvalidTokenToRedeemError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidTokenToRedeem()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidTokenToRedeemError(raw []byte) (*LiquidLaneAdapterInvalidTokenToRedeem, error) {
	out := new(LiquidLaneAdapterInvalidTokenToRedeem)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidTokenToRedeem", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterInvalidVault represents a InvalidVault error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterInvalidVault struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidVault()
func LiquidLaneAdapterInvalidVaultErrorID() common.Hash {
	return common.HexToHash("0xd03a63207f799c8b4a310cf73db481de483ce6543ef24d1f75f918a11e4eae1f")
}

// UnpackInvalidVaultError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidVault()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackInvalidVaultError(raw []byte) (*LiquidLaneAdapterInvalidVault, error) {
	out := new(LiquidLaneAdapterInvalidVault)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "InvalidVault", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterLimitExceeded represents a LimitExceeded error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterLimitExceeded struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error LimitExceeded()
func LiquidLaneAdapterLimitExceededErrorID() common.Hash {
	return common.HexToHash("0x3261c792e4ead8e67529b6c0419eb29dc66259ed919d3fb84aaac617f6deef8c")
}

// UnpackLimitExceededError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error LimitExceeded()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackLimitExceededError(raw []byte) (*LiquidLaneAdapterLimitExceeded, error) {
	out := new(LiquidLaneAdapterLimitExceeded)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "LimitExceeded", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterNotFactory represents a NotFactory error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterNotFactory struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotFactory()
func LiquidLaneAdapterNotFactoryErrorID() common.Hash {
	return common.HexToHash("0x32cc723614e775fc4a8386492bc9a860c12fe98d5f5f28ec17e265818645b229")
}

// UnpackNotFactoryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotFactory()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackNotFactoryError(raw []byte) (*LiquidLaneAdapterNotFactory, error) {
	out := new(LiquidLaneAdapterNotFactory)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "NotFactory", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterNotInitialized represents a NotInitialized error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterNotInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitialized()
func LiquidLaneAdapterNotInitializedErrorID() common.Hash {
	return common.HexToHash("0x87138d5c8c2e77cb9f25c07b03277aad63d22f6a05255580ec55d2c21666e734")
}

// UnpackNotInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitialized()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackNotInitializedError(raw []byte) (*LiquidLaneAdapterNotInitialized, error) {
	out := new(LiquidLaneAdapterNotInitialized)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "NotInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterNotInitializing represents a NotInitializing error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterNotInitializing struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitializing()
func LiquidLaneAdapterNotInitializingErrorID() common.Hash {
	return common.HexToHash("0xd7e6bcf8597daa127dc9f0048d2f08d5ef140a2cb659feabd700beff1f7a8302")
}

// UnpackNotInitializingError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitializing()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackNotInitializingError(raw []byte) (*LiquidLaneAdapterNotInitializing, error) {
	out := new(LiquidLaneAdapterNotInitializing)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "NotInitializing", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterNotVault represents a NotVault error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterNotVault struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotVault()
func LiquidLaneAdapterNotVaultErrorID() common.Hash {
	return common.HexToHash("0x62df0545b0e47f06f6a9990975121b8c49c83a96f18696393f66a69dd2ffe568")
}

// UnpackNotVaultError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotVault()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackNotVaultError(raw []byte) (*LiquidLaneAdapterNotVault, error) {
	out := new(LiquidLaneAdapterNotVault)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "NotVault", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterOwnableInvalidOwner represents a OwnableInvalidOwner error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterOwnableInvalidOwner struct {
	Owner common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableInvalidOwner(address owner)
func LiquidLaneAdapterOwnableInvalidOwnerErrorID() common.Hash {
	return common.HexToHash("0x1e4fbdf7f3ef8bcaa855599e3abf48b232380f183f08f6f813d9ffa5bd585188")
}

// UnpackOwnableInvalidOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableInvalidOwner(address owner)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackOwnableInvalidOwnerError(raw []byte) (*LiquidLaneAdapterOwnableInvalidOwner, error) {
	out := new(LiquidLaneAdapterOwnableInvalidOwner)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "OwnableInvalidOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterOwnableUnauthorizedAccount represents a OwnableUnauthorizedAccount error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterOwnableUnauthorizedAccount struct {
	Account common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func LiquidLaneAdapterOwnableUnauthorizedAccountErrorID() common.Hash {
	return common.HexToHash("0x118cdaa7a341953d1887a2245fd6665d741c67c8c50581daa59e1d03373fa188")
}

// UnpackOwnableUnauthorizedAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackOwnableUnauthorizedAccountError(raw []byte) (*LiquidLaneAdapterOwnableUnauthorizedAccount, error) {
	out := new(LiquidLaneAdapterOwnableUnauthorizedAccount)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "OwnableUnauthorizedAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterReentrancyGuardReentrantCall represents a ReentrancyGuardReentrantCall error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterReentrancyGuardReentrantCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ReentrancyGuardReentrantCall()
func LiquidLaneAdapterReentrancyGuardReentrantCallErrorID() common.Hash {
	return common.HexToHash("0x3ee5aeb571de7fc460830b4d0017439a1ca56fb0bc39062227ade4fe4a24c1ca")
}

// UnpackReentrancyGuardReentrantCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ReentrancyGuardReentrantCall()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackReentrancyGuardReentrantCallError(raw []byte) (*LiquidLaneAdapterReentrancyGuardReentrantCall, error) {
	out := new(LiquidLaneAdapterReentrancyGuardReentrantCall)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "ReentrancyGuardReentrantCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func LiquidLaneAdapterSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackSafeERC20FailedOperationError(raw []byte) (*LiquidLaneAdapterSafeERC20FailedOperation, error) {
	out := new(LiquidLaneAdapterSafeERC20FailedOperation)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterStringTooLong represents a StringTooLong error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterStringTooLong struct {
	Str string
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error StringTooLong(string str)
func LiquidLaneAdapterStringTooLongErrorID() common.Hash {
	return common.HexToHash("0x305a27a93f8e33b7392df0a0f91d6fc63847395853c45991eec52dbf24d72381")
}

// UnpackStringTooLongError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error StringTooLong(string str)
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackStringTooLongError(raw []byte) (*LiquidLaneAdapterStringTooLong, error) {
	out := new(LiquidLaneAdapterStringTooLong)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "StringTooLong", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneAdapterTooManyTokensToRedeem represents a TooManyTokensToRedeem error raised by the LiquidLaneAdapter contract.
type LiquidLaneAdapterTooManyTokensToRedeem struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TooManyTokensToRedeem()
func LiquidLaneAdapterTooManyTokensToRedeemErrorID() common.Hash {
	return common.HexToHash("0x904f787845fa7d25b0cceb1d59aead95dca226fef10ddd80883ee965763a5df7")
}

// UnpackTooManyTokensToRedeemError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TooManyTokensToRedeem()
func (liquidLaneAdapter *LiquidLaneAdapter) UnpackTooManyTokensToRedeemError(raw []byte) (*LiquidLaneAdapterTooManyTokensToRedeem, error) {
	out := new(LiquidLaneAdapterTooManyTokensToRedeem)
	if err := liquidLaneAdapter.abi.UnpackIntoInterface(out, "TooManyTokensToRedeem", raw); err != nil {
		return nil, err
	}
	return out, nil
}
