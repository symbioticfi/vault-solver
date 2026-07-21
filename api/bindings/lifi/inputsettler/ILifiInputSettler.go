// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package inputsettler

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

// InputSettlerBaseSolveParams is an auto generated low-level Go binding around an user-defined struct.
type InputSettlerBaseSolveParams struct {
	Timestamp uint32
	Solver    [32]byte
}

// MandateOutput is an auto generated low-level Go binding around an user-defined struct.
type MandateOutput struct {
	Oracle       [32]byte
	Settler      [32]byte
	ChainId      *big.Int
	Token        [32]byte
	Amount       *big.Int
	Recipient    [32]byte
	CallbackData []byte
	Context      []byte
}

// OrderPurchase is an auto generated low-level Go binding around an user-defined struct.
type OrderPurchase struct {
	OrderId     [32]byte
	Destination common.Address
	CallData    []byte
	Discount    uint64
	TimeToBuy   uint32
}

// StandardOrder is an auto generated low-level Go binding around an user-defined struct.
type StandardOrder struct {
	User          common.Address
	Nonce         *big.Int
	OriginChainId *big.Int
	Expires       uint32
	FillDeadline  uint32
	InputOracle   common.Address
	Inputs        [][2]*big.Int
	Outputs       []MandateOutput
}

// ILifiInputSettlerMetaData contains all meta data concerning the ILifiInputSettler contract.
var ILifiInputSettlerMetaData = bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"initialOwner\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"AlreadyInitialized\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"AlreadyPurchased\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"CallOutOfRange\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"CodeSize0\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ContextOutOfRange\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Expired\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"}],\"name\":\"FillDeadlineAfterExpiry\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint32\",\"name\":\"expected\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"actual\",\"type\":\"uint32\"}],\"name\":\"FilledTooLate\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"GovernanceFeeChangeNotReady\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"GovernanceFeeTooHigh\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"HasDirtyBits\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidOrderStatus\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidPurchaser\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidShortString\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidSigner\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidTimestampLength\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NewOwnerIsZeroAddress\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NoDestination\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NoHandoverRequest\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotOrderOwner\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"provided\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"computed\",\"type\":\"bytes32\"}],\"name\":\"OrderIdMismatch\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ReentrancyDetected\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"}],\"name\":\"SafeERC20FailedOperation\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"SignatureAndInputsNotEqual\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"bytes1\",\"name\":\"\",\"type\":\"bytes1\"}],\"name\":\"SignatureNotSupported\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"str\",\"type\":\"string\"}],\"name\":\"StringTooLong\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TimestampNotPassed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TimestampPassed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Unauthorized\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"expected\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"actual\",\"type\":\"uint256\"}],\"name\":\"WrongChain\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[],\"name\":\"EIP712DomainChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"solver\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"destination\",\"type\":\"bytes32\"}],\"name\":\"Finalised\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"oldGovernanceFee\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"newGovernanceFee\",\"type\":\"uint64\"}],\"name\":\"GovernanceFeeChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"nextGovernanceFee\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"nextGovernanceFeeTime\",\"type\":\"uint64\"}],\"name\":\"NextGovernanceFee\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"Open\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"indexed\":false,\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"}],\"name\":\"Open\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"solver\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"purchaser\",\"type\":\"bytes32\"}],\"name\":\"OrderPurchased\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"pendingOwner\",\"type\":\"address\"}],\"name\":\"OwnershipHandoverCanceled\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"pendingOwner\",\"type\":\"address\"}],\"name\":\"OwnershipHandoverRequested\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"oldOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"Refunded\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"DOMAIN_SEPARATOR\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"applyGovernanceFee\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"cancelOwnershipHandover\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"pendingOwner\",\"type\":\"address\"}],\"name\":\"completeOwnershipHandover\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"eip712Domain\",\"outputs\":[{\"internalType\":\"bytes1\",\"name\":\"fields\",\"type\":\"bytes1\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"version\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"verifyingContract\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"},{\"internalType\":\"uint256[]\",\"name\":\"extensions\",\"type\":\"uint256[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"},{\"components\":[{\"internalType\":\"uint32\",\"name\":\"timestamp\",\"type\":\"uint32\"},{\"internalType\":\"bytes32\",\"name\":\"solver\",\"type\":\"bytes32\"}],\"internalType\":\"structInputSettlerBase.SolveParams[]\",\"name\":\"solveParams\",\"type\":\"tuple[]\"},{\"internalType\":\"bytes32\",\"name\":\"destination\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"call\",\"type\":\"bytes\"}],\"name\":\"finalise\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"},{\"components\":[{\"internalType\":\"uint32\",\"name\":\"timestamp\",\"type\":\"uint32\"},{\"internalType\":\"bytes32\",\"name\":\"solver\",\"type\":\"bytes32\"}],\"internalType\":\"structInputSettlerBase.SolveParams[]\",\"name\":\"solveParams\",\"type\":\"tuple[]\"},{\"internalType\":\"bytes32\",\"name\":\"destination\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"call\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"orderOwnerSignature\",\"type\":\"bytes\"}],\"name\":\"finaliseWithSignature\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"governanceFee\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"nextGovernanceFee\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"nextGovernanceFeeTime\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"}],\"name\":\"open\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"},{\"internalType\":\"address\",\"name\":\"sponsor\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"openFor\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"},{\"internalType\":\"address\",\"name\":\"sponsor\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"},{\"internalType\":\"address\",\"name\":\"destination\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"call\",\"type\":\"bytes\"}],\"name\":\"openForAndFinalise\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"}],\"name\":\"orderIdentifier\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"orderStatus\",\"outputs\":[{\"internalType\":\"enumInputSettlerEscrow.OrderStatus\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"result\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"pendingOwner\",\"type\":\"address\"}],\"name\":\"ownershipHandoverExpiresAt\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"result\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"destination\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"callData\",\"type\":\"bytes\"},{\"internalType\":\"uint64\",\"name\":\"discount\",\"type\":\"uint64\"},{\"internalType\":\"uint32\",\"name\":\"timeToBuy\",\"type\":\"uint32\"}],\"internalType\":\"structOrderPurchase\",\"name\":\"orderPurchase\",\"type\":\"tuple\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"},{\"internalType\":\"bytes32\",\"name\":\"orderSolvedByIdentifier\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"purchaser\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"expiryTimestamp\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"solverSignature\",\"type\":\"bytes\"}],\"name\":\"purchaseOrder\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"solver\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"purchasedOrders\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"lastOrderTimestamp\",\"type\":\"uint32\"},{\"internalType\":\"bytes32\",\"name\":\"purchaser\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"originChainId\",\"type\":\"uint256\"},{\"internalType\":\"uint32\",\"name\":\"expires\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"fillDeadline\",\"type\":\"uint32\"},{\"internalType\":\"address\",\"name\":\"inputOracle\",\"type\":\"address\"},{\"internalType\":\"uint256[2][]\",\"name\":\"inputs\",\"type\":\"uint256[2][]\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"oracle\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"settler\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"token\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"recipient\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"callbackData\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structMandateOutput[]\",\"name\":\"outputs\",\"type\":\"tuple[]\"}],\"internalType\":\"structStandardOrder\",\"name\":\"order\",\"type\":\"tuple\"}],\"name\":\"refund\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"requestOwnershipHandover\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"_nextGovernanceFee\",\"type\":\"uint64\"}],\"name\":\"setGovernanceFee\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"}]",
	ID:  "ILifiInputSettler",
}

