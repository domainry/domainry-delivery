# Domainry Delivery 架构重构 TODO

状态：重构进行中。Delivery 实现仓库已完成领域、应用、持久化、组装和传输边界的物理拆分；跨仓 source/artifact owner 切换与 dev 实际发布仍须按本清单验收。`[x]` 表示已有代码和测试证据，`[ ]` 表示仍有真实缺口或外部发布前置条件。

本清单是后续重构的唯一执行顺序。只有代码、架构门禁、定向测试和跨仓消费者同时完成，任务才能勾选；不保留旧命令、旧数据库、旧 HTTP 合同或双运行分支。

## 0. 规范适用边界

- Plane 的 `docs/architecture/backend-development-guide.md` 描述的是“产品项目后端”，其中 `backend/model.json` 与项目 Go 行为代码是 Runtime 的两个直接输入。
- `domainry-delivery` 是 Product、ProductRevision、Feature、FeatureRevision 和 DeliveryRun 的 Owner Module，不是一个产品项目。因此本仓库不新增 `backend/model.json`，也不照搬 `backend/` 项目目录。
- Delivery 应把该指南变成产品交付的结构化验收合同：它保存可信验证结果和精确引用，但不解析项目源码、不执行 Runtime，也不复制 Runtime 的模型校验规则。
- Delivery 自身的模块目录、SDK、数据库与组装方式遵守 Plane 的 `module-dependency-tree.md`：SDK 独立，Module 借用宿主 Database/Dialect/MigrationRegistrar，SaaS 使用自己的数据库，两种模式执行同一份 Delivery schema 并创建私有 Store。
- 跨边界写入遵守 Plane 的 Mutation Kernel 与 consistency contract：本地状态、命令 receipt，以及确实需要的 Audit/Outbox 必须在一个事务内提交；外部动作只能发生在提交之后。
- 项目尚未上线。重构直接替换当前设计，开发数据库清空重建，不写迁移兼容、旧协议适配、fallback planner 或双执行路径。
- 不使用 Builder skill，不创建 `go.work`，不新建 worktree 或开发分支。

## 1. 当前根因

下列文字记录重构前的根因；勾选表示该根因已从当前代码中消除，不勾选表示还有跨仓缺口。

- [x] **A01：只有一套 DeliveryRun 生命周期。** 当前 `engine.go`、`projection.go` 和 `lifecycle_bridge.go` 通过 `len(DeliveryUnits) > 0` 在 WorkItem/Build/TestRun/AcceptanceResult 与 DeliveryUnit/QualityRun/AcceptanceConfirmation 之间分流。根因是新模型以增量分支接入，旧模型没有同时删除，导致同一个业务事实有两个状态机、两套命令和两套发布门禁。
- [x] **A02：安装必须产生新的不可变 ProductRevision。** 当前 `installV3DeliveryRun` 只把 Feature 标为 installed 并更新部署信息，没有追加 ProductRevision，也没有推进 `current_definition_revision` 和 `current_release_revision`。这会让“功能已上线”和“可执行产品定义”永久分叉。
- [x] **A03：后端指南的验证项必须是结构化事实。** 当前 DeliveryUnit 阶段只接收 Git revision、summary 和自由格式 diagnostics；这些字段不能证明唯一 `backend/model.json`、Go 行为 registry、ProjectHTTP、typed Handler、权限、事务、幂等、重启和真实旅程确实通过。
- [x] **A04：SDK 必须独立于实现仓库。** 当前根目录 `sdk.go` import `internal/domain/delivery` 并用 type alias 暴露内部模型；调用方实际依赖的是实现仓库，SDK 无法独立发布，也无法阻止内部字段泄漏。
- [x] **A05：Module/SaaS 必须共享行为而隔离组装。** 当前 `module.Host` 只有 `Database() *sql.DB`，Module 自己假设 SQLite 并执行裸 DDL；它没有 RuntimeID、Dialect、MigrationRegistrar、Factory/ApplicationRef，也没有私有 module/saas assembly。
- [x] **A06：所有写入必须走同一命令内核。** Product 与 DeliveryRun 命令有 receipt，但附件增删没有 `client_id`；StartDelivery 的 fingerprint 不含 URL 中的 `delivery_run_id`，重放时还读取当前 Run，而不是返回第一次提交的原始结果。根因是幂等、并发与响应 receipt 被分散到各 Repository 方法实现。
- [ ] **A07：Agent 与 Delivery 的数据所有权必须一致。** Delivery 已删除附件 BLOB 和内容路由，Agent verifier 也已验证 canonical source 属于被授权的 Conversation/Run，Delivery domain 负责 decision/source 引用闭包；但 Deck 仍调用旧附件路由，并且本地会话尚未向 Agent 发布 durable provenance。
- [x] **A08：Feature 基线不能被静默改写。** FeatureRevision 已冻结 `baseline_product_revision`，但 `NewDeliveryRun` 把当前 release revision 写入 Run 引用，没有拒绝过期 Feature；授权角色还会从其他草稿和未安装 Feature 中汇总，未发布需求因此会污染当前基线。
- [x] **A09：代码与持久化边界需要拆分。** `engine.go`、`product_engine.go`、`store.go` 都接近或超过 900 行；完整 Product/DeliveryRun 以单个 JSON BLOB 重写，命令、权限、actor、投影和文档又分别维护字符串清单，任何新增行为都需要修改多个无编译关联的位置。

