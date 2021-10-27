package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/types"
	"log"
	"os"
	"regexp"
)

var (
	pTrack1 = regexp.MustCompile(`github.com/Azure/azure-sdk-for-go/services/(preview/)?[\w-]+/mgmt`)

	// The embedded SDK can be either Azure Go SDK Track1 or Pandora SDK
	pEmbeddedSDK = regexp.MustCompile(`github.com/hashicorp/terraform-provider-azurerm/internal/services/\w+/sdk`)
)

const usage = `Terraform resource to Azure API path mapping generator.
Usage: generate-provider-resource-mapping package...

Note that the first package has to be the internal/sdk package. The following packages are the ones under internal/services.
`

func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, usage)
		os.Exit(1)
	}

	pkgs, err := loadPackage(".", flag.Args())

	if err != nil {
		log.Fatal(err)
	}

	sdkPkg := pkgs[0].GoPackage
	var typedRegistration, untypedRegistration *types.Interface
	for _, obj := range sdkPkg.TypesInfo.Defs {
		if obj == nil {
			continue
		}
		t := obj.Type()
		nt, ok := t.(*types.Named)
		if !ok {
			continue
		}
		ut, ok := nt.Underlying().(*types.Interface)
		if !ok {
			continue
		}
		switch obj.Name() {
		case "TypedServiceRegistration":
			typedRegistration = ut
		case "UntypedServiceRegistration":
			untypedRegistration = ut
		}
	}
	if typedRegistration == nil {
		log.Fatal("can't find TypedServiceRegistration")
	}
	if untypedRegistration == nil {
		log.Fatal("can't find UntypedServiceRegistration")
	}

	result := map[string]string{}
	for _, pkg := range pkgs[1:] {
		log.Printf("Processing package %s", pkg.GoPackage.Name)
		for _, obj := range pkg.GoPackage.TypesInfo.Defs {
			if obj == nil || obj.Name() != "Registration" {
				continue
			}
			if types.Implements(obj.Type(), typedRegistration) {
				mappings, err := NewTypedRegistration(pkg, obj).run()
				if err != nil {
					log.Println(err)
				} else {
					for k, v := range mappings {
						result[k] = v
					}
				}
			}
			if types.Implements(obj.Type(), untypedRegistration) {
				mappings, err := NewUntypedRegistration(pkg, obj).run()
				if err != nil {
					log.Println(err)
				} else {
					for k, v := range mappings {
						result[k] = v
					}
				}
			}
		}
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(out))
}
