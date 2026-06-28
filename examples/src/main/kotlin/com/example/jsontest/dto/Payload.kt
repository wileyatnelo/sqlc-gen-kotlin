package com.example.jsontest.dto

/**
 * A hand-written Kotlin data class that a jsonb column is deserialized into.
 * The plugin references this type by its fully-qualified name (configured via
 * the `overrides` option); it does not generate it.
 */
data class Payload(
    val kind: String = "",
    val count: Int = 0,
    val tags: List<String> = emptyList()
)
