package kvstore

import (
	"testing"

	"github.com/sidecus/raft/pkg/raft"
)

func TestCmdSet(t *testing.T) {
	store := NewKVStore()

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "a",
		},
	})

	if v, _ := store.GetValue("a"); v != "a" {
		t.Error("Set doesn't set value correctly")
	}

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "A",
		},
	})

	if v, _ := store.GetValue("a"); v != "A" {
		t.Error("Set doesn't set value correctly upon existing entry")
	}
}

func TestCmdDel(t *testing.T) {
	store := NewKVStore()

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "a",
		},
	})
	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdDel,
		Data: KVCmdData{
			Key:   "a",
			Value: "",
		},
	})

	if _, ok := store.GetValue("a"); ok {
		t.Error("Del doesn't delete value correctly")
	}

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "a",
		},
	})
	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdDel,
		Data: KVCmdData{
			Key:   "A",
			Value: "",
		},
	})

	if v, ok := store.GetValue("a"); !ok || v != "a" {
		t.Error("Del deletes wrong entry")
	}
}
