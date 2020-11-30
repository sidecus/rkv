package rkv

import (
	"fmt"
	"sync"

	"github.com/sidecus/raft/pkg/kvstore"
	"github.com/sidecus/raft/pkg/raft"
)

// StartRKV starts the raft kv store and waits for it to finish
// nodeID: id for current node
// port: port for current node
// peers: info for all other nodes
func StartRKV(nodeID int, port string, peers []raft.PeerInfo) error {
	size := len(peers) + 1
	if size == 1 {
		return fmt.Errorf("Invalid peers, it should be at least 1 to enable a 2 node cluster")
	}

	var nodeIDs []int
	nodeIDs = append(nodeIDs, nodeID)
	for _, v := range peers {
		nodeIDs = append(nodeIDs, v.NodeID)
	}

	// create needed components
	kvStore := kvstore.NewKVStore()
	proxy := kvstore.NewPeerProxy(peers)
	node := raft.NewNode(nodeID, nodeIDs, kvStore, proxy)
	rpcServer := kvstore.NewRPCServer(node)

	// start rpc server
	var wg sync.WaitGroup
	wg.Add(2)
	rpcServer.Start(port)
	node.Start()
	wg.Wait()

	return nil
}
