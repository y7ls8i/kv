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
)

var (
	storage = struct {
		m       sync.RWMutex
		storage map[Key]Value
	}{
		sync.RWMutex{},
		map[Key]Value{},
	}

	subs = struct {
		m      sync.RWMutex
		subs   map[Key][]*subscription  // subscriptions by key
		subIDs map[string]*subscription // subscription by id
	}{
		sync.RWMutex{},
		map[Key][]*subscription{},
		map[string]*subscription{},
	}
)

const (
	OperationAdd Operation = iota
	OperationUpdate
	OperationDelete
)

const CHANGE_CHAN_CAP uint64 = 10

func Get(k Key) (Value, bool) {
	storage.m.RLock()
	defer storage.m.RUnlock()

	v, ok := storage.storage[k]
	return v, ok
}

func Set(k Key, v Value) {
	storage.m.Lock()
	defer storage.m.Unlock()

	op := OperationAdd
	if _, ok := storage.storage[k]; ok {
		op = OperationUpdate
	}

	storage.storage[k] = v

	copied := make(Value, len(v))
	copy(copied, v)
	broadcastChange(op, k, copied)
}

func Delete(k Key) {
	storage.m.Lock()
	defer storage.m.Unlock()

	delete(storage.storage, k)

	broadcastChange(OperationDelete, k, nil)
}

func Length() uint64 {
	storage.m.RLock()
	defer storage.m.RUnlock()

	return uint64(len(storage.storage))
}

func Clear() {
	storage.m.Lock()
	defer storage.m.Unlock()

	for k := range storage.storage {
		broadcastChange(OperationDelete, k, nil)
	}

	storage.storage = map[Key]Value{}
}

func Subscribe(k Key) (id string, ch chan Change) {
	subs.m.Lock()
	defer subs.m.Unlock()

	sub := subscription{
		id:  newSubID(),
		ch:  make(chan Change, CHANGE_CHAN_CAP),
		key: k,
	}

	subs.subs[k] = append(subs.subs[k], &sub)
	subs.subIDs[sub.id] = &sub

	return sub.id, sub.ch
}

func Unsubscribe(id string) {
	subs.m.Lock()
	defer subs.m.Unlock()

	sub, ok := subs.subIDs[id]
	if !ok {
		return
	}

	close(sub.ch)

	subs.subs[sub.key] = slices.DeleteFunc(subs.subs[sub.key], func(sub *subscription) bool {
		return sub.id == id
	})

	delete(subs.subIDs, id)
}

func broadcastChange(op Operation, k Key, v Value) {
	subs.m.RLock()
	defer subs.m.RUnlock()

	change := Change{
		Op:    op,
		Value: v,
	}

	for _, sub := range subs.subs[k] {
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
