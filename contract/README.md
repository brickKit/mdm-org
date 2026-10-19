# mdm-org 的契约

`mdm/org` 发布的契约包：系统面的 protobuf、事件载荷的 JSON Schema、用户面的 OpenAPI。被调方发布契约，放在组件仓库的 `contract/` 子目录里单独发布（与审计组件相同），不另建仓库。

依据：装配项目 [be-assembly-standard](https://github.com/brickKit/be-assembly-standard) 的协议 02（来源要提供什么：S1–S3、S6）、协议 07、协议 14、安全 07、边界卡 `mdm/org`。

## 内容

| 位置 | 是什么 |
| --- | --- |
| `proto/be/mdm/org/v3/org.proto` | `SourceService.Snapshot`、`SourceService.Read`（来源方法）；`CatchUpService.CatchUp`（水位来源的补取，本版不支持） |
| `gen/go/be/mdm/org/v3/` | Go 代码，包 `orgv3`；模块 `github.com/brickKit/mdm-org/contract/v3` |
| `python/` | Python 包 `contract-mdm-org`，模块 `be.mdm.org.v3` |
| `events/` | `mdm.legal_entity.changed.v1`、`mdm.org_unit.changed.v1` 的载荷，同时是来源方法里 `data` 的格式 |
| `openapi.yaml` | 用户面；每个路由的操作写在 `x-operation` |

## 来源的义务（协议 02 的 S1–S3）

与 `mdm-currency` 的契约相同：每次变化版本加一，没有变化不产生新版本；同一事务写完整状态事件，流 `mdm.org`，4 个分区；按身份读取一个身份一个结果、顺序相同，没有的答“已清理”，一次最多 500 个；快照按身份升序分页；每次回答带部署、实现实例、纪元与各分区位置高点，取自同一个读快照。本组件不删除记录，退役的仍可读取。

## 部门路径

`path` 是法人身份、自上而下的部门身份，用 `/` 连接；按段比较（安全 07 的组织树），子树是“前缀加 `/`”。移动一个部门时，它与下面每个部门各发一条事件。
