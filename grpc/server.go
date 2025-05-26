package grpc

import (
	"context"

	"github.com/y7ls8i/kv/grpc/proto"
	"github.com/y7ls8i/kv/kv"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type kvServer struct {
	proto.UnimplementedKVServer
}

func (s kvServer) Get(_ context.Context, inp *proto.GetInput) (*proto.GetResponse, error) {
	v, ok := kv.Get(inp.GetKey())
	if !ok {
		return &proto.GetResponse{Ok: false}, nil
	}

	return &proto.GetResponse{Value: v, Ok: ok}, nil
}

func (s kvServer) Set(_ context.Context, inp *proto.SetInput) (*emptypb.Empty, error) {
	kv.Set(inp.GetKey(), inp.GetValue())
	return nil, nil
}

func (s kvServer) Length(context.Context, *emptypb.Empty) (*proto.LengthResponse, error) {
	return &proto.LengthResponse{Length: kv.Length()}, nil
}

func (s kvServer) Clear(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	kv.Clear()
	return nil, nil
}

func NewServer() *grpc.Server {
	grpcServer := grpc.NewServer()
	proto.RegisterKVServer(grpcServer, &kvServer{})
	return grpcServer
}
