package test

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"
)

const TestToggleEnvVar = "AZTFY_E2E"

func Precheck(t *testing.T) {
	variables := []string{
		TestToggleEnvVar,
		"ARM_CLIENT_ID",
		"ARM_CLIENT_SECRET",
		"ARM_SUBSCRIPTION_ID",
		"ARM_TENANT_ID",
	}
	for _, variable := range variables {
		value := os.Getenv(variable)
		if value == "" {
			t.Skipf("`%s` must be set for e2e tests!", variable)
		}
	}
}

func EnsureTF(t *testing.T) string {
	i := install.NewInstaller()
	execPath, err := i.Ensure(context.Background(), []src.Source{
		&fs.Version{
			Product:     product.Terraform,
			Constraints: version.MustConstraints(version.NewConstraint(">=0.12")),
		},
	})
	if err != nil {
		t.Fatalf("failed to find a Terraform executable: %v", err)
	}
	return execPath
}
