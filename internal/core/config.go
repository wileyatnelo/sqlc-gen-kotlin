package core

import (
	"fmt"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
)

// mapperTypes maps a json_serializer value to the fully-qualified JsonMapper
// type that QueriesImpl is given. Jackson 2 and Jackson 3 share the same
// JsonMapper API (readValue / writeValueAsString) but live in different packages.
var mapperTypes = map[string]string{
	"jackson2": "com.fasterxml.jackson.databind.json.JsonMapper",
	"jackson3": "tools.jackson.databind.json.JsonMapper",
}

type Config struct {
	Package                     string             `json:"package"`
	EmitExactTableNames         bool               `json:"emit_exact_table_names"`
	InflectionExcludeTableNames []string           `json:"inflection_exclude_table_names"`
	JsonSerializer              string             `json:"json_serializer"`
	JsonTypes                   []JsonTypeOverride `json:"json_types"`
}

// JsonTypeOverride maps a json/jsonb column to a user-provided Kotlin type that
// the column is deserialized into (see the json_types option).
type JsonTypeOverride struct {
	// Column identifies the target column as "table.column" or
	// "schema.table.column". Matching is case-insensitive.
	Column string `json:"column"`
	// KtType is the fully-qualified Kotlin type the column maps to,
	// e.g. "com.example.Payload".
	KtType string `json:"kt_type"`
}

// jsonEnabled reports whether jsonb deserialization is configured.
func (c Config) jsonEnabled() bool {
	return c.MapperType() != ""
}

// MapperType returns the fully-qualified JsonMapper type for the configured
// json_serializer, or "" if no (recognized) serializer is set.
func (c Config) MapperType() string {
	return mapperTypes[c.JsonSerializer]
}

// Validate reports a configuration error for an unrecognized json_serializer so
// a typo surfaces instead of silently leaving jsonb columns as String.
func (c Config) Validate() error {
	if c.JsonSerializer == "" {
		return nil
	}
	if _, ok := mapperTypes[c.JsonSerializer]; !ok {
		return fmt.Errorf("unsupported json_serializer %q: expected one of jackson2, jackson3", c.JsonSerializer)
	}
	return nil
}

// ValidateJsonTypes checks each json_types entry against the catalog so a
// mistargeted override fails fast instead of being silently ignored. An entry
// must resolve to a real table column whose database type is json or jsonb;
// otherwise (a typo, a stale reference, or a non-json column like text) it would
// never be applied and the column would quietly stay a String.
func (c Config) ValidateJsonTypes(req *plugin.GenerateRequest) error {
	for _, o := range c.JsonTypes {
		want := strings.ToLower(strings.TrimSpace(o.Column))

		matched := false
		jsonMatched := false
		gotType := ""
		for _, schema := range req.Catalog.Schemas {
			if schema.Name == "pg_catalog" || schema.Name == "information_schema" {
				continue
			}
			for _, table := range schema.Tables {
				for _, col := range table.Columns {
					short := strings.ToLower(table.Rel.Name + "." + col.Name)
					full := strings.ToLower(schema.Name + "." + table.Rel.Name + "." + col.Name)
					if want != short && want != full {
						continue
					}
					matched = true
					gotType = sdk.DataType(col.Type)
					if isJSONColumn(gotType) {
						jsonMatched = true
					}
				}
			}
		}

		switch {
		case !matched:
			return fmt.Errorf("json_types: no table column matches %q", o.Column)
		case !jsonMatched:
			return fmt.Errorf("json_types: column %q is type %q, not json or jsonb", o.Column, gotType)
		}
	}
	return nil
}

// jsonTypeFor returns the Kotlin type configured for the given json column, if
// any. schema may be empty; column references are matched against the
// "table.column" and "schema.table.column" suffixes case-insensitively so a user
// can write the shorter form when there is no ambiguity.
func (c Config) jsonTypeFor(schema, table, column string) (string, bool) {
	if table == "" || column == "" {
		return "", false
	}
	short := strings.ToLower(table + "." + column)
	full := short
	if schema != "" {
		full = strings.ToLower(schema + "." + table + "." + column)
	}
	for _, o := range c.JsonTypes {
		got := strings.ToLower(strings.TrimSpace(o.Column))
		if got == short || got == full {
			return o.KtType, true
		}
	}
	return "", false
}

// SimpleName returns the unqualified Kotlin class name for a possibly
// fully-qualified type, e.g. "com.example.Payload" -> "Payload".
func SimpleName(ktType string) string {
	if i := strings.LastIndex(ktType, "."); i >= 0 {
		return ktType[i+1:]
	}
	return ktType
}
