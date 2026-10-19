package org

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/brickKit/be-sdk-go/v3/audit"
	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/bizdate"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/identity"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/search"
	orgv3 "github.com/brickKit/mdm-org/contract/v3/gen/go/be/mdm/org/v3"
)

// The component's own reasons (the SDK's catalog has the shared ones).
var (
	errVersion = errs.Define(ID, "VERSION_CONFLICT", errs.Aborted, 409, errs.RetryNo, "The record changed since that version")
	errCode    = errs.Define(ID, "CODE_TAKEN", errs.AlreadyExists, 409, errs.RetryNo, "The code belongs to another record")
	errState   = errs.Define(ID, "STATE_CONFLICT", errs.FailedPrecondition, 409, errs.RetryNo,
		"The record's state does not allow this")
	errCycle = errs.Define(ID, "TREE_CYCLE", errs.FailedPrecondition, 409, errs.RetryNo,
		"A unit cannot move into its own subtree")
)

var (
	codeForm = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)
	uuidForm = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	country  = regexp.MustCompile(`^[A-Z]{2}$`)
	isoCcy   = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Module is the organization component.
type Module struct {
	orgv3.UnimplementedSourceServiceServer
	orgv3.UnimplementedCatchUpServiceServer
	rt                   *runtime.Runtime
	deployment, instance string
	defaultZone          string
}

// New builds the module; a DEFAULT_LEGAL_ENTITY_TIMEZONE that is not an IANA zone stops the start.
func New(rt *runtime.Runtime) (*Module, error) {
	m := &Module{rt: rt, deployment: rt.Config.String("DEPLOYMENT_ID"), instance: rt.Config.String("IMPLEMENTATION_INSTANCE"),
		defaultZone: rt.Config.String("DEFAULT_LEGAL_ENTITY_TIMEZONE")}
	if m.defaultZone != "" {
		if _, err := time.LoadLocation(m.defaultZone); err != nil {
			return nil, fmt.Errorf("DEFAULT_LEGAL_ENTITY_TIMEZONE %q is not an IANA time zone", m.defaultZone)
		}
	}
	return m, nil
}

// Routes registers the user surface (contract/openapi.yaml).
func (m *Module) Routes(r *httpx.Routes) error {
	add := func(method, path, op string, h httpx.Handler) {
		r.Add(httpx.Route{Method: method, Path: path, Operation: op, Handle: h})
	}
	add("GET", "/api/org/legal-entities", OpEntityRead, m.listEntities)
	add("POST", "/api/org/legal-entities", OpEntityEdit, m.createEntity)
	add("GET", "/api/org/legal-entities/:id", OpEntityRead, m.getEntity)
	add("PATCH", "/api/org/legal-entities/:id", OpEntityEdit, m.updateEntity)
	add("POST", "/api/org/legal-entities/:id/state", OpEntityEdit, m.entityState)
	add("GET", "/api/org/legal-entities/:id/business-date", OpEntityRead, m.businessDate)
	add("GET", "/api/org/units", OpUnitRead, m.listUnits)
	add("POST", "/api/org/units", OpUnitEdit, m.createUnit)
	add("GET", "/api/org/units/:id", OpUnitRead, m.getUnit)
	add("PATCH", "/api/org/units/:id", OpUnitEdit, m.renameUnit)
	add("POST", "/api/org/units/:id/move", OpUnitEdit, m.moveUnit)
	add("POST", "/api/org/units/:id/retire", OpUnitEdit, m.retireUnit)
	add("GET", "/api/org/units/:id/ancestors", OpUnitRead, m.ancestors)
	add("GET", "/api/org/units/:id/path-history", OpUnitRead, m.pathHistory)
	// in the contract, not in step 6 (D75)
	for _, u := range []struct{ method, path, op string }{
		{"GET", "/api/org/positions", OpUnitRead}, {"POST", "/api/org/positions", OpUnitEdit},
		{"GET", "/api/org/reporting-lines", OpUnitRead}, {"POST", "/api/org/reporting-lines", OpUnitEdit},
		{"GET", "/api/org/locations", OpEntityRead}, {"POST", "/api/org/locations", OpEntityEdit},
		{"GET", "/api/org/legal-entities/:id/calendar/working-day", OpEntityRead},
		{"GET", "/api/org/legal-entities/:id/calendar/add-working-days", OpEntityRead},
		{"POST", "/api/org/changes", OpUnitEdit},
	} {
		add(u.method, u.path, u.op, unsupported)
	}
	return nil
}

