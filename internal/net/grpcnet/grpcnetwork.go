package grpcnet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sidecus/raft/pkg/network"
	"google.golang.org/grpc"

	"github.com/sidecus/raft/internal/net/grpcnet/pb"
)

const rpcTimeoutMS = 200

var errorInvalidNodeEndpoint = errors.New("Node endpoint doesn't have port specified")
var errorNodeIDNotInEndpoints = errors.New("Current node id doesn't exist in the endpoints array")

// NodeEndpoint represents a raft node with grpc capability
type NodeEndpoint struct {
	NodeID   int
	Endpoint string
}

// grpcNetwork is a grpc based INetwork implementation
type grpcNetwork struct {
	nodeID int
	port   string
	cRecv  chan *network.Message
	peers  map[int]*peerNode // grpc clients to other nodes

	logger *log.Logger
}

// peerNode wraps info for a raft peer node
type peerNode struct {
	endpoint   NodeEndpoint
	grpcClient pb.RafterClient
}

// NewGRPCNetwork creates a grpc based raft network for a node to consume
// It starts the grpc server on a separate go routine and returns immediately
func NewGRPCNetwork(selfNodeID int, endpoints []NodeEndpoint, logger *log.Logger) (network.INetwork, error) {
	var lisEndPoint string
	for i := range endpoints {
		if endpoints[i].NodeID == selfNodeID {
			lisEndPoint = endpoints[i].Endpoint
		}
	}
	if lisEndPoint == "" {
		return nil, errorNodeIDNotInEndpoints
	}

	port, err := getPort(lisEndPoint)
	if err != nil {
		return nil, err
	}

	cRecv := make(chan *network.Message, 100)
	peers, err := createRaftPeers(selfNodeID, endpoints)
	if err != nil {
		return nil, err
	}

	nw := &grpcNetwork{
		nodeID: selfNodeID,
		port:   port,
		cRecv:  cRecv,
		peers:  peers,
		logger: logger,
	}

	return nw, nil
}

// createRaftPeers creates the grpc clients for our raft peers
func createRaftPeers(selfNodeID int, endpoints []NodeEndpoint) (map[int]*peerNode, error) {
	peers := make(map[int]*peerNode)

	for _, ep := range endpoints {
		if ep.NodeID == selfNodeID {
			// only create rpc client for peer nodes, never for self
			continue
		}

		conn, err := grpc.Dial(ep.Endpoint, grpc.WithInsecure())
		if err != nil {
			return nil, err
		}
		client := pb.NewRafterClient((conn))

		peer := &peerNode{
			endpoint:   ep,
			grpcClient: client,
		}

		peers[ep.NodeID] = peer
	}

	return peers, nil
}

func getPort(url string) (string, error) {
	parts := strings.SplitN(url, ":", 2)
	if len(parts) < 2 {
		return "", errorInvalidNodeEndpoint
	}

	port := parts[1]
	_, err := strconv.Atoi(port)
	return port, err
}

// Start starts the grpc network (to be more specific, grpc server)
func (nw *grpcNetwork) Start() {
	go func() {
		var opts []grpc.ServerOption
		s := grpc.NewServer(opts...)

		server := &rafterServer{ch: nw.cRecv}
		pb.RegisterRafterServer(s, server)

		lis, err := net.Listen("tcp", ":"+nw.port)
		if err != nil {
			panic(fmt.Sprintf("Cannot listen on port %s for Node%d. Error:%s", nw.port, nw.nodeID, err.Error()))
		}

		s.Serve(lis)
	}()
}

// Send sends a message from source node to target node
// Note this should be "non-blocking", aka don't wait for the result to return
// Requests like AppendEntry will have to be synchronous
func (nw *grpcNetwork) Send(sourceNodeID int, targetNodeID int, msg *network.Message) error {
	if sourceNodeID != nw.nodeID {
		return network.ErrorInvalidNodeID
	}

	peer, ok := nw.peers[targetNodeID]
	if !ok {
		return network.ErrorInvalidNodeID
	}

	// nw.logger.Printf("T%d: Node%d <- Node%d (%s)", msg.Term, targetNodeID, sourceNodeID, msg.MsgType)

	// non blocking send on a separate goroutine, reply is ignored
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeoutMS*time.Millisecond)
		defer cancel()

		_, err := peer.grpcClient.SendMessage(ctx, &pb.RaftMessage{
			NodeID:  int64(msg.NodeID),
			Term:    int64(msg.Term),
			MsgType: string(msg.MsgType),
			Data:    int64(msg.Data),
		})

		if err != nil {
			nw.logger.Printf("T%d: Node%d <- Node%d (%s) failed. Error:%s\n", msg.Term, targetNodeID, sourceNodeID, msg.MsgType, err.Error())
		}
	}()

	return nil
}

// Broadcast broadcasts a message from source node to all other nodes
func (nw *grpcNetwork) Broadcast(sourceNodeID int, msg *network.Message) error {
	for i := range nw.peers {
		nw.Send(sourceNodeID, i, msg)
	}

	return nil
}

// GetRecvChannel returns the message receiving channel for current node
func (nw *grpcNetwork) GetRecvChannel(nodeID int) (chan *network.Message, error) {
	if nodeID != nw.nodeID {
		return nil, network.ErrorInvalidNodeID
	}

	return nw.cRecv, nil
}
