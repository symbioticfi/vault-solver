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

// IInstantRedemptionAdapterDiscount is an auto generated low-level Go binding around an user-defined struct.
type IInstantRedemptionAdapterDiscount struct {
	Vault         common.Address
	TokenToRedeem common.Address
	Discount      *big.Int
	Signer        common.Address
	Protocol      common.Address
	Nonce         *big.Int
	Deadline      *big.Int
}

// IInstantRedemptionAdapterDiscountSwap is an auto generated low-level Go binding around an user-defined struct.
type IInstantRedemptionAdapterDiscountSwap struct {
	Discount         IInstantRedemptionAdapterDiscount
	SignerSignature  []byte
	ProtocolDeadline *big.Int
}

// IInstantRedemptionAdapterSignedSwap is an auto generated low-level Go binding around an user-defined struct.
type IInstantRedemptionAdapterSignedSwap struct {
	Recipient common.Address
	Vault     common.Address
	TokenIn   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
	Caller    common.Address
	Signer    common.Address
	Nonce     *big.Int
	Deadline  *big.Int
}

// IInstantRedemptionAdapterSwap is an auto generated low-level Go binding around an user-defined struct.
type IInstantRedemptionAdapterSwap struct {
	Recipient common.Address
	Vault     common.Address
	TokenIn   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

// InstantRedemptionAdapterMetaData contains all meta data concerning the InstantRedemptionAdapter contract.
var InstantRedemptionAdapterMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"curatorRegistry\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"rewards\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vaultFactory\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"PRIVATE_ACCOUNT_BEACON\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"VAULT_FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"accountBeacons\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"beacon\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"allocated\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"claimCuratorAcquired\",\"inputs\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"claimMarketMakerAcquired\",\"inputs\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"convertRedemption\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"redemptionToken\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"redemptionAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"converters\",\"inputs\":[{\"name\":\"redemptionToken\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"collateralToken\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"converter\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"curatorAcquireBalance\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"deallocAdapters\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"deallocatable\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"deallocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"depositToAcquire\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAccount\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAmountOut\",\"inputs\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenOut\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCuratorAccount\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDeallocAdaptersLength\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMarketMakerAccount\",\"inputs\":[{\"name\":\"curMarketMaker\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMaxAssets\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMaxRate\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTokensToRedeemLength\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"globalAllocated\",\"inputs\":[{\"name\":\"collateral\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"globalLimit\",\"inputs\":[{\"name\":\"collateral\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"globalMaxConvertDiscount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"invalidateNonce\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isFiller\",\"inputs\":[{\"name\":\"marketMaker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isPaused\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isUsedNonce\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"limit\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"marketMaker\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"marketMakerAcquireBalances\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"marketMaker\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"marketMakerCanAcquire\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"minDiscount\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ppm\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"oracles\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"oracle\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pairMaxConvertDiscount\",\"inputs\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenOut\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"ppm\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"recover\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setAccountBeacon\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"beacon\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setConversionAdapter\",\"inputs\":[{\"name\":\"redemptionToken\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"collateralToken\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"conversionAdapter\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDeallocAdapters\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newDeallocAdapters\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setFiller\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"isAuthorized\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setGlobalLimit\",\"inputs\":[{\"name\":\"collateral\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setGlobalMaxDiscount\",\"inputs\":[{\"name\":\"newGlobalMaxDiscount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setLimit\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMakerMaker\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMarketMaker\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newCanAcquire\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMinDiscount\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newMinDiscount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setOracle\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"oracle\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPairMaxDiscount\",\"inputs\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenOut\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newPairMaxDiscount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPauseStatus\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newPauseStatus\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"skim\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"skimmable\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structIInstantRedemptionAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structIInstantRedemptionAdapter.Discount\",\"components\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"swap\",\"type\":\"tuple\",\"internalType\":\"structIInstantRedemptionAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swap\",\"inputs\":[{\"name\":\"signedSwap\",\"type\":\"tuple\",\"internalType\":\"structIInstantRedemptionAdapter.SignedSwap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"tokensToRedeem\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdrawToAcquire\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"Allocate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimCuratorAcquired\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimMarketMakerAcquired\",\"inputs\":[{\"name\":\"marketMaker\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ConvertRedemption\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"redemptionToken\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"redemptionAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Deallocate\",\"inputs\":[{\"name\":\"requestedAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"deallocated\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DepositToAcquire\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DoSwap\",\"inputs\":[{\"name\":\"swap\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structIInstantRedemptionAdapter.Swap\",\"components\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenIn\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialize\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"InvalidateNonce\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Recover\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetAccountBeacon\",\"inputs\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"beacon\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetConversionAdapter\",\"inputs\":[{\"name\":\"redemptionToken\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"collateralToken\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"conversionAdapter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDeallocAdapters\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newDeallocAdapters\",\"type\":\"address[]\",\"indexed\":false,\"internalType\":\"address[]\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetFiller\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"marketMaker\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"filler\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"isAuthorized\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetGlobalLimit\",\"inputs\":[{\"name\":\"asset\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"limit\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetGlobalMaxDiscount\",\"inputs\":[{\"name\":\"newGlobalMaxDiscount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetLimit\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newLimit\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMarketMaker\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newMarketMaker\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newCanAcquire\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetMinDiscount\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newMinDiscount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetOracle\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"oracle\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetPairMaxDiscount\",\"inputs\":[{\"name\":\"tokenIn\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenOut\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newPairMaxDiscount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetPauseStatus\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"isPaused\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Skim\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"WithdrawToAcquire\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyUsedNonce\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DepositNotAllowed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExpiredSwap\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientAllocation\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAccount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAccountBeacon\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCollateralOut\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDiscount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidLimit\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOracle\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidRedemptionToken\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidRwaAmount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSwapRate\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTokenToRedeem\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotCurator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"Paused\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SkimFailed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ZeroAmount\",\"inputs\":[]}]",
}

// InstantRedemptionAdapterABI is the input ABI used to generate the binding from.
// Deprecated: Use InstantRedemptionAdapterMetaData.ABI instead.
var InstantRedemptionAdapterABI = InstantRedemptionAdapterMetaData.ABI

// InstantRedemptionAdapter is an auto generated Go binding around an Ethereum contract.
type InstantRedemptionAdapter struct {
	InstantRedemptionAdapterCaller     // Read-only binding to the contract
	InstantRedemptionAdapterTransactor // Write-only binding to the contract
	InstantRedemptionAdapterFilterer   // Log filterer for contract events
}

// InstantRedemptionAdapterCaller is an auto generated read-only Go binding around an Ethereum contract.
type InstantRedemptionAdapterCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InstantRedemptionAdapterTransactor is an auto generated write-only Go binding around an Ethereum contract.
type InstantRedemptionAdapterTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InstantRedemptionAdapterFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type InstantRedemptionAdapterFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InstantRedemptionAdapterSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type InstantRedemptionAdapterSession struct {
	Contract     *InstantRedemptionAdapter // Generic contract binding to set the session for
	CallOpts     bind.CallOpts             // Call options to use throughout this session
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// InstantRedemptionAdapterCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type InstantRedemptionAdapterCallerSession struct {
	Contract *InstantRedemptionAdapterCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                   // Call options to use throughout this session
}

// InstantRedemptionAdapterTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type InstantRedemptionAdapterTransactorSession struct {
	Contract     *InstantRedemptionAdapterTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                   // Transaction auth options to use throughout this session
}

// InstantRedemptionAdapterRaw is an auto generated low-level Go binding around an Ethereum contract.
type InstantRedemptionAdapterRaw struct {
	Contract *InstantRedemptionAdapter // Generic contract binding to access the raw methods on
}

// InstantRedemptionAdapterCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type InstantRedemptionAdapterCallerRaw struct {
	Contract *InstantRedemptionAdapterCaller // Generic read-only contract binding to access the raw methods on
}

// InstantRedemptionAdapterTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type InstantRedemptionAdapterTransactorRaw struct {
	Contract *InstantRedemptionAdapterTransactor // Generic write-only contract binding to access the raw methods on
}

// NewInstantRedemptionAdapter creates a new instance of InstantRedemptionAdapter, bound to a specific deployed contract.
func NewInstantRedemptionAdapter(address common.Address, backend bind.ContractBackend) (*InstantRedemptionAdapter, error) {
	contract, err := bindInstantRedemptionAdapter(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapter{InstantRedemptionAdapterCaller: InstantRedemptionAdapterCaller{contract: contract}, InstantRedemptionAdapterTransactor: InstantRedemptionAdapterTransactor{contract: contract}, InstantRedemptionAdapterFilterer: InstantRedemptionAdapterFilterer{contract: contract}}, nil
}

// NewInstantRedemptionAdapterCaller creates a new read-only instance of InstantRedemptionAdapter, bound to a specific deployed contract.
func NewInstantRedemptionAdapterCaller(address common.Address, caller bind.ContractCaller) (*InstantRedemptionAdapterCaller, error) {
	contract, err := bindInstantRedemptionAdapter(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterCaller{contract: contract}, nil
}

// NewInstantRedemptionAdapterTransactor creates a new write-only instance of InstantRedemptionAdapter, bound to a specific deployed contract.
func NewInstantRedemptionAdapterTransactor(address common.Address, transactor bind.ContractTransactor) (*InstantRedemptionAdapterTransactor, error) {
	contract, err := bindInstantRedemptionAdapter(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterTransactor{contract: contract}, nil
}

// NewInstantRedemptionAdapterFilterer creates a new log filterer instance of InstantRedemptionAdapter, bound to a specific deployed contract.
func NewInstantRedemptionAdapterFilterer(address common.Address, filterer bind.ContractFilterer) (*InstantRedemptionAdapterFilterer, error) {
	contract, err := bindInstantRedemptionAdapter(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterFilterer{contract: contract}, nil
}

// bindInstantRedemptionAdapter binds a generic wrapper to an already deployed contract.
func bindInstantRedemptionAdapter(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := InstantRedemptionAdapterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_InstantRedemptionAdapter *InstantRedemptionAdapterRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _InstantRedemptionAdapter.Contract.InstantRedemptionAdapterCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_InstantRedemptionAdapter *InstantRedemptionAdapterRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.InstantRedemptionAdapterTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_InstantRedemptionAdapter *InstantRedemptionAdapterRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.InstantRedemptionAdapterTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _InstantRedemptionAdapter.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.contract.Transact(opts, method, params...)
}

// PRIVATEACCOUNTBEACON is a free data retrieval call binding the contract method 0x1a71e4fc.
//
// Solidity: function PRIVATE_ACCOUNT_BEACON() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) PRIVATEACCOUNTBEACON(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "PRIVATE_ACCOUNT_BEACON")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PRIVATEACCOUNTBEACON is a free data retrieval call binding the contract method 0x1a71e4fc.
//
// Solidity: function PRIVATE_ACCOUNT_BEACON() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) PRIVATEACCOUNTBEACON() (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.PRIVATEACCOUNTBEACON(&_InstantRedemptionAdapter.CallOpts)
}

// PRIVATEACCOUNTBEACON is a free data retrieval call binding the contract method 0x1a71e4fc.
//
// Solidity: function PRIVATE_ACCOUNT_BEACON() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) PRIVATEACCOUNTBEACON() (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.PRIVATEACCOUNTBEACON(&_InstantRedemptionAdapter.CallOpts)
}

// VAULTFACTORY is a free data retrieval call binding the contract method 0x103f2907.
//
// Solidity: function VAULT_FACTORY() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) VAULTFACTORY(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "VAULT_FACTORY")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// VAULTFACTORY is a free data retrieval call binding the contract method 0x103f2907.
//
// Solidity: function VAULT_FACTORY() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) VAULTFACTORY() (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.VAULTFACTORY(&_InstantRedemptionAdapter.CallOpts)
}

// VAULTFACTORY is a free data retrieval call binding the contract method 0x103f2907.
//
// Solidity: function VAULT_FACTORY() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) VAULTFACTORY() (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.VAULTFACTORY(&_InstantRedemptionAdapter.CallOpts)
}

// AccountBeacons is a free data retrieval call binding the contract method 0xfa1fbe39.
//
// Solidity: function accountBeacons(address tokenToRedeem) view returns(address beacon)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) AccountBeacons(opts *bind.CallOpts, tokenToRedeem common.Address) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "accountBeacons", tokenToRedeem)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// AccountBeacons is a free data retrieval call binding the contract method 0xfa1fbe39.
//
// Solidity: function accountBeacons(address tokenToRedeem) view returns(address beacon)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) AccountBeacons(tokenToRedeem common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.AccountBeacons(&_InstantRedemptionAdapter.CallOpts, tokenToRedeem)
}

// AccountBeacons is a free data retrieval call binding the contract method 0xfa1fbe39.
//
// Solidity: function accountBeacons(address tokenToRedeem) view returns(address beacon)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) AccountBeacons(tokenToRedeem common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.AccountBeacons(&_InstantRedemptionAdapter.CallOpts, tokenToRedeem)
}

// Allocatable is a free data retrieval call binding the contract method 0xb7820f9c.
//
// Solidity: function allocatable(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Allocatable(opts *bind.CallOpts, vault common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "allocatable", vault)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allocatable is a free data retrieval call binding the contract method 0xb7820f9c.
//
// Solidity: function allocatable(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Allocatable(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Allocatable(&_InstantRedemptionAdapter.CallOpts, vault)
}

// Allocatable is a free data retrieval call binding the contract method 0xb7820f9c.
//
// Solidity: function allocatable(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Allocatable(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Allocatable(&_InstantRedemptionAdapter.CallOpts, vault)
}

// Allocated is a free data retrieval call binding the contract method 0x2b302cbd.
//
// Solidity: function allocated(address vault, address tokenToRedeem) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Allocated(opts *bind.CallOpts, vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "allocated", vault, tokenToRedeem)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allocated is a free data retrieval call binding the contract method 0x2b302cbd.
//
// Solidity: function allocated(address vault, address tokenToRedeem) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Allocated(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Allocated(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// Allocated is a free data retrieval call binding the contract method 0x2b302cbd.
//
// Solidity: function allocated(address vault, address tokenToRedeem) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Allocated(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Allocated(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// Converters is a free data retrieval call binding the contract method 0xe4f2494d.
//
// Solidity: function converters(address redemptionToken, address collateralToken) view returns(address converter)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Converters(opts *bind.CallOpts, redemptionToken common.Address, collateralToken common.Address) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "converters", redemptionToken, collateralToken)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Converters is a free data retrieval call binding the contract method 0xe4f2494d.
//
// Solidity: function converters(address redemptionToken, address collateralToken) view returns(address converter)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Converters(redemptionToken common.Address, collateralToken common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.Converters(&_InstantRedemptionAdapter.CallOpts, redemptionToken, collateralToken)
}

// Converters is a free data retrieval call binding the contract method 0xe4f2494d.
//
// Solidity: function converters(address redemptionToken, address collateralToken) view returns(address converter)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Converters(redemptionToken common.Address, collateralToken common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.Converters(&_InstantRedemptionAdapter.CallOpts, redemptionToken, collateralToken)
}

