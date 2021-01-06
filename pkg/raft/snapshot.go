package raft

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sidecus/raft/pkg/util"
)

const snapshotChunkSize = 8 * 1024

var snapshotPath string
var errorInvalidSnapshotInfo = errors.New("Invalid snapshot index/term")
var errorEmptySnapshot = errors.New("empty snapshot received")
var errorSnapshotFromStaleLeader = errors.New("snapshot received from a stale leader")

// SetSnapshotPath set the snapshot saving path
func SetSnapshotPath(path string) {
	snapshotPath = path
}

// openSnapshot reads a snapshot file and writes to writer
func openSnapshot(file string) (reader io.ReadCloser, err error) {
	return os.Open(file)
}

// createSnapshot creates a snapshot file based on the info provided
// suffix will be appended to the snapshotfile name, can be "remote" when receiving over gRPC and "local" when creating locally
func createSnapshot(nodeID int, term int, index int, suffix string) (file string, writer io.WriteCloser, err error) {
	if term < 0 || index < 0 || suffix == "" {
		return "", nil, errorInvalidSnapshotInfo
	}

	fileName := fmt.Sprintf("Node%d_T%dL%d_%s.rkvsnapshot", nodeID, term, index, suffix)
	fullpath := filepath.Join(snapshotPath, fileName)
	f, err := os.Create(fullpath)
	return fullpath, f, err
}

// deleteSnapshot deletes a snapshot file
func deleteSnapshot(file string) error {
	if file != "" {
		return os.Remove(file)
	}
	return nil
}

// ReceiveSnapshot receives a snapshot and write it to file
func ReceiveSnapshot(nodeID int, reader *SnapshotStreamReader) (req *SnapshotRequest, err error) {
	req = reader.RequestHeader()
	snapshotTerm := req.SnapshotTerm
	snapshotIndex := req.SnapshotIndex

	var file string
	var w io.WriteCloser
	if file, w, err = createSnapshot(nodeID, snapshotTerm, snapshotIndex, "remote"); err != nil {
		return
	}
	defer w.Close()

	// Copy to the file
	if _, err = io.Copy(w, reader); err != nil {
		return
	}

	// Set snapshot file name onto a copy of req and return it
	temp := *req
	temp.File = file
	req = &temp
	return
}

// SendSnapshot sends snapshot over the writer
func SendSnapshot(file string, writer *SnapshotStreamWriter) error {
	reader, err := openSnapshot(file)
	if err != nil {
		return err
	}
	defer reader.Close()

	_, err = io.Copy(writer, reader)
	return err
}

type recvFunc func() (*SnapshotRequest, []byte, error)
type sendFunc func(*SnapshotRequest, []byte) error
type partCallback func(part *SnapshotRequest) bool

// SnapshotStreamReader implements reader interface for reading snapshot messages
type SnapshotStreamReader struct {
	req     *SnapshotRequest
	recv    recvFunc
	partcb  partCallback
	buf     []byte
	readPtr int
}

// NewSnapshotStreamReader creates a new SnapshotStreamReader
func NewSnapshotStreamReader(recv recvFunc, partcb partCallback) (*SnapshotStreamReader, error) {
	// Do the first read to get snapshotTerm and snapshotIndex
	req, data, err := recv()
	if err == io.EOF {
		err = errorEmptySnapshot
	}
	if err != nil {
		return nil, err
	}

	if !partcb(req) {
		return nil, errorSnapshotFromStaleLeader
	}

	return &SnapshotStreamReader{
		req:    req,
		recv:   recv,
		partcb: partcb,
		buf:    data,
	}, nil
}

// RequestHeader returns the snapshot request header
func (reader *SnapshotStreamReader) RequestHeader() *SnapshotRequest {
	return reader.req
}

// Read implements io.Reader to read snapshot messages from a source
func (reader *SnapshotStreamReader) Read(p []byte) (n int, err error) {
	if reader.readPtr == len(reader.buf) {
		// No more data in buf, do another read
		req, data, err := reader.recv()
		if err != nil {
			return 0, err
		}

		if req.SnapshotTerm != reader.req.SnapshotTerm || req.SnapshotIndex != reader.req.SnapshotIndex {
			util.Panicln("snapshot stream message has different headers than former")
		}

		if !reader.partcb(req) {
			return 0, errorSnapshotFromStaleLeader
		}

		reader.buf = data
		reader.readPtr = 0
	}

	n = copy(p, reader.buf[reader.readPtr:])
	reader.readPtr += n

	return n, nil
}

// SnapshotStreamWriter implements a writer interface for sending snapshot messages
type SnapshotStreamWriter struct {
	req  *SnapshotRequest
	send sendFunc
}

// NewSnapshotStreamWriter creates a new gRPCSnapshotStreamWriter
func NewSnapshotStreamWriter(req *SnapshotRequest, send sendFunc) *SnapshotStreamWriter {
	return &SnapshotStreamWriter{
		req:  req,
		send: send,
	}
}

// Write implements io.Writer to send snapshot data over grpc stream
func (writer *SnapshotStreamWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	err = writer.send(writer.req, p)
	return
}