func unsupported(c *httpx.Context) error {
	if _, _, err := check(c, c.Operation()); err != nil {
		return err
	}
	return errs.New(errs.ReasonCapabilityUnsupported, "not in this version of mdm/org (positions, reporting lines, "+
		"locations, work calendars and effective-dated changes come later)")
}

// Services registers the system surface: every component of the deployment reads the organization.
func (m *Module) Services(s *grpcx.Services) error {
	s.Register(&orgv3.SourceService_ServiceDesc, m, map[string]grpcx.Method{
		"Snapshot": {Operation: OpSystem, Callers: []string{grpcx.AnyComponent}},
		"Read":     {Operation: OpSystem, Callers: []string{grpcx.AnyComponent}}})
	s.Register(&orgv3.CatchUpService_ServiceDesc, m, map[string]grpcx.Method{
		"CatchUp": {Operation: OpSystem, Callers: []string{grpcx.AnyComponent}}})
	return nil
}

// CatchUp is the watermark source's catch-up (protocol 02, S6): not in step 6.
func (m *Module) CatchUp(context.Context, *orgv3.CatchUpRequest) (*orgv3.CatchUpResponse, error) {
	return nil, errs.New(errs.ReasonCapabilityUnsupported, "mdm/org is not yet a watermark source (protocol 02, S6)")
}

// check admits the request's subject to op and returns the context to work in and the acting principal.
func check(c *httpx.Context, op string) (context.Context, string, error) {
	s, _ := sdkauthz.From(c.Ctx())
	ctx, _, err := s.Check(c.Ctx(), op, sdkauthz.Facts{})
	if err != nil {
		return ctx, "", err
	}
	u, _ := identity.UserFrom(ctx)
	return ctx, u.Subject, nil
}

func invalid(field, why string) error {
	return errs.New(errs.ReasonRequestInvalid, why).WithViolations(errs.Violation{Field: field, Reason: "INVALID", Description: why})
}

func notFound(what string) error { return errs.New(errs.ReasonNotFound, what+" not found") }

// change records the business change with the record's new state (protocol 03), in the transaction that made it.
func (m *Module) change(ctx context.Context, tx db.Tx, actor, op, typ, id string, after any) error {
	_, err := m.rt.Audit.Write(ctx, tx, audit.Record{Category: audit.BusinessChange, Operation: op, Outcome: audit.Succeeded,
		Actor: actor, Target: map[string]any{"type": typ, "id": id}, Changes: map[string]any{"after": after}})
	return err
}

// checkNames refuses translations without a language tag or a name.
func checkNames(names map[string]string) error {
	for tag, n := range names {
		if tag == "" || len(tag) > 35 || strings.TrimSpace(n) == "" {
			return invalid("names", "names maps a language tag to a non-empty name")
		}
	}
	return nil
}

func searchOf(code, name string, names map[string]string) string {
	vals := []string{code, name}
	for _, n := range names {
		vals = append(vals, n)
	}
	return search.Text(vals...)
}

type entityIn struct {
	Version              *int64             `json:"version"`
	Code                 string             `json:"code"`
	Name                 *string            `json:"name"`
	Names                *map[string]string `json:"names"`
	Country              string             `json:"country"`
	Timezone             *string            `json:"timezone"`
	BaseCurrency         *string            `json:"base_currency"`
	FiscalYearStartMonth *int               `json:"fiscal_year_start_month"`
}