// CuratorAcquireBalance is a free data retrieval call binding the contract method 0x1b6714d9.
//
// Solidity: function curatorAcquireBalance(address vault) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) CuratorAcquireBalance(opts *bind.CallOpts, vault common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "curatorAcquireBalance", vault)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// CuratorAcquireBalance is a free data retrieval call binding the contract method 0x1b6714d9.
//
// Solidity: function curatorAcquireBalance(address vault) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) CuratorAcquireBalance(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.CuratorAcquireBalance(&_InstantRedemptionAdapter.CallOpts, vault)
}

// CuratorAcquireBalance is a free data retrieval call binding the contract method 0x1b6714d9.
//
// Solidity: function curatorAcquireBalance(address vault) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) CuratorAcquireBalance(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.CuratorAcquireBalance(&_InstantRedemptionAdapter.CallOpts, vault)
}

// DeallocAdapters is a free data retrieval call binding the contract method 0x12af6127.
//
// Solidity: function deallocAdapters(address vault, uint256 ) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) DeallocAdapters(opts *bind.CallOpts, vault common.Address, arg1 *big.Int) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "deallocAdapters", vault, arg1)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// DeallocAdapters is a free data retrieval call binding the contract method 0x12af6127.
//
// Solidity: function deallocAdapters(address vault, uint256 ) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) DeallocAdapters(vault common.Address, arg1 *big.Int) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.DeallocAdapters(&_InstantRedemptionAdapter.CallOpts, vault, arg1)
}

// DeallocAdapters is a free data retrieval call binding the contract method 0x12af6127.
//
// Solidity: function deallocAdapters(address vault, uint256 ) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) DeallocAdapters(vault common.Address, arg1 *big.Int) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.DeallocAdapters(&_InstantRedemptionAdapter.CallOpts, vault, arg1)
}

// Deallocatable is a free data retrieval call binding the contract method 0xc36a73ce.
//
// Solidity: function deallocatable(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Deallocatable(opts *bind.CallOpts, vault common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "deallocatable", vault)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Deallocatable is a free data retrieval call binding the contract method 0xc36a73ce.
//
// Solidity: function deallocatable(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Deallocatable(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Deallocatable(&_InstantRedemptionAdapter.CallOpts, vault)
}

// Deallocatable is a free data retrieval call binding the contract method 0xc36a73ce.
//
// Solidity: function deallocatable(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Deallocatable(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Deallocatable(&_InstantRedemptionAdapter.CallOpts, vault)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Eip712Domain(opts *bind.CallOpts) (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "eip712Domain")

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
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _InstantRedemptionAdapter.Contract.Eip712Domain(&_InstantRedemptionAdapter.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _InstantRedemptionAdapter.Contract.Eip712Domain(&_InstantRedemptionAdapter.CallOpts)
}

// GetAccount is a free data retrieval call binding the contract method 0xfd590847.
//
// Solidity: function getAccount(address vault, address tokenToRedeem) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetAccount(opts *bind.CallOpts, vault common.Address, tokenToRedeem common.Address) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getAccount", vault, tokenToRedeem)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetAccount is a free data retrieval call binding the contract method 0xfd590847.
//
// Solidity: function getAccount(address vault, address tokenToRedeem) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetAccount(vault common.Address, tokenToRedeem common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.GetAccount(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// GetAccount is a free data retrieval call binding the contract method 0xfd590847.
//
// Solidity: function getAccount(address vault, address tokenToRedeem) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetAccount(vault common.Address, tokenToRedeem common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.GetAccount(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// GetAmountOut is a free data retrieval call binding the contract method 0x4aa06652.
//
// Solidity: function getAmountOut(address tokenIn, address tokenOut, uint256 amountIn) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetAmountOut(opts *bind.CallOpts, tokenIn common.Address, tokenOut common.Address, amountIn *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getAmountOut", tokenIn, tokenOut, amountIn)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetAmountOut is a free data retrieval call binding the contract method 0x4aa06652.
//
// Solidity: function getAmountOut(address tokenIn, address tokenOut, uint256 amountIn) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetAmountOut(tokenIn common.Address, tokenOut common.Address, amountIn *big.Int) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetAmountOut(&_InstantRedemptionAdapter.CallOpts, tokenIn, tokenOut, amountIn)
}

// GetAmountOut is a free data retrieval call binding the contract method 0x4aa06652.
//
// Solidity: function getAmountOut(address tokenIn, address tokenOut, uint256 amountIn) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetAmountOut(tokenIn common.Address, tokenOut common.Address, amountIn *big.Int) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetAmountOut(&_InstantRedemptionAdapter.CallOpts, tokenIn, tokenOut, amountIn)
}

// GetCuratorAccount is a free data retrieval call binding the contract method 0x4752631c.
//
// Solidity: function getCuratorAccount(address vault) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetCuratorAccount(opts *bind.CallOpts, vault common.Address) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getCuratorAccount", vault)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetCuratorAccount is a free data retrieval call binding the contract method 0x4752631c.
//
// Solidity: function getCuratorAccount(address vault) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetCuratorAccount(vault common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.GetCuratorAccount(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GetCuratorAccount is a free data retrieval call binding the contract method 0x4752631c.
//
// Solidity: function getCuratorAccount(address vault) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetCuratorAccount(vault common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.GetCuratorAccount(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GetDeallocAdaptersLength is a free data retrieval call binding the contract method 0x65d19e54.
//
// Solidity: function getDeallocAdaptersLength(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetDeallocAdaptersLength(opts *bind.CallOpts, vault common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getDeallocAdaptersLength", vault)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDeallocAdaptersLength is a free data retrieval call binding the contract method 0x65d19e54.
//
// Solidity: function getDeallocAdaptersLength(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetDeallocAdaptersLength(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetDeallocAdaptersLength(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GetDeallocAdaptersLength is a free data retrieval call binding the contract method 0x65d19e54.
//
// Solidity: function getDeallocAdaptersLength(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetDeallocAdaptersLength(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetDeallocAdaptersLength(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GetMarketMakerAccount is a free data retrieval call binding the contract method 0xbf203cd0.
//
// Solidity: function getMarketMakerAccount(address curMarketMaker) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetMarketMakerAccount(opts *bind.CallOpts, curMarketMaker common.Address) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getMarketMakerAccount", curMarketMaker)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetMarketMakerAccount is a free data retrieval call binding the contract method 0xbf203cd0.
//
// Solidity: function getMarketMakerAccount(address curMarketMaker) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetMarketMakerAccount(curMarketMaker common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.GetMarketMakerAccount(&_InstantRedemptionAdapter.CallOpts, curMarketMaker)
}

// GetMarketMakerAccount is a free data retrieval call binding the contract method 0xbf203cd0.
//
// Solidity: function getMarketMakerAccount(address curMarketMaker) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetMarketMakerAccount(curMarketMaker common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.GetMarketMakerAccount(&_InstantRedemptionAdapter.CallOpts, curMarketMaker)
}

// GetMaxAssets is a free data retrieval call binding the contract method 0x22135549.
//
// Solidity: function getMaxAssets(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetMaxAssets(opts *bind.CallOpts, vault common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getMaxAssets", vault)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMaxAssets is a free data retrieval call binding the contract method 0x22135549.
//
// Solidity: function getMaxAssets(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetMaxAssets(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetMaxAssets(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GetMaxAssets is a free data retrieval call binding the contract method 0x22135549.
//
// Solidity: function getMaxAssets(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetMaxAssets(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetMaxAssets(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GetMaxRate is a free data retrieval call binding the contract method 0xa105e381.
//
// Solidity: function getMaxRate(address vault, address tokenToRedeem) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetMaxRate(opts *bind.CallOpts, vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getMaxRate", vault, tokenToRedeem)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMaxRate is a free data retrieval call binding the contract method 0xa105e381.
//
// Solidity: function getMaxRate(address vault, address tokenToRedeem) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetMaxRate(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetMaxRate(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// GetMaxRate is a free data retrieval call binding the contract method 0xa105e381.
//
// Solidity: function getMaxRate(address vault, address tokenToRedeem) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetMaxRate(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetMaxRate(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// GetTokensToRedeemLength is a free data retrieval call binding the contract method 0x49c58b2a.
//
// Solidity: function getTokensToRedeemLength(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GetTokensToRedeemLength(opts *bind.CallOpts, vault common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "getTokensToRedeemLength", vault)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetTokensToRedeemLength is a free data retrieval call binding the contract method 0x49c58b2a.
//
// Solidity: function getTokensToRedeemLength(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GetTokensToRedeemLength(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetTokensToRedeemLength(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GetTokensToRedeemLength is a free data retrieval call binding the contract method 0x49c58b2a.
//
// Solidity: function getTokensToRedeemLength(address vault) view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GetTokensToRedeemLength(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GetTokensToRedeemLength(&_InstantRedemptionAdapter.CallOpts, vault)
}

// GlobalAllocated is a free data retrieval call binding the contract method 0xc85771db.
//
// Solidity: function globalAllocated(address collateral) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GlobalAllocated(opts *bind.CallOpts, collateral common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "globalAllocated", collateral)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GlobalAllocated is a free data retrieval call binding the contract method 0xc85771db.
//
// Solidity: function globalAllocated(address collateral) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GlobalAllocated(collateral common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GlobalAllocated(&_InstantRedemptionAdapter.CallOpts, collateral)
}

// GlobalAllocated is a free data retrieval call binding the contract method 0xc85771db.
//
// Solidity: function globalAllocated(address collateral) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GlobalAllocated(collateral common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GlobalAllocated(&_InstantRedemptionAdapter.CallOpts, collateral)
}

// GlobalLimit is a free data retrieval call binding the contract method 0x74be68e0.
//
// Solidity: function globalLimit(address collateral) view returns(uint256 limit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GlobalLimit(opts *bind.CallOpts, collateral common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "globalLimit", collateral)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GlobalLimit is a free data retrieval call binding the contract method 0x74be68e0.
//
// Solidity: function globalLimit(address collateral) view returns(uint256 limit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GlobalLimit(collateral common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GlobalLimit(&_InstantRedemptionAdapter.CallOpts, collateral)
}

// GlobalLimit is a free data retrieval call binding the contract method 0x74be68e0.
//
// Solidity: function globalLimit(address collateral) view returns(uint256 limit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GlobalLimit(collateral common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GlobalLimit(&_InstantRedemptionAdapter.CallOpts, collateral)
}

// GlobalMaxConvertDiscount is a free data retrieval call binding the contract method 0x6f58b392.
//
// Solidity: function globalMaxConvertDiscount() view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) GlobalMaxConvertDiscount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "globalMaxConvertDiscount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GlobalMaxConvertDiscount is a free data retrieval call binding the contract method 0x6f58b392.
//
// Solidity: function globalMaxConvertDiscount() view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) GlobalMaxConvertDiscount() (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GlobalMaxConvertDiscount(&_InstantRedemptionAdapter.CallOpts)
}

// GlobalMaxConvertDiscount is a free data retrieval call binding the contract method 0x6f58b392.
//
// Solidity: function globalMaxConvertDiscount() view returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) GlobalMaxConvertDiscount() (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.GlobalMaxConvertDiscount(&_InstantRedemptionAdapter.CallOpts)
}

// IsFiller is a free data retrieval call binding the contract method 0xb0f9fe6d.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) IsFiller(opts *bind.CallOpts, marketMaker common.Address, filler common.Address) (bool, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "isFiller", marketMaker, filler)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsFiller is a free data retrieval call binding the contract method 0xb0f9fe6d.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) IsFiller(marketMaker common.Address, filler common.Address) (bool, error) {
	return _InstantRedemptionAdapter.Contract.IsFiller(&_InstantRedemptionAdapter.CallOpts, marketMaker, filler)
}

// IsFiller is a free data retrieval call binding the contract method 0xb0f9fe6d.
//
// Solidity: function isFiller(address marketMaker, address filler) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) IsFiller(marketMaker common.Address, filler common.Address) (bool, error) {
	return _InstantRedemptionAdapter.Contract.IsFiller(&_InstantRedemptionAdapter.CallOpts, marketMaker, filler)
}

// IsPaused is a free data retrieval call binding the contract method 0x5b14f183.
//
// Solidity: function isPaused(address vault) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) IsPaused(opts *bind.CallOpts, vault common.Address) (bool, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "isPaused", vault)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsPaused is a free data retrieval call binding the contract method 0x5b14f183.
//
// Solidity: function isPaused(address vault) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) IsPaused(vault common.Address) (bool, error) {
	return _InstantRedemptionAdapter.Contract.IsPaused(&_InstantRedemptionAdapter.CallOpts, vault)
}

// IsPaused is a free data retrieval call binding the contract method 0x5b14f183.
//
// Solidity: function isPaused(address vault) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) IsPaused(vault common.Address) (bool, error) {
	return _InstantRedemptionAdapter.Contract.IsPaused(&_InstantRedemptionAdapter.CallOpts, vault)
}

// IsUsedNonce is a free data retrieval call binding the contract method 0x8ec49685.
//
// Solidity: function isUsedNonce(address vault, address tokenToRedeem, uint256 nonce) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) IsUsedNonce(opts *bind.CallOpts, vault common.Address, tokenToRedeem common.Address, nonce *big.Int) (bool, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "isUsedNonce", vault, tokenToRedeem, nonce)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsUsedNonce is a free data retrieval call binding the contract method 0x8ec49685.
//
// Solidity: function isUsedNonce(address vault, address tokenToRedeem, uint256 nonce) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) IsUsedNonce(vault common.Address, tokenToRedeem common.Address, nonce *big.Int) (bool, error) {
	return _InstantRedemptionAdapter.Contract.IsUsedNonce(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem, nonce)
}

// IsUsedNonce is a free data retrieval call binding the contract method 0x8ec49685.
//
// Solidity: function isUsedNonce(address vault, address tokenToRedeem, uint256 nonce) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) IsUsedNonce(vault common.Address, tokenToRedeem common.Address, nonce *big.Int) (bool, error) {
	return _InstantRedemptionAdapter.Contract.IsUsedNonce(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem, nonce)
}

// Limit is a free data retrieval call binding the contract method 0x0b95c2f6.
//
// Solidity: function limit(address vault, address tokenToRedeem) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Limit(opts *bind.CallOpts, vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "limit", vault, tokenToRedeem)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Limit is a free data retrieval call binding the contract method 0x0b95c2f6.
//
// Solidity: function limit(address vault, address tokenToRedeem) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Limit(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Limit(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// Limit is a free data retrieval call binding the contract method 0x0b95c2f6.
//
// Solidity: function limit(address vault, address tokenToRedeem) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Limit(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Limit(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// MarketMaker is a free data retrieval call binding the contract method 0x36fe51ef.
//
// Solidity: function marketMaker(address vault) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) MarketMaker(opts *bind.CallOpts, vault common.Address) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "marketMaker", vault)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// MarketMaker is a free data retrieval call binding the contract method 0x36fe51ef.
//
// Solidity: function marketMaker(address vault) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) MarketMaker(vault common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.MarketMaker(&_InstantRedemptionAdapter.CallOpts, vault)
}

