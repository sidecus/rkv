package rkv

import (
	"bytes"
	"testing"

	"github.com/sidecus/raft/pkg/raft"
)

func TestCmdSet(t *testing.T) {
	store := newRKVStore()

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "a",
		},
	})

	if v, _ := store.Get("a"); v != "a" {
		t.Error("Set doesn't set value correctly")
	}

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "A",
		},
	})

	if v, _ := store.Get("a"); v != "A" {
		t.Error("Set doesn't set value correctly upon existing entry")
	}
}

func TestCmdDel(t *testing.T) {
	store := newRKVStore()

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

func TestSerilization(t *testing.T) {
	store := newRKVStore()

	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "a",
			Value: "a",
		},
	})
	store.Apply(raft.StateMachineCmd{
		CmdType: KVCmdSet,
		Data: KVCmdData{
			Key:   "ab",
			Value: "ab",
		},
	})

	buf := &bytes.Buffer{}

	if err := store.Serialize(buf); err != nil {
		t.Fatalf("TakeSnapshot returned error %s", err)
		return
	}

	newStore := newRKVStore()
	if err := newStore.Deserialize(buf); err != nil {
		t.Errorf("InstallSnapshot returned error %s", err)
	}

	if v, err := newStore.Get("a"); err != nil || v.(string) != "a" {
		t.Error("InstallSnapshot returns different data")
	}

	if v, err := newStore.Get("ab"); err != nil || v.(string) != "ab" {
		t.Error("InstallSnapshot returns different data")
	}
}
