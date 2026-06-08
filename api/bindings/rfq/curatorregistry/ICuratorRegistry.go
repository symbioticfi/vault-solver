// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package curatorregistry

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

// ICuratorRegistryMetaData contains all meta data concerning the ICuratorRegistry contract.
var ICuratorRegistryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"getCurator\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCuratorAt\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"timestamp\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"hint\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"setCurator\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"curator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"SetCurator\",\"inputs\":[{\"name\":\"vault\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"curator\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"NotAuthorized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotVault\",\"inputs\":[]}]",
}

// ICuratorRegistryABI is the input ABI used to generate the binding from.
// Deprecated: Use ICuratorRegistryMetaData.ABI instead.
var ICuratorRegistryABI = ICuratorRegistryMetaData.ABI

// ICuratorRegistry is an auto generated Go binding around an Ethereum contract.
type ICuratorRegistry struct {
	ICuratorRegistryCaller     // Read-only binding to the contract
	ICuratorRegistryTransactor // Write-only binding to the contract
	ICuratorRegistryFilterer   // Log filterer for contract events
}

// ICuratorRegistryCaller is an auto generated read-only Go binding around an Ethereum contract.
type ICuratorRegistryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ICuratorRegistryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ICuratorRegistryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ICuratorRegistryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ICuratorRegistryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ICuratorRegistrySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ICuratorRegistrySession struct {
	Contract     *ICuratorRegistry // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ICuratorRegistryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ICuratorRegistryCallerSession struct {
	Contract *ICuratorRegistryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts           // Call options to use throughout this session
}

// ICuratorRegistryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ICuratorRegistryTransactorSession struct {
	Contract     *ICuratorRegistryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts           // Transaction auth options to use throughout this session
}

// ICuratorRegistryRaw is an auto generated low-level Go binding around an Ethereum contract.
type ICuratorRegistryRaw struct {
	Contract *ICuratorRegistry // Generic contract binding to access the raw methods on
}

// ICuratorRegistryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ICuratorRegistryCallerRaw struct {
	Contract *ICuratorRegistryCaller // Generic read-only contract binding to access the raw methods on
}

// ICuratorRegistryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ICuratorRegistryTransactorRaw struct {
	Contract *ICuratorRegistryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewICuratorRegistry creates a new instance of ICuratorRegistry, bound to a specific deployed contract.