## 2. 目标所有权与目录

这次重构不是把整条交付链搬进 `domainry-delivery` 。每个业务事实只能有一个 owner，跨仓调用只传递 SDK 合同和不可变引用：

| 仓库 | 唯一责任 | 明确不拥有 |
| --- | --- | --- |
| `domainry-delivery-sdk` | Delivery 公共 DTO、command/error/receipt、Module/SaaS host 边界和合同测试 | 领域状态机、Store、HTTP 实现 |
| `domainry-delivery` | Product、ProductRevision、Feature、FeatureRevision、DeliveryRun/Unit、QA/Acceptance/Release 状态机与持久化 | Conversation/Run、artifact bytes、产品 Runtime 启动、Jenkins/Kubernetes 配置 |
| `domainry-agent-sdk` | Conversation/Run/source/artifact 的窄验证合同 | Delivery decision DTO、领域状态机 |
| `domainry-agent` | Conversation、Run、source/artifact 归属、访问控制与可读性 | Product/Feature/DeliveryRun 状态和 Feature decision identity |
| `domainry-deck` | 本地 PM/RD/QA/OP Agent Runtime roles、项目源码/缓存/outbox/thread state、Runtime/Mock/journey 真实检查与 evidence 生成 | Delivery 状态转换、Delivery 持久化、伪造系统验证结果 |
| `devops` | Jenkins job、镜像发布、Argo CD/Kubernetes dev 部署与 MySQL/Identity/Agent secret 引用 | Delivery 业务逻辑和密钥明文 |

关键调用方向：`Deck -> Agent (source/artifact owner)`，`Deck -> Delivery (command/evidence reference)`，`Delivery -> Agent verifier (confirm lineage/access)`；`devops -> Delivery SaaS` 只负责组装和发布。Delivery 不能因为要验收一个事实，就反向拥有产生该事实的能力。

### 独立 `domainry-delivery-sdk`

```text
domainry-delivery-sdk/
├── contract/       # ApplicationRef、DTO、Command、Error、Binding、Descriptor
├── modulehost/     # Host 与 Module Factory，只暴露宿主原语
├── saashost/       # SaaS Factory/Binding 边界
├── remote/         # 有界 HTTP Remote Factory
└── contracttest/   # Module/SaaS 共同行为合同
```

### `domainry-delivery` 实现仓库

```text
domainry-delivery/
├── cmd/domainry-delivery/                         # 独立 SaaS 组合根
├── module/module.go                               # 单文件薄 facade
└── internal/
    ├── domain/                                    # 共享值对象与稳定错误
    │   ├── command/                               # typed command catalog
    │   ├── product/                               # Product/Revision/Feature 聚合
    │   ├── deliveryrun/                           # DeliveryRun/Unit 状态机
    │   └── lifecycle/                             # 跨聚合纯领域转换
    ├── application/                               # 用例、窄端口、命令协调
    ├── assembly/module/                           # 借宿主原语组装
    ├── assembly/saas/                             # 自有 DB、Identity、HTTP 组装
    ├── infrastructure/persistence/
    │   ├── database/                              # 共用关系型 Repository
    │   ├── mysql/                                 # SaaS MySQL 连接入口
    │   └── sqlite/                                # 本地/合同测试连接入口
    ├── presentation/                              # 稳定错误的 locale/message 映射
    ├── transport/http/
    └── architecture/                              # 目录、依赖和写入口门禁
```

