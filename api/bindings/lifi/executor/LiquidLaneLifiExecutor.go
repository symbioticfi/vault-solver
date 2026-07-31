// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package executor

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

// IInputSettlerStandardOrder is an auto generated low-level Go binding around an user-defined struct.
type IInputSettlerStandardOrder struct {
	User          common.Address
	Nonce         *big.Int
	OriginChainId *big.Int
	Expires       uint32
	FillDeadline  uint32
	InputOracle   common.Address
	Inputs        [][2]*big.Int
	Outputs       []MandateOutput
}

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

// ILiquidLaneLifiExecutorDiscountRoute is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneLifiExecutorDiscountRoute struct {
	Adapter           common.Address
	AmountIn          *big.Int
	DiscountSwap      ILiquidLaneAdapterDiscountSwap
	ProtocolSignature []byte
}

// ILiquidLaneLifiExecutorFillRoute is an auto generated low-level Go binding around an user-defined struct.
type ILiquidLaneLifiExecutorFillRoute struct {
	Adapter   common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
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

// LiquidLaneLifiExecutorMetaData contains all meta data concerning the LiquidLaneLifiExecutor contract.
var LiquidLaneLifiExecutorMetaData = bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"inputSettler\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputSettler\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"INPUT_SETTLER\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"LIFI_REGISTRATION_TYPEHASH\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"OUTPUT_SETTLER\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"callers\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"finaliseWithCurrentTimestamp\",\"inputs\":[{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIInputSettler.StandardOrder\",\"components\":[{\"name\":\"user\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"originChainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expires\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"fillDeadline\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"inputOracle\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"inputs\",\"type\":\"uint256[2][]\",\"internalType\":\"uint256[2][]\"},{\"name\":\"outputs\",\"type\":\"tuple[]\",\"internalType\":\"structMandateOutput[]\",\"components\":[{\"name\":\"oracle\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"settler\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"token\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"recipient\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"callbackData\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"context\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]}]},{\"name\":\"routes\",\"type\":\"tuple[]\",\"internalType\":\"structILiquidLaneLifiExecutor.FillRoute[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amountOut\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"discountRoutes\",\"type\":\"tuple[]\",\"internalType\":\"structILiquidLaneLifiExecutor.DiscountRoute[]\",\"components\":[{\"name\":\"adapter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amountIn\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"discountSwap\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.DiscountSwap\",\"components\":[{\"name\":\"discount\",\"type\":\"tuple\",\"internalType\":\"structILiquidLaneAdapter.Discount\",\"components\":[{\"name\":\"tokenToRedeem\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"discount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"protocol\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"signerSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"protocolDeadline\",\"type\":\"uint48\",\"internalType\":\"uint48\"}]},{\"name\":\"protocolSignature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"owner_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isCaller\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isValidSignature\",\"inputs\":[{\"name\":\"hash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lifiRegistrationDigest\",\"inputs\":[{\"name\":\"messageHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"orderFinalised\",\"inputs\":[{\"name\":\"inputs\",\"type\":\"uint256[2][]\",\"internalType\":\"uint256[2][]\"},{\"name\":\"executionData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"internalType\":\"address[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SetCallers\",\"inputs\":[{\"name\":\"newCallers\",\"type\":\"address[]\",\"indexed\":false,\"internalType\":\"address[]\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotCaller\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInputSettler\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
	ID:  "LiquidLaneLifiExecutor",
}

// LiquidLaneLifiExecutor is an auto generated Go binding around an Ethereum contract.
type LiquidLaneLifiExecutor struct {
	abi abi.ABI
}

// NewLiquidLaneLifiExecutor creates a new instance of LiquidLaneLifiExecutor.
func NewLiquidLaneLifiExecutor() *LiquidLaneLifiExecutor {
	parsed, err := LiquidLaneLifiExecutorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &LiquidLaneLifiExecutor{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *LiquidLaneLifiExecutor) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address inputSettler, address outputSettler) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackConstructor(inputSettler common.Address, outputSettler common.Address) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("", inputSettler, outputSettler)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackINPUTSETTLER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb627707d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function INPUT_SETTLER() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackINPUTSETTLER() []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("INPUT_SETTLER")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackINPUTSETTLER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb627707d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function INPUT_SETTLER() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackINPUTSETTLER() ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("INPUT_SETTLER")
}

