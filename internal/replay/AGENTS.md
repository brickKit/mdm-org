# replay：整套恢复后的重放

整套恢复第 6 步：按证据重新施加收紧的调整（退役、移出上级、法人状态），不重做业务前置校验（它们在调整时成立），只查对象还在；新上级落在本部门子树里的（会成环）答“对象不在”交复核。事件与审计照常写，执行者是 `ops/recover`。

## 负责与不负责

- 负责：`Replay`（入口的 `Replays` 把 `tightening.Category` 交给它）、`replayUnit`、`replayMove`、`replayEntity`。
- 不负责：证据的写法（`tightening`）；正常的调整（`entities`、`units`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `replay.go` | 上面全部 |

## 依赖与被依赖

- 依赖：`core`、`entities`（`WriteEntity`）、`tightening`、`units`（`WriteUnit`、`Repath`）。
- 被依赖：入口 `org/module.go`。

## 规则在哪

装配项目协议 15；线协议 P12.19–P12.21。

## 测试

`itest/evidence_integration_test.go`（`replay`）。
