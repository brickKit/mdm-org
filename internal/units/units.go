package units

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/brickKit/mdm-org/v3/internal/core"
	positions "github.com/brickKit/mdm-org/v3/internal/positions"
	tightening "github.com/brickKit/mdm-org/v3/internal/tightening"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/evidence"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/list"
	"github.com/brickKit/be-sdk-go/v3/search"
)

type unitIn struct {
	Version       *int64             `json:"version"`
	LegalEntityID string             `json:"legal_entity_id"`
	ParentID      *string            `json:"parent_id"`
	Code          string             `json:"code"`
	Name          *string            `json:"name"`
	Names         *map[string]string `json:"names"`
}

// Structure locks the legal entity's row: every change of its tree (a new unit, a move, a retirement) runs one at a
// time, so two concurrent moves cannot build a cycle that neither saw (each checks the tree as the other left it).
func Structure(ctx context.Context, tx db.Tx, legalEntity string) (core.LegalEntity, error) {
	return core.GetEntity(ctx, tx, legalEntity, true)
}

func CreateUnit(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpUnitEdit)
	if err != nil {
		return err
	}
	var in unitIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	switch {
	case !core.UUIDForm.MatchString(in.LegalEntityID):
		return core.Invalid("legal_entity_id", "legal_entity_id is a legal entity's ID")
	case in.ParentID != nil && !core.UUIDForm.MatchString(*in.ParentID):
		return core.Invalid("parent_id", "parent_id is a unit's ID, or null for the top")
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
	var out core.Unit
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		le, err := Structure(ctx, tx, in.LegalEntityID)
		if errors.Is(err, core.ErrNone) {
			return core.Invalid("legal_entity_id", "no such legal entity")
		}
		if err != nil {
			return err
		}
		if le.State != "active" {
			return core.ErrState.New("the legal entity is " + le.State + ": no new units")
		}
		var existing string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM org_unit WHERE legal_entity_id = $1 AND code = $2`,
			in.LegalEntityID, in.Code).Scan(&existing); err == nil {
			cur, err := core.GetUnit(ctx, tx, existing, false)
			if err != nil {
				return err
			}
			if cur.Name == *in.Name && sameParent(cur.ParentID, in.ParentID) {
				out = cur // the same creation again returns it
				return nil
			}
			return core.ErrCode.New("unit code " + in.Code + " is taken in this legal entity")
		}
		id := ids.New().String()
		path := le.ID + "/" + id
		if in.ParentID != nil {
			parent, err := core.GetUnit(ctx, tx, *in.ParentID, false)
			if errors.Is(err, core.ErrNone) || err == nil && parent.LegalEntityID != le.ID {
				return core.Invalid("parent_id", "the parent is not a unit of this legal entity")
			}
			if err != nil {
				return err
			}
			if parent.State != "active" {
				return core.ErrState.New("the parent unit is retired")
			}
			path = parent.Path + "/" + id
		}
		nb, _ := json.Marshal(names)
		if _, err := tx.Exec(ctx, `INSERT INTO org_unit (id, legal_entity_id, parent_id, code, name, names, path, search,
			search_version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, id, le.ID, in.ParentID, in.Code, *in.Name, nb, path,
			core.SearchOf(in.Code, *in.Name, names), search.Version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_unit_path_history (unit_id, path, valid_from) VALUES ($1, $2, now())`,
			id, path); err != nil {
			return err
		}
		if out, err = core.GetUnit(ctx, tx, id, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.UnitChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpUnitEdit, core.Units, out.ID, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func sameParent(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func unit(m *core.Base, ctx context.Context, id string) (core.Unit, error) {
	if !core.UUIDForm.MatchString(id) {
		return core.Unit{}, core.NotFound("unit")
	}
	var u core.Unit
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		u, err = core.GetUnit(ctx, tx, id, false)
		return err
	})
	if errors.Is(err, core.ErrNone) {
		return u, core.NotFound("unit")
	}
	return u, err
}

func GetUnit(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpUnitRead)
	if err != nil {
		return err
	}
	u, err := unit(m, ctx, c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(200, u)
}

// changeUnit locks the unit (and with structural, first its legal entity), applies the change, and writes, publishes
// and audits the unit if it changed. A change that may tighten (tight) writes its intent before, from the unit read
// without a lock, and its conclusion in the transaction when it did tighten; otherwise the intent is cancelled.
func changeUnit(m *core.Base, c *httpx.Context, op string, structural bool, version *int64, tight *tightening.Tightening[core.Unit],
	apply func(ctx context.Context, tx db.Tx, u *core.Unit) error) error {
	ctx, actor, err := core.Check(c, op)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if !core.UUIDForm.MatchString(id) {
		return core.NotFound("unit")
	}
	var p *evidence.Pending
	if tight != nil && m.Ev != nil {
		if pre, err := unit(m, ctx, id); err == nil {
			p = tightening.Intent(ctx, m.Ev, tight, id, pre.Version, pre)
		}
	}
	concluded := false
	defer func() { tightening.Settle(ctx, p, concluded) }()
	var out core.Unit
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		concluded = false
		if structural {
			var le string
			if err := tx.QueryRow(ctx, `SELECT legal_entity_id::text FROM org_unit WHERE id = $1`, id).Scan(&le); err != nil {
				return core.ErrNone
			}
			if _, err := Structure(ctx, tx, le); err != nil {
				return err
			}
		}
		cur, err := core.GetUnit(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if version != nil && *version != cur.Version {
			return core.ErrVersion.New(fmt.Sprintf("the unit is at version %d", cur.Version))
		}
		next := cur
		if err := apply(ctx, tx, &next); err != nil {
			return err
		}
		if core.Same(cur, next) {
			out = cur
			return nil
		}
		if err := WriteUnit(m, ctx, tx, next); err != nil {
			return err
		}
		if out, err = core.GetUnit(ctx, tx, id, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.UnitChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		if concluded, err = tightening.Conclude(ctx, tx, p, tight, cur, next); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, op, core.Units, out.ID, out)
	})
	if err != nil {
		concluded = false // the transaction did not commit
	}
	if errors.Is(err, core.ErrNone) {
		return core.NotFound("unit")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func WriteUnit(m *core.Base, ctx context.Context, tx db.Tx, u core.Unit) error {
	nb, _ := json.Marshal(u.Names)
	_, err := tx.Exec(ctx, `UPDATE org_unit SET parent_id = $2, name = $3, names = $4, path = $5, state = $6, search = $7,
		search_version = $8, version = version + 1, updated_at = now() WHERE id = $1`, u.ID, u.ParentID, u.Name, nb, u.Path,
		u.State, core.SearchOf(u.Code, u.Name, u.Names), search.Version)
	return err
}

func RenameUnit(m *core.Base, c *httpx.Context) error {
	var in unitIn
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
	return changeUnit(m, c, core.OpUnitEdit, false, in.Version, nil, func(_ context.Context, _ db.Tx, u *core.Unit) error {
		if in.Name != nil {
			u.Name = *in.Name
		}
		if in.Names != nil {
			u.Names = *in.Names
		}
		return nil
	})
}

// MoveUnit puts the unit under another parent of the same legal entity (or at the top). Its path and the path of every
// unit below it change in this transaction, each with a new version, an event and a history row; a parent inside the
// unit's own subtree is refused (the tree has no cycles).
func MoveUnit(m *core.Base, c *httpx.Context) error {
	var in struct {
		ParentID *string `json:"parent_id"`
		Version  *int64  `json:"version"`
	}
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.ParentID != nil && !core.UUIDForm.MatchString(*in.ParentID) {
		return core.Invalid("parent_id", "parent_id is a unit's ID, or null for the top")
	}
	return changeUnit(m, c, core.OpUnitEdit, true, in.Version, tightening.MoveOut(in.ParentID), func(ctx context.Context, tx db.Tx, u *core.Unit) error {
		if u.State != "active" {
			return core.ErrState.New("a retired unit does not move")
		}
		path := u.LegalEntityID + "/" + u.ID
		if in.ParentID != nil {
			if *in.ParentID == u.ID {
				return core.ErrCycle.New("a unit cannot be its own parent")
			}
			parent, err := core.GetUnit(ctx, tx, *in.ParentID, false)
			if errors.Is(err, core.ErrNone) || err == nil && parent.LegalEntityID != u.LegalEntityID {
				return core.Invalid("parent_id", "the parent is not a unit of this legal entity")
			}
			if err != nil {
				return err
			}
			if strings.HasPrefix(parent.Path+"/", u.Path+"/") {
				return core.ErrCycle.New("the new parent is inside the unit's own subtree")
			}
			if parent.State != "active" {
				return core.ErrState.New("the parent unit is retired")
			}
			path = parent.Path + "/" + u.ID
		}
		if path == u.Path {
			return nil // already there
		}
		u.ParentID = in.ParentID
		return Repath(m, ctx, tx, u, path)
	})
}

// Repath gives u its new path (written by the caller) and rewrites every unit below it in this transaction: a new
// version, an event and a history row each.
func Repath(m *core.Base, ctx context.Context, tx db.Tx, u *core.Unit, path string) error {
	old := u.Path
	now := time.Now()
	rows, err := tx.Query(ctx, `SELECT `+core.UnitCols+` FROM org_unit WHERE path LIKE $1 ESCAPE '\' ORDER BY path FOR UPDATE`,
		strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(old)+"/%")
	if err != nil {
		return err
	}
	var below []core.Unit
	for rows.Next() {
		d, err := core.ScanUnit(rows)
		if err != nil {
			rows.Close()
			return err
		}
		below = append(below, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	history := func(id, to string) error {
		if _, err := tx.Exec(ctx, `UPDATE org_unit_path_history SET valid_to = $2 WHERE unit_id = $1 AND valid_to IS NULL`,
			id, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO org_unit_path_history (unit_id, path, valid_from) VALUES ($1, $2, $3)`, id, to, now)
		return err
	}
	if err := history(u.ID, path); err != nil {
		return err
	}
	u.Path = path
	for _, d := range below {
		d.Path = path + strings.TrimPrefix(d.Path, old)
		if err := WriteUnit(m, ctx, tx, d); err != nil {
			return err
		}
		if err := history(d.ID, d.Path); err != nil {
			return err
		}
		after, err := core.GetUnit(ctx, tx, d.ID, false)
		if err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.UnitChanged, after.ID, after.Version, after); err != nil {
			return err
		}
	}
	return nil
}

// RetireUnit retires a unit that has no active unit below it and no active position in it.
func RetireUnit(m *core.Base, c *httpx.Context) error {
	var in struct {
		Version *int64 `json:"version"`
	}
	if c.Request().ContentLength > 0 { // the body is optional
		if err := c.BindJSON(&in); err != nil {
			return err
		}
	}
	return changeUnit(m, c, core.OpUnitEdit, true, in.Version, tightening.Retirement, func(ctx context.Context, tx db.Tx, u *core.Unit) error {
		if u.State == "retired" {
			return nil
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_unit WHERE parent_id = $1 AND state = 'active')`,
			u.ID).Scan(&active); err != nil {
			return err
		}
		if active {
			return core.ErrState.New("units below it are still active: retire or move them first")
		}
		active, err := positions.ActiveIn(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		if active {
			return core.ErrState.New("positions in it are still active: retire them first")
		}
		u.State = "retired"
		return nil
	})
}

// Ancestors lists the units above one, from the top down (the boundary card's path and parent query).
func Ancestors(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpUnitRead)
	if err != nil {
		return err
	}
	u, err := unit(m, ctx, c.Param("id"))
	if err != nil {
		return err
	}
	segs := strings.Split(u.Path, "/")
	above := segs[1 : len(segs)-1] // the legal entity first, the unit itself last
	out := []core.Unit{}
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+core.UnitCols+` FROM org_unit WHERE id = ANY($1::uuid[]) ORDER BY length(path)`, above)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := core.ScanUnit(rows)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"ancestors": out})
}

