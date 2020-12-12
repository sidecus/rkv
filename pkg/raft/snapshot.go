package raft

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errorInvalidSnapshotInfo = errors.New("Invalid snapshot index/term")

// snapshot path, will be set by node initialization
var snapshotPath string

// SetSnapshotPath set the snapshot saving path
func SetSnapshotPath(path string) {
	snapshotPath = path
}

// CreateLocalSnapshotFile creates a snapshot file for write. It should be called when the node needs to take a snapshot
func CreateLocalSnapshotFile(nodeID int, snapshotTerm int, snapshotIndex int) (string, io.WriteCloser, error) {
	return createSnapshotFile(nodeID, snapshotTerm, snapshotIndex, "local")
}

// CreateRemoteSnapshotFile creates a snapshot file for write. It should be called when receiving a new snapshot
func CreateRemoteSnapshotFile(nodeID int, snapshotTerm int, snapshotIndex int) (string, io.WriteCloser, error) {
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

// OpenSnapshotFile opens the snapshot file and returns a closable reader
func OpenSnapshotFile(snapshotFile string) (io.ReadCloser, error) {
	return os.Open(snapshotFile)
}
