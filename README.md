# raft
Reinventing the wheel - a naive raft based KV store using Go & gRPC (protobuf)

## Intro
The original version is built on channel (request/reply messges via a channel to node goroutine) based on my naive understanding of the algorithm.

This new version is now built on mutex instead, so that we don't rely on one node goroutine to process everything.

I tried to follow the paper as much as possible, but still there might be some misunderstanding. Please feel free to correct me (open an issue or send a PR) if that's the case.

## Build
```PowerShell
go build .\cmd\raft
go build .\cmd\raftclient
```
This builds two executables (.exe on Windows for example):
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
### Run client
```PowerShell
rpcclient.exe set -address localhost:27016 -key 0 -value v0
rpcclient.exe set -address localhost:27016 -key 1 -value v1
rpcclient.exe set -address localhost:27016 -key 0 -value v2
rpcclient.exe get -address localhost:27016 -key 0
rpcclient.exe get -address localhost:27017 -key 0
```
Try to kill any server, run client query again.
Watch the logs and enjoy.

## Happy coding. Peace.
