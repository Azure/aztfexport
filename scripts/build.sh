#!/usr/bin/env bash

# Usage: build.sh OS ARCH VERSION
# E.g. build.sh linux amd64 v0.1.0
REVISION=$(git rev-parse --short HEAD)
mkdir dist
OS=$1
ARCH=$2
VERSION=$3
echo "OS: ${OS}, ARCH: ${ARCH}, VERSION: ${VERSION}"
output=aztfy_${OS}_${ARCH}
if [[ $OS = windows ]]; then
    output=$output.exe
fi
GOOS="${OS}" GOARCH="${ARCH}" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X 'main.version=${VERSION}' -X 'main.revision=${REVISION}'" -o dist/$output
