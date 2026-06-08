// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package vaultv2

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

// IVaultV2InitParams is an auto generated low-level Go binding around an user-defined struct.
type IVaultV2InitParams struct {
	Name                          string
	Symbol                        string
	Collateral                    common.Address
	Burner                        common.Address
	EpochDuration                 *big.Int
	DepositWhitelist              bool
	DepositorToWhitelist          common.Address
	IsDepositLimit                bool
	DepositLimit                  *big.Int
	DefaultAdminRoleHolder        common.Address
	DepositWhitelistSetRoleHolder common.Address
	DepositorWhitelistRoleHolder  common.Address
	IsDepositLimitSetRoleHolder   common.Address
	DepositLimitSetRoleHolder     common.Address
	SetAdapterLimitRoleHolder     common.Address
	SwapAdaptersRoleHolder        common.Address
	AllocateAdapterRoleHolder     common.Address
	DeallocateAdapterRoleHolder   common.Address
}

// IVaultV2MigrateParams is an auto generated low-level Go binding around an user-defined struct.
type IVaultV2MigrateParams struct {
	Name                        string
	Symbol                      string
	DefaultAdminRoleHolder      common.Address
	SetAdapterLimitRoleHolder   common.Address
	SwapAdaptersRoleHolder      common.Address
	AllocateAdapterRoleHolder   common.Address
	DeallocateAdapterRoleHolder common.Address
	DelegatorParams             []byte
	SlasherParams               []byte
}

// IVaultV2MetaData contains all meta data concerning the IVaultV2 contract.
var IVaultV2MetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"FACTORY\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeBalanceOf\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeBalanceOfAt\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeShares\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeSharesAt\",\"inputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"hint\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeSharesOf\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeSharesOfAt\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"hint\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeStake\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeStakeAt\",\"inputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"hint\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalShares\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalSharesAt\",\"inputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalSharesFor\",\"inputs\":[{\"name\":\"duration\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalSharesForAt\",\"inputs\":[{\"name\":\"duration\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalSharesOfAt\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawals\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalsAt\",\"inputs\":[{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalsFor\",\"inputs\":[{\"name\":\"duration\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeWithdrawalsForAt\",\"inputs\":[{\"name\":\"duration\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"adapterAllocated\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"adapterLimit\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint208\",\"internalType\":\"uint208\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"adapters\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"adaptersAllocated\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"adaptersLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"adaptersOwe\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocatable\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"allocateAdapter\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"allocated\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"burner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"claim\",\"inputs\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"claimBatch\",\"inputs\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"indexes\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"collateral\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"deallocateAdapter\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"deallocated\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deallocateAdapters\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"delegator\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"deposit\",\"inputs\":[{\"name\":\"onBehalfOf\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"depositedAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"mintedShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"depositLimit\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"depositWhitelist\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"epochDuration\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"initialVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"instantWithdraw\",\"inputs\":[{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"withdrawnAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"burnedShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isDepositLimit\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isDepositorWhitelisted\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isInitialized\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isWithdrawalsClaimed\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"migrateTimestamp\",\"inputs\":[],\"outputs\":[{\"name\":\"migrateTimestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"redeem\",\"inputs\":[{\"name\":\"claimer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"shares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"withdrawnAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"mintedShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setAdapterLimit\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"limit\",\"type\":\"uint208\",\"internalType\":\"uint208\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositLimit\",\"inputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositWhitelist\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDepositorWhitelistStatus\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setIsDepositLimit\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"skimAdapters\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"slasher\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"swapAdapters\",\"inputs\":[{\"name\":\"adapter1\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"adapter2\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalStake\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"unclaimed\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdraw\",\"inputs\":[{\"name\":\"claimer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"burnedShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"mintedShares\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdrawalBucket\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint208\",\"internalType\":\"uint208\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawalShares\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawalSharesOf\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawalUnlockAt\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawals\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawalsOf\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawalsOfLength\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"Allocate\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Claim\",\"inputs\":[{\"name\":\"claimer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"recipient\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"index\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Deallocate\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Deposit\",\"inputs\":[{\"name\":\"depositor\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"onBehalfOf\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"shares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Donate\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialize\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structIVaultV2.InitParams\",\"components\":[{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"symbol\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"collateral\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"burner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"epochDuration\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"depositWhitelist\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"depositorToWhitelist\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"isDepositLimit\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"depositLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"defaultAdminRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositWhitelistSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositorWhitelistRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"isDepositLimitSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"depositLimitSetRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"setAdapterLimitRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swapAdaptersRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"allocateAdapterRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"deallocateAdapterRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"InstantWithdraw\",\"inputs\":[{\"name\":\"withdrawer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"burnedShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Migrate\",\"inputs\":[{\"name\":\"params\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structIVaultV2.MigrateParams\",\"components\":[{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"symbol\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"defaultAdminRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"setAdapterLimitRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"swapAdaptersRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"allocateAdapterRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"deallocateAdapterRoleHolder\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"delegatorParams\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"slasherParams\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"name\":\"newDelegator\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"newSlasher\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OnSlash\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"slashedAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetAdapterLimit\",\"inputs\":[{\"name\":\"adapter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"limit\",\"type\":\"uint208\",\"indexed\":false,\"internalType\":\"uint208\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDelegator\",\"inputs\":[{\"name\":\"delegator\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositLimit\",\"inputs\":[{\"name\":\"limit\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositWhitelist\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetDepositorWhitelistStatus\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetIsDepositLimit\",\"inputs\":[{\"name\":\"status\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetSlasher\",\"inputs\":[{\"name\":\"slasher\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SwapAdapters\",\"inputs\":[{\"name\":\"adapter1\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"adapter2\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SyncOwedSlash\",\"inputs\":[{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Withdraw\",\"inputs\":[{\"name\":\"withdrawer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"claimer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"burnedShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"mintedShares\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"index\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AdapterAllocated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyClaimed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DelegatorAlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DepositLimitReached\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"FeeOnTransferNotSupported\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientAmount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAddress\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidCollateral\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDelegator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDepositorToWhitelist\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSlasher\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTimestamp\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NoPreviousEpoch\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotAdapter\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotFactory\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotRewards\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotSlasher\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotWhitelistedDepositor\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SlasherAlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooLongDuration\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooManyAdapters\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooMuchRedeem\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooMuchWithdraw\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WithdrawalNotMatured\",\"inputs\":[]}]",
}

// IVaultV2ABI is the input ABI used to generate the binding from.
// Deprecated: Use IVaultV2MetaData.ABI instead.
var IVaultV2ABI = IVaultV2MetaData.ABI

// IVaultV2 is an auto generated Go binding around an Ethereum contract.
type IVaultV2 struct {
	IVaultV2Caller     // Read-only binding to the contract
	IVaultV2Transactor // Write-only binding to the contract
	IVaultV2Filterer   // Log filterer for contract events
}

// IVaultV2Caller is an auto generated read-only Go binding around an Ethereum contract.
type IVaultV2Caller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultV2Transactor is an auto generated write-only Go binding around an Ethereum contract.
type IVaultV2Transactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultV2Filterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IVaultV2Filterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IVaultV2Session is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IVaultV2Session struct {
	Contract     *IVaultV2         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IVaultV2CallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IVaultV2CallerSession struct {
	Contract *IVaultV2Caller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// IVaultV2TransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IVaultV2TransactorSession struct {
	Contract     *IVaultV2Transactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// IVaultV2Raw is an auto generated low-level Go binding around an Ethereum contract.
type IVaultV2Raw struct {
	Contract *IVaultV2 // Generic contract binding to access the raw methods on
}

// IVaultV2CallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IVaultV2CallerRaw struct {
	Contract *IVaultV2Caller // Generic read-only contract binding to access the raw methods on
}

// IVaultV2TransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IVaultV2TransactorRaw struct {
	Contract *IVaultV2Transactor // Generic write-only contract binding to access the raw methods on
}

// NewIVaultV2 creates a new instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2(address common.Address, backend bind.ContractBackend) (*IVaultV2, error) {
	contract, err := bindIVaultV2(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IVaultV2{IVaultV2Caller: IVaultV2Caller{contract: contract}, IVaultV2Transactor: IVaultV2Transactor{contract: contract}, IVaultV2Filterer: IVaultV2Filterer{contract: contract}}, nil
}

// NewIVaultV2Caller creates a new read-only instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2Caller(address common.Address, caller bind.ContractCaller) (*IVaultV2Caller, error) {
	contract, err := bindIVaultV2(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IVaultV2Caller{contract: contract}, nil
}

// NewIVaultV2Transactor creates a new write-only instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2Transactor(address common.Address, transactor bind.ContractTransactor) (*IVaultV2Transactor, error) {
	contract, err := bindIVaultV2(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IVaultV2Transactor{contract: contract}, nil
}

// NewIVaultV2Filterer creates a new log filterer instance of IVaultV2, bound to a specific deployed contract.
func NewIVaultV2Filterer(address common.Address, filterer bind.ContractFilterer) (*IVaultV2Filterer, error) {
	contract, err := bindIVaultV2(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IVaultV2Filterer{contract: contract}, nil
}

// bindIVaultV2 binds a generic wrapper to an already deployed contract.
func bindIVaultV2(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IVaultV2MetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IVaultV2 *IVaultV2Raw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IVaultV2.Contract.IVaultV2Caller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IVaultV2 *IVaultV2Raw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.Contract.IVaultV2Transactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IVaultV2 *IVaultV2Raw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IVaultV2.Contract.IVaultV2Transactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IVaultV2 *IVaultV2CallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IVaultV2.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IVaultV2 *IVaultV2TransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IVaultV2 *IVaultV2TransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IVaultV2.Contract.contract.Transact(opts, method, params...)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_IVaultV2 *IVaultV2Caller) FACTORY(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "FACTORY")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_IVaultV2 *IVaultV2Session) FACTORY() (common.Address, error) {
	return _IVaultV2.Contract.FACTORY(&_IVaultV2.CallOpts)
}

// FACTORY is a free data retrieval call binding the contract method 0x2dd31000.
//
// Solidity: function FACTORY() view returns(address)
func (_IVaultV2 *IVaultV2CallerSession) FACTORY() (common.Address, error) {
	return _IVaultV2.Contract.FACTORY(&_IVaultV2.CallOpts)
}

// ActiveBalanceOf is a free data retrieval call binding the contract method 0x59f769a9.
//
// Solidity: function activeBalanceOf(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveBalanceOf(opts *bind.CallOpts, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeBalanceOf", account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveBalanceOf is a free data retrieval call binding the contract method 0x59f769a9.
//
// Solidity: function activeBalanceOf(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveBalanceOf(account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveBalanceOf(&_IVaultV2.CallOpts, account)
}

// ActiveBalanceOf is a free data retrieval call binding the contract method 0x59f769a9.
//
// Solidity: function activeBalanceOf(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveBalanceOf(account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveBalanceOf(&_IVaultV2.CallOpts, account)
}

// ActiveBalanceOfAt is a free data retrieval call binding the contract method 0xefb559d6.
//
// Solidity: function activeBalanceOfAt(address account, uint48 timestamp, bytes ) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveBalanceOfAt(opts *bind.CallOpts, account common.Address, timestamp *big.Int, arg2 []byte) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeBalanceOfAt", account, timestamp, arg2)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveBalanceOfAt is a free data retrieval call binding the contract method 0xefb559d6.
//
// Solidity: function activeBalanceOfAt(address account, uint48 timestamp, bytes ) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveBalanceOfAt(account common.Address, timestamp *big.Int, arg2 []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveBalanceOfAt(&_IVaultV2.CallOpts, account, timestamp, arg2)
}

// ActiveBalanceOfAt is a free data retrieval call binding the contract method 0xefb559d6.
//
// Solidity: function activeBalanceOfAt(address account, uint48 timestamp, bytes ) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveBalanceOfAt(account common.Address, timestamp *big.Int, arg2 []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveBalanceOfAt(&_IVaultV2.CallOpts, account, timestamp, arg2)
}

// ActiveShares is a free data retrieval call binding the contract method 0xbfefcd7b.
//
// Solidity: function activeShares() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveShares(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeShares")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveShares is a free data retrieval call binding the contract method 0xbfefcd7b.
//
// Solidity: function activeShares() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveShares() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveShares(&_IVaultV2.CallOpts)
}

// ActiveShares is a free data retrieval call binding the contract method 0xbfefcd7b.
//
// Solidity: function activeShares() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveShares() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveShares(&_IVaultV2.CallOpts)
}

// ActiveSharesAt is a free data retrieval call binding the contract method 0x50f22068.
//
// Solidity: function activeSharesAt(uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveSharesAt(opts *bind.CallOpts, timestamp *big.Int, hint []byte) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeSharesAt", timestamp, hint)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveSharesAt is a free data retrieval call binding the contract method 0x50f22068.
//
// Solidity: function activeSharesAt(uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveSharesAt(timestamp *big.Int, hint []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveSharesAt(&_IVaultV2.CallOpts, timestamp, hint)
}

// ActiveSharesAt is a free data retrieval call binding the contract method 0x50f22068.
//
// Solidity: function activeSharesAt(uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveSharesAt(timestamp *big.Int, hint []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveSharesAt(&_IVaultV2.CallOpts, timestamp, hint)
}

// ActiveSharesOf is a free data retrieval call binding the contract method 0x9d66201b.
//
// Solidity: function activeSharesOf(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveSharesOf(opts *bind.CallOpts, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeSharesOf", account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveSharesOf is a free data retrieval call binding the contract method 0x9d66201b.
//
// Solidity: function activeSharesOf(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveSharesOf(account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveSharesOf(&_IVaultV2.CallOpts, account)
}

// ActiveSharesOf is a free data retrieval call binding the contract method 0x9d66201b.
//
// Solidity: function activeSharesOf(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveSharesOf(account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveSharesOf(&_IVaultV2.CallOpts, account)
}

