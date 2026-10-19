# mdm/org

## 组件定位

法人与组织，所有客户都需要：法人与部门是授权范围、账簿与单号的基础。
- **法人**：代码、名称与译名、国家、时区、本位币、会计年度起始月。时区必须明确：请求给出，或取部署配置的默认值并写进记录（之后改配置不影响已有法人）；没有就拒绝，不取服务器时区或 UTC。法人的业务日期按它的时区计算（`/business-date`）。生命周期按协议 07：启用、暂停、退役，退役不可恢复。
- **部门**：每个法人下一棵树。路径是“法人身份/上级部门……/本部门”，按段比较，就是授权的组织树维度（安全 07）。改名不改路径；移动时本部门与下面全部部门的路径在同一个事务里更新，每个都有新版本、事件与路径历史。树无环；下面还有启用部门的部门不能退役。
- **查询**：部门的上级（自上而下）、路径历史、按法人或上级列出。

会计期间与关账不在这里（D05，归财务）；员工与任职在 `mdm/employee`。

实验第 6 步的范围：岗位、汇报关系、地点、工作日历、按生效日期的组织变更、水位来源的补取（协议 02 的 S6）都在契约里，调用答“能力不支持”（`CAPABILITY_UNSUPPORTED`，D75）。

## 部署前准备

- 数据库：自己的 schema、运行角色与属主角色（装配项目注册表的 `mdm_org`）。法人与组织不进迁移：安装或上线时经本组件建立（协议 16）。
- 消息系统（`BUS_URL`）与实现实例（`IMPLEMENTATION_INSTANCE`）：每次变化以完整状态事件发布到流 `mdm.org`（4 个分区）。
- 部署的服务证书（`SERVICE_TLS_*`）；身份与授权（`IAM_*`、`AUTHZ_GRPC_URL`、`DEPLOYMENT_ID`）；审计（`AUDIT_GRPC_URL`）。
- 可选：`DEFAULT_LEGAL_ENTITY_TIMEZONE`。

## 依赖说明

- 必需：数据库、消息系统、身份成员、授权成员。
- 可选：审计组件。
- 不调用任何组件。

## 配置指南

全部键见 `component.yaml` 的 `configSchema`（由 SDK 生成，测试核对）。

| 键 | 说明 |
| --- | --- |
| `DEFAULT_LEGAL_ENTITY_TIMEZONE` | 新建法人没有给时区时用的 IANA 时区，写进法人记录；不设时，没有时区的新建被拒绝。不是 IANA 时区时组件不启动 |

## 契约索引

- **系统面**（本仓库 `contract/proto`，模块 `github.com/brickKit/mdm-org/contract/v3`，标签 `contract/v3.…`）：
  - `be.mdm.org.v3.SourceService/Snapshot`、`Read`：协议 02 的来源方法，聚合是法人与部门；回答带部署、实现实例、纪元与各分区位置高点；一次最多 500 个。
  - `be.mdm.org.v3.CatchUpService/CatchUp`：水位来源的补取，本版答“能力不支持”。
- **用户面**：`contract/openapi.yaml`。操作 `mdm.legal_entity.read`、`mdm.legal_entity.manage`、`mdm.org_unit.read`、`mdm.org_unit.manage`。
- **事件**（流 `mdm.org`，schema 在 `contract/events/`）：`mdm.legal_entity.changed.v1`、`mdm.org_unit.changed.v1`，都是完整状态。

## 外壳声明

不是外壳。可以被 Go 的基础外壳（`be/go-infra`）承载。
