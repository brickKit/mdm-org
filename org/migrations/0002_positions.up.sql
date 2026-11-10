-- Positions (3.0.34): a post within one unit, on which mdm/employee's appointments hang. A position never moves to
-- another unit (a new one is created there), so it keeps its unit for life; retired is final.
CREATE TABLE org_position (
    id              uuid PRIMARY KEY,
    legal_entity_id uuid NOT NULL REFERENCES legal_entity (id),
    unit_id         uuid NOT NULL REFERENCES org_unit (id),
    code            text NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    name            text NOT NULL CHECK (name <> ''),
    names           jsonb NOT NULL DEFAULT '{}'::jsonb,
    state           text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'retired')),
    search          text NOT NULL,
    search_version  integer NOT NULL,
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (legal_entity_id, code)
);
-- a unit's positions (the list by unit; a unit's retirement checks for active ones)
CREATE INDEX org_position_unit ON org_position (unit_id, code);
