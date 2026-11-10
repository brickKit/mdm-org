# tightening：收紧授权的证据

部门退役、移出上级、法人暂停或退役、岗位退役会收紧授权范围：这类调整先在事务外写意图证据，再在事务里提交，在锁下得出“是不是真的收紧了”的结论，没收紧就取消（装配项目协议 15）。

## 负责与不负责

- 负责：证据的类别 `Category`（`org_tightening`）与对象类型；`Tightening`（每种收紧怎样描述）；`Intent`、`Conclude`、`Settle`；`Retirement`、`MoveOut`、`EntityStatus`、`PositionRetirement`；`above`、`entityRank`（判断是不是收紧）。
- 不负责：调整本身（`entities`、`units`）；重放（`replay`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `tightening.go` | 上面全部 |

## 依赖与被依赖

- 依赖：`core`。
- 被依赖：`entities`、`units`、`positions`、`replay`；入口（`Category`）。

## 规则在哪

装配项目协议 15 的证据；线协议第 12 章。

## 测试

`itest/evidence_integration_test.go`（`evidence`：意图、结论、取消、证据存储停着时的本地等待）。
