package kvstore

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
	"github.com/sidecus/raft/pkg/util"
	"google.golang.org/grpc"
)

const rpcTimeOut = time.Duration(150) * time.Millisecond
const chunkSize = 8 * 1024

var errorInvalidGetRequest = errors.New("Get request doesn't have key")

// KVPeerClient defines the proxy used by kv store, implementing IPeerProxyFactory and IPeerProxy
type KVPeerClient struct {
	client pb.KVStoreRaftClient
}

// KVPeerClientFactory is the const factory instance
var KVPeerClientFactory = &KVPeerClient{}

// NewPeerProxy factory method to create a new proxy
func (proxy *KVPeerClient) NewPeerProxy(info raft.PeerInfo) raft.IPeerProxy {
	conn, err := grpc.Dial(info.Endpoint, grpc.WithInsecure())
	if err != nil {
		// Our RPC connection is nonblocking so should not be expecting an error here
		util.Panicln(err)
	}

	client := pb.NewKVStoreRaftClient(conn)

	return &KVPeerClient{
		client: client,
	}
}

// AppendEntries sends AE request to one single node
func (proxy *KVPeerClient) AppendEntries(req *raft.AppendEntriesRequest, callback func(*raft.AppendEntriesReply)) {
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
func (proxy *KVPeerClient) RequestVote(req *raft.RequestVoteRequest, callback func(*raft.RequestVoteReply)) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()

	rv := fromRaftRVRequest(req)
	resp, err := proxy.client.RequestVote(ctx, rv)

	if err == nil {
		reply := toRaftRVReply(resp)
		callback(reply)
	}
}

// InstallSnapshot takes
func (proxy *KVPeerClient) InstallSnapshot(req *raft.SnapshotRequest, callback func(*raft.AppendEntriesReply)) {
	f, err := os.Open(req.File)
	if err != nil {
		//util.WriteError("Cannot open snapshot file (%s). %s", req.File, err)
		return
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut) // longer timeout
	defer cancel()

	stream, err := proxy.client.InstallSnapshot(ctx)
	if err != nil {
		//util.WriteError("Error opening gRPC snapshot stream. %s", err)
		return
	}

	buf := make([]byte, chunkSize)
	for {
		n, err := f.Read(buf)
		if err == io.EOF {
			// done sending
			break
		} else if err != nil {
			// error reading file. don't continue
			return
		}

		sr := fromRaftSnapshotRequest(req)
		sr.Data = buf[:n]
		stream.Send(sr)
	}

	resp, err := stream.CloseAndRecv()
	if err == nil {
		reply := toRaftAEReply(resp)
		callback(reply)
	}
}

// Get gets values from state machine against leader
func (proxy *KVPeerClient) Get(req *raft.GetRequest) (*raft.GetReply, error) {
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
func (proxy *KVPeerClient) Execute(cmd *raft.StateMachineCmd) (*raft.ExecuteReply, error) {

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
