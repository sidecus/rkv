package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
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
	if len(os.Args) <= 1 {
		printUsage()
		return
	}

	mode := os.Args[1]
	args := os.Args[2:]

	if strings.EqualFold(mode, localMode) {
		// local mode
		runLocal()
	} else if strings.EqualFold(mode, rpcMode) {
		// remote mode
		runRPC(args)
	} else {
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Two modes:")
	fmt.Println("1. raft local")
	fmt.Println("2. raft rpc <id> <node0url> <node1url> ...")
}

func runLocal() {
	net, _ := dummynet.CreateDummyNetwork(3)

	// Start network
	net.Start()

	nodes := make([]raft.INode, 3)
	for i := range nodes {
		nodes[i] = raft.CreateNode(i, 3, net, logger)
	}

	var wg sync.WaitGroup

	// Create nodes and start them
	for i := range nodes {
		wg.Add(1)
		go nodes[i].Start(&wg)
	}

	wg.Wait()
}

func runRPC(args []string) {
	if len(args) < 2 {
		panic("invalid rpc args. need id and at least 2 node urls")
	}

	nodeID, err := strconv.Atoi(args[0])
	if err != nil {
		panic(err.Error())
	}

	addresses := args[1:]
	endpoints := make([]grpcnet.NodeEndpoint, len(addresses))
	for i, v := range addresses {
		endpoints[i].NodeID = i
		endpoints[i].Endpoint = v
	}

	// Create undlying grpc based network
	net, err := grpcnet.CreateGRPCNetwork(nodeID, endpoints, logger)
	if err != nil {
		panic(err.Error())
	}

	// Start network
	net.Start()

	// Start current node
	node := raft.CreateNode(nodeID, len(endpoints), net, logger)
	node.Start(nil)
}
