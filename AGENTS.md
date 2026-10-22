# mdm/org

法人与组织组件的代码。用法与边界在 `BRICKKIT.md`，为什么这样设计（术语、数据、状态与规则、未决问题）在 [docs/design.md](docs/design.md)；跨组件的规则在装配项目 be-assembly-standard 的协议 02（来源）、协议 07（生命周期）、协议 14（时间与日历、列表）、安全 07（组织树维度）。

## 代码地图

| 位置 | 内容 |
| --- | --- |
| `org/definition.go` | 组件定义：身份、配置键、流与事件类型、资源与操作 |
| `org/module.go` | 路由与系统面的注册、法人的新建、修改、状态与业务日期；本版不做的路由 |
| `org/units.go` | 部门：新建、改名、移动（`repath`：子树的路径与历史）、退役、上级、路径历史 |
| `org/lists.go` | 两个列表 |
| `org/records.go` | 记录的类型、读取与事件 |
| `org/source.go` | 来源方法 |
| `org/migrations/` | 表：法人、部门、路径历史 |
| `cmd/seed/` | 种子：角色与授予（含撤销、过期的反例）、法人、部门树（`make seed`） |
| `contract/` | 本组件发布的契约，单独的模块：`contract/proto/`、`contract/events/`、`contract/openapi.yaml` |
| `docs/design.md` | 设计：术语、表、状态与规则、命令、事件、实现状态、未决问题 |

| 想改 | 从这里开始 | 然后 |
| --- | --- | --- |
| 部门树的规则（无环、移动、退役） | `units.go` 的 `moveUnit`、`repath`、`retireUnit` | 集成测试的树一段 |
| 法人的生命周期 | `module.go` 的 `entityState` | 协议 07 的状态表 |
| 加一个用户面路由 | `contract/openapi.yaml` | `module.go` 的 `Routes`；`contract_test.go` 核对 |
| 法人的业务日期、时区 | `module.go` 的 `businessDate` | 集成测试的 Europe/Madrid 一段 |
| 加字段、加接口、改事件 | 装配项目 be-assembly-standard 规范里的“改动配方” | `docs/design.md` 的数据与命令 |

## 构建与测试

- `go test ./...`（`make test`）：清单的 `configSchema` 与 `events`、OpenAPI 与路由、事件载荷、合并安全。
- `go test -tags integration ./org/`（`make test-integration`）：真实的 PostgreSQL 与消息系统，例如 `BE_SDK_TEST_PG_ADMIN=postgres://postgres:<口令>@localhost:5432/brickkit_test_db`（装配项目 `make up` 的 `be-postgres`，口令是 `.env` 的 `POSTGRES_PASSWORD`）与 `BE_SDK_TEST_NATS_URL=nats://localhost:4222`；be-sdk-go 的 `sectest` 是测试部署。
- `brickkit lint`

## 设计取舍

- **树的每次结构变化串行**：新建、移动、退役先锁法人的行。两个并发的移动各自看不到对方时可能拼出环，串行之后每次检查的都是对方留下的树；组织调整很少，串行不是瓶颈。
- **路径用身份，不用代码**：改名、改代码都不改变授权范围；只有移动改变路径。
- **移动是一次变化，下面的部门各自发事件**：消费者按记录的版本应用，不需要理解“子树”。

## 易错点

- 不要按字符串前缀比较路径：子树是“前缀加 `/`”，`a/b` 不是 `a/bc` 的上级（安全 07）。
- 没有部门的主体不贡献部门条件；空路径不是根（安全 07、易错点表）。

## 改代码前自查

- 改了契约（`contract/`）：proto 改后 `make gen`，发布 `contract/v…`，再改这里的依赖；OpenAPI 与路由由测试核对。
- 改了版本号吗？每次改动都升版本。

<!-- brickkit:managed:begin lang=zh -->
<!-- 由 brickkit 维护（init、add、remove、upgrade、skills update）：这对标记之间的改动会被覆盖 -->

## BrickKit

这是一个 BrickKit 组件：平台只读 `component.yaml`。它依赖的规则：

- `configSchema` 的键就是代码读的环境变量名。不能用保留名：`COMPONENT_ID`、`COMPONENT_VERSION`、`PORT`、`BRICKKIT_SERVED_MEMBERS`、`BRICKKIT_SERVED_MEMBERS_CONFIG`，以及任何 `*_ENDPOINT`。
- 依赖写精确版本。依赖的地址以 `<ID>_ENDPOINT` 注入；缺席的可选依赖根本没有这个变量，读的时候要带兜底。
- `/healthz` 只查本进程，不查依赖。迁移命令用同一个镜像跑，遇到不认识的参数必须直接失败；`local.migrationCommand` 从源码跑同一条迁移，给 `mode: local` 用。
- `BRICKKIT.md` 会随版本进入每个使用它的项目，在那里是脱离仓库单独读的：跟代码一起改，不放相对链接。
- 发版：改 `metadata.version`，提交、推送，`brickkit release`。`brickkit lint` 会检查清单和这些文档——在项目里、在这个目录下跑，只查这个组件（`--all` 查整个项目）。
- 完整规则在 `brickkit-component` 技能里（装了技能的项目或仓库根目录下的 `.claude/skills/brickkit-component/SKILL.md`；`brickkit skills update` 会装上）；参数问 `brickkit <命令> --help`；BrickKit 自己的文档用 `brickkit docs` 看。
<!-- brickkit:managed:end -->
