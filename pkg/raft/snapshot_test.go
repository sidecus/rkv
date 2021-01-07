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

func TestOpenSnapshot(t *testing.T) {
	path := setSnapshotPathToTempDir()
	file := filepath.Join(path, "TestSendSnapshot.rkvsnapshot")

	filler := byte(6)
	createTestSnapshot(file, filler)
	defer deleteSnapshot(file)

	reader, err := openSnapshot(file)
	if err != nil {
		t.Error("openSnapshot cannot open the snapshot file")
	}
	defer reader.Close()

	buffer := make([]byte, 1024)
	bytes, err := reader.Read(buffer)
	if err != nil {
		if err != io.EOF {
			t.Error("Failed reading from snapshot")
			return
		}
	}

	for i := 0; i < bytes; i++ {
		if buffer[i] != filler {
			t.Fatal("Content read from snapshot is not expected")
		}
	}
}

func TestCreateSnapshot(t *testing.T) {
	setSnapshotPathToTempDir()

	file, w, err := createSnapshot(1, 20, 5, "remote")
	defer deleteSnapshot(file)
	defer w.Close()

	if err != nil {
		t.Error("createSnapshot failed" + err.Error())
		return
	}

	if !strings.HasSuffix(file, "Node1_T20L5_remote.rkvsnapshot") {
		t.Error("Wrong snapshot file created")
	}

	if w == nil {
		t.Error("createSnapshot failed creating snapshot writer")
	}

	data := []byte{1, 2, 3}
	n, err := w.Write(data)
	if err != nil || n == 0 {
		t.Error("createSnapshot returned writer doesn't work")
	}
}

func TestReceiveSnapshot(t *testing.T) {
	setSnapshotPathToTempDir()

	filler := byte(2)
	partcb := func(*SnapshotRequestHeader) bool { return true }
	recv, n := createTestRecvFunc(filler)
	reader, _ := NewSnapshotStreamReader(recv, partcb)

	req, err := ReceiveSnapshot(3, reader)
	if err != nil {
		t.Error("Receive snapshot failed")
	}
	defer deleteSnapshot(req.File)

	if req.SnapshotRequestHeader != *reader.header {
		t.Error("Received incorrect snapshot header")
	}

	err = validateSnapshotFileContent(req.File, n, filler)
	if err != nil {
		t.Error(err)
	}
}

func TestSendSnapshot(t *testing.T) {
	path := setSnapshotPathToTempDir()
	file := filepath.Join(path, "TestSendSnapshot.rkvsnapshot")
	filler := byte(5)
	n := createTestSnapshot(file, filler)
	defer deleteSnapshot(file)

	req := &SnapshotRequestHeader{
		LeaderID:      5,
		SnapshotTerm:  4,
		SnapshotIndex: 3,
	}
	var result []byte
	writer := NewSnapshotStreamWriter(req, func(r *SnapshotRequestHeader, data []byte) error {
		if r.LeaderID != req.LeaderID ||
			r.SnapshotTerm != req.SnapshotTerm ||
			r.SnapshotIndex != req.SnapshotIndex {
			t.Fatal("Wrong request header used when sending")
		}
		result = append(result, data...)
		return nil
	})

	if err := SendSnapshot(file, writer); err != nil {
		t.Error("Error sending snapshot")
	}
	if len(result) != n {
		t.Error("Wrong number of bytes sent when sending snapshot")
	}
	for i := 0; i < n; i++ {
		if result[i] != filler {
			t.Fatal("Incorrect data sent")
		}
	}
}