func NewICuratorRegistry(address common.Address, backend bind.ContractBackend) (*ICuratorRegistry, error) {
	contract, err := bindICuratorRegistry(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ICuratorRegistry{ICuratorRegistryCaller: ICuratorRegistryCaller{contract: contract}, ICuratorRegistryTransactor: ICuratorRegistryTransactor{contract: contract}, ICuratorRegistryFilterer: ICuratorRegistryFilterer{contract: contract}}, nil
}

// NewICuratorRegistryCaller creates a new read-only instance of ICuratorRegistry, bound to a specific deployed contract.
func NewICuratorRegistryCaller(address common.Address, caller bind.ContractCaller) (*ICuratorRegistryCaller, error) {
	contract, err := bindICuratorRegistry(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ICuratorRegistryCaller{contract: contract}, nil
}

// NewICuratorRegistryTransactor creates a new write-only instance of ICuratorRegistry, bound to a specific deployed contract.
func NewICuratorRegistryTransactor(address common.Address, transactor bind.ContractTransactor) (*ICuratorRegistryTransactor, error) {
	contract, err := bindICuratorRegistry(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ICuratorRegistryTransactor{contract: contract}, nil
}

// NewICuratorRegistryFilterer creates a new log filterer instance of ICuratorRegistry, bound to a specific deployed contract.
func NewICuratorRegistryFilterer(address common.Address, filterer bind.ContractFilterer) (*ICuratorRegistryFilterer, error) {
	contract, err := bindICuratorRegistry(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ICuratorRegistryFilterer{contract: contract}, nil
}

// bindICuratorRegistry binds a generic wrapper to an already deployed contract.
func bindICuratorRegistry(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ICuratorRegistryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ICuratorRegistry *ICuratorRegistryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ICuratorRegistry.Contract.ICuratorRegistryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ICuratorRegistry *ICuratorRegistryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ICuratorRegistry.Contract.ICuratorRegistryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ICuratorRegistry *ICuratorRegistryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ICuratorRegistry.Contract.ICuratorRegistryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ICuratorRegistry *ICuratorRegistryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ICuratorRegistry.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ICuratorRegistry *ICuratorRegistryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ICuratorRegistry.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ICuratorRegistry *ICuratorRegistryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ICuratorRegistry.Contract.contract.Transact(opts, method, params...)
}

// GetCurator is a free data retrieval call binding the contract method 0x92da327d.
//
// Solidity: function getCurator(address vault) view returns(address)
func (_ICuratorRegistry *ICuratorRegistryCaller) GetCurator(opts *bind.CallOpts, vault common.Address) (common.Address, error) {
	var out []interface{}
	err := _ICuratorRegistry.contract.Call(opts, &out, "getCurator", vault)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetCurator is a free data retrieval call binding the contract method 0x92da327d.
//
// Solidity: function getCurator(address vault) view returns(address)
func (_ICuratorRegistry *ICuratorRegistrySession) GetCurator(vault common.Address) (common.Address, error) {
	return _ICuratorRegistry.Contract.GetCurator(&_ICuratorRegistry.CallOpts, vault)
}

// GetCurator is a free data retrieval call binding the contract method 0x92da327d.
//
// Solidity: function getCurator(address vault) view returns(address)
func (_ICuratorRegistry *ICuratorRegistryCallerSession) GetCurator(vault common.Address) (common.Address, error) {
	return _ICuratorRegistry.Contract.GetCurator(&_ICuratorRegistry.CallOpts, vault)
}

// GetCuratorAt is a free data retrieval call binding the contract method 0x99e8a1e5.
//
// Solidity: function getCuratorAt(address vault, uint48 timestamp, bytes hint) view returns(address)
func (_ICuratorRegistry *ICuratorRegistryCaller) GetCuratorAt(opts *bind.CallOpts, vault common.Address, timestamp *big.Int, hint []byte) (common.Address, error) {
	var out []interface{}
	err := _ICuratorRegistry.contract.Call(opts, &out, "getCuratorAt", vault, timestamp, hint)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetCuratorAt is a free data retrieval call binding the contract method 0x99e8a1e5.
//
// Solidity: function getCuratorAt(address vault, uint48 timestamp, bytes hint) view returns(address)
func (_ICuratorRegistry *ICuratorRegistrySession) GetCuratorAt(vault common.Address, timestamp *big.Int, hint []byte) (common.Address, error) {
	return _ICuratorRegistry.Contract.GetCuratorAt(&_ICuratorRegistry.CallOpts, vault, timestamp, hint)
}

// GetCuratorAt is a free data retrieval call binding the contract method 0x99e8a1e5.
//
// Solidity: function getCuratorAt(address vault, uint48 timestamp, bytes hint) view returns(address)
func (_ICuratorRegistry *ICuratorRegistryCallerSession) GetCuratorAt(vault common.Address, timestamp *big.Int, hint []byte) (common.Address, error) {
	return _ICuratorRegistry.Contract.GetCuratorAt(&_ICuratorRegistry.CallOpts, vault, timestamp, hint)
}

// SetCurator is a paid mutator transaction binding the contract method 0xf9559f76.
//
// Solidity: function setCurator(address vault, address curator) returns()
func (_ICuratorRegistry *ICuratorRegistryTransactor) SetCurator(opts *bind.TransactOpts, vault common.Address, curator common.Address) (*types.Transaction, error) {
	return _ICuratorRegistry.contract.Transact(opts, "setCurator", vault, curator)
}

// SetCurator is a paid mutator transaction binding the contract method 0xf9559f76.
//
// Solidity: function setCurator(address vault, address curator) returns()
func (_ICuratorRegistry *ICuratorRegistrySession) SetCurator(vault common.Address, curator common.Address) (*types.Transaction, error) {
	return _ICuratorRegistry.Contract.SetCurator(&_ICuratorRegistry.TransactOpts, vault, curator)
}

// SetCurator is a paid mutator transaction binding the contract method 0xf9559f76.
//
// Solidity: function setCurator(address vault, address curator) returns()
func (_ICuratorRegistry *ICuratorRegistryTransactorSession) SetCurator(vault common.Address, curator common.Address) (*types.Transaction, error) {
	return _ICuratorRegistry.Contract.SetCurator(&_ICuratorRegistry.TransactOpts, vault, curator)
}

// ICuratorRegistrySetCuratorIterator is returned from FilterSetCurator and is used to iterate over the raw logs and unpacked data for SetCurator events raised by the ICuratorRegistry contract.
type ICuratorRegistrySetCuratorIterator struct {
	Event *ICuratorRegistrySetCurator // Event containing the contract specifics and raw log

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
func (it *ICuratorRegistrySetCuratorIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ICuratorRegistrySetCurator)
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
		it.Event = new(ICuratorRegistrySetCurator)
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
func (it *ICuratorRegistrySetCuratorIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ICuratorRegistrySetCuratorIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ICuratorRegistrySetCurator represents a SetCurator event raised by the ICuratorRegistry contract.
type ICuratorRegistrySetCurator struct {
	Vault   common.Address
	Curator common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterSetCurator is a free log retrieval operation binding the contract event 0xcc0b3569ddaeda670da96a55159fea760f507428e58b42a8511820497e56d1a0.
//
// Solidity: event SetCurator(address indexed vault, address indexed curator)
func (_ICuratorRegistry *ICuratorRegistryFilterer) FilterSetCurator(opts *bind.FilterOpts, vault []common.Address, curator []common.Address) (*ICuratorRegistrySetCuratorIterator, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var curatorRule []interface{}
	for _, curatorItem := range curator {
		curatorRule = append(curatorRule, curatorItem)
	}

	logs, sub, err := _ICuratorRegistry.contract.FilterLogs(opts, "SetCurator", vaultRule, curatorRule)
	if err != nil {
		return nil, err
	}
	return &ICuratorRegistrySetCuratorIterator{contract: _ICuratorRegistry.contract, event: "SetCurator", logs: logs, sub: sub}, nil
}

// WatchSetCurator is a free log subscription operation binding the contract event 0xcc0b3569ddaeda670da96a55159fea760f507428e58b42a8511820497e56d1a0.
//
// Solidity: event SetCurator(address indexed vault, address indexed curator)
func (_ICuratorRegistry *ICuratorRegistryFilterer) WatchSetCurator(opts *bind.WatchOpts, sink chan<- *ICuratorRegistrySetCurator, vault []common.Address, curator []common.Address) (event.Subscription, error) {

	var vaultRule []interface{}
	for _, vaultItem := range vault {
		vaultRule = append(vaultRule, vaultItem)
	}
	var curatorRule []interface{}
	for _, curatorItem := range curator {
		curatorRule = append(curatorRule, curatorItem)
	}

	logs, sub, err := _ICuratorRegistry.contract.WatchLogs(opts, "SetCurator", vaultRule, curatorRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ICuratorRegistrySetCurator)
				if err := _ICuratorRegistry.contract.UnpackLog(event, "SetCurator", log); err != nil {
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

// ParseSetCurator is a log parse operation binding the contract event 0xcc0b3569ddaeda670da96a55159fea760f507428e58b42a8511820497e56d1a0.
//
// Solidity: event SetCurator(address indexed vault, address indexed curator)
func (_ICuratorRegistry *ICuratorRegistryFilterer) ParseSetCurator(log types.Log) (*ICuratorRegistrySetCurator, error) {
	event := new(ICuratorRegistrySetCurator)
	if err := _ICuratorRegistry.contract.UnpackLog(event, "SetCurator", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
