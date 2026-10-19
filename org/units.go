package org

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/ids"
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

// structure locks the legal entity's row: every change of its tree (a new unit, a move, a retirement) runs one at a
// time, so two concurrent moves cannot build a cycle that neither saw (each checks the tree as the other left it).
func structure(ctx context.Context, tx db.Tx, legalEntity string) (LegalEntity, error) {
	return getEntity(ctx, tx, legalEntity, true)
}

func (m *Module) createUnit(c *httpx.Context) error {
	ctx, actor, err := check(c, OpUnitEdit)
	if err != nil {
		return err
	}
	var in unitIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	switch {
	case !uuidForm.MatchString(in.LegalEntityID):
		return invalid("legal_entity_id", "legal_entity_id is a legal entity's ID")
	case in.ParentID != nil && !uuidForm.MatchString(*in.ParentID):
		return invalid("parent_id", "parent_id is a unit's ID, or null for the top")
	case !codeForm.MatchString(in.Code):
		return invalid("code", "code is upper case letters, digits, _ and -, at most 32")
	case in.Name == nil || strings.TrimSpace(*in.Name) == "":
		return invalid("name", "name is required")
	}
	names := map[string]string{}
	if in.Names != nil {
		if err := checkNames(*in.Names); err != nil {
			return err
		}
		names = *in.Names
	}
	var out Unit
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		le, err := structure(ctx, tx, in.LegalEntityID)
		if errors.Is(err, errNone) {
			return invalid("legal_entity_id", "no such legal entity")
		}
		if err != nil {
			return err
		}
		if le.State != "active" {
			return errState.New("the legal entity is " + le.State + ": no new units")
		}
		var existing string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM org_unit WHERE legal_entity_id = $1 AND code = $2`,
			in.LegalEntityID, in.Code).Scan(&existing); err == nil {
			cur, err := getUnit(ctx, tx, existing, false)
			if err != nil {
				return err
			}
			if cur.Name == *in.Name && sameParent(cur.ParentID, in.ParentID) {
				out = cur // the same creation again returns it
				return nil
			}
			return errCode.New("unit code " + in.Code + " is taken in this legal entity")
		}
		id := ids.New().String()
		path := le.ID + "/" + id
		if in.ParentID != nil {
			parent, err := getUnit(ctx, tx, *in.ParentID, false)
			if errors.Is(err, errNone) || err == nil && parent.LegalEntityID != le.ID {
				return invalid("parent_id", "the parent is not a unit of this legal entity")
			}
			if err != nil {
				return err
			}
			if parent.State != "active" {
				return errState.New("the parent unit is retired")
			}
			path = parent.Path + "/" + id
		}
		nb, _ := json.Marshal(names)
		if _, err := tx.Exec(ctx, `INSERT INTO org_unit (id, legal_entity_id, parent_id, code, name, names, path, search,
			search_version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, id, le.ID, in.ParentID, in.Code, *in.Name, nb, path,
			searchOf(in.Code, *in.Name, names), search.Version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_unit_path_history (unit_id, path, valid_from) VALUES ($1, $2, now())`,
			id, path); err != nil {
			return err
		}
		if out, err = getUnit(ctx, tx, id, false); err != nil {
			return err
		}
		if err := m.emit(ctx, tx, UnitChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		return m.change(ctx, tx, actor, OpUnitEdit, Units, out.ID, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func sameParent(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func (m *Module) unit(ctx context.Context, id string) (Unit, error) {
	if !uuidForm.MatchString(id) {
		return Unit{}, notFound("unit")
	}
	var u Unit
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		u, err = getUnit(ctx, tx, id, false)
		return err
	})
	if errors.Is(err, errNone) {
		return u, notFound("unit")
	}
	return u, err
}

func (m *Module) getUnit(c *httpx.Context) error {
	ctx, _, err := check(c, OpUnitRead)
	if err != nil {
		return err
	}
	u, err := m.unit(ctx, c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(200, u)
}

// changeUnit locks the unit (and with structural, first its legal entity), applies the change, and writes, publishes
// and audits the unit if it changed.
func (m *Module) changeUnit(c *httpx.Context, op string, structural bool, version *int64,
	apply func(ctx context.Context, tx db.Tx, u *Unit) error) error {
	ctx, actor, err := check(c, op)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if !uuidForm.MatchString(id) {
		return notFound("unit")
	}
	var out Unit
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		if structural {
			var le string
			if err := tx.QueryRow(ctx, `SELECT legal_entity_id::text FROM org_unit WHERE id = $1`, id).Scan(&le); err != nil {
				return errNone
			}
			if _, err := structure(ctx, tx, le); err != nil {
				return err
			}
		}
		cur, err := getUnit(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if version != nil && *version != cur.Version {
			return errVersion.New(fmt.Sprintf("the unit is at version %d", cur.Version))
		}
		next := cur
		if err := apply(ctx, tx, &next); err != nil {
			return err
		}
		if same(cur, next) {
			out = cur
			return nil
		}
		if err := m.writeUnit(ctx, tx, next); err != nil {
			return err
		}
		if out, err = getUnit(ctx, tx, id, false); err != nil {
			return err
		}
		if err := m.emit(ctx, tx, UnitChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		return m.change(ctx, tx, actor, op, Units, out.ID, out)
	})
	if errors.Is(err, errNone) {
		return notFound("unit")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func (m *Module) writeUnit(ctx context.Context, tx db.Tx, u Unit) error {
	nb, _ := json.Marshal(u.Names)
	_, err := tx.Exec(ctx, `UPDATE org_unit SET parent_id = $2, name = $3, names = $4, path = $5, state = $6, search = $7,
		search_version = $8, version = version + 1, updated_at = now() WHERE id = $1`, u.ID, u.ParentID, u.Name, nb, u.Path,
		u.State, searchOf(u.Code, u.Name, u.Names), search.Version)
	return err
}

func (m *Module) renameUnit(c *httpx.Context) error {
	var in unitIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return invalid("name", "name cannot be empty")
	}
	if in.Names != nil {
		if err := checkNames(*in.Names); err != nil {
			return err
		}
	}
	return m.changeUnit(c, OpUnitEdit, false, in.Version, func(_ context.Context, _ db.Tx, u *Unit) error {
		if in.Name != nil {
			u.Name = *in.Name
		}
		if in.Names != nil {
			u.Names = *in.Names
		}
		return nil
	})
}

// moveUnit puts the unit under another parent of the same legal entity (or at the top). Its path and the path of every
// unit below it change in this transaction, each with a new version, an event and a history row; a parent inside the
// unit's own subtree is refused (the tree has no cycles).
func (m *Module) moveUnit(c *httpx.Context) error {
	var in struct {
		ParentID *string `json:"parent_id"`
		Version  *int64  `json:"version"`
	}
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.ParentID != nil && !uuidForm.MatchString(*in.ParentID) {
		return invalid("parent_id", "parent_id is a unit's ID, or null for the top")
	}
	return m.changeUnit(c, OpUnitEdit, true, in.Version, func(ctx context.Context, tx db.Tx, u *Unit) error {
		if u.State != "active" {
			return errState.New("a retired unit does not move")
		}
		path := u.LegalEntityID + "/" + u.ID
		if in.ParentID != nil {
			if *in.ParentID == u.ID {
				return errCycle.New("a unit cannot be its own parent")
			}
			parent, err := getUnit(ctx, tx, *in.ParentID, false)
			if errors.Is(err, errNone) || err == nil && parent.LegalEntityID != u.LegalEntityID {
				return invalid("parent_id", "the parent is not a unit of this legal entity")
			}
			if err != nil {
				return err
			}
			if strings.HasPrefix(parent.Path+"/", u.Path+"/") {
				return errCycle.New("the new parent is inside the unit's own subtree")
			}
			if parent.State != "active" {
				return errState.New("the parent unit is retired")
			}
			path = parent.Path + "/" + u.ID
		}
		if path == u.Path {
			return nil // already there
		}
		u.ParentID = in.ParentID
		return m.repath(ctx, tx, u, path)
	})
}

// repath gives u its new path (written by the caller) and rewrites every unit below it in this transaction: a new
// version, an event and a history row each.
func (m *Module) repath(ctx context.Context, tx db.Tx, u *Unit, path string) error {
	old := u.Path
	now := time.Now()
	rows, err := tx.Query(ctx, `SELECT `+unitCols+` FROM org_unit WHERE path LIKE $1 ESCAPE '\' ORDER BY path FOR UPDATE`,
		strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(old)+"/%")
	if err != nil {
		return err
	}
	var below []Unit
	for rows.Next() {
		d, err := scanUnit(rows)
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
		if err := m.writeUnit(ctx, tx, d); err != nil {
			return err
		}
		if err := history(d.ID, d.Path); err != nil {
			return err
		}
		after, err := getUnit(ctx, tx, d.ID, false)
		if err != nil {
			return err
		}
		if err := m.emit(ctx, tx, UnitChanged, after.ID, after.Version, after); err != nil {
			return err
		}
	}
	return nil
}