// ILifiInputSettler is an auto generated Go binding around an Ethereum contract.
type ILifiInputSettler struct {
	abi abi.ABI
}

// NewILifiInputSettler creates a new instance of ILifiInputSettler.
func NewILifiInputSettler() *ILifiInputSettler {
	parsed, err := ILifiInputSettlerMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &ILifiInputSettler{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *ILifiInputSettler) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address initialOwner) returns()
func (iLifiInputSettler *ILifiInputSettler) PackConstructor(initialOwner common.Address) []byte {
	enc, err := iLifiInputSettler.abi.Pack("", initialOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackDOMAINSEPARATOR is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3644e515.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (iLifiInputSettler *ILifiInputSettler) PackDOMAINSEPARATOR() []byte {
	enc, err := iLifiInputSettler.abi.Pack("DOMAIN_SEPARATOR")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDOMAINSEPARATOR is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3644e515.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (iLifiInputSettler *ILifiInputSettler) TryPackDOMAINSEPARATOR() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("DOMAIN_SEPARATOR")
}

// UnpackDOMAINSEPARATOR is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x3644e515.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (iLifiInputSettler *ILifiInputSettler) UnpackDOMAINSEPARATOR(data []byte) ([32]byte, error) {
	out, err := iLifiInputSettler.abi.Unpack("DOMAIN_SEPARATOR", data)
	if err != nil {
		return *new([32]byte), err
	}
	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)
	return out0, nil
}

// PackApplyGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8198db87.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function applyGovernanceFee() returns()
func (iLifiInputSettler *ILifiInputSettler) PackApplyGovernanceFee() []byte {
	enc, err := iLifiInputSettler.abi.Pack("applyGovernanceFee")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackApplyGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8198db87.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function applyGovernanceFee() returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackApplyGovernanceFee() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("applyGovernanceFee")
}

// PackCancelOwnershipHandover is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54d1f13d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function cancelOwnershipHandover() payable returns()
func (iLifiInputSettler *ILifiInputSettler) PackCancelOwnershipHandover() []byte {
	enc, err := iLifiInputSettler.abi.Pack("cancelOwnershipHandover")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackCancelOwnershipHandover is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54d1f13d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function cancelOwnershipHandover() payable returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackCancelOwnershipHandover() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("cancelOwnershipHandover")
}

// PackCompleteOwnershipHandover is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf04e283e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function completeOwnershipHandover(address pendingOwner) payable returns()
func (iLifiInputSettler *ILifiInputSettler) PackCompleteOwnershipHandover(pendingOwner common.Address) []byte {
	enc, err := iLifiInputSettler.abi.Pack("completeOwnershipHandover", pendingOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackCompleteOwnershipHandover is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf04e283e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function completeOwnershipHandover(address pendingOwner) payable returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackCompleteOwnershipHandover(pendingOwner common.Address) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("completeOwnershipHandover", pendingOwner)
}

// PackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (iLifiInputSettler *ILifiInputSettler) PackEip712Domain() []byte {
	enc, err := iLifiInputSettler.abi.Pack("eip712Domain")
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
func (iLifiInputSettler *ILifiInputSettler) TryPackEip712Domain() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("eip712Domain")
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
func (iLifiInputSettler *ILifiInputSettler) UnpackEip712Domain(data []byte) (Eip712DomainOutput, error) {
	out, err := iLifiInputSettler.abi.Unpack("eip712Domain", data)
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

// PackFinalise is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbab36441.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function finalise((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, (uint32,bytes32)[] solveParams, bytes32 destination, bytes call) returns()
func (iLifiInputSettler *ILifiInputSettler) PackFinalise(order StandardOrder, solveParams []InputSettlerBaseSolveParams, destination [32]byte, call []byte) []byte {
	enc, err := iLifiInputSettler.abi.Pack("finalise", order, solveParams, destination, call)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFinalise is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xbab36441.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function finalise((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, (uint32,bytes32)[] solveParams, bytes32 destination, bytes call) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackFinalise(order StandardOrder, solveParams []InputSettlerBaseSolveParams, destination [32]byte, call []byte) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("finalise", order, solveParams, destination, call)
}

// PackFinaliseWithSignature is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x73ce1aaa.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function finaliseWithSignature((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, (uint32,bytes32)[] solveParams, bytes32 destination, bytes call, bytes orderOwnerSignature) returns()
func (iLifiInputSettler *ILifiInputSettler) PackFinaliseWithSignature(order StandardOrder, solveParams []InputSettlerBaseSolveParams, destination [32]byte, call []byte, orderOwnerSignature []byte) []byte {
	enc, err := iLifiInputSettler.abi.Pack("finaliseWithSignature", order, solveParams, destination, call, orderOwnerSignature)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFinaliseWithSignature is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x73ce1aaa.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function finaliseWithSignature((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, (uint32,bytes32)[] solveParams, bytes32 destination, bytes call, bytes orderOwnerSignature) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackFinaliseWithSignature(order StandardOrder, solveParams []InputSettlerBaseSolveParams, destination [32]byte, call []byte, orderOwnerSignature []byte) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("finaliseWithSignature", order, solveParams, destination, call, orderOwnerSignature)
}

// PackGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ea90a12.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function governanceFee() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) PackGovernanceFee() []byte {
	enc, err := iLifiInputSettler.abi.Pack("governanceFee")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x0ea90a12.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function governanceFee() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) TryPackGovernanceFee() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("governanceFee")
}

// UnpackGovernanceFee is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x0ea90a12.
//
// Solidity: function governanceFee() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) UnpackGovernanceFee(data []byte) (uint64, error) {
	out, err := iLifiInputSettler.abi.Unpack("governanceFee", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// PackNextGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc0e31352.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function nextGovernanceFee() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) PackNextGovernanceFee() []byte {
	enc, err := iLifiInputSettler.abi.Pack("nextGovernanceFee")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackNextGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc0e31352.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function nextGovernanceFee() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) TryPackNextGovernanceFee() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("nextGovernanceFee")
}

// UnpackNextGovernanceFee is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc0e31352.
//
// Solidity: function nextGovernanceFee() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) UnpackNextGovernanceFee(data []byte) (uint64, error) {
	out, err := iLifiInputSettler.abi.Unpack("nextGovernanceFee", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// PackNextGovernanceFeeTime is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5791edc0.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function nextGovernanceFeeTime() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) PackNextGovernanceFeeTime() []byte {
	enc, err := iLifiInputSettler.abi.Pack("nextGovernanceFeeTime")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackNextGovernanceFeeTime is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5791edc0.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function nextGovernanceFeeTime() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) TryPackNextGovernanceFeeTime() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("nextGovernanceFeeTime")
}