// validate checks what is given; create says which fields must be there.
func (in entityIn) validate(create bool) error {
	switch {
	case create && !codeForm.MatchString(in.Code):
		return invalid("code", "code is upper case letters, digits, _ and -, at most 32")
	case create && !country.MatchString(in.Country):
		return invalid("country", "country is an ISO 3166-1 alpha-2 code")
	case create && (in.Name == nil || in.BaseCurrency == nil):
		return invalid("name", "name and base_currency are required")
	case in.Name != nil && strings.TrimSpace(*in.Name) == "":
		return invalid("name", "name cannot be empty")
	case in.BaseCurrency != nil && !isoCcy.MatchString(*in.BaseCurrency):
		return invalid("base_currency", "base_currency is an ISO 4217 code")
	case in.FiscalYearStartMonth != nil && (*in.FiscalYearStartMonth < 1 || *in.FiscalYearStartMonth > 12):
		return invalid("fiscal_year_start_month", "fiscal_year_start_month is 1-12")
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil || *in.Timezone == "" || *in.Timezone == "Local" {
			return invalid("timezone", "timezone is an IANA time zone")
		}
	}
	if in.Names != nil {
		return checkNames(*in.Names)
	}
	return nil
}

func (m *Module) createEntity(c *httpx.Context) error {
	ctx, actor, err := check(c, OpEntityEdit)
	if err != nil {
		return err
	}
	var in entityIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Timezone == nil && m.defaultZone != "" { // written into the record: a later change of the key changes nothing
		in.Timezone = &m.defaultZone
	}
	if in.Timezone == nil {
		return invalid("timezone", "a legal entity needs a time zone: give one, or set DEFAULT_LEGAL_ENTITY_TIMEZONE (protocol 14)")
	}
	if err := in.validate(true); err != nil {
		return err
	}
	names := map[string]string{}
	if in.Names != nil {
		names = *in.Names
	}
	month := 1
	if in.FiscalYearStartMonth != nil {
		month = *in.FiscalYearStartMonth
	}
	want := LegalEntity{Code: in.Code, Name: *in.Name, Names: names, Country: in.Country, Timezone: *in.Timezone,
		BaseCurrency: *in.BaseCurrency, FiscalYearStartMonth: month, State: "active"}
	var out LegalEntity
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var existing string
		err := tx.QueryRow(ctx, `SELECT id::text FROM legal_entity WHERE code = $1 FOR UPDATE`, in.Code).Scan(&existing)
		if err == nil { // the same creation again returns it; other content under the code is refused
			cur, err := getEntity(ctx, tx, existing, false)
			if err != nil {
				return err
			}
			if cur.Name == want.Name && cur.Country == want.Country && cur.Timezone == want.Timezone &&
				cur.BaseCurrency == want.BaseCurrency && cur.FiscalYearStartMonth == want.FiscalYearStartMonth {
				out = cur
				return nil
			}
			return errCode.New("legal entity code " + in.Code + " is taken")
		}
		id := ids.New().String()
		nb, _ := json.Marshal(names)
		if _, err := tx.Exec(ctx, `INSERT INTO legal_entity (id, code, name, names, country, timezone, base_currency,
			fiscal_year_start_month, search, search_version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (code) DO NOTHING`, id, want.Code, want.Name, nb, want.Country, want.Timezone, want.BaseCurrency,
			want.FiscalYearStartMonth, searchOf(want.Code, want.Name, names), search.Version); err != nil {
			return err
		}
		if out, err = getEntity(ctx, tx, id, false); err != nil {
			return err
		}
		if err := m.emit(ctx, tx, LegalEntityChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		return m.change(ctx, tx, actor, OpEntityEdit, LegalEntities, out.ID, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func (m *Module) getEntity(c *httpx.Context) error {
	ctx, _, err := check(c, OpEntityRead)
	if err != nil {
		return err
	}
	e, err := m.entity(ctx, c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(200, e)
}

func (m *Module) entity(ctx context.Context, id string) (LegalEntity, error) {
	if !uuidForm.MatchString(id) {
		return LegalEntity{}, notFound("legal entity")
	}
	var e LegalEntity
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		e, err = getEntity(ctx, tx, id, false)
		return err
	})
	if errors.Is(err, errNone) {
		return e, notFound("legal entity")
	}
	return e, err
}

// updateEntity changes names, time zone, base currency or the fiscal year's start month; the same values again
// change nothing.
func (m *Module) updateEntity(c *httpx.Context) error {
	ctx, actor, err := check(c, OpEntityEdit)
	if err != nil {
		return err
	}
	var in entityIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Code != "" || in.Country != "" {
		return invalid("code", "code and country do not change: a legal entity in another country is another legal entity")
	}
	if err := in.validate(false); err != nil {
		return err
	}
	return m.updateEntityWith(c, ctx, actor, in.Version, func(e *LegalEntity) error {
		if in.Name != nil {
			e.Name = *in.Name
		}
		if in.Names != nil {
			e.Names = *in.Names
		}
		if in.Timezone != nil {
			e.Timezone = *in.Timezone
		}
		if in.BaseCurrency != nil {
			e.BaseCurrency = *in.BaseCurrency
		}
		if in.FiscalYearStartMonth != nil {
			e.FiscalYearStartMonth = *in.FiscalYearStartMonth
		}
		return nil
	})
}

// entityState moves a legal entity through protocol 07's lifecycle: active and suspended both ways, either to
// retired, never back from retired.
func (m *Module) entityState(c *httpx.Context) error {
	ctx, actor, err := check(c, OpEntityEdit)
	if err != nil {
		return err
	}
	var in struct {
		State   string `json:"state"`
		Version *int64 `json:"version"`
	}
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.State != "active" && in.State != "suspended" && in.State != "retired" {
		return invalid("state", "state is active, suspended or retired")
	}
	return m.updateEntityWith(c, ctx, actor, in.Version, func(e *LegalEntity) error {
		if e.State == "retired" && in.State != "retired" {
			return errState.New("a retired legal entity stays retired")
		}
		e.State = in.State
		return nil
	})
}

func (m *Module) updateEntityWith(c *httpx.Context, ctx context.Context, actor string, version *int64,
	apply func(*LegalEntity) error) error {
	id := c.Param("id")
	if !uuidForm.MatchString(id) {
		return notFound("legal entity")
	}
	var out LegalEntity
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		cur, err := getEntity(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if version != nil && *version != cur.Version {
			return errVersion.New(fmt.Sprintf("the legal entity is at version %d", cur.Version))
		}
		next := cur
		next.Names = map[string]string{}
		for k, v := range cur.Names {
			next.Names[k] = v
		}
		if err := apply(&next); err != nil {
			return err
		}
		if same(cur, next) {
			out = cur
			return nil
		}
		nb, _ := json.Marshal(next.Names)
		if _, err := tx.Exec(ctx, `UPDATE legal_entity SET name = $2, names = $3, timezone = $4, base_currency = $5,
			fiscal_year_start_month = $6, state = $7, search = $8, search_version = $9, version = version + 1,
			updated_at = now() WHERE id = $1`, id, next.Name, nb, next.Timezone, next.BaseCurrency,
			next.FiscalYearStartMonth, next.State, searchOf(next.Code, next.Name, next.Names), search.Version); err != nil {
			return err
		}
		if out, err = getEntity(ctx, tx, id, false); err != nil {
			return err
		}
		if err := m.emit(ctx, tx, LegalEntityChanged, out.ID, out.Version, out); err != nil {
			return err
		}
		return m.change(ctx, tx, actor, OpEntityEdit, LegalEntities, out.ID, out)
	})
	if errors.Is(err, errNone) {
		return notFound("legal entity")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func same(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// businessDate is the legal entity's business date at an instant (protocol 14: in the legal entity's time zone).
func (m *Module) businessDate(c *httpx.Context) error {
	ctx, _, err := check(c, OpEntityRead)
	if err != nil {
		return err
	}
	at := time.Now()
	if v := c.Query("at"); v != "" {
		if at, err = time.Parse(time.RFC3339Nano, v); err != nil {
			return invalid("at", "at is an RFC 3339 instant")
		}
	}
	e, err := m.entity(ctx, c.Param("id"))
	if err != nil {
		return err
	}
	d, err := bizdate.Of(at, e.Timezone)
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]string{"date": d, "timezone": e.Timezone})
}
