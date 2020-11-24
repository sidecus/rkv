package grpcnet

import (
	"context"

	"github.com/sidecus/raft/internal/net/grpcnet/pb"
	"github.com/sidecus/raft/pkg/network"
)

// rafterServer is used to implement RafterServer
type rafterServer struct {
	ch chan *network.Message
	pb.UnimplementedRafterServer
}

// SendMessage implements RafterServer.SendMessage
func (s *rafterServer) SendMessage(ctx context.Context, msg *pb.RaftMessage) (*pb.RaftReply, error) {
	message := &network.Message{
		NodeID:  int(msg.NodeID),
		Term:    int(msg.Term),
		MsgType: network.MessageType(msg.MsgType),
		Data:    int(msg.Data),
	}

	// Just send the message to the channel
	s.ch <- message

	return &pb.RaftReply{}, nil
}
