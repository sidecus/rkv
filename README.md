# rkv
Reinventing the wheel - raft based KV store using Go & gRPC (protobuf)

## Intro
A distributed key value store based on Raft consensus algorithm. It supports:
1. Raft election
2. Replication & log shipping
3. Log compaction & snapshots
4. Consensus based writes, with batching to improve throughput
6. Auto follower to leader proxy for write operations (set/del)
5. gRPC based client/server & node/node communication
6. Abstracted raft layer which can potentially be used with other statemachines, or different communication protocols

## Build
```bash
cd cmd/rkv
go build .
cd cmd/rkvclient
go build .
```
This builds two executables, one in each folder: **rkv** (rkv server), **rkvclient** (rkv client)

## Run locally
### Start raft server nodes.
Below cmds start a 3 node cluster on loopback. To watch logs easily, it's better to start each node in a different terminal.
```bash
./rkv -nodeid 0 -addresses localhost:27015,localhost:27016,localhost:27017
./rkv -nodeid 1 -addresses localhost:27015,localhost:27016,localhost:27017
./rkv -nodeid 2 -addresses localhost:27015,localhost:27016,localhost:27017
```
### Run client against any nodes for set/get/del
```bash
./rkvclient set -address localhost:27015 -key somekey0 -value v0
./rkvclient set -address localhost:27016 -key sk1 -value v1
./rkvclient set -address localhost:27016 -key sk2 -value v2
./rkvclient get -address localhost:27016 -key somekey0
./rkvclient get -address localhost:27017 -key sk1
./rkvclient del -address localhost:27015 -key sk2
```
## Benchmark
Below benchmark was run against the leader node directly:
```bash
./rkvclient benchmark -address localhost:27016 -times 20000
=====================================================
Benchmark with 1 connection, 100 concurrent clients:
SET
  Total set clients   : 100
  Total set calls     : 20000
  Total set success   : 20000
  Total time taken    : 989 ms
  Average set latency : 4 ms
GET
  Total get calls     : 20000
  Total get success   : 20000
  Total time taken    : 511ms
  Average get latency : 2ms
```

## Happy coding. Peace.
MIT © [sidecus](https://github.com/sidecus)