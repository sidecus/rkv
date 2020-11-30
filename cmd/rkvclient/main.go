package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/kvstore/pb"
)

func main() {
	mode, address, key, value := parseArgs()

	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewKVStoreRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if mode == "set" {
		set(ctx, client, &pb.SetRequest{Key: key, Value: value})
	} else if mode == "get" {
		get(ctx, client, &pb.GetRequest{Key: key})
	}
}

func get(ctx context.Context, client pb.KVStoreRaftClient, req *pb.GetRequest) {
	reply, err := client.Get(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("NodeID: %d, Success: %v, Value: %s", reply.NodeID, reply.Success, reply.Value)
}

func set(ctx context.Context, client pb.KVStoreRaftClient, req *pb.SetRequest) {
	reply, err := client.Set(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("NodeID: %d, Success: %v", reply.NodeID, reply.Success)
}

func parseArgs() (mode, address, key, value string) {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	mode = os.Args[1]
	args := os.Args[2:]
	switch mode {
	case "set":
		setCmd := flag.NewFlagSet("set", flag.ExitOnError)
		setCmd.StringVar(&address, "address", "", "rpc endpoint")
		setCmd.StringVar(&key, "key", "", "kv store key to set")
		setCmd.StringVar(&value, "value", "", "kv store value to set")
		setCmd.Parse(args)
	case "get":
		getCmd := flag.NewFlagSet("get", flag.ExitOnError)
		getCmd.StringVar(&address, "address", "", "rpc endpoint")
		getCmd.StringVar(&key, "key", "", "kv store key to set")
		getCmd.Parse(args)
	default:
	}

	if address == "" || key == "" {
		printUsage()
		os.Exit(1)
	}

	return
}

func printUsage() {
	fmt.Println("rkvclient set -address <address> -key <key> -value <value>")
	fmt.Println("rkvclient get -address <address> -key <key>")
}
