package org

import (
	"context"
	"fmt"
	"time"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/evidence"
	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	orgv3 "github.com/brickKit/mdm-org/contract/v3/gen/go/be/mdm/org/v3"
	core "github.com/brickKit/mdm-org/v3/internal/core"
	entities "github.com/brickKit/mdm-org/v3/internal/entities"
	positions "github.com/brickKit/mdm-org/v3/internal/positions"
	replay "github.com/brickKit/mdm-org/v3/internal/replay"
	tightening "github.com/brickKit/mdm-org/v3/internal/tightening"
	units "github.com/brickKit/mdm-org/v3/internal/units"
)

// Module is the organization component as the launcher and the shells run it: it only assembles the business modules
// under internal/ (their map is internal/AGENTS.md); the gRPC services answer through them (services.go).
type Module struct {
	orgv3.UnimplementedSourceServiceServer
	orgv3.UnimplementedCatchUpServiceServer
	b *core.Base
}

// New builds the module; a DEFAULT_LEGAL_ENTITY_TIMEZONE that is not an IANA zone stops the start.
func New(rt *runtime.Runtime) (*Module, error) {
	b := &core.Base{RT: rt, Ev: rt.Evidence, Deployment: rt.Config.String("DEPLOYMENT_ID"),
		Instance: rt.Config.String("IMPLEMENTATION_INSTANCE"), DefaultZone: rt.Config.String("DEFAULT_LEGAL_ENTITY_TIMEZONE")}
	if b.DefaultZone != "" {
		if _, err := time.LoadLocation(b.DefaultZone); err != nil {
			return nil, fmt.Errorf("DEFAULT_LEGAL_ENTITY_TIMEZONE %q is not an IANA time zone", b.DefaultZone)
		}
	}
	return &Module{b: b}, nil
}

// Routes registers the user surface (contract/openapi.yaml).
func (m *Module) Routes(r *httpx.Routes) error {
	add := func(method, path, op string, h httpx.Handler) {
		r.Add(httpx.Route{Method: method, Path: path, Operation: op, Handle: h})
	}
	h := m.handler
	add("GET", "/api/org/legal-entities", core.OpEntityRead, h(entities.ListEntities))
	add("POST", "/api/org/legal-entities", core.OpEntityEdit, h(entities.CreateEntity))
	add("GET", "/api/org/legal-entities/:id", core.OpEntityRead, h(entities.GetEntity))
	add("PATCH", "/api/org/legal-entities/:id", core.OpEntityEdit, h(entities.UpdateEntity))
	add("POST", "/api/org/legal-entities/:id/state", core.OpEntityEdit, h(entities.EntityState))
	add("GET", "/api/org/legal-entities/:id/business-date", core.OpEntityRead, h(entities.BusinessDate))
	add("GET", "/api/org/units", core.OpUnitRead, h(units.ListUnits))
	add("POST", "/api/org/units", core.OpUnitEdit, h(units.CreateUnit))
	add("GET", "/api/org/units/:id", core.OpUnitRead, h(units.GetUnit))
	add("PATCH", "/api/org/units/:id", core.OpUnitEdit, h(units.RenameUnit))
	add("POST", "/api/org/units/:id/move", core.OpUnitEdit, h(units.MoveUnit))
	add("POST", "/api/org/units/:id/retire", core.OpUnitEdit, h(units.RetireUnit))
	add("GET", "/api/org/units/:id/ancestors", core.OpUnitRead, h(units.Ancestors))
	add("GET", "/api/org/units/:id/path-history", core.OpUnitRead, h(units.PathHistory))
	add("GET", "/api/org/positions", core.OpPosRead, h(positions.ListPositions))
	add("POST", "/api/org/positions", core.OpPosEdit, h(positions.CreatePosition))
	add("GET", "/api/org/positions/:id", core.OpPosRead, h(positions.GetPosition))
	add("PATCH", "/api/org/positions/:id", core.OpPosEdit, h(positions.RenamePosition))
	add("POST", "/api/org/positions/:id/retire", core.OpPosEdit, h(positions.RetirePosition))
	// in the contract, not implemented yet (D75)
	for _, u := range []struct{ method, path, op string }{
		{"GET", "/api/org/reporting-lines", core.OpUnitRead}, {"POST", "/api/org/reporting-lines", core.OpUnitEdit},
		{"GET", "/api/org/locations", core.OpEntityRead}, {"POST", "/api/org/locations", core.OpEntityEdit},
		{"GET", "/api/org/legal-entities/:id/calendar/working-day", core.OpEntityRead},
		{"GET", "/api/org/legal-entities/:id/calendar/add-working-days", core.OpEntityRead},
		{"POST", "/api/org/changes", core.OpUnitEdit},
	} {
		add(u.method, u.path, u.op, core.Unsupported)
	}
	return nil
}

// Services registers the system surface: every component of the deployment reads the organization.
func (m *Module) Services(s *grpcx.Services) error {
	s.Register(&orgv3.SourceService_ServiceDesc, m, map[string]grpcx.Method{
		"Snapshot": {Operation: core.OpSystem, Callers: []string{grpcx.AnyComponent}},
		"Read":     {Operation: core.OpSystem, Callers: []string{grpcx.AnyComponent}}})
	s.Register(&orgv3.CatchUpService_ServiceDesc, m, map[string]grpcx.Method{
		"CatchUp": {Operation: core.OpSystem, Callers: []string{grpcx.AnyComponent}}})
	return nil
}

// Replays registers the replay of org_tightening after a full recovery (evidence.Replayer).
func (m *Module) Replays(r *evidence.Replays) error {
	r.Add(tightening.Category, func(ctx context.Context, tx db.Tx, rec evidence.Replayed) error {
		return replay.Replay(m.b, ctx, tx, rec)
	})
	return nil
}

// handler adapts a module's handler, which takes the shared base first, to what the SDK registers.
func (m *Module) handler(f func(*core.Base, *httpx.Context) error) httpx.Handler {
	return func(c *httpx.Context) error { return f(m.b, c) }
}
