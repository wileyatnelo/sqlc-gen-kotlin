package core

import (
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestConfigJsonTypeFor(t *testing.T) {
	conf := Config{
		JsonTypes: []JsonTypeOverride{
			{Column: "events.payload", KtType: "com.example.Payload"},
			{Column: "analytics.events.meta", KtType: "com.example.Meta"},
		},
	}

	tests := []struct {
		name          string
		schema, table string
		column        string
		wantType      string
		wantOK        bool
	}{
		{"short form", "public", "events", "payload", "com.example.Payload", true},
		{"short form no schema", "", "events", "payload", "com.example.Payload", true},
		{"case insensitive", "public", "Events", "Payload", "com.example.Payload", true},
		{"schema-qualified", "analytics", "events", "meta", "com.example.Meta", true},
		{"no match", "public", "authors", "bio", "", false},
		{"empty column", "public", "events", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := conf.jsonTypeFor(tt.schema, tt.table, tt.column)
			if ok != tt.wantOK || got != tt.wantType {
				t.Fatalf("jsonTypeFor(%q,%q,%q) = (%q,%v), want (%q,%v)",
					tt.schema, tt.table, tt.column, got, ok, tt.wantType, tt.wantOK)
			}
		})
	}
}

func TestMapperType(t *testing.T) {
	tests := map[string]string{
		"":         "",
		"jackson2": "com.fasterxml.jackson.databind.json.JsonMapper",
		"jackson3": "tools.jackson.databind.json.JsonMapper",
	}
	for serializer, want := range tests {
		if got := (Config{JsonSerializer: serializer}).MapperType(); got != want {
			t.Fatalf("MapperType(%q) = %q, want %q", serializer, got, want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	for _, ok := range []string{"", "jackson2", "jackson3"} {
		if err := (Config{JsonSerializer: ok}).Validate(); err != nil {
			t.Fatalf("Validate(%q) = %v, want nil", ok, err)
		}
	}
	if err := (Config{JsonSerializer: "jackson"}).Validate(); err == nil {
		t.Fatal("Validate(\"jackson\") = nil, want error")
	}
}

func catalogReq() *plugin.GenerateRequest {
	return &plugin.GenerateRequest{
		Settings: &plugin.Settings{Engine: "postgresql"},
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{
				{
					Name: "public",
					Tables: []*plugin.Table{
						{
							Rel: &plugin.Identifier{Schema: "public", Name: "events"},
							Columns: []*plugin.Column{
								{Name: "payload", Type: &plugin.Identifier{Name: "jsonb"}},
								{Name: "name", Type: &plugin.Identifier{Name: "text"}},
							},
						},
					},
				},
			},
		},
	}
}

func TestValidateJsonTypes(t *testing.T) {
	req := catalogReq()

	tests := []struct {
		name      string
		jsonTypes []JsonTypeOverride
		wantErr   bool
	}{
		{"none", nil, false},
		{"valid jsonb column", []JsonTypeOverride{{Column: "events.payload", KtType: "com.example.Payload"}}, false},
		{"schema-qualified", []JsonTypeOverride{{Column: "public.events.payload", KtType: "com.example.Payload"}}, false},
		{"text column", []JsonTypeOverride{{Column: "events.name", KtType: "com.example.Payload"}}, true},
		{"unknown column", []JsonTypeOverride{{Column: "events.nope", KtType: "com.example.Payload"}}, true},
		{"unknown table", []JsonTypeOverride{{Column: "widgets.payload", KtType: "com.example.Payload"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (Config{JsonTypes: tt.jsonTypes}).ValidateJsonTypes(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateJsonTypes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSimpleName(t *testing.T) {
	for in, want := range map[string]string{
		"com.example.Payload": "Payload",
		"Payload":             "Payload",
		"a.b.c.D":             "D",
	} {
		if got := SimpleName(in); got != want {
			t.Fatalf("SimpleName(%q) = %q, want %q", in, got, want)
		}
	}
}

func jsonbColumn(notNull bool) *plugin.Column {
	return &plugin.Column{
		Name:    "payload",
		NotNull: notNull,
		Type:    &plugin.Identifier{Name: "jsonb"},
		Table:   &plugin.Identifier{Schema: "public", Name: "events"},
	}
}

func pgReq() *plugin.GenerateRequest {
	return &plugin.GenerateRequest{Settings: &plugin.Settings{Engine: "postgresql"}}
}

func TestMakeTypeJsonOverride(t *testing.T) {
	conf := Config{
		JsonSerializer: "jackson3",
		JsonTypes:      []JsonTypeOverride{{Column: "events.payload", KtType: "com.example.Payload"}},
	}

	got := makeType(conf, pgReq(), jsonbColumn(true))
	if !got.IsJson || got.Name != "Payload" || got.JsonType != "com.example.Payload" {
		t.Fatalf("makeType json = %+v, want IsJson Payload com.example.Payload", got)
	}

	// Without json_serializer enabled, jsonb stays a String.
	off := Config{JsonTypes: conf.JsonTypes}
	if got := makeType(off, pgReq(), jsonbColumn(true)); got.IsJson || got.Name != "String" {
		t.Fatalf("makeType (disabled) = %+v, want String", got)
	}

	// Enabled but no override for the column -> String.
	noOverride := Config{JsonSerializer: "jackson3"}
	if got := makeType(noOverride, pgReq(), jsonbColumn(true)); got.IsJson || got.Name != "String" {
		t.Fatalf("makeType (no override) = %+v, want String", got)
	}
}

func TestJdbcGetJson(t *testing.T) {
	notNull := makeType(Config{
		JsonSerializer: "jackson3",
		JsonTypes:      []JsonTypeOverride{{Column: "events.payload", KtType: "com.example.Payload"}},
	}, pgReq(), jsonbColumn(true))

	if got := jdbcGet(notNull, 3); got != `mapper.readValue(results.getString(3), Payload::class.java)` {
		t.Fatalf("jdbcGet non-null = %q", got)
	}

	nullable := makeType(Config{
		JsonSerializer: "jackson3",
		JsonTypes:      []JsonTypeOverride{{Column: "events.payload", KtType: "com.example.Payload"}},
	}, pgReq(), jsonbColumn(false))

	want := `results.getString(3)?.let { mapper.readValue(it, Payload::class.java) }`
	if got := jdbcGet(nullable, 3); got != want {
		t.Fatalf("jdbcGet nullable = %q, want %q", got, want)
	}
}

func TestJdbcSetJson(t *testing.T) {
	conf := Config{
		JsonSerializer: "jackson3",
		JsonTypes:      []JsonTypeOverride{{Column: "events.payload", KtType: "com.example.Payload"}},
	}
	notNull := makeType(conf, pgReq(), jsonbColumn(true))
	if got := jdbcSet(notNull, 2, "payload"); got != `stmt.setObject(2, mapper.writeValueAsString(payload), Types.OTHER)` {
		t.Fatalf("jdbcSet non-null = %q", got)
	}

	nullable := makeType(conf, pgReq(), jsonbColumn(false))
	want := `stmt.setObject(2, payload?.let { mapper.writeValueAsString(it) }, Types.OTHER)`
	if got := jdbcSet(nullable, 2, "payload"); got != want {
		t.Fatalf("jdbcSet nullable = %q, want %q", got, want)
	}
}
