package kvstore

import (
	"context"
	"errors"
	"time"

	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
	"google.golang.org/grpc"
)

const rpcTimeOut = time.Duration(150) * time.Millisecond

var errorInvalidGetRequest = errors.New("Get request doesn't have key")

// KVPeerProxy defines the proxy used by kv store
type KVPeerProxy struct {
	client pb.KVStoreRaftClient
}

// KVPeerProxyFactory is the const factory instance
var KVPeerProxyFactory = &KVPeerProxy{}

// NewPeerProxy factory method to create a new proxy
func (proxy *KVPeerProxy) NewPeerProxy(info raft.PeerInfo) raft.IPeerProxy {
	conn, err := grpc.Dial(info.Endpoint, grpc.WithInsecure())
	if err != nil {
		// Our RPC connection is nonblocking so should not be expecting an error here
		panic(err.Error())
	}

	client := pb.NewKVStoreRaftClient(conn)

	return &KVPeerProxy{
		client: client,
	}
}

// AppendEntries sends AE request to one single node
func (proxy *KVPeerProxy) AppendEntries(req *raft.AppendEntriesRequest, callback func(*raft.AppendEntriesReply)) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()

	ae := fromRaftAERequest(req)
	resp, err := proxy.client.AppendEntries(ctx, ae)

	if err == nil {
		reply := toRaftAEReply(resp)
		callback(reply)
	}
}

// RequestVote handles raft RPC RV calls to a given node
func (proxy *KVPeerProxy) RequestVote(req *raft.RequestVoteRequest, callback func(*raft.RequestVoteReply)) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()

	rv := fromRaftRVRequest(req)
	resp, err := proxy.client.RequestVote(ctx, rv)

	if err == nil {
		reply := toRaftRVReply(resp)
		callback(reply)
	}
}

// Get gets values from state machine against leader
func (proxy *KVPeerProxy) Get(req *raft.GetRequest) (*raft.GetReply, error) {
	if len(req.Params) != 1 {
		return nil, errorInvalidGetRequest
	}

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()

	gr := fromRaftGetRequest(req)
	resp, err := proxy.client.Get(ctx, gr)

	if err != nil {
		return nil, err
	}

	return toRaftGetReply(resp), nil
}

// Execute runs a command via the leader
func (proxy *KVPeerProxy) Execute(cmd *raft.StateMachineCmd) (*raft.ExecuteReply, error) {

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()

	var reply *raft.ExecuteReply
	var err error
	if cmd.CmdType == KVCmdSet {
		req := fromRaftSetRequest(cmd)
		resp, errSet := proxy.client.Set(ctx, req)

		reply = toRaftSetReply(resp)
		err = errSet
	} else {
		req := fromRaftDeleteRequest(cmd)
		resp, errDel := proxy.client.Delete(ctx, req)

		reply = toRaftDeleteReply(resp)
		err = errDel
	}

	return reply, err
}
