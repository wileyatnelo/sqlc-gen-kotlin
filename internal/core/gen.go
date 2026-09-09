package core

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/metadata"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"

	"github.com/sqlc-dev/sqlc-gen-kotlin/internal/inflection"
)

var ktIdentPattern = regexp.MustCompile("[^a-zA-Z0-9_]+")

type Constant struct {
	Name  string
	Type  string
	Value string
}

type Enum struct {
	Name      string
	Comment   string
	Constants []Constant
}

type Field struct {
	ID      int
	Name    string
	Type    ktType
	Comment string
}

type Struct struct {
	Table   plugin.Identifier
	Name    string
	Fields  []Field
	Comment string
}

type QueryValue struct {
	Emit   bool
	Name   string
	Struct *Struct
	Typ    ktType
}

func (v QueryValue) EmitStruct() bool {
	return v.Emit
}

func (v QueryValue) IsStruct() bool {
	return v.Struct != nil
}

func (v QueryValue) isEmpty() bool {
	return v.Typ == (ktType{}) && v.Name == "" && v.Struct == nil
}

func (v QueryValue) Type() string {
	if v.Typ != (ktType{}) {
		return v.Typ.String()
	}
	if v.Struct != nil {
		return v.Struct.Name
	}
	panic("no type for QueryValue: " + v.Name)
}

func jdbcSet(t ktType, idx int, name string) string {
	return jdbcSetExpr(t, strconv.Itoa(idx), name)
}

// jdbcSetExpr renders a JDBC setter where the statement index and bound value
// are arbitrary Kotlin expressions (e.g. a literal "1" and a field name, or the
// loop variables "i" and "v" used when binding a slice element by element).
func jdbcSetExpr(t ktType, idx, val string) string {
	if t.IsEnum && t.IsArray {
		return fmt.Sprintf(`stmt.setArray(%s, conn.createArrayOf("%s", %s.map { v -> v.value }.toTypedArray()))`, idx, t.DataType, val)
	}
	if t.IsEnum {
		if t.Engine == "postgresql" {
			return fmt.Sprintf("stmt.setObject(%s, %s.value, %s)", idx, val, "Types.OTHER")
		} else {
			return fmt.Sprintf("stmt.setString(%s, %s.value)", idx, val)
		}
	}
	if t.IsArray {
		return fmt.Sprintf(`stmt.setArray(%s, conn.createArrayOf("%s", %s.toTypedArray()))`, idx, t.DataType, val)
	}
	if t.IsJson {
		if t.IsNull {
			return fmt.Sprintf("stmt.setObject(%s, %s?.let { mapper.writeValueAsString(it) }, Types.OTHER)", idx, val)
		}
		return fmt.Sprintf("stmt.setObject(%s, mapper.writeValueAsString(%s), Types.OTHER)", idx, val)
	}
	if t.IsTime() {
		return fmt.Sprintf("stmt.setObject(%s, %s)", idx, val)
	}
	if t.IsInstant() {
		return fmt.Sprintf("stmt.setTimestamp(%s, Timestamp.from(%s))", idx, val)
	}
	if t.IsUUID() {
		return fmt.Sprintf("stmt.setObject(%s, %s)", idx, val)
	}
	// Every other type's Kotlin name doubles as its JDBC setter suffix, but numeric maps to
	// the qualified java.math.BigDecimal, which would render as the non-existent
	// "stmt.setjava.math.BigDecimal". Route it to the real method, as jdbcGet does.
	if t.IsBigDecimal() {
		return fmt.Sprintf("stmt.setBigDecimal(%s, %s)", idx, val)
	}
	return fmt.Sprintf("stmt.set%s(%s, %s)", t.Name, idx, val)
}

type Params struct {
	Struct  *Struct
	binding []int
}

func (v Params) isEmpty() bool {
	return len(v.Struct.Fields) == 0
}

