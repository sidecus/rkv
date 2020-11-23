package channelnet

import (
	"github.com/sidecus/raft/internal/net"
)

// boradcastAddress is a special NodeId representing broadcasting to all other nodes
const boradcastAddress = -1

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

// CreateChannelNetwork creates a channelNetwork (local machine channel based network)
// and starts it. It mimics real network behavior by retrieving requests from cin and dispatch to couts
func CreateChannelNetwork(n int) (net.INetwork, error) {
	if n <= 0 || n > 1024 {
		return nil, net.ErrorInvalidNodeCount
	}

	cin := make(chan channelNetworkReq, 100)
	couts := make([]chan *net.Message, n)
	for i := range couts {
		// non buffered channel to mimic unrealiable network
		couts[i] = make(chan *net.Message)
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
func (cn *channelNetwork) sendToNode(receiver int, msg *net.Message) {
	// nonblocking lossy sending using channel
	ch := cn.couts[receiver]
	select {
	case ch <- msg:
	default:
	}
}

// Send sends a message from the source node to target node
func (cn *channelNetwork) Send(sourceNodeID int, targetNodeID int, msg *net.Message) error {
	switch {
	case sourceNodeID > cn.size:
		return net.ErrorInvalidNodeID
	case targetNodeID > cn.size:
		return net.ErrorInvalidNodeID
	case sourceNodeID == targetNodeID:
		return net.ErrorSendToSelf
	case msg == nil:
		return net.ErrorInvalidMessage
	}

	req := channelNetworkReq{
		sender:   sourceNodeID,
		receiver: targetNodeID,
		message:  msg,
	}

	cn.cin <- req

	return nil
}

// Broadcast a message to all other nodes
func (cn *channelNetwork) Broadcast(sourceNodeID int, msg *net.Message) error {
	switch {
	case sourceNodeID > cn.size:
		return net.ErrorInvalidNodeID
	case msg == nil:
		return net.ErrorInvalidMessage
	}

	req := channelNetworkReq{
		sender:   sourceNodeID,
		receiver: boradcastAddress,
		message:  msg,
	}

	cn.cin <- req

	return nil
}

// GetRecvChannel returns the receiving channel for the given node
func (cn *channelNetwork) GetRecvChannel(nodeID int) (chan *net.Message, error) {
	if nodeID > cn.size {
		return nil, net.ErrorInvalidNodeID
	}

	return cn.couts[nodeID], nil
}