// ActiveSharesOfAt is a free data retrieval call binding the contract method 0x2d73c69c.
//
// Solidity: function activeSharesOfAt(address account, uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveSharesOfAt(opts *bind.CallOpts, account common.Address, timestamp *big.Int, hint []byte) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeSharesOfAt", account, timestamp, hint)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveSharesOfAt is a free data retrieval call binding the contract method 0x2d73c69c.
//
// Solidity: function activeSharesOfAt(address account, uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveSharesOfAt(account common.Address, timestamp *big.Int, hint []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveSharesOfAt(&_IVaultV2.CallOpts, account, timestamp, hint)
}

// ActiveSharesOfAt is a free data retrieval call binding the contract method 0x2d73c69c.
//
// Solidity: function activeSharesOfAt(address account, uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveSharesOfAt(account common.Address, timestamp *big.Int, hint []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveSharesOfAt(&_IVaultV2.CallOpts, account, timestamp, hint)
}

// ActiveStake is a free data retrieval call binding the contract method 0xbd49c35f.
//
// Solidity: function activeStake() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveStake(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeStake")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveStake is a free data retrieval call binding the contract method 0xbd49c35f.
//
// Solidity: function activeStake() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveStake() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveStake(&_IVaultV2.CallOpts)
}

// ActiveStake is a free data retrieval call binding the contract method 0xbd49c35f.
//
// Solidity: function activeStake() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveStake() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveStake(&_IVaultV2.CallOpts)
}

// ActiveStakeAt is a free data retrieval call binding the contract method 0x810da75d.
//
// Solidity: function activeStakeAt(uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveStakeAt(opts *bind.CallOpts, timestamp *big.Int, hint []byte) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeStakeAt", timestamp, hint)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveStakeAt is a free data retrieval call binding the contract method 0x810da75d.
//
// Solidity: function activeStakeAt(uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveStakeAt(timestamp *big.Int, hint []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveStakeAt(&_IVaultV2.CallOpts, timestamp, hint)
}

// ActiveStakeAt is a free data retrieval call binding the contract method 0x810da75d.
//
// Solidity: function activeStakeAt(uint48 timestamp, bytes hint) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveStakeAt(timestamp *big.Int, hint []byte) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveStakeAt(&_IVaultV2.CallOpts, timestamp, hint)
}

// ActiveWithdrawalShares is a free data retrieval call binding the contract method 0x49578b7b.
//
// Solidity: function activeWithdrawalShares() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalShares(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalShares")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalShares is a free data retrieval call binding the contract method 0x49578b7b.
//
// Solidity: function activeWithdrawalShares() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalShares() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalShares(&_IVaultV2.CallOpts)
}

// ActiveWithdrawalShares is a free data retrieval call binding the contract method 0x49578b7b.
//
// Solidity: function activeWithdrawalShares() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalShares() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalShares(&_IVaultV2.CallOpts)
}

// ActiveWithdrawalSharesAt is a free data retrieval call binding the contract method 0x13e0f932.
//
// Solidity: function activeWithdrawalSharesAt(uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalSharesAt(opts *bind.CallOpts, timestamp *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalSharesAt", timestamp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalSharesAt is a free data retrieval call binding the contract method 0x13e0f932.
//
// Solidity: function activeWithdrawalSharesAt(uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalSharesAt(timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesAt(&_IVaultV2.CallOpts, timestamp)
}

// ActiveWithdrawalSharesAt is a free data retrieval call binding the contract method 0x13e0f932.
//
// Solidity: function activeWithdrawalSharesAt(uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalSharesAt(timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesAt(&_IVaultV2.CallOpts, timestamp)
}

// ActiveWithdrawalSharesFor is a free data retrieval call binding the contract method 0xe9f28b01.
//
// Solidity: function activeWithdrawalSharesFor(uint48 duration) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalSharesFor(opts *bind.CallOpts, duration *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalSharesFor", duration)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalSharesFor is a free data retrieval call binding the contract method 0xe9f28b01.
//
// Solidity: function activeWithdrawalSharesFor(uint48 duration) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalSharesFor(duration *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesFor(&_IVaultV2.CallOpts, duration)
}

// ActiveWithdrawalSharesFor is a free data retrieval call binding the contract method 0xe9f28b01.
//
// Solidity: function activeWithdrawalSharesFor(uint48 duration) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalSharesFor(duration *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesFor(&_IVaultV2.CallOpts, duration)
}

// ActiveWithdrawalSharesForAt is a free data retrieval call binding the contract method 0x204ff5ab.
//
// Solidity: function activeWithdrawalSharesForAt(uint48 duration, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalSharesForAt(opts *bind.CallOpts, duration *big.Int, timestamp *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalSharesForAt", duration, timestamp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalSharesForAt is a free data retrieval call binding the contract method 0x204ff5ab.
//
// Solidity: function activeWithdrawalSharesForAt(uint48 duration, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalSharesForAt(duration *big.Int, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesForAt(&_IVaultV2.CallOpts, duration, timestamp)
}

// ActiveWithdrawalSharesForAt is a free data retrieval call binding the contract method 0x204ff5ab.
//
// Solidity: function activeWithdrawalSharesForAt(uint48 duration, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalSharesForAt(duration *big.Int, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesForAt(&_IVaultV2.CallOpts, duration, timestamp)
}

// ActiveWithdrawalSharesOfAt is a free data retrieval call binding the contract method 0xb039876a.
//
// Solidity: function activeWithdrawalSharesOfAt(address account, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalSharesOfAt(opts *bind.CallOpts, account common.Address, timestamp *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalSharesOfAt", account, timestamp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalSharesOfAt is a free data retrieval call binding the contract method 0xb039876a.
//
// Solidity: function activeWithdrawalSharesOfAt(address account, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalSharesOfAt(account common.Address, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesOfAt(&_IVaultV2.CallOpts, account, timestamp)
}

// ActiveWithdrawalSharesOfAt is a free data retrieval call binding the contract method 0xb039876a.
//
// Solidity: function activeWithdrawalSharesOfAt(address account, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalSharesOfAt(account common.Address, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalSharesOfAt(&_IVaultV2.CallOpts, account, timestamp)
}

// ActiveWithdrawals is a free data retrieval call binding the contract method 0xca0aabbb.
//
// Solidity: function activeWithdrawals() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawals(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawals")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawals is a free data retrieval call binding the contract method 0xca0aabbb.
//
// Solidity: function activeWithdrawals() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawals() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawals(&_IVaultV2.CallOpts)
}

// ActiveWithdrawals is a free data retrieval call binding the contract method 0xca0aabbb.
//
// Solidity: function activeWithdrawals() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawals() (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawals(&_IVaultV2.CallOpts)
}

// ActiveWithdrawalsAt is a free data retrieval call binding the contract method 0xaee9d015.
//
// Solidity: function activeWithdrawalsAt(uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalsAt(opts *bind.CallOpts, timestamp *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalsAt", timestamp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalsAt is a free data retrieval call binding the contract method 0xaee9d015.
//
// Solidity: function activeWithdrawalsAt(uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalsAt(timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalsAt(&_IVaultV2.CallOpts, timestamp)
}

// ActiveWithdrawalsAt is a free data retrieval call binding the contract method 0xaee9d015.
//
// Solidity: function activeWithdrawalsAt(uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalsAt(timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalsAt(&_IVaultV2.CallOpts, timestamp)
}

// ActiveWithdrawalsFor is a free data retrieval call binding the contract method 0xb90c4fdb.
//
// Solidity: function activeWithdrawalsFor(uint48 duration) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalsFor(opts *bind.CallOpts, duration *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalsFor", duration)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalsFor is a free data retrieval call binding the contract method 0xb90c4fdb.
//
// Solidity: function activeWithdrawalsFor(uint48 duration) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalsFor(duration *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalsFor(&_IVaultV2.CallOpts, duration)
}

// ActiveWithdrawalsFor is a free data retrieval call binding the contract method 0xb90c4fdb.
//
// Solidity: function activeWithdrawalsFor(uint48 duration) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalsFor(duration *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalsFor(&_IVaultV2.CallOpts, duration)
}

// ActiveWithdrawalsForAt is a free data retrieval call binding the contract method 0x3dd14288.
//
// Solidity: function activeWithdrawalsForAt(uint48 duration, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) ActiveWithdrawalsForAt(opts *bind.CallOpts, duration *big.Int, timestamp *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "activeWithdrawalsForAt", duration, timestamp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveWithdrawalsForAt is a free data retrieval call binding the contract method 0x3dd14288.
//
// Solidity: function activeWithdrawalsForAt(uint48 duration, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) ActiveWithdrawalsForAt(duration *big.Int, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalsForAt(&_IVaultV2.CallOpts, duration, timestamp)
}

// ActiveWithdrawalsForAt is a free data retrieval call binding the contract method 0x3dd14288.
//
// Solidity: function activeWithdrawalsForAt(uint48 duration, uint48 timestamp) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) ActiveWithdrawalsForAt(duration *big.Int, timestamp *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.ActiveWithdrawalsForAt(&_IVaultV2.CallOpts, duration, timestamp)
}

// AdapterAllocated is a free data retrieval call binding the contract method 0xd06a84ba.
//
// Solidity: function adapterAllocated(address adapter) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) AdapterAllocated(opts *bind.CallOpts, adapter common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "adapterAllocated", adapter)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AdapterAllocated is a free data retrieval call binding the contract method 0xd06a84ba.
//
// Solidity: function adapterAllocated(address adapter) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) AdapterAllocated(adapter common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.AdapterAllocated(&_IVaultV2.CallOpts, adapter)
}

// AdapterAllocated is a free data retrieval call binding the contract method 0xd06a84ba.
//
// Solidity: function adapterAllocated(address adapter) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) AdapterAllocated(adapter common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.AdapterAllocated(&_IVaultV2.CallOpts, adapter)
}

// AdapterLimit is a free data retrieval call binding the contract method 0x6df6f7ab.
//
// Solidity: function adapterLimit(address adapter) view returns(uint208)
func (_IVaultV2 *IVaultV2Caller) AdapterLimit(opts *bind.CallOpts, adapter common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "adapterLimit", adapter)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AdapterLimit is a free data retrieval call binding the contract method 0x6df6f7ab.
//
// Solidity: function adapterLimit(address adapter) view returns(uint208)
func (_IVaultV2 *IVaultV2Session) AdapterLimit(adapter common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.AdapterLimit(&_IVaultV2.CallOpts, adapter)
}

// AdapterLimit is a free data retrieval call binding the contract method 0x6df6f7ab.
//
// Solidity: function adapterLimit(address adapter) view returns(uint208)
func (_IVaultV2 *IVaultV2CallerSession) AdapterLimit(adapter common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.AdapterLimit(&_IVaultV2.CallOpts, adapter)
}

// Adapters is a free data retrieval call binding the contract method 0x4ef501ac.
//
// Solidity: function adapters(uint256 index) view returns(address)
func (_IVaultV2 *IVaultV2Caller) Adapters(opts *bind.CallOpts, index *big.Int) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "adapters", index)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Adapters is a free data retrieval call binding the contract method 0x4ef501ac.
//
// Solidity: function adapters(uint256 index) view returns(address)
func (_IVaultV2 *IVaultV2Session) Adapters(index *big.Int) (common.Address, error) {
	return _IVaultV2.Contract.Adapters(&_IVaultV2.CallOpts, index)
}

// Adapters is a free data retrieval call binding the contract method 0x4ef501ac.
//
// Solidity: function adapters(uint256 index) view returns(address)
func (_IVaultV2 *IVaultV2CallerSession) Adapters(index *big.Int) (common.Address, error) {
	return _IVaultV2.Contract.Adapters(&_IVaultV2.CallOpts, index)
}

// AdaptersAllocated is a free data retrieval call binding the contract method 0xa8e7f264.
//
// Solidity: function adaptersAllocated() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) AdaptersAllocated(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "adaptersAllocated")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AdaptersAllocated is a free data retrieval call binding the contract method 0xa8e7f264.
//
// Solidity: function adaptersAllocated() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) AdaptersAllocated() (*big.Int, error) {
	return _IVaultV2.Contract.AdaptersAllocated(&_IVaultV2.CallOpts)
}

// AdaptersAllocated is a free data retrieval call binding the contract method 0xa8e7f264.
//
// Solidity: function adaptersAllocated() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) AdaptersAllocated() (*big.Int, error) {
	return _IVaultV2.Contract.AdaptersAllocated(&_IVaultV2.CallOpts)
}

// AdaptersLength is a free data retrieval call binding the contract method 0x5aa22bc8.
//
// Solidity: function adaptersLength() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) AdaptersLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "adaptersLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AdaptersLength is a free data retrieval call binding the contract method 0x5aa22bc8.
//
// Solidity: function adaptersLength() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) AdaptersLength() (*big.Int, error) {
	return _IVaultV2.Contract.AdaptersLength(&_IVaultV2.CallOpts)
}

// AdaptersLength is a free data retrieval call binding the contract method 0x5aa22bc8.
//
// Solidity: function adaptersLength() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) AdaptersLength() (*big.Int, error) {
	return _IVaultV2.Contract.AdaptersLength(&_IVaultV2.CallOpts)
}

// AdaptersOwe is a free data retrieval call binding the contract method 0x44e70b33.
//
// Solidity: function adaptersOwe() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) AdaptersOwe(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "adaptersOwe")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AdaptersOwe is a free data retrieval call binding the contract method 0x44e70b33.
//
// Solidity: function adaptersOwe() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) AdaptersOwe() (*big.Int, error) {
	return _IVaultV2.Contract.AdaptersOwe(&_IVaultV2.CallOpts)
}

// AdaptersOwe is a free data retrieval call binding the contract method 0x44e70b33.
//
// Solidity: function adaptersOwe() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) AdaptersOwe() (*big.Int, error) {
	return _IVaultV2.Contract.AdaptersOwe(&_IVaultV2.CallOpts)
}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) Allocatable(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "allocatable")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) Allocatable() (*big.Int, error) {
	return _IVaultV2.Contract.Allocatable(&_IVaultV2.CallOpts)
}

