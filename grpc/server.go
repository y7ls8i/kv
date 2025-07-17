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
	v, ok := kv.Get(inp.GetName(), inp.GetKey())
	if !ok {
		return &proto.GetResponse{Ok: false}, nil
	}

	return &proto.GetResponse{Value: v, Ok: ok}, nil
}

func (s kvServer) Set(_ context.Context, inp *proto.SetInput) (*emptypb.Empty, error) {
	kv.Set(inp.GetName(), inp.GetKey(), inp.GetValue())
	return nil, nil
}

func (s kvServer) Delete(_ context.Context, inp *proto.KeyInput) (*emptypb.Empty, error) {
	kv.Delete(inp.GetName(), inp.GetKey())
	return nil, nil
}

func (s kvServer) Length(_ context.Context, inp *proto.NameInput) (*proto.LengthResponse, error) {
	return &proto.LengthResponse{Length: kv.Length(inp.GetName())}, nil
}

func (s kvServer) Clear(_ context.Context, inp *proto.NameInput) (*emptypb.Empty, error) {
	kv.Clear(inp.GetName())
	return nil, nil
}

func (s kvServer) Subscribe(inp *proto.KeyInput, stream grpc.ServerStreamingServer[proto.Change]) error {
	subID, ch := kv.Subscribe(inp.GetName(), inp.GetKey())
	log.Printf("Subscribed name=%q key=%q id=%q", inp.GetName(), inp.GetKey(), subID)

	defer func(name, key, subID string) {
		kv.Unsubscribe(inp.GetName(), subID)
		log.Printf("Unsubscribed name=%q key=%q id=%q", name, key, subID)
	}(inp.GetName(), inp.GetKey(), subID)

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
