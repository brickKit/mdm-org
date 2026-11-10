# itest/：集成测试与测试索引

法人与组织组件对着真实的 PostgreSQL 与消息系统、经它真实的接口（用户面带 be-sdk-go `sectest` 的令牌，系统面走 mTLS；`sectest` 含证据存储的替身）的测试（L2）。构建标签 `integration`；一条业务流程一个文件。

## 怎样跑

- `make test-integration`（`go test -count=1 -tags integration ./itest/`），约 20 秒（证据一步等证据存储）。
- 环境变量：`BE_SDK_TEST_PG_ADMIN=postgres://postgres:<口令>@localhost:5432/brickkit_test_db`（口令是装配项目 `.env` 的 `POSTGRES_PASSWORD`）、`BE_SDK_TEST_NATS_URL=nats://localhost:4222`。
- 前提：装配项目里 `make up` 起了 `be-postgres` 与 `be-nats`；`be-postgres` 里有库 `brickkit_test_db`（没有就 `docker exec be-postgres createdb -U postgres brickkit_test_db`）；测试每次开始时删掉再建自己的 schema `mdm_org_it`，跑完留着；端口 28083、9093 要空着，被占时测试直接失败。
- **只有一个测试** `TestOrgEndToEnd`：按下表的顺序跑各步，后面的步用前面建的法人与部门，所以一步不能单独跑；证据、重放、恢复核对三步是子测试（`-run TestOrgEndToEnd/evidence` 仍会先跑前面的步）。

## 文件

按运行顺序：

| 文件 | 步 | 测的流程 | 被测的模块 |
| --- | --- | --- | --- |
| `entities_integration_test.go` | `timezones`、`businessDate` | 没给时区也没有默认就拒绝；部署的默认写进记录；同样的新建答原来的、代码只能用一次；西班牙的法人按马德里时间算业务日期；没有授予看不见 | `entities` |
| `units_integration_test.go` | `tree`、`treeRules` | 部门树与路径；移动后下面的路径跟着变、旧路径可追溯、上级；不成环、有启用的下级不能退役 | `units` |
| `lists_integration_test.go` | `lists` | 部门按法人、按路径、按游标；法人的关键字（拼音首字母） | `units`、`entities` |
| `outbox_integration_test.go` | `outbox` | 移动发了被移动的部门与下面的每个部门 | `units` |
| `positions_integration_test.go` | `positions` | 岗位：在启用的部门里建、代码在法人内唯一、同样的新建答原来的；改名新版本；列表；没有授予看不见；部门里有启用的岗位不能退役；退役先写意图再写结论、整套恢复后重放；事件与来源读取 | `positions`、`units`、`tightening`、`replay`、`source` |
| `entities_integration_test.go` | `notInThisVersion` | 汇报关系等本版不做 | 入口 |
| `source_integration_test.go` | `source` | 系统面（mTLS）：`Read`（存在、已清除、不是身份）、戳、`Snapshot` 分页、追补本版不做 | `source` |
| `evidence_integration_test.go` | `evidence`、`replay` | 收紧的证据：意图、结论、取消、证据存储停着时的本地等待（`evidenceStory`）；整套恢复后的重放（`replayStory`） | `tightening`、`replay` |
| `recoverycheck_integration_test.go` | `recoveryChecks` | 整套恢复的核对：法人按版本答“还在不在”（`checksStory`） | 入口 `org/checks.go` |

共用的：

| 文件 | 内容 |
| --- | --- |
| `endtoend_integration_test.go` | `TestOrgEndToEnd`（各步的顺序）；`fixture`（迁移好的库、世界、两个用户、步与步之间传的法人与部门）；`start`（建世界、迁移）；`run`（运行成员直到停）；`client.do`、`portFree` |

## 加测试

新流程加一个文件与一步，在 `TestOrgEndToEnd` 里按依赖排进顺序，在上表加一行；测试不为了通过而改。
