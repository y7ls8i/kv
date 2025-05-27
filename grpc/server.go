package grpc

import (
	"context"
	"log"

	"github.com/y7ls8i/kv/grpc/proto"
	"github.com/y7ls8i/kv/kv"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type kvServer struct {
	proto.UnimplementedKVServer
}

func (s kvServer) Get(_ context.Context, inp *proto.KeyInput) (*proto.GetResponse, error) {
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

func (s kvServer) Delete(_ context.Context, inp *proto.KeyInput) (*emptypb.Empty, error) {
	kv.Delete(inp.GetKey())
	return nil, nil
}

func (s kvServer) Length(context.Context, *emptypb.Empty) (*proto.LengthResponse, error) {
	return &proto.LengthResponse{Length: kv.Length()}, nil
}

func (s kvServer) Clear(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	kv.Clear()
	return nil, nil
}

func (s kvServer) Subscribe(inp *proto.KeyInput, stream grpc.ServerStreamingServer[proto.Change]) error {
	subID, ch := kv.Subscribe(inp.GetKey())
	log.Printf("Subscribed key=%q id=%q", inp.GetKey(), subID)

	defer func(key, subID string) {
		kv.Unsubscribe(subID)
		log.Printf("Unsubscribed key=%q id=%q", key, subID)
	}(inp.GetKey(), subID)

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case change := <-ch:
			send := proto.Change{Value: change.Value}
			switch change.Op {
			case kv.OperationAdd:
				send.Operation = proto.Operation_ADD
			case kv.OperationUpdate:
				send.Operation = proto.Operation_UPDATE
			case kv.OperationDelete:
				send.Operation = proto.Operation_DELETE
			}
			if err := stream.Send(&send); err != nil {
				log.Printf("Could not send stream data, error: %v\n", err)
				return err
			}
		}
	}
}

func NewServer() *grpc.Server {
	grpcServer := grpc.NewServer()
	proto.RegisterKVServer(grpcServer, &kvServer{})
	return grpcServer
}
