package grpc_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	kvgrpc "github.com/y7ls8i/kv/grpc"
	"github.com/y7ls8i/kv/grpc/proto"
	"github.com/y7ls8i/kv/kv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

// setupTestServer creates an in-memory gRPC server for testing
func setupTestServer(t *testing.T) (*grpc.Server, *bufconn.Listener, func()) {
	lis = bufconn.Listen(bufSize)
	grpcServer := kvgrpc.NewServer()

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Errorf("Server exited with error: %v", err)
		}
	}()

	return grpcServer, lis, func() {
		grpcServer.GracefulStop()
	}
}

// dialer creates a client connection to the in-memory server
func dialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	_, lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer(lis)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear()

	t.Run("Get non-existent key", func(t *testing.T) {
		resp, err := client.Get(ctx, &proto.GetInput{Key: "nonexistent"})
		assert.NoError(t, err)
		assert.False(t, resp.Ok, "Get should return false for non-existent key")
		assert.Nil(t, resp.Value, "Value should be nil for non-existent key")
	})

	t.Run("Get existing key", func(t *testing.T) {
		kv.Set("key1", []byte("value1"))
		resp, err := client.Get(ctx, &proto.GetInput{Key: "key1"})
		assert.NoError(t, err)
		assert.True(t, resp.Ok, "Get should return true for existing key")
		assert.Equal(t, []byte("value1"), resp.Value, "Value doesn't match")
	})
}

func TestSet(t *testing.T) {
	ctx := context.Background()
	_, lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer(lis)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear()

	// Test case: Set a value
	_, err = client.Set(ctx, &proto.SetInput{Key: "key1", Value: []byte("value1")})
	assert.NoError(t, err)

	// Verify the value was set
	v, ok := kv.Get("key1")
	assert.True(t, ok, "Set failed to store value")
	assert.Equal(t, []byte("value1"), v, "Value doesn't match")
}

func TestLength(t *testing.T) {
	ctx := context.Background()
	_, lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer(lis)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear()

	t.Run("Empty storage", func(t *testing.T) {
		resp, err := client.Length(ctx, &emptypb.Empty{})
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), resp.Length, "Length should be 0 for empty storage")
	})

	t.Run("Non-empty storage", func(t *testing.T) {
		kv.Set("key1", []byte("value1"))
		kv.Set("key2", []byte("value2"))
		resp, err := client.Length(ctx, &emptypb.Empty{})
		assert.NoError(t, err)
		assert.Equal(t, uint64(2), resp.Length, "Length should be 2")
	})
}

func TestClear(t *testing.T) {
	ctx := context.Background()
	_, lis, cleanup := setupTestServer(t)
	defer cleanup()

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer(lis)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := proto.NewKVClient(conn)
	kv.Clear()

	// Add some data
	kv.Set("key1", []byte("value1"))
	kv.Set("key2", []byte("value2"))

	// Verify data exists
	resp, err := client.Length(ctx, &emptypb.Empty{})
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), resp.Length, "Setup failed: length should be 2")

	// Clear and verify
	_, err = client.Clear(ctx, &emptypb.Empty{})
	assert.NoError(t, err)
	resp, err = client.Length(ctx, &emptypb.Empty{})
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), resp.Length, "Clear failed: length should be 0")
}
