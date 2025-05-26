package main

import (
	"flag"
	"log"
	"net"

	"github.com/y7ls8i/kv/grpc"
)

func main() {
	grpcAddr := flag.String("grpc", "localhost:8080", "gRPC server listen address")
	flag.Parse()

	func() {
		log.Printf("Starting gRPC server on %q", *grpcAddr)

		lis, err := net.Listen("tcp", *grpcAddr)
		if err != nil {
			log.Fatalf("Could not listen to %q, error: %v", *grpcAddr, err)
		}

		grpcServer := grpc.NewServer()
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Could not start gRPC server, error: %v", err)
		}
	}()

	// TODO REST server

	select {}
}
