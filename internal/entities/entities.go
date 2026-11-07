package entities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/brickKit/mdm-org/v3/internal/core"
	tightening "github.com/brickKit/mdm-org/v3/internal/tightening"

	"github.com/brickKit/be-sdk-go/v3/bizdate"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/evidence"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/list"
	"github.com/brickKit/be-sdk-go/v3/search"
)

type entityIn struct {
	Version              *int64             `json:"version"`
	Code                 string             `json:"code"`
	Name                 *string            `json:"name"`
	Names                *map[string]string `json:"names"`
	Country              string             `json:"country"`
	Timezone             *string            `json:"timezone"`
	BaseCurrency         *string            `json:"base_currency"`
	FiscalYearStartMonth *int               `json:"fiscal_year_start_month"`
}

// validate checks what is given; create says which fields must be there.
func (in entityIn) validate(create bool) error {
	switch {
	case create && !core.CodeForm.MatchString(in.Code):
		return core.Invalid("code", "code is upper case letters, digits, _ and -, at most 32")
	case create && !core.Country.MatchString(in.Country):
		return core.Invalid("country", "country is an ISO 3166-1 alpha-2 code")
	case create && (in.Name == nil || in.BaseCurrency == nil):
		return core.Invalid("name", "name and base_currency are required")
	case in.Name != nil && strings.TrimSpace(*in.Name) == "":
		return core.Invalid("name", "name cannot be empty")
	case in.BaseCurrency != nil && !core.IsoCcy.MatchString(*in.BaseCurrency):
		return core.Invalid("base_currency", "base_currency is an ISO 4217 code")
	case in.FiscalYearStartMonth != nil && (*in.FiscalYearStartMonth < 1 || *in.FiscalYearStartMonth > 12):
		return core.Invalid("fiscal_year_start_month", "fiscal_year_start_month is 1-12")
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil || *in.Timezone == "" || *in.Timezone == "Local" {
			return core.Invalid("timezone", "timezone is an IANA time zone")
		}
	}
	if in.Names != nil {
		return core.CheckNames(*in.Names)
	}
	return nil
}

func CreateEntity(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpEntityEdit)
	if err != nil {
		return err
	}
	var in entityIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Timezone == nil && m.DefaultZone != "" { // written into the record: a later change of the key changes nothing
		in.Timezone = &m.DefaultZone
	}
	if in.Timezone == nil {
		return core.Invalid("timezone", "a legal entity needs a time zone: give one, or set DEFAULT_LEGAL_ENTITY_TIMEZONE (protocol 14)")
	}
	if err := in.validate(true); err != nil {
		return err
	}
	names := map[string]string{}
	if in.Names != nil {
		names = *in.Names
	}
	month := 1
	if in.FiscalYearStartMonth != nil {
		month = *in.FiscalYearStartMonth
	}
	want := core.LegalEntity{Code: in.Code, Name: *in.Name, Names: names, Country: in.Country, Timezone: *in.Timezone,
		BaseCurrency: *in.BaseCurrency, FiscalYearStartMonth: month, State: "active"}
	var out core.LegalEntity
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var existing string
		err := tx.QueryRow(ctx, `SELECT id::text FROM legal_entity WHERE code = $1 FOR UPDATE`, in.Code).Scan(&existing)
		if err == nil { // the same creation again returns it; other content under the code is refused
			cur, err := core.GetEntity(ctx, tx, existing, false)
			if err != nil {
				return err
			}
			if cur.Name == want.Name && cur.Country == want.Country && cur.Timezone == want.Timezone &&
				cur.BaseCurrency == want.BaseCurrency && cur.FiscalYearStartMonth == want.FiscalYearStartMonth {
				out = cur
				return nil
			}
			return core.ErrCode.New("legal entity code " + in.Code + " is taken")
		}
		id := ids.New().String()
		nb, _ := json.Marshal(names)
		if _, err := tx.Exec(ctx, `INSERT INTO legal_entity (id, code, name, names, country, timezone, base_currency,
			fiscal_year_start_month, search, search_version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (code) DO NOTHING`, id, want.Code, want.Name, nb, want.Country, want.Timezone, want.BaseCurrency,
			want.FiscalYearStartMonth, core.SearchOf(want.Code, want.Name, names), search.Version); err != nil {
			return err
		}
		if out, err = core.GetEntity(ctx, tx, id, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.LegalEntityChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpEntityEdit, core.LegalEntities, out.ID, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func GetEntity(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpEntityRead)
	if err != nil {
		return err
	}
	e, err := entity(m, ctx, c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(200, e)
}

func entity(m *core.Base, ctx context.Context, id string) (core.LegalEntity, error) {
	if !core.UUIDForm.MatchString(id) {
		return core.LegalEntity{}, core.NotFound("legal entity")
	}
	var e core.LegalEntity
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		e, err = core.GetEntity(ctx, tx, id, false)
		return err
	})
	if errors.Is(err, core.ErrNone) {
		return e, core.NotFound("legal entity")
	}
	return e, err
}

// UpdateEntity changes names, time zone, base currency or the fiscal year's start month; the same values again
// change nothing.
func UpdateEntity(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpEntityEdit)
	if err != nil {
		return err
	}
	var in entityIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Code != "" || in.Country != "" {
		return core.Invalid("code", "code and country do not change: a legal entity in another country is another legal entity")
	}
	if err := in.validate(false); err != nil {
		return err
	}
	return updateEntityWith(m, c, ctx, actor, in.Version, nil, func(e *core.LegalEntity) error {
		if in.Name != nil {
			e.Name = *in.Name
		}
		if in.Names != nil {
			e.Names = *in.Names
		}
		if in.Timezone != nil {
			e.Timezone = *in.Timezone
		}
		if in.BaseCurrency != nil {
			e.BaseCurrency = *in.BaseCurrency
		}
		if in.FiscalYearStartMonth != nil {
			e.FiscalYearStartMonth = *in.FiscalYearStartMonth
		}
		return nil
	})
}

