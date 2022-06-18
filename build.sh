#!/usr/bin/env bash

set -ex

mkdir -p build/windows-amd64
mkdir -p build/linux-amd64
mkdir -p build/linux-arm64
mkdir -p build/darwin-amd64
mkdir -p build/freebsd-amd64

export CGO_ENABLED=0

GOOS=windows GOARCH=amd64 go build -o build/windows-amd64 ./...
GOOS=linux GOARCH=amd64 go build -o build/linux-amd64 ./...
GOOS=linux GOARCH=arm64 go build -o build/linux-arm64 ./...
GOOS=darwin GOARCH=amd64 go build -o build/darwin-amd64 ./...
GOOS=freebsd GOARCH=amd64 go build -o build/freebsd-amd64 ./...

cd build
tar -czf linux-amd64.tar.gz linux-amd64
tar -czf linux-arm64.tar.gz linux-arm64
tar -czf darwin-amd64.tar.gz darwin-amd64
zip -r windows-amd64.zip windows-amd64
tar -czf freebsd-amd64.tar.gz freebsd-amd64