约束：禁止重新创建 `internal/domain/delivery`、`internal/application/delivery` 一类总包；`cmd` 和最外层产品组合根选择具体实现；domain/application 不 import `database/sql`、`net/http` 或其他模块实现；Store 永远不通过 SDK/Host 注入。

## 3. 执行顺序

### R00：冻结目标合同与删除清单

- [x] R00.1 建立一份 typed command catalog，逐项定义 command key、目标 aggregate、actor kind、permission、payload、成功 receipt 和合法前置状态。
- [x] R00.2 将 Product、Feature、DeliveryRun、DeliveryUnit、ProductRevision、Quality、Acceptance、Release 的唯一状态图写入 domain 测试，不再用文档或 Deck 分支补充业务规则。
- [x] R00.3 明确删除清单：WorkItem、Build、TestRun、AcceptanceResult 及其命令、投影、文档、Deck 调用点全部移除；可复用的 ProductRevision candidate 语义并入唯一 DeliveryUnit 流程。
- [x] R00.4 冻结新 HTTP/Binding 请求与响应 DTO。所有写请求统一包含 `client_id`、`expected_revision` 和业务 payload；资源路径中的身份也纳入命令 fingerprint。
- [ ] R00.5 定义一次性切换顺序：Delivery SDK → Delivery 实现 → Deck。不得为了分批发布而保留旧合同兼容层。

完成标准：能从 catalog 生成/校验所有 `available_actions` 元数据；仓库中不再存在第二份手写 command/actor/permission 清单。

### R01：收口唯一领域状态机

- [x] R01.1 删除 `len(DeliveryUnits) > 0` 的所有生命周期分支，`Apply`、projection、release gate 和 installation 只处理 DeliveryUnit 流程。
- [x] R01.2 删除 legacy work/build/test/acceptance 字段、handler、fixture 和测试；不保留旧 JSON decode 结构。
- [x] R01.3 将 QA failure、business acceptance failure 和 system verification gap 统一路由回明确的 DeliveryUnit phase；修复后必须产生新的 integrated Git revision，旧证据自动失效。
- [x] R01.4 Feature 确认与 Delivery 启动都校验冻结基线；要求 `FeatureRevision.BaselineProductRevision == Product.CurrentReleaseRevision == DeliveryRun.Product.ProductRevision`，过期需求必须回到 discovery 后重新确认，不能静默换基线。
- [x] R01.5 Feature authorization 只允许引用当前已安装 ProductRevision 的角色和本 Feature 显式新增角色；删除从其他 draft、confirmed 但未安装 Feature 汇总角色的逻辑。
- [x] R01.6 把 actor/permission 判定收口到 typed command catalog；domain 再验证状态转换，HTTP/SDK/Deck 不直接写状态。

完成标准：仓库搜索不到 legacy 类型与分流条件；一个命令只能命中一个 handler；任意 stale baseline 都在创建 DeliveryRun 前失败。

### R02：把 Plane 后端指南变成可信交付证据

- [x] R02.1 用结构化 evidence 替代通用 `deliveryUnitCommandPayload`。每份证据至少绑定 workspace、product、feature revision、repository identity、clean Git revision、check suite/version、执行主体、发生时间和不可变 evidence refs。
- [x] R02.2 增加 Project Model evidence：证明产品仓库只有一个 `backend/model.json`，严格 decode/validate 通过，并记录 normalized model hash。
- [x] R02.3 增加 model/registry cross-reference evidence：证明 Go typed registries 与 model 中对象、字段、角色、权限引用一致；JSON 中没有镜像可执行行为。
- [x] R02.4 增加 backend behavior evidence：覆盖 ProjectHTTP route/DTO、typed Handler unit-of-work/idempotency、`runtimeext.ProjectDefinitions`、deterministic bootstrap，以及禁止项目自写 schema/CRUD SQL。
- [x] R02.5 增加 topology/layout evidence：产品项目只有 `backend/go.mod`，没有仓库根 `go.mod`、任何 `go.work`、compiler、Builder CLI、中间模型或 generated Runtime contract。
- [x] R02.6 增加 Runtime verification evidence：覆盖认证、Workspace、permission、data scope、audit、persistence、空库初始化、same-model restart 与 changed-model rejection。
- [x] R02.7 增加 journey evidence：同一前端主旅程必须分别对 Mock Backend 与真实 Runtime 通过，并绑定同一个 integrated Git revision。
- [x] R02.8 只有具备 `delivery_deployment.record` 的可信 system principal 能提交验证 evidence；Agent 只能提交实现完成声明，不能自证系统门禁通过。
- [x] R02.9 Domain 只验证 evidence 合同、引用闭包和阶段顺序；真实检查由 Deck/Runtime adapter 执行，Delivery 不复制 Runtime 校验器。