func (v Params) Args() string {
	if v.isEmpty() {
		return ""
	}
	var out []string
	fields := v.Struct.Fields
	for _, f := range fields {
		out = append(out, f.Name+": "+f.Type.String())
	}
	if len(v.binding) > 0 {
		lookup := map[int]int{}
		for i, v := range v.binding {
			lookup[v] = i
		}
		sort.Slice(out, func(i, j int) bool {
			return lookup[fields[i].ID] < lookup[fields[j].ID]
		})
	}
	if len(out) < 3 {
		return strings.Join(out, ", ")
	}
	return "\n" + indent(strings.Join(out, ",\n"), 6, -1)
}

// orderedFields returns the parameter fields in the order their placeholders
// appear in the SQL (Postgres binding refs), or struct order otherwise (MySQL).
func (v Params) orderedFields() []Field {
	if len(v.binding) > 0 {
		out := make([]Field, 0, len(v.binding))
		for _, idx := range v.binding {
			out = append(out, v.Struct.Fields[idx-1])
		}
		return out
	}
	return v.Struct.Fields
}

// HasSlices reports whether any parameter is a sqlc.slice() IN-list, which
// requires binding placeholders dynamically at runtime.
func (v Params) HasSlices() bool {
	if v.Struct == nil {
		return false
	}
	for _, f := range v.Struct.Fields {
		if f.Type.IsSlice {
			return true
		}
	}
	return false
}

func (v Params) Bindings() string {
	if v.isEmpty() {
		return ""
	}
	if v.HasSlices() {
		return v.sliceBindings()
	}
	var out []string
	for i, f := range v.orderedFields() {
		out = append(out, jdbcSet(f.Type, i+1, f.Name))
	}
	return indent(strings.Join(out, "\n"), 10, 0)
}

// sliceBindings binds parameters using a running index because a slice param
// expands to a variable number of placeholders. Scalars are bound once; slices
// are bound one element at a time in a loop.
func (v Params) sliceBindings() string {
	out := []string{"var i = 1"}
	for _, f := range v.orderedFields() {
		if f.Type.IsSlice {
			elem := f.Type
			elem.IsSlice = false
			out = append(out,
				fmt.Sprintf("for (v in %s) {", f.Name),
				"    "+jdbcSetExpr(elem, "i", "v"),
				"    i++",
				"}")
		} else {
			out = append(out, jdbcSetExpr(f.Type, "i", f.Name), "i++")
		}
	}
	return indent(strings.Join(out, "\n"), 6, 0)
}

func jdbcGet(t ktType, idx int) string {
	if t.IsEnum && t.IsArray {
		return fmt.Sprintf(`(results.getArray(%d).array as Array<String>).map { v -> %s.lookup(v)!! }.toList()`, idx, t.Name)
	}
	if t.IsEnum {
		return fmt.Sprintf("%s.lookup(results.getString(%d))!!", t.Name, idx)
	}
	if t.IsArray {
		return fmt.Sprintf(`(results.getArray(%d).array as Array<%s>).toList()`, idx, t.Name)
	}
	if t.IsJson {
		if t.IsNull {
			return fmt.Sprintf(`results.getString(%d)?.let { mapper.readValue(it, %s::class.java) }`, idx, t.Name)
		}
		return fmt.Sprintf(`mapper.readValue(results.getString(%d), %s::class.java)`, idx, t.Name)
	}
	if t.IsTime() {
		return fmt.Sprintf(`results.getObject(%d, %s::class.java)`, idx, t.Name)
	}
	if t.IsInstant() {
		return fmt.Sprintf(`results.getTimestamp(%d).toInstant()`, idx)
	}
	if t.IsUUID() {
		var nullCast string
		if t.IsNull {
			nullCast = "?"
		}
		return fmt.Sprintf(`results.getObject(%d) as%s %s`, idx, nullCast, t.Name)
	}
	if t.IsBigDecimal() {
		return fmt.Sprintf(`results.getBigDecimal(%d)`, idx)
	}
	// A primitive getter returns 0/0.0/false for SQL NULL, so a nullable column needs
	// wasNull() to tell "absent" from "zero". We need to read first, then check if the
	// read value "wasNull()."
	if t.IsNull && t.isPrimitive() {
		return fmt.Sprintf(`results.get%s(%d).takeUnless { results.wasNull() }`, t.Name, idx)
	}
	return fmt.Sprintf(`results.get%s(%d)`, t.Name, idx)
}

