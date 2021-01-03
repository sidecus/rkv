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

func TestGRPCSnapshotStreamReader(t *testing.T) {
	// Directly return EOF should result in an error
	reader, err := NewSnapshotStreamReader(func() (*SnapshotRequest, []byte, error) { return nil, nil, io.EOF })
	if err != errorEmptySnapshot || reader != nil {
		t.Error("reader doesn't return expected error on empty stream")
	}

	// test data
	messages := make([]*SnapshotRequest, 5)
	data := make([][]byte, 5)
	totalExpectedBytes := 0
	for i := 0; i < len(messages); i++ {
		messages[i] = &SnapshotRequest{
			Term:          10,
			LeaderID:      11,
			SnapshotIndex: 100,
			SnapshotTerm:  50,
		}

		size := 10 * i
		totalExpectedBytes += size
		data[i] = make([]byte, size)
	}

	// good flow
	curMsg := 0
	reader, err = NewSnapshotStreamReader(func() (*SnapshotRequest, []byte, error) {
		if curMsg == len(messages) {
			return nil, nil, io.EOF
		} else if curMsg > len(messages) {
			return nil, nil, errors.New("Artificial error")
		}

		msg := messages[curMsg]
		p := data[curMsg]
		curMsg++
		return msg, p, nil
	})
	if err != nil {
		t.Error("SnapshotStreamReader failed")
	}
	if reader.req.Term != 10 || reader.req.LeaderID != 11 || reader.req.SnapshotIndex != 100 || reader.req.SnapshotTerm != 50 {
		t.Error("Reader didn't read the snapshot header info correctly")
	}

	totalReadBytes := 0
	buf := make([]byte, 20)
	for {
		n, err := reader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Error("snapshot stream read error:" + err.Error())
		}
		totalReadBytes += n
	}
	if totalReadBytes != totalExpectedBytes {
		t.Error("snapshot stream didn't read correct number of bytes")
	}

	// error flow
	curMsg = 0
	curMsg = len(messages) + 1
	if _, err = reader.Read(buf); err == nil {
		t.Error("Reader eats error from RPC stream")
	}
}

func TestStreamWriter(t *testing.T) {
	// test data
	totalMessages := 5
	payloads := make([][]byte, totalMessages)
	totalExpectedWritten := 0
	for i := 0; i < len(payloads); i++ {
		size := (i + 1) * 3
		payloads[i] = make([]byte, size)
		totalExpectedWritten += size
	}
	req := &SnapshotRequest{
		Term:          1,
		LeaderID:      2,
		SnapshotTerm:  3,
		SnapshotIndex: 4,
	}

	// positive flow
	var msgSent *SnapshotRequest
	var dataSent []byte
	writer := NewSnapshotStreamWriter(req, func(msg *SnapshotRequest, data []byte) error {
		msgSent = msg
		dataSent = data
		return nil
	})
	totalWritten := 0
	for i := 0; i < len(payloads); i++ {
		n, err := writer.Write(payloads[i])
		if err != nil {
			t.Error("Error writing to stream writer:" + err.Error())
		}
		totalWritten += n

		if msgSent.Term != 1 || msgSent.LeaderID != 2 || msgSent.SnapshotTerm != 3 || msgSent.SnapshotIndex != 4 {
			t.Error("Wrong message sent")
		}
		if len(dataSent) != len(payloads[i]) {
			t.Error("Wrong payload sent")
		}
	}
	if totalWritten != totalExpectedWritten {
		t.Error("Wrong number of bytes sent")
	}

	// error scenario
	writer = NewSnapshotStreamWriter(req, func(msg *SnapshotRequest, data []byte) error {
		return errors.New("artifical write error")
	})
	if _, err := writer.Write([]byte{}); err == nil {
		t.Error("Stream writer eats error from RPC stream")
	}
}
