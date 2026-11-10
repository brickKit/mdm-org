// Package positions is the positions of mdm/org (3.0.34): a post within one unit, on which mdm/employee's appointments
// hang. A position belongs to an active unit of an active legal entity when it is created and never moves (a post in
// another unit is a new position); renames change the names only; retirement is final and a tightening (its holders
// lose what hangs on it), recorded as evidence like a unit's retirement.
package positions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	core "github.com/brickKit/mdm-org/v3/internal/core"
	tightening "github.com/brickKit/mdm-org/v3/internal/tightening"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/evidence"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/list"
	"github.com/brickKit/be-sdk-go/v3/search"
)

type positionIn struct {
	Version *int64             `json:"version"`
	UnitID  string             `json:"unit_id"`
	Code    string             `json:"code"`
	Name    *string            `json:"name"`
	Names   *map[string]string `json:"names"`
}

// ActiveIn reports whether a unit still has an active position (a unit with one does not retire). Called under the
// legal entity's lock, as every creation of a position takes it too.
func ActiveIn(ctx context.Context, tx db.Tx, unit string) (bool, error) {
	var active bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_position WHERE unit_id = $1 AND state = 'active')`,
		unit).Scan(&active)
	return active, err
}

// CreatePosition creates a position in an active unit; the legal entity is the unit's. The legal entity's row is
// locked first, as for every change of its tree: a unit cannot retire while a position is being created in it.
func CreatePosition(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpPosEdit)
	if err != nil {
		return err
	}
	var in positionIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	switch {
	case !core.UUIDForm.MatchString(in.UnitID):
		return core.Invalid("unit_id", "unit_id is a unit's ID")
	case !core.CodeForm.MatchString(in.Code):
		return core.Invalid("code", "code is upper case letters, digits, _ and -, at most 32")
	case in.Name == nil || strings.TrimSpace(*in.Name) == "":
		return core.Invalid("name", "name is required")
	}
	names := map[string]string{}
	if in.Names != nil {
		if err := core.CheckNames(*in.Names); err != nil {
			return err
		}
		names = *in.Names
	}
	var out core.Position
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var le string
		if err := tx.QueryRow(ctx, `SELECT legal_entity_id::text FROM org_unit WHERE id = $1`, in.UnitID).Scan(&le); err != nil {
			return core.Invalid("unit_id", "no such unit")
		}
		entity, err := core.GetEntity(ctx, tx, le, true)
		if err != nil {
			return err
		}
		if entity.State != "active" {
			return core.ErrState.New("the legal entity is " + entity.State + ": no new positions")
		}
		u, err := core.GetUnit(ctx, tx, in.UnitID, false)
		if err != nil {
			return err
		}
		var existing string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM org_position WHERE legal_entity_id = $1 AND code = $2`,
			le, in.Code).Scan(&existing); err == nil {
			cur, err := core.GetPosition(ctx, tx, existing, false)
			if err != nil {
				return err
			}
			if cur.Name == *in.Name && cur.UnitID == in.UnitID {
				out = cur // the same creation again returns it
				return nil
			}
			return core.ErrCode.New("position code " + in.Code + " is taken in this legal entity")
		}
		if u.State != "active" {
			return core.ErrState.New("the unit is retired: no new positions")
		}
		id := ids.New().String()
		nb, _ := json.Marshal(names)
		if _, err := tx.Exec(ctx, `INSERT INTO org_position (id, legal_entity_id, unit_id, code, name, names, search,
			search_version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, le, in.UnitID, in.Code, *in.Name, nb,
			core.SearchOf(in.Code, *in.Name, names), search.Version); err != nil {
			return err
		}
		if out, err = core.GetPosition(ctx, tx, id, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.PositionChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpPosEdit, core.Positions, out.ID, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func position(m *core.Base, ctx context.Context, id string) (core.Position, error) {
	if !core.UUIDForm.MatchString(id) {
		return core.Position{}, core.NotFound("position")
	}
	var p core.Position
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		p, err = core.GetPosition(ctx, tx, id, false)
		return err
	})
	if errors.Is(err, core.ErrNone) {
		return p, core.NotFound("position")
	}
	return p, err
}

func GetPosition(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpPosRead)
	if err != nil {
		return err
	}
	p, err := position(m, ctx, c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(200, p)
}

// change locks the position, applies the change, and writes, publishes and audits it if it changed. A change that may
// tighten (tight) writes its intent before, from the position read without a lock, and its conclusion in the
// transaction when it did tighten; otherwise the intent is cancelled.
func change(m *core.Base, c *httpx.Context, version *int64, tight *tightening.Tightening[core.Position],
	apply func(p *core.Position) error) error {
	ctx, actor, err := core.Check(c, core.OpPosEdit)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if !core.UUIDForm.MatchString(id) {
		return core.NotFound("position")
	}
	var p *evidence.Pending
	if tight != nil && m.Ev != nil {
		if pre, err := position(m, ctx, id); err == nil {
			p = tightening.Intent(ctx, m.Ev, tight, id, pre.Version, pre)
		}
	}
	concluded := false
	defer func() { tightening.Settle(ctx, p, concluded) }()
	var out core.Position
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		concluded = false
		cur, err := core.GetPosition(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if version != nil && *version != cur.Version {
			return core.ErrVersion.New(fmt.Sprintf("the position is at version %d", cur.Version))
		}
		next := cur
		if err := apply(&next); err != nil {
			return err
		}
		if core.Same(cur, next) {
			out = cur
			return nil
		}
		if err := WritePosition(ctx, tx, next); err != nil {
			return err
		}
		if out, err = core.GetPosition(ctx, tx, id, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.PositionChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		if concluded, err = tightening.Conclude(ctx, tx, p, tight, cur, next); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpPosEdit, core.Positions, out.ID, out)
	})
	if err != nil {
		concluded = false // the transaction did not commit
	}
	if errors.Is(err, core.ErrNone) {
		return core.NotFound("position")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

// WritePosition writes a position's changed names and state with a new version (the replay writes through it too).
func WritePosition(ctx context.Context, tx db.Tx, p core.Position) error {
	nb, _ := json.Marshal(p.Names)
	_, err := tx.Exec(ctx, `UPDATE org_position SET name = $2, names = $3, state = $4, search = $5, search_version = $6,
		version = version + 1, updated_at = now() WHERE id = $1`, p.ID, p.Name, nb, p.State,
		core.SearchOf(p.Code, p.Name, p.Names), search.Version)
	return err
}

func RenamePosition(m *core.Base, c *httpx.Context) error {
	var in positionIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return core.Invalid("name", "name cannot be empty")
	}
	if in.Names != nil {
		if err := core.CheckNames(*in.Names); err != nil {
			return err
		}
	}
	return change(m, c, in.Version, nil, func(p *core.Position) error {
		if in.Name != nil {
			p.Name = *in.Name
		}
		if in.Names != nil {
			p.Names = *in.Names
		}
		return nil
	})
}

// RetirePosition retires a position; retiring a retired one changes nothing.
func RetirePosition(m *core.Base, c *httpx.Context) error {
	var in struct {
		Version *int64 `json:"version"`
	}
	if c.Request().ContentLength > 0 { // the body is optional
		if err := c.BindJSON(&in); err != nil {
			return err
		}
	}
	return change(m, c, in.Version, tightening.PositionRetirement, func(p *core.Position) error {
		p.State = "retired"
		return nil
	})
}

var positionList = list.Spec{Operation: core.OpPosRead, Alias: "p", IDColumn: "id", IDKind: list.UUID,
	Columns: []list.Column{
		{Field: "code", Column: "code", Kind: list.Text, Sortable: true},
		{Field: "legal_entity_id", Column: "legal_entity_id", Kind: list.UUID, Filterable: true},
		{Field: "unit_id", Column: "unit_id", Kind: list.UUID, Filterable: true},
		{Field: "state", Column: "state", Kind: list.Text, Filterable: true}},
	Search: []list.SearchColumn{{Name: "names", Column: "search", Fields: []string{"code", "name", "names"}}}}

// ListPositions lists positions by legal entity, unit or state, by code.
func ListPositions(m *core.Base, c *httpx.Context) error {
	for _, f := range []string{"legal_entity_id", "unit_id"} {
		if v := c.Query(f); v != "" && !core.UUIDForm.MatchString(v) {
			return core.Invalid(f, f+" is an ID")
		}
	}
	ctx, q, err := core.Build(c, positionList, list.Request{Filters: core.Eq(c, "legal_entity_id", "unit_id", "state"),
		Q: c.Query("q"), Sort: []list.Order{{Field: "code"}}})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.RT.DB, q, "org_position", core.PositionCols, core.ScanPosition,
		func(p core.Position) ([]string, string) { return []string{p.Code}, p.ID })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"positions": out, "next_cursor": next})
}