// Allocatable is a free data retrieval call binding the contract method 0x1d3b809a.
//
// Solidity: function allocatable() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) Allocatable() (*big.Int, error) {
	return _IVaultV2.Contract.Allocatable(&_IVaultV2.CallOpts)
}

// Burner is a free data retrieval call binding the contract method 0x27810b6e.
//
// Solidity: function burner() view returns(address)
func (_IVaultV2 *IVaultV2Caller) Burner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "burner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Burner is a free data retrieval call binding the contract method 0x27810b6e.
//
// Solidity: function burner() view returns(address)
func (_IVaultV2 *IVaultV2Session) Burner() (common.Address, error) {
	return _IVaultV2.Contract.Burner(&_IVaultV2.CallOpts)
}

// Burner is a free data retrieval call binding the contract method 0x27810b6e.
//
// Solidity: function burner() view returns(address)
func (_IVaultV2 *IVaultV2CallerSession) Burner() (common.Address, error) {
	return _IVaultV2.Contract.Burner(&_IVaultV2.CallOpts)
}

// Collateral is a free data retrieval call binding the contract method 0xd8dfeb45.
//
// Solidity: function collateral() view returns(address)
func (_IVaultV2 *IVaultV2Caller) Collateral(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "collateral")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Collateral is a free data retrieval call binding the contract method 0xd8dfeb45.
//
// Solidity: function collateral() view returns(address)
func (_IVaultV2 *IVaultV2Session) Collateral() (common.Address, error) {
	return _IVaultV2.Contract.Collateral(&_IVaultV2.CallOpts)
}

// Collateral is a free data retrieval call binding the contract method 0xd8dfeb45.
//
// Solidity: function collateral() view returns(address)
func (_IVaultV2 *IVaultV2CallerSession) Collateral() (common.Address, error) {
	return _IVaultV2.Contract.Collateral(&_IVaultV2.CallOpts)
}

// Delegator is a free data retrieval call binding the contract method 0xce9b7930.
//
// Solidity: function delegator() view returns(address)
func (_IVaultV2 *IVaultV2Caller) Delegator(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "delegator")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Delegator is a free data retrieval call binding the contract method 0xce9b7930.
//
// Solidity: function delegator() view returns(address)
func (_IVaultV2 *IVaultV2Session) Delegator() (common.Address, error) {
	return _IVaultV2.Contract.Delegator(&_IVaultV2.CallOpts)
}

// Delegator is a free data retrieval call binding the contract method 0xce9b7930.
//
// Solidity: function delegator() view returns(address)
func (_IVaultV2 *IVaultV2CallerSession) Delegator() (common.Address, error) {
	return _IVaultV2.Contract.Delegator(&_IVaultV2.CallOpts)
}

// DepositLimit is a free data retrieval call binding the contract method 0xecf70858.
//
// Solidity: function depositLimit() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) DepositLimit(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "depositLimit")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// DepositLimit is a free data retrieval call binding the contract method 0xecf70858.
//
// Solidity: function depositLimit() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) DepositLimit() (*big.Int, error) {
	return _IVaultV2.Contract.DepositLimit(&_IVaultV2.CallOpts)
}

// DepositLimit is a free data retrieval call binding the contract method 0xecf70858.
//
// Solidity: function depositLimit() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) DepositLimit() (*big.Int, error) {
	return _IVaultV2.Contract.DepositLimit(&_IVaultV2.CallOpts)
}

// DepositWhitelist is a free data retrieval call binding the contract method 0x48d3b775.
//
// Solidity: function depositWhitelist() view returns(bool)
func (_IVaultV2 *IVaultV2Caller) DepositWhitelist(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "depositWhitelist")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// DepositWhitelist is a free data retrieval call binding the contract method 0x48d3b775.
//
// Solidity: function depositWhitelist() view returns(bool)
func (_IVaultV2 *IVaultV2Session) DepositWhitelist() (bool, error) {
	return _IVaultV2.Contract.DepositWhitelist(&_IVaultV2.CallOpts)
}

// DepositWhitelist is a free data retrieval call binding the contract method 0x48d3b775.
//
// Solidity: function depositWhitelist() view returns(bool)
func (_IVaultV2 *IVaultV2CallerSession) DepositWhitelist() (bool, error) {
	return _IVaultV2.Contract.DepositWhitelist(&_IVaultV2.CallOpts)
}

// EpochDuration is a free data retrieval call binding the contract method 0x4ff0876a.
//
// Solidity: function epochDuration() view returns(uint48)
func (_IVaultV2 *IVaultV2Caller) EpochDuration(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "epochDuration")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// EpochDuration is a free data retrieval call binding the contract method 0x4ff0876a.
//
// Solidity: function epochDuration() view returns(uint48)
func (_IVaultV2 *IVaultV2Session) EpochDuration() (*big.Int, error) {
	return _IVaultV2.Contract.EpochDuration(&_IVaultV2.CallOpts)
}

// EpochDuration is a free data retrieval call binding the contract method 0x4ff0876a.
//
// Solidity: function epochDuration() view returns(uint48)
func (_IVaultV2 *IVaultV2CallerSession) EpochDuration() (*big.Int, error) {
	return _IVaultV2.Contract.EpochDuration(&_IVaultV2.CallOpts)
}

// IsDepositLimit is a free data retrieval call binding the contract method 0xa1b12202.
//
// Solidity: function isDepositLimit() view returns(bool)
func (_IVaultV2 *IVaultV2Caller) IsDepositLimit(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "isDepositLimit")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsDepositLimit is a free data retrieval call binding the contract method 0xa1b12202.
//
// Solidity: function isDepositLimit() view returns(bool)
func (_IVaultV2 *IVaultV2Session) IsDepositLimit() (bool, error) {
	return _IVaultV2.Contract.IsDepositLimit(&_IVaultV2.CallOpts)
}

// IsDepositLimit is a free data retrieval call binding the contract method 0xa1b12202.
//
// Solidity: function isDepositLimit() view returns(bool)
func (_IVaultV2 *IVaultV2CallerSession) IsDepositLimit() (bool, error) {
	return _IVaultV2.Contract.IsDepositLimit(&_IVaultV2.CallOpts)
}

// IsDepositorWhitelisted is a free data retrieval call binding the contract method 0x794b15b7.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool)
func (_IVaultV2 *IVaultV2Caller) IsDepositorWhitelisted(opts *bind.CallOpts, account common.Address) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "isDepositorWhitelisted", account)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsDepositorWhitelisted is a free data retrieval call binding the contract method 0x794b15b7.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool)
func (_IVaultV2 *IVaultV2Session) IsDepositorWhitelisted(account common.Address) (bool, error) {
	return _IVaultV2.Contract.IsDepositorWhitelisted(&_IVaultV2.CallOpts, account)
}

// IsDepositorWhitelisted is a free data retrieval call binding the contract method 0x794b15b7.
//
// Solidity: function isDepositorWhitelisted(address account) view returns(bool)
func (_IVaultV2 *IVaultV2CallerSession) IsDepositorWhitelisted(account common.Address) (bool, error) {
	return _IVaultV2.Contract.IsDepositorWhitelisted(&_IVaultV2.CallOpts, account)
}

// IsInitialized is a free data retrieval call binding the contract method 0x392e53cd.
//
// Solidity: function isInitialized() view returns(bool)
func (_IVaultV2 *IVaultV2Caller) IsInitialized(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "isInitialized")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsInitialized is a free data retrieval call binding the contract method 0x392e53cd.
//
// Solidity: function isInitialized() view returns(bool)
func (_IVaultV2 *IVaultV2Session) IsInitialized() (bool, error) {
	return _IVaultV2.Contract.IsInitialized(&_IVaultV2.CallOpts)
}

// IsInitialized is a free data retrieval call binding the contract method 0x392e53cd.
//
// Solidity: function isInitialized() view returns(bool)
func (_IVaultV2 *IVaultV2CallerSession) IsInitialized() (bool, error) {
	return _IVaultV2.Contract.IsInitialized(&_IVaultV2.CallOpts)
}

// IsWithdrawalsClaimed is a free data retrieval call binding the contract method 0xa5d03223.
//
// Solidity: function isWithdrawalsClaimed(uint256 index, address account) view returns(bool)
func (_IVaultV2 *IVaultV2Caller) IsWithdrawalsClaimed(opts *bind.CallOpts, index *big.Int, account common.Address) (bool, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "isWithdrawalsClaimed", index, account)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsWithdrawalsClaimed is a free data retrieval call binding the contract method 0xa5d03223.
//
// Solidity: function isWithdrawalsClaimed(uint256 index, address account) view returns(bool)
func (_IVaultV2 *IVaultV2Session) IsWithdrawalsClaimed(index *big.Int, account common.Address) (bool, error) {
	return _IVaultV2.Contract.IsWithdrawalsClaimed(&_IVaultV2.CallOpts, index, account)
}

// IsWithdrawalsClaimed is a free data retrieval call binding the contract method 0xa5d03223.
//
// Solidity: function isWithdrawalsClaimed(uint256 index, address account) view returns(bool)
func (_IVaultV2 *IVaultV2CallerSession) IsWithdrawalsClaimed(index *big.Int, account common.Address) (bool, error) {
	return _IVaultV2.Contract.IsWithdrawalsClaimed(&_IVaultV2.CallOpts, index, account)
}

// MigrateTimestamp is a free data retrieval call binding the contract method 0x8a605ccd.
//
// Solidity: function migrateTimestamp() view returns(uint48 migrateTimestamp)
func (_IVaultV2 *IVaultV2Caller) MigrateTimestamp(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "migrateTimestamp")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MigrateTimestamp is a free data retrieval call binding the contract method 0x8a605ccd.
//
// Solidity: function migrateTimestamp() view returns(uint48 migrateTimestamp)
func (_IVaultV2 *IVaultV2Session) MigrateTimestamp() (*big.Int, error) {
	return _IVaultV2.Contract.MigrateTimestamp(&_IVaultV2.CallOpts)
}

// MigrateTimestamp is a free data retrieval call binding the contract method 0x8a605ccd.
//
// Solidity: function migrateTimestamp() view returns(uint48 migrateTimestamp)
func (_IVaultV2 *IVaultV2CallerSession) MigrateTimestamp() (*big.Int, error) {
	return _IVaultV2.Contract.MigrateTimestamp(&_IVaultV2.CallOpts)
}

// Slasher is a free data retrieval call binding the contract method 0xb1344271.
//
// Solidity: function slasher() view returns(address)
func (_IVaultV2 *IVaultV2Caller) Slasher(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "slasher")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Slasher is a free data retrieval call binding the contract method 0xb1344271.
//
// Solidity: function slasher() view returns(address)
func (_IVaultV2 *IVaultV2Session) Slasher() (common.Address, error) {
	return _IVaultV2.Contract.Slasher(&_IVaultV2.CallOpts)
}

// Slasher is a free data retrieval call binding the contract method 0xb1344271.
//
// Solidity: function slasher() view returns(address)
func (_IVaultV2 *IVaultV2CallerSession) Slasher() (common.Address, error) {
	return _IVaultV2.Contract.Slasher(&_IVaultV2.CallOpts)
}

// TotalStake is a free data retrieval call binding the contract method 0x8b0e9f3f.
//
// Solidity: function totalStake() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) TotalStake(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "totalStake")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalStake is a free data retrieval call binding the contract method 0x8b0e9f3f.
//
// Solidity: function totalStake() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) TotalStake() (*big.Int, error) {
	return _IVaultV2.Contract.TotalStake(&_IVaultV2.CallOpts)
}

// TotalStake is a free data retrieval call binding the contract method 0x8b0e9f3f.
//
// Solidity: function totalStake() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) TotalStake() (*big.Int, error) {
	return _IVaultV2.Contract.TotalStake(&_IVaultV2.CallOpts)
}

// Unclaimed is a free data retrieval call binding the contract method 0x669416b8.
//
// Solidity: function unclaimed() view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) Unclaimed(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "unclaimed")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Unclaimed is a free data retrieval call binding the contract method 0x669416b8.
//
// Solidity: function unclaimed() view returns(uint256)
func (_IVaultV2 *IVaultV2Session) Unclaimed() (*big.Int, error) {
	return _IVaultV2.Contract.Unclaimed(&_IVaultV2.CallOpts)
}

// Unclaimed is a free data retrieval call binding the contract method 0x669416b8.
//
// Solidity: function unclaimed() view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) Unclaimed() (*big.Int, error) {
	return _IVaultV2.Contract.Unclaimed(&_IVaultV2.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_IVaultV2 *IVaultV2Caller) Version(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "version")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_IVaultV2 *IVaultV2Session) Version() (uint64, error) {
	return _IVaultV2.Contract.Version(&_IVaultV2.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64)
func (_IVaultV2 *IVaultV2CallerSession) Version() (uint64, error) {
	return _IVaultV2.Contract.Version(&_IVaultV2.CallOpts)
}

// WithdrawalBucket is a free data retrieval call binding the contract method 0x639fa397.
//
// Solidity: function withdrawalBucket() view returns(uint208)
func (_IVaultV2 *IVaultV2Caller) WithdrawalBucket(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawalBucket")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// WithdrawalBucket is a free data retrieval call binding the contract method 0x639fa397.
//
// Solidity: function withdrawalBucket() view returns(uint208)
func (_IVaultV2 *IVaultV2Session) WithdrawalBucket() (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalBucket(&_IVaultV2.CallOpts)
}

// WithdrawalBucket is a free data retrieval call binding the contract method 0x639fa397.
//
// Solidity: function withdrawalBucket() view returns(uint208)
func (_IVaultV2 *IVaultV2CallerSession) WithdrawalBucket() (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalBucket(&_IVaultV2.CallOpts)
}

// WithdrawalShares is a free data retrieval call binding the contract method 0xafba70ad.
//
// Solidity: function withdrawalShares(uint256 index) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) WithdrawalShares(opts *bind.CallOpts, index *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawalShares", index)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// WithdrawalShares is a free data retrieval call binding the contract method 0xafba70ad.
//
// Solidity: function withdrawalShares(uint256 index) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) WithdrawalShares(index *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalShares(&_IVaultV2.CallOpts, index)
}

// WithdrawalShares is a free data retrieval call binding the contract method 0xafba70ad.
//
// Solidity: function withdrawalShares(uint256 index) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) WithdrawalShares(index *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalShares(&_IVaultV2.CallOpts, index)
}

