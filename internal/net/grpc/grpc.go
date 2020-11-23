package grpc

import (
	"github.com/sidecus/raft/pkg/network"
)

// grpcNetwork is a grpc based INetwork implementation
type grpcNetwork struct {
	size  int
	cin   chan *network.Request
	couts []chan *network.Message
}
