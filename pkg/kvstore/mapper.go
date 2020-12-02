package kvstore

import (
	"github.com/sidecus/raft/pkg/kvstore/pb"
	"github.com/sidecus/raft/pkg/raft"
	"github.com/sidecus/raft/pkg/util"
)

// TODO[sidecus]: use automapper?

func toRaftAERequest(req *pb.AppendEntriesRequest) *raft.AppendEntriesRequest {
	entries := make([]raft.LogEntry, len(req.Entries))
	for i, v := range req.Entries {
		cmdData := KVCmdData{
			Key:   v.Cmd.Data.Key,
			Value: v.Cmd.Data.Value,
		}

		cmd := raft.StateMachineCmd{
			CmdType: int(v.Cmd.CmdType),
			Data:    cmdData,
		}

		entries[i] = raft.LogEntry{
			Index: int(v.Index),
			Term:  int(v.Term),
			Cmd:   cmd,
		}
	}

	ae := &raft.AppendEntriesRequest{
		Term:         int(req.Term),
		LeaderID:     int(req.LeaderId),
		PrevLogIndex: int(req.PrevLogIndex),
		PrevLogTerm:  int(req.PrevLogTerm),
		LeaderCommit: int(req.LeaderCommit),
		Entries:      entries,
	}

	return ae
}

func fromRaftAERequest(req *raft.AppendEntriesRequest) *pb.AppendEntriesRequest {
	entries := make([]*pb.LogEntry, len(req.Entries))
	for i, v := range req.Entries {
		cmd := &pb.KVCmd{
			CmdType: int32(v.Cmd.CmdType),
			Data: &pb.KVCmdData{
				Key:   v.Cmd.Data.(KVCmdData).Key,
				Value: v.Cmd.Data.(KVCmdData).Value,
			},
		}
		entry := &pb.LogEntry{
			Index: int64(v.Index),
			Term:  int64(v.Term),
			Cmd:   cmd,
		}

		entries[i] = entry
	}

	ae := &pb.AppendEntriesRequest{
		Term:         int64(req.Term),
		LeaderId:     int64(req.LeaderID),
		PrevLogIndex: int64(req.PrevLogIndex),
		PrevLogTerm:  int64(req.PrevLogTerm),
		LeaderCommit: int64(req.LeaderCommit),
		Entries:      entries,
	}

	return ae
}

func toRaftAEReply(resp *pb.AppendEntriesReply) *raft.AppendEntriesReply {
	return &raft.AppendEntriesReply{
		NodeID:    int(resp.NodeID),
		LeaderID:  int(resp.LeaderID),
		Term:      int(resp.Term),
		Success:   resp.Success,
		LastMatch: int(resp.LastMatch),
	}
}

func fromRaftAEReply(resp *raft.AppendEntriesReply) *pb.AppendEntriesReply {
	return &pb.AppendEntriesReply{
		Term:      int64(resp.Term),
		NodeID:    int64(resp.NodeID),
		LeaderID:  int64(resp.LeaderID),
		Success:   resp.Success,
		LastMatch: int64(resp.LastMatch),
	}
}

func toRaftRVRequest(req *pb.RequestVoteRequest) *raft.RequestVoteRequest {
	rv := &raft.RequestVoteRequest{
		Term:         int(req.Term),
		CandidateID:  int(req.CandidateId),
		LastLogIndex: int(req.LastLogIndex),
		LastLogTerm:  int(req.LastLogTerm),
	}

	return rv
}

func fromRaftRVRequest(req *raft.RequestVoteRequest) *pb.RequestVoteRequest {
	rv := &pb.RequestVoteRequest{
		Term:         int64(req.Term),
		CandidateId:  int64(req.CandidateID),
		LastLogIndex: int64(req.LastLogIndex),
		LastLogTerm:  int64(req.LastLogTerm),
	}

	return rv
}

func toRaftRVReply(resp *pb.RequestVoteReply) *raft.RequestVoteReply {
	return &raft.RequestVoteReply{
		NodeID:      int(resp.NodeID),
		Term:        int(resp.Term),
		VotedTerm:   int(resp.VotedTerm),
		VoteGranted: resp.VoteGranted,
	}
}

func fromRaftRVReply(resp *raft.RequestVoteReply) *pb.RequestVoteReply {
	return &pb.RequestVoteReply{
		NodeID:      int64(resp.NodeID),
		Term:        int64(resp.Term),
		VotedTerm:   int64(resp.VotedTerm),
		VoteGranted: resp.VoteGranted,
	}
}

func toRaftGetRequest(req *pb.GetRequest) *raft.GetRequest {
	key := req.Key
	gr := &raft.GetRequest{Params: []interface{}{key}}

	return gr
}

func fromRaftGetRequest(req *raft.GetRequest) *pb.GetRequest {
	return &pb.GetRequest{
		Key: req.Params[0].(string),
	}
}

func toRaftGetReply(resp *pb.GetReply) *raft.GetReply {
	return &raft.GetReply{
		NodeID: int(resp.NodeID),
		Data:   resp.Value,
	}
}

func fromRaftGetReply(resp *raft.GetReply) *pb.GetReply {
	return &pb.GetReply{
		NodeID:  int64(resp.NodeID),
		Success: true,
		Value:   resp.Data.(string),
	}
}

func toRaftSetRequest(req *pb.SetRequest) *raft.StateMachineCmd {
	cmdData := KVCmdData{
		Key:   req.Key,
		Value: req.Value,
	}

	cmd := &raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data:    cmdData,
	}

	return cmd
}

func fromRaftSetRequest(cmd *raft.StateMachineCmd) *pb.SetRequest {
	if cmd.CmdType != KVCmdSet {
		util.Panicf("invalid cmd type for Set")
	}

	return &pb.SetRequest{
		Key:   cmd.Data.(KVCmdData).Key,
		Value: cmd.Data.(KVCmdData).Value,
	}
}

func toRaftSetReply(resp *pb.SetReply) *raft.ExecuteReply {
	return &raft.ExecuteReply{
		NodeID:  int(resp.NodeID),
		Success: resp.Success,
	}
}

func fromRaftSetReply(resp *raft.ExecuteReply) *pb.SetReply {
	if resp == nil {
		return nil
	}

	return &pb.SetReply{
		NodeID:  int64(resp.NodeID),
		Success: resp.Success,
	}
}

func toRaftDeleteRequest(req *pb.DeleteRequest) *raft.StateMachineCmd {
	cmdData := KVCmdData{
		Key: req.Key,
	}

	cmd := &raft.StateMachineCmd{
		CmdType: KVCmdDel,
		Data:    cmdData,
	}

	return cmd
}

func fromRaftDeleteRequest(cmd *raft.StateMachineCmd) *pb.DeleteRequest {
	return &pb.DeleteRequest{
		Key: cmd.Data.(KVCmdData).Key,
	}
}

func toRaftDeleteReply(resp *pb.DeleteReply) *raft.ExecuteReply {
	if resp == nil {
		return nil
	}

	return &raft.ExecuteReply{
		NodeID:  int(resp.NodeID),
		Success: resp.Success,
	}
}

func fromRaftDeleteReply(resp *raft.ExecuteReply) *pb.DeleteReply {
	return &pb.DeleteReply{
		NodeID:  int64(resp.NodeID),
		Success: resp.Success,
	}
}
