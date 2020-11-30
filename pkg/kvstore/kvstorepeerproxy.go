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

var errorNoPeersProvided = errors.New("No raft peers provided")
var errorInvalidNodeID = errors.New("Invalid node id")
var errorInvalidGetRequest = errors.New("Get request doesn't have key")

type peerNode struct {
	info   raft.PeerInfo
	client pb.KVStoreRaftClient
}

// PeerProxy manages RPC connection to peer nodes
type PeerProxy struct {
	Peers map[int]peerNode
}

// NewPeerProxy creates the node proxy for kv store
func NewPeerProxy(peers []raft.PeerInfo) raft.IPeerProxy {
	if len(peers) == 0 {
		panic(errorNoPeersProvided)
	}

	proxy := PeerProxy{
		Peers: make(map[int]peerNode),
	}

	for _, peer := range peers {
		conn, err := grpc.Dial(peer.Endpoint, grpc.WithInsecure())
		if err != nil {
			// Our RPC connection is nonblocking so should not be expecting an error here
			panic(err.Error())
		}

		client := pb.NewKVStoreRaftClient(conn)

		proxy.Peers[peer.NodeID] = peerNode{
			info:   peer,
			client: client,
		}
	}

	return &proxy
}

// AppendEntries sends AE request to one single node
func (proxy *PeerProxy) AppendEntries(nodeID int, req *raft.AppendEntriesRequest, callback func(*raft.AppendEntriesReply)) {
	entries := make([]*pb.LogEntry, len(req.Entries))
	for i, v := range req.Entries {
		cmd := &pb.KVCmd{
			CmdType: int32(v.Cmd.CmdType),
			Data: &pb.KVCmdData{
				Key:   v.Cmd.Data.(KVCmdData).Key,
				Value: v.Cmd.Data.(KVCmdData).Value,
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

	// Send request to the peer node on different go routine
	client := proxy.Peers[nodeID].client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
		defer cancel()
		resp, err := client.AppendEntries(ctx, ae)
		if err == nil {
			reply := &raft.AppendEntriesReply{
				NodeID:   nodeID,
				LeaderID: int(resp.LeaderID),
				Term:     int(resp.Term),
				Success:  resp.Success,
			}

			callback(reply)
		}
	}()
}

// BroadcastAppendEntries handles raft RPC AE calls to all peers (heartbeat)
func (proxy *PeerProxy) BroadcastAppendEntries(req *raft.AppendEntriesRequest, callback func(*raft.AppendEntriesReply)) {
	// send request to all peers
	for _, peer := range proxy.Peers {
		nodeID := peer.info.NodeID
		proxy.AppendEntries(nodeID, req, callback)
	}
}

// RequestVote handles raft RPC RV calls to a given node
func (proxy *PeerProxy) RequestVote(req *raft.RequestVoteRequest, callback func(*raft.RequestVoteReply)) {
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
				reply := &raft.RequestVoteReply{
					NodeID:      nodeID,
					Term:        int(resp.Term),
					VotedTerm:   int(resp.VotedTerm),
					VoteGranted: resp.VoteGranted,
				}
				callback(reply)
			}
		}()
	}
}

// Get gets values from state machine against leader
func (proxy *PeerProxy) Get(nodeID int, req *raft.GetRequest) (*raft.GetReply, error) {
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

	return &raft.GetReply{
		Data: resp.Value,
	}, nil
}

// Execute runs a command via the leader
func (proxy *PeerProxy) Execute(nodeID int, cmd *raft.StateMachineCmd) (*raft.ExecuteReply, error) {
	peer, ok := proxy.Peers[nodeID]
	if !ok {
		return nil, errorInvalidNodeID
	}

	respNodeID := -1
	success := false
	err := error(nil)
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeOut)
	defer cancel()
	if cmd.CmdType == KVCmdSet {
		req := &pb.SetRequest{
			Key:   cmd.Data.(KVCmdData).Key,
			Value: cmd.Data.(KVCmdData).Value,
		}
		resp, errSet := peer.client.Set(ctx, req)
		respNodeID, success, err = int(resp.NodeID), resp.Success, errSet
	} else {
		req := &pb.DeleteRequest{
			Key: cmd.Data.(KVCmdData).Key,
		}
		resp, errDel := peer.client.Delete(ctx, req)
		respNodeID, success, err = int(resp.NodeID), resp.Success, errDel
	}

	if err != nil {
		return nil, err
	}

	return &raft.ExecuteReply{
		NodeID:  respNodeID,
		Success: success,
	}, err
}
