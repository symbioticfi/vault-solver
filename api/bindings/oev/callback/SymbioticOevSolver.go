// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package callback

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

// SymbioticOevSolverMetaData contains all meta data concerning the SymbioticOevSolver contract.
var SymbioticOevSolverMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"executor\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"morpho\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"liquidLaneAdapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"authSigner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initialOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"receive\",\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"AUTH_SIGNER\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"EXECUTOR\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"LIQUID_LANE_ADAPTER\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MORPHO\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"liquidate\",\"inputs\":[{\"name\":\"bidAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operationData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"onMorphoLiquidate\",\"inputs\":[{\"name\":\"repaidAssets\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"payBid\",\"inputs\":[{\"name\":\"bidAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"usedAuctionKey\",\"inputs\":[{\"name\":\"auctionKey\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"used\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdrawERC20\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdrawNative\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"BundleResult\",\"inputs\":[{\"name\":\"auctionKey\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"totalProfitLoan\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"minProfitLoan\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"gasUsed\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"bidAuthorized\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"LegResult\",\"inputs\":[{\"name\":\"auctionKey\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"marketId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Id\"},{\"name\":\"borrower\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"code\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"seizedAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"repaidAssets\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"profitLoan\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"gasUsed\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnerUpdated\",\"inputs\":[{\"name\":\"previous\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"next\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PayBidResult\",\"inputs\":[{\"name\":\"auctionKey\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"bidAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"paid\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ECDSAInvalidSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ECDSAInvalidSignatureLength\",\"inputs\":[{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ECDSAInvalidSignatureS\",\"inputs\":[{\"name\":\"s\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"InsufficientLoanProceeds\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAuth\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotExecutor\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotMorpho\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotOwner\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ProfitBelowMin\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SwapOutputBelowMin\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TransferFailed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ZeroAddress\",\"inputs\":[]}]",
	ID:  "SymbioticOevSolver",
}

// SymbioticOevSolver is an auto generated Go binding around an Ethereum contract.
type SymbioticOevSolver struct {
	abi abi.ABI
}

// NewSymbioticOevSolver creates a new instance of SymbioticOevSolver.
func NewSymbioticOevSolver() *SymbioticOevSolver {
	parsed, err := SymbioticOevSolverMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &SymbioticOevSolver{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *SymbioticOevSolver) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address executor, address morpho, address liquidLaneAdapter, address authSigner, address initialOwner) returns()
