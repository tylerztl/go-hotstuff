#!/bin/bash

set -eux

PROTO_DIRS="$(find "$(pwd)" -name '*.proto' -exec dirname {} \; | sort | uniq)"

# As this is a proto root, and there may be subdirectories with protos, compile the protos for each sub-directory which contains them
for dir in ${PROTO_DIRS}; do
    protoc --proto_path="$dir" \
           --go_out=plugins=grpc,paths=source_relative:"$dir/pb" \
           "$dir"/*.proto
done


mkdir -p output
output_dir=output
go build -o ${output_dir}/hotstuff-client ./cmd/client/
go build -o ${output_dir}/hotstuff-server ./cmd/server/
