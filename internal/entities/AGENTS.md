# entities：法人

法人：代码、名称、国家、时区（业务日期按它算）、本位币、状态。

## 负责与不负责

- 负责：表 `legal_entity`；路由 `/api/org/legal-entities` 的列表、新建（同样的新建再来一次答原来的）、读取、修改、状态、业务日期；事件 `mdm.legal_entity.changed.v1`；没给时区时用部署的默认时区写进记录，都没有就拒绝（协议 14，不隐式默认）；法人暂停、退役是收紧，先写证据（`tightening.EntityStatus`）。
- 不负责：部门（`units`）；给其他组件的读取（`source`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `entities.go` | `entityIn`、`validate`；`CreateEntity`、`GetEntity`、`entity`、`UpdateEntity`、`EntityState`、`updateEntityWith`；`BusinessDate`；`WriteEntity`（`replay` 也用）；列表 `entityList`、`ListEntities` |

## 依赖与被依赖

- 依赖：`core`、`tightening`。
- 被依赖：`replay`（`WriteEntity`）；入口登记路由。

## 规则在哪

`docs/design.md` 的状态与规则；装配项目协议 07（法人的状态表）、协议 14（时间与日历）。

## 测试

`itest/entities_integration_test.go`（`timezones`、`businessDate`、`notInThisVersion`）、`itest/lists_integration_test.go`；证据在 `itest/evidence_integration_test.go`。
