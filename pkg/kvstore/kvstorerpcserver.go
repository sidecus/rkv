package kvstore

import (
	"context"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
)

// RKVRPCServer is used to implement pb.KVStoreRPCServer
type RKVRPCServer struct {
	wg     *sync.WaitGroup
	node   raft.INode
	server *grpc.Server
	pb.UnimplementedKVStoreRaftServer
}

// NewRKVRPCServer creates a new RPC server
func NewRKVRPCServer(node raft.INode, wg *sync.WaitGroup) *RKVRPCServer {
	return &RKVRPCServer{
		node: node,
		wg:   wg,
	}
}

// AppendEntries implements KVStoreRafterServer.AppendEntries
func (s *RKVRPCServer) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesReply, error) {
	ae := toRaftAERequest(req)
	resp, err := s.node.AppendEntries(ctx, ae)

	if err != nil {
		return nil, err
	}

	return fromRaftAEReply(resp), nil
}

// RequestVote requests a vote from the node
func (s *RKVRPCServer) RequestVote(ctx context.Context, req *pb.RequestVoteRequest) (*pb.RequestVoteReply, error) {
	rv := toRaftRVRequest(req)
	resp, err := s.node.RequestVote(ctx, rv)

	if err != nil {
		return nil, err
	}

	return fromRaftRVReply(resp), nil
}

// InstallSnapshot installs snapshot on current node
// TODO[sidecus]: This keeps the reading loop out of raft and it has no idea of chunk reception (and hence no response on each chunk).
// If the snapshot is big it might cause resending from leader
func (s *RKVRPCServer) InstallSnapshot(stream pb.KVStoreRaft_InstallSnapshotServer) error {
	reader, err := raft.NewSnapshotStreamReader(func() (*raft.SnapshotRequest, []byte, error) {
		var pbReq *pb.SnapshotRequest
		var err error
		if pbReq, err = stream.Recv(); err != nil {
			return nil, nil, err
		}

		return toRaftSnapshotRequest(pbReq), pbReq.Data, nil
	})

	if err != nil {
		return err
	}

	// Open snapshot file
	req := reader.RequestHeader()
	file, w, err := raft.CreateSnapshot(s.node.NodeID(), req.SnapshotTerm, req.SnapshotIndex, "remote")
	if err != nil {
		return err
	}
	defer w.Close()

	// Copy to the file
	req.File = file
	if _, err = io.Copy(w, reader); err != nil {
		return err
	}

	// Close snapshot file and try to install
	w.Close()
	var reply *raft.AppendEntriesReply
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(500)*time.Millisecond)
	defer cancel()
	if reply, err = s.node.InstallSnapshot(ctx, req); err != nil {
		return err
	}

	return stream.SendAndClose(fromRaftAEReply(reply))
}

// Set sets a value in the kv store
func (s *RKVRPCServer) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetReply, error) {
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
func (s *RKVRPCServer) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteReply, error) {
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
func (s *RKVRPCServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetReply, error) {
	gr := toRaftGetRequest(req)
	resp, err := s.node.Get(ctx, gr)

	if err != nil {
		return nil, err
	}

	return fromRaftGetReply(resp), nil
}

// Start starts the grpc server on a different go routine
func (s *RKVRPCServer) Start(port string) {
	var opts []grpc.ServerOption
	s.server = grpc.NewServer(opts...)
	pb.RegisterKVStoreRaftServer(s.server, s)

	s.wg.Add(1)
	go func() {
		if lis, err := net.Listen("tcp", ":"+port); err != nil {
			log.Fatalf("Cannot listen on port %s. Error:%s", port, err)
		} else if err := s.server.Serve(lis); err != nil {
			log.Fatalf("Failed to server %ss", err)
		}

		s.wg.Done()
	}()
}

// Stop stops the rpc server
func (s *RKVRPCServer) Stop() {
	s.server.Stop()
	s.wg.Wait()
}
