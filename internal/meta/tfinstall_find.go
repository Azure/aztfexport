package meta

import (
	"context"

	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"
)

// FindTerraform finds the path to the terraform executable.
func FindTerraform(ctx context.Context) (string, error) {
	i := install.NewInstaller()
	return i.Ensure(ctx, []src.Source{
		&fs.AnyVersion{
			Product: &product.Terraform,
		},
	})
}
