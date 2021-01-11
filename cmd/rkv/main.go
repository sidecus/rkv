package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/sidecus/raft/pkg/raft"
	"github.com/sidecus/raft/pkg/rkv"
	"github.com/sidecus/raft/pkg/util"
)

var logger = log.New(log.Writer(), log.Prefix(), log.Flags())

func main() {
	nodeID := -1
	addresses := ""
	logLevel := 3

	flag.IntVar(&nodeID, "nodeid", -1, "current node ID. 0 to n where n is total nodes")
	flag.StringVar(&addresses, "addresses", "", "comma separated node addresses, ordered by nodeID")
	flag.IntVar(&logLevel, "loglevel", 3, "log level. 1 - error, 2 - warning, 3 - info, 4 - traces, default 3")
	flag.Parse()

	addrArray := strings.Split(addresses, ",")
	if nodeID < 0 || nodeID >= len(addrArray) {
		fmt.Println("nodeID is out of range for addresses")
		printUsage()
		os.Exit(1)
	}

	port, err := getNodePort(nodeID, addrArray)
	if err != nil {
		fmt.Println(err)
		printUsage()
		os.Exit(1)
	}

	util.SetLogLevel(logLevel)

	runRPC(nodeID, port, addrArray)
}

func printUsage() {
	fmt.Println("rkv -nodeid id -addresses node0address:port,node1address:port,node2addresses:port... -loglevel level")
	fmt.Println("   -id: 0 based current node ID, indexed into addresses to get local port")
	fmt.Println("   -addresses: comma separated server:port for all nodes")
	fmt.Println("   -loglevel: number 1-4 (1 - error, 2 - warning, 3 - info, 4 - traces, 5 - verbose), default 3")
}

func runRPC(nodeID int, port string, addresses []string) {
	// initialize peers
	peers := make(map[int]raft.NodeInfo)
	for i, v := range addresses {
		if i == nodeID {
			continue
		}

		peers[i] = raft.NodeInfo{
			NodeID:   i,
			Endpoint: v,
		}
	}

	rkv.StartRKV(nodeID, port, peers)
}

func getNodePort(nodeID int, addresses []string) (string, error) {
	address := addresses[nodeID]

	parts := strings.SplitN(address, ":", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("node %d address (%s) is invalid, should be server:port", nodeID, address)
	}

	port := parts[1]
	_, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("node %d address (%s) has invalid port", nodeID, address)
	}

	return port, nil
}
