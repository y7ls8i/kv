package kv

import "sync"

type (
	Key   = string
	Value = []byte
)

var (
	mutex   = sync.RWMutex{}
	storage = map[Key]Value{}
)

func Get(k Key) (Value, bool) {
	mutex.RLock()
	defer mutex.RUnlock()

	v, ok := storage[k]
	return v, ok
}

func Set(k Key, v Value) {
	mutex.Lock()
	defer mutex.Unlock()

	storage[k] = v
}

func Length() uint64 {
	mutex.RLock()
	defer mutex.RUnlock()

	return uint64(len(storage))
}

func Clear() {
	mutex.Lock()
	defer mutex.Unlock()

	storage = map[Key]Value{}
}
