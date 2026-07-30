-- Nullable columns of every type that is read through a JDBC primitive getter
-- (getLong/getInt/getShort/getDouble/getFloat/getBoolean). Those getters return 0/0.0/false
-- for SQL NULL, so the generated row mapper must guard them with wasNull(). The
-- `required_*` columns are the NOT NULL control group: they must stay unguarded.
CREATE TABLE readings (
    id            BIGSERIAL PRIMARY KEY,
    label         text NOT NULL,
    count_big     bigint,
    count_int     integer,
    count_small   smallint,
    ratio         double precision,
    ratio_real    real,
    enabled       boolean,
    required_big  bigint NOT NULL,
    required_flag boolean NOT NULL
);
