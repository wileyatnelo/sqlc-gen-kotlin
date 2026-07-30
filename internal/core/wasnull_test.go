package core

import "testing"

// A nullable column whose JDBC getter is a primitive getter must be guarded with
// wasNull(). getInt/getLong/getBoolean/getDouble/getFloat/getShort/getByte return
// 0/0.0/false for SQL NULL, so without the guard a NULL is indistinguishable from a
// genuine zero — and the emitted Kotlin type is already nullable, so the value silently
// contradicts its own declared type.
func TestJdbcGetNullablePrimitiveGuardsWasNull(t *testing.T) {
	for _, name := range []string{"Long", "Int", "Short", "Byte", "Double", "Float", "Boolean"} {
		t.Run(name, func(t *testing.T) {
			nullable := ktType{Name: name, IsNull: true}
			want := "results.get" + name + "(4).takeUnless { results.wasNull() }"
			if got := jdbcGet(nullable, 4); got != want {
				t.Fatalf("jdbcGet(%s?) = %q, want %q", name, got, want)
			}

			notNull := ktType{Name: name}
			wantNotNull := "results.get" + name + "(4)"
			if got := jdbcGet(notNull, 4); got != wantNotNull {
				t.Fatalf("jdbcGet(%s) = %q, want %q", name, got, wantNotNull)
			}
		})
	}
}

// Getters that already return a reference type report NULL as null on their own. Wrapping
// them would be pointless churn, and for String it would change a plain getString into a
// boxed round-trip.
func TestJdbcGetNullableReferenceTypesUnwrapped(t *testing.T) {
	tests := []struct {
		name string
		typ  ktType
		want string
	}{
		{"String", ktType{Name: "String", IsNull: true}, `results.getString(4)`},
		{"BigDecimal", ktType{Name: "java.math.BigDecimal", IsNull: true}, `results.getBigDecimal(4)`},
		{"UUID", ktType{Name: "UUID", IsNull: true}, `results.getObject(4) as? UUID`},
		{"OffsetDateTime", ktType{Name: "OffsetDateTime", IsNull: true}, `results.getObject(4, OffsetDateTime::class.java)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jdbcGet(tt.typ, 4); got != tt.want {
				t.Fatalf("jdbcGet(%s?) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// An array column is read through getArray, not a primitive getter, so element nullability
// must not pull in a wasNull() guard.
func TestJdbcGetNullablePrimitiveArrayUnwrapped(t *testing.T) {
	arr := ktType{Name: "Int", IsNull: true, IsArray: true}
	want := `(results.getArray(4).array as Array<Int>).toList()`
	if got := jdbcGet(arr, 4); got != want {
		t.Fatalf("jdbcGet(Int[]?) = %q, want %q", got, want)
	}
}
