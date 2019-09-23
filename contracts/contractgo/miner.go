// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contractgo

import (
	"math/big"
	"strings"

	kaleido "github.com/kaleidochain/kaleido"
	"github.com/kaleidochain/kaleido/accounts/abi"
	"github.com/kaleidochain/kaleido/accounts/abi/bind"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = big.NewInt
	_ = strings.NewReader
	_ = kaleido.NotFound
	_ = abi.U256
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// MinerABI is the input ABI used to generate the binding from.
const MinerABI = "[{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"name\":\"start\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"miner\",\"type\":\"address\"}],\"name\":\"Added\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"name\":\"start\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"miner\",\"type\":\"address\"}],\"name\":\"Updated\",\"type\":\"event\"},{\"constant\":false,\"inputs\":[{\"name\":\"start\",\"type\":\"uint64\"},{\"name\":\"lifespan\",\"type\":\"uint32\"},{\"name\":\"coinbase\",\"type\":\"address\"},{\"name\":\"vrfVerifier\",\"type\":\"bytes32\"},{\"name\":\"voteVerifier\",\"type\":\"bytes32\"}],\"name\":\"set\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"number\",\"type\":\"uint256\"},{\"name\":\"coinbase\",\"type\":\"address\"}],\"name\":\"setCoinbase\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"number\",\"type\":\"uint256\"},{\"name\":\"miner\",\"type\":\"address\"}],\"name\":\"get\",\"outputs\":[{\"name\":\"\",\"type\":\"uint64\"},{\"name\":\"\",\"type\":\"uint32\"},{\"name\":\"\",\"type\":\"address\"},{\"name\":\"\",\"type\":\"bytes32\"},{\"name\":\"\",\"type\":\"bytes32\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"number\",\"type\":\"uint256\"},{\"name\":\"addr\",\"type\":\"address\"}],\"name\":\"isMinerOfHeight\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"number\",\"type\":\"uint256\"}],\"name\":\"getNewAddedMinersCount\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"number\",\"type\":\"uint256\"},{\"name\":\"index\",\"type\":\"uint32\"}],\"name\":\"getNewAddedMiner\",\"outputs\":[{\"name\":\"miner\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"}]"

// Miner is an auto generated Go binding around an Ethereum contract.
type Miner struct {
	MinerCaller     // Read-only binding to the contract
	MinerTransactor // Write-only binding to the contract
	MinerFilterer   // Log filterer for contract events
}

// MinerCaller is an auto generated read-only Go binding around an Ethereum contract.
type MinerCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MinerTransactor is an auto generated write-only Go binding around an Ethereum contract.
type MinerTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MinerFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type MinerFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MinerSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type MinerSession struct {
	Contract     *Miner            // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// MinerCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type MinerCallerSession struct {
	Contract *MinerCaller  // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// MinerTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type MinerTransactorSession struct {
	Contract     *MinerTransactor  // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// MinerRaw is an auto generated low-level Go binding around an Ethereum contract.
type MinerRaw struct {
	Contract *Miner // Generic contract binding to access the raw methods on
}

// MinerCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type MinerCallerRaw struct {
	Contract *MinerCaller // Generic read-only contract binding to access the raw methods on
}

// MinerTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type MinerTransactorRaw struct {
	Contract *MinerTransactor // Generic write-only contract binding to access the raw methods on
}

