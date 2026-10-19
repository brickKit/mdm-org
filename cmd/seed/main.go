// Command seed fills a running mdm/org with the seed data for people (be-assembly-standard's seed and test data
// conventions): legal entities in three countries and two negative states, and a department tree of three levels in
// each, enough to page through, with one retired unit and one moved unit (two paths in its history). Every name
// starts with 「本地测试」; codes are the fixed idempotency keys, so a second run creates nothing.
//
//	SEED_IAM_URL (http://localhost:18200)  the identity member's development exchange
//	SEED_USER (test.bob), SEED_PASSWORD     a test user granted mdm/org's operations
//	MDM_ORG_URL (http://localhost:18083)
//
// Run: go run ./cmd/seed (the component and the identity member up; mdm/org has no seed dependencies).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const prefix = "「本地测试」"

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type client struct{ base, token string }

func (c client) do(method, path string, body any) (int, map[string]any) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (c client) must(method, path string, body any) map[string]any {
	code, out := c.do(method, path, body)
	if code != 200 {
		fail("%s %s: %d %v", method, path, code, out)
	}
	return out
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "seed mdm/org: "+format+"\n", a...)
	os.Exit(1)
}

func token() string {
	pw := os.Getenv("SEED_PASSWORD")
	if pw == "" {
		fail("SEED_PASSWORD is not set (the test users' password in Casdoor)")
	}
	b, _ := json.Marshal(map[string]string{"username": env("SEED_USER", "test.bob"), "password": pw})
	resp, err := http.Post(env("SEED_IAM_URL", "http://localhost:18200")+"/api/iam/dev/token", "application/json", bytes.NewReader(b))
	if err != nil {
		fail("the identity member is not reachable: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&out) != nil {
		fail("the development exchange refused the seed user: %d", resp.StatusCode)
	}
	return out.AccessToken
}

func main() {
	base := env("MDM_ORG_URL", "http://localhost:18083")
	if r, err := http.Get(base + "/readyz"); err != nil || r.StatusCode != 200 { // probe before pouring
		fail("mdm/org at %s is not ready", base)
	}
	c := client{base, token()}
	type le struct{ code, name, country, tz, ccy, state string }
	entities := []le{
		{"CN01", "中国（上海）", "CN", "Asia/Shanghai", "CNY", "active"},
		{"ES01", "España (Madrid)", "ES", "Europe/Madrid", "EUR", "active"},
		{"US01", "United States (New York)", "US", "America/New_York", "USD", "active"},
		{"HK01", "香港（暂停）", "HK", "Asia/Hong_Kong", "HKD", "suspended"},
		{"SG01", "Singapore (closed)", "SG", "Asia/Singapore", "SGD", "retired"},
	}
	ids := map[string]string{}
	for _, e := range entities {
		out := c.must("POST", "/api/org/legal-entities", map[string]any{"code": e.code, "name": prefix + e.name,
			"country": e.country, "timezone": e.tz, "base_currency": e.ccy, "fiscal_year_start_month": 1})
		ids[e.code] = out["id"].(string)
		if out["state"] != e.state {
			c.must("POST", "/api/org/legal-entities/"+ids[e.code]+"/state", map[string]any{"state": e.state})
		}
	}
	units := 0
	for _, code := range []string{"CN01", "ES01", "US01"} {
		le := ids[code]
		byCode := func(ucode string) string {
			_, list := c.do("GET", "/api/org/units?legal_entity_id="+le+"&q="+url.QueryEscape(ucode)+"&limit=200", nil)
			for _, u := range list["units"].([]any) {
				if u.(map[string]any)["code"] == ucode {
					return u.(map[string]any)["id"].(string)
				}
			}
			return ""
		}
		unit := func(ucode, name string, parent any) string {
			units++
			code, out := c.do("POST", "/api/org/units", map[string]any{"legal_entity_id": le, "parent_id": parent,
				"code": ucode, "name": prefix + name})
			if code == 409 && out["reason"] == "CODE_TAKEN" { // seeded before and changed since (moved): keep it
				if id := byCode(ucode); id != "" {
					return id
				}
			}
			if code != 200 {
				fail("unit %s: %d %v", ucode, code, out)
			}
			return out["id"].(string)
		}
		sales := unit("SALES", "销售部", nil)
		for _, region := range []struct{ code, name string }{{"NORTH", "北区"}, {"SOUTH", "南区"}, {"EAST", "东区"}, {"WEST", "西区"}} {
			r := unit("SALES-"+region.code, "销售"+region.name, sales)
			unit("SALES-"+region.code+"-KA", "销售"+region.name+"大客户组", r)
			unit("SALES-"+region.code+"-SMB", "销售"+region.name+"中小客户组", r)
		}
		ops := unit("OPS", "运营部", nil)
		unit("OPS-WH", "仓储组", ops)
		unit("OPS-LOG", "物流组", ops)
		fin := unit("FIN", "财务部", nil)
		unit("FIN-AR", "应收组", fin)
		unit("FIN-AP", "应付组", fin)
		old := unit("PROJ-OLD", "旧项目组（已撤销）", nil)
		if _, u := c.do("GET", "/api/org/units/"+old, nil); u["state"] != "retired" {
			c.must("POST", "/api/org/units/"+old+"/retire", nil)
		}
		// a unit that moved: its path history has two entries (moved only on the first run)
		moved := unit("SALES-EAST-NEW", "销售东区新组（自北区调入）", sales)
		if h := c.must("GET", "/api/org/units/"+moved+"/path-history", nil); len(h["history"].([]any)) == 1 {
			c.must("POST", "/api/org/units/"+moved+"/move", map[string]any{"parent_id": byCode("SALES-EAST")})
		}
	}
	fmt.Printf("seed mdm/org: %d legal entities, %d units (%s)\n", len(entities), units, time.Now().Format(time.RFC3339))
}
