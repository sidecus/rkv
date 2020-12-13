package raft

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sidecus/raft/pkg/util"
)

func TestSetSnapshotPath(t *testing.T) {
	tempDir := setSnapshotPathToTempDir()
	if snapshotPath != tempDir {
		t.Error("SetSnapshotPath to temp dir failed")
	}
}

func TestSendSnapshot(t *testing.T) {
	path := setSnapshotPathToTempDir()
	file := filepath.Join(path, "TestSendSnapshot.rkvsnapshot")

	filler := byte(6)
	n := createTestSnapshot(file, filler)

	req := &SnapshotRequest{File: file}
	bytesSent := 0
	err := SendSnapshot(req, func(data []byte) error {
		bytesSent += len(data)
		for i := 0; i < len(data); i++ {
			if data[i] != filler {
				return errors.New("Saved data is different from raw")
			}
		}

		return nil
	})

	if err != nil {
		t.Error(err)
	}

	if bytesSent != n {
		t.Error("Sent bytes is different from file size")
	}

	// cleanup
	os.Remove(file)
}

func TestReceiveSnapshot(t *testing.T) {
	setSnapshotPathToTempDir()

	filler := byte(8)
	buf := createTestData(filler)
	nextIndex := 0
	req := &SnapshotRequest{
		SnapshotIndex: 5,
		SnapshotTerm:  20,
	}

	recvFunc := func() (*SnapshotRequest, []byte, error) {
		if nextIndex >= len(buf) {
			return nil, nil, io.EOF
		}

		start := nextIndex
		end := util.Min(nextIndex+snapshotChunkSize, len(buf))
		nextIndex = end
		return req, buf[start:end], nil
	}

	r, err := ReceiveSnapshot(1, recvFunc)
	if err != nil {
		t.Error(err)
	}

	if !strings.HasSuffix(r.File, "Node1_20_5_remote.rkvsnapshot") {
		t.Error("Wrong snapshot file created")
	}

	f, _ := os.Open(r.File)
	totalBytes := 0
	for {
		n, err := f.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			t.Error("Cannot read received snapshot file")
			break
		}

		for i := 0; i < n; i++ {
			if buf[i] != filler {
				t.Fatal("Received snapshot file doesn't contain expected data")
			}
		}

		totalBytes += n
	}

	if totalBytes != len(buf) {
		t.Error("Received snapshot doesn't have correct data length")
	}

	f.Close()
	os.Remove(r.File)
}

func createTestData(filler byte) []byte {
	dataSize := snapshotChunkSize * 3 / 2
	buf := make([]byte, dataSize)
	for i := 0; i < len(buf); i++ {
		buf[i] = filler
	}

	return buf
}

func createTestSnapshot(file string, filler byte) int {
	f, err := os.Create(file)
	if err != nil {
		util.Panicln(err)
	}

	defer f.Close()

	buf := createTestData(filler)
	n, err := f.Write(buf)
	if err != nil {
		util.Panicln(err)
	}

	if n != len(buf) {
		util.Panicln("Failed to create test snapshot with intended size")
	}

	return len(buf)
}

func setSnapshotPathToTempDir() string {
	tempDir := os.TempDir()
	SetSnapshotPath(tempDir)

	return tempDir
}
