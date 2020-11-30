package rpc

import (
	"context"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/kvstore"
	"github.com/sidecus/raft/pkg/kvstore/rpc/pb"
	"github.com/sidecus/raft/pkg/raft"
)

// KVStoreRPCServer is used to implement pb.KVStoreRPCServer
type KVStoreRPCServer struct {
	wg     sync.WaitGroup
	node   raft.INode
	server *grpc.Server
	pb.UnimplementedKVStoreRaftServer
}

// NewKVStoreRPCServer creates a new RPC server
func NewKVStoreRPCServer(node raft.INode) KVStoreRPCServer {
	return KVStoreRPCServer{
		node: node,
	}
}

// AppendEntries implements KVStoreRafterServer.AppendEntries
func (s *KVStoreRPCServer) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesReply, error) {
	entries := make([]raft.LogEntry, len(req.Entries))
	for i, v := range req.Entries {
		cmdData := kvstore.KVCmdData{
			Key:   v.Cmd.Data.Key,
			Value: v.Cmd.Data.Value,
		}

		cmd := raft.StateMachineCmd{
			CmdType: int(v.Cmd.CmdType),
			Data:    cmdData,
		}

		entries[i] = raft.LogEntry{
			Index:     int(v.Index),
			Term:      int(v.Term),
			Committed: v.Committed,
			Cmd:       cmd,
		}
	}

	ae := &raft.AppendEntriesRequest{
		Term:         int(req.Term),
		LeaderID:     int(req.LeaderId),
		PrevLogIndex: int(req.PrevLogIndex),
		PrevLogTerm:  int(req.PrevLogTerm),
		LeaderCommit: int(req.LeaderCommit),
		Entries:      entries,
	}

	// Just send the message to the channel
	resp, err := s.node.AppendEntries(ae)

	if err != nil {
		return nil, err
	}

	return &pb.AppendEntriesReply{
		Term:     int64(resp.Term),
		LeaderID: int64(resp.LeaderID),
		Success:  resp.Success,
	}, nil
}

// RequestVote requests a vote from the node
func (s *KVStoreRPCServer) RequestVote(ctx context.Context, req *pb.RequestVoteRequest) (*pb.RequestVoteReply, error) {
	rv := &raft.RequestVoteRequest{
		Term:         int(req.Term),
		CandidateID:  int(req.CandidateId),
		LastLogIndex: int(req.LastLogIndex),
		LastLogTerm:  int(req.LastLogTerm),
	}
	resp, err := s.node.RequestVote(rv)

	if err != nil {
		return nil, err
	}

	return &pb.RequestVoteReply{
		Term:        int64(resp.Term),
		VotedTerm:   int64(resp.VotedTerm),
		VoteGranted: resp.VoteGranted,
	}, nil
}

// Set sets a value in the kv store
func (s *KVStoreRPCServer) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetReply, error) {
	cmdData := kvstore.KVCmdData{
		Key:   req.Key,
		Value: req.Value,
	}

	cmd := raft.StateMachineCmd{
		CmdType: kvstore.KVCmdSet,
		Data:    cmdData,
	}

	resp, err := s.node.Execute(&cmd)

	if err != nil {
		return nil, err
	}

	return &pb.SetReply{
		Success: resp,
	}, nil
}

// Delete deletes a value from the kv store
func (s *KVStoreRPCServer) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteReply, error) {
	cmdData := kvstore.KVCmdData{
		Key: req.Key,
	}

	cmd := &raft.StateMachineCmd{
		CmdType: kvstore.KVCmdDel,
		Data:    cmdData,
	}

	resp, err := s.node.Execute(cmd)

	if err != nil {
		return nil, err
	}

	return &pb.DeleteReply{
		Success: resp,
	}, nil
}

// Get implements pb.KVStoreRaftRPCServer.Get
func (s *KVStoreRPCServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetReply, error) {
	key := req.Key
	gr := &raft.GetRequest{Params: []interface{}{key}}

	resp, err := s.node.Get(gr)

	if err != nil {
		return nil, err
	}

	return &pb.GetReply{
		Value:   resp.Data.(string),
		Success: true,
	}, nil
}

// Start starts the grpc server on a different go routine
func (s *KVStoreRPCServer) Start(port string) {
	s.wg.Add(1)
	go func() {
		var opts []grpc.ServerOption
		s.server = grpc.NewServer(opts...)
		pb.RegisterKVStoreRaftServer(s.server, s)

		lis, err := net.Listen("tcp", ":"+port)
		if err != nil {
			panic(fmt.Sprintf("Cannot listen on port %s. Error:%s", port, err.Error()))
		}

		s.server.Serve(lis)
		s.wg.Done()
	}()
}

// Stop stops the rpc server
func (s *KVStoreRPCServer) Stop() {
	s.server.Stop()
	s.wg.Wait()
}