完成标准：任何一个指南验证项缺失、属于其他 Git revision、来源不可信或 model hash 不一致，release gate 都不能通过。

### R03：闭合 ProductRevision 与安装事务

- [x] R03.1 在唯一 DeliveryUnit 流程加入“记录 executable ProductRevision candidate”命令，由 RD 提交完整 `ProductStory + ProductDefinition + Decisions`、base/target revision、model hash、Git revision 和 evidence refs。
- [x] R03.2 复用并收紧当前 ProductDefinition 引用校验；Narrative 仍然不是可执行定义，Story 与 Definition 必须在同一个 candidate 中提交。
- [x] R03.3 candidate 的 base revision 必须等于 DeliveryRun 冻结基线，target revision 必须为 base+1；candidate、Runtime verification、QA、acceptance 和 Release 必须绑定同一个 Git revision。
- [x] R03.4 V3 Release 恢复 `product_revision` 与 `product_revision_ref` 的强绑定，不能只记录 code revision。
- [x] R03.5 `InstallDeliveryRun` 在一个数据库事务内同时追加 immutable ProductRevision、推进 current definition/release revision、标记 Feature installed、写入 deployment receipt 并完成 Run。
- [x] R03.6 删除当前测试中“V3 安装不创建 ProductRevision”的错误期望，增加事务回滚、重复 receipt 精确重放和基线冲突测试。

完成标准：不存在 installed Feature 没有对应 ProductRevision 的状态；当前 release revision 永远指向已安装的精确 revision。

### R04：修正 Agent source 与 artifact 所有权

- [x] R04.1 删除 Delivery 附件 BLOB 表、上传/下载内容接口和 `FeatureAttachmentContent`；Delivery 只保留不可变 artifact/source reference、hash、media metadata 与来源身份。
- [x] R04.2 先检查当前 `domainry-agent-sdk` 是否已有满足 Conversation/Run/source/artifact 验证的窄 contract；能复用就直接依赖 SDK，不能复用才先在 Agent SDK 增加 source-verifier contract。Feature decision identity 明确归 Delivery，不进入 Agent contract。
- [x] R04.3 application 层通过 Agent owner contract 验证 canonical source 存在、属于当前 Workspace、调用者可读，并与 Conversation、Run、BeforeStep 一致；Delivery domain 再验证当前 source 声明的 decision IDs 精确匹配、全部 evidence/role/decision source 都闭合到已验证来源。两边均不代替对方拥有业务事实。
- [x] R04.4 FeatureRevision 继续冻结形成需求的全部 Conversation、Run、BeforeStep、source 和 decision 引用；确认后任何引用不得被替换。
- [ ] R04.5 Deck 不再向 Delivery 上传本地文件内容，也不上传本地路径；它先把 artifact 交给 Agent owner，再向 Delivery 提交 canonical reference。

完成标准：Delivery 数据库没有 artifact bytes；伪造、跨 Workspace、不可读或与 Run 不一致的引用不能进入 FeatureRevision。

### R05：统一 Mutation、幂等与跨边界一致性

