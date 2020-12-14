package raft

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const snapshotChunkSize = 8 * 1024

var snapshotPath string
var errorInvalidSnapshotInfo = errors.New("Invalid snapshot index/term")
var errorEmptySnapshot = errors.New("empty snapshot received")

// SetSnapshotPath set the snapshot saving path
func SetSnapshotPath(path string) {
	snapshotPath = path
}

// ReadSnapshot reads a snapshot file and writes to writer
func ReadSnapshot(file string) (reader io.ReadCloser, err error) {
	return os.Open(file)
}

// CreateSnapshot creates a snapshot file and writes info to it from the reader
// suffix will be appended to the snapshotfile name, can be "remote" when receiving over gRPC and "local" when creating locally
func CreateSnapshot(nodeID int, term int, index int, suffix string) (file string, writer io.WriteCloser, err error) {
	if term < 0 || index < 0 || suffix == "" {
		return "", nil, errorInvalidSnapshotInfo
	}

	fileName := fmt.Sprintf("Node%d_%d_%d_%s.rkvsnapshot", nodeID, term, index, suffix)
	fullpath := filepath.Join(snapshotPath, fileName)
	f, err := os.Create(fullpath)
	return fullpath, f, err
}