// retireUnit retires a unit that has no active unit below it.
func (m *Module) retireUnit(c *httpx.Context) error {
	var in struct {
		Version *int64 `json:"version"`
	}
	if c.Request().ContentLength > 0 { // the body is optional
		if err := c.BindJSON(&in); err != nil {
			return err
		}
	}
	return m.changeUnit(c, OpUnitEdit, true, in.Version, func(ctx context.Context, tx db.Tx, u *Unit) error {
		if u.State == "retired" {
			return nil
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_unit WHERE parent_id = $1 AND state = 'active')`,
			u.ID).Scan(&active); err != nil {
			return err
		}
		if active {
			return errState.New("units below it are still active: retire or move them first")
		}
		u.State = "retired"
		return nil
	})
}

// ancestors lists the units above one, from the top down (the boundary card's path and parent query).
func (m *Module) ancestors(c *httpx.Context) error {
	ctx, _, err := check(c, OpUnitRead)
	if err != nil {
		return err
	}
	u, err := m.unit(ctx, c.Param("id"))
	if err != nil {
		return err
	}
	segs := strings.Split(u.Path, "/")
	above := segs[1 : len(segs)-1] // the legal entity first, the unit itself last
	out := []Unit{}
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+unitCols+` FROM org_unit WHERE id = ANY($1::uuid[]) ORDER BY length(path)`, above)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanUnit(rows)
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

func (m *Module) pathHistory(c *httpx.Context) error {
	ctx, _, err := check(c, OpUnitRead)
	if err != nil {
		return err
	}
	u, err := m.unit(ctx, c.Param("id"))
	if err != nil {
		return err
	}
	type entry struct {
		Path      string  `json:"path"`
		ValidFrom string  `json:"valid_from"`
		ValidTo   *string `json:"valid_to"`
	}
	out := []entry{}
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
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
