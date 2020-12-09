package kvstore

import (
	"errors"
	"sync"

	"github.com/sidecus/raft/pkg/raft"
)

var errorInvalidPeers = errors.New("peers should contains at least 1 peer to enable a 2 node test cluster")
var errorCurrentNodeInPeers = errors.New("current nodeID should not be part of peers")
var errorInvalidPeerNodeID = errors.New("node in peers has invalid node IDs")

// StartRaftKVStore starts the raft kv store and waits for it to finish
// nodeID: id for current node
// port: port for current node
// peers: info for all other nodes
func StartRaftKVStore(nodeID int, port string, peers map[int]raft.NodeInfo) error {
	size := len(peers) + 1
	if size == 1 {
		return errorInvalidPeers
	}

	if err := validateNodeIDs(nodeID, peers); err != nil {
		return err
	}

	// create components
	kvStore := NewKVStore()
	node := raft.NewNode(nodeID, peers, kvStore, KVPeerClientFactory)
	rpcServer := NewServer(node)

	// start rpc server
	var wg sync.WaitGroup
	wg.Add(2)
	rpcServer.Start(port)
	node.Start()
	wg.Wait()

	return nil
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
