/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

var (
	// Max send and receive bytes for grpc clients and servers
	MaxRecvMsgSize           = 100 * 1024 * 1024 // 100 MiB
	MaxSendMsgSize           = 100 * 1024 * 1024
	DefaultConnectionTimeout = time.Second * 3
	ServerMinInterval        = time.Duration(1) * time.Minute
)

// SecureOptions defines the security parameters (e.g. TLS) for a
// GRPCServer or GRPCClient instance
type TLSOptions struct {
	// PEM-encoded X509 public key to be used for TLS communication
	Certificate []byte
	// PEM-encoded private key to be used for TLS communication
	Key []byte
	// Set of PEM-encoded X509 certificate authorities used by clients to
	// verify server certificates
	ServerRootCAs [][]byte
	// Set of PEM-encoded X509 certificate authorities used by servers to
	// verify client certificates
	ClientRootCAs [][]byte
	// Whether or not to use TLS for communication
	UseTLS bool
	// Whether or not TLS client must present certificates for authentication
	RequireClientCert bool
}

type GrpcClient struct {
	// TLS configuration used by the grpc.ClientConn
	tlsConfig *tls.Config
	// Options for setting up new connections
	dialOpts []grpc.DialOption
	// Duration for which to block while established a new connection
	timeout time.Duration
}

// NewGrpcClient creates a new implementation of GrpcClient given an address
// and client configuration
func NewGrpcClient(opts *TLSOptions) (*GrpcClient, error) {
	client := &GrpcClient{}
	// parse secure options
	var err error
	client.tlsConfig, err = parseTLSOptionsForClient(opts)
	if err != nil {
		return nil, err
	}

	// keepalive options
	kaOpts := keepalive.ClientParameters{
		Time:                1 * time.Minute,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}
	client.dialOpts = append(client.dialOpts, grpc.WithKeepaliveParams(kaOpts))
	// Unless asynchronous connect is set, make connection establishment blocking.
	client.dialOpts = append(client.dialOpts, grpc.WithBlock())
	client.dialOpts = append(client.dialOpts, grpc.FailOnNonTempDialError(true))
	// set send/recv message size to package defaults
	client.dialOpts = append(client.dialOpts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(MaxRecvMsgSize),
		grpc.MaxCallSendMsgSize(MaxSendMsgSize),
	))
	client.timeout = DefaultConnectionTimeout

	return client, nil
}

// NewConnection returns a grpc.ClientConn for the target address and
// overrides the server name used to verify the hostname on the
// certificate returned by a server when using TLS
func (client *GrpcClient) NewConnection(address string) (*grpc.ClientConn, error) {
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, client.dialOpts...)

	if client.tlsConfig != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(client.tlsConfig)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	ctx, cancel := context.WithTimeout(context.Background(), client.timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, errors.Errorf("failed to create new connection, %v", errors.WithStack(err))
	}
	return conn, nil
}

func parseTLSOptionsForClient(opts *TLSOptions) (*tls.Config, error) {
	if opts == nil || !opts.UseTLS {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if len(opts.ServerRootCAs) > 0 {
		certPool := x509.NewCertPool()
		for _, certBytes := range opts.ServerRootCAs {
			if ok := certPool.AppendCertsFromPEM(certBytes); !ok {
				return nil, errors.New("error adding server root certificate")
			}
		}
		tlsConfig.RootCAs = certPool
	}
	if opts.RequireClientCert {
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
	}

	return tlsConfig, nil
}
