#!/usr/bin/env bash
REVISION=$(git rev-parse --short HEAD)
OS_ARCH=("freebsd:amd64"
  "freebsd:386"
  "freebsd:arm"
  "freebsd:arm64"
  "windows:amd64"
  "windows:386"
  "linux:amd64"
  "linux:386"
  "linux:arm"
  "linux:arm64"
  "darwin:amd64"
  "darwin:arm64")
mkdir dist
for os_arch in "${OS_ARCH[@]}" ; do
  OS=${os_arch%%:*}
  ARCH=${os_arch#*:}
  echo "GOOS: ${OS}, GOARCH: ${ARCH}"

  output=aztfy-$OS-$ARCH
  if [[ $OS = windows ]]; then
    output=$output.exe
  fi

  GOOS="${OS}" GOARCH="${ARCH}" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X 'main.version=${VERSION}' -X 'main.revision=${REVISION}'" -o $output
  mv $output dist
done
