# raft
Reinventing the wheel - raft based KV store using Go & gRPC (protobuf)

## Intro
The [original version](https://github.com/sidecus/raft/tree/v0.1-alpha) was built on top of channel based on my initial understanding of the Raft algorithm. All request/reply messges were communicated with a node goroutine via channels. All node data is managed within the node goroutine itself.

This new version is now using mutex instead, so that we don't rely on one node goroutine to process everything.
I tried to follow the paper as much as possible, but still there might be some misunderstanding. Please feel free to correct me (open an issue or send a PR) if that's the case.

## Build
```PowerShell
cd .\cmd\raft
go build .
cd .\cmd\raftclient
go build .
```
This builds two executables, one in each folder (.exe on Windows for example):
- Raft KV store server:
```
rkv.exe
```
- Raft KV store client:
```
rkvclient.exe
```

## Run
### Start raft server nodes
- Terminal#0
```PowerShell
rkv.exe -nodeid 0 -addresses localhost:27015,localhost:27016,localhost:27017
```
- Terminal#1
```PowerShell
rkv.exe -nodeid 1 -addresses localhost:27015,localhost:27016,localhost:27017
```
- Terminal#2
```PowerShell
rkv.exe -nodeid 2 -addresses localhost:27015,localhost:27016,localhost:27017
```
### Run client against any nodes for set/get/del
```PowerShell
rkvclient.exe set -address localhost:27015 -key 0 -value v0
rkvclient.exe set -address localhost:27016 -key 1 -value v1
rkvclient.exe set -address localhost:27016 -key 2 -value v2
rkvclient.exe get -address localhost:27016 -key 0
rkvclient.exe get -address localhost:27017 -key 1
rkvclient.exe del -address localhost:27015 -key 2
```
Try to kill any server, and run more client cmds. Watch the logs. Enjoy!

## Happy coding. Peace.
