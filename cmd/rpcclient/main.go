package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/kvstore/rpc/pb"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("rpcclient set -address <address> -key <key> -value <value> | rpcclient get -address <address> -key <key>")
	}

	var mode, address, key, value string
	switch os.Args[1] {
	case "set":
		mode = "set"
		setCmd := flag.NewFlagSet("set", flag.ExitOnError)
		setCmd.StringVar(&address, "address", "", "rpc endpoint")
		setCmd.StringVar(&key, "key", "", "kv store key to set")
		setCmd.StringVar(&value, "value", "", "kv store value to set")
		setCmd.Parse(os.Args[2:])
	case "get":
		mode = "get"
		getCmd := flag.NewFlagSet("get", flag.ExitOnError)
		getCmd.StringVar(&address, "address", "", "rpc endpoint")
		getCmd.StringVar(&key, "key", "", "kv store key to set")
		getCmd.Parse(os.Args[2:])
	default:
		fmt.Println("rpcclient set -address <address> -key <key> -value <value> | rpcclient get -address <address> -key <key>")
		return
	}

	if address == "" || key == "" {
		fmt.Println("rpcclient set -address <address> -key <key> -value <value> | rpcclient get -address <address> -key <key>")
	}

	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	client := pb.NewKVStoreRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if mode == "set" {
		reply, err := client.Set(ctx, &pb.SetRequest{Key: key, Value: value})
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println(reply)
	} else if mode == "get" {
		reply, err := client.Get(ctx, &pb.GetRequest{Key: key})
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println(reply)
	}
}