// WithdrawalSharesOf is a free data retrieval call binding the contract method 0xa3b54172.
//
// Solidity: function withdrawalSharesOf(uint256 index, address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) WithdrawalSharesOf(opts *bind.CallOpts, index *big.Int, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawalSharesOf", index, account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// WithdrawalSharesOf is a free data retrieval call binding the contract method 0xa3b54172.
//
// Solidity: function withdrawalSharesOf(uint256 index, address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) WithdrawalSharesOf(index *big.Int, account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalSharesOf(&_IVaultV2.CallOpts, index, account)
}

// WithdrawalSharesOf is a free data retrieval call binding the contract method 0xa3b54172.
//
// Solidity: function withdrawalSharesOf(uint256 index, address account) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) WithdrawalSharesOf(index *big.Int, account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalSharesOf(&_IVaultV2.CallOpts, index, account)
}

// WithdrawalUnlockAt is a free data retrieval call binding the contract method 0xf7eee3ec.
//
// Solidity: function withdrawalUnlockAt(uint256 index, address account) view returns(uint48)
func (_IVaultV2 *IVaultV2Caller) WithdrawalUnlockAt(opts *bind.CallOpts, index *big.Int, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawalUnlockAt", index, account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// WithdrawalUnlockAt is a free data retrieval call binding the contract method 0xf7eee3ec.
//
// Solidity: function withdrawalUnlockAt(uint256 index, address account) view returns(uint48)
func (_IVaultV2 *IVaultV2Session) WithdrawalUnlockAt(index *big.Int, account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalUnlockAt(&_IVaultV2.CallOpts, index, account)
}

// WithdrawalUnlockAt is a free data retrieval call binding the contract method 0xf7eee3ec.
//
// Solidity: function withdrawalUnlockAt(uint256 index, address account) view returns(uint48)
func (_IVaultV2 *IVaultV2CallerSession) WithdrawalUnlockAt(index *big.Int, account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalUnlockAt(&_IVaultV2.CallOpts, index, account)
}

// Withdrawals is a free data retrieval call binding the contract method 0x5cc07076.
//
// Solidity: function withdrawals(uint256 index) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) Withdrawals(opts *bind.CallOpts, index *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawals", index)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Withdrawals is a free data retrieval call binding the contract method 0x5cc07076.
//
// Solidity: function withdrawals(uint256 index) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) Withdrawals(index *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.Withdrawals(&_IVaultV2.CallOpts, index)
}

// Withdrawals is a free data retrieval call binding the contract method 0x5cc07076.
//
// Solidity: function withdrawals(uint256 index) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) Withdrawals(index *big.Int) (*big.Int, error) {
	return _IVaultV2.Contract.Withdrawals(&_IVaultV2.CallOpts, index)
}

// WithdrawalsOf is a free data retrieval call binding the contract method 0xf5e7ee0f.
//
// Solidity: function withdrawalsOf(uint256 index, address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) WithdrawalsOf(opts *bind.CallOpts, index *big.Int, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawalsOf", index, account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// WithdrawalsOf is a free data retrieval call binding the contract method 0xf5e7ee0f.
//
// Solidity: function withdrawalsOf(uint256 index, address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) WithdrawalsOf(index *big.Int, account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalsOf(&_IVaultV2.CallOpts, index, account)
}

// WithdrawalsOf is a free data retrieval call binding the contract method 0xf5e7ee0f.
//
// Solidity: function withdrawalsOf(uint256 index, address account) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) WithdrawalsOf(index *big.Int, account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalsOf(&_IVaultV2.CallOpts, index, account)
}

// WithdrawalsOfLength is a free data retrieval call binding the contract method 0x71a93932.
//
// Solidity: function withdrawalsOfLength(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Caller) WithdrawalsOfLength(opts *bind.CallOpts, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IVaultV2.contract.Call(opts, &out, "withdrawalsOfLength", account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// WithdrawalsOfLength is a free data retrieval call binding the contract method 0x71a93932.
//
// Solidity: function withdrawalsOfLength(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2Session) WithdrawalsOfLength(account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalsOfLength(&_IVaultV2.CallOpts, account)
}

// WithdrawalsOfLength is a free data retrieval call binding the contract method 0x71a93932.
//
// Solidity: function withdrawalsOfLength(address account) view returns(uint256)
func (_IVaultV2 *IVaultV2CallerSession) WithdrawalsOfLength(account common.Address) (*big.Int, error) {
	return _IVaultV2.Contract.WithdrawalsOfLength(&_IVaultV2.CallOpts, account)
}

// AllocateAdapter is a paid mutator transaction binding the contract method 0xd7fab410.
//
// Solidity: function allocateAdapter(address adapter, uint256 amount) returns(uint256 allocated)
func (_IVaultV2 *IVaultV2Transactor) AllocateAdapter(opts *bind.TransactOpts, adapter common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "allocateAdapter", adapter, amount)
}

// AllocateAdapter is a paid mutator transaction binding the contract method 0xd7fab410.
//
// Solidity: function allocateAdapter(address adapter, uint256 amount) returns(uint256 allocated)
func (_IVaultV2 *IVaultV2Session) AllocateAdapter(adapter common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.AllocateAdapter(&_IVaultV2.TransactOpts, adapter, amount)
}

// AllocateAdapter is a paid mutator transaction binding the contract method 0xd7fab410.
//
// Solidity: function allocateAdapter(address adapter, uint256 amount) returns(uint256 allocated)
func (_IVaultV2 *IVaultV2TransactorSession) AllocateAdapter(adapter common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.AllocateAdapter(&_IVaultV2.TransactOpts, adapter, amount)
}

// Claim is a paid mutator transaction binding the contract method 0xaad3ec96.
//
// Solidity: function claim(address recipient, uint256 index) returns(uint256 amount)
func (_IVaultV2 *IVaultV2Transactor) Claim(opts *bind.TransactOpts, recipient common.Address, index *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "claim", recipient, index)
}

// Claim is a paid mutator transaction binding the contract method 0xaad3ec96.
//
// Solidity: function claim(address recipient, uint256 index) returns(uint256 amount)
func (_IVaultV2 *IVaultV2Session) Claim(recipient common.Address, index *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Claim(&_IVaultV2.TransactOpts, recipient, index)
}

// Claim is a paid mutator transaction binding the contract method 0xaad3ec96.
//
// Solidity: function claim(address recipient, uint256 index) returns(uint256 amount)
func (_IVaultV2 *IVaultV2TransactorSession) Claim(recipient common.Address, index *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Claim(&_IVaultV2.TransactOpts, recipient, index)
}

// ClaimBatch is a paid mutator transaction binding the contract method 0x7c04c80a.
//
// Solidity: function claimBatch(address recipient, uint256[] indexes) returns(uint256 amount)
func (_IVaultV2 *IVaultV2Transactor) ClaimBatch(opts *bind.TransactOpts, recipient common.Address, indexes []*big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "claimBatch", recipient, indexes)
}

// ClaimBatch is a paid mutator transaction binding the contract method 0x7c04c80a.
//
// Solidity: function claimBatch(address recipient, uint256[] indexes) returns(uint256 amount)
func (_IVaultV2 *IVaultV2Session) ClaimBatch(recipient common.Address, indexes []*big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.ClaimBatch(&_IVaultV2.TransactOpts, recipient, indexes)
}

// ClaimBatch is a paid mutator transaction binding the contract method 0x7c04c80a.
//
// Solidity: function claimBatch(address recipient, uint256[] indexes) returns(uint256 amount)
func (_IVaultV2 *IVaultV2TransactorSession) ClaimBatch(recipient common.Address, indexes []*big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.ClaimBatch(&_IVaultV2.TransactOpts, recipient, indexes)
}

// DeallocateAdapter is a paid mutator transaction binding the contract method 0x37407957.
//
// Solidity: function deallocateAdapter(address adapter, uint256 amount) returns(uint256 deallocated)
func (_IVaultV2 *IVaultV2Transactor) DeallocateAdapter(opts *bind.TransactOpts, adapter common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "deallocateAdapter", adapter, amount)
}

// DeallocateAdapter is a paid mutator transaction binding the contract method 0x37407957.
//
// Solidity: function deallocateAdapter(address adapter, uint256 amount) returns(uint256 deallocated)
func (_IVaultV2 *IVaultV2Session) DeallocateAdapter(adapter common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.DeallocateAdapter(&_IVaultV2.TransactOpts, adapter, amount)
}

// DeallocateAdapter is a paid mutator transaction binding the contract method 0x37407957.
//
// Solidity: function deallocateAdapter(address adapter, uint256 amount) returns(uint256 deallocated)
func (_IVaultV2 *IVaultV2TransactorSession) DeallocateAdapter(adapter common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.DeallocateAdapter(&_IVaultV2.TransactOpts, adapter, amount)
}

// DeallocateAdapters is a paid mutator transaction binding the contract method 0x81b63eac.
//
// Solidity: function deallocateAdapters() returns()
func (_IVaultV2 *IVaultV2Transactor) DeallocateAdapters(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "deallocateAdapters")
}

// DeallocateAdapters is a paid mutator transaction binding the contract method 0x81b63eac.
//
// Solidity: function deallocateAdapters() returns()
func (_IVaultV2 *IVaultV2Session) DeallocateAdapters() (*types.Transaction, error) {
	return _IVaultV2.Contract.DeallocateAdapters(&_IVaultV2.TransactOpts)
}

// DeallocateAdapters is a paid mutator transaction binding the contract method 0x81b63eac.
//
// Solidity: function deallocateAdapters() returns()
func (_IVaultV2 *IVaultV2TransactorSession) DeallocateAdapters() (*types.Transaction, error) {
	return _IVaultV2.Contract.DeallocateAdapters(&_IVaultV2.TransactOpts)
}

// Deposit is a paid mutator transaction binding the contract method 0x47e7ef24.
//
// Solidity: function deposit(address onBehalfOf, uint256 amount) returns(uint256 depositedAmount, uint256 mintedShares)
func (_IVaultV2 *IVaultV2Transactor) Deposit(opts *bind.TransactOpts, onBehalfOf common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "deposit", onBehalfOf, amount)
}

// Deposit is a paid mutator transaction binding the contract method 0x47e7ef24.
//
// Solidity: function deposit(address onBehalfOf, uint256 amount) returns(uint256 depositedAmount, uint256 mintedShares)
func (_IVaultV2 *IVaultV2Session) Deposit(onBehalfOf common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Deposit(&_IVaultV2.TransactOpts, onBehalfOf, amount)
}

// Deposit is a paid mutator transaction binding the contract method 0x47e7ef24.
//
// Solidity: function deposit(address onBehalfOf, uint256 amount) returns(uint256 depositedAmount, uint256 mintedShares)
func (_IVaultV2 *IVaultV2TransactorSession) Deposit(onBehalfOf common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Deposit(&_IVaultV2.TransactOpts, onBehalfOf, amount)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (_IVaultV2 *IVaultV2Transactor) Initialize(opts *bind.TransactOpts, initialVersion uint64, owner common.Address, data []byte) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "initialize", initialVersion, owner, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (_IVaultV2 *IVaultV2Session) Initialize(initialVersion uint64, owner common.Address, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Initialize(&_IVaultV2.TransactOpts, initialVersion, owner, data)
}

// Initialize is a paid mutator transaction binding the contract method 0x57ec83cc.
//
// Solidity: function initialize(uint64 initialVersion, address owner, bytes data) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Initialize(initialVersion uint64, owner common.Address, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Initialize(&_IVaultV2.TransactOpts, initialVersion, owner, data)
}

// InstantWithdraw is a paid mutator transaction binding the contract method 0xa900ad6a.
//
// Solidity: function instantWithdraw(address recipient, uint256 amount) returns(uint256 withdrawnAssets, uint256 burnedShares)
func (_IVaultV2 *IVaultV2Transactor) InstantWithdraw(opts *bind.TransactOpts, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "instantWithdraw", recipient, amount)
}

// InstantWithdraw is a paid mutator transaction binding the contract method 0xa900ad6a.
//
// Solidity: function instantWithdraw(address recipient, uint256 amount) returns(uint256 withdrawnAssets, uint256 burnedShares)
func (_IVaultV2 *IVaultV2Session) InstantWithdraw(recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.InstantWithdraw(&_IVaultV2.TransactOpts, recipient, amount)
}

// InstantWithdraw is a paid mutator transaction binding the contract method 0xa900ad6a.
//
// Solidity: function instantWithdraw(address recipient, uint256 amount) returns(uint256 withdrawnAssets, uint256 burnedShares)
func (_IVaultV2 *IVaultV2TransactorSession) InstantWithdraw(recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.InstantWithdraw(&_IVaultV2.TransactOpts, recipient, amount)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_IVaultV2 *IVaultV2Transactor) Migrate(opts *bind.TransactOpts, newVersion uint64, data []byte) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "migrate", newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_IVaultV2 *IVaultV2Session) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Migrate(&_IVaultV2.TransactOpts, newVersion, data)
}

// Migrate is a paid mutator transaction binding the contract method 0x2abe3048.
//
// Solidity: function migrate(uint64 newVersion, bytes data) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Migrate(newVersion uint64, data []byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Migrate(&_IVaultV2.TransactOpts, newVersion, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_IVaultV2 *IVaultV2Transactor) Multicall(opts *bind.TransactOpts, data [][]byte) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "multicall", data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_IVaultV2 *IVaultV2Session) Multicall(data [][]byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Multicall(&_IVaultV2.TransactOpts, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns()
func (_IVaultV2 *IVaultV2TransactorSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _IVaultV2.Contract.Multicall(&_IVaultV2.TransactOpts, data)
}

