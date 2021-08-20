This package defines a `Schema` type, which is almost the same as the `terraform-json.Schema` in the package `github.com/hashicorp/terraform-json`, but removed some properties, and kept the `Required`, `Optional`, `Computed` properties for the block type (comparing to attribute type).

The whole picture is illustrated below:

![schema transform](schema_transform.png)

The usage of this package is:

1. Run the generator to generate the json marshalled `Schema` from a certain AzureRM provider release.
2. The `aztfy` will leverage this `Schema` to modify the HCL (from `terraform add -from-state`), based on the additional information resides in the `Schema`.
