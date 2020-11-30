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

	if v, _ := store.getValue("a"); v != "a" {
		t.Error("Set doesn't set value correctly")
	}

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "A",
		},
	})

	if v, _ := store.getValue("a"); v != "A" {
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

	if _, err := store.Get("a"); err == nil {
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

	if v, err := store.Get("a"); err != nil || v.(string) != "a" {
		t.Error("Del deletes wrong entry")
	}
}
