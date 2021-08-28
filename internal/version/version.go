package version

import "fmt"

var (
	// Following are expected to be filled by the compiler `-ldflags`
	GitCommit   string
	GitDescribe string
)

func GetVersion() string {
	// no tag found
	if GitDescribe == GitCommit {
		return fmt.Sprintf("0.0.0 (%s)", GitCommit)
	}
	return fmt.Sprintf("%s (%s)", GitDescribe, GitCommit)
}