// NewMiner creates a new instance of Miner, bound to a specific deployed contract.
func NewMiner(address common.Address, backend bind.ContractBackend) (*Miner, error) {
	contract, err := bindMiner(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Miner{MinerCaller: MinerCaller{contract: contract}, MinerTransactor: MinerTransactor{contract: contract}, MinerFilterer: MinerFilterer{contract: contract}}, nil
}

// NewMinerCaller creates a new read-only instance of Miner, bound to a specific deployed contract.
func NewMinerCaller(address common.Address, caller bind.ContractCaller) (*MinerCaller, error) {
	contract, err := bindMiner(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &MinerCaller{contract: contract}, nil
}

// NewMinerTransactor creates a new write-only instance of Miner, bound to a specific deployed contract.
func NewMinerTransactor(address common.Address, transactor bind.ContractTransactor) (*MinerTransactor, error) {
	contract, err := bindMiner(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &MinerTransactor{contract: contract}, nil
}

// NewMinerFilterer creates a new log filterer instance of Miner, bound to a specific deployed contract.
func NewMinerFilterer(address common.Address, filterer bind.ContractFilterer) (*MinerFilterer, error) {
	contract, err := bindMiner(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &MinerFilterer{contract: contract}, nil
}

// bindMiner binds a generic wrapper to an already deployed contract.
func bindMiner(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(MinerABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Miner *MinerRaw) Call(opts *bind.CallOpts, result interface{}, method string, params ...interface{}) error {
	return _Miner.Contract.MinerCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Miner *MinerRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Miner.Contract.MinerTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Miner *MinerRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Miner.Contract.MinerTransactor.contract.Transact(opts, method, params...)
}

// Transact invokes the (paid) contract method with params as input values, without autofill gas limit.
func (_Miner *MinerRaw) TransactExact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Miner.Contract.MinerTransactor.contract.TransactExact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Miner *MinerCallerRaw) Call(opts *bind.CallOpts, result interface{}, method string, params ...interface{}) error {
	return _Miner.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Miner *MinerTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Miner.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Miner *MinerTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Miner.Contract.contract.Transact(opts, method, params...)
}

// Get is a free data retrieval call binding the contract method 0x7db6e675.
//
// Solidity: function get(uint256 number, address miner) constant returns(uint64, uint32, address, bytes32, bytes32)
func (_Miner *MinerCaller) Get(opts *bind.CallOpts, number *big.Int, miner common.Address) (uint64, uint32, common.Address, [32]byte, [32]byte, error) {
	var (
		ret0 = new(uint64)
		ret1 = new(uint32)
		ret2 = new(common.Address)
		ret3 = new([32]byte)
		ret4 = new([32]byte)
	)
	out := &[]interface{}{
		ret0,
		ret1,
		ret2,
		ret3,
		ret4,
	}
	err := _Miner.contract.Call(opts, out, "get", number, miner)
	return *ret0, *ret1, *ret2, *ret3, *ret4, err
}

// Get is a free data retrieval call binding the contract method 0x7db6e675.
//
// Solidity: function get(uint256 number, address miner) constant returns(uint64, uint32, address, bytes32, bytes32)
func (_Miner *MinerSession) Get(number *big.Int, miner common.Address) (uint64, uint32, common.Address, [32]byte, [32]byte, error) {
	return _Miner.Contract.Get(&_Miner.CallOpts, number, miner)
}

// Get is a free data retrieval call binding the contract method 0x7db6e675.
//
// Solidity: function get(uint256 number, address miner) constant returns(uint64, uint32, address, bytes32, bytes32)
func (_Miner *MinerCallerSession) Get(number *big.Int, miner common.Address) (uint64, uint32, common.Address, [32]byte, [32]byte, error) {
	return _Miner.Contract.Get(&_Miner.CallOpts, number, miner)
}

// GetNewAddedMiner is a free data retrieval call binding the contract method 0x1089602c.
//
// Solidity: function getNewAddedMiner(uint256 number, uint32 index) constant returns(address miner)
func (_Miner *MinerCaller) GetNewAddedMiner(opts *bind.CallOpts, number *big.Int, index uint32) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Miner.contract.Call(opts, out, "getNewAddedMiner", number, index)
	return *ret0, err
}

// GetNewAddedMiner is a free data retrieval call binding the contract method 0x1089602c.
//
// Solidity: function getNewAddedMiner(uint256 number, uint32 index) constant returns(address miner)
func (_Miner *MinerSession) GetNewAddedMiner(number *big.Int, index uint32) (common.Address, error) {
	return _Miner.Contract.GetNewAddedMiner(&_Miner.CallOpts, number, index)
}

// GetNewAddedMiner is a free data retrieval call binding the contract method 0x1089602c.
//
// Solidity: function getNewAddedMiner(uint256 number, uint32 index) constant returns(address miner)
func (_Miner *MinerCallerSession) GetNewAddedMiner(number *big.Int, index uint32) (common.Address, error) {
	return _Miner.Contract.GetNewAddedMiner(&_Miner.CallOpts, number, index)
}

// GetNewAddedMinersCount is a free data retrieval call binding the contract method 0x9532c1fb.
//
// Solidity: function getNewAddedMinersCount(uint256 number) constant returns(uint256)
func (_Miner *MinerCaller) GetNewAddedMinersCount(opts *bind.CallOpts, number *big.Int) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Miner.contract.Call(opts, out, "getNewAddedMinersCount", number)
	return *ret0, err
}

// GetNewAddedMinersCount is a free data retrieval call binding the contract method 0x9532c1fb.
//
// Solidity: function getNewAddedMinersCount(uint256 number) constant returns(uint256)
func (_Miner *MinerSession) GetNewAddedMinersCount(number *big.Int) (*big.Int, error) {
	return _Miner.Contract.GetNewAddedMinersCount(&_Miner.CallOpts, number)
}

// GetNewAddedMinersCount is a free data retrieval call binding the contract method 0x9532c1fb.
//
// Solidity: function getNewAddedMinersCount(uint256 number) constant returns(uint256)
func (_Miner *MinerCallerSession) GetNewAddedMinersCount(number *big.Int) (*big.Int, error) {
	return _Miner.Contract.GetNewAddedMinersCount(&_Miner.CallOpts, number)
}

// IsMinerOfHeight is a free data retrieval call binding the contract method 0xe2162f0b.
//
// Solidity: function isMinerOfHeight(uint256 number, address addr) constant returns(bool)
func (_Miner *MinerCaller) IsMinerOfHeight(opts *bind.CallOpts, number *big.Int, addr common.Address) (bool, error) {
	var (
		ret0 = new(bool)
	)
	out := ret0
	err := _Miner.contract.Call(opts, out, "isMinerOfHeight", number, addr)
	return *ret0, err
}

// IsMinerOfHeight is a free data retrieval call binding the contract method 0xe2162f0b.
//
// Solidity: function isMinerOfHeight(uint256 number, address addr) constant returns(bool)
func (_Miner *MinerSession) IsMinerOfHeight(number *big.Int, addr common.Address) (bool, error) {
	return _Miner.Contract.IsMinerOfHeight(&_Miner.CallOpts, number, addr)
}

// IsMinerOfHeight is a free data retrieval call binding the contract method 0xe2162f0b.
//
// Solidity: function isMinerOfHeight(uint256 number, address addr) constant returns(bool)
func (_Miner *MinerCallerSession) IsMinerOfHeight(number *big.Int, addr common.Address) (bool, error) {
	return _Miner.Contract.IsMinerOfHeight(&_Miner.CallOpts, number, addr)
}

// Set is a paid mutator transaction binding the contract method 0x39fb25e9.
//
// Solidity: function set(uint64 start, uint32 lifespan, address coinbase, bytes32 vrfVerifier, bytes32 voteVerifier) returns(bool)
func (_Miner *MinerTransactor) Set(opts *bind.TransactOpts, start uint64, lifespan uint32, coinbase common.Address, vrfVerifier [32]byte, voteVerifier [32]byte) (*types.Transaction, error) {
	return _Miner.contract.Transact(opts, "set", start, lifespan, coinbase, vrfVerifier, voteVerifier)
}

// Set is a paid mutator transaction binding the contract method 0x39fb25e9.
//
// Solidity: function set(uint64 start, uint32 lifespan, address coinbase, bytes32 vrfVerifier, bytes32 voteVerifier) returns(bool)
func (_Miner *MinerSession) Set(start uint64, lifespan uint32, coinbase common.Address, vrfVerifier [32]byte, voteVerifier [32]byte) (*types.Transaction, error) {
	return _Miner.Contract.Set(&_Miner.TransactOpts, start, lifespan, coinbase, vrfVerifier, voteVerifier)
}

// Set is a paid mutator transaction binding the contract method 0x39fb25e9.
//
// Solidity: function set(uint64 start, uint32 lifespan, address coinbase, bytes32 vrfVerifier, bytes32 voteVerifier) returns(bool)
func (_Miner *MinerTransactorSession) Set(start uint64, lifespan uint32, coinbase common.Address, vrfVerifier [32]byte, voteVerifier [32]byte) (*types.Transaction, error) {
	return _Miner.Contract.Set(&_Miner.TransactOpts, start, lifespan, coinbase, vrfVerifier, voteVerifier)
}

// SetCoinbase is a paid mutator transaction binding the contract method 0xfcea86d7.
//
// Solidity: function setCoinbase(uint256 number, address coinbase) returns(bool)
func (_Miner *MinerTransactor) SetCoinbase(opts *bind.TransactOpts, number *big.Int, coinbase common.Address) (*types.Transaction, error) {
	return _Miner.contract.Transact(opts, "setCoinbase", number, coinbase)
}

// SetCoinbase is a paid mutator transaction binding the contract method 0xfcea86d7.
//
// Solidity: function setCoinbase(uint256 number, address coinbase) returns(bool)
func (_Miner *MinerSession) SetCoinbase(number *big.Int, coinbase common.Address) (*types.Transaction, error) {
	return _Miner.Contract.SetCoinbase(&_Miner.TransactOpts, number, coinbase)
}

// SetCoinbase is a paid mutator transaction binding the contract method 0xfcea86d7.
//
// Solidity: function setCoinbase(uint256 number, address coinbase) returns(bool)
func (_Miner *MinerTransactorSession) SetCoinbase(number *big.Int, coinbase common.Address) (*types.Transaction, error) {
	return _Miner.Contract.SetCoinbase(&_Miner.TransactOpts, number, coinbase)
}

// MinerAddedIterator is returned from FilterAdded and is used to iterate over the raw logs and unpacked data for Added events raised by the Miner contract.
type MinerAddedIterator struct {
	Event *MinerAdded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log       // Log channel receiving the found contract events
	sub  kaleido.Subscription // Subscription for errors, completion and termination
	done bool                 // Whether the subscription completed delivering logs
	fail error                // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *MinerAddedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MinerAdded)
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
		it.Event = new(MinerAdded)
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
func (it *MinerAddedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MinerAddedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MinerAdded represents a Added event raised by the Miner contract.
type MinerAdded struct {
	Start *big.Int
	Miner common.Address
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterAdded is a free log retrieval operation binding the contract event 0x8ba5cdc385bc6c37c9a05ad09fbc238589c43602f4cee09c46856027df8a935a.
//
// Solidity: event Added(uint256 start, address miner)
func (_Miner *MinerFilterer) FilterAdded(opts *bind.FilterOpts) (*MinerAddedIterator, error) {

	logs, sub, err := _Miner.contract.FilterLogs(opts, "Added")
	if err != nil {
		return nil, err
	}
	return &MinerAddedIterator{contract: _Miner.contract, event: "Added", logs: logs, sub: sub}, nil
}

// WatchAdded is a free log subscription operation binding the contract event 0x8ba5cdc385bc6c37c9a05ad09fbc238589c43602f4cee09c46856027df8a935a.
//
// Solidity: event Added(uint256 start, address miner)
func (_Miner *MinerFilterer) WatchAdded(opts *bind.WatchOpts, sink chan<- *MinerAdded) (event.Subscription, error) {

	logs, sub, err := _Miner.contract.WatchLogs(opts, "Added")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MinerAdded)
				if err := _Miner.contract.UnpackLog(event, "Added", log); err != nil {
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

// MinerUpdatedIterator is returned from FilterUpdated and is used to iterate over the raw logs and unpacked data for Updated events raised by the Miner contract.
type MinerUpdatedIterator struct {
	Event *MinerUpdated // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log       // Log channel receiving the found contract events
	sub  kaleido.Subscription // Subscription for errors, completion and termination
	done bool                 // Whether the subscription completed delivering logs
	fail error                // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *MinerUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MinerUpdated)
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
		it.Event = new(MinerUpdated)
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
func (it *MinerUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MinerUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MinerUpdated represents a Updated event raised by the Miner contract.
type MinerUpdated struct {
	Start *big.Int
	Miner common.Address
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterUpdated is a free log retrieval operation binding the contract event 0x335a1db8707ab5a5665bf4ad34e12411cb5e3ca47341ada89f8e263e20312347.
//
// Solidity: event Updated(uint256 start, address miner)
func (_Miner *MinerFilterer) FilterUpdated(opts *bind.FilterOpts) (*MinerUpdatedIterator, error) {

	logs, sub, err := _Miner.contract.FilterLogs(opts, "Updated")
	if err != nil {
		return nil, err
	}
	return &MinerUpdatedIterator{contract: _Miner.contract, event: "Updated", logs: logs, sub: sub}, nil
}

// WatchUpdated is a free log subscription operation binding the contract event 0x335a1db8707ab5a5665bf4ad34e12411cb5e3ca47341ada89f8e263e20312347.
//
// Solidity: event Updated(uint256 start, address miner)
func (_Miner *MinerFilterer) WatchUpdated(opts *bind.WatchOpts, sink chan<- *MinerUpdated) (event.Subscription, error) {

	logs, sub, err := _Miner.contract.WatchLogs(opts, "Updated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MinerUpdated)
				if err := _Miner.contract.UnpackLog(event, "Updated", log); err != nil {
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