func (v QueryValue) ResultSet() string {
	var out []string
	if v.Struct == nil {
		return jdbcGet(v.Typ, 1)
	}
	for i, f := range v.Struct.Fields {
		out = append(out, jdbcGet(f.Type, i+1))
	}
	ret := indent(strings.Join(out, ",\n"), 4, -1)
	ret = indent(v.Struct.Name+"(\n"+ret+"\n)", 12, 0)
	return ret
}

func indent(s string, n int, firstIndent int) string {
	lines := strings.Split(s, "\n")
	buf := bytes.NewBuffer(nil)
	for i, l := range lines {
		indent := n
		if i == 0 && firstIndent != -1 {
			indent = firstIndent
		}
		if i != 0 {
			buf.WriteRune('\n')
		}
		for i := 0; i < indent; i++ {
			buf.WriteRune(' ')
		}
		buf.WriteString(l)
	}
	return buf.String()
}

// A struct used to generate methods and fields on the Queries struct
type Query struct {
	ClassName    string
	Cmd          string
	Comments     []string
	MethodName   string
	FieldName    string
	ConstantName string
	SQL          string
	SourceName   string
	Ret          QueryValue
	Arg          Params
}

// SliceReplaceChain returns the Kotlin expression chained onto the SQL constant
// to expand each sqlc.slice() marker into the right number of placeholders at
// runtime (or NULL when the list is empty). It is "" when the query has no
// slices, leaving the prepared statement source byte-identical to before.
func (q Query) SliceReplaceChain() string {
	if q.Arg.Struct == nil {
		return ""
	}
	var b strings.Builder
	for _, f := range q.Arg.Struct.Fields {
		if !f.Type.IsSlice {
			continue
		}
		marker := fmt.Sprintf("/*SLICE:%s*/?", f.Type.SliceName)
		fmt.Fprintf(&b, ".replaceFirst(%q, if (%s.isEmpty()) \"NULL\" else List(%s.size) { \"?\" }.joinToString(\",\"))",
			marker, f.Name, f.Name)
	}
	return b.String()
}

func ktEnumValueName(value string) string {
	id := strings.Replace(value, "-", "_", -1)
	id = strings.Replace(id, ":", "_", -1)
	id = strings.Replace(id, "/", "_", -1)
	id = ktIdentPattern.ReplaceAllString(id, "")
	return strings.ToUpper(id)
}

func BuildEnums(req *plugin.GenerateRequest) []Enum {
	var enums []Enum
	for _, schema := range req.Catalog.Schemas {
		if schema.Name == "pg_catalog" || schema.Name == "information_schema" {
			continue
		}
		for _, enum := range schema.Enums {
			var enumName string
			if schema.Name == req.Catalog.DefaultSchema {
				enumName = enum.Name
			} else {
				enumName = schema.Name + "_" + enum.Name
			}
			e := Enum{
				Name:    dataClassName(enumName, req.Settings),
				Comment: enum.Comment,
			}
			for _, v := range enum.Vals {
				e.Constants = append(e.Constants, Constant{
					Name:  ktEnumValueName(v),
					Value: v,
					Type:  e.Name,
				})
			}
			enums = append(enums, e)
		}
	}
	if len(enums) > 0 {
		sort.Slice(enums, func(i, j int) bool { return enums[i].Name < enums[j].Name })
	}
	return enums
}

func dataClassName(name string, settings *plugin.Settings) string {
	out := ""
	for _, p := range strings.Split(name, "_") {
		out += strings.Title(p)
	}
	return out
}

func memberName(name string, settings *plugin.Settings) string {
	return sdk.LowerTitle(dataClassName(name, settings))
}

