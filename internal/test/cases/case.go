package cases

import (
	"github.com/Azure/aztfy/internal/resmap"
	"github.com/Azure/aztfy/internal/test"
)

type Case interface {
	Tpl(test.Data) string
	ResourceMapping(test.Data) (resmap.ResourceMapping, error)
	AzureResourceIds(test.Data) ([]string, error)
}
