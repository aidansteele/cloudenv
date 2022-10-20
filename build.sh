#!/bin/sh
set -eux

export GOOS=linux
export CGO_ENABLED=0

mkdir -p built/arm64
GOARCH=arm64 go build -ldflags="-s -w -buildid=" -o built/arm64/cloudenv ./cloudenv

mkdir -p built/x86_64
GOARCH=amd64 go build -ldflags="-s -w -buildid=" -o built/x86_64/cloudenv ./cloudenv

cd example
GOARCH=arm64 go build -ldflags="-s -w -buildid="
