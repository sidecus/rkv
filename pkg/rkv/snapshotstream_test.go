package rkv

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/sidecus/raft/pkg/rkv/pb"
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