func TestSnapshotStreamReader(t *testing.T) {
	var callbackheader *SnapshotRequestHeader
	partCallback := func(part *SnapshotRequestHeader) bool {
		callbackheader = part
		return true
	}

	// Directly return EOF should result in an error
	reader, err := NewSnapshotStreamReader(func() (*SnapshotRequestHeader, []byte, error) { return nil, nil, io.EOF }, partCallback)
	if err != errorEmptySnapshot || reader != nil || callbackheader != nil {
		t.Error("reader doesn't return expected error on empty stream")
	}

	// prepare test data
	header := &SnapshotRequestHeader{
		Term:          10,
		LeaderID:      11,
		SnapshotIndex: 100,
		SnapshotTerm:  50,
	}
	data := make([][]byte, 5)
	totalExpectedBytes := 0
	for i := 0; i < len(data); i++ {
		size := 10 * i
		totalExpectedBytes += size
		data[i] = make([]byte, size)
	}
	curMsg := 0
	recvFunc := func() (*SnapshotRequestHeader, []byte, error) {
		if curMsg == len(data) {
			return nil, nil, io.EOF
		} else if curMsg > len(data) {
			return nil, nil, errors.New("Artificial error")
		}

		p := data[curMsg]
		curMsg++
		return header, p, nil
	}

	// good flow
	reader, err = NewSnapshotStreamReader(recvFunc, partCallback)
	if err != nil {
		t.Error("SnapshotStreamReader failed")
	}
	if *reader.header != *header {
		t.Error("Reader didn't read the snapshot header info correctly")
	}

	totalReadBytes := 0
	result := make([]byte, 20)
	for {
		n, err := reader.Read(result)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Error("snapshot stream read error:" + err.Error())
		}
		if *callbackheader != *header {
			t.Error("Reader didn't invoke callback with the right info")
		}
		totalReadBytes += n
	}
	if totalReadBytes != totalExpectedBytes {
		t.Error("snapshot stream didn't read correct number of bytes")
	}

	// error flow - returns error when recvFunc fails
	curMsg = len(data) + 1
	if _, err = reader.Read(result); err == nil {
		t.Error("Reader eats error from RPC stream")
	}

	// error flow - returns error when partCallback returns false (mimicing part has lower term scenario)
	curMsg = 0
	errCallback := func(*SnapshotRequestHeader) bool { return curMsg <= 1 }
	reader, _ = NewSnapshotStreamReader(recvFunc, errCallback)
	if _, err = reader.Read(result); err == nil {
		t.Error("Reader should return error if partCallback returns false")
	}
}

func TestGRPCSnapshotStreamWriter(t *testing.T) {
	// test data
	totalMessages := 5
	payloads := make([][]byte, totalMessages)
	totalExpectedWritten := 0
	for i := 0; i < len(payloads); i++ {
		size := (i + 1) * 3
		payloads[i] = make([]byte, size)
		totalExpectedWritten += size
	}
	req := &SnapshotRequestHeader{
		Term:          1,
		LeaderID:      2,
		SnapshotTerm:  3,
		SnapshotIndex: 4,
	}

	// positive flow
	var msgSent *SnapshotRequestHeader
	var dataSent []byte
	writer := NewSnapshotStreamWriter(req, func(msg *SnapshotRequestHeader, data []byte) error {
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
	writer = NewSnapshotStreamWriter(req, func(msg *SnapshotRequestHeader, data []byte) error {
		return errors.New("artifical write error")
	})
	if _, err := writer.Write([]byte{}); err == nil {
		t.Error("Stream writer eats error from RPC stream")
	}
}

func createTestData(filler byte) []byte {
	dataSize := snapshotChunkSize * 3 / 2
	buf := make([]byte, dataSize)
	for i := 0; i < len(buf); i++ {
		buf[i] = filler
	}

	return buf
}

func validateSnapshotFileContent(f string, n int, filler byte) error {
	r, _ := openSnapshot(f)
	defer r.Close()

	buf := make([]byte, 100)
	total := 0
	for {
		bytes, err := r.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		for i := 0; i < bytes; i++ {
			if buf[i] != filler {
				return errors.New("Incorrecte data read from snapshot file")
			}
		}
		total += bytes
	}
	if total != n {
		return errors.New("Incorrect nubmer of bytes read from snapshot file")
	}
	return nil
}

func createTestRecvFunc(filler byte) (recvFunc, int) {
	i := 0
	testData := createTestData(filler)
	return func() (*SnapshotRequestHeader, []byte, error) {
		i++
		if i == 1 {
			return &SnapshotRequestHeader{
				Term:          3,
				SnapshotIndex: 20,
				SnapshotTerm:  2,
			}, testData, nil
		}
		return nil, nil, io.EOF
	}, len(testData)
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
