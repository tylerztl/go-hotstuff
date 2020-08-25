# Copyright zhigui Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - build - generate protobuf by .proto files and build hotstuff node
#   - clean - delete generated files(output dir)
#   - server - start hotstuff node by different id
#   - client - start hotstuff test client to send test transaction

node_id = 0

.PHONY: build
build:
	./build.sh

.PHONY: clean
clean:
	rm -rf output

.PHONY: server
server:
	./output/hotstuff-server start --replicaId $(node_id)

.PHONY: client
client:
	./output/hotstuff-client --server 127.0.0.1:8000