func BuildDataClasses(conf Config, req *plugin.GenerateRequest) []Struct {
	var structs []Struct
	for _, schema := range req.Catalog.Schemas {
		if schema.Name == "pg_catalog" || schema.Name == "information_schema" {
			continue
		}
		for _, table := range schema.Tables {
			var tableName string
			if schema.Name == req.Catalog.DefaultSchema {
				tableName = table.Rel.Name
			} else {
				tableName = schema.Name + "_" + table.Rel.Name
			}
			structName := dataClassName(tableName, req.Settings)
			if !conf.EmitExactTableNames {
				structName = inflection.Singular(inflection.SingularParams{
					Name:       structName,
					Exclusions: conf.InflectionExcludeTableNames,
				})
			}
			s := Struct{
				Table:   plugin.Identifier{Schema: schema.Name, Name: table.Rel.Name},
				Name:    structName,
				Comment: table.Comment,
			}
			for _, column := range table.Columns {
				typ := makeType(conf, req, column)
				// Catalog columns nested under a table do not carry their own
				// table identifier, so supply it for json override matching.
				typ.applyJsonOverride(conf, schema.Name, table.Rel.Name, column.Name)
				s.Fields = append(s.Fields, Field{
					Name:    memberName(column.Name, req.Settings),
					Type:    typ,
					Comment: column.Comment,
				})
			}
			structs = append(structs, s)
		}
	}
	if len(structs) > 0 {
		sort.Slice(structs, func(i, j int) bool { return structs[i].Name < structs[j].Name })
	}
	return structs
}

type ktType struct {
	Name      string
	IsEnum    bool
	IsArray   bool
	IsNull    bool
	IsJson    bool
	JsonType  string // fully-qualified Kotlin type for json columns
	IsSlice   bool   // sqlc.slice() param: bound as a variable-length IN list
	SliceName string // the /*SLICE:name*/ marker name for a slice param
	DataType  string
	Engine    string
}

func (t ktType) String() string {
	v := t.Name
	if t.IsArray || t.IsSlice {
		v = fmt.Sprintf("List<%s>", v)
	} else if t.IsNull {
		v += "?"
	}
	return v
}

func (t ktType) jdbcSetter() string {
	return "set" + t.jdbcType()
}

func (t ktType) jdbcType() string {
	if t.IsArray {
		return "Array"
	}
	if t.IsEnum || t.IsTime() {
		return "Object"
	}
	if t.IsInstant() {
		return "Timestamp"
	}
	return t.Name
}

func (t ktType) IsTime() bool {
	return t.Name == "LocalDate" || t.Name == "LocalDateTime" || t.Name == "LocalTime" || t.Name == "OffsetDateTime"
}

func (t ktType) IsInstant() bool {
	return t.Name == "Instant"
}

func (t ktType) IsUUID() bool {
	return t.Name == "UUID"
}

func (t ktType) IsBigDecimal() bool {
	return t.Name == "java.math.BigDecimal"
}

// isPrimitive reports whether this type is read through a JDBC primitive getter --
// ResultSet.getInt/getLong/getShort/getByte/getDouble/getFloat/getBoolean. Those return a
// zero value rather than null for SQL NULL, so a nullable one needs a wasNull() guard.
// Reference-typed getters (getString, getBigDecimal, getObject) report NULL as null.
func (t ktType) isPrimitive() bool {
	switch t.Name {
	case "Byte", "Short", "Int", "Long", "Float", "Double", "Boolean":
		return true
	default:
		return false
	}
}

// isJSONColumn reports whether a database data type is a json/jsonb column
// eligible for deserialization into a Kotlin data class.
func isJSONColumn(dataType string) bool {
	switch dataType {
	case "json", "jsonb":
		return true
	default:
		return false
	}
}

// colTable returns the schema and table a column belongs to, if known.
func colTable(col *plugin.Column) (string, string) {
	if t := col.GetTable(); t != nil {
		return t.GetSchema(), t.GetName()
	}
	return "", ""
}

func makeType(conf Config, req *plugin.GenerateRequest, col *plugin.Column) ktType {
	typ, isEnum := ktInnerType(req, col)
	t := ktType{
		Name:     typ,
		IsEnum:   isEnum,
		IsArray:  col.IsArray,
		IsNull:   !col.NotNull,
		IsSlice:  col.IsSqlcSlice,
		DataType: sdk.DataType(col.Type),
		Engine:   req.Settings.Engine,
	}
	if t.IsSlice {
		t.SliceName = col.Name
	}
	schema, table := colTable(col)
	t.applyJsonOverride(conf, schema, table, col.Name)
	return t
}

