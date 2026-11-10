# org/：入口与组装

组件对外的一个包：`main.go` 与外壳只引用它（`org.Definition()`、`org.New`）。这里只**组装**：把 `internal/` 下模块的函数登记成 SDK 要的路由、gRPC 方法、证据的重放与恢复核对；业务一行也不写。

## 负责与不负责

- 负责：组件的版本、端口、配置键（`Keys`、`Own`：`DEFAULT_LEGAL_ENTITY_TIMEZONE`）、迁移、组件定义；`New` 检查默认时区是 IANA 时区；哪个路由、哪个 gRPC 方法交给哪个模块；契约里有、本版不做的路由（汇报线、地点、工作日历、按生效日期的调整，交给 `core.Unsupported`）。
- 不负责：业务规则（去 [internal/AGENTS.md](../internal/AGENTS.md) 找模块）；资源、操作、事件类型的名字（`internal/core/names.go`）；集成测试（`itest/`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `definition.go` | `Version`、`GRPCPort`、迁移、`Keys`、`Own`；`Definition()`：身份、配置、流、资源与操作（`core.Definitions()`） |
| `module.go` | `Module` 与 `New`（`core.Base` 带着运行时、证据写入器、部署、实现实例、默认时区）；`Routes`、`Services`（来源方法与追补，任何组件可调）、`Replays`（`tightening.Category` 的重放 → `replay.Replay`）；`handler` 适配 |
| `services.go` | gRPC 方法 `Snapshot`、`Read`、`CatchUp` 转给 `internal/source` |
| `checks.go` | 整套恢复的核对：法人、部门、岗位按版本答“还在不在” |
| `contract_test.go` | 单元测试：清单的 `configSchema` 与 `events`、OpenAPI 与路由一一对应、事件载荷合乎 schema、合并安全 |
| `migrations/` | 表 |

## 依赖与被依赖

- 依赖：`internal/core`、`entities`、`units`、`positions`、`tightening`、`replay`、`source`。
- 被依赖：`main.go`、`cmd/seed`、外壳 `be/go-infra`、`itest/`。

## 测试

`go test ./org/`。
