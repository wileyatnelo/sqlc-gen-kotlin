package core

import (
	"sort"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

type Importer struct {
	Settings    *plugin.Settings
	DataClasses []Struct
	Enums       []Enum
	Queries     []Query
	MapperType  string
}

// sortedKeys returns the keys of a string set in sorted order.
func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// modelJsonTypes collects the fully-qualified json types referenced by data classes.
func (i *Importer) modelJsonTypes() map[string]struct{} {
	set := map[string]struct{}{}
	for si := range i.DataClasses {
		for _, f := range i.DataClasses[si].Fields {
			if f.Type.IsJson && f.Type.JsonType != "" {
				set[f.Type.JsonType] = struct{}{}
			}
		}
	}
	return set
}

// UsesJson reports whether any query reads or writes a json column, meaning
// QueriesImpl needs a JsonMapper injected.
func (i *Importer) UsesJson() bool {
	return len(i.queryJsonTypes()) > 0
}

// queryJsonTypes collects the fully-qualified json types referenced by queries
// (results and parameters).
func (i *Importer) queryJsonTypes() map[string]struct{} {
	set := map[string]struct{}{}
	add := func(t ktType) {
		if t.IsJson && t.JsonType != "" {
			set[t.JsonType] = struct{}{}
		}
	}
	for _, q := range i.Queries {
		if !q.Ret.isEmpty() {
			add(q.Ret.Typ)
			if q.Ret.Struct != nil {
				for _, f := range q.Ret.Struct.Fields {
					add(f.Type)
				}
			}
		}
		if !q.Arg.isEmpty() {
			for _, f := range q.Arg.Struct.Fields {
				add(f.Type)
			}
		}
	}
	return set
}

func (i *Importer) usesType(typ string) bool {
	for _, strct := range i.DataClasses {
		for _, f := range strct.Fields {
			if f.Type.Name == typ {
				return true
			}
		}
	}
	return false
}

func (i *Importer) Imports(filename string) [][]string {
	switch filename {
	case "Models.kt":
		return i.modelImports()
	case "Querier.kt":
		return i.interfaceImports()
	default:
		return i.queryImports(filename)
	}
}

func (i *Importer) interfaceImports() [][]string {
	uses := func(name string) bool {
		for _, q := range i.Queries {
			if !q.Ret.isEmpty() {
				if strings.HasPrefix(q.Ret.Type(), name) {
					return true
				}
			}
			if !q.Arg.isEmpty() {
				for _, f := range q.Arg.Struct.Fields {
					if strings.HasPrefix(f.Type.Name, name) {
						return true
					}
				}
			}
		}
		return false
	}

	std := stdImports(uses)
	stds := make([]string, 0, len(std))
	for s := range std {
		stds = append(stds, s)
	}

	sort.Strings(stds)
	return groups(stds, sortedKeys(i.queryJsonTypes()))
}

// groups assembles import blocks, dropping any that are empty so the template
// does not emit stray blank lines.
func groups(blocks ...[]string) [][]string {
	out := [][]string{}
	for _, b := range blocks {
		if len(b) > 0 {
			out = append(out, b)
		}
	}
	return out
}

func (i *Importer) modelImports() [][]string {
	std := make(map[string]struct{})
	if i.usesType("Instant") {
		std["java.time.Instant"] = struct{}{}
		std["java.sql.Timestamp"] = struct{}{}
	}
	if i.usesType("LocalDate") {
		std["java.time.LocalDate"] = struct{}{}
	}
	if i.usesType("LocalTime") {
		std["java.time.LocalTime"] = struct{}{}
	}
	if i.usesType("LocalDateTime") {
		std["java.time.LocalDateTime"] = struct{}{}
	}
	if i.usesType("OffsetDateTime") {
		std["java.time.OffsetDateTime"] = struct{}{}
	}
	if i.usesType("UUID") {
		std["java.util.UUID"] = struct{}{}
	}

	stds := make([]string, 0, len(std))
	for s := range std {
		stds = append(stds, s)
	}

	sort.Strings(stds)
	return groups(stds, sortedKeys(i.modelJsonTypes()))
}

func stdImports(uses func(name string) bool) map[string]struct{} {
	std := map[string]struct{}{
		"java.sql.SQLException": {},
		"java.sql.Statement":    {},
	}
	if uses("Instant") {
		std["java.time.Instant"] = struct{}{}
		std["java.sql.Timestamp"] = struct{}{}
	}
	if uses("LocalDate") {
		std["java.time.LocalDate"] = struct{}{}
	}
	if uses("LocalTime") {
		std["java.time.LocalTime"] = struct{}{}
	}
	if uses("LocalDateTime") {
		std["java.time.LocalDateTime"] = struct{}{}
	}
	if uses("OffsetDateTime") {
		std["java.time.OffsetDateTime"] = struct{}{}
	}
	if uses("UUID") {
		std["java.util.UUID"] = struct{}{}
	}

	return std
}

func (i *Importer) queryImports(filename string) [][]string {
	uses := func(name string) bool {
		for _, q := range i.Queries {
			if !q.Ret.isEmpty() {
				if q.Ret.Struct != nil {
					for _, f := range q.Ret.Struct.Fields {
						if f.Type.Name == name {
							return true
						}
					}
				}
				if q.Ret.Type() == name {
					return true
				}
			}
			if !q.Arg.isEmpty() {
				for _, f := range q.Arg.Struct.Fields {
					if f.Type.Name == name {
						return true
					}
				}
			}
		}
		return false
	}

	hasEnum := func() bool {
		for _, q := range i.Queries {
			if !q.Arg.isEmpty() {
				for _, f := range q.Arg.Struct.Fields {
					if f.Type.IsEnum {
						return true
					}
				}
			}
		}
		return false
	}

	std := stdImports(uses)
	std["java.sql.Connection"] = struct{}{}
	if hasEnum() && i.Settings.Engine == "postgresql" {
		std["java.sql.Types"] = struct{}{}
	}

	jsonTypes := i.queryJsonTypes()
	if len(jsonTypes) > 0 {
		// setObject(idx, ..., Types.OTHER) is used to bind jsonb parameters.
		std["java.sql.Types"] = struct{}{}
		if i.MapperType != "" {
			jsonTypes[i.MapperType] = struct{}{}
		}
	}

	stds := make([]string, 0, len(std))
	for s := range std {
		stds = append(stds, s)
	}

	sort.Strings(stds)
	return groups(stds, sortedKeys(jsonTypes))
}
