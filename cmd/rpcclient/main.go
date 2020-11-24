package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/internal/net/grpcnet/pb"
	"github.com/sidecus/raft/pkg/network"
)

func main() {
	url := os.Args[1]
	conn, err := grpc.Dial(url, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	client := pb.NewRafterClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reply, err := client.SendMessage(ctx, &pb.RaftMessage{
		NodeID:  1,
		Term:    30000,
		MsgType: network.MsgHeartbeat,
		Data:    3,
	})

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(reply)
}
