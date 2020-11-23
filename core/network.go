package core

import (
	"errors"
)

// IRaftNetwork interfaces defines the interface for the underlying communication among nodes
type IRaftNetwork interface {
	Send(sourceNodeID int, targetNodID int, msg *RaftMessage) error
	Broadcast(sourceNodeID int, msg *RaftMessage) error
	GetRecvChannel(nodeID int) (chan *RaftMessage, error)
}

// ErrorInvalidNodeCount - negative node count
var ErrorInvalidNodeCount = errors.New("node count is invalid, must be greater than 0")

// ErrorInvalidNodeID node id doesn't exist
var ErrorInvalidNodeID = errors.New("node id doesn't exist in the network")

// ErrorSendToSelf cannot send message to the sender itself
var ErrorSendToSelf = errors.New("sending message to self is not allowed")

// ErrorInvalidMessage RaftMesage obj is nil
var ErrorInvalidMessage = errors.New("message is nil")
