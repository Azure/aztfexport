package main

import "fmt"

var (
	// Followings are filled by compiler during goreleaser
	version string = "dev"
	commit  string = "unknown"
)

func getVersion() string {
	return fmt.Sprintf("%s (%s)", version, commit)
}