- [x] R05.1 建立一个 owner-neutral Delivery command executor：authorize → immutable context → plan → domain transition → atomic commit → receipt → post-commit dispatch。
- [x] R05.2 所有 Product、Feature、DeliveryRun、evidence、acceptance、release 写入统一经过 executor；删除 Repository 中各自实现的幂等分支。
- [x] R05.3 用 Foundation `_operations` 记录 Delivery command identity、fingerprint、状态与原始 terminal receipt；删除 `delivery_command_receipts` 和 `delivery_product_command_receipts`，不再新增模块私有 receipt 表。
- [x] R05.4 fingerprint 覆盖 owner、command kind、workspace/resource identity、actor snapshot 和 canonical payload；同一 `client_id` 改变任一事实都返回冲突。
- [x] R05.5 精确重放返回第一次提交的完整 receipt，不读取当前 aggregate 拼装响应；StartDelivery receipt 同时冻结 Product 与新 Run 的结果。
- [x] R05.6 乐观并发只在 service/application 事务中处理；HTTP client、remote binding 和 Deck 只提交 `expected_revision`，绝不直接更新 status。
- [x] R05.7 如果 Delivery 将来发布跨模块事件，aggregate、Audit、Outbox 和 operation receipt 必须同事务提交；当前没有外发事件时不预建通用 intent/outbox 表。
- [x] R05.8 部署 provider 调用继续由部署 adapter 所有；Delivery 只接收绑定原始 attempt identity 的 success/failure/unknown receipt，并保留 reconciliation，不跨网络持有事务。

完成标准：Mutation 入口 inventory 覆盖全部写能力；未登记入口、直接 Store 写状态、缺失 client_id/expected_revision 或 changed fingerprint 都被架构测试拒绝。

### R06：拆出独立 SDK 并收口 Module/SaaS 组装

- [x] R06.1 创建独立 `domainry-delivery-sdk`；公共 DTO 是 SDK 自有类型，不 alias `domainry-delivery/internal`，SDK 不依赖实现仓库。
- [x] R06.2 SDK 提供 `contract.ApplicationRef`、Descriptor、Binding、typed requests/results/errors 和 command constants；为所有输入提供严格 Validate。
- [x] R06.3 SDK `modulehost.Host` 只暴露 RuntimeID、Database、Dialect、MigrationRegistrar；`Factory.OpenModule` 校验 ApplicationRef/Host audience 一致。
- [x] R06.4 SDK `saashost` 和 `remote` 提供独立 SaaS 协议；Remote 有请求/响应大小上限、严格 descriptor/audience、认证和稳定错误映射。
- [x] R06.5 Delivery 实现仓库删除根 `sdk.go` 与 `remote/`；改为依赖已发布 Delivery SDK。
- [x] R06.6 `module/module.go` 变成单文件薄 facade，只把 `NewFactory` 转给 `internal/assembly/module`，不包含业务、Store 或 DTO 映射实现。
- [x] R06.7 `internal/assembly/module` 使用 host Database/Dialect/MigrationRegistrar 执行 Delivery 自有 schema 并创建私有 Store；不创建第二个连接，不推断 driver，不接收外部 Store。
- [x] R06.8 `internal/assembly/saas` 打开服务自有数据库，执行与 Module 同源的 schema，组装 Identity、application 和 HTTP；`cmd` 只读取配置并启动/关闭服务。
- [x] R06.9 Module 与 SaaS 运行同一个 application/domain 实现，通过 SDK contract tests 验证 descriptor、身份、命令 receipt、错误和状态机完全一致。

完成标准：生产依赖方向是 consumer → delivery-sdk ← delivery implementation；Module 与 SaaS 只有组装和物理数据库归属不同。

### R07：重做持久化与代码分层

- [x] R07.1 使用 `domainry-orm` dialect 与 migration descriptors 定义 Delivery owned schema；删除 `sqlite.OpenBorrowed` 内的裸 SQLite DDL。
- [x] R07.2 Module/SaaS 共用同一 schema source 与私有 Store，表归属和 migration owner 唯一；standalone SaaS 通过 `_schema_migrations(owner, version)` 分别记录 Delivery 与 shared Operations 的不可变 checksum，重启不重复执行 DDL；开发期直接清空旧数据库，不编写旧表迁移。
- [x] R07.3 按 aggregate 边界持久化 Product、ProductRevision、Feature、FeatureRevision、DeliveryRun、DeliveryUnit、quality、acceptance、release；只对不可变文档 payload 使用有界 JSON，不再每次重写整个 Product/Run 历史 BLOB。
- [x] R07.4 为 Workspace、product code、Feature/Run identity、revision、delivery queue 和当前状态建立真实约束与索引；数据库约束与 domain invariant 保持同一语义。
- [x] R07.5 application 拆成按 Product、Feature、DeliveryRun 用例组织的窄 ports；Store 接口不再是一个包含所有读写的巨型 Repository。
- [x] R07.6 将 `engine.go`、`product_engine.go`、`store.go` 按状态机/用例/表所有权拆分；单文件目标不超过约 500 行，测试跟随所属 invariant。
- [ ] R07.7 domain 只返回稳定 error code 与结构化 detail；locale/message 映射移到 contract/transport presentation，避免领域规则承担 UI 文案。

