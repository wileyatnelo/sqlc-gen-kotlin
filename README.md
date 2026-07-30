## Usage

```yaml
version: '2'
plugins:
- name: kt
  wasm:
    url: https://downloads.sqlc.dev/plugin/sqlc-gen-kotlin_1.2.0.wasm
    sha256: 22b437ecaea66417bbd3b958339d9868ba89368ce542c936c37305acf373104b
sql:
- schema: src/main/resources/authors/postgresql/schema.sql
  queries: src/main/resources/authors/postgresql/query.sql
  engine: postgresql
  codegen:
  - out: src/main/kotlin/com/example/authors/postgresql
    plugin: kt
    options:
      package: com.example.authors.postgresql
```

## Options

| Option                           | Description                                                                                          |
| -------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `package`                        | Kotlin package for the generated code.                                                               |
| `emit_exact_table_names`         | Keep table names as-is instead of singularizing them for data class names.                           |
| `inflection_exclude_table_names` | Table names to exclude from singularization.                                                         |
| `json_serializer`                | Enables jsonb deserialization. `jackson2` (`com.fasterxml.jackson`) or `jackson3` (`tools.jackson`). |
| `json_types`                     | List of `{column, kt_type}` entries mapping a `json`/`jsonb` column to a Kotlin type.                |

### Deserializing `jsonb` into Kotlin data classes

By default a `jsonb` column maps to `String`. With `json_serializer` enabled and a
matching `json_types` entry, the column is deserialized into a Kotlin data class you
provide. The plugin references your class by its fully-qualified name — it does not
generate it.

```yaml
  - out: src/main/kotlin/com/example/events
    plugin: kt
    options:
      package: com.example.events
      json_serializer: jackson3       # or jackson2
      json_types:
      - column: events.payload          # table.column or schema.table.column
        kt_type: com.example.events.dto.Payload
```

`QueriesImpl` then takes a `JsonMapper` in its constructor and uses it to
serialize/deserialize the column:

```kotlin
val mapper = JsonMapper.builder().addModule(kotlinModule()).build()
val db = QueriesImpl(conn, mapper)
```

See `examples/src/main/resources/jsontest` for example usage.

## IN-list queries with `sqlc.slice()`

`sqlc.slice()` lets a single parameter expand into a variable-length `IN (...)` list:

```sql
-- name: ListEventsByIds :many
SELECT id, name, payload, metadata FROM events
WHERE id IN (sqlc.slice('ids'))
ORDER BY id;
```

generates a `List` parameter and expands the placeholders at runtime:

```kotlin
fun listEventsByIds(ids: List<Long>): List<Event>
```

An empty list expands to `IN (NULL)`, which matches no rows. This works on both
MySQL and PostgreSQL. (On PostgreSQL you can also use the native array form,
`WHERE id = ANY($1::bigint[])`, which likewise produces a `List` parameter.)

## Nullable numeric and boolean columns

JDBC's primitive getters — `getInt`, `getLong`, `getShort`, `getByte`, `getDouble`,
`getFloat`, `getBoolean` — return `0`/`0.0`/`false` for a SQL NULL, so on their own
they cannot distinguish "absent" from "zero". Row mappers therefore guard a nullable
column with `wasNull()`:

```kotlin
data class Reading (
  val countBig: Long?,
  val requiredBig: Long
)

// generated read
Reading(
    results.getLong(1).takeUnless { results.wasNull() },  // nullable -> guarded
    results.getLong(2)                                    // NOT NULL -> plain read
)
```

A genuine `0` still reads as `0`; only SQL NULL becomes `null`. `NOT NULL` columns are
read directly, and getters that already return a reference type (`getString`,
`getBigDecimal`, `getObject`) need no guard because they report NULL as `null`.

See `examples/src/main/resources/nulltest` for example usage.
