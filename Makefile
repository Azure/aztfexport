GIT_COMMIT=$$(git rev-parse --short HEAD)
GIT_DESCRIBE=$$(git describe --tags --always --match "v*")
GIT_IMPORT="github.com/magodo/aztfy/internal/version"
GOLDFLAGS="-s -w -X $(GIT_IMPORT).GitCommit=$(GIT_COMMIT) -X $(GIT_IMPORT).GitDescribe=$(GIT_DESCRIBE)"

install:
	@CGO_ENABLED=0 go install -ldflags $(GOLDFLAGS)

gen:
	@./tools/generate-provider-schema/run.sh $(PROVIDER_DIR) $(PROVIDER_VERSION)
