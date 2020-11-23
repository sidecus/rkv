package channelnet

import (
	"github.com/sidecus/raft/pkg/raft"
)

// boradcastAddress is a special NodeId representing broadcasting to all other nodes
const boradcastAddress = -1

// channelNetworkReq request type used by channelNetworkReq internally to send Message to nodes
type channelNetworkReq struct {
	sender   int
	receiver int
	message  *raft.Message
}

// channelNetwork is a channel based network implementation without real RPC calls
type channelNetwork struct {
	size  int
	cin   chan channelNetworkReq
	couts []chan *raft.Message
}

// CreateChannelNetwork creates a channelNetwork (local machine channel based network)
// and starts it. It mimics real network behavior by retrieving requests from cin and dispatch to couts
func CreateChannelNetwork(n int) (raft.INetwork, error) {
	if n <= 0 || n > 1024 {
		return nil, raft.ErrorInvalidNodeCount
	}

	cin := make(chan channelNetworkReq, 100)
	couts := make([]chan *raft.Message, n)
	for i := range couts {
		// non buffered channel to mimic unrealiable network
		couts[i] = make(chan *raft.Message)
	}

	net := &channelNetwork{
		size:  n,
		cin:   cin,
		couts: couts,
	}

	// start the network, which reads from cin and dispatches to one or more couts
	go func() {
		for r := range net.cin {
			if r.receiver != boradcastAddress {
				net.sendToNode(r.receiver, r.message)
			} else {
				// broadcast to others
				for i := range net.couts {
					if i != r.sender {
						net.sendToNode(i, r.message)
					}
				}
			}
		}
	}()

	return net, nil
}

// sendToNode sends one message to one receiver
func (net *channelNetwork) sendToNode(receiver int, msg *raft.Message) {
	// nonblocking lossy sending using channel
	ch := net.couts[receiver]
	select {
	case ch <- msg:
	default:
	}
}

// Send sends a message from the source node to target node
func (net *channelNetwork) Send(sourceNodeID int, targetNodeID int, msg *raft.Message) error {
	switch {
	case sourceNodeID > net.size:
		return raft.ErrorInvalidNodeID
	case targetNodeID > net.size:
		return raft.ErrorInvalidNodeID
	case sourceNodeID == targetNodeID:
		return raft.ErrorSendToSelf
	case msg == nil:
		return raft.ErrorInvalidMessage
	}

	req := channelNetworkReq{
		sender:   sourceNodeID,
		receiver: targetNodeID,
		message:  msg,
	}

	net.cin <- req

	return nil
}

// Broadcast a message to all other nodes
func (net *channelNetwork) Broadcast(sourceNodeID int, msg *raft.Message) error {
	switch {
	case sourceNodeID > net.size:
		return raft.ErrorInvalidNodeID
	case msg == nil:
		return raft.ErrorInvalidMessage
	}

	req := channelNetworkReq{
		sender:   sourceNodeID,
		receiver: boradcastAddress,
		message:  msg,
	}

	net.cin <- req

	return nil
}

// GetRecvChannel returns the receiving channel for the given node
func (net *channelNetwork) GetRecvChannel(nodeID int) (chan *raft.Message, error) {
	if nodeID > net.size {
		return nil, raft.ErrorInvalidNodeID
	}

	return net.couts[nodeID], nil
}
