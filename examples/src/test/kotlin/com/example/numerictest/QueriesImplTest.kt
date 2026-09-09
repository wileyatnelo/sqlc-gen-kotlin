package com.example.numerictest

import com.example.dbtest.PostgresDbTestExtension
import org.junit.jupiter.api.Assertions
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.extension.RegisterExtension
import java.math.BigDecimal

/**
 * A numeric parameter has to bind through `setBigDecimal`. Every other type takes its JDBC
 * method name straight from its Kotlin type name, but numeric maps to the qualified
 * `java.math.BigDecimal`, and `stmt.setjava.math.BigDecimal(...)` does not compile -- so
 * before the fix these queries could not be built at all, in any of the four shapes below:
 * a NOT NULL parameter, a nullable one, a comparison in a WHERE clause, and an element of a
 * `sqlc.slice()` IN-list.
 *
 * Values are compared with `compareTo`, not `equals`, because BigDecimal.equals is
 * scale-sensitive: Postgres hands back 10.50 for an unqualified `numeric`, which is a
 * different object from 10.5 while being the same number. Scale is not what this test is about.
 */
class QueriesImplTest {

    companion object {
        @JvmField
        @RegisterExtension
        val dbtest = PostgresDbTestExtension("src/main/resources/numerictest/postgresql/schema.sql")
    }

    private fun assertEqualsNumeric(expected: BigDecimal, actual: BigDecimal?, message: String) {
        Assertions.assertNotNull(actual, message)
        Assertions.assertEquals(
            0,
            expected.compareTo(actual),
            "$message: expected $expected, got $actual",
        )
    }

    @Test
    fun `numeric parameters round-trip`() {
        val conn = dbtest.getConnection()
        val queries = QueriesImpl(conn)

        val created = queries.createEntry("rent", BigDecimal("1234.56"), BigDecimal("7.89"))!!
        val read = queries.getEntry(created.id)!!

        Assertions.assertEquals("rent", read.label)
        assertEqualsNumeric(BigDecimal("1234.56"), read.amount, "numeric NOT NULL parameter")
        assertEqualsNumeric(BigDecimal("7.89"), read.fee, "nullable numeric parameter")
    }

    @Test
    fun `a null numeric parameter binds SQL NULL`() {
        val conn = dbtest.getConnection()
        val queries = QueriesImpl(conn)

        val created = queries.createEntry("no-fee", BigDecimal("10"), null)!!

        Assertions.assertNull(created.fee, "setBigDecimal(idx, null) must bind SQL NULL")
        Assertions.assertNull(queries.getEntry(created.id)!!.fee)
    }

    @Test
    fun `numeric precision survives the round-trip`() {
        val conn = dbtest.getConnection()
        val queries = QueriesImpl(conn)

        // Well past what a Double could hold exactly -- the point of numeric over float8, and
        // the reason binding it as anything else would be a silent data-loss bug rather than
        // just a compile error.
        val exact = BigDecimal("123456789012345678901234567890.123456789")
        val created = queries.createEntry("exact", exact, null)!!

        assertEqualsNumeric(exact, queries.getEntry(created.id)!!.amount, "numeric precision")
    }

    @Test
    fun `a numeric comparison parameter filters`() {
        val conn = dbtest.getConnection()
        val queries = QueriesImpl(conn)

        queries.createEntry("small", BigDecimal("5.00"), null)
        queries.createEntry("large", BigDecimal("50.00"), null)

        val over = queries.listEntriesOver(BigDecimal("10"))

        Assertions.assertEquals(listOf("large"), over.map { it.label })
    }

    @Test
    fun `each element of a numeric slice binds`() {
        val conn = dbtest.getConnection()
        val queries = QueriesImpl(conn)

        queries.createEntry("a", BigDecimal("1.10"), null)
        queries.createEntry("b", BigDecimal("2.20"), null)
        queries.createEntry("c", BigDecimal("3.30"), null)

        val matched = queries.listEntriesByAmounts(listOf(BigDecimal("1.10"), BigDecimal("3.30")))

        Assertions.assertEquals(listOf("a", "c"), matched.map { it.label })
    }

    @Test
    fun `an empty numeric slice matches nothing`() {
        val conn = dbtest.getConnection()
        val queries = QueriesImpl(conn)

        queries.createEntry("a", BigDecimal("1.10"), null)

        Assertions.assertEquals(emptyList<Ledger>(), queries.listEntriesByAmounts(emptyList()))
    }
}
