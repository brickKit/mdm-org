// Package org is mdm/org (boundary card mdm/org): legal entities (with their time zone, base currency and the fiscal
// year's start month) and the organization units below them, whose paths are the org-tree facts of authorization
// (security 07). Kept by people over the user surface, read by components over the system surface (protocol 02's
// source methods), published as full-state events. Step 6 of the representative deployment experiment: positions,
// reporting lines, locations, work calendars, effective-dated changes and the watermark catch-up answer
// CAPABILITY_UNSUPPORTED (D75).
package org

import (
	"embed"
	"io/fs"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/brickKit/be-sdk-go/v3/runtime"
)

// Identity of the component.
const (
	ID       = "mdm/org"
	Version  = "3.0.0"
	GRPCPort = 9093
)

// The stream and its event types (contract/events).
const (
	Stream             = "mdm.org"
	LegalEntityChanged = "mdm.legal_entity.changed.v1"
	UnitChanged        = "mdm.org_unit.changed.v1"
)

// Resources and operations (contract/openapi.yaml names each route's operation).
const (
	LegalEntities = "mdm.legal_entity"
	OpEntityRead  = "mdm.legal_entity.read"
	OpEntityEdit  = "mdm.legal_entity.manage"
	Units         = "mdm.org_unit"
	OpUnitRead    = "mdm.org_unit.read"
	OpUnitEdit    = "mdm.org_unit.manage"
	System        = "mdm.org.system"
	OpSystem      = "mdm.org.system.read"
)

// MaxBatch is the most IDs a Read takes and records a Snapshot page gives (decision 0304).
const MaxBatch = 500

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrations() fs.FS {
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}

// Keys are the component's own configuration keys.
func Keys() []config.Key {
	return []config.Key{
		// the time zone a legal entity gets when its creation gives none (protocol 14); written into the record, so a
		// later change of the key changes no existing legal entity. Unset: a creation without a time zone is refused
		{Name: "DEFAULT_LEGAL_ENTITY_TIMEZONE", Type: config.String},
	}
}

// Own describes the own keys for the manifest's configSchema.
var Own = map[string]string{"DEFAULT_LEGAL_ENTITY_TIMEZONE": "the IANA time zone a legal entity gets when its " +
	"creation gives none, written into the record; unset: a creation without a time zone is refused"}

// Definitions are the component's resources and operations. No dimensions in step 6, declared as such (an empty set,
// not a missing one).
func Definitions() *sdkauthz.Definitions {
	none := map[string]sdkauthz.Dimension{}
	fields := func(names ...string) map[string]sdkauthz.Field {
		out := map[string]sdkauthz.Field{}
		for _, n := range names {
			out[n] = sdkauthz.Field{Class: sdkauthz.Ordinary}
		}
		return out
	}
	return &sdkauthz.Definitions{Component: ID,
		Resources: []sdkauthz.Resource{
			{Type: LegalEntities, ViewOperation: OpEntityRead, Dimensions: none, Fields: fields("id", "code", "name", "names",
				"country", "timezone", "base_currency", "fiscal_year_start_month", "state", "version", "updated_at")},
			{Type: Units, ViewOperation: OpUnitRead, Dimensions: none, Fields: fields("id", "legal_entity_id", "parent_id",
				"code", "name", "names", "path", "state", "version", "updated_at")},
			{Type: System, ViewOperation: OpSystem, Dimensions: none, Fields: map[string]sdkauthz.Field{}},
		},
		Operations: []sdkauthz.Operation{
			{Key: OpEntityRead, ResourceType: LegalEntities, Purpose: "read", RiskClass: "low"},
			{Key: OpEntityEdit, ResourceType: LegalEntities, Purpose: "manage", RiskClass: "high",
				SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpUnitRead, ResourceType: Units, Purpose: "read", RiskClass: "low"},
			{Key: OpUnitEdit, ResourceType: Units, Purpose: "manage", RiskClass: "high",
				SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpSystem, ResourceType: System, Purpose: "read", RiskClass: "low"},
		}}
}

// Definition is the component as the launcher and the shells take it.
func Definition() component.Definition {
	return component.Definition{ID: ID, Version: Version, Keys: Keys(), Migrations: migrations(), GRPCPort: GRPCPort,
		Streams: []events.Stream{{Name: Stream, Partitions: 4}}, Authz: Definitions(),
		New: func(rt *runtime.Runtime) (component.Module, error) { return New(rt) }}
}
