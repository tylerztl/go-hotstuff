package transport

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type GrpcServer struct {
	// Listen address for the server specified as hostname:port
	address string
	// Listener for handling network requests
	listener net.Listener
	// GRPC server
	server *grpc.Server
}

// NewGrpcServer creates a new implementation of a NewGrpcServer given a
// listen address
func NewGrpcServer(address string, opts *TLSOptions) (*GrpcServer, error) {
	if address == "" {
		return nil, errors.New("missing address parameter")
	}
	//create our listener
	listen, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := parseTLSOptionsForServer(opts)
	if err != nil {
		return nil, err
	}
	var serverOpts []grpc.ServerOption

	//set up server options for keepalive and TLS
	if tlsConfig != nil {
		serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	serverKeepAliveParameters := keepalive.ServerParameters{
		Time:    1 * time.Minute,
		Timeout: 20 * time.Second,
	}
	serverOpts = append(serverOpts, grpc.KeepaliveParams(serverKeepAliveParameters))
	// set max send and recv msg sizes
	serverOpts = append(serverOpts, grpc.MaxSendMsgSize(MaxSendMsgSize))
	serverOpts = append(serverOpts, grpc.MaxRecvMsgSize(MaxRecvMsgSize))
	// set connection timeout
	serverOpts = append(serverOpts, grpc.ConnectionTimeout(DefaultConnectionTimeout))
	//set enforcement policy
	kep := keepalive.EnforcementPolicy{
		MinTime: ServerMinInterval,
		// allow keepalive w/o rpc
		PermitWithoutStream: true,
	}
	serverOpts = append(serverOpts, grpc.KeepaliveEnforcementPolicy(kep))

	server := grpc.NewServer(serverOpts...)

	return &GrpcServer{address: listen.Addr().String(), listener: listen, server: server}, nil
}

// Address returns the listen address for this GRPCServer instance
func (s *GrpcServer) Address() string {
	return s.address
}

// Server returns the grpc.Server for the GRPCServer instance
func (s *GrpcServer) Server() *grpc.Server {
	return s.server
}

// Start starts the underlying grpc.Server
func (s *GrpcServer) Start() error {
	if s.listener == nil {
		return errors.New("nil listener")
	}

	if s.server == nil {
		return errors.New("nil server")
	}

	return s.server.Serve(s.listener)
}

// Stop stops the underlying grpc.Server
func (s *GrpcServer) Stop() {
	if s.server != nil {
		s.server.Stop()
	}
}

func parseTLSOptionsForServer(opts *TLSOptions) (*tls.Config, error) {
	if opts == nil || !opts.UseTLS {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
		SessionTicketsDisabled: true,
	}
	// make sure we have both Key and Certificate
	if opts.Key != nil && opts.Certificate != nil {
		cert, err := tls.X509KeyPair(opts.Certificate, opts.Key)
		if err != nil {
			return nil, errors.Errorf("failed to load client certificate, err: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else {
		return nil, errors.New("both Key and Certificate are required when using mutual TLS")
	}

	tlsConfig.ClientAuth = tls.RequestClientCert
	if opts.RequireClientCert {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		//if we have client root CAs, create a certPool
		if len(opts.ClientRootCAs) > 0 {
			certPool := x509.NewCertPool()
			for _, certBytes := range opts.ClientRootCAs {
				if ok := certPool.AppendCertsFromPEM(certBytes); !ok {
					return nil, errors.New("error adding client root certificate")
				}
			}
			tlsConfig.ClientCAs = certPool
		}
	}

	return tlsConfig, nil
}
