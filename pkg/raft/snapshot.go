package raft

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const snapshotChunkSize = 8 * 1024

type snapshotRecvFunc func() (*SnapshotRequest, []byte, error)
type snapshotSendFunc func([]byte) error
type snapshotWriteFunc func(w io.Writer) error
type snapshotReadFunc func(w io.Reader) error

var snapshotPath string
var errorInvalidSnapshotInfo = errors.New("Invalid snapshot index/term")
var errorEmptySnapshot = errors.New("empty snapshot received")

// SetSnapshotPath set the snapshot saving path
func SetSnapshotPath(path string) {
	snapshotPath = path
}

// SendSnapshot sends a snapshot file
func SendSnapshot(req *SnapshotRequest, sendFunc func([]byte) error) error {
	var (
		r   io.ReadCloser
		err error
	)

	if r, err = openSnapshotFile(req.File); err != nil {
		return err
	}
	defer r.Close()

	buf := make([]byte, snapshotChunkSize)
	for {
		var n int
		if n, err = r.Read(buf); err != nil {
			if err == io.EOF {
				break // done sending
			}
			return err
		}

		if err = sendFunc(buf[:n]); err != nil {
			return err
		}
	}

	return nil
}

// ReceiveSnapshot receives a snap shot as file and returns a SnapshotRequest with File set to the received file
func ReceiveSnapshot(nodeID int, recvFunc snapshotRecvFunc) (*SnapshotRequest, error) {
	var (
		req      *SnapshotRequest
		w        io.WriteCloser
		err      error
		chunkReq *SnapshotRequest
		data     []byte
	)

	for {
		// Read chunk
		if chunkReq, data, err = recvFunc(); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// remember request and create snapshot file if not yet
		if w == nil {
			req = chunkReq
			if req.File, w, err = createRemoteSnapshotFile(nodeID, req.SnapshotTerm, req.SnapshotIndex); err != nil {
				return nil, err
			}
			defer w.Close()
		}

		// Write data
		if _, err = w.Write(data); err != nil {
			return nil, err
		}
	}

	if req == nil {
		return nil, errorEmptySnapshot
	}

	return req, nil
}

// CreateSnapshot creates a new snapshot
func CreateSnapshot(nodeID int, term int, index int, writeFunc snapshotWriteFunc) (string, error) {
	file, w, err := createLocalSnapshotFile(nodeID, term, index)
	if err != nil {
		return "", err
	}
	defer w.Close()

	// Write to file
	return file, writeFunc(w)
}

// ReadSnapshot reads the snapshot file
func ReadSnapshot(file string, readFunc snapshotReadFunc) error {
	r, err := openSnapshotFile(file)
	if err != nil {
		return err
	}
	defer r.Close()

	return readFunc(r)
}

// openSnapshotFile opens the snapshot file and returns a closable reader
func openSnapshotFile(snapshotFile string) (io.ReadCloser, error) {
	return os.Open(snapshotFile)
}

// createLocalSnapshotFile creates a snapshot file for write. It should be called when the node needs to take a snapshot
func createLocalSnapshotFile(nodeID int, snapshotTerm int, snapshotIndex int) (string, io.WriteCloser, error) {
	return createSnapshotFile(nodeID, snapshotTerm, snapshotIndex, "local")
}

// createRemoteSnapshotFile creates a snapshot file for write. It should be called when receiving a new snapshot
func createRemoteSnapshotFile(nodeID int, snapshotTerm int, snapshotIndex int) (string, io.WriteCloser, error) {
	return createSnapshotFile(nodeID, snapshotTerm, snapshotIndex, "remote")
}

// createSnapshotFile creates a new snapshot file, it returns the full path for the created file, a writercloser for writing
func createSnapshotFile(nodeID int, snapshotTerm int, snapshotIndex int, postfix string) (string, io.WriteCloser, error) {
	if snapshotIndex < 0 || snapshotTerm < 0 {
		return "", nil, errorInvalidSnapshotInfo
	}

	fileName := fmt.Sprintf("Node%d_%d_%d_%s.rkvsnapshot", nodeID, snapshotTerm, snapshotIndex, postfix)
	fullpath := filepath.Join(snapshotPath, fileName)
	f, err := os.Create(fullpath)
	return fullpath, f, err
}
