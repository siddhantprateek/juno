package core

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/NethermindEth/juno/core/crypto"
	"github.com/NethermindEth/juno/core/felt"
	"github.com/NethermindEth/juno/core/trie"
	"github.com/NethermindEth/juno/utils"
	"github.com/ethereum/go-ethereum/common"
)

type Event struct {
	Data []*felt.Felt
	From *felt.Felt
	Keys []*felt.Felt
}

type L1ToL2Message struct {
	From     common.Address
	Nonce    *felt.Felt
	Payload  []*felt.Felt
	Selector *felt.Felt
	To       *felt.Felt
}

type L2ToL1Message struct {
	From    *felt.Felt
	Payload []*felt.Felt
	To      common.Address
}

type ExecutionResources struct {
	BuiltinInstanceCounter BuiltinInstanceCounter
	MemoryHoles            uint64
	Steps                  uint64
}

type BuiltinInstanceCounter struct {
	Bitwise    uint64
	EcOp       uint64
	Ecsda      uint64
	Output     uint64
	Pedersen   uint64
	RangeCheck uint64
}

type TransactionReceipt struct {
	Fee                *felt.Felt
	Events             []*Event
	ExecutionResources *ExecutionResources
	L1ToL2Message      *L1ToL2Message
	L2ToL1Message      []*L2ToL1Message
	TransactionHash    *felt.Felt
}

type Transaction interface {
	Hash() *felt.Felt
	Signature() []*felt.Felt
}

var (
	_ Transaction = (*DeployTransaction)(nil)
	_ Transaction = (*DeployAccountTransaction)(nil)
	_ Transaction = (*DeclareTransaction)(nil)
	_ Transaction = (*InvokeTransaction)(nil)
	_ Transaction = (*L1HandlerTransaction)(nil)
)

type DeployTransaction struct {
	TransactionHash *felt.Felt
	// A random number used to distinguish between different instances of the contract.
	ContractAddressSalt *felt.Felt
	// The address of the contract.
	ContractAddress *felt.Felt
	// The hash of the class which defines the contract’s functionality.
	ClassHash *felt.Felt
	// The arguments passed to the constructor during deployment.
	ConstructorCallData []*felt.Felt
	// The transaction’s version. Possible values are 1 or 0.
	//
	// When the fields that comprise a transaction change,
	// either with the addition of a new field or the removal of an existing field,
	// then the transaction version increases.
	// Transaction version 0 is deprecated and will be removed in a future version of Starknet.
	Version *felt.Felt
}

func (d *DeployTransaction) Hash() *felt.Felt {
	return d.TransactionHash
}

func (d *DeployTransaction) Signature() []*felt.Felt {
	return make([]*felt.Felt, 0)
}

type DeployAccountTransaction struct {
	DeployTransaction
	// The maximum fee that the sender is willing to pay for the transaction.
	MaxFee *felt.Felt
	// Additional information given by the sender, used to validate the transaction.
	TransactionSignature []*felt.Felt
	// The transaction nonce.
	Nonce *felt.Felt
}

func (d *DeployAccountTransaction) Hash() *felt.Felt {
	return d.TransactionHash
}

func (d *DeployAccountTransaction) Signature() []*felt.Felt {
	return d.TransactionSignature
}

type InvokeTransaction struct {
	TransactionHash *felt.Felt
	// The arguments that are passed to the validated and execute functions.
	CallData []*felt.Felt
	// Additional information given by the sender, used to validate the transaction.
	TransactionSignature []*felt.Felt
	// The maximum fee that the sender is willing to pay for the transaction
	MaxFee *felt.Felt
	// The address of the contract invoked by this transaction.
	ContractAddress *felt.Felt
	// When the fields that comprise a transaction change,
	// either with the addition of a new field or the removal of an existing field,
	// then the transaction version increases.
	Version *felt.Felt

	// Version 0 fields
	// The encoding of the selector for the function invoked (the entry point in the contract)
	EntryPointSelector *felt.Felt

	// Version 1 fields
	// The transaction nonce.
	Nonce *felt.Felt
	// The address of the sender of this transaction
	SenderAddress *felt.Felt
}

func (i *InvokeTransaction) Hash() *felt.Felt {
	return i.TransactionHash
}

func (i *InvokeTransaction) Signature() []*felt.Felt {
	return i.TransactionSignature
}

