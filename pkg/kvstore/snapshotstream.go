package kvstore

import (
	"errors"
	"io"

	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
	"github.com/sidecus/raft/pkg/util"
)

const snapshotChunkSize = 8 * 1024

var errorEmptySnapshot = errors.New("empty snapshot received")

// gRPCSnapshotStreamReader implements grpc snapshot reading and provides a reader interface
type gRPCSnapshotStreamReader struct {
	req     *raft.SnapshotRequest
	stream  pb.KVStoreRaft_InstallSnapshotServer
	buf     []byte
	readPtr int
}

// newGRPCSnapshotStreamReader creates a new gRPCSnapshotStreamReader
func newGRPCSnapshotStreamReader(stream pb.KVStoreRaft_InstallSnapshotServer) (*gRPCSnapshotStreamReader, error) {
	// Do the first read to get snapshotTerm and snapshotIndex
	req, err := stream.Recv()
	if err == io.EOF {
		err = errorEmptySnapshot
	}
	if err != nil {
		return nil, err
	}

	sr := toRaftSnapshotRequest(req)
	return &gRPCSnapshotStreamReader{
		req:    sr,
		stream: stream,
		buf:    req.Data,
	}, nil
}

// Read implements io.Reader to read snapshot data from grpc stream
func (reader *gRPCSnapshotStreamReader) Read(p []byte) (n int, err error) {
	if reader.readPtr == len(reader.buf) {
		// No more data in buf, do another read
		req, err := reader.stream.Recv()
		if err != nil {
			return 0, err
		}

		if int(req.SnapshotTerm) != reader.req.SnapshotTerm || int(req.SnapshotIndex) != reader.req.SnapshotIndex {
			util.Panicln("gRPC snapshot stream message has different headers")
		}

		reader.buf = req.Data
		reader.readPtr = 0
	}

	n = copy(p, reader.buf[reader.readPtr:])
	reader.readPtr += n

	return n, nil
}

// gRPCSnapshotStreamWriter provides a writer to send snapshot data over grpc
type gRPCSnapshotStreamWriter struct {
	req    *raft.SnapshotRequest
	stream pb.KVStoreRaft_InstallSnapshotClient
}

// newGRPCSnapshotStreamWriter creates a new gRPCSnapshotStreamWriter
func newGRPCSnapshotStreamWriter(req *raft.SnapshotRequest, stream pb.KVStoreRaft_InstallSnapshotClient) *gRPCSnapshotStreamWriter {
	return &gRPCSnapshotStreamWriter{
		req:    req,
		stream: stream,
	}
}

// Write implements io.Writer to send snapshot data over grpc stream
func (writer *gRPCSnapshotStreamWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	sr := fromRaftSnapshotRequest(writer.req)
	sr.Data = p

	err = writer.stream.Send(sr)
	return
}
