package kvstore

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
	"github.com/sidecus/raft/pkg/util"
	"google.golang.org/grpc"
)

var errorInvalidGetRequest = errors.New("Get request doesn't have key")
var errorInvalidExecuteRequest = errors.New("Execute request is neither Set nor Delete")

type execFunc func(context.Context, *raft.StateMachineCmd) (*raft.ExecuteReply, error)

// rkvRPCProxy defines the proxy used by kv store, implementing IPeerProxyFactory and IPeerProxy
type rkvRPCProxy struct {
	executeMap map[int]execFunc
	rpcClient  pb.KVStoreRaftClient
}

// rkvProxyFactory is the const factory instance
var rkvProxyFactory = &rkvRPCProxy{}

// NewPeerProxy factory method to create a new proxy
func (proxy *rkvRPCProxy) NewPeerProxy(info raft.NodeInfo) raft.IPeerProxy {
	conn, err := grpc.Dial(info.Endpoint, grpc.WithInsecure())
	if err != nil {
		// Our RPC connection is nonblocking so should not be expecting an error here
		util.Panicln(err)
	}
	client := pb.NewKVStoreRaftClient(conn)

	newProxy := &rkvRPCProxy{
		executeMap: make(map[int]execFunc, 2),
		rpcClient:  client,
	}
	newProxy.executeMap[KVCmdSet] = newProxy.executeSet
	newProxy.executeMap[KVCmdDel] = newProxy.executeDelete

	return newProxy
}

// AppendEntries sends AE request to one single node
func (proxy *rkvRPCProxy) AppendEntries(ctx context.Context, req *raft.AppendEntriesRequest) (reply *raft.AppendEntriesReply, err error) {
	var resp *pb.AppendEntriesReply
	if resp, err = proxy.rpcClient.AppendEntries(ctx, fromRaftAERequest(req)); err == nil {
		reply = toRaftAEReply(resp)
	}

	return reply, err
}

// RequestVote handles raft RPC RV calls to a given node
func (proxy *rkvRPCProxy) RequestVote(ctx context.Context, req *raft.RequestVoteRequest) (reply *raft.RequestVoteReply, err error) {
	var resp *pb.RequestVoteReply
	rv := fromRaftRVRequest(req)
	if resp, err = proxy.rpcClient.RequestVote(ctx, rv); err == nil {
		reply = toRaftRVReply(resp)
	}

	return reply, err
}

// InstallSnapshot takes snapshot request (with snapshotfile) and send it to the remote peer
// onReply is gauranteed to be called
func (proxy *rkvRPCProxy) InstallSnapshot(ctx context.Context, req *raft.SnapshotRequest) (reply *raft.AppendEntriesReply, err error) {
	stream, err := proxy.rpcClient.InstallSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	reader, err := raft.ReadSnapshot(req.File)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Create stream writer and copy stream
	writer := raft.NewSnapshotStreamWriter(req, func(r *raft.SnapshotRequest, data []byte) error {
		sr := fromRaftSnapshotRequest(r)
		sr.Data = data
		return stream.Send(sr)
	})

	if _, err = io.Copy(writer, reader); err != nil {
		return nil, err
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, err
	}

	return toRaftAEReply(resp), nil
}

// Get gets values from state machine against leader
func (proxy *rkvRPCProxy) Get(ctx context.Context, req *raft.GetRequest) (*raft.GetReply, error) {
	if len(req.Params) != 1 {
		return nil, errorInvalidGetRequest
	}

	gr := fromRaftGetRequest(req)
	resp, err := proxy.rpcClient.Get(ctx, gr)

	if err != nil {
		return nil, err
	}

	return toRaftGetReply(resp), nil
}

// Execute runs a command via the leader
func (proxy *rkvRPCProxy) Execute(ctx context.Context, cmd *raft.StateMachineCmd) (*raft.ExecuteReply, error) {
	handler, ok := proxy.executeMap[cmd.CmdType]
	if !ok {
		return nil, errorInvalidExecuteRequest
	}

	return handler(ctx, cmd)
}

func (proxy *rkvRPCProxy) executeSet(ctx context.Context, cmd *raft.StateMachineCmd) (*raft.ExecuteReply, error) {
	if cmd.CmdType != KVCmdSet {
		util.Panicln("Wrong cmd passed to executeSet")
	}

	req := fromRaftSetRequest(cmd)

	var resp *pb.SetReply
	var err error
	if resp, err = proxy.rpcClient.Set(ctx, req); err != nil {
		return nil, fmt.Errorf("Error proxying Set request to leader. %s", err)
	}

	return toRaftSetReply(resp), nil
}

func (proxy *rkvRPCProxy) executeDelete(ctx context.Context, cmd *raft.StateMachineCmd) (*raft.ExecuteReply, error) {
	if cmd.CmdType != KVCmdDel {
		util.Panicln("Wrong cmd passed to executeDelete")
	}

	req := fromRaftDeleteRequest(cmd)

	var resp *pb.DeleteReply
	var err error
	if resp, err = proxy.rpcClient.Delete(ctx, req); err != nil {
		return nil, fmt.Errorf("Error proxying Del request to leader. %s", err)
	}

	return toRaftDeleteReply(resp), nil
}
