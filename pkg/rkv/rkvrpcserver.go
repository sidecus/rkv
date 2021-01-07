package rkv

import (
	"context"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/raft"
	"github.com/sidecus/raft/pkg/rkv/pb"
	"github.com/sidecus/raft/pkg/util"
)

// rkvRPCServer is used to implement pb.KVStoreRPCServer
type rkvRPCServer struct {
	wg     *sync.WaitGroup
	node   raft.INode
	server *grpc.Server
	pb.UnimplementedKVStoreRaftServer
}

// newRKVRPCServer creates a new RPC server
func newRKVRPCServer(node raft.INode, wg *sync.WaitGroup) *rkvRPCServer {
	return &rkvRPCServer{
		node: node,
		wg:   wg,
	}
}

// AppendEntries implements KVStoreRPCServer.AppendEntries
func (s *rkvRPCServer) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesReply, error) {
	ae := toRaftAERequest(req)
	resp, err := s.node.AppendEntries(ctx, ae)

	if err != nil {
		return nil, err
	}

	return fromRaftAEReply(resp), nil
}

// RequestVote requests a vote from the node
func (s *rkvRPCServer) RequestVote(ctx context.Context, req *pb.RequestVoteRequest) (*pb.RequestVoteReply, error) {
	rv := toRaftRVRequest(req)
	resp, err := s.node.RequestVote(ctx, rv)

	if err != nil {
		return nil, err
	}

	return fromRaftRVReply(resp), nil
}

// InstallSnapshot receives and installs snapshot on current node
func (s *rkvRPCServer) InstallSnapshot(stream pb.KVStoreRaft_InstallSnapshotServer) error {
	// Create snapshot reader over grpc
	recvFunc := func() (*raft.SnapshotRequestHeader, []byte, error) {
		var pbReq *pb.SnapshotRequest
		var err error
		if pbReq, err = stream.Recv(); err != nil {
			return nil, nil, err
		}

		return toRaftSnapshotRequestHeader(pbReq), pbReq.Data, nil
	}
	reader, err := raft.NewSnapshotStreamReader(recvFunc, s.node.OnSnapshotPart)
	if err != nil {
		return err
	}

	// receive snapshot to file
	req, err := raft.ReceiveSnapshot(s.node.NodeID(), reader)
	if err != nil {
		return err
	}

	// Install and reply
	var reply *raft.AppendEntriesReply
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(500)*time.Millisecond)
	defer cancel()

	if reply, err = s.node.InstallSnapshot(ctx, req); err != nil {
		return err
	}

	return stream.SendAndClose(fromRaftAEReply(reply))
}

// Set sets a value in the kv store
func (s *rkvRPCServer) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetReply, error) {
	cmd := toRaftSetRequest(req)

	exeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := s.node.Execute(exeCtx, cmd)

	if err != nil {
		return nil, err
	}

	return fromRaftSetReply(resp), nil
}

// Delete deletes a value from the kv store
func (s *rkvRPCServer) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteReply, error) {
	cmd := toRaftDeleteRequest(req)

	exeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := s.node.Execute(exeCtx, cmd)

	if err != nil {
		return nil, err
	}

	return fromRaftDeleteReply(resp), nil
}

// Get implements pb.KVStoreRaftRPCServer.Get
func (s *rkvRPCServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetReply, error) {
	gr := toRaftGetRequest(req)
	resp, err := s.node.Get(ctx, gr)

	if err != nil {
		return nil, err
	}

	return fromRaftGetReply(resp), nil
}

// Start starts the grpc server on a different go routine
func (s *rkvRPCServer) Start(port string) {
	var opts []grpc.ServerOption
	s.server = grpc.NewServer(opts...)
	pb.RegisterKVStoreRaftServer(s.server, s)

	s.wg.Add(1)
	go func() {
		if lis, err := net.Listen("tcp", ":"+port); err != nil {
			util.Fatalf("Cannot listen on port %s. Error:%s", port, err)
		} else if err := s.server.Serve(lis); err != nil {
			util.Fatalf("Failed to server %ss", err)
		}

		s.wg.Done()
	}()
}

// Stop stops the rpc server
func (s *rkvRPCServer) Stop() {
	s.server.Stop()
	s.wg.Wait()
}
