package consensus

import (
	"io"
	"time"

	"github.com/zhigui-projects/go-hotstuff/pb"
	"github.com/zhigui-projects/go-hotstuff/transport"
)

type NodeInfo struct {
	Id      ReplicaID
	Addr    string
	TlsOpts *transport.TLSOptions
}

type NodeManager struct {
	server *transport.GrpcServer
	self   *NodeInfo
	// all nodes info contains self
	nodes map[ReplicaID]*NodeInfo
}

func NewNodeManager(id ReplicaID, replicas []*NodeInfo) *NodeManager {
	mgr := &NodeManager{
		nodes: make(map[ReplicaID]*NodeInfo, len(replicas)),
	}
	for _, node := range replicas {
		if node.Id == id {
			grpcServer, err := transport.NewGrpcServer(node.Addr, node.TlsOpts)
			if err != nil {
				logger.Error("Failed to new grpc server", "error", err)
				panic(err)
			}

			server := transport.NewABServer()
			pb.RegisterAtomicBroadcastServer(grpcServer.Server(), server)
			mgr.server = grpcServer
			mgr.self = node
		}
		mgr.nodes[node.Id] = node
	}

	return mgr
}

func (n *NodeManager) StartServer() {
	if err := n.server.Start(); err != nil {
		logger.Error(" Hotstuff node server start failed", "error", err)
		panic(err)
	}
	logger.Info("Hotstuff node started, beginning to serve requests", "replicaId", n.self.Id, "serverAddress", n.self.Addr)
}

func (n *NodeManager) ConnectWorkers(queue chan<- MsgExecutor) {
	for _, node := range n.nodes {
		if node.Id == n.self.Id {
			continue
		}
		go func(node *NodeInfo) {
			delay := time.After(0)
			for {
				// reconnect attempts
				<-delay
				delay = time.After(transport.DefaultConnectionTimeout)

				logger.Info("connecting to replica node", "id", node.Id, "address", node.Addr)

				bc, err := transport.NewBroadcastClient(node.Addr, int64(node.Id), node.TlsOpts)
				if err != nil {
					logger.Warn("could not connect to replica node", "id", node.Id, "address", node.Addr, "error", err)
					continue
				}
				logger.Info("connection to replica node established", "id", node.Id, "address", node.Addr)

				for {
					msg, err := bc.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						logger.Warn("consensus stream with replica node broke", "id", node.Id, "address", node.Addr, "error", err)
						break
					}

					go func() {
						queue <- &msgEvent{msg: msg, src: node.Id}
					}()
				}
			}
		}(node)
	}
}