// UnpackINPUTSETTLER is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xb627707d.
//
// Solidity: function INPUT_SETTLER() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackINPUTSETTLER(data []byte) (common.Address, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("INPUT_SETTLER", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackLIFIREGISTRATIONTYPEHASH is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd0c83dad.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function LIFI_REGISTRATION_TYPEHASH() view returns(bytes32)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackLIFIREGISTRATIONTYPEHASH() []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("LIFI_REGISTRATION_TYPEHASH")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLIFIREGISTRATIONTYPEHASH is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd0c83dad.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function LIFI_REGISTRATION_TYPEHASH() view returns(bytes32)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackLIFIREGISTRATIONTYPEHASH() ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("LIFI_REGISTRATION_TYPEHASH")
}

// UnpackLIFIREGISTRATIONTYPEHASH is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xd0c83dad.
//
// Solidity: function LIFI_REGISTRATION_TYPEHASH() view returns(bytes32)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackLIFIREGISTRATIONTYPEHASH(data []byte) ([32]byte, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("LIFI_REGISTRATION_TYPEHASH", data)
	if err != nil {
		return *new([32]byte), err
	}
	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)
	return out0, nil
}

// PackOUTPUTSETTLER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6d9d466.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function OUTPUT_SETTLER() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackOUTPUTSETTLER() []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("OUTPUT_SETTLER")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOUTPUTSETTLER is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc6d9d466.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function OUTPUT_SETTLER() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackOUTPUTSETTLER() ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("OUTPUT_SETTLER")
}

// UnpackOUTPUTSETTLER is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc6d9d466.
//
// Solidity: function OUTPUT_SETTLER() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackOUTPUTSETTLER(data []byte) (common.Address, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("OUTPUT_SETTLER", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaa03fa3d.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function callers(uint256 ) view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackCallers(arg0 *big.Int) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("callers", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xaa03fa3d.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function callers(uint256 ) view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackCallers(arg0 *big.Int) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("callers", arg0)
}

// UnpackCallers is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xaa03fa3d.
//
// Solidity: function callers(uint256 ) view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackCallers(data []byte) (common.Address, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("callers", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackEip712Domain is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x84b0196e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackEip712Domain() []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("eip712Domain")
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackEip712Domain() ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("eip712Domain")
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackEip712Domain(data []byte) (Eip712DomainOutput, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("eip712Domain", data)
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

// PackFinaliseWithCurrentTimestamp is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd24f1d03.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function finaliseWithCurrentTimestamp((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, (address,uint256,uint256)[] routes, (address,uint256,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes)[] discountRoutes) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackFinaliseWithCurrentTimestamp(order IInputSettlerStandardOrder, routes []ILiquidLaneLifiExecutorFillRoute, discountRoutes []ILiquidLaneLifiExecutorDiscountRoute) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("finaliseWithCurrentTimestamp", order, routes, discountRoutes)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackFinaliseWithCurrentTimestamp is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xd24f1d03.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function finaliseWithCurrentTimestamp((address,uint256,uint256,uint32,uint32,address,uint256[2][],(bytes32,bytes32,uint256,bytes32,uint256,bytes32,bytes,bytes)[]) order, (address,uint256,uint256)[] routes, (address,uint256,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes)[] discountRoutes) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackFinaliseWithCurrentTimestamp(order IInputSettlerStandardOrder, routes []ILiquidLaneLifiExecutorFillRoute, discountRoutes []ILiquidLaneLifiExecutorDiscountRoute) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("finaliseWithCurrentTimestamp", order, routes, discountRoutes)
}

// PackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x946d9204.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function initialize(address owner_, address[] initCallers) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackInitialize(owner common.Address, initCallers []common.Address) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("initialize", owner, initCallers)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackInitialize is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x946d9204.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function initialize(address owner_, address[] initCallers) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackInitialize(owner common.Address, initCallers []common.Address) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("initialize", owner, initCallers)
}

// PackIsCaller is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7ac07dcc.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isCaller(address caller) view returns(bool)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackIsCaller(caller common.Address) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("isCaller", caller)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsCaller is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7ac07dcc.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isCaller(address caller) view returns(bool)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackIsCaller(caller common.Address) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("isCaller", caller)
}

// UnpackIsCaller is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x7ac07dcc.
//
// Solidity: function isCaller(address caller) view returns(bool)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackIsCaller(data []byte) (bool, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("isCaller", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackIsValidSignature is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1626ba7e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackIsValidSignature(hash [32]byte, signature []byte) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("isValidSignature", hash, signature)
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackIsValidSignature(hash [32]byte, signature []byte) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("isValidSignature", hash, signature)
}

