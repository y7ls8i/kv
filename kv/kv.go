package kv

import (
	"fmt"
	"slices"
	"sync"
	"time"
)

type (
	Key   = string
	Value = []byte

	Operation int

	Change struct {
		Op    Operation
		Value Value
	}

	subscription struct {
		id  string
		ch  chan Change
		key Key
	}

	kvStorage struct {
		m       sync.RWMutex
		storage map[Key]Value
	}

	kvSubs struct {
		m      sync.RWMutex
		subs   map[Key][]*subscription  // subscriptions by key
		subIDs map[string]*subscription // subscription by id
	}

	kv struct {
		storage kvStorage
		subs    kvSubs
	}

	kvs struct {
		m   sync.RWMutex
		kvs map[string]*kv
	}
)

var all = kvs{
	sync.RWMutex{},
	map[string]*kv{},
}

const (
	OperationAdd Operation = iota
	OperationUpdate
	OperationDelete
)

const CHANGE_CHAN_CAP uint64 = 10

func getOrCreate(name string) *kv {
	all.m.RLock()
	existing := all.kvs[name]
	all.m.RUnlock()

	if existing != nil {
		return existing
	}

	newKV := &kv{
		storage: kvStorage{
			sync.RWMutex{},
			map[Key]Value{},
		},
		subs: kvSubs{
			sync.RWMutex{},
			map[Key][]*subscription{},
			map[string]*subscription{},
		},
	}

	all.m.Lock()
	all.kvs[name] = newKV
	all.m.Unlock()

	return newKV
}

func Get(name string, k Key) (Value, bool) {
	kv := getOrCreate(name)

	kv.storage.m.RLock()
	defer kv.storage.m.RUnlock()

	v, ok := kv.storage.storage[k]
	return v, ok
}

func Set(name string, k Key, v Value) {
	kv := getOrCreate(name)

	kv.storage.m.Lock()
	defer kv.storage.m.Unlock()

	op := OperationAdd
	if _, ok := kv.storage.storage[k]; ok {
		op = OperationUpdate
	}

	kv.storage.storage[k] = v

	copied := make(Value, len(v))
	copy(copied, v)
	broadcastChange(name, op, k, copied)
}

func Delete(name string, k Key) {
	kv := getOrCreate(name)

	kv.storage.m.Lock()
	defer kv.storage.m.Unlock()

	delete(kv.storage.storage, k)

	broadcastChange(name, OperationDelete, k, nil)
}

func Length(name string) uint64 {
	kv := getOrCreate(name)

	kv.storage.m.RLock()
	defer kv.storage.m.RUnlock()

	return uint64(len(kv.storage.storage))
}

func Clear(name string) {
	kv := getOrCreate(name)

	kv.storage.m.Lock()
	defer kv.storage.m.Unlock()

	for k := range kv.storage.storage {
		broadcastChange(name, OperationDelete, k, nil)
	}

	kv.storage.storage = map[Key]Value{}
}

func Subscribe(name string, k Key) (id string, ch <-chan Change) {
	kv := getOrCreate(name)

	kv.subs.m.Lock()
	defer kv.subs.m.Unlock()

	sub := subscription{
		id:  newSubID(),
		ch:  make(chan Change, CHANGE_CHAN_CAP),
		key: k,
	}

	kv.subs.subs[k] = append(kv.subs.subs[k], &sub)
	kv.subs.subIDs[sub.id] = &sub

	return sub.id, sub.ch
}

func Unsubscribe(name string, id string) {
	kv := getOrCreate(name)

	kv.subs.m.Lock()
	defer kv.subs.m.Unlock()

	sub, ok := kv.subs.subIDs[id]
	if !ok {
		return
	}

	close(sub.ch)

	kv.subs.subs[sub.key] = slices.DeleteFunc(kv.subs.subs[sub.key], func(sub *subscription) bool {
		return sub.id == id
	})

	delete(kv.subs.subIDs, id)
}

func broadcastChange(name string, op Operation, k Key, v Value) {
	kv := getOrCreate(name)

	kv.subs.m.RLock()
	defer kv.subs.m.RUnlock()

	change := Change{
		Op:    op,
		Value: v,
	}

	for _, sub := range kv.subs.subs[k] {
		// if the consumer is too slow, we drop the change notification.
		if len(sub.ch) < cap(sub.ch) {
			sub.ch <- change
		}
	}
}

// TODO may add some random string
func newSubID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
