package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/sidecus/raft/internal/net/channelnet"
	"github.com/sidecus/raft/pkg/raft"
)

const (
	localMode = "local"
	rpcMode   = "rpc"
)

func main() {
	if len(os.Args) <= 1 {
		printUsage()
		return
	}

	mode := os.Args[1]
	urls := os.Args[2:]

	if strings.EqualFold(mode, localMode) {
		// local mode
		runLocal()
	} else if strings.EqualFold(mode, rpcMode) {
		// remote mode
		fmt.Println(urls)
	} else {
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Two modes:")
	fmt.Println("1. raft local")
	fmt.Println("2. raft rpc <peer1url> <peer2url> ...")
}

func runLocal() {
	rn, _ := channelnet.CreateChannelNetwork(3)
	logger := log.New(log.Writer(), log.Prefix(), log.Flags())

	nodes := make([]raft.INode, 3)
	for i := range nodes {
		nodes[i] = raft.CreateNode(i, 3, rn, logger)
	}

	var wg sync.WaitGroup

	// Create nodes and start them
	for i := range nodes {
		wg.Add(1)
		go nodes[i].Start(&wg)
	}

	wg.Wait()
}
