package main

import (
	"go/types"
	"golang.org/x/tools/go/callgraph"
)

func AllCalleesOf(root *callgraph.Node) map[*callgraph.Node]bool {
	s := map[*callgraph.Node]bool{}
	wl := map[*callgraph.Node]bool{root: true}
	for len(wl) != 0 {
		callees := map[*callgraph.Node]bool{}
		for node := range wl {
			directCallees := callgraph.CalleesOf(node)
			for k := range directCallees {
				callees[k] = true
			}
		}
		for k := range wl {
			s[k] = true
		}

		wl = map[*callgraph.Node]bool{}
		for k := range callees {
			if !s[k] {
				wl[k] = true
			}
		}
	}
	delete(s, root)
	return s
}

// DeduplicateInterfaceMethod deduplicate the interface method resulting from CHA callgraph spurious call edges.
func DeduplicateInterfaceMethod(in map[*callgraph.Node]bool) map[*types.Func]bool {
	out := map[*types.Func]bool{}
	for k := range in {
		// Skipping the anonymous functions, which has no corresponding type object
		if k.Func.Object() == nil {
			continue
		}
		out[k.Func.Object().(*types.Func)] = true
	}
	return out
}
