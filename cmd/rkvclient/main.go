package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/rkv/pb"
)

const (
	getMode       = "get"
	setMode       = "set"
	delMode       = "del"
	benchMarkMode = "benchmark"
)

func main() {
	mode := parseArgs()

	conn, err := getConnection(mode.address)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	switch mode.name {
	case getMode:
		get(conn, &pb.GetRequest{Key: mode.params.(string)})
	case setMode:
		set(conn, &pb.SetRequest{Key: mode.params.(keyValuePair).key, Value: mode.params.(keyValuePair).value})
	case delMode:
		delete(conn, &pb.DeleteRequest{Key: mode.params.(string)})
	case benchMarkMode:
		benchmark(conn, mode.params.(int))
	}
}

func getConnection(address string) (*grpc.ClientConn, error) {
	return grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
}

func get(conn *grpc.ClientConn, req *pb.GetRequest) {
	client := pb.NewKVStoreRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reply, err := client.Get(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Success :%v\n", reply.Success)
	fmt.Printf("Value   :%s\n", reply.Value)
	fmt.Printf("Run on  :Node%d\n", reply.NodeID)
}

func set(conn *grpc.ClientConn, req *pb.SetRequest) {
	client := pb.NewKVStoreRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reply, err := client.Set(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Success :%v\n", reply.Success)
	fmt.Printf("Run on  :Node%d\n", reply.NodeID)
}

func delete(conn *grpc.ClientConn, req *pb.DeleteRequest) {
	client := pb.NewKVStoreRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reply, err := client.Delete(ctx, req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Success :%v\n", reply.Success)
	fmt.Printf("Run on  :Node%d\n", reply.NodeID)
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
	default:
		mode.name = ""
	}

	if mode.name == "" {
		printUsage()
		log.Fatalln("Unsupported mode")
	}

	if mode.address == "" {
		printUsage()
		log.Fatalln("address cannot be empty")
	}

	return mode
}

func printUsage() {
	fmt.Println("Usage of rkvclient:")
	fmt.Println("\trkvclient <mode> -address <nodeaddress> <othermodeparams>")
	fmt.Println("Supported modes:")
	fmt.Println("\tset       -address <address> -key <key> -value <value>")
	fmt.Println("\tget       -address <address> -key <key>")
	fmt.Println("\tdel       -address <address> -key <key>")
	fmt.Println("\tbenchmark -address <address> -times <times>")
	fmt.Println()
}
