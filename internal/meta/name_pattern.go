package meta

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/magodo/armid"
)

const (
	phType      = "{type}"       // last Azure resource type segment, e.g. "virtual_machines"
	phRP        = "{rp}"         // Azure resource provider namespace, e.g. "microsoft_compute"
	phName      = "{name}"       // last name segment of the Azure resource id
	phRootScope = "{root_scope}" // last name of the root scope (e.g. resource group name)
)

const (
	// idxOptional expands to an incremental index only when the same name is
	// shared by more than one resource, in which case the index starts from 2
	// (i.e. the first occurrence has no index at all).
	idxOptional = '*'
	// idxAlways always expands to an incremental index, starting from 1.
	idxAlways = '+'
)

// idxChars is the set of the index characters supported in a name pattern.
var idxChars = string([]rune{idxOptional, idxAlways})

// ValidateNamePattern validates the resource name pattern, which can contain at
// most one index character, either `*` or `+`.
func ValidateNamePattern(pattern string) error {
	if n := strings.Count(pattern, string(idxOptional)) + strings.Count(pattern, string(idxAlways)); n > 1 {
		return fmt.Errorf("the name pattern %q contains %d %q/%q, while at most one (exclusively) is allowed", pattern, n, string(idxOptional), string(idxAlways))
	}
	return nil
}

type nameExpander struct {
	// prefix and suffix are the pattern segments before/after the index character.
	prefix string
	suffix string
	// always indicates the index character is `+`, rather than `*`.
	always bool
	// counts counts the name per resource type.
	counts map[string]map[string]int
}

func newNameExpander(pattern string) *nameExpander {
	// An `*` is implicitly appended at the end when no index character is specified.
	if !strings.ContainsAny(pattern, idxChars) {
		pattern += string(idxOptional)
	}

	pos := strings.IndexAny(pattern, idxChars)
	return &nameExpander{
		prefix: pattern[:pos],
		suffix: pattern[pos+1:],
		always: rune(pattern[pos]) == idxAlways,
		counts: map[string]map[string]int{},
	}
}

// Expand returns the resource name produced by applying the pattern to the
// given TF resource.
func (e *nameExpander) Expand(res resourceset.TFResource) string {
	prefix, suffix := expandPlaceholders(e.prefix, res), expandPlaceholders(e.suffix, res)

	key := prefix + "\x00" + suffix

	if e.counts[res.TFType] == nil {
		e.counts[res.TFType] = map[string]int{}
	}
	ic := e.counts[res.TFType]
	ic[key]++

	var idx string
	if n := ic[key]; e.always || n > 1 {
		idx = strconv.Itoa(n)
	}

	return toTFName(prefix + idx + suffix)
}

func expandPlaceholders(pattern string, res resourceset.TFResource) string {
	id := res.AzureId

	out := pattern
	if strings.Contains(out, phType) {
		out = strings.ReplaceAll(out, phType, snakeCase(lastSegment(id.Types())))
	}
	if strings.Contains(out, phRP) {
		out = strings.ReplaceAll(out, phRP, snakeCase(id.Provider()))
	}
	if strings.Contains(out, phName) {
		out = strings.ReplaceAll(out, phName, snakeCase(lastSegment(id.Names())))
	}
	if strings.Contains(out, phRootScope) {
		out = strings.ReplaceAll(out, phRootScope, snakeCase(rootScopeName(id)))
	}
	return out
}

func lastSegment(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	return segs[len(segs)-1]
}

// rootScopeName returns a short, identifier-friendly representation of the
// root scope of the resource id (e.g. the resource group name, the
// subscription id, or the management group name).
func rootScopeName(id armid.ResourceId) string {
	if id == nil {
		return ""
	}
	root := id.RootScope()
	if root == nil {
		return ""
	}
	names := root.Names()
	if len(names) == 0 {
		return ""
	}
	return names[len(names)-1]
}

// snakeCase converts a string (potentially CamelCase / dotted / mixed) to a
// lowercase, underscore-separated identifier. Non-alphanumeric characters
// become underscores; runs of underscores are collapsed; leading/trailing
// underscores are trimmed.
func snakeCase(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case unicode.IsUpper(r):
			// Insert `_` before an uppercase letter when:
			//   - it follows a lowercase / digit, or
			//   - it is followed by a lowercase letter and preceded by another uppercase
			//     (so that "HTTPServer" -> "http_server")
			if i > 0 {
				prev := runes[i-1]
				switch {
				case unicode.IsLower(prev) || unicode.IsDigit(prev):
					b.WriteByte('_')
				case unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]):
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsLower(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	// Collapse runs of underscores and trim.
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}

// toTFName makes sure the final name is a valid Terraform identifier.
// Terraform identifiers must start with a letter or underscore and may then
// contain letters, digits, underscores and dashes. We restrict ourselves to
// the conservative subset [A-Za-z0-9_].
func toTFName(s string) string {
	if s == "" {
		return "res"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "res"
	}
	if c := out[0]; c >= '0' && c <= '9' {
		out = "_" + out
	}
	return out
}
