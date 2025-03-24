package meta_test

import (
	"context"
	"testing"

	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/aztfexport/pkg/meta"
)

func TestNewDummyMeta(t *testing.T) {
	cfg := config.Config{
		CommonConfig: config.CommonConfig{
			ProviderName: "test-provider",
		},
		ResourceGroupName: "test-rg",
	}

	meta, err := meta.NewDummyMeta(cfg)
	if err != nil {
		t.Fatalf("NewDummyMeta: expected no error, got %v", err)
	}

	resources, err := meta.ListResource(context.Background())
	if err != nil {
		t.Fatalf("ListResource: expected no error, got %v", err)
	}

	expectedNumResources := 5
	if len(resources) != expectedNumResources {
		t.Fatalf("ListResource: expected %d resources, got %d", expectedNumResources, len(resources))
	}
}
