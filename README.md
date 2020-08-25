# Go Hotstuff

**Note:** This is still a work in progress (WIP).

Golang implementation of the chained [hotstuff](https://arxiv.org/pdf/1803.05069.pdf) protocol.

Hotstuff is a Byzantine fault-tolerant (BFT) state machine replication (SMR) library. 
The implementation is inspired by the [libhotstuff project](https://github.com/hot-stuff/libhotstuff). 

## Prerequisites
- Go 1.13+ installation or later
- Protoc Plugin
- Protocol Buffers

## Getting started

### build project
```bash
make build
```

### start four replica nodes on the different terminals
```bash
make server node_id=0

make server node_id=1

make server node_id=2

make server node_id=3
```

### run client to send proposal request
```
make client
```
