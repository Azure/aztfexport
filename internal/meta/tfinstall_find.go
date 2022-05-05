package meta

import (
	"context"

	"github.com/hashicorp/go-version"
	install "github.com/magodo/hc-install"
	"github.com/magodo/hc-install/checkpoint"
	"github.com/magodo/hc-install/fs"
	"github.com/magodo/hc-install/product"
	"github.com/magodo/hc-install/src"
)

// FindTerraform finds the path to the terraform executable.
func FindTerraform(ctx context.Context, tfDir string) (string, error) {
	i := install.NewInstaller()
	return i.Ensure(ctx, []src.Source{
		&fs.AnyVersion{
			Product:     &product.Terraform,
			ExtraPaths:  []string{tfDir},
			Constraints: version.MustConstraints(version.NewConstraint(">=0.12")),
		},
		&checkpoint.LatestVersion{
			Product:    product.Terraform,
			InstallDir: tfDir,
		},
	})

}