type DeclareTransaction struct {
	TransactionHash *felt.Felt
	// The class hash
	ClassHash *felt.Felt
	// The address of the account initiating the transaction.
	SenderAddress *felt.Felt
	// The maximum fee that the sender is willing to pay for the transaction.
	MaxFee *felt.Felt
	// Additional information given by the sender, used to validate the transaction.
	TransactionSignature []*felt.Felt
	// The transaction nonce.
	Nonce *felt.Felt
	// The transaction’s version. Possible values are 1 or 0.
	// When the fields that comprise a transaction change,
	// either with the addition of a new field or the removal of an existing field,
	// then the transaction version increases.
	// Transaction version 0 is deprecated and will be removed in a future version of Starknet.
	Version *felt.Felt

	// Version 2 fields
	CompiledClassHash *felt.Felt
}

func (d *DeclareTransaction) Hash() *felt.Felt {
	return d.TransactionHash
}

func (d *DeclareTransaction) Signature() []*felt.Felt {
	return d.TransactionSignature
}

type L1HandlerTransaction struct {
	TransactionHash *felt.Felt
	// The address of the contract.
	ContractAddress *felt.Felt
	// The encoding of the selector for the function invoked (the entry point in the contract)
	EntryPointSelector *felt.Felt
	// The transaction nonce.
	Nonce *felt.Felt
	// The arguments that are passed to the validated and execute functions.
	CallData []*felt.Felt
	// When the fields that comprise a transaction change,
	// either with the addition of a new field or the removal of an existing field,
	// then the transaction version increases.
	Version *felt.Felt
}

func (l *L1HandlerTransaction) Hash() *felt.Felt {
	return l.TransactionHash
}

func (l *L1HandlerTransaction) Signature() []*felt.Felt {
	return make([]*felt.Felt, 0)
}

func transactionHash(transaction Transaction, n utils.Network) (*felt.Felt, error) {
	switch t := transaction.(type) {
	case *DeclareTransaction:
		return declareTransactionHash(t, n)
	case *InvokeTransaction:
		return invokeTransactionHash(t, n)
	case *DeployTransaction:
		// deploy transactions are deprecated after re-genesis therefore we don't verify
		// transaction hash
		return t.TransactionHash, nil
	case *L1HandlerTransaction:
		return l1HandlerTransactionHash(t, n)
	case *DeployAccountTransaction:
		return deployAccountTransactionHash(t, n)
	default:
		return nil, errors.New("unknown transaction")
	}
}

var (
	invokeFelt        = new(felt.Felt).SetBytes([]byte("invoke"))
	declareFelt       = new(felt.Felt).SetBytes([]byte("declare"))
	l1HandlerFelt     = new(felt.Felt).SetBytes([]byte("l1_handler"))
	deployAccountFelt = new(felt.Felt).SetBytes([]byte("deploy_account"))
)

func errInvalidTransactionVersion(t Transaction, version *felt.Felt) error {
	return fmt.Errorf("invalid Transaction (type: %v) verion: %v", reflect.TypeOf(t), version.Text(felt.Base10))
}

func invokeTransactionHash(i *InvokeTransaction, n utils.Network) (*felt.Felt, error) {
	switch {
	case i.Version.IsZero():
		// Due to inconsistencies in version 0 hash calculation we don't verify the hash
		return i.TransactionHash, nil
	case i.Version.IsOne():
		return crypto.PedersenArray(
			invokeFelt,
			i.Version,
			i.SenderAddress,
			new(felt.Felt),
			crypto.PedersenArray(i.CallData...),
			i.MaxFee,
			n.ChainID(),
			i.Nonce,
		), nil
	default:
		return nil, errInvalidTransactionVersion(i, i.Version)
	}
}

func declareTransactionHash(d *DeclareTransaction, n utils.Network) (*felt.Felt, error) {
	switch {
	case d.Version.IsZero():
		// Due to inconsistencies in version 0 hash calculation we don't verify the hash
		return d.TransactionHash, nil
	case d.Version.IsOne():
		return crypto.PedersenArray(
			declareFelt,
			d.Version,
			d.SenderAddress,
			new(felt.Felt),
			crypto.PedersenArray(d.ClassHash),
			d.MaxFee,
			n.ChainID(),
			d.Nonce,
		), nil
	case d.Version.Equal(new(felt.Felt).SetUint64(2)):
		return crypto.PedersenArray(
			declareFelt,
			d.Version,
			d.SenderAddress,
			&felt.Zero,
			crypto.PedersenArray(d.ClassHash),
			d.MaxFee,
			n.ChainID(),
			d.Nonce,
			d.CompiledClassHash,
		), nil

	default:
		return nil, errInvalidTransactionVersion(d, d.Version)
	}
}

