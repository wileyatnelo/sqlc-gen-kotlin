package core

import "testing"

// numeric columns map to the qualified name java.math.BigDecimal so the generated Kotlin
// needs no import for them. The JDBC setter name, though, is built from that same field,
// and "set" + "java.math.BigDecimal" is not a method -- it is not even valid Kotlin. So a
// numeric parameter needs the same explicit routing to setBigDecimal that jdbcGet already
// gives getBigDecimal.
func TestJdbcSetBigDecimal(t *testing.T) {
	tests := []struct {
		name string
		typ  ktType
		want string
	}{
		{"not null", ktType{Name: "java.math.BigDecimal"}, "stmt.setBigDecimal(1, amount)"},
		// setBigDecimal(idx, null) binds SQL NULL on its own, so nullable needs no setNull branch.
		{"nullable", ktType{Name: "java.math.BigDecimal", IsNull: true}, "stmt.setBigDecimal(1, amount)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jdbcSet(tt.typ, 1, "amount"); got != tt.want {
				t.Fatalf("jdbcSet(%s) = %q, want %q", tt.typ.String(), got, tt.want)
			}
		})
	}
}

// numeric[] binds through setArray, and a sqlc.slice() of numeric binds element by element.
// Both carry Name == "java.math.BigDecimal", so the BigDecimal case must not shadow them.
func TestJdbcSetBigDecimalArrayAndSlice(t *testing.T) {
	arr := ktType{Name: "java.math.BigDecimal", IsArray: true, DataType: "numeric"}
	wantArr := `stmt.setArray(1, conn.createArrayOf("numeric", amounts.toTypedArray()))`
	if got := jdbcSet(arr, 1, "amounts"); got != wantArr {
		t.Fatalf("jdbcSet(BigDecimal[]) = %q, want %q", got, wantArr)
	}

	elem := ktType{Name: "java.math.BigDecimal"}
	wantElem := "stmt.setBigDecimal(i, v)"
	if got := jdbcSetExpr(elem, "i", "v"); got != wantElem {
		t.Fatalf("jdbcSetExpr(BigDecimal slice element) = %q, want %q", got, wantElem)
	}
}