// UnpackNextGovernanceFeeTime is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5791edc0.
//
// Solidity: function nextGovernanceFeeTime() view returns(uint64)
func (iLifiInputSettler *ILifiInputSettler) UnpackNextGovernanceFeeTime(data []byte) (uint64, error) {
	out, err := iLifiInputSettler.abi.Unpack("nextGovernanceFeeTime", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// PackOpen is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7515fd56.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function open((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order) returns()
func (iLifiInputSettler *ILifiInputSettler) PackOpen(order StandardOrder) []byte {
	enc, err := iLifiInputSettler.abi.Pack("open", order)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOpen is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7515fd56.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function open((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackOpen(order StandardOrder) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("open", order)
}

// PackOpenFor is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x49927074.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function openFor((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, address sponsor, bytes signature) returns()
func (iLifiInputSettler *ILifiInputSettler) PackOpenFor(order StandardOrder, sponsor common.Address, signature []byte) []byte {
	enc, err := iLifiInputSettler.abi.Pack("openFor", order, sponsor, signature)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOpenFor is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x49927074.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function openFor((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, address sponsor, bytes signature) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackOpenFor(order StandardOrder, sponsor common.Address, signature []byte) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("openFor", order, sponsor, signature)
}

// PackOpenForAndFinalise is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xafe55c7e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function openForAndFinalise((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, address sponsor, bytes signature, address destination, bytes call) returns()
func (iLifiInputSettler *ILifiInputSettler) PackOpenForAndFinalise(order StandardOrder, sponsor common.Address, signature []byte, destination common.Address, call []byte) []byte {
	enc, err := iLifiInputSettler.abi.Pack("openForAndFinalise", order, sponsor, signature, destination, call)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOpenForAndFinalise is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xafe55c7e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function openForAndFinalise((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, address sponsor, bytes signature, address destination, bytes call) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackOpenForAndFinalise(order StandardOrder, sponsor common.Address, signature []byte, destination common.Address, call []byte) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("openForAndFinalise", order, sponsor, signature, destination, call)
}

// PackOrderIdentifier is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x609dbfa0.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function orderIdentifier((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order) view returns(bytes32)
func (iLifiInputSettler *ILifiInputSettler) PackOrderIdentifier(order StandardOrder) []byte {
	enc, err := iLifiInputSettler.abi.Pack("orderIdentifier", order)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOrderIdentifier is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x609dbfa0.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function orderIdentifier((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order) view returns(bytes32)
func (iLifiInputSettler *ILifiInputSettler) TryPackOrderIdentifier(order StandardOrder) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("orderIdentifier", order)
}

// UnpackOrderIdentifier is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x609dbfa0.
//
// Solidity: function orderIdentifier((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order) view returns(bytes32)
func (iLifiInputSettler *ILifiInputSettler) UnpackOrderIdentifier(data []byte) ([32]byte, error) {
	out, err := iLifiInputSettler.abi.Unpack("orderIdentifier", data)
	if err != nil {
		return *new([32]byte), err
	}
	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)
	return out0, nil
}

// PackOrderStatus is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dff692d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function orderStatus(bytes32 orderId) view returns(uint8)
func (iLifiInputSettler *ILifiInputSettler) PackOrderStatus(orderId [32]byte) []byte {
	enc, err := iLifiInputSettler.abi.Pack("orderStatus", orderId)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOrderStatus is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x2dff692d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function orderStatus(bytes32 orderId) view returns(uint8)
func (iLifiInputSettler *ILifiInputSettler) TryPackOrderStatus(orderId [32]byte) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("orderStatus", orderId)
}

// UnpackOrderStatus is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x2dff692d.
//
// Solidity: function orderStatus(bytes32 orderId) view returns(uint8)
func (iLifiInputSettler *ILifiInputSettler) UnpackOrderStatus(data []byte) (uint8, error) {
	out, err := iLifiInputSettler.abi.Unpack("orderStatus", data)
	if err != nil {
		return *new(uint8), err
	}
	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)
	return out0, nil
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address result)
func (iLifiInputSettler *ILifiInputSettler) PackOwner() []byte {
	enc, err := iLifiInputSettler.abi.Pack("owner")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function owner() view returns(address result)
func (iLifiInputSettler *ILifiInputSettler) TryPackOwner() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address result)
func (iLifiInputSettler *ILifiInputSettler) UnpackOwner(data []byte) (common.Address, error) {
	out, err := iLifiInputSettler.abi.Unpack("owner", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackOwnershipHandoverExpiresAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfee81cf4.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function ownershipHandoverExpiresAt(address pendingOwner) view returns(uint256 result)
func (iLifiInputSettler *ILifiInputSettler) PackOwnershipHandoverExpiresAt(pendingOwner common.Address) []byte {
	enc, err := iLifiInputSettler.abi.Pack("ownershipHandoverExpiresAt", pendingOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOwnershipHandoverExpiresAt is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfee81cf4.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function ownershipHandoverExpiresAt(address pendingOwner) view returns(uint256 result)
func (iLifiInputSettler *ILifiInputSettler) TryPackOwnershipHandoverExpiresAt(pendingOwner common.Address) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("ownershipHandoverExpiresAt", pendingOwner)
}

// UnpackOwnershipHandoverExpiresAt is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfee81cf4.
//
// Solidity: function ownershipHandoverExpiresAt(address pendingOwner) view returns(uint256 result)
func (iLifiInputSettler *ILifiInputSettler) UnpackOwnershipHandoverExpiresAt(data []byte) (*big.Int, error) {
	out, err := iLifiInputSettler.abi.Unpack("ownershipHandoverExpiresAt", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackPurchaseOrder is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x72903ef8.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function purchaseOrder((bytes32,address,bytes,uint64,uint32) orderPurchase, (address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, bytes32 orderSolvedByIdentifier, bytes32 purchaser, uint256 expiryTimestamp, bytes solverSignature) returns()
func (iLifiInputSettler *ILifiInputSettler) PackPurchaseOrder(orderPurchase OrderPurchase, order StandardOrder, orderSolvedByIdentifier [32]byte, purchaser [32]byte, expiryTimestamp *big.Int, solverSignature []byte) []byte {
	enc, err := iLifiInputSettler.abi.Pack("purchaseOrder", orderPurchase, order, orderSolvedByIdentifier, purchaser, expiryTimestamp, solverSignature)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPurchaseOrder is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x72903ef8.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function purchaseOrder((bytes32,address,bytes,uint64,uint32) orderPurchase, (address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, bytes32 orderSolvedByIdentifier, bytes32 purchaser, uint256 expiryTimestamp, bytes solverSignature) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackPurchaseOrder(orderPurchase OrderPurchase, order StandardOrder, orderSolvedByIdentifier [32]byte, purchaser [32]byte, expiryTimestamp *big.Int, solverSignature []byte) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("purchaseOrder", orderPurchase, order, orderSolvedByIdentifier, purchaser, expiryTimestamp, solverSignature)
}

// PackPurchasedOrders is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9efa6120.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function purchasedOrders(bytes32 solver, bytes32 orderId) view returns(uint32 lastOrderTimestamp, bytes32 purchaser)
func (iLifiInputSettler *ILifiInputSettler) PackPurchasedOrders(solver [32]byte, orderId [32]byte) []byte {
	enc, err := iLifiInputSettler.abi.Pack("purchasedOrders", solver, orderId)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPurchasedOrders is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9efa6120.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function purchasedOrders(bytes32 solver, bytes32 orderId) view returns(uint32 lastOrderTimestamp, bytes32 purchaser)
func (iLifiInputSettler *ILifiInputSettler) TryPackPurchasedOrders(solver [32]byte, orderId [32]byte) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("purchasedOrders", solver, orderId)
}

// PurchasedOrdersOutput serves as a container for the return parameters of contract
// method PurchasedOrders.
type PurchasedOrdersOutput struct {
	LastOrderTimestamp uint32
	Purchaser          [32]byte
}

// UnpackPurchasedOrders is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x9efa6120.
//
// Solidity: function purchasedOrders(bytes32 solver, bytes32 orderId) view returns(uint32 lastOrderTimestamp, bytes32 purchaser)
func (iLifiInputSettler *ILifiInputSettler) UnpackPurchasedOrders(data []byte) (PurchasedOrdersOutput, error) {
	out, err := iLifiInputSettler.abi.Unpack("purchasedOrders", data)
	outstruct := new(PurchasedOrdersOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.LastOrderTimestamp = *abi.ConvertType(out[0], new(uint32)).(*uint32)
	outstruct.Purchaser = *abi.ConvertType(out[1], new([32]byte)).(*[32]byte)
	return *outstruct, nil
}

// PackRefund is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x48f49eaf.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function refund((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order) returns()
func (iLifiInputSettler *ILifiInputSettler) PackRefund(order StandardOrder) []byte {
	enc, err := iLifiInputSettler.abi.Pack("refund", order)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRefund is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x48f49eaf.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function refund((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackRefund(order StandardOrder) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("refund", order)
}

// PackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function renounceOwnership() payable returns()
func (iLifiInputSettler *ILifiInputSettler) PackRenounceOwnership() []byte {
	enc, err := iLifiInputSettler.abi.Pack("renounceOwnership")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRenounceOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x715018a6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function renounceOwnership() payable returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackRenounceOwnership() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("renounceOwnership")
}

// PackRequestOwnershipHandover is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x25692962.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function requestOwnershipHandover() payable returns()
func (iLifiInputSettler *ILifiInputSettler) PackRequestOwnershipHandover() []byte {
	enc, err := iLifiInputSettler.abi.Pack("requestOwnershipHandover")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackRequestOwnershipHandover is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x25692962.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function requestOwnershipHandover() payable returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackRequestOwnershipHandover() ([]byte, error) {
	return iLifiInputSettler.abi.Pack("requestOwnershipHandover")
}

// PackSetGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x586f9800.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setGovernanceFee(uint64 _nextGovernanceFee) returns()
func (iLifiInputSettler *ILifiInputSettler) PackSetGovernanceFee(nextGovernanceFee uint64) []byte {
	enc, err := iLifiInputSettler.abi.Pack("setGovernanceFee", nextGovernanceFee)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetGovernanceFee is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x586f9800.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setGovernanceFee(uint64 _nextGovernanceFee) returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackSetGovernanceFee(nextGovernanceFee uint64) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("setGovernanceFee", nextGovernanceFee)
}

// PackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferOwnership(address newOwner) payable returns()
func (iLifiInputSettler *ILifiInputSettler) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := iLifiInputSettler.abi.Pack("transferOwnership", newOwner)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function transferOwnership(address newOwner) payable returns()
func (iLifiInputSettler *ILifiInputSettler) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return iLifiInputSettler.abi.Pack("transferOwnership", newOwner)
}

// ILifiInputSettlerEIP712DomainChanged represents a EIP712DomainChanged event raised by the ILifiInputSettler contract.
type ILifiInputSettlerEIP712DomainChanged struct {
	Raw *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerEIP712DomainChangedEventName = "EIP712DomainChanged"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerEIP712DomainChanged) ContractEventName() string {
	return ILifiInputSettlerEIP712DomainChangedEventName
}

// UnpackEIP712DomainChangedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event EIP712DomainChanged()
func (iLifiInputSettler *ILifiInputSettler) UnpackEIP712DomainChangedEvent(log *types.Log) (*ILifiInputSettlerEIP712DomainChanged, error) {
	event := "EIP712DomainChanged"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerEIP712DomainChanged)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerFinalised represents a Finalised event raised by the ILifiInputSettler contract.
type ILifiInputSettlerFinalised struct {
	OrderId     [32]byte
	Solver      [32]byte
	Destination [32]byte
	Raw         *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerFinalisedEventName = "Finalised"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerFinalised) ContractEventName() string {
	return ILifiInputSettlerFinalisedEventName
}

// UnpackFinalisedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Finalised(bytes32 indexed orderId, bytes32 solver, bytes32 destination)
func (iLifiInputSettler *ILifiInputSettler) UnpackFinalisedEvent(log *types.Log) (*ILifiInputSettlerFinalised, error) {
	event := "Finalised"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerFinalised)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerGovernanceFeeChanged represents a GovernanceFeeChanged event raised by the ILifiInputSettler contract.
type ILifiInputSettlerGovernanceFeeChanged struct {
	OldGovernanceFee uint64
	NewGovernanceFee uint64
	Raw              *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerGovernanceFeeChangedEventName = "GovernanceFeeChanged"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerGovernanceFeeChanged) ContractEventName() string {
	return ILifiInputSettlerGovernanceFeeChangedEventName
}

// UnpackGovernanceFeeChangedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event GovernanceFeeChanged(uint64 oldGovernanceFee, uint64 newGovernanceFee)
func (iLifiInputSettler *ILifiInputSettler) UnpackGovernanceFeeChangedEvent(log *types.Log) (*ILifiInputSettlerGovernanceFeeChanged, error) {
	event := "GovernanceFeeChanged"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerGovernanceFeeChanged)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerNextGovernanceFee represents a NextGovernanceFee event raised by the ILifiInputSettler contract.
type ILifiInputSettlerNextGovernanceFee struct {
	NextGovernanceFee     uint64
	NextGovernanceFeeTime uint64
	Raw                   *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerNextGovernanceFeeEventName = "NextGovernanceFee"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerNextGovernanceFee) ContractEventName() string {
	return ILifiInputSettlerNextGovernanceFeeEventName
}

// UnpackNextGovernanceFeeEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event NextGovernanceFee(uint64 nextGovernanceFee, uint64 nextGovernanceFeeTime)
func (iLifiInputSettler *ILifiInputSettler) UnpackNextGovernanceFeeEvent(log *types.Log) (*ILifiInputSettlerNextGovernanceFee, error) {
	event := "NextGovernanceFee"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerNextGovernanceFee)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerOpen represents a Open event raised by the ILifiInputSettler contract.
type ILifiInputSettlerOpen struct {
	OrderId [32]byte
	Raw     *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerOpenEventName = "Open"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerOpen) ContractEventName() string {
	return ILifiInputSettlerOpenEventName
}

// UnpackOpenEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Open(bytes32 indexed orderId)
func (iLifiInputSettler *ILifiInputSettler) UnpackOpenEvent(log *types.Log) (*ILifiInputSettlerOpen, error) {
	event := "Open"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerOpen)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerOpen0 represents a Open0 event raised by the ILifiInputSettler contract.
type ILifiInputSettlerOpen0 struct {
	OrderId [32]byte
	Order   StandardOrder
	Raw     *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerOpen0EventName = "Open0"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerOpen0) ContractEventName() string {
	return ILifiInputSettlerOpen0EventName
}

// UnpackOpen0Event is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Open(bytes32 indexed orderId, (address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order)
func (iLifiInputSettler *ILifiInputSettler) UnpackOpen0Event(log *types.Log) (*ILifiInputSettlerOpen0, error) {
	event := "Open0"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerOpen0)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerOrderPurchased represents a OrderPurchased event raised by the ILifiInputSettler contract.
type ILifiInputSettlerOrderPurchased struct {
	OrderId   [32]byte
	Solver    [32]byte
	Purchaser [32]byte
	Raw       *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerOrderPurchasedEventName = "OrderPurchased"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerOrderPurchased) ContractEventName() string {
	return ILifiInputSettlerOrderPurchasedEventName
}

// UnpackOrderPurchasedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OrderPurchased(bytes32 indexed orderId, bytes32 solver, bytes32 purchaser)
func (iLifiInputSettler *ILifiInputSettler) UnpackOrderPurchasedEvent(log *types.Log) (*ILifiInputSettlerOrderPurchased, error) {
	event := "OrderPurchased"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerOrderPurchased)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerOwnershipHandoverCanceled represents a OwnershipHandoverCanceled event raised by the ILifiInputSettler contract.
type ILifiInputSettlerOwnershipHandoverCanceled struct {
	PendingOwner common.Address
	Raw          *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerOwnershipHandoverCanceledEventName = "OwnershipHandoverCanceled"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerOwnershipHandoverCanceled) ContractEventName() string {
	return ILifiInputSettlerOwnershipHandoverCanceledEventName
}

// UnpackOwnershipHandoverCanceledEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipHandoverCanceled(address indexed pendingOwner)
func (iLifiInputSettler *ILifiInputSettler) UnpackOwnershipHandoverCanceledEvent(log *types.Log) (*ILifiInputSettlerOwnershipHandoverCanceled, error) {
	event := "OwnershipHandoverCanceled"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerOwnershipHandoverCanceled)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerOwnershipHandoverRequested represents a OwnershipHandoverRequested event raised by the ILifiInputSettler contract.
type ILifiInputSettlerOwnershipHandoverRequested struct {
	PendingOwner common.Address
	Raw          *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerOwnershipHandoverRequestedEventName = "OwnershipHandoverRequested"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerOwnershipHandoverRequested) ContractEventName() string {
	return ILifiInputSettlerOwnershipHandoverRequestedEventName
}

// UnpackOwnershipHandoverRequestedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipHandoverRequested(address indexed pendingOwner)
func (iLifiInputSettler *ILifiInputSettler) UnpackOwnershipHandoverRequestedEvent(log *types.Log) (*ILifiInputSettlerOwnershipHandoverRequested, error) {
	event := "OwnershipHandoverRequested"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerOwnershipHandoverRequested)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerOwnershipTransferred represents a OwnershipTransferred event raised by the ILifiInputSettler contract.
type ILifiInputSettlerOwnershipTransferred struct {
	OldOwner common.Address
	NewOwner common.Address
	Raw      *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerOwnershipTransferred) ContractEventName() string {
	return ILifiInputSettlerOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed oldOwner, address indexed newOwner)
func (iLifiInputSettler *ILifiInputSettler) UnpackOwnershipTransferredEvent(log *types.Log) (*ILifiInputSettlerOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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

// ILifiInputSettlerRefunded represents a Refunded event raised by the ILifiInputSettler contract.
type ILifiInputSettlerRefunded struct {
	OrderId [32]byte
	Raw     *types.Log // Blockchain specific contextual infos
}

const ILifiInputSettlerRefundedEventName = "Refunded"

// ContractEventName returns the user-defined event name.
func (ILifiInputSettlerRefunded) ContractEventName() string {
	return ILifiInputSettlerRefundedEventName
}

// UnpackRefundedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Refunded(bytes32 indexed orderId)
func (iLifiInputSettler *ILifiInputSettler) UnpackRefundedEvent(log *types.Log) (*ILifiInputSettlerRefunded, error) {
	event := "Refunded"
	if log.Topics[0] != iLifiInputSettler.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(ILifiInputSettlerRefunded)
	if len(log.Data) > 0 {
		if err := iLifiInputSettler.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iLifiInputSettler.abi.Events[event].Inputs {
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
func (iLifiInputSettler *ILifiInputSettler) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["AlreadyInitialized"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackAlreadyInitializedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["AlreadyPurchased"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackAlreadyPurchasedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["CallOutOfRange"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackCallOutOfRangeError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["CodeSize0"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackCodeSize0Error(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["ContextOutOfRange"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackContextOutOfRangeError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["Expired"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackExpiredError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["FillDeadlineAfterExpiry"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackFillDeadlineAfterExpiryError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["FilledTooLate"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackFilledTooLateError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["GovernanceFeeChangeNotReady"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackGovernanceFeeChangeNotReadyError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["GovernanceFeeTooHigh"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackGovernanceFeeTooHighError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["HasDirtyBits"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackHasDirtyBitsError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["InvalidOrderStatus"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackInvalidOrderStatusError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["InvalidPurchaser"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackInvalidPurchaserError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["InvalidShortString"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackInvalidShortStringError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["InvalidSigner"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackInvalidSignerError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["InvalidTimestampLength"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackInvalidTimestampLengthError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["NewOwnerIsZeroAddress"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackNewOwnerIsZeroAddressError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["NoDestination"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackNoDestinationError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["NoHandoverRequest"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackNoHandoverRequestError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["NotOrderOwner"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackNotOrderOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["OrderIdMismatch"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackOrderIdMismatchError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["ReentrancyDetected"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackReentrancyDetectedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["SignatureAndInputsNotEqual"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackSignatureAndInputsNotEqualError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["SignatureNotSupported"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackSignatureNotSupportedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["StringTooLong"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackStringTooLongError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["TimestampNotPassed"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackTimestampNotPassedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["TimestampPassed"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackTimestampPassedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["Unauthorized"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackUnauthorizedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iLifiInputSettler.abi.Errors["WrongChain"].ID.Bytes()[:4]) {
		return iLifiInputSettler.UnpackWrongChainError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// ILifiInputSettlerAlreadyInitialized represents a AlreadyInitialized error raised by the ILifiInputSettler contract.
type ILifiInputSettlerAlreadyInitialized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyInitialized()
func ILifiInputSettlerAlreadyInitializedErrorID() common.Hash {
	return common.HexToHash("0x0dc149f07762891dbcea3fe72770f3d63a1863fc54b2f084e8c59ec476996927")
}

// UnpackAlreadyInitializedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyInitialized()
func (iLifiInputSettler *ILifiInputSettler) UnpackAlreadyInitializedError(raw []byte) (*ILifiInputSettlerAlreadyInitialized, error) {
	out := new(ILifiInputSettlerAlreadyInitialized)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "AlreadyInitialized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerAlreadyPurchased represents a AlreadyPurchased error raised by the ILifiInputSettler contract.
type ILifiInputSettlerAlreadyPurchased struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyPurchased()
func ILifiInputSettlerAlreadyPurchasedErrorID() common.Hash {
	return common.HexToHash("0x3367b554dccf0f6b7e731388e7b58cf6b61aa57a5d2d9b20798abf1e9a9eb9d9")
}

// UnpackAlreadyPurchasedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyPurchased()
func (iLifiInputSettler *ILifiInputSettler) UnpackAlreadyPurchasedError(raw []byte) (*ILifiInputSettlerAlreadyPurchased, error) {
	out := new(ILifiInputSettlerAlreadyPurchased)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "AlreadyPurchased", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerCallOutOfRange represents a CallOutOfRange error raised by the ILifiInputSettler contract.
type ILifiInputSettlerCallOutOfRange struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error CallOutOfRange()
func ILifiInputSettlerCallOutOfRangeErrorID() common.Hash {
	return common.HexToHash("0x4fe9ad238b0efcfdcc07e41ff080de6477c45b0a2b23e6a1710bf7a4561340e9")
}

// UnpackCallOutOfRangeError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error CallOutOfRange()
func (iLifiInputSettler *ILifiInputSettler) UnpackCallOutOfRangeError(raw []byte) (*ILifiInputSettlerCallOutOfRange, error) {
	out := new(ILifiInputSettlerCallOutOfRange)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "CallOutOfRange", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerCodeSize0 represents a CodeSize0 error raised by the ILifiInputSettler contract.
type ILifiInputSettlerCodeSize0 struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error CodeSize0()
func ILifiInputSettlerCodeSize0ErrorID() common.Hash {
	return common.HexToHash("0xfbc1d8e2c3f2772770ee2062b2b56e4b23e4e91332347f7656f0f8aafbb9cb0c")
}

// UnpackCodeSize0Error is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error CodeSize0()
func (iLifiInputSettler *ILifiInputSettler) UnpackCodeSize0Error(raw []byte) (*ILifiInputSettlerCodeSize0, error) {
	out := new(ILifiInputSettlerCodeSize0)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "CodeSize0", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerContextOutOfRange represents a ContextOutOfRange error raised by the ILifiInputSettler contract.
type ILifiInputSettlerContextOutOfRange struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ContextOutOfRange()
func ILifiInputSettlerContextOutOfRangeErrorID() common.Hash {
	return common.HexToHash("0xd94d6ce6aedb93cc32ffa64d0fd16f10262e85593f5410fe9a0c38743fb09af7")
}

// UnpackContextOutOfRangeError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ContextOutOfRange()
func (iLifiInputSettler *ILifiInputSettler) UnpackContextOutOfRangeError(raw []byte) (*ILifiInputSettlerContextOutOfRange, error) {
	out := new(ILifiInputSettlerContextOutOfRange)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "ContextOutOfRange", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerExpired represents a Expired error raised by the ILifiInputSettler contract.
type ILifiInputSettlerExpired struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error Expired()
func ILifiInputSettlerExpiredErrorID() common.Hash {
	return common.HexToHash("0x203d82d8d99f63bfecc8335216735e0271df4249ea752b030f9ab305b94e5afe")
}

// UnpackExpiredError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error Expired()
func (iLifiInputSettler *ILifiInputSettler) UnpackExpiredError(raw []byte) (*ILifiInputSettlerExpired, error) {
	out := new(ILifiInputSettlerExpired)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "Expired", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerFillDeadlineAfterExpiry represents a FillDeadlineAfterExpiry error raised by the ILifiInputSettler contract.
type ILifiInputSettlerFillDeadlineAfterExpiry struct {
	FillDeadline uint32
	Expires      uint32
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error FillDeadlineAfterExpiry(uint32 fillDeadline, uint32 expires)
func ILifiInputSettlerFillDeadlineAfterExpiryErrorID() common.Hash {
	return common.HexToHash("0xf31549efc20c86d21b99f1bedbff489d9bf9d83f68b5771ceeb0cadd440a8415")
}

// UnpackFillDeadlineAfterExpiryError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error FillDeadlineAfterExpiry(uint32 fillDeadline, uint32 expires)
func (iLifiInputSettler *ILifiInputSettler) UnpackFillDeadlineAfterExpiryError(raw []byte) (*ILifiInputSettlerFillDeadlineAfterExpiry, error) {
	out := new(ILifiInputSettlerFillDeadlineAfterExpiry)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "FillDeadlineAfterExpiry", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerFilledTooLate represents a FilledTooLate error raised by the ILifiInputSettler contract.
type ILifiInputSettlerFilledTooLate struct {
	Expected uint32
	Actual   uint32
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error FilledTooLate(uint32 expected, uint32 actual)
func ILifiInputSettlerFilledTooLateErrorID() common.Hash {
	return common.HexToHash("0x0ad67c09a1e19240ccd1a72ebab6667d70cc8087485302bc38f12238e5e9d074")
}

// UnpackFilledTooLateError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error FilledTooLate(uint32 expected, uint32 actual)
func (iLifiInputSettler *ILifiInputSettler) UnpackFilledTooLateError(raw []byte) (*ILifiInputSettlerFilledTooLate, error) {
	out := new(ILifiInputSettlerFilledTooLate)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "FilledTooLate", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerGovernanceFeeChangeNotReady represents a GovernanceFeeChangeNotReady error raised by the ILifiInputSettler contract.
type ILifiInputSettlerGovernanceFeeChangeNotReady struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error GovernanceFeeChangeNotReady()
func ILifiInputSettlerGovernanceFeeChangeNotReadyErrorID() common.Hash {
	return common.HexToHash("0x6f4cfed1c34a227615bf9d3fb4f3149b79498b8ff3c30e5c7dba10fc2c31e408")
}

// UnpackGovernanceFeeChangeNotReadyError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error GovernanceFeeChangeNotReady()
func (iLifiInputSettler *ILifiInputSettler) UnpackGovernanceFeeChangeNotReadyError(raw []byte) (*ILifiInputSettlerGovernanceFeeChangeNotReady, error) {
	out := new(ILifiInputSettlerGovernanceFeeChangeNotReady)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "GovernanceFeeChangeNotReady", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerGovernanceFeeTooHigh represents a GovernanceFeeTooHigh error raised by the ILifiInputSettler contract.
type ILifiInputSettlerGovernanceFeeTooHigh struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error GovernanceFeeTooHigh()
func ILifiInputSettlerGovernanceFeeTooHighErrorID() common.Hash {
	return common.HexToHash("0x0f4820d8a6b3e19893860b79e29977fda9aa6ef4b2e1a7d09c8e8955b69be56c")
}

// UnpackGovernanceFeeTooHighError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error GovernanceFeeTooHigh()
func (iLifiInputSettler *ILifiInputSettler) UnpackGovernanceFeeTooHighError(raw []byte) (*ILifiInputSettlerGovernanceFeeTooHigh, error) {
	out := new(ILifiInputSettlerGovernanceFeeTooHigh)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "GovernanceFeeTooHigh", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerHasDirtyBits represents a HasDirtyBits error raised by the ILifiInputSettler contract.
type ILifiInputSettlerHasDirtyBits struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error HasDirtyBits()
func ILifiInputSettlerHasDirtyBitsErrorID() common.Hash {
	return common.HexToHash("0x5f3d6d4f57bdccabacd05058457a7e7ae88d95331a81a9def1d147b62fdf9eab")
}

// UnpackHasDirtyBitsError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error HasDirtyBits()
func (iLifiInputSettler *ILifiInputSettler) UnpackHasDirtyBitsError(raw []byte) (*ILifiInputSettlerHasDirtyBits, error) {
	out := new(ILifiInputSettlerHasDirtyBits)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "HasDirtyBits", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerInvalidOrderStatus represents a InvalidOrderStatus error raised by the ILifiInputSettler contract.
type ILifiInputSettlerInvalidOrderStatus struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidOrderStatus()
func ILifiInputSettlerInvalidOrderStatusErrorID() common.Hash {
	return common.HexToHash("0x2916ae33cf4ed00872aaf269c86d13a12e9ad47f836db89ea191297fecc7a2e7")
}

// UnpackInvalidOrderStatusError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidOrderStatus()
func (iLifiInputSettler *ILifiInputSettler) UnpackInvalidOrderStatusError(raw []byte) (*ILifiInputSettlerInvalidOrderStatus, error) {
	out := new(ILifiInputSettlerInvalidOrderStatus)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "InvalidOrderStatus", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerInvalidPurchaser represents a InvalidPurchaser error raised by the ILifiInputSettler contract.
type ILifiInputSettlerInvalidPurchaser struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidPurchaser()
func ILifiInputSettlerInvalidPurchaserErrorID() common.Hash {
	return common.HexToHash("0xcf7899a1ea308d1129fe0e01fbd4fdca283f8c93391f8c697f69a9b2d02d339e")
}

// UnpackInvalidPurchaserError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidPurchaser()
func (iLifiInputSettler *ILifiInputSettler) UnpackInvalidPurchaserError(raw []byte) (*ILifiInputSettlerInvalidPurchaser, error) {
	out := new(ILifiInputSettlerInvalidPurchaser)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "InvalidPurchaser", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerInvalidShortString represents a InvalidShortString error raised by the ILifiInputSettler contract.
type ILifiInputSettlerInvalidShortString struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidShortString()
func ILifiInputSettlerInvalidShortStringErrorID() common.Hash {
	return common.HexToHash("0xb3512b0c6163e5f0bafab72bb631b9d58cd7a731b082f910338aa21c83d5c274")
}

// UnpackInvalidShortStringError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidShortString()
func (iLifiInputSettler *ILifiInputSettler) UnpackInvalidShortStringError(raw []byte) (*ILifiInputSettlerInvalidShortString, error) {
	out := new(ILifiInputSettlerInvalidShortString)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "InvalidShortString", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerInvalidSigner represents a InvalidSigner error raised by the ILifiInputSettler contract.
type ILifiInputSettlerInvalidSigner struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidSigner()
func ILifiInputSettlerInvalidSignerErrorID() common.Hash {
	return common.HexToHash("0x815e1d64efb74fbe314c20a2b8a2335d18bce12a19165e447fa36bcb35959528")
}

// UnpackInvalidSignerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidSigner()
func (iLifiInputSettler *ILifiInputSettler) UnpackInvalidSignerError(raw []byte) (*ILifiInputSettlerInvalidSigner, error) {
	out := new(ILifiInputSettlerInvalidSigner)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "InvalidSigner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerInvalidTimestampLength represents a InvalidTimestampLength error raised by the ILifiInputSettler contract.
type ILifiInputSettlerInvalidTimestampLength struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidTimestampLength()
func ILifiInputSettlerInvalidTimestampLengthErrorID() common.Hash {
	return common.HexToHash("0x12d486097b64be32f9dcb600781aa0b64747f2a80f9865544107941f4921cea0")
}

// UnpackInvalidTimestampLengthError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidTimestampLength()
func (iLifiInputSettler *ILifiInputSettler) UnpackInvalidTimestampLengthError(raw []byte) (*ILifiInputSettlerInvalidTimestampLength, error) {
	out := new(ILifiInputSettlerInvalidTimestampLength)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "InvalidTimestampLength", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerNewOwnerIsZeroAddress represents a NewOwnerIsZeroAddress error raised by the ILifiInputSettler contract.
type ILifiInputSettlerNewOwnerIsZeroAddress struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NewOwnerIsZeroAddress()
func ILifiInputSettlerNewOwnerIsZeroAddressErrorID() common.Hash {
	return common.HexToHash("0x7448fbae245b5163a637f61fac94c5376c3e155928452ce47ee52d8c1b99587a")
}

// UnpackNewOwnerIsZeroAddressError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NewOwnerIsZeroAddress()
func (iLifiInputSettler *ILifiInputSettler) UnpackNewOwnerIsZeroAddressError(raw []byte) (*ILifiInputSettlerNewOwnerIsZeroAddress, error) {
	out := new(ILifiInputSettlerNewOwnerIsZeroAddress)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "NewOwnerIsZeroAddress", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerNoDestination represents a NoDestination error raised by the ILifiInputSettler contract.
type ILifiInputSettlerNoDestination struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NoDestination()
func ILifiInputSettlerNoDestinationErrorID() common.Hash {
	return common.HexToHash("0xb8e78e8013c2b18060a5e1d1d47e7c487b3f4c9e26fe84ba199887e6c88abda5")
}

// UnpackNoDestinationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NoDestination()
func (iLifiInputSettler *ILifiInputSettler) UnpackNoDestinationError(raw []byte) (*ILifiInputSettlerNoDestination, error) {
	out := new(ILifiInputSettlerNoDestination)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "NoDestination", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerNoHandoverRequest represents a NoHandoverRequest error raised by the ILifiInputSettler contract.
type ILifiInputSettlerNoHandoverRequest struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NoHandoverRequest()
func ILifiInputSettlerNoHandoverRequestErrorID() common.Hash {
	return common.HexToHash("0x6f5e8818469c73d5be4a0d17c371cde64695907022629c1d064c895f98d466a6")
}

// UnpackNoHandoverRequestError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NoHandoverRequest()
func (iLifiInputSettler *ILifiInputSettler) UnpackNoHandoverRequestError(raw []byte) (*ILifiInputSettlerNoHandoverRequest, error) {
	out := new(ILifiInputSettlerNoHandoverRequest)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "NoHandoverRequest", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerNotOrderOwner represents a NotOrderOwner error raised by the ILifiInputSettler contract.
type ILifiInputSettlerNotOrderOwner struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotOrderOwner()
func ILifiInputSettlerNotOrderOwnerErrorID() common.Hash {
	return common.HexToHash("0xf6412b5a9f98f861af79c1937e4ad40c98a45a023657259dd5775a8de7ecca15")
}

// UnpackNotOrderOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotOrderOwner()
func (iLifiInputSettler *ILifiInputSettler) UnpackNotOrderOwnerError(raw []byte) (*ILifiInputSettlerNotOrderOwner, error) {
	out := new(ILifiInputSettlerNotOrderOwner)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "NotOrderOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerOrderIdMismatch represents a OrderIdMismatch error raised by the ILifiInputSettler contract.
type ILifiInputSettlerOrderIdMismatch struct {
	Provided [32]byte
	Computed [32]byte
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OrderIdMismatch(bytes32 provided, bytes32 computed)
func ILifiInputSettlerOrderIdMismatchErrorID() common.Hash {
	return common.HexToHash("0x0517adf9c87f4f5cb24c4c43e313f684b98703db5b126fdd4f5ac47cc02267d5")
}

// UnpackOrderIdMismatchError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OrderIdMismatch(bytes32 provided, bytes32 computed)
func (iLifiInputSettler *ILifiInputSettler) UnpackOrderIdMismatchError(raw []byte) (*ILifiInputSettlerOrderIdMismatch, error) {
	out := new(ILifiInputSettlerOrderIdMismatch)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "OrderIdMismatch", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerReentrancyDetected represents a ReentrancyDetected error raised by the ILifiInputSettler contract.
type ILifiInputSettlerReentrancyDetected struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error ReentrancyDetected()
func ILifiInputSettlerReentrancyDetectedErrorID() common.Hash {
	return common.HexToHash("0xc5f2be51ec4ec0ad8a7972d497da993a6fcbb89cf72c05f97d654ed81ce53492")
}

// UnpackReentrancyDetectedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error ReentrancyDetected()
func (iLifiInputSettler *ILifiInputSettler) UnpackReentrancyDetectedError(raw []byte) (*ILifiInputSettlerReentrancyDetected, error) {
	out := new(ILifiInputSettlerReentrancyDetected)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "ReentrancyDetected", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the ILifiInputSettler contract.
type ILifiInputSettlerSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func ILifiInputSettlerSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (iLifiInputSettler *ILifiInputSettler) UnpackSafeERC20FailedOperationError(raw []byte) (*ILifiInputSettlerSafeERC20FailedOperation, error) {
	out := new(ILifiInputSettlerSafeERC20FailedOperation)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerSignatureAndInputsNotEqual represents a SignatureAndInputsNotEqual error raised by the ILifiInputSettler contract.
type ILifiInputSettlerSignatureAndInputsNotEqual struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SignatureAndInputsNotEqual()
func ILifiInputSettlerSignatureAndInputsNotEqualErrorID() common.Hash {
	return common.HexToHash("0x06f68b62ffd2436fe64050449d9d38b1823a747a993c088a72845ae5994b9883")
}

// UnpackSignatureAndInputsNotEqualError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SignatureAndInputsNotEqual()
func (iLifiInputSettler *ILifiInputSettler) UnpackSignatureAndInputsNotEqualError(raw []byte) (*ILifiInputSettlerSignatureAndInputsNotEqual, error) {
	out := new(ILifiInputSettlerSignatureAndInputsNotEqual)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "SignatureAndInputsNotEqual", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerSignatureNotSupported represents a SignatureNotSupported error raised by the ILifiInputSettler contract.
type ILifiInputSettlerSignatureNotSupported struct {
	Arg0 [1]byte
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SignatureNotSupported(bytes1 arg0)
func ILifiInputSettlerSignatureNotSupportedErrorID() common.Hash {
	return common.HexToHash("0x5d0b6f18a8b247272db8eeca2dbe086a9850e7bfd217b19bd636e0a15fbd7861")
}

// UnpackSignatureNotSupportedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SignatureNotSupported(bytes1 arg0)
func (iLifiInputSettler *ILifiInputSettler) UnpackSignatureNotSupportedError(raw []byte) (*ILifiInputSettlerSignatureNotSupported, error) {
	out := new(ILifiInputSettlerSignatureNotSupported)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "SignatureNotSupported", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerStringTooLong represents a StringTooLong error raised by the ILifiInputSettler contract.
type ILifiInputSettlerStringTooLong struct {
	Str string
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error StringTooLong(string str)
func ILifiInputSettlerStringTooLongErrorID() common.Hash {
	return common.HexToHash("0x305a27a93f8e33b7392df0a0f91d6fc63847395853c45991eec52dbf24d72381")
}

// UnpackStringTooLongError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error StringTooLong(string str)
func (iLifiInputSettler *ILifiInputSettler) UnpackStringTooLongError(raw []byte) (*ILifiInputSettlerStringTooLong, error) {
	out := new(ILifiInputSettlerStringTooLong)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "StringTooLong", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerTimestampNotPassed represents a TimestampNotPassed error raised by the ILifiInputSettler contract.
type ILifiInputSettlerTimestampNotPassed struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TimestampNotPassed()
func ILifiInputSettlerTimestampNotPassedErrorID() common.Hash {
	return common.HexToHash("0xeb21afbdfbff45b8884b33197c58f4fdd57aeee3ef678ac1e61248dc84fa5ac0")
}

// UnpackTimestampNotPassedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TimestampNotPassed()
func (iLifiInputSettler *ILifiInputSettler) UnpackTimestampNotPassedError(raw []byte) (*ILifiInputSettlerTimestampNotPassed, error) {
	out := new(ILifiInputSettlerTimestampNotPassed)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "TimestampNotPassed", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerTimestampPassed represents a TimestampPassed error raised by the ILifiInputSettler contract.
type ILifiInputSettlerTimestampPassed struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error TimestampPassed()
func ILifiInputSettlerTimestampPassedErrorID() common.Hash {
	return common.HexToHash("0x4a313c2dac3291054a75303df1d71c904dff49517ea33f42a82307b8ddca441a")
}

// UnpackTimestampPassedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error TimestampPassed()
func (iLifiInputSettler *ILifiInputSettler) UnpackTimestampPassedError(raw []byte) (*ILifiInputSettlerTimestampPassed, error) {
	out := new(ILifiInputSettlerTimestampPassed)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "TimestampPassed", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerUnauthorized represents a Unauthorized error raised by the ILifiInputSettler contract.
type ILifiInputSettlerUnauthorized struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error Unauthorized()
func ILifiInputSettlerUnauthorizedErrorID() common.Hash {
	return common.HexToHash("0x82b4290015f7ec7256ca2a6247d3c2a89c4865c0e791456df195f40ad0a81367")
}

// UnpackUnauthorizedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error Unauthorized()
func (iLifiInputSettler *ILifiInputSettler) UnpackUnauthorizedError(raw []byte) (*ILifiInputSettlerUnauthorized, error) {
	out := new(ILifiInputSettlerUnauthorized)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "Unauthorized", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ILifiInputSettlerWrongChain represents a WrongChain error raised by the ILifiInputSettler contract.
type ILifiInputSettlerWrongChain struct {
	Expected *big.Int
	Actual   *big.Int
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error WrongChain(uint256 expected, uint256 actual)
func ILifiInputSettlerWrongChainErrorID() common.Hash {
	return common.HexToHash("0x24497bc308635bccbc06f4997297d2158da178be2267eeead419bfdb19d42d4b")
}

// UnpackWrongChainError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error WrongChain(uint256 expected, uint256 actual)
func (iLifiInputSettler *ILifiInputSettler) UnpackWrongChainError(raw []byte) (*ILifiInputSettlerWrongChain, error) {
	out := new(ILifiInputSettlerWrongChain)
	if err := iLifiInputSettler.abi.UnpackIntoInterface(out, "WrongChain", raw); err != nil {
		return nil, err
	}
	return out, nil
}
