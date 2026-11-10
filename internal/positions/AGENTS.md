# positions：岗位

部门里的岗位（3.0.34）：员工的任职挂在法人、部门、岗位上（`mdm/employee`）。岗位只属于一个部门、不挪动；换部门是退役旧岗位、新建岗位。

## 负责与不负责

- 负责：表 `org_position`；路由 `/api/org/positions` 的列表、新建、读取、改名、退役；事件 `mdm.org_position.changed.v1`；代码在法人内唯一、同样的新建答原来的；新建先锁法人的行（与部门退役串行）；退役是收紧，先写证据；`ActiveIn`（部门退役前查还有没有启用的岗位）；`WritePosition`（`replay` 也用）。
- 不负责：部门（`units`）；给其他组件的读取（`source`）；持有人与任职（`mdm/employee`，本组件不读员工）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `positions.go` | `positionIn`、`ActiveIn`、`CreatePosition`、`position`、`GetPosition`、`change`、`WritePosition`、`RenamePosition`、`RetirePosition`；列表 `positionList`、`ListPositions` |

## 依赖与被依赖

- 依赖：`core`、`tightening`（`PositionRetirement`）。
- 被依赖：`units`（`ActiveIn`）、`replay`（`WritePosition`）；入口登记路由。

## 规则在哪

`docs/design.md` 的术语与状态与规则；装配项目安全 04（任职挂在岗位上，有安全含义的变化结束旧任职）、协议 09（按岗位找处理人）、协议 15（收紧的证据）。

## 测试

`itest/positions_integration_test.go`（`positions`）；改坏检查在装配项目 `dev/rebuild/c1-data/org-positions-mutants.txt`。
