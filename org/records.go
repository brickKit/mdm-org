package org

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/jackc/pgx/v5"
)

// LegalEntity is a legal entity's complete state: the payload of mdm.legal_entity.changed.v1 and of its reads.
type LegalEntity struct {
	ID                   string            `json:"id"`
	Code                 string            `json:"code"`
	Name                 string            `json:"name"`
	Names                map[string]string `json:"names"`
	Country              string            `json:"country"`
	Timezone             string            `json:"timezone"`
	BaseCurrency         string            `json:"base_currency"`
	FiscalYearStartMonth int               `json:"fiscal_year_start_month"`
	State                string            `json:"state"`
	Version              int64             `json:"version"`
	UpdatedAt            string            `json:"updated_at"`
}

// Unit is an organization unit's complete state (mdm.org_unit.changed.v1).
type Unit struct {
	ID            string            `json:"id"`
	LegalEntityID string            `json:"legal_entity_id"`
	ParentID      *string           `json:"parent_id"`
	Code          string            `json:"code"`
	Name          string            `json:"name"`
	Names         map[string]string `json:"names"`
	Path          string            `json:"path"`
	State         string            `json:"state"`
	Version       int64             `json:"version"`
	UpdatedAt     string            `json:"updated_at"`
}

const (
	entityCols = `id::text, code, name, names, country, timezone, base_currency, fiscal_year_start_month, state, version,
		updated_at`
	unitCols = `id::text, legal_entity_id::text, parent_id::text, code, name, names, path, state, version, updated_at`
)

func scanEntity(row pgx.Row) (LegalEntity, error) {
	var e LegalEntity
	var names []byte
	var at time.Time
	err := row.Scan(&e.ID, &e.Code, &e.Name, &names, &e.Country, &e.Timezone, &e.BaseCurrency, &e.FiscalYearStartMonth,
		&e.State, &e.Version, &at)
	if err == nil {
		e.UpdatedAt, err = canon.Time(at), json.Unmarshal(names, &e.Names)
	}
	return e, err
}

func scanUnit(row pgx.Row) (Unit, error) {
	var u Unit
	var names []byte
	var at time.Time
	err := row.Scan(&u.ID, &u.LegalEntityID, &u.ParentID, &u.Code, &u.Name, &names, &u.Path, &u.State, &u.Version, &at)
	if err == nil {
		u.UpdatedAt, err = canon.Time(at), json.Unmarshal(names, &u.Names)
	}
	return u, err
}

// errNone is a lookup that found no row.
var errNone = errors.New("no such record")

func one[T any](row pgx.Row, scan func(pgx.Row) (T, error)) (T, error) {
	v, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, errNone
	}
	return v, err
}

func getEntity(ctx context.Context, tx db.Tx, id string, lock bool) (LegalEntity, error) {
	q := `SELECT ` + entityCols + ` FROM legal_entity WHERE id = $1::uuid`
	if lock {
		q += ` FOR UPDATE`
	}
	return one(tx.QueryRow(ctx, q, id), scanEntity)
}

func getUnit(ctx context.Context, tx db.Tx, id string, lock bool) (Unit, error) {
	q := `SELECT ` + unitCols + ` FROM org_unit WHERE id = $1::uuid`
	if lock {
		q += ` FOR UPDATE`
	}
	return one(tx.QueryRow(ctx, q, id), scanUnit)
}

// emit writes the record's full-state event into the outbox, in the transaction that changed it (protocol 02, S2).
func (m *Module) emit(ctx context.Context, tx db.Tx, typ, id string, version int64, data any) error {
	_, err := m.rt.Events.Emit(ctx, tx, Stream, events.Event{Type: typ, RecordID: id, Version: version,
		Projection: events.Full, Data: data})
	return err
}
