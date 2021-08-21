install:
	@go install
gen:
	@./tools/generate-provider-schema/run.sh $(PROVIDER_DIR) $(PROVIDER_VERSION)