// MarketMaker is a free data retrieval call binding the contract method 0x36fe51ef.
//
// Solidity: function marketMaker(address vault) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) MarketMaker(vault common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.MarketMaker(&_InstantRedemptionAdapter.CallOpts, vault)
}

// MarketMakerAcquireBalances is a free data retrieval call binding the contract method 0x46c9f159.
//
// Solidity: function marketMakerAcquireBalances(address vault, address marketMaker) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) MarketMakerAcquireBalances(opts *bind.CallOpts, vault common.Address, marketMaker common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "marketMakerAcquireBalances", vault, marketMaker)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MarketMakerAcquireBalances is a free data retrieval call binding the contract method 0x46c9f159.
//
// Solidity: function marketMakerAcquireBalances(address vault, address marketMaker) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) MarketMakerAcquireBalances(vault common.Address, marketMaker common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.MarketMakerAcquireBalances(&_InstantRedemptionAdapter.CallOpts, vault, marketMaker)
}

// MarketMakerAcquireBalances is a free data retrieval call binding the contract method 0x46c9f159.
//
// Solidity: function marketMakerAcquireBalances(address vault, address marketMaker) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) MarketMakerAcquireBalances(vault common.Address, marketMaker common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.MarketMakerAcquireBalances(&_InstantRedemptionAdapter.CallOpts, vault, marketMaker)
}

// MarketMakerCanAcquire is a free data retrieval call binding the contract method 0xfdc21a77.
//
// Solidity: function marketMakerCanAcquire(address vault) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) MarketMakerCanAcquire(opts *bind.CallOpts, vault common.Address) (bool, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "marketMakerCanAcquire", vault)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// MarketMakerCanAcquire is a free data retrieval call binding the contract method 0xfdc21a77.
//
// Solidity: function marketMakerCanAcquire(address vault) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) MarketMakerCanAcquire(vault common.Address) (bool, error) {
	return _InstantRedemptionAdapter.Contract.MarketMakerCanAcquire(&_InstantRedemptionAdapter.CallOpts, vault)
}

// MarketMakerCanAcquire is a free data retrieval call binding the contract method 0xfdc21a77.
//
// Solidity: function marketMakerCanAcquire(address vault) view returns(bool)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) MarketMakerCanAcquire(vault common.Address) (bool, error) {
	return _InstantRedemptionAdapter.Contract.MarketMakerCanAcquire(&_InstantRedemptionAdapter.CallOpts, vault)
}

// MinDiscount is a free data retrieval call binding the contract method 0x286cd4e5.
//
// Solidity: function minDiscount(address vault, address tokenToRedeem) view returns(uint256 ppm)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) MinDiscount(opts *bind.CallOpts, vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "minDiscount", vault, tokenToRedeem)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MinDiscount is a free data retrieval call binding the contract method 0x286cd4e5.
//
// Solidity: function minDiscount(address vault, address tokenToRedeem) view returns(uint256 ppm)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) MinDiscount(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.MinDiscount(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// MinDiscount is a free data retrieval call binding the contract method 0x286cd4e5.
//
// Solidity: function minDiscount(address vault, address tokenToRedeem) view returns(uint256 ppm)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) MinDiscount(vault common.Address, tokenToRedeem common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.MinDiscount(&_InstantRedemptionAdapter.CallOpts, vault, tokenToRedeem)
}

// Oracles is a free data retrieval call binding the contract method 0xaddd5099.
//
// Solidity: function oracles(address token) view returns(address oracle)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Oracles(opts *bind.CallOpts, token common.Address) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "oracles", token)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Oracles is a free data retrieval call binding the contract method 0xaddd5099.
//
// Solidity: function oracles(address token) view returns(address oracle)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Oracles(token common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.Oracles(&_InstantRedemptionAdapter.CallOpts, token)
}

// Oracles is a free data retrieval call binding the contract method 0xaddd5099.
//
// Solidity: function oracles(address token) view returns(address oracle)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Oracles(token common.Address) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.Oracles(&_InstantRedemptionAdapter.CallOpts, token)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Owner() (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.Owner(&_InstantRedemptionAdapter.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Owner() (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.Owner(&_InstantRedemptionAdapter.CallOpts)
}

// PairMaxConvertDiscount is a free data retrieval call binding the contract method 0x04734add.
//
// Solidity: function pairMaxConvertDiscount(address tokenIn, address tokenOut) view returns(uint256 ppm)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) PairMaxConvertDiscount(opts *bind.CallOpts, tokenIn common.Address, tokenOut common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "pairMaxConvertDiscount", tokenIn, tokenOut)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// PairMaxConvertDiscount is a free data retrieval call binding the contract method 0x04734add.
//
// Solidity: function pairMaxConvertDiscount(address tokenIn, address tokenOut) view returns(uint256 ppm)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) PairMaxConvertDiscount(tokenIn common.Address, tokenOut common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.PairMaxConvertDiscount(&_InstantRedemptionAdapter.CallOpts, tokenIn, tokenOut)
}

// PairMaxConvertDiscount is a free data retrieval call binding the contract method 0x04734add.
//
// Solidity: function pairMaxConvertDiscount(address tokenIn, address tokenOut) view returns(uint256 ppm)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) PairMaxConvertDiscount(tokenIn common.Address, tokenOut common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.PairMaxConvertDiscount(&_InstantRedemptionAdapter.CallOpts, tokenIn, tokenOut)
}

// Skimmable is a free data retrieval call binding the contract method 0x62e6ba28.
//
// Solidity: function skimmable(address vault) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) Skimmable(opts *bind.CallOpts, vault common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "skimmable", vault)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Skimmable is a free data retrieval call binding the contract method 0x62e6ba28.
//
// Solidity: function skimmable(address vault) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Skimmable(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Skimmable(&_InstantRedemptionAdapter.CallOpts, vault)
}

// Skimmable is a free data retrieval call binding the contract method 0x62e6ba28.
//
// Solidity: function skimmable(address vault) view returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) Skimmable(vault common.Address) (*big.Int, error) {
	return _InstantRedemptionAdapter.Contract.Skimmable(&_InstantRedemptionAdapter.CallOpts, vault)
}

// TokensToRedeem is a free data retrieval call binding the contract method 0x5eb91a67.
//
// Solidity: function tokensToRedeem(address vault, uint256 ) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCaller) TokensToRedeem(opts *bind.CallOpts, vault common.Address, arg1 *big.Int) (common.Address, error) {
	var out []interface{}
	err := _InstantRedemptionAdapter.contract.Call(opts, &out, "tokensToRedeem", vault, arg1)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// TokensToRedeem is a free data retrieval call binding the contract method 0x5eb91a67.
//
// Solidity: function tokensToRedeem(address vault, uint256 ) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) TokensToRedeem(vault common.Address, arg1 *big.Int) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.TokensToRedeem(&_InstantRedemptionAdapter.CallOpts, vault, arg1)
}

// TokensToRedeem is a free data retrieval call binding the contract method 0x5eb91a67.
//
// Solidity: function tokensToRedeem(address vault, uint256 ) view returns(address)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterCallerSession) TokensToRedeem(vault common.Address, arg1 *big.Int) (common.Address, error) {
	return _InstantRedemptionAdapter.Contract.TokensToRedeem(&_InstantRedemptionAdapter.CallOpts, vault, arg1)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Allocate(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "allocate", amount)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Allocate(amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Allocate(&_InstantRedemptionAdapter.TransactOpts, amount)
}

// Allocate is a paid mutator transaction binding the contract method 0x90ca796b.
//
// Solidity: function allocate(uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Allocate(amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Allocate(&_InstantRedemptionAdapter.TransactOpts, amount)
}

// ClaimCuratorAcquired is a paid mutator transaction binding the contract method 0x3269fbf4.
//
// Solidity: function claimCuratorAcquired(address recipient, address vault, address tokenToRedeem, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) ClaimCuratorAcquired(opts *bind.TransactOpts, recipient common.Address, vault common.Address, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "claimCuratorAcquired", recipient, vault, tokenToRedeem, amount)
}

// ClaimCuratorAcquired is a paid mutator transaction binding the contract method 0x3269fbf4.
//
// Solidity: function claimCuratorAcquired(address recipient, address vault, address tokenToRedeem, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) ClaimCuratorAcquired(recipient common.Address, vault common.Address, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.ClaimCuratorAcquired(&_InstantRedemptionAdapter.TransactOpts, recipient, vault, tokenToRedeem, amount)
}

// ClaimCuratorAcquired is a paid mutator transaction binding the contract method 0x3269fbf4.
//
// Solidity: function claimCuratorAcquired(address recipient, address vault, address tokenToRedeem, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) ClaimCuratorAcquired(recipient common.Address, vault common.Address, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.ClaimCuratorAcquired(&_InstantRedemptionAdapter.TransactOpts, recipient, vault, tokenToRedeem, amount)
}

// ClaimMarketMakerAcquired is a paid mutator transaction binding the contract method 0x750fe113.
//
// Solidity: function claimMarketMakerAcquired(address recipient, address tokenToRedeem, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) ClaimMarketMakerAcquired(opts *bind.TransactOpts, recipient common.Address, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "claimMarketMakerAcquired", recipient, tokenToRedeem, amount)
}

// ClaimMarketMakerAcquired is a paid mutator transaction binding the contract method 0x750fe113.
//
// Solidity: function claimMarketMakerAcquired(address recipient, address tokenToRedeem, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) ClaimMarketMakerAcquired(recipient common.Address, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.ClaimMarketMakerAcquired(&_InstantRedemptionAdapter.TransactOpts, recipient, tokenToRedeem, amount)
}

// ClaimMarketMakerAcquired is a paid mutator transaction binding the contract method 0x750fe113.
//
// Solidity: function claimMarketMakerAcquired(address recipient, address tokenToRedeem, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) ClaimMarketMakerAcquired(recipient common.Address, tokenToRedeem common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.ClaimMarketMakerAcquired(&_InstantRedemptionAdapter.TransactOpts, recipient, tokenToRedeem, amount)
}

// ConvertRedemption is a paid mutator transaction binding the contract method 0xa75b282f.
//
// Solidity: function convertRedemption(address vault, address tokenToRedeem, address redemptionToken, uint256 redemptionAmount, bytes data) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) ConvertRedemption(opts *bind.TransactOpts, vault common.Address, tokenToRedeem common.Address, redemptionToken common.Address, redemptionAmount *big.Int, data []byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "convertRedemption", vault, tokenToRedeem, redemptionToken, redemptionAmount, data)
}

// ConvertRedemption is a paid mutator transaction binding the contract method 0xa75b282f.
//
// Solidity: function convertRedemption(address vault, address tokenToRedeem, address redemptionToken, uint256 redemptionAmount, bytes data) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) ConvertRedemption(vault common.Address, tokenToRedeem common.Address, redemptionToken common.Address, redemptionAmount *big.Int, data []byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.ConvertRedemption(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, redemptionToken, redemptionAmount, data)
}

// ConvertRedemption is a paid mutator transaction binding the contract method 0xa75b282f.
//
// Solidity: function convertRedemption(address vault, address tokenToRedeem, address redemptionToken, uint256 redemptionAmount, bytes data) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) ConvertRedemption(vault common.Address, tokenToRedeem common.Address, redemptionToken common.Address, redemptionAmount *big.Int, data []byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.ConvertRedemption(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, redemptionToken, redemptionAmount, data)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Deallocate(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "deallocate", amount)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Deallocate(amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Deallocate(&_InstantRedemptionAdapter.TransactOpts, amount)
}

// Deallocate is a paid mutator transaction binding the contract method 0x6f6c441f.
//
// Solidity: function deallocate(uint256 amount) returns(uint256)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Deallocate(amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Deallocate(&_InstantRedemptionAdapter.TransactOpts, amount)
}

// DepositToAcquire is a paid mutator transaction binding the contract method 0xd6c83d24.
//
// Solidity: function depositToAcquire(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) DepositToAcquire(opts *bind.TransactOpts, vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "depositToAcquire", vault, amount)
}

// DepositToAcquire is a paid mutator transaction binding the contract method 0xd6c83d24.
//
// Solidity: function depositToAcquire(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) DepositToAcquire(vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.DepositToAcquire(&_InstantRedemptionAdapter.TransactOpts, vault, amount)
}

// DepositToAcquire is a paid mutator transaction binding the contract method 0xd6c83d24.
//
// Solidity: function depositToAcquire(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) DepositToAcquire(vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.DepositToAcquire(&_InstantRedemptionAdapter.TransactOpts, vault, amount)
}

// Initialize is a paid mutator transaction binding the contract method 0x8129fc1c.
//
// Solidity: function initialize() returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Initialize(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "initialize")
}

// Initialize is a paid mutator transaction binding the contract method 0x8129fc1c.
//
// Solidity: function initialize() returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Initialize() (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Initialize(&_InstantRedemptionAdapter.TransactOpts)
}

// Initialize is a paid mutator transaction binding the contract method 0x8129fc1c.
//
// Solidity: function initialize() returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Initialize() (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Initialize(&_InstantRedemptionAdapter.TransactOpts)
}

// Initialize0 is a paid mutator transaction binding the contract method 0xc4d66de8.
//
// Solidity: function initialize(address newOwner) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Initialize0(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "initialize0", newOwner)
}

// Initialize0 is a paid mutator transaction binding the contract method 0xc4d66de8.
//
// Solidity: function initialize(address newOwner) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Initialize0(newOwner common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Initialize0(&_InstantRedemptionAdapter.TransactOpts, newOwner)
}

// Initialize0 is a paid mutator transaction binding the contract method 0xc4d66de8.
//
// Solidity: function initialize(address newOwner) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Initialize0(newOwner common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Initialize0(&_InstantRedemptionAdapter.TransactOpts, newOwner)
}

// InvalidateNonce is a paid mutator transaction binding the contract method 0xddebd73e.
//
// Solidity: function invalidateNonce(address vault, address tokenToRedeem, uint256 nonce) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) InvalidateNonce(opts *bind.TransactOpts, vault common.Address, tokenToRedeem common.Address, nonce *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "invalidateNonce", vault, tokenToRedeem, nonce)
}

// InvalidateNonce is a paid mutator transaction binding the contract method 0xddebd73e.
//
// Solidity: function invalidateNonce(address vault, address tokenToRedeem, uint256 nonce) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) InvalidateNonce(vault common.Address, tokenToRedeem common.Address, nonce *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.InvalidateNonce(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, nonce)
}

// InvalidateNonce is a paid mutator transaction binding the contract method 0xddebd73e.
//
// Solidity: function invalidateNonce(address vault, address tokenToRedeem, uint256 nonce) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) InvalidateNonce(vault common.Address, tokenToRedeem common.Address, nonce *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.InvalidateNonce(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, nonce)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Multicall(opts *bind.TransactOpts, data [][]byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "multicall", data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Multicall(&_InstantRedemptionAdapter.TransactOpts, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Multicall(&_InstantRedemptionAdapter.TransactOpts, data)
}

// Recover is a paid mutator transaction binding the contract method 0x5705ae43.
//
// Solidity: function recover(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Recover(opts *bind.TransactOpts, vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "recover", vault, amount)
}

