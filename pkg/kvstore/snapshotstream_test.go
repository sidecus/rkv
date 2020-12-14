package kvstore

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
	"google.golang.org/grpc/metadata"
)

// moke gRPC server stream (both ServerStream and ClientStream)
type mockGRPCStream struct {
	recvPtr      int
	recvMessages []interface{}
	sendMessages []interface{}
	sendPtr      int
	closed       bool
}

func (x *mockGRPCStream) Header() (metadata.MD, error) {
	return make(map[string][]string), nil
}
func (x *mockGRPCStream) SetHeader(metadata.MD) error {
	return nil
}
func (x *mockGRPCStream) SendHeader(metadata.MD) error {
	return nil
}
func (x *mockGRPCStream) Trailer() metadata.MD {
	return make(map[string][]string)
}
func (x *mockGRPCStream) SetTrailer(metadata.MD) {
}
func (x *mockGRPCStream) Context() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	return ctx
}
func (x *mockGRPCStream) CloseSend() error {
	x.closed = true
	return nil
}
func (x *mockGRPCStream) SendMsg(m interface{}) error {
	if x.sendPtr == len(x.sendMessages) {
		return errors.New("Cannot send more")
	}
	x.sendMessages[x.sendPtr] = m
	x.sendPtr++
	return nil
}
func (x *mockGRPCStream) RecvMsg(m interface{}) error {
	if x.recvPtr == len(x.recvMessages) {
		return io.EOF
	} else if x.recvPtr > len(x.recvMessages) {
		return errors.New("Cannot receive non existent message")
	}
	target := x.recvMessages[x.recvPtr].(*pb.SnapshotRequest)
	*(m.(*pb.SnapshotRequest)) = pb.SnapshotRequest{
		Term:          target.Term,
		LeaderID:      target.LeaderID,
		SnapshotIndex: target.SnapshotIndex,
		SnapshotTerm:  target.SnapshotTerm,
		Data:          target.Data,
	}

	x.recvPtr++

	return nil
}
func (x *mockGRPCStream) SendAndClose(reply *pb.AppendEntriesReply) error {
	return x.SendMsg(reply)
}
func (x *mockGRPCStream) Recv() (*pb.SnapshotRequest, error) {
	m := new(pb.SnapshotRequest)
	if err := x.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}
func (x *mockGRPCStream) Send(req *pb.SnapshotRequest) error {
	return x.SendMsg(req)
}
func (x *mockGRPCStream) CloseAndRecv() (*pb.AppendEntriesReply, error) {
	return x.recvMessages[0].(*pb.AppendEntriesReply), nil
}

func TestGRPCSnapshotStreamReader(t *testing.T) {
	// Directly return EOF should result in an error
	stream := &mockGRPCStream{}
	reader, err := newGRPCSnapshotStreamReader(stream)
	if err != errorEmptySnapshot || reader != nil {
		t.Error("reader doesn't return expected error on empty stream")
	}

	// good flow
	stream.recvMessages = make([]interface{}, 5)
	totalExpectedBytes := 0
	for i := 0; i < len(stream.recvMessages); i++ {
		size := 10 * i
		totalExpectedBytes += size
		stream.recvMessages[i] = &pb.SnapshotRequest{
			Term:          10,
			LeaderID:      11,
			SnapshotIndex: 100,
			SnapshotTerm:  50,
			Data:          make([]byte, size),
		}
	}
	reader, err = newGRPCSnapshotStreamReader(stream)
	if err != nil {
		t.Error("newGRPCSnapshotStreamReader failed")
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
	stream.recvPtr = 0
	reader, err = newGRPCSnapshotStreamReader(stream)
	if err != nil {
		t.Error("newGRPCSnapshotStreamReader failed")
	}
	if reader.req.Term != 10 || reader.req.LeaderID != 11 || reader.req.SnapshotIndex != 100 || reader.req.SnapshotTerm != 50 {
		t.Error("Reader didn't read the snapshot header info correctly")
	}
	stream.recvPtr = len(stream.recvMessages) + 2
	_, err = reader.Read(buf)
	if err == nil {
		t.Error("Reader eats error from RPC stream")
	}
}

func TestStreamWriter(t *testing.T) {
	totalMessages := 5

	stream := &mockGRPCStream{
		sendMessages: make([]interface{}, totalMessages),
	}

	// normal scenario
	payloads := make([][]byte, totalMessages)
	totalExpectedWritten := 0
	for i := 0; i < len(payloads); i++ {
		size := (i + 1) * 3
		payloads[i] = make([]byte, size)
		totalExpectedWritten += size
	}
	req := &raft.SnapshotRequest{
		Term:          1,
		LeaderID:      2,
		SnapshotTerm:  3,
		SnapshotIndex: 4,
	}

	writer := newGRPCSnapshotStreamWriter(req, stream)
	totalWritten := 0
	for i := 0; i < len(payloads); i++ {
		n, err := writer.Write(payloads[i])
		if err != nil {
			t.Error("Error writing to stream writer:" + err.Error())
		}
		totalWritten += n
	}
	for i := 0; i < len(stream.sendMessages); i++ {
		msgSent := stream.sendMessages[i].(*pb.SnapshotRequest)
		if msgSent.Term != 1 || msgSent.LeaderID != 2 || msgSent.SnapshotTerm != 3 || msgSent.SnapshotIndex != 4 {
			t.Error("Wrong message sent")
		}
		if len(msgSent.Data) != len(payloads[i]) {
			t.Error("Wrong payload sent")
		}
	}

	// error scenario
	_, err := writer.Write([]byte{})
	if err == nil {
		t.Error("Stream writer eats error from RPC stream")
	}
}
