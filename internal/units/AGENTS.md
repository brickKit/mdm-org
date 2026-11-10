# units：部门树

部门的树：每个法人下的部门，路径是法人、再从上到下的部门身份（不是代码：改名、改代码不改变授权范围）。

## 负责与不负责

- 负责：表 `org_unit`、`org_unit_path_history`；路由 `/api/org/units` 的列表、新建、读取、改名、移动、退役、上级、路径历史；事件 `mdm.org_unit.changed.v1`（移动时下面的每个部门各发一条）；不成环、不在有启用的下级或启用的岗位时退役；结构变化先锁法人的行（串行）；退役、移出上级是收紧，先写证据。
- 不负责：法人（`entities`）；给其他组件的读取（`source`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `units.go` | `unitIn`、`Structure`（锁法人的行）、`CreateUnit`、`sameParent`、`unit`、`GetUnit`、`changeUnit`、`WriteUnit`、`RenameUnit`、`MoveUnit`、`Repath`（子树的路径与历史，`replay` 也用）、`RetireUnit`、`Ancestors`、`PathHistory`；列表 `unitList`、`ListUnits` |

## 依赖与被依赖

- 依赖：`core`、`positions`（`ActiveIn`）、`tightening`。
- 被依赖：`replay`（`WriteUnit`、`Repath`）；入口登记路由。

## 规则在哪

`docs/design.md` 的状态与规则、“设计取舍”（`AGENTS.md`，仓库根）；安全 07（子树是“前缀加 `/`”）。

## 测试

`itest/units_integration_test.go`（`tree`、`treeRules`）、`itest/lists_integration_test.go`、`itest/outbox_integration_test.go`；证据在 `itest/evidence_integration_test.go`。
