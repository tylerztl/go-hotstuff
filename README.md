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
```
make build
```

### run replica 0
```
./output/hotstuff-server start
```

### run replica 1 on new terminal
```
./output/hotstuff-server start --replicaId 1 -p 8001
```

### run replica 2 on new terminal
```
./output/hotstuff-server start --replicaId 2 -p 8002
```

### run replica 3 on new terminal
```
./output/hotstuff-server start --replicaId 3 -p 8003
```

### run client to send proposal request
```
./output/hotstuff-client --server 127.0.0.1:8000
```
