package channelnet

import (
	"github.com/sidecus/raft/pkg/network"
)

// channelNetwork is a channel based network implementation without real RPC calls
type channelNetwork struct {
	size  int
	cin   chan *network.Request
	couts []chan *network.Message
}

// CreateChannelNetwork creates a channelNetwork (local machine channel based network)
// and starts it. It mimics real network behavior by retrieving requests from cin and dispatch to couts
func CreateChannelNetwork(n int) (network.INetwork, error) {
	if n <= 0 || n > 1024 {
		return nil, network.ErrorInvalidNodeCount
	}

	cin := make(chan *network.Request, 100)
	couts := make([]chan *network.Message, n)
	for i := range couts {
		// non buffered channel to mimic unrealiable network
		couts[i] = make(chan *network.Message)
	}

	nw := &channelNetwork{
		size:  n,
		cin:   cin,
		couts: couts,
	}

	// start the network, which reads from cin and dispatches to one or more couts
	go func() {
		for r := range nw.cin {
			if r.Receiver != network.BoradcastAddress {
				nw.sendToNode(r.Receiver, r.Message)
			} else {
				// broadcast to others
				for i := range nw.couts {
					if i != r.Sender {
						nw.sendToNode(i, r.Message)
					}
				}
			}
		}
	}()

	return nw, nil
}

// sendToNode sends one message to one receiver
func (cn *channelNetwork) sendToNode(receiver int, msg *network.Message) {
	// nonblocking lossy sending using channel
	ch := cn.couts[receiver]
	select {
	case ch <- msg:
	default:
	}
}

// Send sends a message from the source node to target node
func (cn *channelNetwork) Send(sourceNodeID int, targetNodeID int, msg *network.Message) error {
	switch {
	case sourceNodeID > cn.size:
		return network.ErrorInvalidNodeID
	case targetNodeID > cn.size:
		return network.ErrorInvalidNodeID
	case sourceNodeID == targetNodeID:
		return network.ErrorSendToSelf
	case msg == nil:
		return network.ErrorInvalidMessage
	}

	req := &network.Request{
		Sender:   sourceNodeID,
		Receiver: targetNodeID,
		Message:  msg,
	}

	cn.cin <- req

	return nil
}

// Broadcast a message to all other nodes
func (cn *channelNetwork) Broadcast(sourceNodeID int, msg *network.Message) error {
	switch {
	case sourceNodeID > cn.size:
		return network.ErrorInvalidNodeID
	case msg == nil:
		return network.ErrorInvalidMessage
	}

	req := &network.Request{
		Sender:   sourceNodeID,
		Receiver: network.BoradcastAddress,
		Message:  msg,
	}

	cn.cin <- req

	return nil
}

// GetRecvChannel returns the receiving channel for the given node
func (cn *channelNetwork) GetRecvChannel(nodeID int) (chan *network.Message, error) {
	if nodeID > cn.size {
		return nil, network.ErrorInvalidNodeID
	}

	return cn.couts[nodeID], nil
}
