package main

// To set this from outside, use go build -ldflags "-X 'main.version=$(VERSION)'"
var version string = "dev"

// revision describes latest commit hash.
// This value is extracted by git command when building.
// To set this from outside, use go build -ldflags "-X 'main.revision=$(REVISION)'"
var revision string
