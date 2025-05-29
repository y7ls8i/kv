package main

import (
	"flag"
	"log"
	"net"
	"net/http"

	kvgraphql "github.com/y7ls8i/kv/graphql"
	kvgrpc "github.com/y7ls8i/kv/grpc"
	kvhttp "github.com/y7ls8i/kv/http"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	grpcAddr := flag.String("grpc", "localhost:9901", "gRPC server listen address")
	httpAddr := flag.String("http", "localhost:9902", "HTTP server listen address")
	graphqlAddr := flag.String("graphql", "localhost:9903", "GraphQL server listen address")
	flag.Parse()

	go func() {
		log.Printf("Starting gRPC server on %q", *grpcAddr)

		grpcServer := kvgrpc.NewServer()

		lis, err := net.Listen("tcp", *grpcAddr)
		if err != nil {
			log.Fatalf("Could not listen to %q, error: %v", *grpcAddr, err)
		}

		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Could not start gRPC server, error: %v", err)
		}
	}()

	go func() {
		log.Printf("Starting H2C server on %q", *httpAddr)

		mux := kvhttp.NewServeMux()

		h2cServer := &http2.Server{}
		handler := h2c.NewHandler(mux, h2cServer)
		server := &http.Server{
			Addr:    *httpAddr,
			Handler: handler,
		}
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Could not start H2C server, error: %v", err)
		}
	}()

	go func() {
		log.Printf("Starting GraphQL server on %q", *graphqlAddr)

		mux := kvgraphql.NewServeMux()

		h2cServer := &http2.Server{}
		handler := h2c.NewHandler(mux, h2cServer)
		server := &http.Server{
			Addr:    *graphqlAddr,
			Handler: handler,
		}
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Could not start GraphQL server, error: %v", err)
		}
	}()

	select {}
}
