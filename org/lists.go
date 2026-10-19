package org

import (
	"context"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/list"
)

// The lists of the user surface (protocol 14: cursor pages, a stable order ending in the record's ID, the filters and
// keyword declared here). Every field is ordinary, so a row is listed whole or not at all.
var (
	entityList = list.Spec{Operation: OpEntityRead, Alias: "e", IDColumn: "id", IDKind: list.UUID,
		Columns: []list.Column{
			{Field: "code", Column: "code", Kind: list.Text, Sortable: true},
			{Field: "state", Column: "state", Kind: list.Text, Filterable: true}},
		Search: []list.SearchColumn{{Name: "names", Column: "search", Fields: []string{"code", "name", "names"}}}}
	unitList = list.Spec{Operation: OpUnitRead, Alias: "u", IDColumn: "id", IDKind: list.UUID,
		Columns: []list.Column{
			{Field: "path", Column: "path", Kind: list.Text, Sortable: true},
			{Field: "legal_entity_id", Column: "legal_entity_id", Kind: list.UUID, Filterable: true},
			{Field: "parent_id", Column: "parent_id", Kind: list.UUID, Filterable: true},
			{Field: "state", Column: "state", Kind: list.Text, Filterable: true}},
		Search: []list.SearchColumn{{Name: "names", Column: "search", Fields: []string{"code", "name", "names"}}}}
)

func build(c *httpx.Context, spec list.Spec, req list.Request) (context.Context, *list.Query, error) {
	limit, err := list.ParseLimit(c.Query("limit"))
	if err != nil {
		return nil, nil, err
	}
	req.Limit, req.Cursor = limit, c.Query("cursor")
	s, _ := sdkauthz.From(c.Ctx())
	return list.Build(c.Ctx(), s, spec, req)
}

func eq(c *httpx.Context, fields ...string) []list.Filter {
	var out []list.Filter
	for _, f := range fields {
		if v := c.Query(f); v != "" {
			out = append(out, list.Filter{Field: f, Op: "eq", Values: []string{v}})
		}
	}
	return out
}

func (m *Module) listEntities(c *httpx.Context) error {
	ctx, q, err := build(c, entityList, list.Request{Filters: eq(c, "state"), Q: c.Query("q"),
		Sort: []list.Order{{Field: "code"}}})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.rt.DB, q, "legal_entity", entityCols, scanEntity,
		func(e LegalEntity) ([]string, string) { return []string{e.Code}, e.ID })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"legal_entities": out, "next_cursor": next})
}

func (m *Module) listUnits(c *httpx.Context) error {
	for _, f := range []string{"legal_entity_id", "parent_id"} {
		if v := c.Query(f); v != "" && !uuidForm.MatchString(v) {
			return invalid(f, f+" is an ID")
		}
	}
	ctx, q, err := build(c, unitList, list.Request{Filters: eq(c, "legal_entity_id", "parent_id", "state"),
		Q: c.Query("q"), Sort: []list.Order{{Field: "path"}}})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.rt.DB, q, "org_unit", unitCols, scanUnit,
		func(u Unit) ([]string, string) { return []string{u.Path}, u.ID })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"units": out, "next_cursor": next})
}
