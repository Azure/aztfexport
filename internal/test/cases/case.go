package cases

import (
	"github.com/Azure/aztfy/internal/resmap"
	"github.com/Azure/aztfy/internal/test"
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