// Recover is a paid mutator transaction binding the contract method 0x5705ae43.
//
// Solidity: function recover(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Recover(vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Recover(&_InstantRedemptionAdapter.TransactOpts, vault, amount)
}

// Recover is a paid mutator transaction binding the contract method 0x5705ae43.
//
// Solidity: function recover(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Recover(vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Recover(&_InstantRedemptionAdapter.TransactOpts, vault, amount)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) RenounceOwnership() (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.RenounceOwnership(&_InstantRedemptionAdapter.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.RenounceOwnership(&_InstantRedemptionAdapter.TransactOpts)
}

// SetAccountBeacon is a paid mutator transaction binding the contract method 0x2ee8ce3b.
//
// Solidity: function setAccountBeacon(address tokenToRedeem, address beacon) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetAccountBeacon(opts *bind.TransactOpts, tokenToRedeem common.Address, beacon common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setAccountBeacon", tokenToRedeem, beacon)
}

// SetAccountBeacon is a paid mutator transaction binding the contract method 0x2ee8ce3b.
//
// Solidity: function setAccountBeacon(address tokenToRedeem, address beacon) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetAccountBeacon(tokenToRedeem common.Address, beacon common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetAccountBeacon(&_InstantRedemptionAdapter.TransactOpts, tokenToRedeem, beacon)
}

// SetAccountBeacon is a paid mutator transaction binding the contract method 0x2ee8ce3b.
//
// Solidity: function setAccountBeacon(address tokenToRedeem, address beacon) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetAccountBeacon(tokenToRedeem common.Address, beacon common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetAccountBeacon(&_InstantRedemptionAdapter.TransactOpts, tokenToRedeem, beacon)
}

// SetConversionAdapter is a paid mutator transaction binding the contract method 0x435160b5.
//
// Solidity: function setConversionAdapter(address redemptionToken, address collateralToken, address conversionAdapter) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetConversionAdapter(opts *bind.TransactOpts, redemptionToken common.Address, collateralToken common.Address, conversionAdapter common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setConversionAdapter", redemptionToken, collateralToken, conversionAdapter)
}

// SetConversionAdapter is a paid mutator transaction binding the contract method 0x435160b5.
//
// Solidity: function setConversionAdapter(address redemptionToken, address collateralToken, address conversionAdapter) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetConversionAdapter(redemptionToken common.Address, collateralToken common.Address, conversionAdapter common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetConversionAdapter(&_InstantRedemptionAdapter.TransactOpts, redemptionToken, collateralToken, conversionAdapter)
}

// SetConversionAdapter is a paid mutator transaction binding the contract method 0x435160b5.
//
// Solidity: function setConversionAdapter(address redemptionToken, address collateralToken, address conversionAdapter) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetConversionAdapter(redemptionToken common.Address, collateralToken common.Address, conversionAdapter common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetConversionAdapter(&_InstantRedemptionAdapter.TransactOpts, redemptionToken, collateralToken, conversionAdapter)
}

// SetDeallocAdapters is a paid mutator transaction binding the contract method 0xe860d156.
//
// Solidity: function setDeallocAdapters(address vault, address[] newDeallocAdapters) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetDeallocAdapters(opts *bind.TransactOpts, vault common.Address, newDeallocAdapters []common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setDeallocAdapters", vault, newDeallocAdapters)
}

// SetDeallocAdapters is a paid mutator transaction binding the contract method 0xe860d156.
//
// Solidity: function setDeallocAdapters(address vault, address[] newDeallocAdapters) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetDeallocAdapters(vault common.Address, newDeallocAdapters []common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetDeallocAdapters(&_InstantRedemptionAdapter.TransactOpts, vault, newDeallocAdapters)
}

// SetDeallocAdapters is a paid mutator transaction binding the contract method 0xe860d156.
//
// Solidity: function setDeallocAdapters(address vault, address[] newDeallocAdapters) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetDeallocAdapters(vault common.Address, newDeallocAdapters []common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetDeallocAdapters(&_InstantRedemptionAdapter.TransactOpts, vault, newDeallocAdapters)
}

// SetFiller is a paid mutator transaction binding the contract method 0xca1f4198.
//
// Solidity: function setFiller(address vault, address filler, bool isAuthorized) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetFiller(opts *bind.TransactOpts, vault common.Address, filler common.Address, isAuthorized bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setFiller", vault, filler, isAuthorized)
}

// SetFiller is a paid mutator transaction binding the contract method 0xca1f4198.
//
// Solidity: function setFiller(address vault, address filler, bool isAuthorized) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetFiller(vault common.Address, filler common.Address, isAuthorized bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetFiller(&_InstantRedemptionAdapter.TransactOpts, vault, filler, isAuthorized)
}

// SetFiller is a paid mutator transaction binding the contract method 0xca1f4198.
//
// Solidity: function setFiller(address vault, address filler, bool isAuthorized) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetFiller(vault common.Address, filler common.Address, isAuthorized bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetFiller(&_InstantRedemptionAdapter.TransactOpts, vault, filler, isAuthorized)
}

// SetGlobalLimit is a paid mutator transaction binding the contract method 0xa69b5109.
//
// Solidity: function setGlobalLimit(address collateral, uint256 limit) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetGlobalLimit(opts *bind.TransactOpts, collateral common.Address, limit *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setGlobalLimit", collateral, limit)
}

// SetGlobalLimit is a paid mutator transaction binding the contract method 0xa69b5109.
//
// Solidity: function setGlobalLimit(address collateral, uint256 limit) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetGlobalLimit(collateral common.Address, limit *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetGlobalLimit(&_InstantRedemptionAdapter.TransactOpts, collateral, limit)
}

// SetGlobalLimit is a paid mutator transaction binding the contract method 0xa69b5109.
//
// Solidity: function setGlobalLimit(address collateral, uint256 limit) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetGlobalLimit(collateral common.Address, limit *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetGlobalLimit(&_InstantRedemptionAdapter.TransactOpts, collateral, limit)
}

// SetGlobalMaxDiscount is a paid mutator transaction binding the contract method 0x75327233.
//
// Solidity: function setGlobalMaxDiscount(uint256 newGlobalMaxDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetGlobalMaxDiscount(opts *bind.TransactOpts, newGlobalMaxDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setGlobalMaxDiscount", newGlobalMaxDiscount)
}

// SetGlobalMaxDiscount is a paid mutator transaction binding the contract method 0x75327233.
//
// Solidity: function setGlobalMaxDiscount(uint256 newGlobalMaxDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetGlobalMaxDiscount(newGlobalMaxDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetGlobalMaxDiscount(&_InstantRedemptionAdapter.TransactOpts, newGlobalMaxDiscount)
}

// SetGlobalMaxDiscount is a paid mutator transaction binding the contract method 0x75327233.
//
// Solidity: function setGlobalMaxDiscount(uint256 newGlobalMaxDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetGlobalMaxDiscount(newGlobalMaxDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetGlobalMaxDiscount(&_InstantRedemptionAdapter.TransactOpts, newGlobalMaxDiscount)
}

// SetLimit is a paid mutator transaction binding the contract method 0x21d4e316.
//
// Solidity: function setLimit(address vault, address tokenToRedeem, uint256 newLimit) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetLimit(opts *bind.TransactOpts, vault common.Address, tokenToRedeem common.Address, newLimit *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setLimit", vault, tokenToRedeem, newLimit)
}

// SetLimit is a paid mutator transaction binding the contract method 0x21d4e316.
//
// Solidity: function setLimit(address vault, address tokenToRedeem, uint256 newLimit) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetLimit(vault common.Address, tokenToRedeem common.Address, newLimit *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetLimit(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, newLimit)
}

// SetLimit is a paid mutator transaction binding the contract method 0x21d4e316.
//
// Solidity: function setLimit(address vault, address tokenToRedeem, uint256 newLimit) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetLimit(vault common.Address, tokenToRedeem common.Address, newLimit *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetLimit(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, newLimit)
}

// SetMakerMaker is a paid mutator transaction binding the contract method 0x506147ba.
//
// Solidity: function setMakerMaker(address vault, address newMarketMaker, bool newCanAcquire) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetMakerMaker(opts *bind.TransactOpts, vault common.Address, newMarketMaker common.Address, newCanAcquire bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setMakerMaker", vault, newMarketMaker, newCanAcquire)
}

// SetMakerMaker is a paid mutator transaction binding the contract method 0x506147ba.
//
// Solidity: function setMakerMaker(address vault, address newMarketMaker, bool newCanAcquire) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetMakerMaker(vault common.Address, newMarketMaker common.Address, newCanAcquire bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetMakerMaker(&_InstantRedemptionAdapter.TransactOpts, vault, newMarketMaker, newCanAcquire)
}

// SetMakerMaker is a paid mutator transaction binding the contract method 0x506147ba.
//
// Solidity: function setMakerMaker(address vault, address newMarketMaker, bool newCanAcquire) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetMakerMaker(vault common.Address, newMarketMaker common.Address, newCanAcquire bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetMakerMaker(&_InstantRedemptionAdapter.TransactOpts, vault, newMarketMaker, newCanAcquire)
}

// SetMinDiscount is a paid mutator transaction binding the contract method 0x5298e3c6.
//
// Solidity: function setMinDiscount(address vault, address tokenToRedeem, uint256 newMinDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetMinDiscount(opts *bind.TransactOpts, vault common.Address, tokenToRedeem common.Address, newMinDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setMinDiscount", vault, tokenToRedeem, newMinDiscount)
}

// SetMinDiscount is a paid mutator transaction binding the contract method 0x5298e3c6.
//
// Solidity: function setMinDiscount(address vault, address tokenToRedeem, uint256 newMinDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetMinDiscount(vault common.Address, tokenToRedeem common.Address, newMinDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetMinDiscount(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, newMinDiscount)
}

// SetMinDiscount is a paid mutator transaction binding the contract method 0x5298e3c6.
//
// Solidity: function setMinDiscount(address vault, address tokenToRedeem, uint256 newMinDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetMinDiscount(vault common.Address, tokenToRedeem common.Address, newMinDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetMinDiscount(&_InstantRedemptionAdapter.TransactOpts, vault, tokenToRedeem, newMinDiscount)
}

// SetOracle is a paid mutator transaction binding the contract method 0x5c38eb3a.
//
// Solidity: function setOracle(address token, address oracle) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetOracle(opts *bind.TransactOpts, token common.Address, oracle common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setOracle", token, oracle)
}

// SetOracle is a paid mutator transaction binding the contract method 0x5c38eb3a.
//
// Solidity: function setOracle(address token, address oracle) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetOracle(token common.Address, oracle common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetOracle(&_InstantRedemptionAdapter.TransactOpts, token, oracle)
}

// SetOracle is a paid mutator transaction binding the contract method 0x5c38eb3a.
//
// Solidity: function setOracle(address token, address oracle) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetOracle(token common.Address, oracle common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetOracle(&_InstantRedemptionAdapter.TransactOpts, token, oracle)
}

// SetPairMaxDiscount is a paid mutator transaction binding the contract method 0x0dbd27ac.
//
// Solidity: function setPairMaxDiscount(address tokenIn, address tokenOut, uint256 newPairMaxDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetPairMaxDiscount(opts *bind.TransactOpts, tokenIn common.Address, tokenOut common.Address, newPairMaxDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setPairMaxDiscount", tokenIn, tokenOut, newPairMaxDiscount)
}

// SetPairMaxDiscount is a paid mutator transaction binding the contract method 0x0dbd27ac.
//
// Solidity: function setPairMaxDiscount(address tokenIn, address tokenOut, uint256 newPairMaxDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetPairMaxDiscount(tokenIn common.Address, tokenOut common.Address, newPairMaxDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetPairMaxDiscount(&_InstantRedemptionAdapter.TransactOpts, tokenIn, tokenOut, newPairMaxDiscount)
}

// SetPairMaxDiscount is a paid mutator transaction binding the contract method 0x0dbd27ac.
//
// Solidity: function setPairMaxDiscount(address tokenIn, address tokenOut, uint256 newPairMaxDiscount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetPairMaxDiscount(tokenIn common.Address, tokenOut common.Address, newPairMaxDiscount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetPairMaxDiscount(&_InstantRedemptionAdapter.TransactOpts, tokenIn, tokenOut, newPairMaxDiscount)
}

// SetPauseStatus is a paid mutator transaction binding the contract method 0x582d44bb.
//
// Solidity: function setPauseStatus(address vault, bool newPauseStatus) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) SetPauseStatus(opts *bind.TransactOpts, vault common.Address, newPauseStatus bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "setPauseStatus", vault, newPauseStatus)
}

// SetPauseStatus is a paid mutator transaction binding the contract method 0x582d44bb.
//
// Solidity: function setPauseStatus(address vault, bool newPauseStatus) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) SetPauseStatus(vault common.Address, newPauseStatus bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetPauseStatus(&_InstantRedemptionAdapter.TransactOpts, vault, newPauseStatus)
}

// SetPauseStatus is a paid mutator transaction binding the contract method 0x582d44bb.
//
// Solidity: function setPauseStatus(address vault, bool newPauseStatus) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) SetPauseStatus(vault common.Address, newPauseStatus bool) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.SetPauseStatus(&_InstantRedemptionAdapter.TransactOpts, vault, newPauseStatus)
}

// Skim is a paid mutator transaction binding the contract method 0xbc25cf77.
//
// Solidity: function skim(address vault) returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Skim(opts *bind.TransactOpts, vault common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "skim", vault)
}

// Skim is a paid mutator transaction binding the contract method 0xbc25cf77.
//
// Solidity: function skim(address vault) returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Skim(vault common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Skim(&_InstantRedemptionAdapter.TransactOpts, vault)
}

// Skim is a paid mutator transaction binding the contract method 0xbc25cf77.
//
// Solidity: function skim(address vault) returns(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Skim(vault common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Skim(&_InstantRedemptionAdapter.TransactOpts, vault)
}

// Swap is a paid mutator transaction binding the contract method 0x2f6e568b.
//
// Solidity: function swap(((address,address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn, uint256 amountOut) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Swap(opts *bind.TransactOpts, discountSwap IInstantRedemptionAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int, amountOut *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "swap", discountSwap, protocolSignature, recipient, amountIn, amountOut)
}

// Swap is a paid mutator transaction binding the contract method 0x2f6e568b.
//
// Solidity: function swap(((address,address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn, uint256 amountOut) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Swap(discountSwap IInstantRedemptionAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int, amountOut *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Swap(&_InstantRedemptionAdapter.TransactOpts, discountSwap, protocolSignature, recipient, amountIn, amountOut)
}

// Swap is a paid mutator transaction binding the contract method 0x2f6e568b.
//
// Solidity: function swap(((address,address,uint256,address,address,uint256,uint48),bytes,uint48) discountSwap, bytes protocolSignature, address recipient, uint256 amountIn, uint256 amountOut) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Swap(discountSwap IInstantRedemptionAdapterDiscountSwap, protocolSignature []byte, recipient common.Address, amountIn *big.Int, amountOut *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Swap(&_InstantRedemptionAdapter.TransactOpts, discountSwap, protocolSignature, recipient, amountIn, amountOut)
}