func (symbioticOevSolver *SymbioticOevSolver) PackConstructor(executor common.Address, morpho common.Address, liquidLaneAdapter common.Address, authSigner common.Address, initialOwner common.Address) []byte {
	enc, err := symbioticOevSolver.abi.Pack("", executor, morpho, liquidLaneAdapter, authSigner, initialOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackAUTHSIGNER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0a5c9024.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function AUTH_SIGNER() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) PackAUTHSIGNER() []byte {
	enc, err := symbioticOevSolver.abi.Pack("AUTH_SIGNER")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAUTHSIGNER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0a5c9024.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function AUTH_SIGNER() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) TryPackAUTHSIGNER() ([]byte, error) {
	return symbioticOevSolver.abi.Pack("AUTH_SIGNER")
}

// UnpackAUTHSIGNER is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0a5c9024.
//
// Solidity: function AUTH_SIGNER() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) UnpackAUTHSIGNER(data []byte) (common.Address, error) {
	out, err := symbioticOevSolver.abi.Unpack("AUTH_SIGNER", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackEXECUTOR is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x630dc7cb.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function EXECUTOR() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) PackEXECUTOR() []byte {
	enc, err := symbioticOevSolver.abi.Pack("EXECUTOR")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackEXECUTOR is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x630dc7cb.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function EXECUTOR() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) TryPackEXECUTOR() ([]byte, error) {
	return symbioticOevSolver.abi.Pack("EXECUTOR")
}

// UnpackEXECUTOR is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x630dc7cb.
//
// Solidity: function EXECUTOR() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) UnpackEXECUTOR(data []byte) (common.Address, error) {
	out, err := symbioticOevSolver.abi.Unpack("EXECUTOR", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackLIQUIDLANEADAPTER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x86e7c9d0.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function LIQUID_LANE_ADAPTER() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) PackLIQUIDLANEADAPTER() []byte {
	enc, err := symbioticOevSolver.abi.Pack("LIQUID_LANE_ADAPTER")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLIQUIDLANEADAPTER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x86e7c9d0.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function LIQUID_LANE_ADAPTER() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) TryPackLIQUIDLANEADAPTER() ([]byte, error) {
	return symbioticOevSolver.abi.Pack("LIQUID_LANE_ADAPTER")
}

// UnpackLIQUIDLANEADAPTER is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x86e7c9d0.
//
// Solidity: function LIQUID_LANE_ADAPTER() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) UnpackLIQUIDLANEADAPTER(data []byte) (common.Address, error) {
	out, err := symbioticOevSolver.abi.Unpack("LIQUID_LANE_ADAPTER", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackMORPHO is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3acb5624.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function MORPHO() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) PackMORPHO() []byte {
	enc, err := symbioticOevSolver.abi.Pack("MORPHO")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMORPHO is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3acb5624.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function MORPHO() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) TryPackMORPHO() ([]byte, error) {
	return symbioticOevSolver.abi.Pack("MORPHO")
}

// UnpackMORPHO is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x3acb5624.
//
// Solidity: function MORPHO() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) UnpackMORPHO(data []byte) (common.Address, error) {
	out, err := symbioticOevSolver.abi.Unpack("MORPHO", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackLiquidate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7ebcdf30.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function liquidate(uint256 bidAmount, address , bytes operationData) returns()
func (symbioticOevSolver *SymbioticOevSolver) PackLiquidate(bidAmount *big.Int, arg1 common.Address, operationData []byte) []byte {
	enc, err := symbioticOevSolver.abi.Pack("liquidate", bidAmount, arg1, operationData)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLiquidate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7ebcdf30.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function liquidate(uint256 bidAmount, address , bytes operationData) returns()
func (symbioticOevSolver *SymbioticOevSolver) TryPackLiquidate(bidAmount *big.Int, arg1 common.Address, operationData []byte) ([]byte, error) {
	return symbioticOevSolver.abi.Pack("liquidate", bidAmount, arg1, operationData)
}

// PackOnMorphoLiquidate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xcf7ea196.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function onMorphoLiquidate(uint256 repaidAssets, bytes data) returns()
func (symbioticOevSolver *SymbioticOevSolver) PackOnMorphoLiquidate(repaidAssets *big.Int, data []byte) []byte {
	enc, err := symbioticOevSolver.abi.Pack("onMorphoLiquidate", repaidAssets, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOnMorphoLiquidate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xcf7ea196.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function onMorphoLiquidate(uint256 repaidAssets, bytes data) returns()
func (symbioticOevSolver *SymbioticOevSolver) TryPackOnMorphoLiquidate(repaidAssets *big.Int, data []byte) ([]byte, error) {
	return symbioticOevSolver.abi.Pack("onMorphoLiquidate", repaidAssets, data)
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) PackOwner() []byte {
	enc, err := symbioticOevSolver.abi.Pack("owner")
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
func (symbioticOevSolver *SymbioticOevSolver) TryPackOwner() ([]byte, error) {
	return symbioticOevSolver.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (symbioticOevSolver *SymbioticOevSolver) UnpackOwner(data []byte) (common.Address, error) {
	out, err := symbioticOevSolver.abi.Unpack("owner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackPayBid is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1e1769ed.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function payBid(uint256 bidAmount) returns()
func (symbioticOevSolver *SymbioticOevSolver) PackPayBid(bidAmount *big.Int) []byte {
	enc, err := symbioticOevSolver.abi.Pack("payBid", bidAmount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPayBid is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1e1769ed.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function payBid(uint256 bidAmount) returns()
func (symbioticOevSolver *SymbioticOevSolver) TryPackPayBid(bidAmount *big.Int) ([]byte, error) {
	return symbioticOevSolver.abi.Pack("payBid", bidAmount)
}

// PackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (symbioticOevSolver *SymbioticOevSolver) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := symbioticOevSolver.abi.Pack("transferOwnership", newOwner)
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
func (symbioticOevSolver *SymbioticOevSolver) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return symbioticOevSolver.abi.Pack("transferOwnership", newOwner)
}

// PackUsedAuctionKey is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0f9e1b51.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function usedAuctionKey(bytes32 auctionKey) view returns(bool used)
func (symbioticOevSolver *SymbioticOevSolver) PackUsedAuctionKey(auctionKey [32]byte) []byte {
	enc, err := symbioticOevSolver.abi.Pack("usedAuctionKey", auctionKey)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackUsedAuctionKey is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0f9e1b51.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function usedAuctionKey(bytes32 auctionKey) view returns(bool used)
func (symbioticOevSolver *SymbioticOevSolver) TryPackUsedAuctionKey(auctionKey [32]byte) ([]byte, error) {
	return symbioticOevSolver.abi.Pack("usedAuctionKey", auctionKey)
}

// UnpackUsedAuctionKey is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0f9e1b51.
//
// Solidity: function usedAuctionKey(bytes32 auctionKey) view returns(bool used)
func (symbioticOevSolver *SymbioticOevSolver) UnpackUsedAuctionKey(data []byte) (bool, error) {
	out, err := symbioticOevSolver.abi.Unpack("usedAuctionKey", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackWithdrawERC20 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x44004cc1.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function withdrawERC20(address token, address to, uint256 amount) returns()
func (symbioticOevSolver *SymbioticOevSolver) PackWithdrawERC20(token common.Address, to common.Address, amount *big.Int) []byte {
	enc, err := symbioticOevSolver.abi.Pack("withdrawERC20", token, to, amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackWithdrawERC20 is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x44004cc1.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function withdrawERC20(address token, address to, uint256 amount) returns()
func (symbioticOevSolver *SymbioticOevSolver) TryPackWithdrawERC20(token common.Address, to common.Address, amount *big.Int) ([]byte, error) {
	return symbioticOevSolver.abi.Pack("withdrawERC20", token, to, amount)
}

// PackWithdrawNative is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x07b18bde.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function withdrawNative(address to, uint256 amount) returns()
func (symbioticOevSolver *SymbioticOevSolver) PackWithdrawNative(to common.Address, amount *big.Int) []byte {
	enc, err := symbioticOevSolver.abi.Pack("withdrawNative", to, amount)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackWithdrawNative is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x07b18bde.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function withdrawNative(address to, uint256 amount) returns()
func (symbioticOevSolver *SymbioticOevSolver) TryPackWithdrawNative(to common.Address, amount *big.Int) ([]byte, error) {
	return symbioticOevSolver.abi.Pack("withdrawNative", to, amount)
}

// SymbioticOevSolverBundleResult represents a BundleResult event raised by the SymbioticOevSolver contract.
type SymbioticOevSolverBundleResult struct {
	AuctionKey      [32]byte
	TotalProfitLoan *big.Int
	MinProfitLoan   *big.Int
	GasUsed         *big.Int
	BidAuthorized   bool
	Raw             *types.Log // Blockchain specific contextual infos
}

const SymbioticOevSolverBundleResultEventName = "BundleResult"

// ContractEventName returns the user-defined event name.
func (SymbioticOevSolverBundleResult) ContractEventName() string {
	return SymbioticOevSolverBundleResultEventName
}

// UnpackBundleResultEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event BundleResult(bytes32 indexed auctionKey, uint256 totalProfitLoan, uint256 minProfitLoan, uint256 gasUsed, bool bidAuthorized)
func (symbioticOevSolver *SymbioticOevSolver) UnpackBundleResultEvent(log *types.Log) (*SymbioticOevSolverBundleResult, error) {
	event := "BundleResult"
	if log.Topics[0] != symbioticOevSolver.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(SymbioticOevSolverBundleResult)
	if len(log.Data) > 0 {
		if err := symbioticOevSolver.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range symbioticOevSolver.abi.Events[event].Inputs {
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

// SymbioticOevSolverLegResult represents a LegResult event raised by the SymbioticOevSolver contract.
type SymbioticOevSolverLegResult struct {
	AuctionKey   [32]byte
	MarketId     [32]byte
	Borrower     common.Address
	Code         *big.Int
	SeizedAssets *big.Int
	RepaidAssets *big.Int
	ProfitLoan   *big.Int
	GasUsed      *big.Int
	Raw          *types.Log // Blockchain specific contextual infos
}

const SymbioticOevSolverLegResultEventName = "LegResult"

// ContractEventName returns the user-defined event name.
func (SymbioticOevSolverLegResult) ContractEventName() string {
	return SymbioticOevSolverLegResultEventName
}

// UnpackLegResultEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event LegResult(bytes32 indexed auctionKey, bytes32 indexed marketId, address indexed borrower, uint256 code, uint256 seizedAssets, uint256 repaidAssets, uint256 profitLoan, uint256 gasUsed)
func (symbioticOevSolver *SymbioticOevSolver) UnpackLegResultEvent(log *types.Log) (*SymbioticOevSolverLegResult, error) {
	event := "LegResult"
	if log.Topics[0] != symbioticOevSolver.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(SymbioticOevSolverLegResult)
	if len(log.Data) > 0 {
		if err := symbioticOevSolver.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range symbioticOevSolver.abi.Events[event].Inputs {
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

// SymbioticOevSolverOwnerUpdated represents a OwnerUpdated event raised by the SymbioticOevSolver contract.
type SymbioticOevSolverOwnerUpdated struct {
	Previous common.Address
	Next     common.Address
	Raw      *types.Log // Blockchain specific contextual infos
}

const SymbioticOevSolverOwnerUpdatedEventName = "OwnerUpdated"

// ContractEventName returns the user-defined event name.
func (SymbioticOevSolverOwnerUpdated) ContractEventName() string {
	return SymbioticOevSolverOwnerUpdatedEventName
}

// UnpackOwnerUpdatedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnerUpdated(address indexed previous, address indexed next)
func (symbioticOevSolver *SymbioticOevSolver) UnpackOwnerUpdatedEvent(log *types.Log) (*SymbioticOevSolverOwnerUpdated, error) {
	event := "OwnerUpdated"
	if log.Topics[0] != symbioticOevSolver.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(SymbioticOevSolverOwnerUpdated)
	if len(log.Data) > 0 {
		if err := symbioticOevSolver.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range symbioticOevSolver.abi.Events[event].Inputs {
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

// SymbioticOevSolverPayBidResult represents a PayBidResult event raised by the SymbioticOevSolver contract.
type SymbioticOevSolverPayBidResult struct {
	AuctionKey [32]byte
	BidAmount  *big.Int
	Paid       bool
	Raw        *types.Log // Blockchain specific contextual infos
}

const SymbioticOevSolverPayBidResultEventName = "PayBidResult"

// ContractEventName returns the user-defined event name.
func (SymbioticOevSolverPayBidResult) ContractEventName() string {
	return SymbioticOevSolverPayBidResultEventName
}

// UnpackPayBidResultEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event PayBidResult(bytes32 indexed auctionKey, uint256 bidAmount, bool paid)
func (symbioticOevSolver *SymbioticOevSolver) UnpackPayBidResultEvent(log *types.Log) (*SymbioticOevSolverPayBidResult, error) {
	event := "PayBidResult"
	if log.Topics[0] != symbioticOevSolver.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(SymbioticOevSolverPayBidResult)
	if len(log.Data) > 0 {
		if err := symbioticOevSolver.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range symbioticOevSolver.abi.Events[event].Inputs {
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
func (symbioticOevSolver *SymbioticOevSolver) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["ECDSAInvalidSignature"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackECDSAInvalidSignatureError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["ECDSAInvalidSignatureLength"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackECDSAInvalidSignatureLengthError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["ECDSAInvalidSignatureS"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackECDSAInvalidSignatureSError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["InsufficientLoanProceeds"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackInsufficientLoanProceedsError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["InvalidAuth"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackInvalidAuthError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["NotExecutor"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackNotExecutorError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["NotMorpho"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackNotMorphoError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["NotOwner"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackNotOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["ProfitBelowMin"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackProfitBelowMinError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["ReentrancyGuardReentrantCall"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackReentrancyGuardReentrantCallError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["SwapOutputBelowMin"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackSwapOutputBelowMinError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["TransferFailed"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackTransferFailedError(raw[4:])
	}
	if bytes.Equal(raw[:4], symbioticOevSolver.abi.Errors["ZeroAddress"].ID.Bytes()[:4]) {
		return symbioticOevSolver.UnpackZeroAddressError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// SymbioticOevSolverECDSAInvalidSignature represents a ECDSAInvalidSignature error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverECDSAInvalidSignature struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ECDSAInvalidSignature()
func SymbioticOevSolverECDSAInvalidSignatureErrorID() common.Hash {
	return common.HexToHash("0xf645eedf0193584640b6b90cb9477e4c95b98636c148a891d4c0a146dc46e75a")
}

// UnpackECDSAInvalidSignatureError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ECDSAInvalidSignature()
func (symbioticOevSolver *SymbioticOevSolver) UnpackECDSAInvalidSignatureError(raw []byte) (*SymbioticOevSolverECDSAInvalidSignature, error) {
	out := new(SymbioticOevSolverECDSAInvalidSignature)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "ECDSAInvalidSignature", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverECDSAInvalidSignatureLength represents a ECDSAInvalidSignatureLength error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverECDSAInvalidSignatureLength struct {
	Length *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ECDSAInvalidSignatureLength(uint256 length)
func SymbioticOevSolverECDSAInvalidSignatureLengthErrorID() common.Hash {
	return common.HexToHash("0xfce698f7e8e5342cd615f641317bc45fe7e1e4a8b0a14dd1383ff8dc9c41917f")
}

// UnpackECDSAInvalidSignatureLengthError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ECDSAInvalidSignatureLength(uint256 length)
func (symbioticOevSolver *SymbioticOevSolver) UnpackECDSAInvalidSignatureLengthError(raw []byte) (*SymbioticOevSolverECDSAInvalidSignatureLength, error) {
	out := new(SymbioticOevSolverECDSAInvalidSignatureLength)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "ECDSAInvalidSignatureLength", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverECDSAInvalidSignatureS represents a ECDSAInvalidSignatureS error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverECDSAInvalidSignatureS struct {
	S [32]byte
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ECDSAInvalidSignatureS(bytes32 s)
func SymbioticOevSolverECDSAInvalidSignatureSErrorID() common.Hash {
	return common.HexToHash("0xd78bce0cccb935155ed6428d1c13e50b7f3550fd2b66b9fe266006fea4a5e1eb")
}

// UnpackECDSAInvalidSignatureSError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ECDSAInvalidSignatureS(bytes32 s)
func (symbioticOevSolver *SymbioticOevSolver) UnpackECDSAInvalidSignatureSError(raw []byte) (*SymbioticOevSolverECDSAInvalidSignatureS, error) {
	out := new(SymbioticOevSolverECDSAInvalidSignatureS)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "ECDSAInvalidSignatureS", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverInsufficientLoanProceeds represents a InsufficientLoanProceeds error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverInsufficientLoanProceeds struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InsufficientLoanProceeds()
func SymbioticOevSolverInsufficientLoanProceedsErrorID() common.Hash {
	return common.HexToHash("0x8dff298421d9adf12071798b4a2ba2b222fcafe9e2d1885468bb553b5152ddaf")
}

// UnpackInsufficientLoanProceedsError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InsufficientLoanProceeds()
func (symbioticOevSolver *SymbioticOevSolver) UnpackInsufficientLoanProceedsError(raw []byte) (*SymbioticOevSolverInsufficientLoanProceeds, error) {
	out := new(SymbioticOevSolverInsufficientLoanProceeds)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "InsufficientLoanProceeds", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverInvalidAuth represents a InvalidAuth error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverInvalidAuth struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidAuth()
func SymbioticOevSolverInvalidAuthErrorID() common.Hash {
	return common.HexToHash("0x60907fd1eaf0aeb8678cf1ed7e0848c38b81ff6b751719093cce13e43c4aa3a7")
}

// UnpackInvalidAuthError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidAuth()
func (symbioticOevSolver *SymbioticOevSolver) UnpackInvalidAuthError(raw []byte) (*SymbioticOevSolverInvalidAuth, error) {
	out := new(SymbioticOevSolverInvalidAuth)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "InvalidAuth", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverNotExecutor represents a NotExecutor error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverNotExecutor struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotExecutor()
func SymbioticOevSolverNotExecutorErrorID() common.Hash {
	return common.HexToHash("0xc32d1d764229d81292df6f25b9d1e0888374ee366ac172b5c5162f2d6fcf3ce2")
}

// UnpackNotExecutorError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotExecutor()
func (symbioticOevSolver *SymbioticOevSolver) UnpackNotExecutorError(raw []byte) (*SymbioticOevSolverNotExecutor, error) {
	out := new(SymbioticOevSolverNotExecutor)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "NotExecutor", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverNotMorpho represents a NotMorpho error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverNotMorpho struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotMorpho()
func SymbioticOevSolverNotMorphoErrorID() common.Hash {
	return common.HexToHash("0xe51b512366538cee8c853e063e54221c196d4d7f44b7cc806f3763062d129db9")
}

// UnpackNotMorphoError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotMorpho()
func (symbioticOevSolver *SymbioticOevSolver) UnpackNotMorphoError(raw []byte) (*SymbioticOevSolverNotMorpho, error) {
	out := new(SymbioticOevSolverNotMorpho)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "NotMorpho", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverNotOwner represents a NotOwner error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverNotOwner struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotOwner()
func SymbioticOevSolverNotOwnerErrorID() common.Hash {
	return common.HexToHash("0x30cd74712f59d478562d48e2d35de830db72c60a63dd08ae59199eec990b5bc4")
}

// UnpackNotOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotOwner()
func (symbioticOevSolver *SymbioticOevSolver) UnpackNotOwnerError(raw []byte) (*SymbioticOevSolverNotOwner, error) {
	out := new(SymbioticOevSolverNotOwner)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "NotOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverProfitBelowMin represents a ProfitBelowMin error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverProfitBelowMin struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ProfitBelowMin()
func SymbioticOevSolverProfitBelowMinErrorID() common.Hash {
	return common.HexToHash("0xe42f715d4b8066279da9ab3b7d708b4d7702d6769277c477f0f130466b02a066")
}

// UnpackProfitBelowMinError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ProfitBelowMin()
func (symbioticOevSolver *SymbioticOevSolver) UnpackProfitBelowMinError(raw []byte) (*SymbioticOevSolverProfitBelowMin, error) {
	out := new(SymbioticOevSolverProfitBelowMin)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "ProfitBelowMin", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverReentrancyGuardReentrantCall represents a ReentrancyGuardReentrantCall error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverReentrancyGuardReentrantCall struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ReentrancyGuardReentrantCall()
func SymbioticOevSolverReentrancyGuardReentrantCallErrorID() common.Hash {
	return common.HexToHash("0x3ee5aeb571de7fc460830b4d0017439a1ca56fb0bc39062227ade4fe4a24c1ca")
}

// UnpackReentrancyGuardReentrantCallError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ReentrancyGuardReentrantCall()
func (symbioticOevSolver *SymbioticOevSolver) UnpackReentrancyGuardReentrantCallError(raw []byte) (*SymbioticOevSolverReentrancyGuardReentrantCall, error) {
	out := new(SymbioticOevSolverReentrancyGuardReentrantCall)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "ReentrancyGuardReentrantCall", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func SymbioticOevSolverSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (symbioticOevSolver *SymbioticOevSolver) UnpackSafeERC20FailedOperationError(raw []byte) (*SymbioticOevSolverSafeERC20FailedOperation, error) {
	out := new(SymbioticOevSolverSafeERC20FailedOperation)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverSwapOutputBelowMin represents a SwapOutputBelowMin error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverSwapOutputBelowMin struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SwapOutputBelowMin()
func SymbioticOevSolverSwapOutputBelowMinErrorID() common.Hash {
	return common.HexToHash("0x4f3f768fa41bcfdbeb273b7d91fd78101b766e92d4b008ec24772933ae5c425c")
}

// UnpackSwapOutputBelowMinError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SwapOutputBelowMin()
func (symbioticOevSolver *SymbioticOevSolver) UnpackSwapOutputBelowMinError(raw []byte) (*SymbioticOevSolverSwapOutputBelowMin, error) {
	out := new(SymbioticOevSolverSwapOutputBelowMin)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "SwapOutputBelowMin", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverTransferFailed represents a TransferFailed error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverTransferFailed struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TransferFailed()
func SymbioticOevSolverTransferFailedErrorID() common.Hash {
	return common.HexToHash("0x90b8ec1877afffd816d05d9b13947f3ff18ec5851c38bad15ec2b710f92391b1")
}

// UnpackTransferFailedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TransferFailed()
func (symbioticOevSolver *SymbioticOevSolver) UnpackTransferFailedError(raw []byte) (*SymbioticOevSolverTransferFailed, error) {
	out := new(SymbioticOevSolverTransferFailed)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "TransferFailed", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// SymbioticOevSolverZeroAddress represents a ZeroAddress error raised by the SymbioticOevSolver contract.
type SymbioticOevSolverZeroAddress struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ZeroAddress()
func SymbioticOevSolverZeroAddressErrorID() common.Hash {
	return common.HexToHash("0xd92e233df2717d4a40030e20904abd27b68fcbeede117eaaccbbdac9618c8c73")
}

// UnpackZeroAddressError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ZeroAddress()
func (symbioticOevSolver *SymbioticOevSolver) UnpackZeroAddressError(raw []byte) (*SymbioticOevSolverZeroAddress, error) {
	out := new(SymbioticOevSolverZeroAddress)
	if err := symbioticOevSolver.abi.UnpackIntoInterface(out, "ZeroAddress", raw); err != nil {
		return nil, err
	}
	return out, nil
}
