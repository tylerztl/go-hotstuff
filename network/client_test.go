package network

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNonTLSClient(t *testing.T) {
	server, err := NewGrpcServer("127.0.0.1:", &TLSOptions{})
	assert.NoError(t, err)
	go server.Start()
	defer server.Stop()

	client, err := NewGrpcClient(&TLSOptions{})
	assert.NoError(t, err)
	_, port, _ := net.SplitHostPort(server.Address())
	conn, err := client.NewConnection(net.JoinHostPort("127.0.0.1", port))
	require.NoError(t, err)
	_ = conn.Close()
}

func TestTLSClient(t *testing.T) {
	server, err := NewGrpcServer("127.0.0.1:", &TLSOptions{
		UseTLS: true,
		//RequireClientCert: true,
		//ClientRootCAs:     [][]byte{loadFileOrDie(filepath.Join("testdata", "client", "ca.pem"))},
		Key:         loadFileOrDie(filepath.Join("testdata", "server", "key.pem")),
		Certificate: loadFileOrDie(filepath.Join("testdata", "server", "cert.pem")),
	})
	assert.NoError(t, err)
	go server.Start()
	defer server.Stop()

	client, err := NewGrpcClient(&TLSOptions{
		UseTLS:            true,
		RequireClientCert: true,
		Key:               loadFileOrDie(filepath.Join("testdata", "client", "key.pem")),
		Certificate:       loadFileOrDie(filepath.Join("testdata", "client", "cert.pem")),
		ServerRootCAs:     [][]byte{loadFileOrDie(filepath.Join("testdata", "server", "ca.pem"))}})
	assert.NoError(t, err)
	_, port, _ := net.SplitHostPort(server.Address())
	conn, err := client.NewConnection(net.JoinHostPort("localhost", port))
	require.NoError(t, err)
	_ = conn.Close()
}

func loadFileOrDie(path string) []byte {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println("Failed opening file", path, ":", err)
		os.Exit(1)
	}
	return b
}
