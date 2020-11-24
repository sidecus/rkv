# Install protobuf
```bash
export GO111MODULE=on  # Enable module mode
go get google.golang.org/protobuf/cmd/protoc-gen-go google.golang.org/grpc/cmd/protoc-gen-go-grpc
go get google.golang.org/grpc/cmd/protoc-gen-go-grpc
```

# Generate proto file and grpc file
```bash
protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative pb/raft.proto
```