完成标准：domain/application 无具体 I/O；SQLite 只是 SaaS 可选 dialect 之一；高频命令不会重写无关 revision、evidence 和历史集合。

### R08：HTTP、Deck 与跨仓一次性切换

- [x] R08.1 HTTP handler 只负责认证上下文、严格 DTO decode、调用 SDK-shaped application service 和错误/status 映射；删除业务 command 分支与状态判断。
- [x] R08.2 删除附件内容路由和所有 legacy lifecycle 路由；更新 `docs/api.md` 与 `docs/agent-integration.md` 为唯一新合同。
- [ ] R08.3 Deck Rust client 改为新 DTO/receipt；所有 Delivery 命令继续由 Rust 发送，TypeScript 不持有凭据、不构造 raw command、不校验状态机。
- [x] R08.4 Deck 的 PM/RD/QA/OP 仍是本地 Agent Runtime roles；提示词只能提出命令，提交前必须重新读取 server-projected `available_actions` 和 revision。
- [x] R08.5 Deck verification adapter 产出 R02 的 typed evidence；禁止 Agent 用自由文本声明 model/runtime/journey 已通过。
- [x] R08.6 Deck deployment adapter 保持 canonical remote repository identity、clean Git revision 和 provider receipt；永不上传本地路径或另造平行 artifact/evidence identity。
- [ ] R08.7 同步删除 Deck 对 legacy WorkItem/Build/TestRun/AcceptanceResult 和旧附件 API 的 presentation、scheduler、fixture 与测试。

完成标准：同一个 Deck journey 可在 Module contract fixture 与 SaaS HTTP contract 上运行；客户端没有复制领域转换条件。

### R09：架构门禁与最终验收

- [x] R09.1 新增 `internal/architecture/layout_test.go`，检查目标目录、薄 module facade、标准 Go 文件名与私有实现位置。
- [x] R09.2 新增 import gate：domain/application 禁止具体 I/O和外部实现；SDK 禁止实现依赖；非 `cmd`/assembly 禁止选择 Identity/Agent 等具体实现；禁止跨模块 internal import。
- [x] R09.3 新增 mutation entrypoint inventory，列出所有强一致写入口、owner、permission、receipt、transaction 和外部 effect；测试拒绝未知入口与 repository bypass。
- [x] R09.4 新增 schema ownership 测试，证明 Module 与 SaaS 执行同源 Delivery migrations，宿主/Runtime 不复制 Delivery DDL，SDK 不接受 Store。
- [x] R09.5 新增完整领域验收：ProductStory+Definition 原子 revision、精确 Feature lineage、stale baseline、typed backend evidence、QA/acceptance、release、deployment unknown/reconcile、atomic install、idempotent replay 和 optimistic conflict。
- [x] R09.6 新增 Plane 后端指南验收 fixture：strict model、registry cross-reference、ProjectHTTP/Handler、权限/审计/持久化、same-model restart、changed-model rejection、Mock/Runtime 同旅程。
- [ ] R09.7 更新 README，删除“已实现”但代码尚未满足的声明；README、SDK descriptor、HTTP 文档、Deck DTO 与 command catalog 由测试校验一致。
- [ ] R09.8 最终执行 Delivery SDK、Delivery、Deck 的定向测试与静态检查；重新审计生产 import graph，确认无环、无实现反向依赖、无 legacy symbol。

完成标准：所有 R00-R09 项均有代码证据；`go test ./...`、`go vet ./...`、Delivery SDK contract tests 和 Deck 定向测试通过后，才把本计划标记完成。

### R10：Jenkins 与 MySQL dev 部署

