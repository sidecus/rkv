package grpc

import (
	"github.com/sidecus/raft/internal/net"
)

// broadcastAddress is a special NodeId representing broadcasting to all other nodes
const broadcastAddress = -1

// channelNetworkReq request type used by channelNetworkReq internally to send Message to nodes
type channelNetworkReq struct {
	sender   int
	receiver int
	message  *net.Message
}

// channelNetwork is a channel based network implementation without real RPC calls
type channelNetwork struct {
	size  int
	cin   chan channelNetworkReq
	couts []chan *net.Message
}
