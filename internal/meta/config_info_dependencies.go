package meta

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/internal/tfresourceid"
)

type ReferenceDependencies struct {
	internalMap map[tfresourceid.TFResourceId]tfaddr.TFAddr
}

type TFAddrSet struct {
	internalMap map[tfaddr.TFAddr]bool
}

type AmbiguousDependencies struct {
	internalMap map[tfresourceid.TFResourceId]*TFAddrSet
}

func (d *ReferenceDependencies) Add(id tfresourceid.TFResourceId, addr tfaddr.TFAddr) {
	if d.internalMap == nil {
		d.internalMap = make(map[tfresourceid.TFResourceId]tfaddr.TFAddr)
	}
	d.internalMap[id] = addr
}

func (d *ReferenceDependencies) Contains(id tfresourceid.TFResourceId) bool {
	_, exists := d.internalMap[id]
	return exists
}

func (d *ReferenceDependencies) Get(id tfresourceid.TFResourceId) tfaddr.TFAddr {
	return d.internalMap[id]
}

func (d *ReferenceDependencies) Size() int {
	return len(d.internalMap)
}

func (d *ReferenceDependencies) HasDependencyWithPrefix(prefix string) bool {
	for tfResourceId := range d.internalMap {
		if strings.HasPrefix(string(tfResourceId), prefix) {
			return true
		}
	}
	return false
}

func (s *TFAddrSet) Add(addr tfaddr.TFAddr) {
	if s.internalMap == nil {
		s.internalMap = make(map[tfaddr.TFAddr]bool)
	}
	s.internalMap[addr] = true
}

func (s *TFAddrSet) Size() int {
	return len(s.internalMap)
}

func (s *TFAddrSet) List() []string {
	var result []string
	for tfAddr := range s.internalMap {
		result = append(result, tfAddr.String())
	}
	sort.Strings(result)
	return result
}

func (d *AmbiguousDependencies) Add(id tfresourceid.TFResourceId, addr tfaddr.TFAddr) {
	if d.internalMap == nil {
		d.internalMap = make(map[tfresourceid.TFResourceId]*TFAddrSet)
	}
	if _, ok := d.internalMap[id]; !ok {
		d.internalMap[id] = &TFAddrSet{internalMap: make(map[tfaddr.TFAddr]bool)}
	}
	d.internalMap[id].Add(addr)
}

func (d *AmbiguousDependencies) Size() int {
	return len(d.internalMap)
}

func (d *AmbiguousDependencies) HasDependencyWithPrefix(prefix string) bool {
	for tfResourceId := range d.internalMap {
		if strings.HasPrefix(string(tfResourceId), prefix) {
			return true
		}
	}
	return false
}

func (d *AmbiguousDependencies) List() []string {
	var result []string
	for _, tfAddrSet := range d.internalMap {
		result = append(result, fmt.Sprintf(
			"# One of %s (can't auto-resolve as their ids are identical)",
			strings.Join(tfAddrSet.List(), ","),
		))
	}
	return result
}
