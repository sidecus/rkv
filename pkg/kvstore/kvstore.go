package kvstore

import (
	"errors"
	"sync"

	"github.com/sidecus/raft/pkg/raft"
)

const (
	// KVCmdSet Set a key/value pair
	KVCmdSet = 1
	// KVCmdDel Delete a key/value pair
	KVCmdDel = 2
)

// KVCmdData represents one Key/Value command data in the log entry
type KVCmdData struct {
	Key   string
	Value string
}

// KVStore is a concurrency safe kv store
type KVStore struct {
	mu   sync.RWMutex
	data map[string]string
}

// NewKVStore creates a kv store
func NewKVStore() *KVStore {
	store := &KVStore{
		data: make(map[string]string),
	}
	return store
}

// Apply applies the cmd to the kv store with concurrency safety
func (store *KVStore) Apply(cmd raft.StateMachineCmd) {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.apply(cmd)
}

// Init clears the map and then applies all the given cmds with concurrency safety
func (store *KVStore) Init(cmds []raft.StateMachineCmd) {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.data = make(map[string]string)
	for _, c := range cmds {
		store.apply(c)
	}
}

// GetValue a value from store
func (store *KVStore) GetValue(key string) (val string, ok bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	val, ok = store.data[key]
	return val, ok
}

// Get Implements IStateMachine.Get
func (store *KVStore) Get(param ...interface{}) (result interface{}, err error) {
	if len(param) != 1 {
		return nil, errors.New("no key provided")
	}

	key := param[0].(string)
	val, ok := store.GetValue(key)
	if !ok {
		return nil, errors.New("key doesn't exist")
	}

	return val, nil
}

// apply applies a command to the store, with no lock
func (store *KVStore) apply(cmd raft.StateMachineCmd) {
	data := cmd.Data.(KVCmdData)
	if cmd.CmdType == KVCmdSet {
		store.data[data.Key] = data.Value
	} else if cmd.CmdType == KVCmdDel {
		delete(store.data, data.Key)
	} else {
		panic("unexpected kv cmdtype")
	}
}
