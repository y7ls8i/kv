# KV

KV is a simple subscribable in-memory Key-Value server.

The **key** is a **string**.
The **value** is any **bytes** data. The serialization and deserialization of the value is the responsibility of the client.

It has 3 interfaces: gRPC, HTTP, GraphQL each has its own port number.
By default, gRPC listens to port `9901`, HTTP listens to port `9902`, GraphQL listens to port `9903`.

It is not meant as an internet-facing server, hence there is no authorization nor authentication. It is normally only accessible within the cluster network or within local machine.

## gRPC

See [grpc/proto/kv.proto](https://github.com/y7ls8i/kv/blob/main/grpc/proto/kv.proto) for details.

## HTTP

Values are base64 encoded.

Setting the value of `mykey1`:
```shell
curl -v http://localhost:9902/values/mykey1 -d "c29tZXRoaW5n"
```

Getting the value of `mykey1`:
```shell
curl -v http://localhost:9902/values/mykey1
```
```json
{"value":"c29tZXRoaW5n","ok":true}
```

Deleting the value of `mykey1`:
```shell
curl -v http://localhost:9902/values/mykey1 -X DELETE
```

Deleting all data:
```shell
curl -v http://localhost:9902/values/ -X DELETE
```

Getting number of keys in storage:
```shell
curl -v http://localhost:9902/length
```
```json
{"length":0}
```

Subscription for changes is supported using SSE (Server-Sent Events).
Subscribing for value changes for `mykey1`:
```shell
 curl -v  http://localhost:9902/subscribe/mykey1
```
```json
{"operation":"ADD","value":"c29tZXRoaW5n"}
{"operation":"UPDATE","value":"c29tZXRoaW5n"}
{"operation":"DELETE","value":""}
```

## GraphQL

Values are base64 encoded.

See [graphql/graph/schema.graphqls](https://github.com/y7ls8i/kv/blob/main/graphql/graph/schema.graphqls) for details.
