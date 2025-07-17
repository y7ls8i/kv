package grpc_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kvgrpc "github.com/y7ls8i/kv/grpc"
	"github.com/y7ls8i/kv/grpc/proto"
	"github.com/y7ls8i/kv/kv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const name = "mykv"

// setupTestServer creates an in-memory gRPC server for testing
func setupTestServer(t *testing.T) (net.Listener, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := kvgrpc.NewServer()

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Errorf("Server exited with error: %v", err)
		}
	}()

	return lis, func() {
		grpcServer.GracefulStop()
		_ = lis.Close()
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial grpc server: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear(name)

	t.Run("Get non-existent key", func(t *testing.T) {
		resp, err := client.Get(ctx, &proto.KeyInput{Name: name, Key: "nonexistent"})
		assert.NoError(t, err)
		assert.False(t, resp.Ok, "Get should return false for non-existent key")
		assert.Nil(t, resp.Value, "Value should be nil for non-existent key")
	})

	t.Run("Get existing key", func(t *testing.T) {
		kv.Set(name, "key1", []byte("value1"))
		resp, err := client.Get(ctx, &proto.KeyInput{Name: name, Key: "key1"})
		assert.NoError(t, err)
		assert.True(t, resp.Ok, "Get should return true for existing key")
		assert.Equal(t, []byte("value1"), resp.Value, "Value doesn't match")
	})
}

func TestSet(t *testing.T) {
	ctx := context.Background()
	lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial grpc server: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear(name)

	// Test case: Set a value
	_, err = client.Set(ctx, &proto.SetInput{Name: name, Key: "key1", Value: []byte("value1")})
	assert.NoError(t, err)

	// Verify the value was set
	v, ok := kv.Get(name, "key1")
	assert.True(t, ok, "Set failed to store value")
	assert.Equal(t, []byte("value1"), v, "Value doesn't match")
}

func TestLength(t *testing.T) {
	ctx := context.Background()
	lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial grpc server: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear(name)

	t.Run("Empty storage", func(t *testing.T) {
		resp, err := client.Length(ctx, &proto.NameInput{Name: name})
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), resp.Length, "Length should be 0 for empty storage")
	})

	t.Run("Non-empty storage", func(t *testing.T) {
		kv.Set(name, "key1", []byte("value1"))
		kv.Set(name, "key2", []byte("value2"))
		resp, err := client.Length(ctx, &proto.NameInput{Name: name})
		assert.NoError(t, err)
		assert.Equal(t, uint64(2), resp.Length, "Length should be 2")
	})
}

func TestClear(t *testing.T) {
	ctx := context.Background()
	lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial grpc server: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear(name)

	// Add some data
	kv.Set(name, "key1", []byte("value1"))
	kv.Set(name, "key2", []byte("value2"))
	kv.Set(name, "key3", []byte("value3"))

	// Verify data exists
	resp, err := client.Length(ctx, &proto.NameInput{Name: name})
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), resp.Length, "Setup failed: length should be 3")

	// Delete and verify
	_, err = client.Delete(ctx, &proto.KeyInput{Name: name, Key: "key1"})
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), kv.Length(name), "Clear failed: length should be 2")
	value, ok := kv.Get(name, "key1")
	assert.False(t, ok, "Get should return false for deleted key")
	assert.Nil(t, value, "Value should be nil for deleted key")

	// Clear and verify
	_, err = client.Clear(ctx, &proto.NameInput{Name: name})
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), kv.Length(name), "Clear failed: length should be 0")
}

func TestSubscribe(t *testing.T) {
	lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial grpc server: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear(name)

	key := fmt.Sprintf("TestSubscribe%d", time.Now().UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.Subscribe(ctx, &proto.KeyInput{Name: name, Key: key})
	assert.NoError(t, err)

	go func() {
		i := 0
		for {
			resp, err := stream.Recv()
			require.NoError(t, err, "Could not receive stream")

			switch i {
			case 0:
				assert.Equal(t, proto.Operation_ADD, resp.GetOperation(), "Expected add operation to be received")
				assert.Equal(t, []byte("value1"), resp.GetValue(), "Expected value1 to be received")
			case 1:
				assert.Equal(t, proto.Operation_UPDATE, resp.GetOperation(), "Expected update operation to be received")
				assert.Equal(t, []byte("value2"), resp.GetValue(), "Expected value2 to be received")
			case 2:
				assert.Equal(t, proto.Operation_DELETE, resp.GetOperation(), "Expected delete operation to be received")
				assert.Nil(t, resp.GetValue(), "Expected nil value to be received")

				cancel()
				return
			}

			i++
		}
	}()

	time.Sleep(time.Millisecond) // make sure the subscription happens first before continuing test.

	kv.Set(name, key, []byte("value1"))
	kv.Set(name, key, []byte("value2"))
	kv.Delete(name, key)

	<-ctx.Done()
}
