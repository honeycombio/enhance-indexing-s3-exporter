#!/bin/bash

GOGO_LIB="github.com/gogo/protobuf@v1.3.2"
GOGO_PATH="$(go env GOMODCACHE)/$GOGO_LIB"


protoc \
    -I ./index/protos \
    --proto_path=${GOGO_PATH} \
    --gogofaster_out=plugins=grpc:./index/ \
    ./index/protos/*.proto