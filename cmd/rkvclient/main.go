package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/kvstore/pb"
)

const (
	getMode       = "get"
	setMode       = "set"
	delMode       = "del"
	benchMarkMode = "benchmark"
)

func main() {
	mode := parseArgs()

	conn, err := grpc.Dial(mode.address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewKVStoreRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	switch mode.name {
	case getMode:
		get(ctx, client, &pb.GetRequest{Key: mode.params.(string)})
	case setMode:
		set(ctx, client, &pb.SetRequest{Key: mode.params.(keyValuePair).key, Value: mode.params.(keyValuePair).value})
	case delMode:
		delete(ctx, client, &pb.DeleteRequest{Key: mode.params.(string)})
	case benchMarkMode:
		benchMark(ctx, client, mode.params.(int))
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

func benchMark(ctx context.Context, client pb.KVStoreRaftClient, times int) {
	if times <= 0 {
		fmt.Println("times for benchmark mode cannot be 0 or less")
		os.Exit(1)
	}

	start := time.Now()
	successCount := 0
	var wg sync.WaitGroup
	for i := 0; i < times; i++ {
		wg.Add(1)
		go func(i int) {
			_, err := client.Set(ctx, &pb.SetRequest{Key: fmt.Sprintf("k%d", i), Value: fmt.Sprint(i)})
			if err == nil {
				successCount++
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	elapsed := time.Now().Sub(start)
	fmt.Printf("Ran Set for %d times. Total time taken: %d ms, success count: %d", times, elapsed/time.Millisecond, successCount)
}

type runMode struct {
	name    string
	address string
	params  interface{}
}

type keyValuePair struct {
	key   string
	value string
}

func parseArgs() runMode {
	if len(os.Args) < 2 {
		fmt.Println("Not enough arguments")
		printUsage()
		os.Exit(1)
	}

	mode := runMode{
		name: os.Args[1],
	}

	args := os.Args[2:]
	switch mode.name {
	case getMode:
		key := ""
		getCmd := flag.NewFlagSet(getMode, flag.ExitOnError)
		getCmd.StringVar(&mode.address, "address", "", "rpc endpoint")
		getCmd.StringVar(&key, "key", "", "kv store key to get")
		getCmd.Parse(args)
		mode.params = key
	case setMode:
		kvp := keyValuePair{}
		setCmd := flag.NewFlagSet(setMode, flag.ExitOnError)
		setCmd.StringVar(&mode.address, "address", "", "rpc endpoint")
		setCmd.StringVar(&kvp.key, "key", "", "kv store key to set")
		setCmd.StringVar(&kvp.value, "value", "", "kv store value to set")
		setCmd.Parse(args)
		mode.params = kvp
	case delMode:
		key := ""
		delCmd := flag.NewFlagSet(delMode, flag.ExitOnError)
		delCmd.StringVar(&mode.address, "address", "", "rpc endpoint")
		delCmd.StringVar(&key, "key", "", "kv store key to delete")
		delCmd.Parse(args)
		mode.params = key
	case benchMarkMode:
		times := 10000
		benchMarkCmd := flag.NewFlagSet(benchMarkMode, flag.ExitOnError)
		benchMarkCmd.StringVar(&mode.address, "address", "", "rpc endpoint")
		benchMarkCmd.IntVar(&times, "times", 10000, "times to run")
		benchMarkCmd.Parse(args)
		mode.params = times
	}

	if mode.name == "" || mode.address == "" {
		fmt.Println("unknown mode or address")
		printUsage()
		os.Exit(1)
	}

	return mode
}

func printUsage() {
	fmt.Println("rkvclient set -address <address> -key <key> -value <value>")
	fmt.Println("rkvclient get -address <address> -key <key>")
	fmt.Println("rkvclient del -address <address> -key <key>")
	fmt.Println("rkvclient benchmark -address <address> -times <times>")
}
