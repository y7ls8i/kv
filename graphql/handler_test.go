package graphql_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kvgraphql "github.com/y7ls8i/kv/graphql"
	"github.com/y7ls8i/kv/graphql/graph/model"
	"github.com/y7ls8i/kv/kv"
)

// setupTestServer creates an in-memory gRPC server for testing
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	mux := kvgraphql.NewServeMux()

	server := httptest.NewServer(mux)

	return server, func() {
		server.Close()
	}
}

func postGraphQL(t *testing.T, server *httptest.Server, query string) *http.Response {
	t.Helper()

	payload := map[string]any{"query": query}
	j, err := json.Marshal(payload)
	require.NoError(t, err)

	httpResp, err := http.Post(fmt.Sprintf("%s/query", server.URL), "application/json", bytes.NewBuffer(j))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpResp.StatusCode)

	return httpResp
}

func TestGet(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	t.Run("Get non-existent key", func(t *testing.T) {
		httpResp := postGraphQL(t, server, fmt.Sprintf(
			`query {
			  get(key: %q) {
				ok
				value
			  }
			}`,
			"nonexistent",
		))

		defer func() { _ = httpResp.Body.Close() }()

		var resp struct {
			Data struct {
				GetResponse model.GetResponse `json:"get"`
			} `json:"data"`
		}
		err := json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.False(t, resp.Data.GetResponse.Ok, "Get should return false for non-existent key")
		assert.Equal(t, "", resp.Data.GetResponse.Value, "Value should be empty for non-existent key")
	})

	t.Run("Get existing key", func(t *testing.T) {
		kv.Set("key1", []byte("value1"))

		httpResp := postGraphQL(t, server, fmt.Sprintf(
			`query {
			  get(key: %q) {
				ok
				value
			  }
			}`,
			"key1",
		))

		defer func() { _ = httpResp.Body.Close() }()

		var resp struct {
			Data struct {
				GetResponse model.GetResponse `json:"get"`
			} `json:"data"`
		}
		err := json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.True(t, resp.Data.GetResponse.Ok, "Get should return true for existing key")
		decoded, err := base64.StdEncoding.DecodeString(resp.Data.GetResponse.Value)
		assert.NoError(t, err, "Value should be base64 encoded")
		assert.Equal(t, "value1", string(decoded), "Value doesn't match")
	})
}

func TestSet(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	_ = postGraphQL(t, server, fmt.Sprintf(
		`mutation {
		  set(input: { key: %q, value: %q })
		}`,
		"key1",
		base64.StdEncoding.EncodeToString([]byte("value1")),
	))

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
		httpResp := postGraphQL(t, server, `
			query {
			  length {
				length
			  }
			}
		`)

		defer func() { _ = httpResp.Body.Close() }()

		var resp struct {
			Data struct {
				LengthResponse model.LengthResponse `json:"length"`
			} `json:"data"`
		}
		err := json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.NoError(t, err)
		assert.Equal(t, int32(0), resp.Data.LengthResponse.Length, "Length should be 0 for empty storage")
	})

	t.Run("Non-empty storage", func(t *testing.T) {
		kv.Set("key1", []byte("value1"))
		kv.Set("key2", []byte("value2"))

		httpResp := postGraphQL(t, server, `
			query {
			  length {
				length
			  }
			}
		`)

		defer func() { _ = httpResp.Body.Close() }()

		var resp struct {
			Data struct {
				LengthResponse model.LengthResponse `json:"length"`
			} `json:"data"`
		}
		err := json.NewDecoder(httpResp.Body).Decode(&resp)
		assert.NoError(t, err)

		assert.NoError(t, err)
		assert.Equal(t, int32(2), resp.Data.LengthResponse.Length, "Length should be 2")
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
	assert.Equal(t, uint64(3), kv.Length(), "Length should be 3")

	// Delete
	_ = postGraphQL(t, server, fmt.Sprintf(
		`mutation {
		  delete(key: %q)
		}`,
		"key1",
	))

	// Verify data deleted
	assert.Equal(t, uint64(2), kv.Length(), "Delete failed: length should be 2")
	value, ok := kv.Get("key1")
	assert.False(t, ok, "Get should return false for deleted key")
	assert.Nil(t, value, "Value should be nil for deleted key")

	// Clear
	_ = postGraphQL(t, server, `mutation {
		  clear
		}`,
	)

	// Verify data cleared
	assert.Equal(t, uint64(0), kv.Length(), "Clear failed: length should be 0")
}

func TestSubscribe(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	kv.Clear()

	key := fmt.Sprintf("TestSubscribe%d", time.Now().UnixNano())

	// connect WS
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/query"
	reqHeader := http.Header{}
	reqHeader.Set("content-type", "application/json")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, reqHeader)
	require.NoError(t, err)

	// connection_init
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"connection_init","payload":{}}`)))
	_, m, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, `{"type":"connection_ack"}`, strings.TrimSpace(string(m)))

	// start subscription
	subscriptionQuery := fmt.Sprintf(`
		subscription {
		  subscribe(key:%q) {
			operation
			value
		  }
		}
	`, key)
	startMsg := map[string]interface{}{
		"type": "start",
		"id":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"payload": map[string]any{
			"query": subscriptionQuery,
		},
	}
	err = ws.WriteJSON(startMsg)
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		defer close(done)

		var msg struct {
			Type    string `json:"type"`
			Payload struct {
				Data struct {
					Subscribe model.Change `json:"subscribe"`
				} `json:"data"`
			} `json:"payload"`
		}

		i := 0

		for {
			err = ws.ReadJSON(&msg)
			require.NoError(t, err)

			if msg.Type != "data" {
				continue
			}

			change := msg.Payload.Data.Subscribe

			switch i {
			case 0:
				assert.Equal(t, model.OperationAdd, change.Operation, "Expected add operation to be received")
				decoded, err := base64.StdEncoding.DecodeString(change.Value)
				assert.NoError(t, err, "Value should be base64 encoded")
				assert.Equal(t, "value1", string(decoded), "Expected value1 to be received")
			case 1:
				assert.Equal(t, model.OperationUpdate, change.Operation, "Expected update operation to be received")
				decoded, err := base64.StdEncoding.DecodeString(change.Value)
				assert.NoError(t, err, "Value should be base64 encoded")
				assert.Equal(t, "value2", string(decoded), "Expected value2 to be received")
			case 2:
				assert.Equal(t, model.OperationDelete, change.Operation, "Expected delete operation to be received")
				assert.Equal(t, "", change.Value, "Expected empty value to be received")

				_ = ws.Close()
				return
			}

			i++
		}
	}()

	time.Sleep(time.Millisecond) // make sure the subscription happens first before continuing test.

	kv.Set(key, []byte("value1"))
	kv.Set(key, []byte("value2"))
	kv.Delete(key)

	<-done
}