- [x] R10.1 Delivery 提供多阶段 `Dockerfile` 和 `.dockerignore`，镜像运行非 root 静态 Linux 二进制，本地已通过 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build`。
- [x] R10.2 在独立 `devops` 仓库增加 `jenkins-configs/domainry-delivery-dev.yaml` 和 `domainry-delivery/k8s/dev/Jenkinsfile`，复用现有 shared library、ECR 和 Argo CD 流程。
- [x] R10.3 dev Deployment 显式使用 `DELIVERY_DB_DRIVER=mysql` 和 secret-backed `DELIVERY_MYSQL_DSN`，Identity/Agent 也只引用 Kubernetes Secret，仓库不保存明文密钥；本机 MySQL 9.5 已完成空库启动、HTTP 创建 Product、进程停止、同库重启及 ProductRevision 读取验证（201 → 200，migration ledger 两个 owner 均为 clean）。
- [x] R10.4 Kubernetes ServiceAccount、Deployment、Service、Ingress、Kustomization 和 Argo CD Application 已配置；YAML 可解析且 `kubectl kustomize` 渲染通过。
- [ ] R10.5 在 `verdent-dev` 创建 `domainry-delivery-database`、`domainry-delivery-identity` 和 `domainry-delivery-agent` 三个 Secret；本机当前没有这些凭据，不伪造值。
- [ ] R10.6 在 Jenkins 创建/更新 dev job 并运行一次成功 pipeline；当前 Jenkins/AWS/Argo/Kubernetes 认证在本任务中不可用。
- [ ] R10.7 Argo CD 同步后从集群外执行 `/healthz`、descriptor、Identity 认证、MySQL 写入/重启持久化和 Agent source-verifier 写入旅程。

完成标准：不是“YAML 已写”，而是 Jenkins 成功推送唯一 image digest、Argo CD 健康同步、Pod 使用 MySQL 启动，且真实写入与重启旅程通过。

## 4. 当前不得假装完成的缺口

1. **Deck 本地 provenance 尚未进入 Agent owner。** Agent SDK v0.1.29 和 Agent 已拒绝不属于已授权 Conversation/Run 的 source，Delivery 也已闭合自己的 decision/source 引用；但 Deck 当前生成的 `conversation://<local-thread>/turn/<turn>` 只是本地身份，Agent 并不认识。正确缺口是新增“Deck 本地执行结果 → Agent durable provenance”的发布/outbox 合同，不是在 Delivery 增加绕过校验的 fallback。
2. **Deck 仍依赖已删除的 Delivery 附件 API。** Rust `delivery_client.rs`/`feature_attachment.rs` 仍有 upload/download/remove 调用和 `delivery-attachment://` 引用；它必须改为 Agent-owned artifact/source 合同，因此 R04.5/R08.7 不能勾选。
3. **Deck 仍有 raw payload 弱类型边界。** Delivery SDK v0.1.5 已为每个 command 提供 typed payload、strict `Validate` 和 typed `NewCommand`；Deck 的 Rust client 仍用 `serde_json::Value` 组装命令，因此 R08.3 不能勾选。
4. **domain error 仍携带 English message。** 稳定 code 和 presentation locale 已存在，但领域调用点仍构造 `code + message`，因此 R07.7 不能勾选。
5. **真实 dev 发布缺外部凭据。** manifest 已完成静态验证，但没有 Jenkins/AWS/Argo/Kubernetes 认证和三组 Secret，因此不能声称 dev 已部署。

## 5. 推荐落地批次

1. **批次一：R00 + R01 + R03** — 先消灭双状态机并闭合 ProductRevision。这是当前会制造错误业务状态的根因。
2. **批次二：R02 + R04 + R05** — 再把可信 evidence、source owner 和统一 mutation 内核补齐，避免围绕错误边界继续扩展协议。
3. **批次三：R06 + R07** — 领域合同稳定后拆 SDK、Module/SaaS assembly 与 persistence，减少跨仓反复改版。
4. **批次四：R08 + R09** — 一次性切 Deck 和 HTTP，删除旧合同，补齐架构门禁与最终验收。

在 A07、R04.3、R04.5、R07.7、R08.3、R08.7 闭环前，不继续增加新的 Delivery command、legacy 分支、附件存储或 SaaS 特例。