// EntityState moves a legal entity through protocol 07's lifecycle: active and suspended both ways, either to
// retired, never back from retired.
func EntityState(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpEntityEdit)
	if err != nil {
		return err
	}
	var in struct {
		State   string `json:"state"`
		Version *int64 `json:"version"`
	}
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.State != "active" && in.State != "suspended" && in.State != "retired" {
		return core.Invalid("state", "state is active, suspended or retired")
	}
	return updateEntityWith(m, c, ctx, actor, in.Version, tightening.EntityStatus(in.State), func(e *core.LegalEntity) error {
		if e.State == "retired" && in.State != "retired" {
			return core.ErrState.New("a retired legal entity stays retired")
		}
		e.State = in.State
		return nil
	})
}

// updateEntityWith locks the legal entity, applies the change, and writes, publishes and audits it if it changed; a
// change that may tighten (tight) is recorded as changeUnit does.
func updateEntityWith(m *core.Base, c *httpx.Context, ctx context.Context, actor string, version *int64,
	tight *tightening.Tightening[core.LegalEntity], apply func(*core.LegalEntity) error) error {
	id := c.Param("id")
	if !core.UUIDForm.MatchString(id) {
		return core.NotFound("legal entity")
	}
	var p *evidence.Pending
	if tight != nil && m.Ev != nil {
		var pre core.LegalEntity
		if m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
			var err error
			pre, err = core.GetEntity(ctx, tx, id, false)
			return err
		}) == nil {
			p = tightening.Intent(ctx, m.Ev, tight, id, pre.Version, pre)
		}
	}
	concluded := false
	defer func() { tightening.Settle(ctx, p, concluded) }()
	var out core.LegalEntity
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		concluded = false
		cur, err := core.GetEntity(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if version != nil && *version != cur.Version {
			return core.ErrVersion.New(fmt.Sprintf("the legal entity is at version %d", cur.Version))
		}
		next := cur
		next.Names = map[string]string{}
		for k, v := range cur.Names {
			next.Names[k] = v
		}
		if err := apply(&next); err != nil {
			return err
		}
		if core.Same(cur, next) {
			out = cur
			return nil
		}
		if err := WriteEntity(ctx, tx, next); err != nil {
			return err
		}
		if out, err = core.GetEntity(ctx, tx, id, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.LegalEntityChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		if concluded, err = tightening.Conclude(ctx, tx, p, tight, cur, next); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpEntityEdit, core.LegalEntities, out.ID, out)
	})
	if err != nil {
		concluded = false // the transaction did not commit
	}
	if errors.Is(err, core.ErrNone) {
		return core.NotFound("legal entity")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

// BusinessDate is the legal entity's business date at an instant (protocol 14: in the legal entity's time zone).
func BusinessDate(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpEntityRead)
	if err != nil {
		return err
	}
	at := time.Now()
	if v := c.Query("at"); v != "" {
		if at, err = time.Parse(time.RFC3339Nano, v); err != nil {
			return core.Invalid("at", "at is an RFC 3339 instant")
		}
	}
	e, err := entity(m, ctx, c.Param("id"))
	if err != nil {
		return err
	}
	d, err := bizdate.Of(at, e.Timezone)
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]string{"date": d, "timezone": e.Timezone})
}

// WriteEntity writes a legal entity's changed fields with a new version.
func WriteEntity(ctx context.Context, tx db.Tx, e core.LegalEntity) error {
	nb, _ := json.Marshal(e.Names)
	_, err := tx.Exec(ctx, `UPDATE legal_entity SET name = $2, names = $3, timezone = $4, base_currency = $5,
		fiscal_year_start_month = $6, state = $7, search = $8, search_version = $9, version = version + 1,
		updated_at = now() WHERE id = $1`, e.ID, e.Name, nb, e.Timezone, e.BaseCurrency,
		e.FiscalYearStartMonth, e.State, core.SearchOf(e.Code, e.Name, e.Names), search.Version)
	return err
}

// The lists of the user surface (protocol 14: cursor pages, a stable order ending in the record's ID, the filters and
// keyword declared here). Every field is ordinary, so a row is listed whole or not at all.
var entityList = list.Spec{Operation: core.OpEntityRead, Alias: "e", IDColumn: "id", IDKind: list.UUID,
	Columns: []list.Column{
		{Field: "code", Column: "code", Kind: list.Text, Sortable: true},
		{Field: "state", Column: "state", Kind: list.Text, Filterable: true}},
	Search: []list.SearchColumn{{Name: "names", Column: "search", Fields: []string{"code", "name", "names"}}}}

func ListEntities(m *core.Base, c *httpx.Context) error {
	ctx, q, err := core.Build(c, entityList, list.Request{Filters: core.Eq(c, "state"), Q: c.Query("q"),
		Sort: []list.Order{{Field: "code"}}})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.RT.DB, q, "legal_entity", core.EntityCols, core.ScanEntity,
		func(e core.LegalEntity) ([]string, string) { return []string{e.Code}, e.ID })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"legal_entities": out, "next_cursor": next})
}
