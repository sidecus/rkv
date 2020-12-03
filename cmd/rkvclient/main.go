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

const (
	getMode = "get"
	setMode = "set"
	delMode = "del"
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

	switch mode {
	case getMode:
		get(ctx, client, &pb.GetRequest{Key: key})
	case setMode:
		set(ctx, client, &pb.SetRequest{Key: key, Value: value})
	case delMode:
		delete(ctx, client, &pb.DeleteRequest{Key: key})
	}
}

func get(ctx context.Context, client pb.KVStoreRaftClient, req *pb.GetRequest) {
	reply, err := client.Get(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Node%d - Get Success: %v, Value: %s\n", reply.NodeID, reply.Success, reply.Value)
}

func set(ctx context.Context, client pb.KVStoreRaftClient, req *pb.SetRequest) {
	reply, err := client.Set(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Node%d - Set Success: %v\n", reply.NodeID, reply.Success)
}

func delete(ctx context.Context, client pb.KVStoreRaftClient, req *pb.DeleteRequest) {
	reply, err := client.Delete(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Node%d - Delete Success: %v\n", reply.NodeID, reply.Success)
}

func parseArgs() (mode, address, key, value string) {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	mode = os.Args[1]
	args := os.Args[2:]
	switch mode {
	case getMode:
		getCmd := flag.NewFlagSet("get", flag.ExitOnError)
		getCmd.StringVar(&address, "address", "", "rpc endpoint")
		getCmd.StringVar(&key, "key", "", "kv store key to get")
		getCmd.Parse(args)
	case setMode:
		setCmd := flag.NewFlagSet("set", flag.ExitOnError)
		setCmd.StringVar(&address, "address", "", "rpc endpoint")
		setCmd.StringVar(&key, "key", "", "kv store key to set")
		setCmd.StringVar(&value, "value", "", "kv store value to set")
		setCmd.Parse(args)
	case delMode:
		delCmd := flag.NewFlagSet("del", flag.ExitOnError)
		delCmd.StringVar(&address, "address", "", "rpc endpoint")
		delCmd.StringVar(&key, "key", "", "kv store key to delete")
		delCmd.Parse(args)
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
	fmt.Println("rkvclient del -address <address> -key <key>")
}
