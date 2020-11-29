package rpc

import (
	"context"
	"errors"
	"time"

	"github.com/sidecus/raft/pkg/kvstore"
	"github.com/sidecus/raft/pkg/kvstore/rpc/pb"
	"github.com/sidecus/raft/pkg/raft"
	"google.golang.org/grpc"
)

const rpcTimeOut = time.Duration(150) * time.Millisecond

var errorNoPeersProvided = errors.New("No raft peers provided")
var errorInvalidNodeID = errors.New("Invalid node id")
var errorInvalidGetRequest = errors.New("Get request doesn't have key")

type peerNode struct {
	info   raft.PeerInfo
	client pb.KVStoreRaftClient
}

// KVStorePeerProxy manages RPC connection to peer nodes
type KVStorePeerProxy struct {
	Peers map[int]peerNode
}

// NewKVStorePeerProxy creates the node proxy for kv store
func NewKVStorePeerProxy(peers []raft.PeerInfo) raft.IPeerProxy {
	if len(peers) == 0 {
		panic(errorNoPeersProvided)
	}

	proxy := KVStorePeerProxy{
		Peers: make(map[int]peerNode),
	}

	for i, peer := range peers {
		conn, err := grpc.Dial(peer.Endpoint, grpc.WithInsecure())
		if err != nil {
			// Our RPC connection is nonblocking so should not be expecting an error here
			panic(err.Error())
		}

		client := pb.NewKVStoreRaftClient(conn)

		proxy.Peers[i] = peerNode{
			info:   peer,
			client: client,
		}
	}

	return &proxy
}

// AppendEntries handles raft RPC AE calls to all peers
func (proxy *KVStorePeerProxy) AppendEntries(req *raft.AppendEntriesRequest, callback func(*raft.AppendEntriesReply)) {
	entries := make([]*pb.LogEntry, len(req.Entries))
	for i, v := range req.Entries {
		cmd := &pb.KVCmd{
			CmdType: int32(v.Cmd.CmdType),
			Data: &pb.KVCmdData{
				Key:   v.Cmd.Data.(kvstore.KVCmdData).Key,
				Value: v.Cmd.Data.(kvstore.KVCmdData).Value,
			},
		}
		entry := &pb.LogEntry{
			Index:     int64(v.Index),
			Term:      int64(v.Term),
			Committed: v.Committed,
			Cmd:       cmd,
		}

		entries[i] = entry
	}

	ae := &pb.AppendEntriesRequest{
		Term:         int64(req.Term),
		LeaderId:     int64(req.LeaderID),
		PrevLogIndex: int64(req.PrevLogIndex),
		PrevLogTerm:  int64(req.PrevLogTerm),
		LeaderCommit: int64(req.LeaderCommit),
		Entries:      entries,
	}

	for _, peer := range proxy.Peers {
		nodeID := peer.info.NodeID
		client := peer.client
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
			defer cancel()
			resp, err := client.AppendEntries(ctx, ae)
			if err == nil {
				reply := raft.NewAppendEntriesReply(nodeID, int(resp.Term), int(resp.LeaderID), resp.Success)
				callback(reply)
			}
		}()
	}
}

// RequestVote handles raft RPC RV calls to a given node
func (proxy *KVStorePeerProxy) RequestVote(req *raft.RequestVoteRequest, callback func(*raft.RequestVoteReply)) {
	rv := &pb.RequestVoteRequest{
		Term:         int64(req.Term),
		CandidateId:  int64(req.CandidateID),
		LastLogIndex: int64(req.LastLogIndex),
		LastLogTerm:  int64(req.LastLogTerm),
	}

	for _, peer := range proxy.Peers {
		nodeID := peer.info.NodeID
		client := peer.client
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
			defer cancel()
			resp, err := client.RequestVote(ctx, rv)
			if err == nil {
				reply := raft.NewRequestVoteReply(nodeID, int(resp.Term), int(resp.VotedTerm), resp.VoteGranted)
				callback(reply)
			}
		}()
	}
}

// Get gets values from state machine against leader
func (proxy *KVStorePeerProxy) Get(nodeID int, req *raft.GetRequest) (*raft.GetReply, error) {
	peer, ok := proxy.Peers[nodeID]
	if !ok {
		return nil, errorInvalidNodeID
	}

	if len(req.Params) != 1 {
		return nil, errorInvalidGetRequest
	}

	get := &pb.GetRequest{
		Key: req.Params[0].(string),
	}

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()
	resp, err := peer.client.Get(ctx, get)
	if err != nil {
		return nil, err
	}

	ret := raft.NewGetReply(resp.Value)
	return ret, nil
}

// Execute runs a command via the leader
func (proxy *KVStorePeerProxy) Execute(nodeID int, cmd *raft.StateMachineCmd) (bool, error) {
	peer, ok := proxy.Peers[nodeID]
	if !ok {
		return false, errorInvalidNodeID
	}

	ret := false
	err := error(nil)
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()
	if cmd.CmdType == kvstore.KVCmdSet {
		req := &pb.SetRequest{
			Key:   cmd.Data.(kvstore.KVCmdData).Key,
			Value: cmd.Data.(kvstore.KVCmdData).Value,
		}
		resp, errSet := peer.client.Set(ctx, req)
		ret, err = resp.Success, errSet
	} else {
		req := &pb.DeleteRequest{
			Key: cmd.Data.(kvstore.KVCmdData).Key,
		}
		resp, errSet := peer.client.Delete(ctx, req)
		ret, err = resp.Success, errSet
	}

	if err != nil {
		return false, err
	}

	return ret, err
}