// Swap0 is a paid mutator transaction binding the contract method 0x91ec43b1.
//
// Solidity: function swap((address,address,address,uint256,uint256) swap) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Swap0(opts *bind.TransactOpts, swap IInstantRedemptionAdapterSwap) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "swap0", swap)
}

// Swap0 is a paid mutator transaction binding the contract method 0x91ec43b1.
//
// Solidity: function swap((address,address,address,uint256,uint256) swap) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Swap0(swap IInstantRedemptionAdapterSwap) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Swap0(&_InstantRedemptionAdapter.TransactOpts, swap)
}

// Swap0 is a paid mutator transaction binding the contract method 0x91ec43b1.
//
// Solidity: function swap((address,address,address,uint256,uint256) swap) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Swap0(swap IInstantRedemptionAdapterSwap) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Swap0(&_InstantRedemptionAdapter.TransactOpts, swap)
}

// Swap1 is a paid mutator transaction binding the contract method 0xc111fdb1.
//
// Solidity: function swap((address,address,address,uint256,uint256,address,address,uint256,uint256) signedSwap, bytes signature) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) Swap1(opts *bind.TransactOpts, signedSwap IInstantRedemptionAdapterSignedSwap, signature []byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "swap1", signedSwap, signature)
}

// Swap1 is a paid mutator transaction binding the contract method 0xc111fdb1.
//
// Solidity: function swap((address,address,address,uint256,uint256,address,address,uint256,uint256) signedSwap, bytes signature) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) Swap1(signedSwap IInstantRedemptionAdapterSignedSwap, signature []byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Swap1(&_InstantRedemptionAdapter.TransactOpts, signedSwap, signature)
}

// Swap1 is a paid mutator transaction binding the contract method 0xc111fdb1.
//
// Solidity: function swap((address,address,address,uint256,uint256,address,address,uint256,uint256) signedSwap, bytes signature) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) Swap1(signedSwap IInstantRedemptionAdapterSignedSwap, signature []byte) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.Swap1(&_InstantRedemptionAdapter.TransactOpts, signedSwap, signature)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.TransferOwnership(&_InstantRedemptionAdapter.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.TransferOwnership(&_InstantRedemptionAdapter.TransactOpts, newOwner)
}

// WithdrawToAcquire is a paid mutator transaction binding the contract method 0x00812e34.
//
// Solidity: function withdrawToAcquire(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactor) WithdrawToAcquire(opts *bind.TransactOpts, vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.contract.Transact(opts, "withdrawToAcquire", vault, amount)
}

// WithdrawToAcquire is a paid mutator transaction binding the contract method 0x00812e34.
//
// Solidity: function withdrawToAcquire(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterSession) WithdrawToAcquire(vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.WithdrawToAcquire(&_InstantRedemptionAdapter.TransactOpts, vault, amount)
}

// WithdrawToAcquire is a paid mutator transaction binding the contract method 0x00812e34.
//
// Solidity: function withdrawToAcquire(address vault, uint256 amount) returns()
func (_InstantRedemptionAdapter *InstantRedemptionAdapterTransactorSession) WithdrawToAcquire(vault common.Address, amount *big.Int) (*types.Transaction, error) {
	return _InstantRedemptionAdapter.Contract.WithdrawToAcquire(&_InstantRedemptionAdapter.TransactOpts, vault, amount)
}

