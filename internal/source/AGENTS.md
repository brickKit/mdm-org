# source：系统面

给部署里其他组件的读取（装配项目协议 02 的来源方法）：法人与部门两种记录，带戳；本组件是组织树的水位来源，追补（`CatchUp`）本版不做。

## 负责与不负责

- 负责：`SourceService.Read`（不是身份的也答已清除）、`Snapshot`；`CatchUpService.CatchUp`（答本版不做）；一次最多 `core.MaxBatch` 个。
- 不负责：记录怎样写（`entities`、`units`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `source.go` | `aggregates`、`stamp`、`Snapshot`、`Read`、`CatchUp` |

## 依赖与被依赖

- 依赖：`core`。
- 被依赖：入口 `org/services.go`。

## 规则在哪

装配项目协议 02（来源、戳、S6 的追补）、决定 0304。

## 测试

`itest/source_integration_test.go`（`source`）。
