package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/sidecus/raft/pkg/kvstore"
	"github.com/sidecus/raft/pkg/kvstore/rpc"
	"github.com/sidecus/raft/pkg/raft"
)

const (
	localMode = "local"
	rpcMode   = "rpc"
)

var logger = log.New(log.Writer(), log.Prefix(), log.Flags())

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case localMode:
		rpcCmd := flag.NewFlagSet("local", flag.ExitOnError)
		rpcCmd.Parse(os.Args[2:])
		//runLocal()
	case rpcMode:
		rpcCmd := flag.NewFlagSet("rpc", flag.ExitOnError)
		nodeID := rpcCmd.Int("nodeid", 0, "current node ID. 0 to n where n is total nodes")
		addresses := rpcCmd.String("addresses", "", "comma separated node addresses, ordered by nodeID")

		rpcCmd.Parse(os.Args[2:])
		addressArray := strings.Split(*addresses, ",")

		runRPC(*nodeID, addressArray)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("1. Local mode - Runs on a channel based dummy network locally")
	fmt.Println("   raft local")
	fmt.Println("2. gRPC mode - Use this to run the raft cluster across machines/processes/pods")
	fmt.Println("   raft rpc -nodeid id -addresses node0address,node1address,node2addresses...")
}

func runRPC(nodeID int, addresses []string) {
	port := getNodePort(nodeID, addresses)
	var peers []raft.PeerInfo
	for i, v := range addresses {
		if i == nodeID {
			continue
		}

		peers = append(peers, raft.PeerInfo{
			NodeID:   i,
			Endpoint: v,
		})
	}
	kvStore := kvstore.NewKVStore()
	proxy := rpc.NewKVStorePeerProxy(peers)
	node := raft.NewNode(nodeID, len(addresses), kvStore, proxy)
	rpcServer := rpc.NewKVStoreRPCServer(node)

	var wg sync.WaitGroup
	wg.Add(2)
	rpcServer.Start(port)
	node.Start()
	wg.Wait()
}

func getNodePort(nodeID int, addresses []string) string {
	var address string
	for i, v := range addresses {
		if i == nodeID {
			address = v
		}
	}

	parts := strings.SplitN(address, ":", 2)
	if len(parts) < 2 {
		panic("Cannot get port for current node")
	}

	port := parts[1]
	_, err := strconv.Atoi(port)
	if err != nil {
		panic("Invalid port. not a number")
	}

	return port
}
