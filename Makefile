install:
	@go install

gen:
	@./.tools/generate-provider-schema/run.sh $(PROVIDER_DIR) $(PROVIDER_VERSION)
	@./.tools/generate-provider-resource-mapping/run.sh $(PROVIDER_DIR)

depscheck:
	@echo "==> Checking source code with go mod tidy..."
	@go mod tidy
	@git diff --exit-code -- go.mod go.sum || \
		(echo; echo "Unexpected difference in go.mod/go.sum files. Run 'go mod tidy' command or revert any go.mod/go.sum changes and commit."; exit 1)

test:
	@go test ./...
