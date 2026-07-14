// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package adapterfactory

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

// IAdapterFactoryMetaData contains all meta data concerning the IAdapterFactory contract.
var IAdapterFactoryMetaData = bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"blacklist\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"blacklisted\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"create\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"entity\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"implementation\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isEntity\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"lastVersion\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"entity\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newVersion\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"totalEntities\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"whitelist\",\"inputs\":[{\"name\":\"implementation\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AddEntity\",\"inputs\":[{\"name\":\"entity\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Blacklist\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Migrate\",\"inputs\":[{\"name\":\"entity\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newVersion\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Whitelist\",\"inputs\":[{\"name\":\"implementation\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyBlacklisted\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AlreadyWhitelisted\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"EntityNotExist\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidImplementation\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidVersion\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotOwner\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OldVersion\",\"inputs\":[]}]",
	ID:  "IAdapterFactory",
}

// IAdapterFactory is an auto generated Go binding around an Ethereum contract.
type IAdapterFactory struct {
	abi abi.ABI
}

// NewIAdapterFactory creates a new instance of IAdapterFactory.
func NewIAdapterFactory() *IAdapterFactory {
	parsed, err := IAdapterFactoryMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &IAdapterFactory{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *IAdapterFactory) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackBlacklist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb572a966.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function blacklist(uint64 version) returns()
func (iAdapterFactory *IAdapterFactory) PackBlacklist(version uint64) []byte {
	enc, err := iAdapterFactory.abi.Pack("blacklist", version)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBlacklist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb572a966.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function blacklist(uint64 version) returns()
func (iAdapterFactory *IAdapterFactory) TryPackBlacklist(version uint64) ([]byte, error) {
	return iAdapterFactory.abi.Pack("blacklist", version)
}

// PackBlacklisted is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb6caa119.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function blacklisted(uint64 version) view returns(bool)
func (iAdapterFactory *IAdapterFactory) PackBlacklisted(version uint64) []byte {
	enc, err := iAdapterFactory.abi.Pack("blacklisted", version)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackBlacklisted is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb6caa119.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function blacklisted(uint64 version) view returns(bool)
func (iAdapterFactory *IAdapterFactory) TryPackBlacklisted(version uint64) ([]byte, error) {
	return iAdapterFactory.abi.Pack("blacklisted", version)
}

// UnpackBlacklisted is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xb6caa119.
//
// Solidity: function blacklisted(uint64 version) view returns(bool)
func (iAdapterFactory *IAdapterFactory) UnpackBlacklisted(data []byte) (bool, error) {
	out, err := iAdapterFactory.abi.Unpack("blacklisted", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackCreate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3ac04911.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function create(uint64 version, address owner, bytes data) returns(address)
func (iAdapterFactory *IAdapterFactory) PackCreate(version uint64, owner common.Address, data []byte) []byte {
	enc, err := iAdapterFactory.abi.Pack("create", version, owner, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackCreate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x3ac04911.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function create(uint64 version, address owner, bytes data) returns(address)
func (iAdapterFactory *IAdapterFactory) TryPackCreate(version uint64, owner common.Address, data []byte) ([]byte, error) {
	return iAdapterFactory.abi.Pack("create", version, owner, data)
}

// UnpackCreate is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x3ac04911.
//
// Solidity: function create(uint64 version, address owner, bytes data) returns(address)
func (iAdapterFactory *IAdapterFactory) UnpackCreate(data []byte) (common.Address, error) {
	out, err := iAdapterFactory.abi.Unpack("create", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackEntity is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb42ba2a2.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function entity(uint256 index) view returns(address)
func (iAdapterFactory *IAdapterFactory) PackEntity(index *big.Int) []byte {
	enc, err := iAdapterFactory.abi.Pack("entity", index)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackEntity is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xb42ba2a2.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function entity(uint256 index) view returns(address)
func (iAdapterFactory *IAdapterFactory) TryPackEntity(index *big.Int) ([]byte, error) {
	return iAdapterFactory.abi.Pack("entity", index)
}

// UnpackEntity is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xb42ba2a2.
//
// Solidity: function entity(uint256 index) view returns(address)
func (iAdapterFactory *IAdapterFactory) UnpackEntity(data []byte) (common.Address, error) {
	out, err := iAdapterFactory.abi.Unpack("entity", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackImplementation is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf9661602.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function implementation(uint64 version) view returns(address)
func (iAdapterFactory *IAdapterFactory) PackImplementation(version uint64) []byte {
	enc, err := iAdapterFactory.abi.Pack("implementation", version)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackImplementation is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf9661602.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function implementation(uint64 version) view returns(address)
func (iAdapterFactory *IAdapterFactory) TryPackImplementation(version uint64) ([]byte, error) {
	return iAdapterFactory.abi.Pack("implementation", version)
}

// UnpackImplementation is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xf9661602.
//
// Solidity: function implementation(uint64 version) view returns(address)
func (iAdapterFactory *IAdapterFactory) UnpackImplementation(data []byte) (common.Address, error) {
	out, err := iAdapterFactory.abi.Unpack("implementation", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackIsEntity is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x14887c58.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function isEntity(address account) view returns(bool)
func (iAdapterFactory *IAdapterFactory) PackIsEntity(account common.Address) []byte {
	enc, err := iAdapterFactory.abi.Pack("isEntity", account)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackIsEntity is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x14887c58.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function isEntity(address account) view returns(bool)
func (iAdapterFactory *IAdapterFactory) TryPackIsEntity(account common.Address) ([]byte, error) {
	return iAdapterFactory.abi.Pack("isEntity", account)
}

// UnpackIsEntity is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x14887c58.
//
// Solidity: function isEntity(address account) view returns(bool)
func (iAdapterFactory *IAdapterFactory) UnpackIsEntity(data []byte) (bool, error) {
	out, err := iAdapterFactory.abi.Unpack("isEntity", data)
	if err != nil {
		return *new(bool), err
	}
	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	return out0, nil
}

// PackLastVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x64dfea06.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function lastVersion() view returns(uint64)
func (iAdapterFactory *IAdapterFactory) PackLastVersion() []byte {
	enc, err := iAdapterFactory.abi.Pack("lastVersion")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLastVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x64dfea06.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function lastVersion() view returns(uint64)
func (iAdapterFactory *IAdapterFactory) TryPackLastVersion() ([]byte, error) {
	return iAdapterFactory.abi.Pack("lastVersion")
}

// UnpackLastVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x64dfea06.
//
// Solidity: function lastVersion() view returns(uint64)
func (iAdapterFactory *IAdapterFactory) UnpackLastVersion(data []byte) (uint64, error) {
	out, err := iAdapterFactory.abi.Unpack("lastVersion", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// PackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x58336662.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function migrate(address entity, uint64 newVersion, bytes data) returns()
func (iAdapterFactory *IAdapterFactory) PackMigrate(entity common.Address, newVersion uint64, data []byte) []byte {
	enc, err := iAdapterFactory.abi.Pack("migrate", entity, newVersion, data)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMigrate is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x58336662.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function migrate(address entity, uint64 newVersion, bytes data) returns()
func (iAdapterFactory *IAdapterFactory) TryPackMigrate(entity common.Address, newVersion uint64, data []byte) ([]byte, error) {
	return iAdapterFactory.abi.Pack("migrate", entity, newVersion, data)
}

// PackTotalEntities is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5cd8b15e.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function totalEntities() view returns(uint256)
func (iAdapterFactory *IAdapterFactory) PackTotalEntities() []byte {
	enc, err := iAdapterFactory.abi.Pack("totalEntities")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTotalEntities is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x5cd8b15e.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function totalEntities() view returns(uint256)
func (iAdapterFactory *IAdapterFactory) TryPackTotalEntities() ([]byte, error) {
	return iAdapterFactory.abi.Pack("totalEntities")
}

// UnpackTotalEntities is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x5cd8b15e.
//
// Solidity: function totalEntities() view returns(uint256)
func (iAdapterFactory *IAdapterFactory) UnpackTotalEntities(data []byte) (*big.Int, error) {
	out, err := iAdapterFactory.abi.Unpack("totalEntities", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackWhitelist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9b19251a.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function whitelist(address implementation) returns()
func (iAdapterFactory *IAdapterFactory) PackWhitelist(implementation common.Address) []byte {
	enc, err := iAdapterFactory.abi.Pack("whitelist", implementation)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackWhitelist is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9b19251a.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function whitelist(address implementation) returns()
func (iAdapterFactory *IAdapterFactory) TryPackWhitelist(implementation common.Address) ([]byte, error) {
	return iAdapterFactory.abi.Pack("whitelist", implementation)
}

// IAdapterFactoryAddEntity represents a AddEntity event raised by the IAdapterFactory contract.
type IAdapterFactoryAddEntity struct {
	Entity common.Address
	Raw    *types.Log // Blockchain specific contextual infos
}

const IAdapterFactoryAddEntityEventName = "AddEntity"

// ContractEventName returns the user-defined event name.
func (IAdapterFactoryAddEntity) ContractEventName() string {
	return IAdapterFactoryAddEntityEventName
}

// UnpackAddEntityEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event AddEntity(address indexed entity)
func (iAdapterFactory *IAdapterFactory) UnpackAddEntityEvent(log *types.Log) (*IAdapterFactoryAddEntity, error) {
	event := "AddEntity"
	if log.Topics[0] != iAdapterFactory.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IAdapterFactoryAddEntity)
	if len(log.Data) > 0 {
		if err := iAdapterFactory.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iAdapterFactory.abi.Events[event].Inputs {
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

// IAdapterFactoryBlacklist represents a Blacklist event raised by the IAdapterFactory contract.
type IAdapterFactoryBlacklist struct {
	Version uint64
	Raw     *types.Log // Blockchain specific contextual infos
}

const IAdapterFactoryBlacklistEventName = "Blacklist"

// ContractEventName returns the user-defined event name.
func (IAdapterFactoryBlacklist) ContractEventName() string {
	return IAdapterFactoryBlacklistEventName
}

// UnpackBlacklistEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Blacklist(uint64 indexed version)
func (iAdapterFactory *IAdapterFactory) UnpackBlacklistEvent(log *types.Log) (*IAdapterFactoryBlacklist, error) {
	event := "Blacklist"
	if log.Topics[0] != iAdapterFactory.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IAdapterFactoryBlacklist)
	if len(log.Data) > 0 {
		if err := iAdapterFactory.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iAdapterFactory.abi.Events[event].Inputs {
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

// IAdapterFactoryMigrate represents a Migrate event raised by the IAdapterFactory contract.
type IAdapterFactoryMigrate struct {
	Entity     common.Address
	NewVersion uint64
	Raw        *types.Log // Blockchain specific contextual infos
}

const IAdapterFactoryMigrateEventName = "Migrate"

// ContractEventName returns the user-defined event name.
func (IAdapterFactoryMigrate) ContractEventName() string {
	return IAdapterFactoryMigrateEventName
}

// UnpackMigrateEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Migrate(address indexed entity, uint64 newVersion)
func (iAdapterFactory *IAdapterFactory) UnpackMigrateEvent(log *types.Log) (*IAdapterFactoryMigrate, error) {
	event := "Migrate"
	if log.Topics[0] != iAdapterFactory.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IAdapterFactoryMigrate)
	if len(log.Data) > 0 {
		if err := iAdapterFactory.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iAdapterFactory.abi.Events[event].Inputs {
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

// IAdapterFactoryWhitelist represents a Whitelist event raised by the IAdapterFactory contract.
type IAdapterFactoryWhitelist struct {
	Implementation common.Address
	Raw            *types.Log // Blockchain specific contextual infos
}

const IAdapterFactoryWhitelistEventName = "Whitelist"

// ContractEventName returns the user-defined event name.
func (IAdapterFactoryWhitelist) ContractEventName() string {
	return IAdapterFactoryWhitelistEventName
}

// UnpackWhitelistEvent is the Go binding that unpacks the event data emitted
// by contract.
//
// Solidity: event Whitelist(address indexed implementation)
func (iAdapterFactory *IAdapterFactory) UnpackWhitelistEvent(log *types.Log) (*IAdapterFactoryWhitelist, error) {
	event := "Whitelist"
	if log.Topics[0] != iAdapterFactory.abi.Events[event].ID {
		return nil, errors.New("event signature mismatch")
	}
	out := new(IAdapterFactoryWhitelist)
	if len(log.Data) > 0 {
		if err := iAdapterFactory.abi.UnpackIntoInterface(out, event, log.Data); err != nil {
			return nil, err
		}
	}
	var indexed abi.Arguments
	for _, arg := range iAdapterFactory.abi.Events[event].Inputs {
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
func (iAdapterFactory *IAdapterFactory) UnpackError(raw []byte) (any, error) {
	if bytes.Equal(raw[:4], iAdapterFactory.abi.Errors["AlreadyBlacklisted"].ID.Bytes()[:4]) {
		return iAdapterFactory.UnpackAlreadyBlacklistedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iAdapterFactory.abi.Errors["AlreadyWhitelisted"].ID.Bytes()[:4]) {
		return iAdapterFactory.UnpackAlreadyWhitelistedError(raw[4:])
	}
	if bytes.Equal(raw[:4], iAdapterFactory.abi.Errors["EntityNotExist"].ID.Bytes()[:4]) {
		return iAdapterFactory.UnpackEntityNotExistError(raw[4:])
	}
	if bytes.Equal(raw[:4], iAdapterFactory.abi.Errors["InvalidImplementation"].ID.Bytes()[:4]) {
		return iAdapterFactory.UnpackInvalidImplementationError(raw[4:])
	}
	if bytes.Equal(raw[:4], iAdapterFactory.abi.Errors["InvalidVersion"].ID.Bytes()[:4]) {
		return iAdapterFactory.UnpackInvalidVersionError(raw[4:])
	}
	if bytes.Equal(raw[:4], iAdapterFactory.abi.Errors["NotOwner"].ID.Bytes()[:4]) {
		return iAdapterFactory.UnpackNotOwnerError(raw[4:])
	}
	if bytes.Equal(raw[:4], iAdapterFactory.abi.Errors["OldVersion"].ID.Bytes()[:4]) {
		return iAdapterFactory.UnpackOldVersionError(raw[4:])
	}
	return nil, errors.New("Unknown error")
}

// IAdapterFactoryAlreadyBlacklisted represents a AlreadyBlacklisted error raised by the IAdapterFactory contract.
type IAdapterFactoryAlreadyBlacklisted struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyBlacklisted()
func IAdapterFactoryAlreadyBlacklistedErrorID() common.Hash {
	return common.HexToHash("0xf53de75f1e31621ad6a944a755bdeb0c9e6ce21f9741928443fc729611349ad0")
}

// UnpackAlreadyBlacklistedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyBlacklisted()
func (iAdapterFactory *IAdapterFactory) UnpackAlreadyBlacklistedError(raw []byte) (*IAdapterFactoryAlreadyBlacklisted, error) {
	out := new(IAdapterFactoryAlreadyBlacklisted)
	if err := iAdapterFactory.abi.UnpackIntoInterface(out, "AlreadyBlacklisted", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IAdapterFactoryAlreadyWhitelisted represents a AlreadyWhitelisted error raised by the IAdapterFactory contract.
type IAdapterFactoryAlreadyWhitelisted struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error AlreadyWhitelisted()
func IAdapterFactoryAlreadyWhitelistedErrorID() common.Hash {
	return common.HexToHash("0xb73e95e172f49f27697561bc619185e51aa120c14e4c6bef832c9e9277e4cdcc")
}

// UnpackAlreadyWhitelistedError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error AlreadyWhitelisted()
func (iAdapterFactory *IAdapterFactory) UnpackAlreadyWhitelistedError(raw []byte) (*IAdapterFactoryAlreadyWhitelisted, error) {
	out := new(IAdapterFactoryAlreadyWhitelisted)
	if err := iAdapterFactory.abi.UnpackIntoInterface(out, "AlreadyWhitelisted", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IAdapterFactoryEntityNotExist represents a EntityNotExist error raised by the IAdapterFactory contract.
type IAdapterFactoryEntityNotExist struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error EntityNotExist()
func IAdapterFactoryEntityNotExistErrorID() common.Hash {
	return common.HexToHash("0xe3fd10ffa8201bd89f8a91d79dc11e821a5e146bffbcccfb4306a569bb46eae2")
}

// UnpackEntityNotExistError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error EntityNotExist()
func (iAdapterFactory *IAdapterFactory) UnpackEntityNotExistError(raw []byte) (*IAdapterFactoryEntityNotExist, error) {
	out := new(IAdapterFactoryEntityNotExist)
	if err := iAdapterFactory.abi.UnpackIntoInterface(out, "EntityNotExist", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IAdapterFactoryInvalidImplementation represents a InvalidImplementation error raised by the IAdapterFactory contract.
type IAdapterFactoryInvalidImplementation struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidImplementation()
func IAdapterFactoryInvalidImplementationErrorID() common.Hash {
	return common.HexToHash("0x68155f9a907e5d62f79efc98cfae07e66c5c497ee19d258a092eaffa242b7f65")
}

// UnpackInvalidImplementationError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidImplementation()
func (iAdapterFactory *IAdapterFactory) UnpackInvalidImplementationError(raw []byte) (*IAdapterFactoryInvalidImplementation, error) {
	out := new(IAdapterFactoryInvalidImplementation)
	if err := iAdapterFactory.abi.UnpackIntoInterface(out, "InvalidImplementation", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IAdapterFactoryInvalidVersion represents a InvalidVersion error raised by the IAdapterFactory contract.
type IAdapterFactoryInvalidVersion struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error InvalidVersion()
func IAdapterFactoryInvalidVersionErrorID() common.Hash {
	return common.HexToHash("0xa9146eebce4eb0a5304713148983d3e5e6237160b32fa1cb60ab806d5c36c5ca")
}

// UnpackInvalidVersionError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error InvalidVersion()
func (iAdapterFactory *IAdapterFactory) UnpackInvalidVersionError(raw []byte) (*IAdapterFactoryInvalidVersion, error) {
	out := new(IAdapterFactoryInvalidVersion)
	if err := iAdapterFactory.abi.UnpackIntoInterface(out, "InvalidVersion", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IAdapterFactoryNotOwner represents a NotOwner error raised by the IAdapterFactory contract.
type IAdapterFactoryNotOwner struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error NotOwner()
func IAdapterFactoryNotOwnerErrorID() common.Hash {
	return common.HexToHash("0x30cd74712f59d478562d48e2d35de830db72c60a63dd08ae59199eec990b5bc4")
}

// UnpackNotOwnerError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error NotOwner()
func (iAdapterFactory *IAdapterFactory) UnpackNotOwnerError(raw []byte) (*IAdapterFactoryNotOwner, error) {
	out := new(IAdapterFactoryNotOwner)
	if err := iAdapterFactory.abi.UnpackIntoInterface(out, "NotOwner", raw); err != nil {
		return nil, err
	}
	return out, nil
}

// IAdapterFactoryOldVersion represents a OldVersion error raised by the IAdapterFactory contract.
type IAdapterFactoryOldVersion struct {
}

// ErrorID returns the hash of canonical representation of the error's signature.
//
// Solidity: error OldVersion()
func IAdapterFactoryOldVersionErrorID() common.Hash {
	return common.HexToHash("0x384ebd90a535625f7ad4cce3a0801a073ae6623b808733454b45d176f6722fe2")
}

// UnpackOldVersionError is the Go binding used to decode the provided
// error data into the corresponding Go error struct.
//
// Solidity: error OldVersion()
func (iAdapterFactory *IAdapterFactory) UnpackOldVersionError(raw []byte) (*IAdapterFactoryOldVersion, error) {
	out := new(IAdapterFactoryOldVersion)
	if err := iAdapterFactory.abi.UnpackIntoInterface(out, "OldVersion", raw); err != nil {
		return nil, err
	}
	return out, nil
}