// Redeem is a paid mutator transaction binding the contract method 0x1e9a6950.
//
// Solidity: function redeem(address claimer, uint256 shares) returns(uint256 withdrawnAssets, uint256 mintedShares)
func (_IVaultV2 *IVaultV2Transactor) Redeem(opts *bind.TransactOpts, claimer common.Address, shares *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "redeem", claimer, shares)
}

// Redeem is a paid mutator transaction binding the contract method 0x1e9a6950.
//
// Solidity: function redeem(address claimer, uint256 shares) returns(uint256 withdrawnAssets, uint256 mintedShares)
func (_IVaultV2 *IVaultV2Session) Redeem(claimer common.Address, shares *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Redeem(&_IVaultV2.TransactOpts, claimer, shares)
}

// Redeem is a paid mutator transaction binding the contract method 0x1e9a6950.
//
// Solidity: function redeem(address claimer, uint256 shares) returns(uint256 withdrawnAssets, uint256 mintedShares)
func (_IVaultV2 *IVaultV2TransactorSession) Redeem(claimer common.Address, shares *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Redeem(&_IVaultV2.TransactOpts, claimer, shares)
}

// SetAdapterLimit is a paid mutator transaction binding the contract method 0x7fd4aec4.
//
// Solidity: function setAdapterLimit(address adapter, uint208 limit) returns()
func (_IVaultV2 *IVaultV2Transactor) SetAdapterLimit(opts *bind.TransactOpts, adapter common.Address, limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setAdapterLimit", adapter, limit)
}

// SetAdapterLimit is a paid mutator transaction binding the contract method 0x7fd4aec4.
//
// Solidity: function setAdapterLimit(address adapter, uint208 limit) returns()
func (_IVaultV2 *IVaultV2Session) SetAdapterLimit(adapter common.Address, limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetAdapterLimit(&_IVaultV2.TransactOpts, adapter, limit)
}

// SetAdapterLimit is a paid mutator transaction binding the contract method 0x7fd4aec4.
//
// Solidity: function setAdapterLimit(address adapter, uint208 limit) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetAdapterLimit(adapter common.Address, limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetAdapterLimit(&_IVaultV2.TransactOpts, adapter, limit)
}

// SetDepositLimit is a paid mutator transaction binding the contract method 0xbdc8144b.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (_IVaultV2 *IVaultV2Transactor) SetDepositLimit(opts *bind.TransactOpts, limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setDepositLimit", limit)
}

// SetDepositLimit is a paid mutator transaction binding the contract method 0xbdc8144b.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (_IVaultV2 *IVaultV2Session) SetDepositLimit(limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositLimit(&_IVaultV2.TransactOpts, limit)
}

// SetDepositLimit is a paid mutator transaction binding the contract method 0xbdc8144b.
//
// Solidity: function setDepositLimit(uint256 limit) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetDepositLimit(limit *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositLimit(&_IVaultV2.TransactOpts, limit)
}

// SetDepositWhitelist is a paid mutator transaction binding the contract method 0x4105a7dd.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (_IVaultV2 *IVaultV2Transactor) SetDepositWhitelist(opts *bind.TransactOpts, status bool) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setDepositWhitelist", status)
}

// SetDepositWhitelist is a paid mutator transaction binding the contract method 0x4105a7dd.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (_IVaultV2 *IVaultV2Session) SetDepositWhitelist(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositWhitelist(&_IVaultV2.TransactOpts, status)
}

// SetDepositWhitelist is a paid mutator transaction binding the contract method 0x4105a7dd.
//
// Solidity: function setDepositWhitelist(bool status) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetDepositWhitelist(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositWhitelist(&_IVaultV2.TransactOpts, status)
}

// SetDepositorWhitelistStatus is a paid mutator transaction binding the contract method 0xa2861466.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (_IVaultV2 *IVaultV2Transactor) SetDepositorWhitelistStatus(opts *bind.TransactOpts, account common.Address, status bool) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setDepositorWhitelistStatus", account, status)
}

// SetDepositorWhitelistStatus is a paid mutator transaction binding the contract method 0xa2861466.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (_IVaultV2 *IVaultV2Session) SetDepositorWhitelistStatus(account common.Address, status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositorWhitelistStatus(&_IVaultV2.TransactOpts, account, status)
}

// SetDepositorWhitelistStatus is a paid mutator transaction binding the contract method 0xa2861466.
//
// Solidity: function setDepositorWhitelistStatus(address account, bool status) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetDepositorWhitelistStatus(account common.Address, status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetDepositorWhitelistStatus(&_IVaultV2.TransactOpts, account, status)
}

// SetIsDepositLimit is a paid mutator transaction binding the contract method 0x5346e34f.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (_IVaultV2 *IVaultV2Transactor) SetIsDepositLimit(opts *bind.TransactOpts, status bool) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "setIsDepositLimit", status)
}

// SetIsDepositLimit is a paid mutator transaction binding the contract method 0x5346e34f.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (_IVaultV2 *IVaultV2Session) SetIsDepositLimit(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetIsDepositLimit(&_IVaultV2.TransactOpts, status)
}

// SetIsDepositLimit is a paid mutator transaction binding the contract method 0x5346e34f.
//
// Solidity: function setIsDepositLimit(bool status) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SetIsDepositLimit(status bool) (*types.Transaction, error) {
	return _IVaultV2.Contract.SetIsDepositLimit(&_IVaultV2.TransactOpts, status)
}

// SkimAdapters is a paid mutator transaction binding the contract method 0xed8938b3.
//
// Solidity: function skimAdapters() returns()
func (_IVaultV2 *IVaultV2Transactor) SkimAdapters(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "skimAdapters")
}

// SkimAdapters is a paid mutator transaction binding the contract method 0xed8938b3.
//
// Solidity: function skimAdapters() returns()
func (_IVaultV2 *IVaultV2Session) SkimAdapters() (*types.Transaction, error) {
	return _IVaultV2.Contract.SkimAdapters(&_IVaultV2.TransactOpts)
}

// SkimAdapters is a paid mutator transaction binding the contract method 0xed8938b3.
//
// Solidity: function skimAdapters() returns()
func (_IVaultV2 *IVaultV2TransactorSession) SkimAdapters() (*types.Transaction, error) {
	return _IVaultV2.Contract.SkimAdapters(&_IVaultV2.TransactOpts)
}

// SwapAdapters is a paid mutator transaction binding the contract method 0x8ef5792a.
//
// Solidity: function swapAdapters(address adapter1, address adapter2) returns()
func (_IVaultV2 *IVaultV2Transactor) SwapAdapters(opts *bind.TransactOpts, adapter1 common.Address, adapter2 common.Address) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "swapAdapters", adapter1, adapter2)
}

// SwapAdapters is a paid mutator transaction binding the contract method 0x8ef5792a.
//
// Solidity: function swapAdapters(address adapter1, address adapter2) returns()
func (_IVaultV2 *IVaultV2Session) SwapAdapters(adapter1 common.Address, adapter2 common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SwapAdapters(&_IVaultV2.TransactOpts, adapter1, adapter2)
}

// SwapAdapters is a paid mutator transaction binding the contract method 0x8ef5792a.
//
// Solidity: function swapAdapters(address adapter1, address adapter2) returns()
func (_IVaultV2 *IVaultV2TransactorSession) SwapAdapters(adapter1 common.Address, adapter2 common.Address) (*types.Transaction, error) {
	return _IVaultV2.Contract.SwapAdapters(&_IVaultV2.TransactOpts, adapter1, adapter2)
}

// Withdraw is a paid mutator transaction binding the contract method 0xf3fef3a3.
//
// Solidity: function withdraw(address claimer, uint256 amount) returns(uint256 burnedShares, uint256 mintedShares)
func (_IVaultV2 *IVaultV2Transactor) Withdraw(opts *bind.TransactOpts, claimer common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.contract.Transact(opts, "withdraw", claimer, amount)
}

// Withdraw is a paid mutator transaction binding the contract method 0xf3fef3a3.
//
// Solidity: function withdraw(address claimer, uint256 amount) returns(uint256 burnedShares, uint256 mintedShares)
func (_IVaultV2 *IVaultV2Session) Withdraw(claimer common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Withdraw(&_IVaultV2.TransactOpts, claimer, amount)
}

// Withdraw is a paid mutator transaction binding the contract method 0xf3fef3a3.
//
// Solidity: function withdraw(address claimer, uint256 amount) returns(uint256 burnedShares, uint256 mintedShares)
func (_IVaultV2 *IVaultV2TransactorSession) Withdraw(claimer common.Address, amount *big.Int) (*types.Transaction, error) {
	return _IVaultV2.Contract.Withdraw(&_IVaultV2.TransactOpts, claimer, amount)
}