func PathHistory(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpUnitRead)
	if err != nil {
		return err
	}
	u, err := unit(m, ctx, c.Param("id"))
	if err != nil {
		return err
	}
	type entry struct {
		Path      string  `json:"path"`
		ValidFrom string  `json:"valid_from"`
		ValidTo   *string `json:"valid_to"`
	}
	out := []entry{}
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT path, valid_from, valid_to FROM org_unit_path_history WHERE unit_id = $1
			ORDER BY valid_from`, u.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e entry
			var from time.Time
			var to *time.Time
			if err := rows.Scan(&e.Path, &from, &to); err != nil {
				return err
			}
			e.ValidFrom = from.UTC().Format(time.RFC3339Nano)
			if to != nil {
				s := to.UTC().Format(time.RFC3339Nano)
				e.ValidTo = &s
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"history": out})
}

var unitList = list.Spec{Operation: core.OpUnitRead, Alias: "u", IDColumn: "id", IDKind: list.UUID,
	Columns: []list.Column{
		{Field: "path", Column: "path", Kind: list.Text, Sortable: true},
		{Field: "legal_entity_id", Column: "legal_entity_id", Kind: list.UUID, Filterable: true},
		{Field: "parent_id", Column: "parent_id", Kind: list.UUID, Filterable: true},
		{Field: "state", Column: "state", Kind: list.Text, Filterable: true}},
	Search: []list.SearchColumn{{Name: "names", Column: "search", Fields: []string{"code", "name", "names"}}}}

func ListUnits(m *core.Base, c *httpx.Context) error {
	for _, f := range []string{"legal_entity_id", "parent_id"} {
		if v := c.Query(f); v != "" && !core.UUIDForm.MatchString(v) {
			return core.Invalid(f, f+" is an ID")
		}
	}
	ctx, q, err := core.Build(c, unitList, list.Request{Filters: core.Eq(c, "legal_entity_id", "parent_id", "state"),
		Q: c.Query("q"), Sort: []list.Order{{Field: "path"}}})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.RT.DB, q, "org_unit", core.UnitCols, core.ScanUnit,
		func(u core.Unit) ([]string, string) { return []string{u.Path}, u.ID })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"units": out, "next_cursor": next})
}
