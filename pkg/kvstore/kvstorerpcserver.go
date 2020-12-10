package kvstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/grpc"

	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
)

// RPCServer is used to implement pb.KVStoreRPCServer
type RPCServer struct {
	wg     sync.WaitGroup
	node   raft.INodeRPCProvider
	server *grpc.Server
	pb.UnimplementedKVStoreRaftServer
}

// NewServer creates a new RPC server
func NewServer(node raft.INodeRPCProvider) *RPCServer {
	return &RPCServer{
		node: node,
	}
}

// AppendEntries implements KVStoreRafterServer.AppendEntries
func (s *RPCServer) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesReply, error) {
	ae := toRaftAERequest(req)
	resp, err := s.node.AppendEntries(ae)

	if err != nil {
		return nil, err
	}

	return fromRaftAEReply(resp), nil
}

// RequestVote requests a vote from the node
func (s *RPCServer) RequestVote(ctx context.Context, req *pb.RequestVoteRequest) (*pb.RequestVoteReply, error) {
	rv := toRaftRVRequest(req)
	resp, err := s.node.RequestVote(rv)

	if err != nil {
		return nil, err
	}

	return fromRaftRVReply(resp), nil
}

// InstallSnapshot installs snapshot on the target node
func (s *RPCServer) InstallSnapshot(stream pb.KVStoreRaft_InstallSnapshotServer) error {
	// TODO[sidecus]: Allow passing snapshot path as parameter instead of using current working directory
	var sr *raft.SnapshotRequest
	var file *os.File

	for {
		reqChunk, err := stream.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		// Create file if not yet
		if file == nil {
			if sr, file, err = s.generateSnapshotFileName(reqChunk); err != nil {
				return err
			}

			defer file.Close()
		}

		// Write data
		if _, err = file.Write(reqChunk.Data); err != nil {
			return err
		}
	}

	if sr == nil {
		return errors.New("empty snapshot received")
	}

	// close file before installing
	file.Close()
	var resp *raft.AppendEntriesReply
	var err error
	if resp, err = s.node.InstallSnapshot(sr); err != nil {
		return err
	}
	return stream.SendAndClose(fromRaftAEReply(resp))
}

// Set sets a value in the kv store
func (s *RPCServer) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetReply, error) {
	cmd := toRaftSetRequest(req)
	resp, err := s.node.Execute(cmd)

	if err != nil {
		return nil, err
	}

	return fromRaftSetReply(resp), nil
}

// Delete deletes a value from the kv store
func (s *RPCServer) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteReply, error) {
	cmd := toRaftDeleteRequest(req)
	resp, err := s.node.Execute(cmd)

	if err != nil {
		return nil, err
	}

	return fromRaftDeleteReply(resp), nil
}

// Get implements pb.KVStoreRaftRPCServer.Get
func (s *RPCServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetReply, error) {
	gr := toRaftGetRequest(req)
	resp, err := s.node.Get(gr)

	if err != nil {
		return nil, err
	}

	return fromRaftGetReply(resp), nil
}

// Start starts the grpc server on a different go routine
func (s *RPCServer) Start(port string) {
	var opts []grpc.ServerOption
	s.server = grpc.NewServer(opts...)
	pb.RegisterKVStoreRaftServer(s.server, s)

	s.wg.Add(1)
	go func() {
		lis, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatalf("Cannot listen on port %s. Error:%s", port, err.Error())
		}

		s.server.Serve(lis)
		s.wg.Done()
	}()
}

// Stop stops the rpc server
func (s *RPCServer) Stop() {
	s.server.Stop()
	s.wg.Wait()
}

// generateSnapshotFileName converts the request to raft request, and generates snapshot file path and name
func (s *RPCServer) generateSnapshotFileName(req *pb.SnapshotRequest) (*raft.SnapshotRequest, *os.File, error) {
	sr := toRaftSnapshotRequest(req)

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}

	// Generate file name
	sr.File = filepath.Join(cwd, fmt.Sprintf("Node%d_%d_%d_remote.rkvsnapshot", s.node.NodeID(), req.SnapshotIndex, req.SnapshotTerm))

	// Create file
	var f *os.File
	if f, err = os.Create(sr.File); err != nil {
		return nil, nil, err
	}

	return sr, f, nil
}
