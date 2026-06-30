package core

import (
	"strings"
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func sliceColumn() *plugin.Column {
	return &plugin.Column{
		Name:        "ids",
		NotNull:     true,
		IsSqlcSlice: true,
		Type:        &plugin.Identifier{Name: "bigint"},
	}
}

func TestMakeTypeSlice(t *testing.T) {
	got := makeType(Config{}, pgReq(), sliceColumn())
	if !got.IsSlice || got.SliceName != "ids" {
		t.Fatalf("makeType slice = %+v, want IsSlice + SliceName=ids", got)
	}
	if got.String() != "List<Long>" {
		t.Fatalf("slice String() = %q, want List<Long>", got.String())
	}
}

func TestJdbcSQLSliceMarker(t *testing.T) {
	in := "SELECT id FROM t WHERE name = $1 AND id IN ($2)"
	out, _ := jdbcSQL(in, "postgresql", map[int]string{2: "ids"})
	want := "SELECT id FROM t WHERE name = ? AND id IN (/*SLICE:ids*/?)"
	if out != want {
		t.Fatalf("jdbcSQL slice = %q, want %q", out, want)
	}

	// MySQL already carries the marker; jdbcSQL leaves the text untouched.
	my := "SELECT id FROM t WHERE id IN (/*SLICE:ids*/?)"
	if out, _ := jdbcSQL(my, "mysql", nil); out != my {
		t.Fatalf("jdbcSQL mysql = %q, want unchanged", out)
	}
}

// sliceQueryArg builds a Params with a scalar "name" param ($1) followed by a
// slice "ids" param ($2), matching how BuildQueries would assemble them.
func sliceQueryArg() Params {
	return Params{
		Struct: &Struct{
			Name: "QBindings",
			Fields: []Field{
				{ID: 1, Name: "name", Type: ktType{Name: "String", Engine: "postgresql"}},
				{ID: 2, Name: "ids", Type: ktType{Name: "Long", IsSlice: true, SliceName: "ids", Engine: "postgresql"}},
			},
		},
		binding: []int{1, 2},
	}
}

func TestHasSlices(t *testing.T) {
	if !sliceQueryArg().HasSlices() {
		t.Fatal("HasSlices() = false, want true")
	}
	noSlice := Params{Struct: &Struct{Fields: []Field{{Name: "id", Type: ktType{Name: "Long"}}}}}
	if noSlice.HasSlices() {
		t.Fatal("HasSlices() = true for scalar-only params")
	}
}

func TestSliceReplaceChain(t *testing.T) {
	q := Query{ConstantName: "q", Arg: sliceQueryArg()}
	want := `.replaceFirst("/*SLICE:ids*/?", if (ids.isEmpty()) "NULL" else List(ids.size) { "?" }.joinToString(","))`
	if got := q.SliceReplaceChain(); got != want {
		t.Fatalf("SliceReplaceChain() =\n%q\nwant\n%q", got, want)
	}

	// No slice params -> empty chain, so the prepared statement source is unchanged.
	noSlice := Query{ConstantName: "q", Arg: Params{Struct: &Struct{Fields: []Field{{Name: "id", Type: ktType{Name: "Long"}}}}}}
	if got := noSlice.SliceReplaceChain(); got != "" {
		t.Fatalf("SliceReplaceChain() = %q, want empty", got)
	}
}

func TestSliceBindings(t *testing.T) {
	got := sliceQueryArg().Bindings()
	for _, want := range []string{
		"var i = 1",
		"stmt.setString(i, name)",
		"for (v in ids) {",
		"stmt.setLong(i, v)",
		"i++",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("slice Bindings() missing %q in:\n%s", want, got)
		}
	}
	// The scalar must be bound before the slice loop (binding order $1, $2).
	if strings.Index(got, "setString") > strings.Index(got, "for (v in ids)") {
		t.Fatalf("expected scalar bound before slice loop:\n%s", got)
	}
}
