package source

import (
	"context"
	"encoding/json"
	"fmt"

	core "github.com/brickKit/mdm-org/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/events"
	orgv3 "github.com/brickKit/mdm-org/contract/v3/gen/go/be/mdm/org/v3"
	"github.com/jackc/pgx/v5"
)

// aggregate is how the source methods read one kind of record.
type aggregate struct {
	table, cols string
	data        func(pgx.Row) (string, int64, any, error) // the ID, the version and the payload
}

var aggregates = map[orgv3.Aggregate]aggregate{
	orgv3.Aggregate_AGGREGATE_LEGAL_ENTITY: {"legal_entity", core.EntityCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := core.ScanEntity(r)
		return v.ID, v.Version, v, err
	}},
	orgv3.Aggregate_AGGREGATE_ORG_UNIT: {"org_unit", core.UnitCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := core.ScanUnit(r)
		return v.ID, v.Version, v, err
	}},
}

func stamp(m *core.Base, ctx context.Context, tx db.Tx) (*orgv3.Stamp, error) {
	st, err := events.ReadStamp(ctx, tx, m.Instance, events.Stream{Name: core.Stream, Partitions: 4})
	if err != nil {
		return nil, err
	}
	return &orgv3.Stamp{Deployment: m.Deployment, Instance: st.Instance, Epoch: st.Epoch.String(), Positions: st.Positions}, nil
}

// Snapshot pages through one aggregate by ID, with the stamp of the same snapshot (protocol 02, S3).
func Snapshot(m *core.Base, ctx context.Context, in *orgv3.SnapshotRequest) (*orgv3.SnapshotResponse, error) {
	a, ok := aggregates[in.GetAggregate()]
	if !ok {
		return nil, core.Invalid("aggregate", "an aggregate of this component")
	}
	limit := int(in.GetLimit())
	if limit == 0 {
		limit = core.MaxBatch
	}
	if limit < 0 || limit > core.MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, fmt.Sprintf("at most %d records a page", core.MaxBatch))
	}
	after := in.GetCursor()
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	} else if !core.UUIDForm.MatchString(after) {
		return nil, errs.New(errs.ReasonCursorInvalid, "the cursor cannot be read")
	}
	out := &orgv3.SnapshotResponse{}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if out.Stamp, err = stamp(m, ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE id > $1::uuid ORDER BY id LIMIT %d`,
			a.cols, a.table, limit+1), after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if len(out.Records) == limit {
				out.NextCursor = out.Records[len(out.Records)-1].Id
				break
			}
			id, version, data, err := a.data(rows)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			out.Records = append(out.Records, &orgv3.Record{Id: id, Version: version, Data: raw})
		}
		return rows.Err()
	}, db.Snapshot())
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Read answers by ID in the requested order: exists, or purged (nothing is deleted here, so no tombstones; an ID that
// is not a UUID was never one of ours).
func Read(m *core.Base, ctx context.Context, in *orgv3.ReadRequest) (*orgv3.ReadResponse, error) {
	a, ok := aggregates[in.GetAggregate()]
	if !ok {
		return nil, core.Invalid("aggregate", "an aggregate of this component")
	}
	if len(in.GetIds()) > core.MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, fmt.Sprintf("at most %d IDs a call", core.MaxBatch))
	}
	var valid []string
	for _, id := range in.GetIds() {
		if core.UUIDForm.MatchString(id) {
			valid = append(valid, id)
		}
	}
	found := map[string]*orgv3.ReadResult{}
	out := &orgv3.ReadResponse{}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if out.Stamp, err = stamp(m, ctx, tx); err != nil {
			return err
		}
		if len(valid) == 0 {
			return nil
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE id = ANY($1::uuid[])`, a.cols, a.table), valid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			id, version, data, err := a.data(rows)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			found[id] = &orgv3.ReadResult{Id: id, State: orgv3.ReadState_READ_STATE_EXISTS, Version: version, Data: raw}
		}
		return rows.Err()
	}, db.Snapshot())
	if err != nil {
		return nil, err
	}
	for _, id := range in.GetIds() {
		r, ok := found[id]
		if !ok {
			r = &orgv3.ReadResult{Id: id, State: orgv3.ReadState_READ_STATE_PURGED}
		}
		out.Results = append(out.Results, r)
	}
	return out, nil
}

// CatchUp is the watermark source's catch-up (protocol 02, S6): not in step 6.
func CatchUp(m *core.Base, _ context.Context, _ *orgv3.CatchUpRequest) (*orgv3.CatchUpResponse, error) {
	return nil, errs.New(errs.ReasonCapabilityUnsupported, "mdm/org is not yet a watermark source (protocol 02, S6)")
}
