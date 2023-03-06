package cases

import (
	"github.com/Azure/aztfexport/internal/resmap"
	"github.com/Azure/aztfexport/internal/test"
)

type SingleResourceContext struct {
	AzureId             string
	ExpectResourceCount int
}

type Case interface {
	Tpl(test.Data) string
	Total() int
	ResourceMapping(test.Data) (resmap.ResourceMapping, error)
	SingleResourceContext(test.Data) ([]SingleResourceContext, error)
}
