package rkv

import (
	"errors"
	"sync"

	"github.com/sidecus/raft/pkg/raft"
	"github.com/sidecus/raft/pkg/util"
)

var errorInsufficientPeers = errors.New("peers should contains at least 2 peers")
var errorCurrentNodeInPeers = errors.New("current nodeID should not be part of peers")
var errorInvalidPeerNodeID = errors.New("node in peers has invalid node IDs")

// StartRKV starts the raft kv store and waits for it to finish
// nodeID: id for current node
// port: port for current node
// peers: info for all other nodes
func StartRKV(nodeID int, port string, peers map[int]raft.NodeInfo) {
	size := len(peers) + 1
	if size < 3 {
		util.Fatalf("%s\n", errorInsufficientPeers)
	}

	if err := validateNodeIDs(nodeID, peers); err != nil {
		util.Fatalf("%s\n", errorInsufficientPeers)
	}

	var wg sync.WaitGroup

	// create components
	node := raft.NewNode(nodeID, peers, newRKVStore(), rkvProxyFactory)
	rpcServer := newRKVRPCServer(node, &wg)

	// start
	rpcServer.Start(port)
	node.Start()
	wg.Wait()
}

func validateNodeIDs(nodeID int, peers map[int]raft.NodeInfo) error {
	if _, found := peers[nodeID]; found {
		return errorCurrentNodeInPeers
	}

	for i, p := range peers {
		if p.NodeID != i {
			return errorInvalidPeerNodeID
		}
	}

	return nil
}
