package http_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	kvhttp "github.com/y7ls8i/kv/http"
	"github.com/y7ls8i/kv/kv"
)

// setupTestServer creates an in-memory gRPC server for testing
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	mux := kvhttp.NewServeMux()

	server := httptest.NewServer(mux)

	return server, func() {
		server.Close()
	}
}

func TestGet(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	t.Run("Get non-existent key", func(t *testing.T) {
		httpResp, err := http.Get(fmt.Sprintf("%s/values/nonexistent", server.URL))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResp.StatusCode)

		var resp kvhttp.GetResponse
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.False(t, resp.OK, "Get should return false for non-existent key")
		assert.Equal(t, "", string(resp.Value), "Value should be empty for non-existent key")
	})

	t.Run("Get existing key", func(t *testing.T) {
		kv.Set("key1", []byte("value1"))

		httpResp, err := http.Get(fmt.Sprintf("%s/values/key1", server.URL))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResp.StatusCode)

		var resp kvhttp.GetResponse
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.True(t, resp.OK, "Get should return true for existing key")
		decoded, err := base64.StdEncoding.DecodeString(string(resp.Value))
		assert.NoError(t, err, "Value should be base64 encoded")
		assert.Equal(t, "value1", string(decoded), "Value doesn't match")
	})
}

func TestSet(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	httpResp, err := http.Post(
		fmt.Sprintf("%s/values/key1", server.URL),
		"text/plain",
		bytes.NewBuffer([]byte(base64.StdEncoding.EncodeToString([]byte("value1")))))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, httpResp.StatusCode)

	// Verify the value was set
	v, ok := kv.Get("key1")
	assert.True(t, ok, "Set failed to store value")
	assert.Equal(t, []byte("value1"), v, "Value doesn't match")
}

func TestLength(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	t.Run("Empty storage", func(t *testing.T) {
		httpResp, err := http.Get(fmt.Sprintf("%s/length", server.URL))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResp.StatusCode)

		var resp kvhttp.LengthResponse
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.NoError(t, err)
		assert.Equal(t, uint64(0), resp.Length, "Length should be 0 for empty storage")
	})

	t.Run("Non-empty storage", func(t *testing.T) {
		kv.Set("key1", []byte("value1"))
		kv.Set("key2", []byte("value2"))

		httpResp, err := http.Get(fmt.Sprintf("%s/length", server.URL))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResp.StatusCode)

		var resp kvhttp.LengthResponse
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.NoError(t, err)
		assert.Equal(t, uint64(2), resp.Length, "Length should be 2")
	})
}

func TestDelete(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	// Add some data
	kv.Set("key1", []byte("value1"))
	kv.Set("key2", []byte("value2"))
	kv.Set("key3", []byte("value3"))
	assert.Equal(t, uint64(3), kv.Length(), "Setup failed: length should be 3")

	// Delete
	httpReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/values/key1", server.URL), http.NoBody)
	assert.NoError(t, err)
	httpResp, err := http.DefaultClient.Do(httpReq)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, httpResp.StatusCode)

	// Verify data deleted
	assert.Equal(t, uint64(2), kv.Length(), "Delete failed: length should be 2")
	value, ok := kv.Get("key1")
	assert.False(t, ok, "Get should return false for deleted key")
	assert.Nil(t, value, "Value should be nil for deleted key")

	// Clear
	httpReq, err = http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/values/", server.URL), http.NoBody)
	assert.NoError(t, err)
	httpResp, err = http.DefaultClient.Do(httpReq)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, httpResp.StatusCode)

	// Verify data cleared
	assert.Equal(t, uint64(0), kv.Length(), "Clear failed: length should be 0")
}

func TestSubscribe(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	key := fmt.Sprintf("TestSubscribe%d", time.Now().UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(time.Millisecond) // make sure the subscription happens first before continuing test.

		kv.Set(key, []byte("value1"))
		kv.Set(key, []byte("value2"))
		kv.Delete(key)

		time.Sleep(time.Millisecond) // make sure handlers work before continuing test.

		cancel()
	}()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/subscribe/%s", server.URL, key), nil)
	assert.NoError(t, err)
	httpResp, err := http.DefaultClient.Do(httpReq)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)
	assert.Equal(t, "text/event-stream", httpResp.Header.Get("Content-Type"))

	reader := bufio.NewReader(httpResp.Body)

	i := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && strings.Contains(err.Error(), "canceled") {
			break
		} else {
			assert.NoError(t, err, "Could not receive stream")
		}

		var resp kvhttp.SubscribeResponse
		err = json.Unmarshal(line, &resp)
		assert.NoError(t, err, "Could not unmarshal stream data")

		switch i {
		case 0:
			assert.Equal(t, kvhttp.OperationAdd, resp.Operation, "Expected add operation to be received")
			decoded, err := base64.StdEncoding.DecodeString(string(resp.Value))
			assert.NoError(t, err, "Value should be base64 encoded")
			assert.Equal(t, "value1", string(decoded), "Expected value1 to be received")
		case 1:
			assert.Equal(t, kvhttp.OperationUpdate, resp.Operation, "Expected update operation to be received")
			decoded, err := base64.StdEncoding.DecodeString(string(resp.Value))
			assert.NoError(t, err, "Value should be base64 encoded")
			assert.Equal(t, "value2", string(decoded), "Expected value2 to be received")
		case 2:
			assert.Equal(t, kvhttp.OperationUDelete, resp.Operation, "Expected delete operation to be received")
			assert.Equal(t, "", string(resp.Value), "Expected empty value to be received")
		}

		i++
	}
}