// IVaultV2AllocateIterator is returned from FilterAllocate and is used to iterate over the raw logs and unpacked data for Allocate events raised by the IVaultV2 contract.
type IVaultV2AllocateIterator struct {
	Event *IVaultV2Allocate // Event containing the contract specifics and raw log

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
func (it *IVaultV2AllocateIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Allocate)
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
		it.Event = new(IVaultV2Allocate)
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
func (it *IVaultV2AllocateIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2AllocateIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Allocate represents a Allocate event raised by the IVaultV2 contract.
type IVaultV2Allocate struct {
	Adapter common.Address
	Amount  *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterAllocate is a free log retrieval operation binding the contract event 0x249d8eb76d5a22983620d741de2470148d1a9a26ab923aec4262770690d11ebc.
//
// Solidity: event Allocate(address indexed adapter, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) FilterAllocate(opts *bind.FilterOpts, adapter []common.Address) (*IVaultV2AllocateIterator, error) {

	var adapterRule []interface{}
	for _, adapterItem := range adapter {
		adapterRule = append(adapterRule, adapterItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Allocate", adapterRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2AllocateIterator{contract: _IVaultV2.contract, event: "Allocate", logs: logs, sub: sub}, nil
}

// WatchAllocate is a free log subscription operation binding the contract event 0x249d8eb76d5a22983620d741de2470148d1a9a26ab923aec4262770690d11ebc.
//
// Solidity: event Allocate(address indexed adapter, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) WatchAllocate(opts *bind.WatchOpts, sink chan<- *IVaultV2Allocate, adapter []common.Address) (event.Subscription, error) {

	var adapterRule []interface{}
	for _, adapterItem := range adapter {
		adapterRule = append(adapterRule, adapterItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Allocate", adapterRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Allocate)
				if err := _IVaultV2.contract.UnpackLog(event, "Allocate", log); err != nil {
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

// ParseAllocate is a log parse operation binding the contract event 0x249d8eb76d5a22983620d741de2470148d1a9a26ab923aec4262770690d11ebc.
//
// Solidity: event Allocate(address indexed adapter, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) ParseAllocate(log types.Log) (*IVaultV2Allocate, error) {
	event := new(IVaultV2Allocate)
	if err := _IVaultV2.contract.UnpackLog(event, "Allocate", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2ClaimIterator is returned from FilterClaim and is used to iterate over the raw logs and unpacked data for Claim events raised by the IVaultV2 contract.
type IVaultV2ClaimIterator struct {
	Event *IVaultV2Claim // Event containing the contract specifics and raw log

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
func (it *IVaultV2ClaimIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Claim)
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
		it.Event = new(IVaultV2Claim)
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
func (it *IVaultV2ClaimIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2ClaimIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Claim represents a Claim event raised by the IVaultV2 contract.
type IVaultV2Claim struct {
	Claimer   common.Address
	Recipient common.Address
	Index     *big.Int
	Amount    *big.Int
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterClaim is a free log retrieval operation binding the contract event 0x865ca08d59f5cb456e85cd2f7ef63664ea4f73327414e9d8152c4158b0e94645.
//
// Solidity: event Claim(address indexed claimer, address indexed recipient, uint256 index, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) FilterClaim(opts *bind.FilterOpts, claimer []common.Address, recipient []common.Address) (*IVaultV2ClaimIterator, error) {

	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}
	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Claim", claimerRule, recipientRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2ClaimIterator{contract: _IVaultV2.contract, event: "Claim", logs: logs, sub: sub}, nil
}

// WatchClaim is a free log subscription operation binding the contract event 0x865ca08d59f5cb456e85cd2f7ef63664ea4f73327414e9d8152c4158b0e94645.
//
// Solidity: event Claim(address indexed claimer, address indexed recipient, uint256 index, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) WatchClaim(opts *bind.WatchOpts, sink chan<- *IVaultV2Claim, claimer []common.Address, recipient []common.Address) (event.Subscription, error) {

	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}
	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Claim", claimerRule, recipientRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Claim)
				if err := _IVaultV2.contract.UnpackLog(event, "Claim", log); err != nil {
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

// ParseClaim is a log parse operation binding the contract event 0x865ca08d59f5cb456e85cd2f7ef63664ea4f73327414e9d8152c4158b0e94645.
//
// Solidity: event Claim(address indexed claimer, address indexed recipient, uint256 index, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) ParseClaim(log types.Log) (*IVaultV2Claim, error) {
	event := new(IVaultV2Claim)
	if err := _IVaultV2.contract.UnpackLog(event, "Claim", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2DeallocateIterator is returned from FilterDeallocate and is used to iterate over the raw logs and unpacked data for Deallocate events raised by the IVaultV2 contract.
type IVaultV2DeallocateIterator struct {
	Event *IVaultV2Deallocate // Event containing the contract specifics and raw log

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
func (it *IVaultV2DeallocateIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Deallocate)
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
		it.Event = new(IVaultV2Deallocate)
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
func (it *IVaultV2DeallocateIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2DeallocateIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Deallocate represents a Deallocate event raised by the IVaultV2 contract.
type IVaultV2Deallocate struct {
	Adapter common.Address
	Amount  *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterDeallocate is a free log retrieval operation binding the contract event 0xd338c9f6c5eed79757e45cc8cc8b14bce8f5413e34e2dbbe362bc914bf6c439b.
//
// Solidity: event Deallocate(address indexed adapter, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) FilterDeallocate(opts *bind.FilterOpts, adapter []common.Address) (*IVaultV2DeallocateIterator, error) {

	var adapterRule []interface{}
	for _, adapterItem := range adapter {
		adapterRule = append(adapterRule, adapterItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Deallocate", adapterRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2DeallocateIterator{contract: _IVaultV2.contract, event: "Deallocate", logs: logs, sub: sub}, nil
}

// WatchDeallocate is a free log subscription operation binding the contract event 0xd338c9f6c5eed79757e45cc8cc8b14bce8f5413e34e2dbbe362bc914bf6c439b.
//
// Solidity: event Deallocate(address indexed adapter, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) WatchDeallocate(opts *bind.WatchOpts, sink chan<- *IVaultV2Deallocate, adapter []common.Address) (event.Subscription, error) {

	var adapterRule []interface{}
	for _, adapterItem := range adapter {
		adapterRule = append(adapterRule, adapterItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Deallocate", adapterRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Deallocate)
				if err := _IVaultV2.contract.UnpackLog(event, "Deallocate", log); err != nil {
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

// ParseDeallocate is a log parse operation binding the contract event 0xd338c9f6c5eed79757e45cc8cc8b14bce8f5413e34e2dbbe362bc914bf6c439b.
//
// Solidity: event Deallocate(address indexed adapter, uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) ParseDeallocate(log types.Log) (*IVaultV2Deallocate, error) {
	event := new(IVaultV2Deallocate)
	if err := _IVaultV2.contract.UnpackLog(event, "Deallocate", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2DepositIterator is returned from FilterDeposit and is used to iterate over the raw logs and unpacked data for Deposit events raised by the IVaultV2 contract.
type IVaultV2DepositIterator struct {
	Event *IVaultV2Deposit // Event containing the contract specifics and raw log

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
func (it *IVaultV2DepositIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Deposit)
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
		it.Event = new(IVaultV2Deposit)
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
func (it *IVaultV2DepositIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2DepositIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Deposit represents a Deposit event raised by the IVaultV2 contract.
type IVaultV2Deposit struct {
	Depositor  common.Address
	OnBehalfOf common.Address
	Amount     *big.Int
	Shares     *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterDeposit is a free log retrieval operation binding the contract event 0xdcbc1c05240f31ff3ad067ef1ee35ce4997762752e3a095284754544f4c709d7.
//
// Solidity: event Deposit(address indexed depositor, address indexed onBehalfOf, uint256 amount, uint256 shares)
func (_IVaultV2 *IVaultV2Filterer) FilterDeposit(opts *bind.FilterOpts, depositor []common.Address, onBehalfOf []common.Address) (*IVaultV2DepositIterator, error) {

	var depositorRule []interface{}
	for _, depositorItem := range depositor {
		depositorRule = append(depositorRule, depositorItem)
	}
	var onBehalfOfRule []interface{}
	for _, onBehalfOfItem := range onBehalfOf {
		onBehalfOfRule = append(onBehalfOfRule, onBehalfOfItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Deposit", depositorRule, onBehalfOfRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2DepositIterator{contract: _IVaultV2.contract, event: "Deposit", logs: logs, sub: sub}, nil
}

// WatchDeposit is a free log subscription operation binding the contract event 0xdcbc1c05240f31ff3ad067ef1ee35ce4997762752e3a095284754544f4c709d7.
//
// Solidity: event Deposit(address indexed depositor, address indexed onBehalfOf, uint256 amount, uint256 shares)
func (_IVaultV2 *IVaultV2Filterer) WatchDeposit(opts *bind.WatchOpts, sink chan<- *IVaultV2Deposit, depositor []common.Address, onBehalfOf []common.Address) (event.Subscription, error) {

	var depositorRule []interface{}
	for _, depositorItem := range depositor {
		depositorRule = append(depositorRule, depositorItem)
	}
	var onBehalfOfRule []interface{}
	for _, onBehalfOfItem := range onBehalfOf {
		onBehalfOfRule = append(onBehalfOfRule, onBehalfOfItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Deposit", depositorRule, onBehalfOfRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Deposit)
				if err := _IVaultV2.contract.UnpackLog(event, "Deposit", log); err != nil {
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

// ParseDeposit is a log parse operation binding the contract event 0xdcbc1c05240f31ff3ad067ef1ee35ce4997762752e3a095284754544f4c709d7.
//
// Solidity: event Deposit(address indexed depositor, address indexed onBehalfOf, uint256 amount, uint256 shares)
func (_IVaultV2 *IVaultV2Filterer) ParseDeposit(log types.Log) (*IVaultV2Deposit, error) {
	event := new(IVaultV2Deposit)
	if err := _IVaultV2.contract.UnpackLog(event, "Deposit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2DonateIterator is returned from FilterDonate and is used to iterate over the raw logs and unpacked data for Donate events raised by the IVaultV2 contract.
type IVaultV2DonateIterator struct {
	Event *IVaultV2Donate // Event containing the contract specifics and raw log

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
func (it *IVaultV2DonateIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Donate)
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
		it.Event = new(IVaultV2Donate)
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
func (it *IVaultV2DonateIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2DonateIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Donate represents a Donate event raised by the IVaultV2 contract.
type IVaultV2Donate struct {
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterDonate is a free log retrieval operation binding the contract event 0x33ac262747c8397a2c737ef15aa625b857fa57c6987e46fe8590677c9a3b7a2e.
//
// Solidity: event Donate(uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) FilterDonate(opts *bind.FilterOpts) (*IVaultV2DonateIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Donate")
	if err != nil {
		return nil, err
	}
	return &IVaultV2DonateIterator{contract: _IVaultV2.contract, event: "Donate", logs: logs, sub: sub}, nil
}

// WatchDonate is a free log subscription operation binding the contract event 0x33ac262747c8397a2c737ef15aa625b857fa57c6987e46fe8590677c9a3b7a2e.
//
// Solidity: event Donate(uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) WatchDonate(opts *bind.WatchOpts, sink chan<- *IVaultV2Donate) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Donate")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Donate)
				if err := _IVaultV2.contract.UnpackLog(event, "Donate", log); err != nil {
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

// ParseDonate is a log parse operation binding the contract event 0x33ac262747c8397a2c737ef15aa625b857fa57c6987e46fe8590677c9a3b7a2e.
//
// Solidity: event Donate(uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) ParseDonate(log types.Log) (*IVaultV2Donate, error) {
	event := new(IVaultV2Donate)
	if err := _IVaultV2.contract.UnpackLog(event, "Donate", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2InitializeIterator is returned from FilterInitialize and is used to iterate over the raw logs and unpacked data for Initialize events raised by the IVaultV2 contract.
type IVaultV2InitializeIterator struct {
	Event *IVaultV2Initialize // Event containing the contract specifics and raw log

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
func (it *IVaultV2InitializeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Initialize)
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
		it.Event = new(IVaultV2Initialize)
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
func (it *IVaultV2InitializeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2InitializeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Initialize represents a Initialize event raised by the IVaultV2 contract.
type IVaultV2Initialize struct {
	Params IVaultV2InitParams
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterInitialize is a free log retrieval operation binding the contract event 0x650f363dcf5a924b4ebf66a583f0646fb0cee5eacf4ed68a3404d167ff2df7c1.
//
// Solidity: event Initialize((string,string,address,address,uint48,bool,address,bool,uint256,address,address,address,address,address,address,address,address,address) params)
func (_IVaultV2 *IVaultV2Filterer) FilterInitialize(opts *bind.FilterOpts) (*IVaultV2InitializeIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return &IVaultV2InitializeIterator{contract: _IVaultV2.contract, event: "Initialize", logs: logs, sub: sub}, nil
}

// WatchInitialize is a free log subscription operation binding the contract event 0x650f363dcf5a924b4ebf66a583f0646fb0cee5eacf4ed68a3404d167ff2df7c1.
//
// Solidity: event Initialize((string,string,address,address,uint48,bool,address,bool,uint256,address,address,address,address,address,address,address,address,address) params)
func (_IVaultV2 *IVaultV2Filterer) WatchInitialize(opts *bind.WatchOpts, sink chan<- *IVaultV2Initialize) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Initialize")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Initialize)
				if err := _IVaultV2.contract.UnpackLog(event, "Initialize", log); err != nil {
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

// ParseInitialize is a log parse operation binding the contract event 0x650f363dcf5a924b4ebf66a583f0646fb0cee5eacf4ed68a3404d167ff2df7c1.
//
// Solidity: event Initialize((string,string,address,address,uint48,bool,address,bool,uint256,address,address,address,address,address,address,address,address,address) params)
func (_IVaultV2 *IVaultV2Filterer) ParseInitialize(log types.Log) (*IVaultV2Initialize, error) {
	event := new(IVaultV2Initialize)
	if err := _IVaultV2.contract.UnpackLog(event, "Initialize", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2InstantWithdrawIterator is returned from FilterInstantWithdraw and is used to iterate over the raw logs and unpacked data for InstantWithdraw events raised by the IVaultV2 contract.
type IVaultV2InstantWithdrawIterator struct {
	Event *IVaultV2InstantWithdraw // Event containing the contract specifics and raw log

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
func (it *IVaultV2InstantWithdrawIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2InstantWithdraw)
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
		it.Event = new(IVaultV2InstantWithdraw)
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
func (it *IVaultV2InstantWithdrawIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2InstantWithdrawIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2InstantWithdraw represents a InstantWithdraw event raised by the IVaultV2 contract.
type IVaultV2InstantWithdraw struct {
	Withdrawer   common.Address
	Amount       *big.Int
	BurnedShares *big.Int
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterInstantWithdraw is a free log retrieval operation binding the contract event 0xab2daf3c146ca6416cbccd2a86ed2ba995e171ef6319df14a38aef01403a9c96.
//
// Solidity: event InstantWithdraw(address indexed withdrawer, uint256 amount, uint256 burnedShares)
func (_IVaultV2 *IVaultV2Filterer) FilterInstantWithdraw(opts *bind.FilterOpts, withdrawer []common.Address) (*IVaultV2InstantWithdrawIterator, error) {

	var withdrawerRule []interface{}
	for _, withdrawerItem := range withdrawer {
		withdrawerRule = append(withdrawerRule, withdrawerItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "InstantWithdraw", withdrawerRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2InstantWithdrawIterator{contract: _IVaultV2.contract, event: "InstantWithdraw", logs: logs, sub: sub}, nil
}

// WatchInstantWithdraw is a free log subscription operation binding the contract event 0xab2daf3c146ca6416cbccd2a86ed2ba995e171ef6319df14a38aef01403a9c96.
//
// Solidity: event InstantWithdraw(address indexed withdrawer, uint256 amount, uint256 burnedShares)
func (_IVaultV2 *IVaultV2Filterer) WatchInstantWithdraw(opts *bind.WatchOpts, sink chan<- *IVaultV2InstantWithdraw, withdrawer []common.Address) (event.Subscription, error) {

	var withdrawerRule []interface{}
	for _, withdrawerItem := range withdrawer {
		withdrawerRule = append(withdrawerRule, withdrawerItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "InstantWithdraw", withdrawerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2InstantWithdraw)
				if err := _IVaultV2.contract.UnpackLog(event, "InstantWithdraw", log); err != nil {
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

// ParseInstantWithdraw is a log parse operation binding the contract event 0xab2daf3c146ca6416cbccd2a86ed2ba995e171ef6319df14a38aef01403a9c96.
//
// Solidity: event InstantWithdraw(address indexed withdrawer, uint256 amount, uint256 burnedShares)
func (_IVaultV2 *IVaultV2Filterer) ParseInstantWithdraw(log types.Log) (*IVaultV2InstantWithdraw, error) {
	event := new(IVaultV2InstantWithdraw)
	if err := _IVaultV2.contract.UnpackLog(event, "InstantWithdraw", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2MigrateIterator is returned from FilterMigrate and is used to iterate over the raw logs and unpacked data for Migrate events raised by the IVaultV2 contract.
type IVaultV2MigrateIterator struct {
	Event *IVaultV2Migrate // Event containing the contract specifics and raw log

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
func (it *IVaultV2MigrateIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Migrate)
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
		it.Event = new(IVaultV2Migrate)
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
func (it *IVaultV2MigrateIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2MigrateIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Migrate represents a Migrate event raised by the IVaultV2 contract.
type IVaultV2Migrate struct {
	Params       IVaultV2MigrateParams
	NewDelegator common.Address
	NewSlasher   common.Address
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterMigrate is a free log retrieval operation binding the contract event 0x49bf37d2569e6fea435da3e11e30505b25a1bc332b6a5173b6e74476a81ac0cb.
//
// Solidity: event Migrate((string,string,address,address,address,address,address,bytes,bytes) params, address newDelegator, address newSlasher)
func (_IVaultV2 *IVaultV2Filterer) FilterMigrate(opts *bind.FilterOpts) (*IVaultV2MigrateIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Migrate")
	if err != nil {
		return nil, err
	}
	return &IVaultV2MigrateIterator{contract: _IVaultV2.contract, event: "Migrate", logs: logs, sub: sub}, nil
}

// WatchMigrate is a free log subscription operation binding the contract event 0x49bf37d2569e6fea435da3e11e30505b25a1bc332b6a5173b6e74476a81ac0cb.
//
// Solidity: event Migrate((string,string,address,address,address,address,address,bytes,bytes) params, address newDelegator, address newSlasher)
func (_IVaultV2 *IVaultV2Filterer) WatchMigrate(opts *bind.WatchOpts, sink chan<- *IVaultV2Migrate) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Migrate")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Migrate)
				if err := _IVaultV2.contract.UnpackLog(event, "Migrate", log); err != nil {
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

// ParseMigrate is a log parse operation binding the contract event 0x49bf37d2569e6fea435da3e11e30505b25a1bc332b6a5173b6e74476a81ac0cb.
//
// Solidity: event Migrate((string,string,address,address,address,address,address,bytes,bytes) params, address newDelegator, address newSlasher)
func (_IVaultV2 *IVaultV2Filterer) ParseMigrate(log types.Log) (*IVaultV2Migrate, error) {
	event := new(IVaultV2Migrate)
	if err := _IVaultV2.contract.UnpackLog(event, "Migrate", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2OnSlashIterator is returned from FilterOnSlash and is used to iterate over the raw logs and unpacked data for OnSlash events raised by the IVaultV2 contract.
type IVaultV2OnSlashIterator struct {
	Event *IVaultV2OnSlash // Event containing the contract specifics and raw log

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
func (it *IVaultV2OnSlashIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2OnSlash)
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
		it.Event = new(IVaultV2OnSlash)
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
func (it *IVaultV2OnSlashIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2OnSlashIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2OnSlash represents a OnSlash event raised by the IVaultV2 contract.
type IVaultV2OnSlash struct {
	Amount        *big.Int
	SlashedAmount *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOnSlash is a free log retrieval operation binding the contract event 0x2b99884293d551451b145cf48341a86d08a355a86be8e2f0b22cea964226a093.
//
// Solidity: event OnSlash(uint256 amount, uint256 slashedAmount)
func (_IVaultV2 *IVaultV2Filterer) FilterOnSlash(opts *bind.FilterOpts) (*IVaultV2OnSlashIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "OnSlash")
	if err != nil {
		return nil, err
	}
	return &IVaultV2OnSlashIterator{contract: _IVaultV2.contract, event: "OnSlash", logs: logs, sub: sub}, nil
}

// WatchOnSlash is a free log subscription operation binding the contract event 0x2b99884293d551451b145cf48341a86d08a355a86be8e2f0b22cea964226a093.
//
// Solidity: event OnSlash(uint256 amount, uint256 slashedAmount)
func (_IVaultV2 *IVaultV2Filterer) WatchOnSlash(opts *bind.WatchOpts, sink chan<- *IVaultV2OnSlash) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "OnSlash")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2OnSlash)
				if err := _IVaultV2.contract.UnpackLog(event, "OnSlash", log); err != nil {
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

// ParseOnSlash is a log parse operation binding the contract event 0x2b99884293d551451b145cf48341a86d08a355a86be8e2f0b22cea964226a093.
//
// Solidity: event OnSlash(uint256 amount, uint256 slashedAmount)
func (_IVaultV2 *IVaultV2Filterer) ParseOnSlash(log types.Log) (*IVaultV2OnSlash, error) {
	event := new(IVaultV2OnSlash)
	if err := _IVaultV2.contract.UnpackLog(event, "OnSlash", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetAdapterLimitIterator is returned from FilterSetAdapterLimit and is used to iterate over the raw logs and unpacked data for SetAdapterLimit events raised by the IVaultV2 contract.
type IVaultV2SetAdapterLimitIterator struct {
	Event *IVaultV2SetAdapterLimit // Event containing the contract specifics and raw log

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
func (it *IVaultV2SetAdapterLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetAdapterLimit)
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
		it.Event = new(IVaultV2SetAdapterLimit)
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
func (it *IVaultV2SetAdapterLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetAdapterLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetAdapterLimit represents a SetAdapterLimit event raised by the IVaultV2 contract.
type IVaultV2SetAdapterLimit struct {
	Adapter common.Address
	Limit   *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterSetAdapterLimit is a free log retrieval operation binding the contract event 0xc46a0292f919e4ce9780bd4e410e91b531f9b883ebc6b8953c5ce7692fd5d312.
//
// Solidity: event SetAdapterLimit(address indexed adapter, uint208 limit)
func (_IVaultV2 *IVaultV2Filterer) FilterSetAdapterLimit(opts *bind.FilterOpts, adapter []common.Address) (*IVaultV2SetAdapterLimitIterator, error) {

	var adapterRule []interface{}
	for _, adapterItem := range adapter {
		adapterRule = append(adapterRule, adapterItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetAdapterLimit", adapterRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetAdapterLimitIterator{contract: _IVaultV2.contract, event: "SetAdapterLimit", logs: logs, sub: sub}, nil
}

// WatchSetAdapterLimit is a free log subscription operation binding the contract event 0xc46a0292f919e4ce9780bd4e410e91b531f9b883ebc6b8953c5ce7692fd5d312.
//
// Solidity: event SetAdapterLimit(address indexed adapter, uint208 limit)
func (_IVaultV2 *IVaultV2Filterer) WatchSetAdapterLimit(opts *bind.WatchOpts, sink chan<- *IVaultV2SetAdapterLimit, adapter []common.Address) (event.Subscription, error) {

	var adapterRule []interface{}
	for _, adapterItem := range adapter {
		adapterRule = append(adapterRule, adapterItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetAdapterLimit", adapterRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetAdapterLimit)
				if err := _IVaultV2.contract.UnpackLog(event, "SetAdapterLimit", log); err != nil {
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

// ParseSetAdapterLimit is a log parse operation binding the contract event 0xc46a0292f919e4ce9780bd4e410e91b531f9b883ebc6b8953c5ce7692fd5d312.
//
// Solidity: event SetAdapterLimit(address indexed adapter, uint208 limit)
func (_IVaultV2 *IVaultV2Filterer) ParseSetAdapterLimit(log types.Log) (*IVaultV2SetAdapterLimit, error) {
	event := new(IVaultV2SetAdapterLimit)
	if err := _IVaultV2.contract.UnpackLog(event, "SetAdapterLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDelegatorIterator is returned from FilterSetDelegator and is used to iterate over the raw logs and unpacked data for SetDelegator events raised by the IVaultV2 contract.
type IVaultV2SetDelegatorIterator struct {
	Event *IVaultV2SetDelegator // Event containing the contract specifics and raw log

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
func (it *IVaultV2SetDelegatorIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDelegator)
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
		it.Event = new(IVaultV2SetDelegator)
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
func (it *IVaultV2SetDelegatorIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDelegatorIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDelegator represents a SetDelegator event raised by the IVaultV2 contract.
type IVaultV2SetDelegator struct {
	Delegator common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterSetDelegator is a free log retrieval operation binding the contract event 0xdb2160616f776a37b24808115554e79439bf26cccbbd4438190cc6d28e80ecd1.
//
// Solidity: event SetDelegator(address indexed delegator)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDelegator(opts *bind.FilterOpts, delegator []common.Address) (*IVaultV2SetDelegatorIterator, error) {

	var delegatorRule []interface{}
	for _, delegatorItem := range delegator {
		delegatorRule = append(delegatorRule, delegatorItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDelegator", delegatorRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDelegatorIterator{contract: _IVaultV2.contract, event: "SetDelegator", logs: logs, sub: sub}, nil
}

// WatchSetDelegator is a free log subscription operation binding the contract event 0xdb2160616f776a37b24808115554e79439bf26cccbbd4438190cc6d28e80ecd1.
//
// Solidity: event SetDelegator(address indexed delegator)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDelegator(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDelegator, delegator []common.Address) (event.Subscription, error) {

	var delegatorRule []interface{}
	for _, delegatorItem := range delegator {
		delegatorRule = append(delegatorRule, delegatorItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDelegator", delegatorRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDelegator)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDelegator", log); err != nil {
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

// ParseSetDelegator is a log parse operation binding the contract event 0xdb2160616f776a37b24808115554e79439bf26cccbbd4438190cc6d28e80ecd1.
//
// Solidity: event SetDelegator(address indexed delegator)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDelegator(log types.Log) (*IVaultV2SetDelegator, error) {
	event := new(IVaultV2SetDelegator)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDelegator", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDepositLimitIterator is returned from FilterSetDepositLimit and is used to iterate over the raw logs and unpacked data for SetDepositLimit events raised by the IVaultV2 contract.
type IVaultV2SetDepositLimitIterator struct {
	Event *IVaultV2SetDepositLimit // Event containing the contract specifics and raw log

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
func (it *IVaultV2SetDepositLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDepositLimit)
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
		it.Event = new(IVaultV2SetDepositLimit)
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
func (it *IVaultV2SetDepositLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDepositLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDepositLimit represents a SetDepositLimit event raised by the IVaultV2 contract.
type IVaultV2SetDepositLimit struct {
	Limit *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterSetDepositLimit is a free log retrieval operation binding the contract event 0x854df3eb95564502c8bc871ebdd15310ee26270f955f6c6bd8cea68e75045bc0.
//
// Solidity: event SetDepositLimit(uint256 limit)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDepositLimit(opts *bind.FilterOpts) (*IVaultV2SetDepositLimitIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDepositLimit")
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDepositLimitIterator{contract: _IVaultV2.contract, event: "SetDepositLimit", logs: logs, sub: sub}, nil
}

// WatchSetDepositLimit is a free log subscription operation binding the contract event 0x854df3eb95564502c8bc871ebdd15310ee26270f955f6c6bd8cea68e75045bc0.
//
// Solidity: event SetDepositLimit(uint256 limit)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDepositLimit(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDepositLimit) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDepositLimit")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDepositLimit)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDepositLimit", log); err != nil {
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

// ParseSetDepositLimit is a log parse operation binding the contract event 0x854df3eb95564502c8bc871ebdd15310ee26270f955f6c6bd8cea68e75045bc0.
//
// Solidity: event SetDepositLimit(uint256 limit)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDepositLimit(log types.Log) (*IVaultV2SetDepositLimit, error) {
	event := new(IVaultV2SetDepositLimit)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDepositLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDepositWhitelistIterator is returned from FilterSetDepositWhitelist and is used to iterate over the raw logs and unpacked data for SetDepositWhitelist events raised by the IVaultV2 contract.
type IVaultV2SetDepositWhitelistIterator struct {
	Event *IVaultV2SetDepositWhitelist // Event containing the contract specifics and raw log

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
func (it *IVaultV2SetDepositWhitelistIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDepositWhitelist)
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
		it.Event = new(IVaultV2SetDepositWhitelist)
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
func (it *IVaultV2SetDepositWhitelistIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDepositWhitelistIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDepositWhitelist represents a SetDepositWhitelist event raised by the IVaultV2 contract.
type IVaultV2SetDepositWhitelist struct {
	Status bool
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSetDepositWhitelist is a free log retrieval operation binding the contract event 0x3e12b7b36c75ac9609a3f58609b331210428e1a85909132638955ba0301eec33.
//
// Solidity: event SetDepositWhitelist(bool status)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDepositWhitelist(opts *bind.FilterOpts) (*IVaultV2SetDepositWhitelistIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDepositWhitelist")
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDepositWhitelistIterator{contract: _IVaultV2.contract, event: "SetDepositWhitelist", logs: logs, sub: sub}, nil
}

// WatchSetDepositWhitelist is a free log subscription operation binding the contract event 0x3e12b7b36c75ac9609a3f58609b331210428e1a85909132638955ba0301eec33.
//
// Solidity: event SetDepositWhitelist(bool status)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDepositWhitelist(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDepositWhitelist) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDepositWhitelist")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDepositWhitelist)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDepositWhitelist", log); err != nil {
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

// ParseSetDepositWhitelist is a log parse operation binding the contract event 0x3e12b7b36c75ac9609a3f58609b331210428e1a85909132638955ba0301eec33.
//
// Solidity: event SetDepositWhitelist(bool status)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDepositWhitelist(log types.Log) (*IVaultV2SetDepositWhitelist, error) {
	event := new(IVaultV2SetDepositWhitelist)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDepositWhitelist", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetDepositorWhitelistStatusIterator is returned from FilterSetDepositorWhitelistStatus and is used to iterate over the raw logs and unpacked data for SetDepositorWhitelistStatus events raised by the IVaultV2 contract.
type IVaultV2SetDepositorWhitelistStatusIterator struct {
	Event *IVaultV2SetDepositorWhitelistStatus // Event containing the contract specifics and raw log

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
func (it *IVaultV2SetDepositorWhitelistStatusIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetDepositorWhitelistStatus)
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
		it.Event = new(IVaultV2SetDepositorWhitelistStatus)
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
func (it *IVaultV2SetDepositorWhitelistStatusIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetDepositorWhitelistStatusIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetDepositorWhitelistStatus represents a SetDepositorWhitelistStatus event raised by the IVaultV2 contract.
type IVaultV2SetDepositorWhitelistStatus struct {
	Account common.Address
	Status  bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterSetDepositorWhitelistStatus is a free log retrieval operation binding the contract event 0xf991b1ecfb5115cbb36a2b2e2240c058406d2acc2fcc6e9e2dc99d845ff70a62.
//
// Solidity: event SetDepositorWhitelistStatus(address indexed account, bool status)
func (_IVaultV2 *IVaultV2Filterer) FilterSetDepositorWhitelistStatus(opts *bind.FilterOpts, account []common.Address) (*IVaultV2SetDepositorWhitelistStatusIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetDepositorWhitelistStatus", accountRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetDepositorWhitelistStatusIterator{contract: _IVaultV2.contract, event: "SetDepositorWhitelistStatus", logs: logs, sub: sub}, nil
}

// WatchSetDepositorWhitelistStatus is a free log subscription operation binding the contract event 0xf991b1ecfb5115cbb36a2b2e2240c058406d2acc2fcc6e9e2dc99d845ff70a62.
//
// Solidity: event SetDepositorWhitelistStatus(address indexed account, bool status)
func (_IVaultV2 *IVaultV2Filterer) WatchSetDepositorWhitelistStatus(opts *bind.WatchOpts, sink chan<- *IVaultV2SetDepositorWhitelistStatus, account []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetDepositorWhitelistStatus", accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetDepositorWhitelistStatus)
				if err := _IVaultV2.contract.UnpackLog(event, "SetDepositorWhitelistStatus", log); err != nil {
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

// ParseSetDepositorWhitelistStatus is a log parse operation binding the contract event 0xf991b1ecfb5115cbb36a2b2e2240c058406d2acc2fcc6e9e2dc99d845ff70a62.
//
// Solidity: event SetDepositorWhitelistStatus(address indexed account, bool status)
func (_IVaultV2 *IVaultV2Filterer) ParseSetDepositorWhitelistStatus(log types.Log) (*IVaultV2SetDepositorWhitelistStatus, error) {
	event := new(IVaultV2SetDepositorWhitelistStatus)
	if err := _IVaultV2.contract.UnpackLog(event, "SetDepositorWhitelistStatus", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetIsDepositLimitIterator is returned from FilterSetIsDepositLimit and is used to iterate over the raw logs and unpacked data for SetIsDepositLimit events raised by the IVaultV2 contract.
type IVaultV2SetIsDepositLimitIterator struct {
	Event *IVaultV2SetIsDepositLimit // Event containing the contract specifics and raw log

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
func (it *IVaultV2SetIsDepositLimitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetIsDepositLimit)
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
		it.Event = new(IVaultV2SetIsDepositLimit)
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
func (it *IVaultV2SetIsDepositLimitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetIsDepositLimitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetIsDepositLimit represents a SetIsDepositLimit event raised by the IVaultV2 contract.
type IVaultV2SetIsDepositLimit struct {
	Status bool
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSetIsDepositLimit is a free log retrieval operation binding the contract event 0xfa7a25a0b611d4ba3c0ea990e90dc23d484a5dd7a1be4733fef2946ba74530c6.
//
// Solidity: event SetIsDepositLimit(bool status)
func (_IVaultV2 *IVaultV2Filterer) FilterSetIsDepositLimit(opts *bind.FilterOpts) (*IVaultV2SetIsDepositLimitIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetIsDepositLimit")
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetIsDepositLimitIterator{contract: _IVaultV2.contract, event: "SetIsDepositLimit", logs: logs, sub: sub}, nil
}

// WatchSetIsDepositLimit is a free log subscription operation binding the contract event 0xfa7a25a0b611d4ba3c0ea990e90dc23d484a5dd7a1be4733fef2946ba74530c6.
//
// Solidity: event SetIsDepositLimit(bool status)
func (_IVaultV2 *IVaultV2Filterer) WatchSetIsDepositLimit(opts *bind.WatchOpts, sink chan<- *IVaultV2SetIsDepositLimit) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetIsDepositLimit")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetIsDepositLimit)
				if err := _IVaultV2.contract.UnpackLog(event, "SetIsDepositLimit", log); err != nil {
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

// ParseSetIsDepositLimit is a log parse operation binding the contract event 0xfa7a25a0b611d4ba3c0ea990e90dc23d484a5dd7a1be4733fef2946ba74530c6.
//
// Solidity: event SetIsDepositLimit(bool status)
func (_IVaultV2 *IVaultV2Filterer) ParseSetIsDepositLimit(log types.Log) (*IVaultV2SetIsDepositLimit, error) {
	event := new(IVaultV2SetIsDepositLimit)
	if err := _IVaultV2.contract.UnpackLog(event, "SetIsDepositLimit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SetSlasherIterator is returned from FilterSetSlasher and is used to iterate over the raw logs and unpacked data for SetSlasher events raised by the IVaultV2 contract.
type IVaultV2SetSlasherIterator struct {
	Event *IVaultV2SetSlasher // Event containing the contract specifics and raw log

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
func (it *IVaultV2SetSlasherIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SetSlasher)
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
		it.Event = new(IVaultV2SetSlasher)
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
func (it *IVaultV2SetSlasherIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SetSlasherIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SetSlasher represents a SetSlasher event raised by the IVaultV2 contract.
type IVaultV2SetSlasher struct {
	Slasher common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterSetSlasher is a free log retrieval operation binding the contract event 0xe7e4c932e03abddfe20f83af42c33627e816115c7ec2b168441f65dc14bfc3ba.
//
// Solidity: event SetSlasher(address indexed slasher)
func (_IVaultV2 *IVaultV2Filterer) FilterSetSlasher(opts *bind.FilterOpts, slasher []common.Address) (*IVaultV2SetSlasherIterator, error) {

	var slasherRule []interface{}
	for _, slasherItem := range slasher {
		slasherRule = append(slasherRule, slasherItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SetSlasher", slasherRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SetSlasherIterator{contract: _IVaultV2.contract, event: "SetSlasher", logs: logs, sub: sub}, nil
}

// WatchSetSlasher is a free log subscription operation binding the contract event 0xe7e4c932e03abddfe20f83af42c33627e816115c7ec2b168441f65dc14bfc3ba.
//
// Solidity: event SetSlasher(address indexed slasher)
func (_IVaultV2 *IVaultV2Filterer) WatchSetSlasher(opts *bind.WatchOpts, sink chan<- *IVaultV2SetSlasher, slasher []common.Address) (event.Subscription, error) {

	var slasherRule []interface{}
	for _, slasherItem := range slasher {
		slasherRule = append(slasherRule, slasherItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SetSlasher", slasherRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SetSlasher)
				if err := _IVaultV2.contract.UnpackLog(event, "SetSlasher", log); err != nil {
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

// ParseSetSlasher is a log parse operation binding the contract event 0xe7e4c932e03abddfe20f83af42c33627e816115c7ec2b168441f65dc14bfc3ba.
//
// Solidity: event SetSlasher(address indexed slasher)
func (_IVaultV2 *IVaultV2Filterer) ParseSetSlasher(log types.Log) (*IVaultV2SetSlasher, error) {
	event := new(IVaultV2SetSlasher)
	if err := _IVaultV2.contract.UnpackLog(event, "SetSlasher", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SwapAdaptersIterator is returned from FilterSwapAdapters and is used to iterate over the raw logs and unpacked data for SwapAdapters events raised by the IVaultV2 contract.
type IVaultV2SwapAdaptersIterator struct {
	Event *IVaultV2SwapAdapters // Event containing the contract specifics and raw log

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
func (it *IVaultV2SwapAdaptersIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SwapAdapters)
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
		it.Event = new(IVaultV2SwapAdapters)
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
func (it *IVaultV2SwapAdaptersIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SwapAdaptersIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SwapAdapters represents a SwapAdapters event raised by the IVaultV2 contract.
type IVaultV2SwapAdapters struct {
	Adapter1 common.Address
	Adapter2 common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterSwapAdapters is a free log retrieval operation binding the contract event 0x50230392019880e94081f70d0afa878bc16c64b62bfed1d012d5ed9f6f2d991d.
//
// Solidity: event SwapAdapters(address indexed adapter1, address indexed adapter2)
func (_IVaultV2 *IVaultV2Filterer) FilterSwapAdapters(opts *bind.FilterOpts, adapter1 []common.Address, adapter2 []common.Address) (*IVaultV2SwapAdaptersIterator, error) {

	var adapter1Rule []interface{}
	for _, adapter1Item := range adapter1 {
		adapter1Rule = append(adapter1Rule, adapter1Item)
	}
	var adapter2Rule []interface{}
	for _, adapter2Item := range adapter2 {
		adapter2Rule = append(adapter2Rule, adapter2Item)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SwapAdapters", adapter1Rule, adapter2Rule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2SwapAdaptersIterator{contract: _IVaultV2.contract, event: "SwapAdapters", logs: logs, sub: sub}, nil
}

// WatchSwapAdapters is a free log subscription operation binding the contract event 0x50230392019880e94081f70d0afa878bc16c64b62bfed1d012d5ed9f6f2d991d.
//
// Solidity: event SwapAdapters(address indexed adapter1, address indexed adapter2)
func (_IVaultV2 *IVaultV2Filterer) WatchSwapAdapters(opts *bind.WatchOpts, sink chan<- *IVaultV2SwapAdapters, adapter1 []common.Address, adapter2 []common.Address) (event.Subscription, error) {

	var adapter1Rule []interface{}
	for _, adapter1Item := range adapter1 {
		adapter1Rule = append(adapter1Rule, adapter1Item)
	}
	var adapter2Rule []interface{}
	for _, adapter2Item := range adapter2 {
		adapter2Rule = append(adapter2Rule, adapter2Item)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SwapAdapters", adapter1Rule, adapter2Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SwapAdapters)
				if err := _IVaultV2.contract.UnpackLog(event, "SwapAdapters", log); err != nil {
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

// ParseSwapAdapters is a log parse operation binding the contract event 0x50230392019880e94081f70d0afa878bc16c64b62bfed1d012d5ed9f6f2d991d.
//
// Solidity: event SwapAdapters(address indexed adapter1, address indexed adapter2)
func (_IVaultV2 *IVaultV2Filterer) ParseSwapAdapters(log types.Log) (*IVaultV2SwapAdapters, error) {
	event := new(IVaultV2SwapAdapters)
	if err := _IVaultV2.contract.UnpackLog(event, "SwapAdapters", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2SyncOwedSlashIterator is returned from FilterSyncOwedSlash and is used to iterate over the raw logs and unpacked data for SyncOwedSlash events raised by the IVaultV2 contract.
type IVaultV2SyncOwedSlashIterator struct {
	Event *IVaultV2SyncOwedSlash // Event containing the contract specifics and raw log

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
func (it *IVaultV2SyncOwedSlashIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2SyncOwedSlash)
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
		it.Event = new(IVaultV2SyncOwedSlash)
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
func (it *IVaultV2SyncOwedSlashIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2SyncOwedSlashIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2SyncOwedSlash represents a SyncOwedSlash event raised by the IVaultV2 contract.
type IVaultV2SyncOwedSlash struct {
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSyncOwedSlash is a free log retrieval operation binding the contract event 0x3cea97cd4c5defe1459bcf2518350c9cb741392d1a028b99b49fad7ac3c47e86.
//
// Solidity: event SyncOwedSlash(uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) FilterSyncOwedSlash(opts *bind.FilterOpts) (*IVaultV2SyncOwedSlashIterator, error) {

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "SyncOwedSlash")
	if err != nil {
		return nil, err
	}
	return &IVaultV2SyncOwedSlashIterator{contract: _IVaultV2.contract, event: "SyncOwedSlash", logs: logs, sub: sub}, nil
}

// WatchSyncOwedSlash is a free log subscription operation binding the contract event 0x3cea97cd4c5defe1459bcf2518350c9cb741392d1a028b99b49fad7ac3c47e86.
//
// Solidity: event SyncOwedSlash(uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) WatchSyncOwedSlash(opts *bind.WatchOpts, sink chan<- *IVaultV2SyncOwedSlash) (event.Subscription, error) {

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "SyncOwedSlash")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2SyncOwedSlash)
				if err := _IVaultV2.contract.UnpackLog(event, "SyncOwedSlash", log); err != nil {
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

// ParseSyncOwedSlash is a log parse operation binding the contract event 0x3cea97cd4c5defe1459bcf2518350c9cb741392d1a028b99b49fad7ac3c47e86.
//
// Solidity: event SyncOwedSlash(uint256 amount)
func (_IVaultV2 *IVaultV2Filterer) ParseSyncOwedSlash(log types.Log) (*IVaultV2SyncOwedSlash, error) {
	event := new(IVaultV2SyncOwedSlash)
	if err := _IVaultV2.contract.UnpackLog(event, "SyncOwedSlash", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IVaultV2WithdrawIterator is returned from FilterWithdraw and is used to iterate over the raw logs and unpacked data for Withdraw events raised by the IVaultV2 contract.
type IVaultV2WithdrawIterator struct {
	Event *IVaultV2Withdraw // Event containing the contract specifics and raw log

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
func (it *IVaultV2WithdrawIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IVaultV2Withdraw)
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
		it.Event = new(IVaultV2Withdraw)
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
func (it *IVaultV2WithdrawIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IVaultV2WithdrawIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IVaultV2Withdraw represents a Withdraw event raised by the IVaultV2 contract.
type IVaultV2Withdraw struct {
	Withdrawer   common.Address
	Claimer      common.Address
	Amount       *big.Int
	BurnedShares *big.Int
	MintedShares *big.Int
	Index        *big.Int
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterWithdraw is a free log retrieval operation binding the contract event 0x7ff9a08662c21e17b8071f3aef03a9712ea9d3824dfb0139bba272915d59a919.
//
// Solidity: event Withdraw(address indexed withdrawer, address indexed claimer, uint256 amount, uint256 burnedShares, uint256 mintedShares, uint256 index)
func (_IVaultV2 *IVaultV2Filterer) FilterWithdraw(opts *bind.FilterOpts, withdrawer []common.Address, claimer []common.Address) (*IVaultV2WithdrawIterator, error) {

	var withdrawerRule []interface{}
	for _, withdrawerItem := range withdrawer {
		withdrawerRule = append(withdrawerRule, withdrawerItem)
	}
	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}

	logs, sub, err := _IVaultV2.contract.FilterLogs(opts, "Withdraw", withdrawerRule, claimerRule)
	if err != nil {
		return nil, err
	}
	return &IVaultV2WithdrawIterator{contract: _IVaultV2.contract, event: "Withdraw", logs: logs, sub: sub}, nil
}

// WatchWithdraw is a free log subscription operation binding the contract event 0x7ff9a08662c21e17b8071f3aef03a9712ea9d3824dfb0139bba272915d59a919.
//
// Solidity: event Withdraw(address indexed withdrawer, address indexed claimer, uint256 amount, uint256 burnedShares, uint256 mintedShares, uint256 index)
func (_IVaultV2 *IVaultV2Filterer) WatchWithdraw(opts *bind.WatchOpts, sink chan<- *IVaultV2Withdraw, withdrawer []common.Address, claimer []common.Address) (event.Subscription, error) {

	var withdrawerRule []interface{}
	for _, withdrawerItem := range withdrawer {
		withdrawerRule = append(withdrawerRule, withdrawerItem)
	}
	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}

	logs, sub, err := _IVaultV2.contract.WatchLogs(opts, "Withdraw", withdrawerRule, claimerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IVaultV2Withdraw)
				if err := _IVaultV2.contract.UnpackLog(event, "Withdraw", log); err != nil {
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

// ParseWithdraw is a log parse operation binding the contract event 0x7ff9a08662c21e17b8071f3aef03a9712ea9d3824dfb0139bba272915d59a919.
//
// Solidity: event Withdraw(address indexed withdrawer, address indexed claimer, uint256 amount, uint256 burnedShares, uint256 mintedShares, uint256 index)
func (_IVaultV2 *IVaultV2Filterer) ParseWithdraw(log types.Log) (*IVaultV2Withdraw, error) {
	event := new(IVaultV2Withdraw)
	if err := _IVaultV2.contract.UnpackLog(event, "Withdraw", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
