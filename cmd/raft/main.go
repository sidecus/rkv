package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/sidecus/raft/internal/net/dummynet"
	"github.com/sidecus/raft/internal/net/grpcnet"
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
		runLocal()
	case rpcMode:
		rpcCmd := flag.NewFlagSet("rpc", flag.ExitOnError)
		nodeID := rpcCmd.Int("nodeid", 0, "current node ID. 0 to n where n is total nodes")
		addresses := rpcCmd.String("addresses", "", "comma separated node addresses, ordered by nodeID")

		rpcCmd.Parse(os.Args[2:])
		addressArray := strings.Split(*addresses, ",")

		if *nodeID == -1 || len(addressArray) < 3 || *nodeID >= len(addressArray) {
			printUsage()
			os.Exit(1)
		}
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

func runLocal() {
	net, _ := dummynet.CreateDummyNetwork(3)

	// Start network
	net.Start()

	nodes := make([]raft.INode, 3)
	for i := range nodes {
		nodes[i] = raft.CreateNode(i, 3, net, logger)
	}

	// Create nodes and start them
	var wg sync.WaitGroup
	wg.Add(len(nodes))
	for i := range nodes {
		go nodes[i].Start(&wg)
	}

	wg.Wait()
}

func runRPC(nodeID int, addresses []string) {
	endpoints := make([]grpcnet.NodeEndpoint, len(addresses))
	for i, v := range addresses {
		endpoints[i].NodeID = i
		endpoints[i].Endpoint = v
	}

	// Create undlying grpc based network
	net, err := grpcnet.NewGRPCNetwork(nodeID, endpoints, logger)
	if err != nil {
		panic(err.Error())
	}

	// Start network
	net.Start()

	// Start current node
	node := raft.CreateNode(nodeID, len(endpoints), net, logger)
	node.Start(nil)
}
