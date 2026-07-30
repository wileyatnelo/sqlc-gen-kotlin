package com.example.nulltest

import com.example.dbtest.PostgresDbTestExtension
import org.junit.jupiter.api.Assertions
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.extension.RegisterExtension
import java.sql.Connection

/**
 * A SQL NULL in a column read through a JDBC primitive getter must arrive in Kotlin as `null`,
 * not as the getter's zero value. `getLong`/`getInt`/`getShort`/`getDouble`/`getFloat`/`getBoolean`
 * all return 0/0.0/false for NULL, so the generated mapper has to consult `wasNull()`.
 *
 * The zero-valued row is the half that makes this test meaningful: without it, a mapper that
 * returned `null` for everything would also pass.
 */
class QueriesImplTest {

    companion object {
        @JvmField
        @RegisterExtension
        val dbtest = PostgresDbTestExtension("src/main/resources/nulltest/postgresql/schema.sql")
    }

    private fun seedNullRow(conn: Connection): Long = insert(
        conn,
        "INSERT INTO readings (label, required_big, required_flag) " +
            "VALUES ('all-null', 7, true) RETURNING id",
    )

    private fun seedZeroRow(conn: Connection): Long = insert(
        conn,
        "INSERT INTO readings (label, count_big, count_int, count_small, ratio, ratio_real, " +
            "enabled, required_big, required_flag) " +
            "VALUES ('all-zero', 0, 0, 0, 0.0, 0.0, false, 0, false) RETURNING id",
    )

    private fun insert(conn: Connection, sql: String): Long = conn.createStatement().executeQuery(sql).use {
        it.next()
        it.getLong(1)
    }

    @Test
    fun `null columns read back as null, not as zero`() {
        val conn = dbtest.getConnection()
        val id = seedNullRow(conn)

        val row = QueriesImpl(conn).getReading(id)!!

        Assertions.assertNull(row.countBig, "bigint NULL must not read as 0L")
        Assertions.assertNull(row.countInt, "integer NULL must not read as 0")
        Assertions.assertNull(row.countSmall, "smallint NULL must not read as 0")
        Assertions.assertNull(row.ratio, "double precision NULL must not read as 0.0")
        Assertions.assertNull(row.ratioReal, "real NULL must not read as 0.0f")
        Assertions.assertNull(row.enabled, "boolean NULL must not read as false")

        // NOT NULL columns are unaffected.
        Assertions.assertEquals("all-null", row.label)
        Assertions.assertEquals(7L, row.requiredBig)
        Assertions.assertEquals(true, row.requiredFlag)
    }

    @Test
    fun `genuine zero values read back as zero, not as null`() {
        val conn = dbtest.getConnection()
        val id = seedZeroRow(conn)

        val row = QueriesImpl(conn).getReading(id)!!

        Assertions.assertEquals(0L, row.countBig)
        Assertions.assertEquals(0, row.countInt)
        Assertions.assertEquals(0.toShort(), row.countSmall)
        Assertions.assertEquals(0.0, row.ratio)
        Assertions.assertEquals(0.0f, row.ratioReal)
        Assertions.assertEquals(false, row.enabled)
        Assertions.assertEquals(false, row.requiredFlag)
    }

    @Test
    fun `null and zero stay distinguishable across a multi-row read`() {
        val conn = dbtest.getConnection()
        seedNullRow(conn)
        seedZeroRow(conn)

        val rows = QueriesImpl(conn).listReadings()

        Assertions.assertEquals(listOf("all-null", "all-zero"), rows.map { it.label })
        Assertions.assertEquals(listOf(null, 0L), rows.map { it.countBig })
        Assertions.assertEquals(listOf(null, false), rows.map { it.enabled })
        Assertions.assertEquals(listOf(null, 0.0), rows.map { it.ratio })
    }
}