// InstantRedemptionAdapterAllocateIterator is returned from FilterAllocate and is used to iterate over the raw logs and unpacked data for Allocate events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterAllocateIterator struct {
	Event *InstantRedemptionAdapterAllocate // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterAllocateIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterAllocate)
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
		it.Event = new(InstantRedemptionAdapterAllocate)
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
func (it *InstantRedemptionAdapterAllocateIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterAllocateIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterAllocate represents a Allocate event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterAllocate struct {
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterAllocate is a free log retrieval operation binding the contract event 0xc6fd06c7e8e9efd7340ccdd848bd29fa0c943f01dfa356b717147ee09037c9fb.
//
// Solidity: event Allocate(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterAllocate(opts *bind.FilterOpts) (*InstantRedemptionAdapterAllocateIterator, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "Allocate")
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterAllocateIterator{contract: _InstantRedemptionAdapter.contract, event: "Allocate", logs: logs, sub: sub}, nil
}

// WatchAllocate is a free log subscription operation binding the contract event 0xc6fd06c7e8e9efd7340ccdd848bd29fa0c943f01dfa356b717147ee09037c9fb.
//
// Solidity: event Allocate(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchAllocate(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterAllocate) (event.Subscription, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "Allocate")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterAllocate)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Allocate", log); err != nil {
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

// ParseAllocate is a log parse operation binding the contract event 0xc6fd06c7e8e9efd7340ccdd848bd29fa0c943f01dfa356b717147ee09037c9fb.
//
// Solidity: event Allocate(uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseAllocate(log types.Log) (*InstantRedemptionAdapterAllocate, error) {
	event := new(InstantRedemptionAdapterAllocate)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Allocate", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterClaimCuratorAcquiredIterator is returned from FilterClaimCuratorAcquired and is used to iterate over the raw logs and unpacked data for ClaimCuratorAcquired events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterClaimCuratorAcquiredIterator struct {
	Event *InstantRedemptionAdapterClaimCuratorAcquired // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterClaimCuratorAcquiredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterClaimCuratorAcquired)
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
		it.Event = new(InstantRedemptionAdapterClaimCuratorAcquired)
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
func (it *InstantRedemptionAdapterClaimCuratorAcquiredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterClaimCuratorAcquiredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterClaimCuratorAcquired represents a ClaimCuratorAcquired event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterClaimCuratorAcquired struct {
	Vault         common.Address
	TokenToRedeem common.Address
	Amount        *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterClaimCuratorAcquired is a free log retrieval operation binding the contract event 0x75ca5538729632a2090e30cae1e0647375f4768d57d417d865c26c729b4f70c3.
//
// Solidity: event ClaimCuratorAcquired(address indexed vault, address indexed tokenToRedeem, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterClaimCuratorAcquired(opts *bind.FilterOpts, vault []common.Address, tokenToRedeem []common.Address) (*InstantRedemptionAdapterClaimCuratorAcquiredIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "ClaimCuratorAcquired", vaultRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterClaimCuratorAcquiredIterator{contract: _InstantRedemptionAdapter.contract, event: "ClaimCuratorAcquired", logs: logs, sub: sub}, nil
}

// WatchClaimCuratorAcquired is a free log subscription operation binding the contract event 0x75ca5538729632a2090e30cae1e0647375f4768d57d417d865c26c729b4f70c3.
//
// Solidity: event ClaimCuratorAcquired(address indexed vault, address indexed tokenToRedeem, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchClaimCuratorAcquired(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterClaimCuratorAcquired, vault []common.Address, tokenToRedeem []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "ClaimCuratorAcquired", vaultRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterClaimCuratorAcquired)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "ClaimCuratorAcquired", log); err != nil {
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

// ParseClaimCuratorAcquired is a log parse operation binding the contract event 0x75ca5538729632a2090e30cae1e0647375f4768d57d417d865c26c729b4f70c3.
//
// Solidity: event ClaimCuratorAcquired(address indexed vault, address indexed tokenToRedeem, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseClaimCuratorAcquired(log types.Log) (*InstantRedemptionAdapterClaimCuratorAcquired, error) {
	event := new(InstantRedemptionAdapterClaimCuratorAcquired)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "ClaimCuratorAcquired", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterClaimMarketMakerAcquiredIterator is returned from FilterClaimMarketMakerAcquired and is used to iterate over the raw logs and unpacked data for ClaimMarketMakerAcquired events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterClaimMarketMakerAcquiredIterator struct {
	Event *InstantRedemptionAdapterClaimMarketMakerAcquired // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterClaimMarketMakerAcquiredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterClaimMarketMakerAcquired)
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
		it.Event = new(InstantRedemptionAdapterClaimMarketMakerAcquired)
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
func (it *InstantRedemptionAdapterClaimMarketMakerAcquiredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterClaimMarketMakerAcquiredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterClaimMarketMakerAcquired represents a ClaimMarketMakerAcquired event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterClaimMarketMakerAcquired struct {
	MarketMaker   common.Address
	TokenToRedeem common.Address
	Amount        *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterClaimMarketMakerAcquired is a free log retrieval operation binding the contract event 0x2808db107e0d21c0f367f455aad7ebe5e640568b97a8c834802c69d76fcead21.
//
// Solidity: event ClaimMarketMakerAcquired(address indexed marketMaker, address indexed tokenToRedeem, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterClaimMarketMakerAcquired(opts *bind.FilterOpts, marketMaker []common.Address, tokenToRedeem []common.Address) (*InstantRedemptionAdapterClaimMarketMakerAcquiredIterator, error) {

	var marketMakerRule []interface{}
	for _, marketMakerItem := range marketMaker {
		marketMakerRule = append(marketMakerRule, marketMakerItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "ClaimMarketMakerAcquired", marketMakerRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterClaimMarketMakerAcquiredIterator{contract: _InstantRedemptionAdapter.contract, event: "ClaimMarketMakerAcquired", logs: logs, sub: sub}, nil
}

// WatchClaimMarketMakerAcquired is a free log subscription operation binding the contract event 0x2808db107e0d21c0f367f455aad7ebe5e640568b97a8c834802c69d76fcead21.
//
// Solidity: event ClaimMarketMakerAcquired(address indexed marketMaker, address indexed tokenToRedeem, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchClaimMarketMakerAcquired(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterClaimMarketMakerAcquired, marketMaker []common.Address, tokenToRedeem []common.Address) (event.Subscription, error) {

	var marketMakerRule []interface{}
	for _, marketMakerItem := range marketMaker {
		marketMakerRule = append(marketMakerRule, marketMakerItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "ClaimMarketMakerAcquired", marketMakerRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterClaimMarketMakerAcquired)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "ClaimMarketMakerAcquired", log); err != nil {
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

// ParseClaimMarketMakerAcquired is a log parse operation binding the contract event 0x2808db107e0d21c0f367f455aad7ebe5e640568b97a8c834802c69d76fcead21.
//
// Solidity: event ClaimMarketMakerAcquired(address indexed marketMaker, address indexed tokenToRedeem, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseClaimMarketMakerAcquired(log types.Log) (*InstantRedemptionAdapterClaimMarketMakerAcquired, error) {
	event := new(InstantRedemptionAdapterClaimMarketMakerAcquired)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "ClaimMarketMakerAcquired", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterConvertRedemptionIterator is returned from FilterConvertRedemption and is used to iterate over the raw logs and unpacked data for ConvertRedemption events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterConvertRedemptionIterator struct {
	Event *InstantRedemptionAdapterConvertRedemption // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterConvertRedemptionIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterConvertRedemption)
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
		it.Event = new(InstantRedemptionAdapterConvertRedemption)
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
func (it *InstantRedemptionAdapterConvertRedemptionIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterConvertRedemptionIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterConvertRedemption represents a ConvertRedemption event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterConvertRedemption struct {
	Vault            common.Address
	TokenToRedeem    common.Address
	RedemptionToken  common.Address
	RedemptionAmount *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterConvertRedemption is a free log retrieval operation binding the contract event 0x1834055e60b2ddcfb76848ec8145193643dc7b9ca6aad486f68b0b314db623ce.
//
// Solidity: event ConvertRedemption(address indexed vault, address indexed tokenToRedeem, address indexed redemptionToken, uint256 redemptionAmount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterConvertRedemption(opts *bind.FilterOpts, vault []common.Address, tokenToRedeem []common.Address, redemptionToken []common.Address) (*InstantRedemptionAdapterConvertRedemptionIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var redemptionTokenRule []interface{}
	for _, redemptionTokenItem := range redemptionToken {
		redemptionTokenRule = append(redemptionTokenRule, redemptionTokenItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "ConvertRedemption", vaultRule, tokenToRedeemRule, redemptionTokenRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterConvertRedemptionIterator{contract: _InstantRedemptionAdapter.contract, event: "ConvertRedemption", logs: logs, sub: sub}, nil
}

// WatchConvertRedemption is a free log subscription operation binding the contract event 0x1834055e60b2ddcfb76848ec8145193643dc7b9ca6aad486f68b0b314db623ce.
//
// Solidity: event ConvertRedemption(address indexed vault, address indexed tokenToRedeem, address indexed redemptionToken, uint256 redemptionAmount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchConvertRedemption(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterConvertRedemption, vault []common.Address, tokenToRedeem []common.Address, redemptionToken []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var redemptionTokenRule []interface{}
	for _, redemptionTokenItem := range redemptionToken {
		redemptionTokenRule = append(redemptionTokenRule, redemptionTokenItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "ConvertRedemption", vaultRule, tokenToRedeemRule, redemptionTokenRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterConvertRedemption)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "ConvertRedemption", log); err != nil {
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

// ParseConvertRedemption is a log parse operation binding the contract event 0x1834055e60b2ddcfb76848ec8145193643dc7b9ca6aad486f68b0b314db623ce.
//
// Solidity: event ConvertRedemption(address indexed vault, address indexed tokenToRedeem, address indexed redemptionToken, uint256 redemptionAmount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseConvertRedemption(log types.Log) (*InstantRedemptionAdapterConvertRedemption, error) {
	event := new(InstantRedemptionAdapterConvertRedemption)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "ConvertRedemption", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterDeallocateIterator is returned from FilterDeallocate and is used to iterate over the raw logs and unpacked data for Deallocate events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterDeallocateIterator struct {
	Event *InstantRedemptionAdapterDeallocate // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterDeallocateIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterDeallocate)
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
		it.Event = new(InstantRedemptionAdapterDeallocate)
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
func (it *InstantRedemptionAdapterDeallocateIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterDeallocateIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterDeallocate represents a Deallocate event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterDeallocate struct {
	RequestedAmount *big.Int
	Deallocated     *big.Int
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterDeallocate is a free log retrieval operation binding the contract event 0x486efa184b769280632cd5dee06c3e2a96af54a4aaa618756953b4149bb29c3a.
//
// Solidity: event Deallocate(uint256 requestedAmount, uint256 deallocated)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterDeallocate(opts *bind.FilterOpts) (*InstantRedemptionAdapterDeallocateIterator, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "Deallocate")
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterDeallocateIterator{contract: _InstantRedemptionAdapter.contract, event: "Deallocate", logs: logs, sub: sub}, nil
}

// WatchDeallocate is a free log subscription operation binding the contract event 0x486efa184b769280632cd5dee06c3e2a96af54a4aaa618756953b4149bb29c3a.
//
// Solidity: event Deallocate(uint256 requestedAmount, uint256 deallocated)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchDeallocate(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterDeallocate) (event.Subscription, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "Deallocate")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterDeallocate)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Deallocate", log); err != nil {
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

// ParseDeallocate is a log parse operation binding the contract event 0x486efa184b769280632cd5dee06c3e2a96af54a4aaa618756953b4149bb29c3a.
//
// Solidity: event Deallocate(uint256 requestedAmount, uint256 deallocated)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseDeallocate(log types.Log) (*InstantRedemptionAdapterDeallocate, error) {
	event := new(InstantRedemptionAdapterDeallocate)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Deallocate", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterDepositToAcquireIterator is returned from FilterDepositToAcquire and is used to iterate over the raw logs and unpacked data for DepositToAcquire events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterDepositToAcquireIterator struct {
	Event *InstantRedemptionAdapterDepositToAcquire // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterDepositToAcquireIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterDepositToAcquire)
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
		it.Event = new(InstantRedemptionAdapterDepositToAcquire)
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
func (it *InstantRedemptionAdapterDepositToAcquireIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterDepositToAcquireIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterDepositToAcquire represents a DepositToAcquire event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterDepositToAcquire struct {
	Vault  common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterDepositToAcquire is a free log retrieval operation binding the contract event 0x7121e2d032b2b85e2a75dcfe9598568a93c97203d6a9c51b1c26a7969c1a0b57.
//
// Solidity: event DepositToAcquire(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterDepositToAcquire(opts *bind.FilterOpts, vault []common.Address) (*InstantRedemptionAdapterDepositToAcquireIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "DepositToAcquire", vaultRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterDepositToAcquireIterator{contract: _InstantRedemptionAdapter.contract, event: "DepositToAcquire", logs: logs, sub: sub}, nil
}

// WatchDepositToAcquire is a free log subscription operation binding the contract event 0x7121e2d032b2b85e2a75dcfe9598568a93c97203d6a9c51b1c26a7969c1a0b57.
//
// Solidity: event DepositToAcquire(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchDepositToAcquire(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterDepositToAcquire, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "DepositToAcquire", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterDepositToAcquire)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "DepositToAcquire", log); err != nil {
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

// ParseDepositToAcquire is a log parse operation binding the contract event 0x7121e2d032b2b85e2a75dcfe9598568a93c97203d6a9c51b1c26a7969c1a0b57.
//
// Solidity: event DepositToAcquire(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseDepositToAcquire(log types.Log) (*InstantRedemptionAdapterDepositToAcquire, error) {
	event := new(InstantRedemptionAdapterDepositToAcquire)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "DepositToAcquire", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterDoSwapIterator is returned from FilterDoSwap and is used to iterate over the raw logs and unpacked data for DoSwap events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterDoSwapIterator struct {
	Event *InstantRedemptionAdapterDoSwap // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterDoSwapIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterDoSwap)
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
		it.Event = new(InstantRedemptionAdapterDoSwap)
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
func (it *InstantRedemptionAdapterDoSwapIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterDoSwapIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterDoSwap represents a DoSwap event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterDoSwap struct {
	Swap IInstantRedemptionAdapterSwap
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterDoSwap is a free log retrieval operation binding the contract event 0x5670a345aac78da831c4a6fcfbdd68f5bf55965792b45e7bbcf5a4fcea234954.
//
// Solidity: event DoSwap((address,address,address,uint256,uint256) swap)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterDoSwap(opts *bind.FilterOpts) (*InstantRedemptionAdapterDoSwapIterator, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "DoSwap")
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterDoSwapIterator{contract: _InstantRedemptionAdapter.contract, event: "DoSwap", logs: logs, sub: sub}, nil
}

// WatchDoSwap is a free log subscription operation binding the contract event 0x5670a345aac78da831c4a6fcfbdd68f5bf55965792b45e7bbcf5a4fcea234954.
//
// Solidity: event DoSwap((address,address,address,uint256,uint256) swap)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchDoSwap(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterDoSwap) (event.Subscription, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "DoSwap")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterDoSwap)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "DoSwap", log); err != nil {
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

// ParseDoSwap is a log parse operation binding the contract event 0x5670a345aac78da831c4a6fcfbdd68f5bf55965792b45e7bbcf5a4fcea234954.
//
// Solidity: event DoSwap((address,address,address,uint256,uint256) swap)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseDoSwap(log types.Log) (*InstantRedemptionAdapterDoSwap, error) {
	event := new(InstantRedemptionAdapterDoSwap)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "DoSwap", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterInitializeIterator is returned from FilterInitialize and is used to iterate over the raw logs and unpacked data for Initialize events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterInitializeIterator struct {
	Event *InstantRedemptionAdapterInitialize // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterInitializeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterInitialize)
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
		it.Event = new(InstantRedemptionAdapterInitialize)
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
func (it *InstantRedemptionAdapterInitializeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterInitializeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterInitialize represents a Initialize event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterInitialize struct {
	Owner common.Address
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterInitialize is a free log retrieval operation binding the contract event 0x36b1453565f45af7b509b59d5e2eac8f1948ea9e3e193db6663b4101afb6382c.
//
// Solidity: event Initialize(address owner)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterInitialize(opts *bind.FilterOpts) (*InstantRedemptionAdapterInitializeIterator, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterInitializeIterator{contract: _InstantRedemptionAdapter.contract, event: "Initialize", logs: logs, sub: sub}, nil
}

// WatchInitialize is a free log subscription operation binding the contract event 0x36b1453565f45af7b509b59d5e2eac8f1948ea9e3e193db6663b4101afb6382c.
//
// Solidity: event Initialize(address owner)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchInitialize(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterInitialize) (event.Subscription, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterInitialize)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Initialize", log); err != nil {
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

// ParseInitialize is a log parse operation binding the contract event 0x36b1453565f45af7b509b59d5e2eac8f1948ea9e3e193db6663b4101afb6382c.
//
// Solidity: event Initialize(address owner)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseInitialize(log types.Log) (*InstantRedemptionAdapterInitialize, error) {
	event := new(InstantRedemptionAdapterInitialize)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Initialize", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterInitializedIterator struct {
	Event *InstantRedemptionAdapterInitialized // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterInitialized)
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
		it.Event = new(InstantRedemptionAdapterInitialized)
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
func (it *InstantRedemptionAdapterInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterInitialized represents a Initialized event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterInitialized struct {
	Version uint64
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterInitialized(opts *bind.FilterOpts) (*InstantRedemptionAdapterInitializedIterator, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterInitializedIterator{contract: _InstantRedemptionAdapter.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterInitialized) (event.Subscription, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterInitialized)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Initialized", log); err != nil {
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
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseInitialized(log types.Log) (*InstantRedemptionAdapterInitialized, error) {
	event := new(InstantRedemptionAdapterInitialized)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterInvalidateNonceIterator is returned from FilterInvalidateNonce and is used to iterate over the raw logs and unpacked data for InvalidateNonce events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterInvalidateNonceIterator struct {
	Event *InstantRedemptionAdapterInvalidateNonce // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterInvalidateNonceIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterInvalidateNonce)
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
		it.Event = new(InstantRedemptionAdapterInvalidateNonce)
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
func (it *InstantRedemptionAdapterInvalidateNonceIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterInvalidateNonceIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterInvalidateNonce represents a InvalidateNonce event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterInvalidateNonce struct {
	Vault         common.Address
	TokenToRedeem common.Address
	Nonce         *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterInvalidateNonce is a free log retrieval operation binding the contract event 0xd903578f5f3967478f80c09b6e6fe0253bc1aa52a0e1f8ef2f0d7046020e9360.
//
// Solidity: event InvalidateNonce(address indexed vault, address indexed tokenToRedeem, uint256 indexed nonce)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterInvalidateNonce(opts *bind.FilterOpts, vault []common.Address, tokenToRedeem []common.Address, nonce []*big.Int) (*InstantRedemptionAdapterInvalidateNonceIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "InvalidateNonce", vaultRule, tokenToRedeemRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterInvalidateNonceIterator{contract: _InstantRedemptionAdapter.contract, event: "InvalidateNonce", logs: logs, sub: sub}, nil
}

// WatchInvalidateNonce is a free log subscription operation binding the contract event 0xd903578f5f3967478f80c09b6e6fe0253bc1aa52a0e1f8ef2f0d7046020e9360.
//
// Solidity: event InvalidateNonce(address indexed vault, address indexed tokenToRedeem, uint256 indexed nonce)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchInvalidateNonce(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterInvalidateNonce, vault []common.Address, tokenToRedeem []common.Address, nonce []*big.Int) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "InvalidateNonce", vaultRule, tokenToRedeemRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterInvalidateNonce)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "InvalidateNonce", log); err != nil {
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

// ParseInvalidateNonce is a log parse operation binding the contract event 0xd903578f5f3967478f80c09b6e6fe0253bc1aa52a0e1f8ef2f0d7046020e9360.
//
// Solidity: event InvalidateNonce(address indexed vault, address indexed tokenToRedeem, uint256 indexed nonce)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseInvalidateNonce(log types.Log) (*InstantRedemptionAdapterInvalidateNonce, error) {
	event := new(InstantRedemptionAdapterInvalidateNonce)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "InvalidateNonce", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterOwnershipTransferredIterator struct {
	Event *InstantRedemptionAdapterOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterOwnershipTransferred)
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
		it.Event = new(InstantRedemptionAdapterOwnershipTransferred)
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
func (it *InstantRedemptionAdapterOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterOwnershipTransferred represents a OwnershipTransferred event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*InstantRedemptionAdapterOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterOwnershipTransferredIterator{contract: _InstantRedemptionAdapter.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterOwnershipTransferred)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseOwnershipTransferred(log types.Log) (*InstantRedemptionAdapterOwnershipTransferred, error) {
	event := new(InstantRedemptionAdapterOwnershipTransferred)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterRecoverIterator is returned from FilterRecover and is used to iterate over the raw logs and unpacked data for Recover events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterRecoverIterator struct {
	Event *InstantRedemptionAdapterRecover // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterRecoverIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterRecover)
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
		it.Event = new(InstantRedemptionAdapterRecover)
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
func (it *InstantRedemptionAdapterRecoverIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterRecoverIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterRecover represents a Recover event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterRecover struct {
	Vault  common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRecover is a free log retrieval operation binding the contract event 0x817c5912299b2d8eea4d9429e557c7b42c96a31499b4229932d1f070f068e37a.
//
// Solidity: event Recover(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterRecover(opts *bind.FilterOpts, vault []common.Address) (*InstantRedemptionAdapterRecoverIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "Recover", vaultRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterRecoverIterator{contract: _InstantRedemptionAdapter.contract, event: "Recover", logs: logs, sub: sub}, nil
}

// WatchRecover is a free log subscription operation binding the contract event 0x817c5912299b2d8eea4d9429e557c7b42c96a31499b4229932d1f070f068e37a.
//
// Solidity: event Recover(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchRecover(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterRecover, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "Recover", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterRecover)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Recover", log); err != nil {
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

// ParseRecover is a log parse operation binding the contract event 0x817c5912299b2d8eea4d9429e557c7b42c96a31499b4229932d1f070f068e37a.
//
// Solidity: event Recover(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseRecover(log types.Log) (*InstantRedemptionAdapterRecover, error) {
	event := new(InstantRedemptionAdapterRecover)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Recover", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetAccountBeaconIterator is returned from FilterSetAccountBeacon and is used to iterate over the raw logs and unpacked data for SetAccountBeacon events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetAccountBeaconIterator struct {
	Event *InstantRedemptionAdapterSetAccountBeacon // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetAccountBeaconIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetAccountBeacon)
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
		it.Event = new(InstantRedemptionAdapterSetAccountBeacon)
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
func (it *InstantRedemptionAdapterSetAccountBeaconIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetAccountBeaconIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetAccountBeacon represents a SetAccountBeacon event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetAccountBeacon struct {
	TokenToRedeem common.Address
	Beacon        common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterSetAccountBeacon is a free log retrieval operation binding the contract event 0x7d3678f9c650f2d06a895ea5d6270092e747607cd1f97e79f3abacac584c5fc1.
//
// Solidity: event SetAccountBeacon(address indexed tokenToRedeem, address indexed beacon)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetAccountBeacon(opts *bind.FilterOpts, tokenToRedeem []common.Address, beacon []common.Address) (*InstantRedemptionAdapterSetAccountBeaconIterator, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetAccountBeacon", tokenToRedeemRule, beaconRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetAccountBeaconIterator{contract: _InstantRedemptionAdapter.contract, event: "SetAccountBeacon", logs: logs, sub: sub}, nil
}

// WatchSetAccountBeacon is a free log subscription operation binding the contract event 0x7d3678f9c650f2d06a895ea5d6270092e747607cd1f97e79f3abacac584c5fc1.
//
// Solidity: event SetAccountBeacon(address indexed tokenToRedeem, address indexed beacon)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetAccountBeacon(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetAccountBeacon, tokenToRedeem []common.Address, beacon []common.Address) (event.Subscription, error) {

	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}
	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetAccountBeacon", tokenToRedeemRule, beaconRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetAccountBeacon)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetAccountBeacon", log); err != nil {
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

// ParseSetAccountBeacon is a log parse operation binding the contract event 0x7d3678f9c650f2d06a895ea5d6270092e747607cd1f97e79f3abacac584c5fc1.
//
// Solidity: event SetAccountBeacon(address indexed tokenToRedeem, address indexed beacon)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetAccountBeacon(log types.Log) (*InstantRedemptionAdapterSetAccountBeacon, error) {
	event := new(InstantRedemptionAdapterSetAccountBeacon)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetAccountBeacon", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetConversionAdapterIterator is returned from FilterSetConversionAdapter and is used to iterate over the raw logs and unpacked data for SetConversionAdapter events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetConversionAdapterIterator struct {
	Event *InstantRedemptionAdapterSetConversionAdapter // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetConversionAdapterIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetConversionAdapter)
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
		it.Event = new(InstantRedemptionAdapterSetConversionAdapter)
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
func (it *InstantRedemptionAdapterSetConversionAdapterIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetConversionAdapterIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetConversionAdapter represents a SetConversionAdapter event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetConversionAdapter struct {
	RedemptionToken   common.Address
	CollateralToken   common.Address
	ConversionAdapter common.Address
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterSetConversionAdapter is a free log retrieval operation binding the contract event 0xa7cb63df4a4bb0d4878266a21ad37659c5656471779bf90199c141c3b92f4c54.
//
// Solidity: event SetConversionAdapter(address indexed redemptionToken, address indexed collateralToken, address indexed conversionAdapter)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetConversionAdapter(opts *bind.FilterOpts, redemptionToken []common.Address, collateralToken []common.Address, conversionAdapter []common.Address) (*InstantRedemptionAdapterSetConversionAdapterIterator, error) {

	var redemptionTokenRule []interface{}
	for _, redemptionTokenItem := range redemptionToken {
		redemptionTokenRule = append(redemptionTokenRule, redemptionTokenItem)
	}
	var collateralTokenRule []interface{}
	for _, collateralTokenItem := range collateralToken {
		collateralTokenRule = append(collateralTokenRule, collateralTokenItem)
	}
	var conversionAdapterRule []interface{}
	for _, conversionAdapterItem := range conversionAdapter {
		conversionAdapterRule = append(conversionAdapterRule, conversionAdapterItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetConversionAdapter", redemptionTokenRule, collateralTokenRule, conversionAdapterRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetConversionAdapterIterator{contract: _InstantRedemptionAdapter.contract, event: "SetConversionAdapter", logs: logs, sub: sub}, nil
}

// WatchSetConversionAdapter is a free log subscription operation binding the contract event 0xa7cb63df4a4bb0d4878266a21ad37659c5656471779bf90199c141c3b92f4c54.
//
// Solidity: event SetConversionAdapter(address indexed redemptionToken, address indexed collateralToken, address indexed conversionAdapter)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetConversionAdapter(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetConversionAdapter, redemptionToken []common.Address, collateralToken []common.Address, conversionAdapter []common.Address) (event.Subscription, error) {

	var redemptionTokenRule []interface{}
	for _, redemptionTokenItem := range redemptionToken {
		redemptionTokenRule = append(redemptionTokenRule, redemptionTokenItem)
	}
	var collateralTokenRule []interface{}
	for _, collateralTokenItem := range collateralToken {
		collateralTokenRule = append(collateralTokenRule, collateralTokenItem)
	}
	var conversionAdapterRule []interface{}
	for _, conversionAdapterItem := range conversionAdapter {
		conversionAdapterRule = append(conversionAdapterRule, conversionAdapterItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetConversionAdapter", redemptionTokenRule, collateralTokenRule, conversionAdapterRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetConversionAdapter)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetConversionAdapter", log); err != nil {
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

// ParseSetConversionAdapter is a log parse operation binding the contract event 0xa7cb63df4a4bb0d4878266a21ad37659c5656471779bf90199c141c3b92f4c54.
//
// Solidity: event SetConversionAdapter(address indexed redemptionToken, address indexed collateralToken, address indexed conversionAdapter)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetConversionAdapter(log types.Log) (*InstantRedemptionAdapterSetConversionAdapter, error) {
	event := new(InstantRedemptionAdapterSetConversionAdapter)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetConversionAdapter", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetDeallocAdaptersIterator is returned from FilterSetDeallocAdapters and is used to iterate over the raw logs and unpacked data for SetDeallocAdapters events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetDeallocAdaptersIterator struct {
	Event *InstantRedemptionAdapterSetDeallocAdapters // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetDeallocAdaptersIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetDeallocAdapters)
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
		it.Event = new(InstantRedemptionAdapterSetDeallocAdapters)
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
func (it *InstantRedemptionAdapterSetDeallocAdaptersIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetDeallocAdaptersIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetDeallocAdapters represents a SetDeallocAdapters event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetDeallocAdapters struct {
	Vault              common.Address
	NewDeallocAdapters []common.Address
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterSetDeallocAdapters is a free log retrieval operation binding the contract event 0x26bb36f5cf810e5125daec318cb8212e0e85c0717e4bcfbedff3b0236bcabe16.
//
// Solidity: event SetDeallocAdapters(address indexed vault, address[] newDeallocAdapters)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetDeallocAdapters(opts *bind.FilterOpts, vault []common.Address) (*InstantRedemptionAdapterSetDeallocAdaptersIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetDeallocAdapters", vaultRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetDeallocAdaptersIterator{contract: _InstantRedemptionAdapter.contract, event: "SetDeallocAdapters", logs: logs, sub: sub}, nil
}

// WatchSetDeallocAdapters is a free log subscription operation binding the contract event 0x26bb36f5cf810e5125daec318cb8212e0e85c0717e4bcfbedff3b0236bcabe16.
//
// Solidity: event SetDeallocAdapters(address indexed vault, address[] newDeallocAdapters)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetDeallocAdapters(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetDeallocAdapters, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetDeallocAdapters", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetDeallocAdapters)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetDeallocAdapters", log); err != nil {
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

// ParseSetDeallocAdapters is a log parse operation binding the contract event 0x26bb36f5cf810e5125daec318cb8212e0e85c0717e4bcfbedff3b0236bcabe16.
//
// Solidity: event SetDeallocAdapters(address indexed vault, address[] newDeallocAdapters)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetDeallocAdapters(log types.Log) (*InstantRedemptionAdapterSetDeallocAdapters, error) {
	event := new(InstantRedemptionAdapterSetDeallocAdapters)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetDeallocAdapters", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetFillerIterator is returned from FilterSetFiller and is used to iterate over the raw logs and unpacked data for SetFiller events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetFillerIterator struct {
	Event *InstantRedemptionAdapterSetFiller // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetFillerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetFiller)
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
		it.Event = new(InstantRedemptionAdapterSetFiller)
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
func (it *InstantRedemptionAdapterSetFillerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetFillerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetFiller represents a SetFiller event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetFiller struct {
	Vault        common.Address
	MarketMaker  common.Address
	Filler       common.Address
	IsAuthorized bool
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterSetFiller is a free log retrieval operation binding the contract event 0xead34af39f38c14bb32a8b0f6d59b7fd6532ac9996575b0e4f35429f5c4058fa.
//
// Solidity: event SetFiller(address indexed vault, address indexed marketMaker, address indexed filler, bool isAuthorized)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetFiller(opts *bind.FilterOpts, vault []common.Address, marketMaker []common.Address, filler []common.Address) (*InstantRedemptionAdapterSetFillerIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var marketMakerRule []interface{}
	for _, marketMakerItem := range marketMaker {
		marketMakerRule = append(marketMakerRule, marketMakerItem)
	}
	var fillerRule []interface{}
	for _, fillerItem := range filler {
		fillerRule = append(fillerRule, fillerItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetFiller", vaultRule, marketMakerRule, fillerRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetFillerIterator{contract: _InstantRedemptionAdapter.contract, event: "SetFiller", logs: logs, sub: sub}, nil
}

// WatchSetFiller is a free log subscription operation binding the contract event 0xead34af39f38c14bb32a8b0f6d59b7fd6532ac9996575b0e4f35429f5c4058fa.
//
// Solidity: event SetFiller(address indexed vault, address indexed marketMaker, address indexed filler, bool isAuthorized)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetFiller(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetFiller, vault []common.Address, marketMaker []common.Address, filler []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var marketMakerRule []interface{}
	for _, marketMakerItem := range marketMaker {
		marketMakerRule = append(marketMakerRule, marketMakerItem)
	}
	var fillerRule []interface{}
	for _, fillerItem := range filler {
		fillerRule = append(fillerRule, fillerItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetFiller", vaultRule, marketMakerRule, fillerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetFiller)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetFiller", log); err != nil {
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

// ParseSetFiller is a log parse operation binding the contract event 0xead34af39f38c14bb32a8b0f6d59b7fd6532ac9996575b0e4f35429f5c4058fa.
//
// Solidity: event SetFiller(address indexed vault, address indexed marketMaker, address indexed filler, bool isAuthorized)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetFiller(log types.Log) (*InstantRedemptionAdapterSetFiller, error) {
	event := new(InstantRedemptionAdapterSetFiller)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetFiller", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetGlobalLimitIterator is returned from FilterSetGlobalLimit and is used to iterate over the raw logs and unpacked data for SetGlobalLimit events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetGlobalLimitIterator struct {
	Event *InstantRedemptionAdapterSetGlobalLimit // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetGlobalLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetGlobalLimit)
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
		it.Event = new(InstantRedemptionAdapterSetGlobalLimit)
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
func (it *InstantRedemptionAdapterSetGlobalLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetGlobalLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetGlobalLimit represents a SetGlobalLimit event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetGlobalLimit struct {
	Asset common.Address
	Limit *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterSetGlobalLimit is a free log retrieval operation binding the contract event 0x7bd864b8ef4d7c1c4558c27c45f97152e628a3a3ef3866f9c3f9ac3b798ea954.
//
// Solidity: event SetGlobalLimit(address indexed asset, uint256 limit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetGlobalLimit(opts *bind.FilterOpts, asset []common.Address) (*InstantRedemptionAdapterSetGlobalLimitIterator, error) {

	var assetRule []interface{}
	for _, assetItem := range asset {
		assetRule = append(assetRule, assetItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetGlobalLimit", assetRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetGlobalLimitIterator{contract: _InstantRedemptionAdapter.contract, event: "SetGlobalLimit", logs: logs, sub: sub}, nil
}

// WatchSetGlobalLimit is a free log subscription operation binding the contract event 0x7bd864b8ef4d7c1c4558c27c45f97152e628a3a3ef3866f9c3f9ac3b798ea954.
//
// Solidity: event SetGlobalLimit(address indexed asset, uint256 limit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetGlobalLimit(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetGlobalLimit, asset []common.Address) (event.Subscription, error) {

	var assetRule []interface{}
	for _, assetItem := range asset {
		assetRule = append(assetRule, assetItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetGlobalLimit", assetRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetGlobalLimit)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetGlobalLimit", log); err != nil {
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

// ParseSetGlobalLimit is a log parse operation binding the contract event 0x7bd864b8ef4d7c1c4558c27c45f97152e628a3a3ef3866f9c3f9ac3b798ea954.
//
// Solidity: event SetGlobalLimit(address indexed asset, uint256 limit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetGlobalLimit(log types.Log) (*InstantRedemptionAdapterSetGlobalLimit, error) {
	event := new(InstantRedemptionAdapterSetGlobalLimit)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetGlobalLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetGlobalMaxDiscountIterator is returned from FilterSetGlobalMaxDiscount and is used to iterate over the raw logs and unpacked data for SetGlobalMaxDiscount events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetGlobalMaxDiscountIterator struct {
	Event *InstantRedemptionAdapterSetGlobalMaxDiscount // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetGlobalMaxDiscountIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetGlobalMaxDiscount)
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
		it.Event = new(InstantRedemptionAdapterSetGlobalMaxDiscount)
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
func (it *InstantRedemptionAdapterSetGlobalMaxDiscountIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetGlobalMaxDiscountIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetGlobalMaxDiscount represents a SetGlobalMaxDiscount event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetGlobalMaxDiscount struct {
	NewGlobalMaxDiscount *big.Int
	Raw                  types.Log // Blockchain specific contextual infos
}

// FilterSetGlobalMaxDiscount is a free log retrieval operation binding the contract event 0xd567abee17ceb96fad169cf4b37d62bb00cc3b82856b0f8da3cbdd122c439ac6.
//
// Solidity: event SetGlobalMaxDiscount(uint256 newGlobalMaxDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetGlobalMaxDiscount(opts *bind.FilterOpts) (*InstantRedemptionAdapterSetGlobalMaxDiscountIterator, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetGlobalMaxDiscount")
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetGlobalMaxDiscountIterator{contract: _InstantRedemptionAdapter.contract, event: "SetGlobalMaxDiscount", logs: logs, sub: sub}, nil
}

// WatchSetGlobalMaxDiscount is a free log subscription operation binding the contract event 0xd567abee17ceb96fad169cf4b37d62bb00cc3b82856b0f8da3cbdd122c439ac6.
//
// Solidity: event SetGlobalMaxDiscount(uint256 newGlobalMaxDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetGlobalMaxDiscount(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetGlobalMaxDiscount) (event.Subscription, error) {

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetGlobalMaxDiscount")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetGlobalMaxDiscount)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetGlobalMaxDiscount", log); err != nil {
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

// ParseSetGlobalMaxDiscount is a log parse operation binding the contract event 0xd567abee17ceb96fad169cf4b37d62bb00cc3b82856b0f8da3cbdd122c439ac6.
//
// Solidity: event SetGlobalMaxDiscount(uint256 newGlobalMaxDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetGlobalMaxDiscount(log types.Log) (*InstantRedemptionAdapterSetGlobalMaxDiscount, error) {
	event := new(InstantRedemptionAdapterSetGlobalMaxDiscount)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetGlobalMaxDiscount", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetLimitIterator is returned from FilterSetLimit and is used to iterate over the raw logs and unpacked data for SetLimit events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetLimitIterator struct {
	Event *InstantRedemptionAdapterSetLimit // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetLimit)
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
		it.Event = new(InstantRedemptionAdapterSetLimit)
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
func (it *InstantRedemptionAdapterSetLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetLimit represents a SetLimit event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetLimit struct {
	Vault         common.Address
	TokenToRedeem common.Address
	NewLimit      *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterSetLimit is a free log retrieval operation binding the contract event 0xe0cd3dd8d3e5db48930cbd35eac84370a908a51a0682ef297ad21062f9e5f61f.
//
// Solidity: event SetLimit(address indexed vault, address indexed tokenToRedeem, uint256 newLimit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetLimit(opts *bind.FilterOpts, vault []common.Address, tokenToRedeem []common.Address) (*InstantRedemptionAdapterSetLimitIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetLimit", vaultRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetLimitIterator{contract: _InstantRedemptionAdapter.contract, event: "SetLimit", logs: logs, sub: sub}, nil
}

// WatchSetLimit is a free log subscription operation binding the contract event 0xe0cd3dd8d3e5db48930cbd35eac84370a908a51a0682ef297ad21062f9e5f61f.
//
// Solidity: event SetLimit(address indexed vault, address indexed tokenToRedeem, uint256 newLimit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetLimit(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetLimit, vault []common.Address, tokenToRedeem []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetLimit", vaultRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetLimit)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetLimit", log); err != nil {
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

// ParseSetLimit is a log parse operation binding the contract event 0xe0cd3dd8d3e5db48930cbd35eac84370a908a51a0682ef297ad21062f9e5f61f.
//
// Solidity: event SetLimit(address indexed vault, address indexed tokenToRedeem, uint256 newLimit)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetLimit(log types.Log) (*InstantRedemptionAdapterSetLimit, error) {
	event := new(InstantRedemptionAdapterSetLimit)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetMarketMakerIterator is returned from FilterSetMarketMaker and is used to iterate over the raw logs and unpacked data for SetMarketMaker events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetMarketMakerIterator struct {
	Event *InstantRedemptionAdapterSetMarketMaker // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetMarketMakerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetMarketMaker)
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
		it.Event = new(InstantRedemptionAdapterSetMarketMaker)
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
func (it *InstantRedemptionAdapterSetMarketMakerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetMarketMakerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetMarketMaker represents a SetMarketMaker event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetMarketMaker struct {
	Vault          common.Address
	NewMarketMaker common.Address
	NewCanAcquire  bool
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterSetMarketMaker is a free log retrieval operation binding the contract event 0xe58fa2a6d93b7d39564bfd02fe8ed99ff39ca16ca173d5b059e88b49b5675413.
//
// Solidity: event SetMarketMaker(address indexed vault, address indexed newMarketMaker, bool newCanAcquire)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetMarketMaker(opts *bind.FilterOpts, vault []common.Address, newMarketMaker []common.Address) (*InstantRedemptionAdapterSetMarketMakerIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var newMarketMakerRule []interface{}
	for _, newMarketMakerItem := range newMarketMaker {
		newMarketMakerRule = append(newMarketMakerRule, newMarketMakerItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetMarketMaker", vaultRule, newMarketMakerRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetMarketMakerIterator{contract: _InstantRedemptionAdapter.contract, event: "SetMarketMaker", logs: logs, sub: sub}, nil
}

// WatchSetMarketMaker is a free log subscription operation binding the contract event 0xe58fa2a6d93b7d39564bfd02fe8ed99ff39ca16ca173d5b059e88b49b5675413.
//
// Solidity: event SetMarketMaker(address indexed vault, address indexed newMarketMaker, bool newCanAcquire)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetMarketMaker(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetMarketMaker, vault []common.Address, newMarketMaker []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var newMarketMakerRule []interface{}
	for _, newMarketMakerItem := range newMarketMaker {
		newMarketMakerRule = append(newMarketMakerRule, newMarketMakerItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetMarketMaker", vaultRule, newMarketMakerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetMarketMaker)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetMarketMaker", log); err != nil {
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

// ParseSetMarketMaker is a log parse operation binding the contract event 0xe58fa2a6d93b7d39564bfd02fe8ed99ff39ca16ca173d5b059e88b49b5675413.
//
// Solidity: event SetMarketMaker(address indexed vault, address indexed newMarketMaker, bool newCanAcquire)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetMarketMaker(log types.Log) (*InstantRedemptionAdapterSetMarketMaker, error) {
	event := new(InstantRedemptionAdapterSetMarketMaker)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetMarketMaker", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetMinDiscountIterator is returned from FilterSetMinDiscount and is used to iterate over the raw logs and unpacked data for SetMinDiscount events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetMinDiscountIterator struct {
	Event *InstantRedemptionAdapterSetMinDiscount // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetMinDiscountIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetMinDiscount)
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
		it.Event = new(InstantRedemptionAdapterSetMinDiscount)
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
func (it *InstantRedemptionAdapterSetMinDiscountIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetMinDiscountIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetMinDiscount represents a SetMinDiscount event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetMinDiscount struct {
	Vault          common.Address
	TokenToRedeem  common.Address
	NewMinDiscount *big.Int
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterSetMinDiscount is a free log retrieval operation binding the contract event 0xcb6697441f4076732b53fbca47ce9de8a5d024883441287c59ede043d7984253.
//
// Solidity: event SetMinDiscount(address indexed vault, address indexed tokenToRedeem, uint256 newMinDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetMinDiscount(opts *bind.FilterOpts, vault []common.Address, tokenToRedeem []common.Address) (*InstantRedemptionAdapterSetMinDiscountIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetMinDiscount", vaultRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetMinDiscountIterator{contract: _InstantRedemptionAdapter.contract, event: "SetMinDiscount", logs: logs, sub: sub}, nil
}

// WatchSetMinDiscount is a free log subscription operation binding the contract event 0xcb6697441f4076732b53fbca47ce9de8a5d024883441287c59ede043d7984253.
//
// Solidity: event SetMinDiscount(address indexed vault, address indexed tokenToRedeem, uint256 newMinDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetMinDiscount(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetMinDiscount, vault []common.Address, tokenToRedeem []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var tokenToRedeemRule []interface{}
	for _, tokenToRedeemItem := range tokenToRedeem {
		tokenToRedeemRule = append(tokenToRedeemRule, tokenToRedeemItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetMinDiscount", vaultRule, tokenToRedeemRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetMinDiscount)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetMinDiscount", log); err != nil {
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

// ParseSetMinDiscount is a log parse operation binding the contract event 0xcb6697441f4076732b53fbca47ce9de8a5d024883441287c59ede043d7984253.
//
// Solidity: event SetMinDiscount(address indexed vault, address indexed tokenToRedeem, uint256 newMinDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetMinDiscount(log types.Log) (*InstantRedemptionAdapterSetMinDiscount, error) {
	event := new(InstantRedemptionAdapterSetMinDiscount)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetMinDiscount", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetOracleIterator is returned from FilterSetOracle and is used to iterate over the raw logs and unpacked data for SetOracle events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetOracleIterator struct {
	Event *InstantRedemptionAdapterSetOracle // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetOracleIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetOracle)
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
		it.Event = new(InstantRedemptionAdapterSetOracle)
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
func (it *InstantRedemptionAdapterSetOracleIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetOracleIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetOracle represents a SetOracle event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetOracle struct {
	Token  common.Address
	Oracle common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSetOracle is a free log retrieval operation binding the contract event 0xb7261e9c33aa7c56209c3bf60b424a8f9551ce28876c0ab3d0c487695e943487.
//
// Solidity: event SetOracle(address indexed token, address indexed oracle)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetOracle(opts *bind.FilterOpts, token []common.Address, oracle []common.Address) (*InstantRedemptionAdapterSetOracleIterator, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var oracleRule []interface{}
	for _, oracleItem := range oracle {
		oracleRule = append(oracleRule, oracleItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetOracle", tokenRule, oracleRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetOracleIterator{contract: _InstantRedemptionAdapter.contract, event: "SetOracle", logs: logs, sub: sub}, nil
}

// WatchSetOracle is a free log subscription operation binding the contract event 0xb7261e9c33aa7c56209c3bf60b424a8f9551ce28876c0ab3d0c487695e943487.
//
// Solidity: event SetOracle(address indexed token, address indexed oracle)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetOracle(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetOracle, token []common.Address, oracle []common.Address) (event.Subscription, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var oracleRule []interface{}
	for _, oracleItem := range oracle {
		oracleRule = append(oracleRule, oracleItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetOracle", tokenRule, oracleRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetOracle)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetOracle", log); err != nil {
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

// ParseSetOracle is a log parse operation binding the contract event 0xb7261e9c33aa7c56209c3bf60b424a8f9551ce28876c0ab3d0c487695e943487.
//
// Solidity: event SetOracle(address indexed token, address indexed oracle)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetOracle(log types.Log) (*InstantRedemptionAdapterSetOracle, error) {
	event := new(InstantRedemptionAdapterSetOracle)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetOracle", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetPairMaxDiscountIterator is returned from FilterSetPairMaxDiscount and is used to iterate over the raw logs and unpacked data for SetPairMaxDiscount events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetPairMaxDiscountIterator struct {
	Event *InstantRedemptionAdapterSetPairMaxDiscount // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetPairMaxDiscountIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetPairMaxDiscount)
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
		it.Event = new(InstantRedemptionAdapterSetPairMaxDiscount)
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
func (it *InstantRedemptionAdapterSetPairMaxDiscountIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetPairMaxDiscountIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetPairMaxDiscount represents a SetPairMaxDiscount event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetPairMaxDiscount struct {
	TokenIn            common.Address
	TokenOut           common.Address
	NewPairMaxDiscount *big.Int
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterSetPairMaxDiscount is a free log retrieval operation binding the contract event 0x6cf776fd35c263193b3b018b458aa8b23d9d1f19f181efb3f42fdf75a750b078.
//
// Solidity: event SetPairMaxDiscount(address indexed tokenIn, address indexed tokenOut, uint256 newPairMaxDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetPairMaxDiscount(opts *bind.FilterOpts, tokenIn []common.Address, tokenOut []common.Address) (*InstantRedemptionAdapterSetPairMaxDiscountIterator, error) {

	var tokenInRule []interface{}
	for _, tokenInItem := range tokenIn {
		tokenInRule = append(tokenInRule, tokenInItem)
	}
	var tokenOutRule []interface{}
	for _, tokenOutItem := range tokenOut {
		tokenOutRule = append(tokenOutRule, tokenOutItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetPairMaxDiscount", tokenInRule, tokenOutRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetPairMaxDiscountIterator{contract: _InstantRedemptionAdapter.contract, event: "SetPairMaxDiscount", logs: logs, sub: sub}, nil
}

// WatchSetPairMaxDiscount is a free log subscription operation binding the contract event 0x6cf776fd35c263193b3b018b458aa8b23d9d1f19f181efb3f42fdf75a750b078.
//
// Solidity: event SetPairMaxDiscount(address indexed tokenIn, address indexed tokenOut, uint256 newPairMaxDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetPairMaxDiscount(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetPairMaxDiscount, tokenIn []common.Address, tokenOut []common.Address) (event.Subscription, error) {

	var tokenInRule []interface{}
	for _, tokenInItem := range tokenIn {
		tokenInRule = append(tokenInRule, tokenInItem)
	}
	var tokenOutRule []interface{}
	for _, tokenOutItem := range tokenOut {
		tokenOutRule = append(tokenOutRule, tokenOutItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetPairMaxDiscount", tokenInRule, tokenOutRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetPairMaxDiscount)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetPairMaxDiscount", log); err != nil {
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

// ParseSetPairMaxDiscount is a log parse operation binding the contract event 0x6cf776fd35c263193b3b018b458aa8b23d9d1f19f181efb3f42fdf75a750b078.
//
// Solidity: event SetPairMaxDiscount(address indexed tokenIn, address indexed tokenOut, uint256 newPairMaxDiscount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetPairMaxDiscount(log types.Log) (*InstantRedemptionAdapterSetPairMaxDiscount, error) {
	event := new(InstantRedemptionAdapterSetPairMaxDiscount)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetPairMaxDiscount", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSetPauseStatusIterator is returned from FilterSetPauseStatus and is used to iterate over the raw logs and unpacked data for SetPauseStatus events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetPauseStatusIterator struct {
	Event *InstantRedemptionAdapterSetPauseStatus // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSetPauseStatusIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSetPauseStatus)
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
		it.Event = new(InstantRedemptionAdapterSetPauseStatus)
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
func (it *InstantRedemptionAdapterSetPauseStatusIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSetPauseStatusIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSetPauseStatus represents a SetPauseStatus event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSetPauseStatus struct {
	Vault    common.Address
	IsPaused bool
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterSetPauseStatus is a free log retrieval operation binding the contract event 0xa772808562f9d6b27175f584dcbfc6bf5c4a786027addc0e3a5f6bb512dff163.
//
// Solidity: event SetPauseStatus(address indexed vault, bool isPaused)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSetPauseStatus(opts *bind.FilterOpts, vault []common.Address) (*InstantRedemptionAdapterSetPauseStatusIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "SetPauseStatus", vaultRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSetPauseStatusIterator{contract: _InstantRedemptionAdapter.contract, event: "SetPauseStatus", logs: logs, sub: sub}, nil
}

// WatchSetPauseStatus is a free log subscription operation binding the contract event 0xa772808562f9d6b27175f584dcbfc6bf5c4a786027addc0e3a5f6bb512dff163.
//
// Solidity: event SetPauseStatus(address indexed vault, bool isPaused)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSetPauseStatus(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSetPauseStatus, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "SetPauseStatus", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSetPauseStatus)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetPauseStatus", log); err != nil {
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

// ParseSetPauseStatus is a log parse operation binding the contract event 0xa772808562f9d6b27175f584dcbfc6bf5c4a786027addc0e3a5f6bb512dff163.
//
// Solidity: event SetPauseStatus(address indexed vault, bool isPaused)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSetPauseStatus(log types.Log) (*InstantRedemptionAdapterSetPauseStatus, error) {
	event := new(InstantRedemptionAdapterSetPauseStatus)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "SetPauseStatus", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterSkimIterator is returned from FilterSkim and is used to iterate over the raw logs and unpacked data for Skim events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSkimIterator struct {
	Event *InstantRedemptionAdapterSkim // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterSkimIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterSkim)
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
		it.Event = new(InstantRedemptionAdapterSkim)
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
func (it *InstantRedemptionAdapterSkimIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterSkimIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterSkim represents a Skim event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterSkim struct {
	Vault common.Address
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterSkim is a free log retrieval operation binding the contract event 0x7460d2216ad827491779b20c9921030f955ee2e6f588f0b64b75c4cd031096bf.
//
// Solidity: event Skim(address indexed vault)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterSkim(opts *bind.FilterOpts, vault []common.Address) (*InstantRedemptionAdapterSkimIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "Skim", vaultRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterSkimIterator{contract: _InstantRedemptionAdapter.contract, event: "Skim", logs: logs, sub: sub}, nil
}

// WatchSkim is a free log subscription operation binding the contract event 0x7460d2216ad827491779b20c9921030f955ee2e6f588f0b64b75c4cd031096bf.
//
// Solidity: event Skim(address indexed vault)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchSkim(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterSkim, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "Skim", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterSkim)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Skim", log); err != nil {
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

// ParseSkim is a log parse operation binding the contract event 0x7460d2216ad827491779b20c9921030f955ee2e6f588f0b64b75c4cd031096bf.
//
// Solidity: event Skim(address indexed vault)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseSkim(log types.Log) (*InstantRedemptionAdapterSkim, error) {
	event := new(InstantRedemptionAdapterSkim)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "Skim", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// InstantRedemptionAdapterWithdrawToAcquireIterator is returned from FilterWithdrawToAcquire and is used to iterate over the raw logs and unpacked data for WithdrawToAcquire events raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterWithdrawToAcquireIterator struct {
	Event *InstantRedemptionAdapterWithdrawToAcquire // Event containing the contract specifics and raw log

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
func (it *InstantRedemptionAdapterWithdrawToAcquireIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InstantRedemptionAdapterWithdrawToAcquire)
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
		it.Event = new(InstantRedemptionAdapterWithdrawToAcquire)
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
func (it *InstantRedemptionAdapterWithdrawToAcquireIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InstantRedemptionAdapterWithdrawToAcquireIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InstantRedemptionAdapterWithdrawToAcquire represents a WithdrawToAcquire event raised by the InstantRedemptionAdapter contract.
type InstantRedemptionAdapterWithdrawToAcquire struct {
	Vault  common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterWithdrawToAcquire is a free log retrieval operation binding the contract event 0x1b497d976969a80ad9a87325e72c1b392c2201179ff82f73ccd83c7a3777987e.
//
// Solidity: event WithdrawToAcquire(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) FilterWithdrawToAcquire(opts *bind.FilterOpts, vault []common.Address) (*InstantRedemptionAdapterWithdrawToAcquireIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.FilterLogs(opts, "WithdrawToAcquire", vaultRule)
	if err != nil {
		return nil, err
	}
	return &InstantRedemptionAdapterWithdrawToAcquireIterator{contract: _InstantRedemptionAdapter.contract, event: "WithdrawToAcquire", logs: logs, sub: sub}, nil
}

// WatchWithdrawToAcquire is a free log subscription operation binding the contract event 0x1b497d976969a80ad9a87325e72c1b392c2201179ff82f73ccd83c7a3777987e.
//
// Solidity: event WithdrawToAcquire(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) WatchWithdrawToAcquire(opts *bind.WatchOpts, sink chan<- *InstantRedemptionAdapterWithdrawToAcquire, vault []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}

	logs, sub, err := _InstantRedemptionAdapter.contract.WatchLogs(opts, "WithdrawToAcquire", vaultRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InstantRedemptionAdapterWithdrawToAcquire)
				if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "WithdrawToAcquire", log); err != nil {
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

// ParseWithdrawToAcquire is a log parse operation binding the contract event 0x1b497d976969a80ad9a87325e72c1b392c2201179ff82f73ccd83c7a3777987e.
//
// Solidity: event WithdrawToAcquire(address indexed vault, uint256 amount)
func (_InstantRedemptionAdapter *InstantRedemptionAdapterFilterer) ParseWithdrawToAcquire(log types.Log) (*InstantRedemptionAdapterWithdrawToAcquire, error) {
	event := new(InstantRedemptionAdapterWithdrawToAcquire)
	if err := _InstantRedemptionAdapter.contract.UnpackLog(event, "WithdrawToAcquire", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
