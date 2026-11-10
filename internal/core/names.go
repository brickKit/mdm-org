package core

import (
	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
)

// ID is the component's ID.
const ID = "mdm/org"

// The stream and its event types (contract/events).
const (
	Stream             = "mdm.org"
	LegalEntityChanged = "mdm.legal_entity.changed.v1"
	UnitChanged        = "mdm.org_unit.changed.v1"
	PositionChanged    = "mdm.org_position.changed.v1"
)

// Resources and operations (contract/openapi.yaml names each route's operation).
const (
	LegalEntities = "mdm.legal_entity"
	OpEntityRead  = "mdm.legal_entity.read"
	OpEntityEdit  = "mdm.legal_entity.manage"
	Units         = "mdm.org_unit"
	OpUnitRead    = "mdm.org_unit.read"
	OpUnitEdit    = "mdm.org_unit.manage"
	Positions     = "mdm.org_position"
	OpPosRead     = "mdm.org_position.read"
	OpPosEdit     = "mdm.org_position.manage"
	System        = "mdm.org.system"
	// OpResend is the redelivery of this component's stream (be-protocol P6.38: every component with streams serves
	// it; any component of the deployment may call it).
	OpResend = "mdm.org.system.resend"
	// OpRecover is a full recovery's checks (be-protocol chapter 14: the recovery tool only).
	OpRecover = "mdm.org.system.recover"
	// OpReplay is the replay of this member's evidence in a full recovery (be-protocol P12.19: the recovery tool only).
	OpReplay = "mdm.org.system.replay"
	OpSystem = "mdm.org.system.read"
)

// MaxBatch is the most IDs a Read takes and records a Snapshot page gives (decision 0304).
const MaxBatch = 500

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
			{Type: Positions, ViewOperation: OpPosRead, Dimensions: none, Fields: fields("id", "legal_entity_id", "unit_id",
				"code", "name", "names", "state", "version", "updated_at")},
			{Type: System, ViewOperation: OpSystem, Dimensions: none, Fields: map[string]sdkauthz.Field{}},
		},
		Operations: []sdkauthz.Operation{
			{Key: OpEntityRead, ResourceType: LegalEntities, Purpose: "read", RiskClass: "low"},
			{Key: OpEntityEdit, ResourceType: LegalEntities, Purpose: "manage", RiskClass: "high",
				SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpUnitRead, ResourceType: Units, Purpose: "read", RiskClass: "low"},
			{Key: OpUnitEdit, ResourceType: Units, Purpose: "manage", RiskClass: "high",
				SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpPosRead, ResourceType: Positions, Purpose: "read", RiskClass: "low"},
			{Key: OpPosEdit, ResourceType: Positions, Purpose: "manage", RiskClass: "high",
				SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpSystem, ResourceType: System, Purpose: "read", RiskClass: "low"},
			{Key: OpResend, ResourceType: System, Purpose: "read", RiskClass: "low"},
			{Key: OpRecover, ResourceType: System, Purpose: "read", RiskClass: "low"},
			{Key: OpReplay, ResourceType: System, Purpose: "action", RiskClass: "high", SideEffects: []string{"business_write"}},
		}}
}
