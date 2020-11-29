# raft
Reinventing the wheel - a naive raft implementation with Go & grpc

The original versio is built on channel (request/reply messges via a channel to node goroutine) based on my naive understanding of the algorithm.

This new version is now built on mutex instead, so that we don't rely on one node goroutine to process everything.

I tried to follow the paper as much as possible, but still there might be some misunderstanding. Please feel free to correct me (open an issue or send a PR) if that's the case.

Haven't finished coding 5.3 yet for log  replication. Will do it later to enable a KV store based on raft.