// applyJsonOverride upgrades a jsonb/json column to a user-provided Kotlin type
// when a matching override is configured. It is a no-op otherwise, so it is safe
// to call again with richer table context (e.g. from BuildDataClasses, where the
// catalog column does not carry its own table identifier).
func (t *ktType) applyJsonOverride(conf Config, schema, table, column string) {
	if t.IsJson || !conf.jsonEnabled() || !isJSONColumn(t.DataType) {
		return
	}
	if kt, ok := conf.jsonTypeFor(schema, table, column); ok {
		t.Name = SimpleName(kt)
		t.JsonType = kt
		t.IsJson = true
	}
}

func ktInnerType(req *plugin.GenerateRequest, col *plugin.Column) (string, bool) {
	// TODO: Extend the engine interface to handle types
	switch req.Settings.Engine {
	case "mysql":
		return mysqlType(req, col)
	case "postgresql":
		return postgresType(req, col)
	default:
		return "Any", false
	}
}

type goColumn struct {
	id int
	*plugin.Column
}

func ktColumnsToStruct(conf Config, req *plugin.GenerateRequest, name string, columns []goColumn, namer func(*plugin.Column, int) string) *Struct {
	gs := Struct{
		Name: name,
	}
	idSeen := map[int]Field{}
	nameSeen := map[string]int{}
	for _, c := range columns {
		if _, ok := idSeen[c.id]; ok {
			continue
		}
		fieldName := memberName(namer(c.Column, c.id), req.Settings)
		if v := nameSeen[c.Name]; v > 0 {
			fieldName = fmt.Sprintf("%s_%d", fieldName, v+1)
		}
		field := Field{
			ID:   c.id,
			Name: fieldName,
			Type: makeType(conf, req, c.Column),
		}
		gs.Fields = append(gs.Fields, field)
		nameSeen[c.Name]++
		idSeen[c.id] = field
	}
	return &gs
}

func ktArgName(name string) string {
	out := ""
	for i, p := range strings.Split(name, "_") {
		if i == 0 {
			out += strings.ToLower(p)
		} else {
			out += strings.Title(p)
		}
	}
	return out
}

func ktParamName(c *plugin.Column, number int) string {
	if c.Name != "" {
		return ktArgName(c.Name)
	}
	return fmt.Sprintf("dollar_%d", number)
}

func ktColumnName(c *plugin.Column, pos int) string {
	if c.Name != "" {
		return c.Name
	}
	return fmt.Sprintf("column_%d", pos+1)
}

var postgresPlaceholderRegexp = regexp.MustCompile(`\B\$\d+\b`)

// HACK: jdbc doesn't support numbered parameters, so we need to transform them to question marks...
// But there's no access to the SQL parser here, so we just do a dumb regexp replace instead. This won't work if
// the literal strings contain matching values, but good enough for a prototype.
//
// sliceParams maps a 1-based parameter number to its sqlc.slice() marker name.
// Postgres hands us slice params as a plain "$N" (unlike MySQL, which already
// emits "/*SLICE:name*/?"); we rewrite those to the same marker so the runtime
// IN-list expansion is uniform across engines.
func jdbcSQL(s, engine string, sliceParams map[int]string) (string, []string) {
	if engine != "postgresql" {
		return s, nil
	}
	var args []string
	q := postgresPlaceholderRegexp.ReplaceAllStringFunc(s, func(placeholder string) string {
		args = append(args, placeholder)
		if n, err := strconv.Atoi(strings.TrimPrefix(placeholder, "$")); err == nil {
			if name, ok := sliceParams[n]; ok {
				return fmt.Sprintf("/*SLICE:%s*/?", name)
			}
		}
		return "?"
	})
	return q, args
}

func parseInts(s []string) ([]int, error) {
	if len(s) == 0 {
		return nil, nil
	}
	var refs []int
	for _, v := range s {
		i, err := strconv.Atoi(strings.TrimPrefix(v, "$"))
		if err != nil {
			return nil, err
		}
		refs = append(refs, i)
	}
	return refs, nil
}

