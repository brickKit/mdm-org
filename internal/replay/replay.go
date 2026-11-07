package replay

import (
	"context"
	"errors"
	"fmt"
	"strings"

	core "github.com/brickKit/mdm-org/v3/internal/core"
	entities "github.com/brickKit/mdm-org/v3/internal/entities"
	tightening "github.com/brickKit/mdm-org/v3/internal/tightening"
	units "github.com/brickKit/mdm-org/v3/internal/units"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/evidence"
)

// The replay of this member's evidence in a full recovery's step 6 (assembly protocol 15, 6.3; be-protocol
// P12.19-P12.21): a retirement, a move out from under a unit and a legal entity's status, applied again as recorded,
// without the business preconditions (units still active below, the requester's rights): they held when the change
// was made. Only the objects must still exist; a move whose new parent now lies below the unit would make a cycle, so
// it is not applied either (for review). Each writes its event and its audit record as the change did, the recovery
// tool as the actor.

// replayActor is the audit records' actor of a replay.
const replayActor = "ops/recover"

func Replay(m *core.Base, ctx context.Context, tx db.Tx, rec evidence.Replayed) error {
	kind, _ := rec.Change["kind"].(string)
	switch {
	case rec.Object.Type == tightening.UnitType && kind == "retirement":
		return replayUnit(m, ctx, tx, rec.Object.ID, func(u *core.Unit) error {
			u.State = "retired"
			return nil
		})
	case rec.Object.Type == tightening.UnitType && kind == "move_out":
		parent, _ := rec.Change["parent_after"].(string)
		return replayUnit(m, ctx, tx, rec.Object.ID, func(u *core.Unit) error { return replayMove(m, ctx, tx, u, parent) })
	case rec.Object.Type == tightening.LegalEntityType && kind == "legal_entity_status":
		state, _ := rec.Change["state"].(string)
		return replayEntity(m, ctx, tx, rec.Object.ID, state)
	}
	return fmt.Errorf("org_tightening of kind %q on %s is not one this version replays", kind, rec.Object.Type)
}

// replayUnit locks the unit's tree and the unit, applies the change and writes, publishes and audits it.
func replayUnit(m *core.Base, ctx context.Context, tx db.Tx, id string, apply func(u *core.Unit) error) error {
	var le string
	if err := tx.QueryRow(ctx, `SELECT legal_entity_id::text FROM org_unit WHERE id = $1`, id).Scan(&le); err != nil {
		return fmt.Errorf("unit %s: %w", id, evidence.ErrDangling)
	}
	if _, err := units.Structure(ctx, tx, le); err != nil {
		return err
	}
	cur, err := core.GetUnit(ctx, tx, id, true)
	if err != nil {
		return err
	}
	next := cur
	if err := apply(&next); err != nil {
		return err
	}
	if core.Same(cur, next) {
		return nil // already so: nothing to write
	}
	if err := units.WriteUnit(m, ctx, tx, next); err != nil {
		return err
	}
	out, err := core.GetUnit(ctx, tx, id, false)
	if err != nil {
		return err
	}
	if err := core.Emit(m, ctx, tx, core.UnitChanged, out.ID, out.Version, out); err != nil {
		return err
	}
	return core.Change(m, ctx, tx, replayActor, core.OpUnitEdit, core.Units, out.ID, out)
}

// replayMove puts the unit under the recorded parent (or at the top), its subtree with it.
func replayMove(m *core.Base, ctx context.Context, tx db.Tx, u *core.Unit, parentID string) error {
	path := u.LegalEntityID + "/" + u.ID
	var parent *string
	if parentID != "" {
		p, err := core.GetUnit(ctx, tx, parentID, false)
		if errors.Is(err, core.ErrNone) || err == nil && p.LegalEntityID != u.LegalEntityID {
			return fmt.Errorf("the parent %s: %w", parentID, evidence.ErrDangling)
		}
		if err != nil {
			return err
		}
		if strings.HasPrefix(p.Path+"/", u.Path+"/") {
			return fmt.Errorf("the parent %s lies below the unit now (a cycle): %w", parentID, evidence.ErrDangling)
		}
		path, parent = p.Path+"/"+u.ID, &parentID
	}
	if path == u.Path {
		return nil
	}
	u.ParentID = parent
	return units.Repath(m, ctx, tx, u, path)
}

// replayEntity sets a legal entity's recorded status; a retired one stays retired.
func replayEntity(m *core.Base, ctx context.Context, tx db.Tx, id, state string) error {
	cur, err := core.GetEntity(ctx, tx, id, true)
	if errors.Is(err, core.ErrNone) {
		return fmt.Errorf("legal entity %s: %w", id, evidence.ErrDangling)
	}
	if err != nil {
		return err
	}
	if cur.State == state || cur.State == "retired" {
		return nil
	}
	next := cur
	next.State = state
	if err := entities.WriteEntity(ctx, tx, next); err != nil {
		return err
	}
	out, err := core.GetEntity(ctx, tx, id, false)
	if err != nil {
		return err
	}
	if err := core.Emit(m, ctx, tx, core.LegalEntityChanged, out.ID, out.Version, out); err != nil {
		return err
	}
	return core.Change(m, ctx, tx, replayActor, core.OpEntityEdit, core.LegalEntities, out.ID, out)
}