// UnpackIsValidSignature is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 hash, bytes signature) view returns(bytes4)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackIsValidSignature(data []byte) ([4]byte, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("isValidSignature", data)
	if err != nil {
		return *new([4]byte), err
	}
	out0 := *abi.ConvertType(out[0], new([4]byte)).(*[4]byte)
	return out0, nil
}

// PackLifiRegistrationDigest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1ce5298e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function lifiRegistrationDigest(bytes32 messageHash) view returns(bytes32)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackLifiRegistrationDigest(messageHash [32]byte) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("lifiRegistrationDigest", messageHash)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLifiRegistrationDigest is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x1ce5298e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function lifiRegistrationDigest(bytes32 messageHash) view returns(bytes32)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackLifiRegistrationDigest(messageHash [32]byte) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("lifiRegistrationDigest", messageHash)
}

// UnpackLifiRegistrationDigest is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x1ce5298e.
//
// Solidity: function lifiRegistrationDigest(bytes32 messageHash) view returns(bytes32)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackLifiRegistrationDigest(data []byte) ([32]byte, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("lifiRegistrationDigest", data)
	if err != nil {
		return *new([32]byte), err
	}
	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)
	return out0, nil
}

// PackOrderFinalised is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x73e57c27.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function orderFinalised(uint256[2][] inputs, bytes executionData) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackOrderFinalised(inputs [][2]*big.Int, executionData []byte) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("orderFinalised", inputs, executionData)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackOrderFinalised is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x73e57c27.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function orderFinalised(uint256[2][] inputs, bytes executionData) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackOrderFinalised(inputs [][2]*big.Int, executionData []byte) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("orderFinalised", inputs, executionData)
}

// PackOwner is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8da5cb5b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function owner() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackOwner() []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("owner")
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackOwner() ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("owner")
}

// UnpackOwner is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackOwner(data []byte) (common.Address, error) {
	out, err := liquidLaneLifiExecutor.abi.Unpack("owner", data)
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackRenounceOwnership() []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("renounceOwnership")
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackRenounceOwnership() ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("renounceOwnership")
}

// PackSetCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x43ded848.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackSetCallers(newCallers []common.Address) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("setCallers", newCallers)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetCallers is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x43ded848.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setCallers(address[] newCallers) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackSetCallers(newCallers []common.Address) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("setCallers", newCallers)
}

// PackTransferOwnership is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf2fde38b.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) PackTransferOwnership(newOwner common.Address) []byte {
	enc, err := liquidLaneLifiExecutor.abi.Pack("transferOwnership", newOwner)
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) TryPackTransferOwnership(newOwner common.Address) ([]byte, error) {
	return liquidLaneLifiExecutor.abi.Pack("transferOwnership", newOwner)
}

// LiquidLaneLifiExecutorEIP712DomainChanged represents a EIP712DomainChanged event raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorEIP712DomainChanged struct {
	Raw *types.Log // Blockchain specific contextual infos
}

const LiquidLaneLifiExecutorEIP712DomainChangedEventName = "EIP712DomainChanged"

// ContractEventName returns the user-defined event name.
func (LiquidLaneLifiExecutorEIP712DomainChanged) ContractEventName() string {
	return LiquidLaneLifiExecutorEIP712DomainChangedEventName
}