func BuildQueries(conf Config, req *plugin.GenerateRequest, structs []Struct) ([]Query, error) {
	qs := make([]Query, 0, len(req.Queries))
	for _, query := range req.Queries {
		if query.Name == "" {
			continue
		}
		if query.Cmd == "" {
			continue
		}
		if query.Cmd == metadata.CmdCopyFrom {
			return nil, errors.New("Support for CopyFrom in Kotlin is not implemented")
		}

		sliceParams := map[int]string{}
		for _, p := range query.Params {
			if p.Column.GetIsSqlcSlice() {
				sliceParams[int(p.Number)] = p.Column.Name
			}
		}

		ql, args := jdbcSQL(query.Text, req.Settings.Engine, sliceParams)
		refs, err := parseInts(args)
		if err != nil {
			return nil, fmt.Errorf("Invalid parameter reference: %w", err)
		}
		gq := Query{
			Cmd:          query.Cmd,
			ClassName:    strings.Title(query.Name),
			ConstantName: sdk.LowerTitle(query.Name),
			FieldName:    sdk.LowerTitle(query.Name) + "Stmt",
			MethodName:   sdk.LowerTitle(query.Name),
			SourceName:   query.Filename,
			SQL:          ql,
			Comments:     query.Comments,
		}

		var cols []goColumn
		for _, p := range query.Params {
			cols = append(cols, goColumn{
				id:     int(p.Number),
				Column: p.Column,
			})
		}
		params := ktColumnsToStruct(conf, req, gq.ClassName+"Bindings", cols, ktParamName)
		gq.Arg = Params{
			Struct:  params,
			binding: refs,
		}

		if len(query.Columns) == 1 {
			c := query.Columns[0]
			gq.Ret = QueryValue{
				Name: "results",
				Typ:  makeType(conf, req, c),
			}
		} else if len(query.Columns) > 1 {
			var gs *Struct
			var emit bool

			for _, s := range structs {
				if len(s.Fields) != len(query.Columns) {
					continue
				}
				same := true
				for i, f := range s.Fields {
					c := query.Columns[i]
					sameName := f.Name == memberName(ktColumnName(c, i), req.Settings)
					sameType := f.Type == makeType(conf, req, c)
					sameTable := sdk.SameTableName(c.Table, &s.Table, req.Catalog.DefaultSchema)

					if !sameName || !sameType || !sameTable {
						same = false
					}
				}
				if same {
					gs = &s
					break
				}
			}

			if gs == nil {
				var columns []goColumn
				for i, c := range query.Columns {
					columns = append(columns, goColumn{
						id:     i,
						Column: c,
					})
				}
				gs = ktColumnsToStruct(conf, req, gq.ClassName+"Row", columns, ktColumnName)
				emit = true
			}
			gq.Ret = QueryValue{
				Emit:   emit,
				Name:   "results",
				Struct: gs,
			}
		}

		qs = append(qs, gq)
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].MethodName < qs[j].MethodName })
	return qs, nil
}

type KtTmplCtx struct {
	Q           string
	Package     string
	Enums       []Enum
	DataClasses []Struct
	Queries     []Query
	Settings    *plugin.Settings
	SqlcVersion string

	// TODO: Race conditions
	SourceName string

	EmitJSONTags        bool
	EmitPreparedQueries bool
	EmitInterface       bool

	// JsonMapperClass is the simple (unqualified) name of the JsonMapper to
	// inject into QueriesImpl, set only when a query in this file reads or
	// writes a json column. Empty means no JsonMapper is needed.
	JsonMapperClass string
}

func Offset(v int) int {
	return v + 1
}

func KtFormat(s string) string {
	// TODO: do more than just skip multiple blank lines, like maybe run ktlint to format
	skipNextSpace := false
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		isSpace := len(strings.TrimSpace(l)) == 0
		if !isSpace || !skipNextSpace {
			lines = append(lines, l)
		}
		skipNextSpace = isSpace
	}
	o := strings.Join(lines, "\n")
	o += "\n"
	return o
}
