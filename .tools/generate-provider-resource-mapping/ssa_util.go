package main

import "golang.org/x/tools/go/callgraph"

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