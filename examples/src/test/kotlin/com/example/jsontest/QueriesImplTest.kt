package com.example.jsontest

import com.example.dbtest.PostgresDbTestExtension
import com.example.jsontest.dto.Payload
import org.junit.jupiter.api.Assertions
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.extension.RegisterExtension
import tools.jackson.databind.json.JsonMapper
import tools.jackson.module.kotlin.kotlinModule

class QueriesImplTest {

    companion object {
        @JvmField
        @RegisterExtension
        val dbtest = PostgresDbTestExtension("src/main/resources/jsontest/postgresql/schema.sql")

        val mapper: JsonMapper = JsonMapper.builder().addModule(kotlinModule()).build()
    }

    @Test
    fun testJsonRoundTrip() {
        val db = QueriesImpl(dbtest.getConnection(), mapper)

        val payload = Payload(kind = "click", count = 3, tags = listOf("a", "b"))
        val metadata = Payload(kind = "meta", count = 1, tags = listOf("x"))

        val created = db.createEvent(name = "signup", payload = payload, metadata = metadata)!!
        Assertions.assertEquals(payload, created.payload)
        Assertions.assertEquals(metadata, created.metadata)

        val fetched = db.getEvent(created.id)!!
        Assertions.assertEquals(payload, fetched.payload)
        Assertions.assertEquals(metadata, fetched.metadata)

        val all = db.listEvents()
        Assertions.assertEquals(1, all.size)
        Assertions.assertEquals(payload, all[0].payload)
    }

    @Test
    fun testNullableJsonColumn() {
        val db = QueriesImpl(dbtest.getConnection(), mapper)

        val payload = Payload(kind = "view", count = 0, tags = emptyList())
        val created = db.createEvent(name = "nometa", payload = payload, metadata = null)!!
        Assertions.assertNull(created.metadata)

        val fetched = db.getEvent(created.id)!!
        Assertions.assertNull(fetched.metadata)
        Assertions.assertEquals(payload, fetched.payload)
    }
}
