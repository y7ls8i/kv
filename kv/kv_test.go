package kv_test // Changed package name

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/y7ls8i/kv/kv"
)

// ensure we start with a clean state for each test
func setup() {
	kv.Clear() // Call Clear from the imported kv package
}

func TestSetAndGet(t *testing.T) {
	setup()

	key1 := "testKey1"
	value1 := []byte("testValue1")
	kv.Set(key1, value1) // Call Set from the imported kv package

	val, ok := kv.Get(key1) // Call Get from the imported kv package
	assert.True(t, ok, "Expected key to be found")
	assert.Equal(t, value1, val, "Expected retrieved value to match set value")

	key2 := "testKey2"
	value2 := []byte("anotherValue")
	kv.Set(key2, value2)

	val, ok = kv.Get(key2)
	assert.True(t, ok, "Expected second key to be found")
	assert.Equal(t, value2, val, "Expected retrieved value for second key to match")

	// Test getting a non-existent key
	_, ok = kv.Get("nonExistentKey")
	assert.False(t, ok, "Expected non-existent key not to be found")
}

func TestLength(t *testing.T) {
	setup()

	assert.Equal(t, uint64(0), kv.Length(), "Expected initial length to be 0") // Call Length from the imported kv package

	kv.Set("k1", []byte("v1"))
	assert.Equal(t, uint64(1), kv.Length(), "Expected length to be 1 after one Set")

	kv.Set("k2", []byte("v2"))
	assert.Equal(t, uint64(2), kv.Length(), "Expected length to be 2 after two Sets")

	// Setting an existing key should not increase length
	kv.Set("k1", []byte("updatedV1"))
	assert.Equal(t, uint64(2), kv.Length(), "Expected length to remain 2 after updating an existing key")
}

func TestClear(t *testing.T) {
	setup()

	kv.Set("k1", []byte("v1"))
	kv.Set("k2", []byte("v2"))
	assert.Equal(t, uint64(2), kv.Length(), "Expected length to be 2 before clearing")

	kv.Clear()
	assert.Equal(t, uint64(0), kv.Length(), "Expected length to be 0 after Clear")

	_, ok := kv.Get("k1")
	assert.False(t, ok, "Expected k1 not to be found after Clear")
	_, ok = kv.Get("k2")
	assert.False(t, ok, "Expected k2 not to be found after Clear")
}

