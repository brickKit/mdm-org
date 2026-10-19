-- Legal entities and organization units (departments). Every record has a version (protocol 02, S1) and is published
-- as a full-state event in the transaction that changed it. Nothing is deleted: retired records stay readable.
-- search is the list keyword's normalized column (protocol 14), written by the module with each change.
CREATE TABLE legal_entity (
    id                      uuid PRIMARY KEY,
    code                    text NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    name                    text NOT NULL CHECK (name <> ''),
    names                   jsonb NOT NULL DEFAULT '{}'::jsonb,
    country                 text NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
    timezone                text NOT NULL CHECK (timezone <> ''),
    base_currency           text NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'),
    fiscal_year_start_month smallint NOT NULL DEFAULT 1 CHECK (fiscal_year_start_month BETWEEN 1 AND 12),
    state                   text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'suspended', 'retired')),
    search                  text NOT NULL,
    search_version          integer NOT NULL,
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

-- path: the legal entity's ID, then the unit IDs from the top down to this one, joined by / (security 07's org-tree
-- fact, compared by segment); it changes when the unit or one above it moves, never when one is renamed
CREATE TABLE org_unit (
    id              uuid PRIMARY KEY,
    legal_entity_id uuid NOT NULL REFERENCES legal_entity (id),
    parent_id       uuid REFERENCES org_unit (id),
    code            text NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    name            text NOT NULL CHECK (name <> ''),
    names           jsonb NOT NULL DEFAULT '{}'::jsonb,
    path            text NOT NULL UNIQUE,
    state           text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'retired')),
    search          text NOT NULL,
    search_version  integer NOT NULL,
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (legal_entity_id, code),
    CHECK (parent_id IS NULL OR parent_id <> id)
);
-- a subtree is a path prefix
CREATE INDEX org_unit_subtree ON org_unit (path text_pattern_ops);
CREATE INDEX org_unit_children ON org_unit (parent_id, path);

-- every path a unit has had (the boundary card: paths are traceable); the current one has no end
CREATE TABLE org_unit_path_history (
    unit_id    uuid NOT NULL REFERENCES org_unit (id),
    path       text NOT NULL,
    valid_from timestamptz NOT NULL,
    valid_to   timestamptz,
    PRIMARY KEY (unit_id, valid_from)
);
CREATE UNIQUE INDEX org_unit_path_current ON org_unit_path_history (unit_id) WHERE valid_to IS NULL;
