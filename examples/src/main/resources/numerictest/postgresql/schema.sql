-- numeric is the one type whose Kotlin mapping is a qualified name, java.math.BigDecimal,
-- so that it needs no import. That name cannot double as a JDBC method suffix the way
-- "String" or "Long" does, so both directions need explicit routing: getBigDecimal to read
-- and setBigDecimal to bind. Only a numeric *parameter* exercises the binding side, which
-- is why this table is written to as well as read from.
CREATE TABLE ledger (
    id     BIGSERIAL PRIMARY KEY,
    label  text NOT NULL,
    amount numeric NOT NULL,
    fee    numeric
);