// UnpackEIP712DomainChangedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event EIP712DomainChanged()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackEIP712DomainChangedEvent(log *types.Log) (*LiquidLaneLifiExecutorEIP712DomainChanged, error) {
	event := "EIP712DomainChanged"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneLifiExecutor.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneLifiExecutorEIP712DomainChanged)
	if len(log.Data) > 0 {
		if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneLifiExecutor.abi.Events[event].Inputs {
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

// LiquidLaneLifiExecutorInitialized represents a Initialized event raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorInitialized struct {
	Version uint64
	Raw     *types.Log // Blockchain specific contextual infos
}

const LiquidLaneLifiExecutorInitializedEventName = "Initialized"

// ContractEventName returns the user-defined event name.
func (LiquidLaneLifiExecutorInitialized) ContractEventName() string {
	return LiquidLaneLifiExecutorInitializedEventName
}

// UnpackInitializedEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Initialized(uint64 version)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackInitializedEvent(log *types.Log) (*LiquidLaneLifiExecutorInitialized, error) {
	event := "Initialized"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneLifiExecutor.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneLifiExecutorInitialized)
	if len(log.Data) > 0 {
		if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneLifiExecutor.abi.Events[event].Inputs {
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

// LiquidLaneLifiExecutorOwnershipTransferred represents a OwnershipTransferred event raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           *types.Log // Blockchain specific contextual infos
}

const LiquidLaneLifiExecutorOwnershipTransferredEventName = "OwnershipTransferred"

// ContractEventName returns the user-defined event name.
func (LiquidLaneLifiExecutorOwnershipTransferred) ContractEventName() string {
	return LiquidLaneLifiExecutorOwnershipTransferredEventName
}

// UnpackOwnershipTransferredEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackOwnershipTransferredEvent(log *types.Log) (*LiquidLaneLifiExecutorOwnershipTransferred, error) {
	event := "OwnershipTransferred"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneLifiExecutor.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneLifiExecutorOwnershipTransferred)
	if len(log.Data) > 0 {
		if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneLifiExecutor.abi.Events[event].Inputs {
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

// LiquidLaneLifiExecutorSetCallers represents a SetCallers event raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorSetCallers struct {
	NewCallers []common.Address
	Raw        *types.Log // Blockchain specific contextual infos
}

const LiquidLaneLifiExecutorSetCallersEventName = "SetCallers"

// ContractEventName returns the user-defined event name.
func (LiquidLaneLifiExecutorSetCallers) ContractEventName() string {
	return LiquidLaneLifiExecutorSetCallersEventName
}

// UnpackSetCallersEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event SetCallers(address[] newCallers)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackSetCallersEvent(log *types.Log) (*LiquidLaneLifiExecutorSetCallers, error) {
	event := "SetCallers"
	if len(log.Topics) == 0 {
		return nil, bind.ErrNoEventSignature
	}
	if log.Topics[0] != liquidLaneLifiExecutor.abi.Events[event].ID {
		return nil, bind.ErrEventSignatureMismatch
	}
	out := new(LiquidLaneLifiExecutorSetCallers)
	if len(log.Data) > 0 {
		if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range liquidLaneLifiExecutor.abi.Events[event].Inputs {
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
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], liquidLaneLifiExecutor.abi.Errors["InvalidInitialization"].ID.Bytes()[:4]) {
		return liquidLaneLifiExecutor.UnpackInvalidInitializationError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneLifiExecutor.abi.Errors["NotCaller"].ID.Bytes()[:4]) {
		return liquidLaneLifiExecutor.UnpackNotCallerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneLifiExecutor.abi.Errors["NotInitializing"].ID.Bytes()[:4]) {
		return liquidLaneLifiExecutor.UnpackNotInitializingError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneLifiExecutor.abi.Errors["NotInputSettler"].ID.Bytes()[:4]) {
		return liquidLaneLifiExecutor.UnpackNotInputSettlerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneLifiExecutor.abi.Errors["OwnableInvalidOwner"].ID.Bytes()[:4]) {
		return liquidLaneLifiExecutor.UnpackOwnableInvalidOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneLifiExecutor.abi.Errors["OwnableUnauthorizedAccount"].ID.Bytes()[:4]) {
		return liquidLaneLifiExecutor.UnpackOwnableUnauthorizedAccountError(raw[4:])
	}
	if bytes.Equal(raw[:4], liquidLaneLifiExecutor.abi.Errors["SafeERC20FailedOperation"].ID.Bytes()[:4]) {
		return liquidLaneLifiExecutor.UnpackSafeERC20FailedOperationError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// LiquidLaneLifiExecutorInvalidInitialization represents a InvalidInitialization error raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorInvalidInitialization struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidInitialization()
func LiquidLaneLifiExecutorInvalidInitializationErrorID() common.Hash {
	return common.HexToHash("0xf92ee8a957075833165f68c320933b1a1294aafc84ee6e0dd3fb178008f9aaf5")
}

// UnpackInvalidInitializationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidInitialization()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackInvalidInitializationError(raw []byte) (*LiquidLaneLifiExecutorInvalidInitialization, error) {
	out := new(LiquidLaneLifiExecutorInvalidInitialization)
	if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, "InvalidInitialization", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneLifiExecutorNotCaller represents a NotCaller error raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorNotCaller struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotCaller()
func LiquidLaneLifiExecutorNotCallerErrorID() common.Hash {
	return common.HexToHash("0x16c618d80989492b64dbf0ed90935e3959f670b9b9d57385b45d00c0d1cdedf9")
}

// UnpackNotCallerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotCaller()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackNotCallerError(raw []byte) (*LiquidLaneLifiExecutorNotCaller, error) {
	out := new(LiquidLaneLifiExecutorNotCaller)
	if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, "NotCaller", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneLifiExecutorNotInitializing represents a NotInitializing error raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorNotInitializing struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInitializing()
func LiquidLaneLifiExecutorNotInitializingErrorID() common.Hash {
	return common.HexToHash("0xd7e6bcf8597daa127dc9f0048d2f08d5ef140a2cb659feabd700beff1f7a8302")
}

// UnpackNotInitializingError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInitializing()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackNotInitializingError(raw []byte) (*LiquidLaneLifiExecutorNotInitializing, error) {
	out := new(LiquidLaneLifiExecutorNotInitializing)
	if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, "NotInitializing", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneLifiExecutorNotInputSettler represents a NotInputSettler error raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorNotInputSettler struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotInputSettler()
func LiquidLaneLifiExecutorNotInputSettlerErrorID() common.Hash {
	return common.HexToHash("0xde89f63ea338ef13c2e1dd13cfee098f9c2ac145dbd7f1e315fcaffdc099d30a")
}

// UnpackNotInputSettlerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotInputSettler()
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackNotInputSettlerError(raw []byte) (*LiquidLaneLifiExecutorNotInputSettler, error) {
	out := new(LiquidLaneLifiExecutorNotInputSettler)
	if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, "NotInputSettler", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneLifiExecutorOwnableInvalidOwner represents a OwnableInvalidOwner error raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorOwnableInvalidOwner struct {
	Owner common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableInvalidOwner(address owner)
func LiquidLaneLifiExecutorOwnableInvalidOwnerErrorID() common.Hash {
	return common.HexToHash("0x1e4fbdf7f3ef8bcaa855599e3abf48b232380f183f08f6f813d9ffa5bd585188")
}

// UnpackOwnableInvalidOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableInvalidOwner(address owner)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackOwnableInvalidOwnerError(raw []byte) (*LiquidLaneLifiExecutorOwnableInvalidOwner, error) {
	out := new(LiquidLaneLifiExecutorOwnableInvalidOwner)
	if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, "OwnableInvalidOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneLifiExecutorOwnableUnauthorizedAccount represents a OwnableUnauthorizedAccount error raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorOwnableUnauthorizedAccount struct {
	Account common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func LiquidLaneLifiExecutorOwnableUnauthorizedAccountErrorID() common.Hash {
	return common.HexToHash("0x118cdaa7a341953d1887a2245fd6665d741c67c8c50581daa59e1d03373fa188")
}

// UnpackOwnableUnauthorizedAccountError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OwnableUnauthorizedAccount(address account)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackOwnableUnauthorizedAccountError(raw []byte) (*LiquidLaneLifiExecutorOwnableUnauthorizedAccount, error) {
	out := new(LiquidLaneLifiExecutorOwnableUnauthorizedAccount)
	if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, "OwnableUnauthorizedAccount", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// LiquidLaneLifiExecutorSafeERC20FailedOperation represents a SafeERC20FailedOperation error raised by the LiquidLaneLifiExecutor contract.
type LiquidLaneLifiExecutorSafeERC20FailedOperation struct {
	Token common.Address
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SafeERC20FailedOperation(address token)
func LiquidLaneLifiExecutorSafeERC20FailedOperationErrorID() common.Hash {
	return common.HexToHash("0x5274afe73c98b4749fc91ffae6b7b574e7842cb2144a159e9377a5f20b32edf9")
}

// UnpackSafeERC20FailedOperationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SafeERC20FailedOperation(address token)
func (liquidLaneLifiExecutor *LiquidLaneLifiExecutor) UnpackSafeERC20FailedOperationError(raw []byte) (*LiquidLaneLifiExecutorSafeERC20FailedOperation, error) {
	out := new(LiquidLaneLifiExecutorSafeERC20FailedOperation)
	if err := liquidLaneLifiExecutor.abi.UnpackIntoInterface(out, "SafeERC20FailedOperation", raw); err != nil {
		return nil, err
	}
	return out, nil
}
