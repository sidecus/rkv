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
)

var logger = log.New(log.Writer(), log.Prefix(), log.Flags())

func main() {
	nodeID := -1
	addresses := ""

	flag.IntVar(&nodeID, "nodeid", -1, "current node ID. 0 to n where n is total nodes")
	flag.StringVar(&addresses, "addresses", "", "comma separated node addresses, ordered by nodeID")
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

	runRPC(nodeID, port, addrArray)
}

func printUsage() {
	fmt.Println("rkv -nodeid id -addresses node0address,node1address,node2addresses...")
	fmt.Println("  id: 0 based current node ID, indexed into addresses to get local port")
	fmt.Println("  addresses: comma separated, address should be in server:port format")
}

func runRPC(nodeID int, port string, addresses []string) {
	// initialize peers
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