func l1HandlerTransactionHash(l *L1HandlerTransaction, n utils.Network) (*felt.Felt, error) {
	switch {
	case l.Version.IsZero():
		// There are some l1 handler transaction which do not return a nonce and for some random
		// transaction the following hash fails.
		if l.Nonce == nil {
			return l.TransactionHash, nil
		}
		return crypto.PedersenArray(
			l1HandlerFelt,
			l.Version,
			l.ContractAddress,
			l.EntryPointSelector,
			crypto.PedersenArray(l.CallData...),
			&felt.Zero,
			n.ChainID(),
			l.Nonce,
		), nil
	default:
		return nil, errInvalidTransactionVersion(l, l.Version)
	}
}

func deployAccountTransactionHash(d *DeployAccountTransaction, n utils.Network) (*felt.Felt, error) {
	callData := []*felt.Felt{d.ClassHash, d.ContractAddressSalt}
	callData = append(callData, d.ConstructorCallData...)
	// There is no version 0 for deploy account
	if d.Version.IsOne() {
		return crypto.PedersenArray(
			deployAccountFelt,
			d.Version,
			d.ContractAddress,
			&felt.Zero,
			crypto.PedersenArray(callData...),
			d.MaxFee,
			n.ChainID(),
			d.Nonce,
		), nil
	}
	return nil, errInvalidTransactionVersion(d, d.Version)
}

type CantVerifyTransactionHashError struct {
	t           Transaction
	hashFailure error
	next        *CantVerifyTransactionHashError
}

func (e CantVerifyTransactionHashError) Unwrap() error {
	if e.next != nil {
		return *e.next
	}
	return nil
}

func (e CantVerifyTransactionHashError) Error() string {
	errStr := fmt.Sprintf("cannot verify transaction hash(%v) of Transaction Type: %v",
		e.t.Hash().String(), reflect.TypeOf(e.t))
	if e.hashFailure != nil {
		errStr = fmt.Sprintf("%v: %v", errStr, e.hashFailure.Error())
	}
	return errStr
}

func verifyTransactions(txs []Transaction, n utils.Network) error {
	var head *CantVerifyTransactionHashError
	for _, tx := range txs {
		if err := verifyTransactionHash(tx, n); err != nil {
			err.next = head
			head = err
		}
	}
	if head != nil {
		return *head
	}
	return nil
}

func verifyTransactionHash(t Transaction, n utils.Network) *CantVerifyTransactionHashError {
	calculatedTxHash, err := transactionHash(t, n)
	if err != nil {
		return &CantVerifyTransactionHashError{t: t, hashFailure: err}
	}
	if !calculatedTxHash.Equal(t.Hash()) {
		return &CantVerifyTransactionHashError{t: t}
	}
	return nil
}

const commitmentTrieHeight uint = 64

// transactionCommitment is the root of a height 64 binary Merkle Patricia tree of the
// transaction hashes and signatures in a block.
func transactionCommitment(transactions []Transaction) (*felt.Felt, error) {
	var commitment *felt.Felt
	return commitment, trie.RunOnTempTrie(commitmentTrieHeight, func(trie *trie.Trie) error {
		for i, transaction := range transactions {
			signatureHash := crypto.PedersenArray()
			if _, ok := transaction.(*InvokeTransaction); ok {
				signatureHash = crypto.PedersenArray(transaction.Signature()...)
			}

			if _, err := trie.Put(new(felt.Felt).SetUint64(uint64(i)),
				crypto.Pedersen(transaction.Hash(), signatureHash)); err != nil {
				return err
			}
		}
		root, err := trie.Root()
		if err != nil {
			return err
		}
		commitment = root
		return nil
	})
}

// eventCommitment computes the event commitment for a block.
func eventCommitment(receipts []*TransactionReceipt) (*felt.Felt, error) {
	var commitment *felt.Felt
	return commitment, trie.RunOnTempTrie(commitmentTrieHeight, func(trie *trie.Trie) error {
		count := uint64(0)
		for _, receipt := range receipts {
			for _, event := range receipt.Events {
				eventHash := crypto.PedersenArray(
					event.From,
					crypto.PedersenArray(event.Keys...),
					crypto.PedersenArray(event.Data...),
				)

				if _, err := trie.Put(new(felt.Felt).SetUint64(count), eventHash); err != nil {
					return err
				}
				count++
			}
		}
		root, err := trie.Root()
		if err != nil {
			return err
		}
		commitment = root
		return nil
	})
}
