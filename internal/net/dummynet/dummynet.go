package dummynet

import (
	"github.com/sidecus/raft/pkg/network"
)

// dummyNetwork is a channel based network implementation without real RPC calls
type dummyNetwork struct {
	size  int
	cin   chan *network.Request
	couts []chan *network.Message
}

// CreateDummyNetwork creates a channelNetwork (local machine channel based network)
// and starts it. It mimics real network behavior by retrieving requests from cin and dispatch to couts
func CreateDummyNetwork(n int) (network.INetwork, error) {
	if n <= 0 || n > 1024 {
		return nil, network.ErrorInvalidNodeCount
	}

	cin := make(chan *network.Request, 100)
	couts := make([]chan *network.Message, n)
	for i := range couts {
		// non buffered channel to mimic unrealiable network
		couts[i] = make(chan *network.Message)
	}

	nw := &dummyNetwork{
		size:  n,
		cin:   cin,
		couts: couts,
	}

	return nw, nil
}

// sendToNode sends one message to one receiver
func (dn *dummyNetwork) sendToNode(receiver int, msg *network.Message) {
	// nonblocking lossy sending using channel
	ch := dn.couts[receiver]
	select {
	case ch <- msg:
	default:
	}
}

// Start starts the dummy network on a separate goroutine, which reads from cin and dispatches to one or more couts
func (dn *dummyNetwork) Start() {
	go func() {
		for r := range dn.cin {
			if r.Receiver != network.BoradcastAddress {
				dn.sendToNode(r.Receiver, r.Message)
			} else {
				// broadcast to others
				for i := range dn.couts {
					if i != r.Sender {
						dn.sendToNode(i, r.Message)
					}
				}
			}
		}
	}()
}

// Send sends a message from the source node to target node
func (dn *dummyNetwork) Send(sourceNodeID int, targetNodeID int, msg *network.Message) error {
	switch {
	case sourceNodeID > dn.size:
		return network.ErrorInvalidNodeID
	case targetNodeID > dn.size:
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

	dn.cin <- req

	return nil
}

// Broadcast a message to all other nodes
func (dn *dummyNetwork) Broadcast(sourceNodeID int, msg *network.Message) error {
	switch {
	case sourceNodeID > dn.size:
		return network.ErrorInvalidNodeID
	case msg == nil:
		return network.ErrorInvalidMessage
	}

	req := &network.Request{
		Sender:   sourceNodeID,
		Receiver: network.BoradcastAddress,
		Message:  msg,
	}

	dn.cin <- req

	return nil
}

// GetRecvChannel returns the receiving channel for the given node
func (dn *dummyNetwork) GetRecvChannel(nodeID int) (chan *network.Message, error) {
	if nodeID > dn.size {
		return nil, network.ErrorInvalidNodeID
	}

	return dn.couts[nodeID], nil
}
