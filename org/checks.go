package org

import (
	"github.com/brickKit/be-sdk-go/v3/recovery"
	core "github.com/brickKit/mdm-org/v3/internal/core"
)

// RecoveryChecks registers what a full recovery asks this member (be-protocol chapter 14): whether the objects this
// member audits are still here, at which version, and the checks it reports.
func (m *Module) RecoveryChecks(c *recovery.Checks) error {
	c.Presence(core.LegalEntities, recovery.TablePresence("legal_entity", "id", "version"))
	c.Presence(core.Units, recovery.TablePresence("org_unit", "id", "version"))
	c.Presence(core.Positions, recovery.TablePresence("org_position", "id", "version"))
	return nil
}
