-- How far each partition of the journal has been taken: the offset of the
-- next record to take. Workers start every partition they are given from
-- here, not from offsets kept in Kafka (ADR-0009).
CREATE TABLE journal_offsets (
    topic       text   NOT NULL,
    partition   int    NOT NULL,
    next_offset bigint NOT NULL,
    PRIMARY KEY (topic, partition)
);

-- The records taken from the journal, as they were sent. It stands in for
-- interpretation in the walking skeleton and goes when interpretation comes.
CREATE TABLE skeleton_records (
    topic          text        NOT NULL,
    partition      int         NOT NULL,
    journal_offset bigint      NOT NULL,
    source         text        NOT NULL,
    body           text        NOT NULL,
    contract       text        NOT NULL,
    traceparent    text        NOT NULL,
    taken_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (topic, partition, journal_offset)
);

CREATE INDEX skeleton_records_by_source
    ON skeleton_records (source, topic, partition, journal_offset);
