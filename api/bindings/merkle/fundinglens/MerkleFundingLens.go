// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package fundinglens

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

// IMerkleFundingLensSource is an auto generated low-level Go binding around an user-defined struct.
type IMerkleFundingLensSource struct {
	Id     [32]byte
	Target common.Address
	Data   []byte
	Offset *big.Int
}

// MerkleFundingLensMetaData contains all meta data concerning the MerkleFundingLens contract.
var MerkleFundingLensMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"getAvailableAssets\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"asset\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"sources\",\"type\":\"tuple[]\",\"internalType\":\"structIMerkleFundingLens.Source[]\",\"components\":[{\"name\":\"id\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"outputs\":[{\"name\":\"assets\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"DuplicateSource\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSource\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SourceReadFailed\",\"inputs\":[]}]",
	ID:  "MerkleFundingLens",
}

// MerkleFundingLens is an auto generated Go binding around an Ethereum contract.
type MerkleFundingLens struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *MerkleFundingLens) GetABI() abi.ABI {
	return c.abi
}

// NewMerkleFundingLens creates a new instance of MerkleFundingLens.
func NewMerkleFundingLens() *MerkleFundingLens {
	parsed, err := MerkleFundingLensMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &MerkleFundingLens{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *MerkleFundingLens) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackGetAvailableAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x65678642.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getAvailableAssets(address account, address asset, (bytes32,address,bytes,uint256)[] sources) view returns(uint256 assets)
func (merkleFundingLens *MerkleFundingLens) PackGetAvailableAssets(account common.Address, asset common.Address, sources []IMerkleFundingLensSource) []byte {
	enc, err := merkleFundingLens.abi.Pack("getAvailableAssets", account, asset, sources)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetAvailableAssets is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x65678642.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getAvailableAssets(address account, address asset, (bytes32,address,bytes,uint256)[] sources) view returns(uint256 assets)
func (merkleFundingLens *MerkleFundingLens) TryPackGetAvailableAssets(account common.Address, asset common.Address, sources []IMerkleFundingLensSource) ([]byte, error) {
	return merkleFundingLens.abi.Pack("getAvailableAssets", account, asset, sources)
}

// UnpackGetAvailableAssets is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x65678642.
//
// Solidity: function getAvailableAssets(address account, address asset, (bytes32,address,bytes,uint256)[] sources) view returns(uint256 assets)
func (merkleFundingLens *MerkleFundingLens) UnpackGetAvailableAssets(data []byte) (*big.Int, error) {
	out, err := merkleFundingLens.abi.Unpack("getAvailableAssets", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// UnpackError attempts to decode the provided error data using user-defined
// error definitions.
func (merkleFundingLens *MerkleFundingLens) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], merkleFundingLens.abi.Errors["DuplicateSource"].ID.Bytes()[:4]) {
		return merkleFundingLens.UnpackDuplicateSourceError(raw[4:])
	}
	if bytes.Equal(raw[:4], merkleFundingLens.abi.Errors["InvalidSource"].ID.Bytes()[:4]) {
		return merkleFundingLens.UnpackInvalidSourceError(raw[4:])
	}
	if bytes.Equal(raw[:4], merkleFundingLens.abi.Errors["SourceReadFailed"].ID.Bytes()[:4]) {
		return merkleFundingLens.UnpackSourceReadFailedError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// MerkleFundingLensDuplicateSource represents a DuplicateSource error raised by the MerkleFundingLens contract.
type MerkleFundingLensDuplicateSource struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error DuplicateSource()
func MerkleFundingLensDuplicateSourceErrorID() common.Hash {
	return common.HexToHash("0xb2d47efec429cabe50c9ef7e45ef45117a33199fa4cd49058ab0476fdfd3a848")
}

// UnpackDuplicateSourceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error DuplicateSource()
func (merkleFundingLens *MerkleFundingLens) UnpackDuplicateSourceError(raw []byte) (*MerkleFundingLensDuplicateSource, error) {
	out := new(MerkleFundingLensDuplicateSource)
	if err := merkleFundingLens.abi.UnpackIntoInterface(out, "DuplicateSource", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// MerkleFundingLensInvalidSource represents a InvalidSource error raised by the MerkleFundingLens contract.
type MerkleFundingLensInvalidSource struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidSource()
func MerkleFundingLensInvalidSourceErrorID() common.Hash {
	return common.HexToHash("0x8154374bf1623a6501cb022459b995901b93af0b7ab746379f687624b50457e9")
}

// UnpackInvalidSourceError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidSource()
func (merkleFundingLens *MerkleFundingLens) UnpackInvalidSourceError(raw []byte) (*MerkleFundingLensInvalidSource, error) {
	out := new(MerkleFundingLensInvalidSource)
	if err := merkleFundingLens.abi.UnpackIntoInterface(out, "InvalidSource", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// MerkleFundingLensSourceReadFailed represents a SourceReadFailed error raised by the MerkleFundingLens contract.
type MerkleFundingLensSourceReadFailed struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error SourceReadFailed()
func MerkleFundingLensSourceReadFailedErrorID() common.Hash {
	return common.HexToHash("0xe5fabbd65d6a80917e1de283a04d1479f4a74dedb22de07e6c7a528b9aeaf174")
}

// UnpackSourceReadFailedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error SourceReadFailed()
func (merkleFundingLens *MerkleFundingLens) UnpackSourceReadFailedError(raw []byte) (*MerkleFundingLensSourceReadFailed, error) {
	out := new(MerkleFundingLensSourceReadFailed)
	if err := merkleFundingLens.abi.UnpackIntoInterface(out, "SourceReadFailed", raw); err != nil {
		return nil, err
	}
	return out, nil
}
