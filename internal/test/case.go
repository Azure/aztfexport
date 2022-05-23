package test

import "github.com/Azure/aztfy/internal/resmap"

type Case interface {
	Tpl(Data) string
	ResourceMapping(Data) (resmap.ResourceMapping, error)
}
