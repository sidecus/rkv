package raft

import (
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

func TestReadSnapshot(t *testing.T) {
	path := setSnapshotPathToTempDir()
	file := filepath.Join(path, "TestSendSnapshot.rkvsnapshot")

	filler := byte(6)
	n := createTestSnapshot(file, filler)

	reader, err := ReadSnapshot(file)
	if err != nil {
		t.Error("ReadSnapshot cannot open the snapshot file")
	}
	defer reader.Close()

	bytesRead := 0
	buffer := make([]byte, 1024)
	for {
		bytes, err := reader.Read(buffer)
		if err != nil {
			if err != io.EOF {
				t.Error("Failed reading from snapshot")
				return
			}
			break
		}

		same := true
		for i := 0; i < bytes; i++ {
			if buffer[i] != filler {
				t.Error("Content read from snapshot is not expected")
				same = false
			}
		}
		if !same {
			break
		}

		bytesRead += bytes
	}

	if bytesRead != n {
		t.Error("Sent bytes is different from file size")
	}

	// cleanup
	reader.Close()
	os.Remove(file)
}

func TestCreateSnapshot(t *testing.T) {
	setSnapshotPathToTempDir()

	file, w, err := CreateSnapshot(1, 20, 5, "remote")

	if err != nil {
		t.Error("CreateSnapshot failed" + err.Error())
		return
	}

	if !strings.HasSuffix(file, "Node1_20_5_remote.rkvsnapshot") {
		t.Error("Wrong snapshot file created")
	}

	if w == nil {
		t.Error("CreateSnapshot failed creating snapshot writer")
	}

	data := []byte{1, 2, 3}
	n, err := w.Write(data)
	if err != nil || n == 0 {
		t.Error("CreateSnapshot returned writer doesn't work")
	}

	w.Close()
	os.Remove(file)
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
