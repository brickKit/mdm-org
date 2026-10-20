// Command seed fills a running mdm/org with the seed data for people (be-assembly-standard's seed and test data
// conventions):
//
//   - access: the roles of mdm/org's operations (maintain, read) granted to the seed accounts of infra/iam-casdoor's
//     seed (make -C ../../infra/iam-casdoor seed), by the authorization admin; test.revoked held the read role and lost
//     it, test.expired held it until 2026-02-01. The seed user is granted the maintain role first and waits until
//     mdm/org takes it;
//   - legal entities in five countries, three usable, one suspended and one retired;
//   - a department tree under each usable one: CN01 with more than one page (53 units: sales regions with city teams,
//     and the other departments), ES01 and US01 with 21 each; one retired unit and one moved unit (two paths in its
//     history) in each.
//
// Every name starts with 「本地测试」; codes and change keys are the fixed idempotency keys, so a second run creates
// nothing. HK01 is suspended here; the partner and product seeds open their data under it first (they resume it,
// open, and suspend it again), as business comes before a suspension.
//
//	SEED_IAM_URL (http://localhost:18200)    the identity member's development exchange
//	SEED_AUTHZ_URL (http://localhost:18223)  the authorization member's effect point
//	SEED_ADMIN (test.alice)                  the authorization admin (the install claim)
//	SEED_USER (test.bob), SEED_PASSWORD      the user that writes the data; the test accounts' password
//	MDM_ORG_URL (http://localhost:18083)
//
// Run: go run ./cmd/seed (the component and the security members up; mdm/org has no seed dependencies).
package main

import (
	"fmt"
	"net/url"
	"os"
	"time"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/seed"
)

const prefix = "「本地测试」"

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "seed mdm/org: "+format+"\n", a...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}

type client struct{ seed.Client }

func (c client) do(method, path string, body any) (int, map[string]any) {
	code, out, err := c.Do(method, path, body)
	check(err)
	return code, out
}

func (c client) must(method, path string, body any) map[string]any {
	out, err := c.Must(method, path, body)
	check(err)
	return out
}

// readers are the seed accounts that read the organization (their own scopes come with the business components).
var readers = []string{"test.mdm.viewer", "test.sales.cn", "test.sales.es", "test.sales.multi", "test.unit.admin.cn"}

// access grants mdm/org's roles and answers the seed user's client once the maintain role has reached mdm/org.
func access(base string) client {
	pw := os.Getenv("SEED_PASSWORD")
	if pw == "" {
		fail("SEED_PASSWORD is not set (the test accounts' password in Casdoor)")
	}
	iam, user := env("SEED_IAM_URL", "http://localhost:18200"), env("SEED_USER", "test.bob")
	accounts, err := seed.Accounts(iam, pw, append([]string{env("SEED_ADMIN", "test.alice"), user, "test.mdm.admin",
		"test.revoked", "test.expired"}, readers...)...)
	check(err)
	a, err := seed.NewAuthz(env("SEED_AUTHZ_URL", "http://localhost:18223"), accounts[env("SEED_ADMIN", "test.alice")].Token, "mdm/org")
	check(err)
	read := []sdkauthz.Segment{seed.Segment("mdm.legal_entity", "mdm.legal_entity.read"), seed.Segment("mdm.org_unit", "mdm.org_unit.read")}
	roles, err := a.GrantRoles([]seed.RoleSpec{
		{Key: "maintain", Name: prefix + "组织维护", To: []string{user, "test.mdm.admin"}, Segments: append(read,
			seed.Segment("mdm.legal_entity", "mdm.legal_entity.manage"), seed.Segment("mdm.org_unit", "mdm.org_unit.manage"))},
		{Key: "read", Name: prefix + "组织只读", To: append([]string{"test.revoked"}, readers...), Segments: read},
	}, accounts)
	check(err)
	// negative: a grant revoked, and one whose validity has ended
	g, err := a.GrantRole("read", "", accounts["test.revoked"].Principal, roles["read"])
	check(err)
	_, err = a.Revoke("read", g)
	check(err)
	ended := make([]sdkauthz.Segment, len(read))
	for i, s := range read {
		s.Valid = sdkauthz.Window{From: seed.Since, Until: "2026-02-01T00:00:00Z"}
		ended[i] = s
	}
	_, err = a.Grant("read-ended", "", accounts["test.expired"].Principal, ended)
	check(err)
	c := client{seed.NewClient(base, accounts[user].Token)}
	check(c.WaitGranted("POST", "/api/org/legal-entities", 30*time.Second))
	return c
}

func main() {
	base := env("MDM_ORG_URL", "http://localhost:18083")
	if !seed.Ready(base) { // probe before pouring
		fail("mdm/org at %s is not ready", base)
	}
	c := access(base)
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
		if out["state"] != e.state { // a retired one stays retired: SG01 is retired from the first run on
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
		for _, region := range []struct {
			code, name string
			cities     []string
		}{{"NORTH", "北区", []string{"北京", "天津", "济南", "石家庄"}}, {"SOUTH", "南区", []string{"广州", "深圳", "厦门", "福州"}},
			{"EAST", "东区", []string{"上海", "杭州", "南京", "苏州"}}, {"WEST", "西区", []string{"成都", "重庆", "西安", "昆明"}}} {
			r := unit("SALES-"+region.code, "销售"+region.name, sales)
			unit("SALES-"+region.code+"-KA", "销售"+region.name+"大客户组", r)
			unit("SALES-"+region.code+"-SMB", "销售"+region.name+"中小客户组", r)
			if code == "CN01" { // city teams: CN01's tree needs more than one page
				for i, city := range region.cities {
					unit(fmt.Sprintf("SALES-%s-C%d", region.code, i+1), city+"销售组", r)
				}
			}
		}
		ops := unit("OPS", "运营部", nil)
		unit("OPS-WH", "仓储组", ops)
		unit("OPS-LOG", "物流组", ops)
		fin := unit("FIN", "财务部", nil)
		unit("FIN-AR", "应收组", fin)
		unit("FIN-AP", "应付组", fin)
		if code == "CN01" {
			for _, d := range []struct {
				code, name string
				teams      [][2]string
			}{{"RD", "研发部", [][2]string{{"FE", "前端组"}, {"BE", "后端组"}, {"QA", "测试组"}}},
				{"HR", "人力资源部", [][2]string{{"REC", "招聘组"}, {"PAY", "薪酬组"}}},
				{"PUR", "采购部", [][2]string{{"RAW", "原料采购组"}, {"EQ", "设备采购组"}}},
				{"QC", "质量部", [][2]string{{"IN", "来料检验组"}, {"OUT", "成品检验组"}}},
				{"CS", "客服部", [][2]string{{"PRE", "售前组"}, {"POST", "售后组"}}}} {
				p := unit(d.code, d.name, nil)
				for _, t := range d.teams {
					unit(d.code+"-"+t[0], t[1], p)
				}
			}
		}
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
	fmt.Printf("seed mdm/org: %d legal entities, %d units; roles maintain and read, 1 revoked, 1 ended (%s)\n",
		len(entities), units, time.Now().Format(time.RFC3339))
}