func TestConcurrency(t *testing.T) {
	setup()

	numGoroutines := 100
	numOperationsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // For Set and Get operations

	// Concurrently Set operations
	for i := 0; i < numGoroutines; i++ {
		go func(g int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				key := kv.Key(fmt.Sprintf("key_%d_%d", g, j)) // Use kv.Key
				value := []byte(fmt.Sprintf("value_%d_%d", g, j))
				kv.Set(key, value)
			}
		}(i)
	}

	// Concurrently Get operations (after some sets have potentially happened)
	for i := 0; i < numGoroutines; i++ {
		go func(g int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				key := kv.Key(fmt.Sprintf("key_%d_%d", g, j))
				kv.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// After all concurrent operations, verify the final length and some values
	expectedLength := uint64(numGoroutines * numOperationsPerGoroutine)
	assert.Equal(t, expectedLength, kv.Length(), "Expected final length to match total unique sets")

	// Verify a few specific keys/values
	for i := 0; i < 5; i++ { // Check a sample
		for j := 0; j < 5; j++ {
			key := kv.Key(fmt.Sprintf("key_%d_%d", i, j))
			value := []byte(fmt.Sprintf("value_%d_%d", i, j))
			retrievedVal, ok := kv.Get(key)
			assert.True(t, ok, "Expected concurrently set key to be found: %s", key)
			assert.Equal(t, value, retrievedVal, "Expected concurrently set value to match for key: %s", key)
		}
	}
}

func TestZeroValueKeyAndValue(t *testing.T) {
	setup()

	// Test setting with an empty key
	emptyKey := ""
	valForEmptyKey := []byte("valueForEmptyKey")
	kv.Set(emptyKey, valForEmptyKey)
	retrievedVal, ok := kv.Get(emptyKey)
	assert.True(t, ok, "Expected empty key to be found")
	assert.Equal(t, valForEmptyKey, retrievedVal, "Expected value for empty key to match")

	// Test setting with an empty value
	keyForEmptyValue := "keyForEmptyValue"
	emptyValue := []byte{}
	kv.Set(keyForEmptyValue, emptyValue)
	retrievedVal, ok = kv.Get(keyForEmptyValue)
	assert.True(t, ok, "Expected key for empty value to be found")
	assert.Equal(t, emptyValue, retrievedVal, "Expected empty value to match")

	// Test setting with nil value (Go's []byte handles nil and empty slice differently, but both are valid)
	keyForNilValue := "keyForNilValue"
	var nilValue []byte = nil
	kv.Set(keyForNilValue, nilValue)
	retrievedVal, ok = kv.Get(keyForNilValue)
	assert.True(t, ok, "Expected key for nil value to be found")
	assert.Nil(t, retrievedVal, "Expected nil value to be retrieved")
}

func TestSubscribe(t *testing.T) {
	setup()

	t.Run("Add, Update, Delete", func(t *testing.T) {
		key := fmt.Sprintf("TestSubscribe%d", time.Now().UnixNano())
		id, ch := kv.Subscribe(key)

		kv.Set(key, []byte("value1"))
		change1 := <-ch
		assert.Equal(t, kv.OperationAdd, change1.Op, "Expected add operation to be received")
		assert.Equal(t, []byte("value1"), change1.Value, "Expected value1 to be received")

		kv.Set(key, []byte("value2"))
		change2 := <-ch
		assert.Equal(t, kv.OperationUpdate, change2.Op, "Expected update operation to be received")
		assert.Equal(t, []byte("value2"), change2.Value, "Expected value2 to be received")

		kv.Delete(key)
		change3 := <-ch
		assert.Equal(t, kv.OperationDelete, change3.Op, "Expected delete operation to be received")
		assert.Nil(t, change3.Value, "Expected nil value to be received")

		kv.Unsubscribe(id)
		kv.Set(key, []byte("value1"))
		var received int
		for range ch {
			received++
		}
		assert.Equal(t, 0, received, "Expected 0 changes received after unsubscribed")
	})

	t.Run("Clear", func(t *testing.T) {
		key := fmt.Sprintf("TestSubscribe%d", time.Now().UnixNano())
		id, ch := kv.Subscribe(key)
		defer kv.Unsubscribe(id)

		kv.Set(key, []byte("value1"))
		change1 := <-ch
		assert.Equal(t, kv.OperationAdd, change1.Op, "Expected add operation to be received")
		assert.Equal(t, []byte("value1"), change1.Value, "Expected value1 to be received")

		kv.Clear()
		change2 := <-ch
		assert.Equal(t, kv.OperationDelete, change2.Op, "Expected delete operation to be received")
		assert.Nil(t, change2.Value, "Expected nil value to be received")
	})

	t.Run("Slow consumer", func(t *testing.T) {
		key := fmt.Sprintf("TestSubscribe%d", time.Now().UnixNano())
		id, ch := kv.Subscribe(key)
		defer kv.Unsubscribe(id)

		for i := uint64(0); i < kv.CHANGE_CHAN_CAP*2; i++ {
			kv.Set(key, []byte(fmt.Sprintf("value%d", i)))
		}

		var count atomic.Uint64
		go func() {
			for range ch {
				count.Add(1)
			}
		}()

		time.Sleep(time.Millisecond) // wait until receiving all the notifications
		kv.Unsubscribe(id)

		assert.Equal(t, kv.CHANGE_CHAN_CAP, count.Load(), "Expected to only receive %d notifications", kv.CHANGE_CHAN_CAP)
	})
}
