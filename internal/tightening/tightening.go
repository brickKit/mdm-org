package tightening

import (
	"context"
	"slices"
	"strconv"
	"strings"

	core "github.com/brickKit/mdm-org/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/evidence"
)

// The evidence this member records (assembly protocol 15's evidence table, "a tightening adjustment of the
// organization"; be-protocol P12.15-P12.18): retiring a unit, moving a unit out from under a unit above it, suspending
// or retiring a legal entity, in two phases (the intent before the change, the conclusion in its transaction, cancelled
// when it did not take place). New units, renames, a move that only adds units above and a legal entity made active
// again narrow nothing and are not recorded.

// The object types of this member's evidence.
const (
	UnitType        = "mdm.org.unit"
	LegalEntityType = "mdm.org.legal_entity"
)

// Category is the evidence category of every record here (be-protocol's evidence-record vector); the change's "kind"
// tells retirement, move_out and legal_entity_status apart.
const Category = "org_tightening"

// Tightening describes a change that may narrow what the tree grants, for a record of type T (Unit, LegalEntity).
type Tightening[T any] struct {
	typ string
	// due reports, from the record read without a lock before the change, whether the change may tighten: the intent
	// is written then. It must hold whenever the change under the lock can tighten (the intent comes first).
	due func(before T) bool
	// change is the change itself (what it does, not the resulting state), from the record read before.
	change func(before T) map[string]any
	// tightens reports whether the change made under the lock narrowed anything: only then is it concluded.
	tightens func(before, after T) bool
}

// Intent writes a tightening's intent; nil when the member writes no evidence or the change cannot tighten.
func Intent[T any](ctx context.Context, w *evidence.Writer, t *Tightening[T], id string, version int64, before T) *evidence.Pending {
	if w == nil || t == nil || !t.due(before) {
		return nil
	}
	p, err := w.Intent(ctx, Category, "", evidence.Object{Type: t.typ, ID: id, Version: strconv.FormatInt(version, 10)},
		t.change(before))
	if err != nil { // the record could not be encoded: a defect here, never a reason to hold the change back
		if w.Log != nil {
			w.Log.Error("an intent could not be sealed", "event", "evidence_intent_unsealed", "object", id, "error", err)
		}
		return nil
	}
	return p
}

// Conclude writes the effective conclusion in the change's transaction when the change tightened.
func Conclude[T any](ctx context.Context, tx db.Tx, p *evidence.Pending, t *Tightening[T], before, after T) (bool, error) {
	if p == nil || !t.tightens(before, after) {
		return false, nil
	}
	return true, p.Effective(ctx, tx)
}

// Settle writes cancelled when the change whose intent p is did not tighten (refused, no change, or a widening).
func Settle(ctx context.Context, p *evidence.Pending, concluded bool) {
	if p != nil && !concluded {
		_ = p.Cancelled(context.WithoutCancel(ctx))
	}
}

// retirement: an active unit retired.
var Retirement = &Tightening[core.Unit]{typ: UnitType,
	due:      func(u core.Unit) bool { return u.State == "active" },
	change:   func(core.Unit) map[string]any { return map[string]any{"kind": "retirement"} },
	tightens: func(b, a core.Unit) bool { return b.State == "active" && a.State == "retired" },
}

// MoveOut: a unit no longer below one of the units it was below. Every move of an active unit writes its intent:
// the units above it after the move are known only under the tree's lock.
func MoveOut(parent *string) *Tightening[core.Unit] {
	return &Tightening[core.Unit]{typ: UnitType,
		due: func(u core.Unit) bool { return u.State == "active" },
		change: func(u core.Unit) map[string]any {
			var to any // the top: null
			if parent != nil {
				to = *parent
			}
			return map[string]any{"kind": "move_out", "parent_after": to, "path_before": u.Path}
		},
		tightens: func(b, a core.Unit) bool {
			after := above(a.Path)
			return slices.ContainsFunc(above(b.Path), func(id string) bool { return !slices.Contains(after, id) })
		},
	}
}

// above lists the units above one by its path (the legal entity first, the unit itself last; both left out).
func above(path string) []string {
	segs := strings.Split(path, "/")
	if len(segs) < 3 {
		return nil
	}
	return segs[1 : len(segs)-1]
}

var entityRank = map[string]int{"active": 0, "suspended": 1, "retired": 2}

// EntityStatus: a legal entity suspended or retired (a retired one stays retired).
func EntityStatus(state string) *Tightening[core.LegalEntity] {
	return &Tightening[core.LegalEntity]{typ: LegalEntityType,
		due: func(e core.LegalEntity) bool { return state != "active" && e.State != "retired" },
		change: func(core.LegalEntity) map[string]any {
			return map[string]any{"kind": "legal_entity_status", "state": state}
		},
		tightens: func(b, a core.LegalEntity) bool { return entityRank[a.State] > entityRank[b.State] },
	}
}
