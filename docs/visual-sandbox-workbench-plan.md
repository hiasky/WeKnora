# WeKnora 可视化沙箱工作台实施规划

> 用户操作说明见：[可视化沙箱工作台操作指南](visual-sandbox-workbench-user-guide.md)。

## 1. 目标与现状

目标是在现有会话级沙箱之上提供用户可见、可操作的工作台，并保持现有的租户/会话隔离、路径约束和资源限额不变。

仓库当前已经具备：

- Docker、CubeSandbox、E2B 三类会话持久沙箱，以及统一的 `SessionShellExecutor` / `SessionFileStore` 能力接口；
- `/workspace/input`（附件）与 `/workspace/output`（产物）目录约定、会话绑定和租户配置固定；
- Agent 产物收集、鉴权下载抽屉，以及 PPTX、HTML、CSV/XLSX 的内嵌预览；
- HTML 产物通过不含 `allow-same-origin` 的 `sandbox="allow-scripts"` iframe 运行，不能继承主站源、Cookie 或登录状态；
- Docker CPU、内存、进程数与命令时长限制。

本课题采用增量实现：补齐面向用户的终端与文件操作 API/界面，显式化产物类型，并新增演示文稿 Skill；不复制已有沙箱生命周期和安全逻辑。

## 2. 设计原则

1. **能力接口优先**：应用层只依赖统一会话能力，不按 Docker/Cube/E2B 分支。
2. **服务端强校验**：所有会话先做租户与用户归属校验；文件操作只允许 `/workspace/output`，前端校验仅用于体验。
3. **最小权限**：终端使用非 root 沙箱账户，工作目录限制在 `/workspace`；文件管理器不暴露技能镜像目录和系统目录。
4. **可中断、可审计**：每条命令有独立 ID、生命周期和取消函数；开始与结束状态写入审计日志，不记录环境变量等秘密。
5. **渐进增强**：无已创建沙箱时返回清晰状态；后端不提供能力时返回 `unsupported`，不静默降级到宿主机执行。

## 3. 分阶段实施

### 阶段 A：统一工作台服务与安全边界

- 扩展会话文件能力，提供列目录、读取、写入、重命名、删除等 provider-neutral 操作。
- 新增 `SessionWorkbenchService`，集中处理：
  - 会话归属校验；
  - 产物路径规范化与目录逃逸拒绝；
  - 命令创建、运行、输出事件、超时与取消；
  - 文件上传、下载、重命名与删除。
- 为终端命令新增审计动作 `sandbox.command_executed`，元数据包含命令 ID、会话 ID、工作目录、退出码、耗时、是否超时/取消。

### 阶段 B：HTTP/SSE 接口

- `POST /api/v1/sessions/:id/workbench/commands`：启动命令。
- `GET /api/v1/sessions/:id/workbench/commands/:command_id/events`：SSE 输出与终态。
- `DELETE /api/v1/sessions/:id/workbench/commands/:command_id`：中断命令。
- `GET /api/v1/sessions/:id/workbench/files?path=...`：浏览产物目录。
- `POST /api/v1/sessions/:id/workbench/files/upload`：上传文件。
- `GET /api/v1/sessions/:id/workbench/files/download?path=...`：下载文件。
- `PATCH /api/v1/sessions/:id/workbench/files`：重命名。
- `DELETE /api/v1/sessions/:id/workbench/files?path=...`：删除。

所有接口复用 `/sessions` 的 Viewer、API Key chat capability 和会话 owner 校验。

### 阶段 C：前端工作台

- 在聊天页面增加“沙箱工作台”抽屉，包含终端与文件两个页签。
- 终端支持命令输入、实时追加输出、运行状态与中断；输出区域使用等宽字体、保留空白并自动换行，使用 `ResizeObserver` 适配抽屉缩放。
- 文件管理器支持目录导航、上传、下载、重命名、删除和刷新。
- 产物列表与文件管理器共用类型推断和预览入口。

### 阶段 D：产物类型与演示文稿 Skill

- 在产物元数据中增加稳定类型标记：`presentation`、`web`、`spreadsheet`、`document`、`image`、`other`。
- 前端优先按类型标记选 renderer，并保留扩展名推断作为兼容旧消息的 fallback。
- 新增预置 `presentation-generator` Skill，要求输出 PPTX 到 `WEKNORA_SKILL_OUTPUT_DIR`，提供模板脚本和依赖说明，打通 Agent 生成、收集、预览与下载。

### 阶段 E：测试与文档

- 单元测试：路径穿越、非产物目录拒绝、会话/租户隔离、取消与超时、审计字段、产物类型推断。
- 后端能力测试：Docker 与 provider-neutral fake remote；保留 Cube/E2B 集成测试入口。
- 前端测试：终端事件归并、窗口变化、类型到 renderer 映射、iframe sandbox 属性。
- 更新 Agent 与沙箱文档，列出接口、安全边界、后端支持矩阵和人工验收步骤。

## 4. 验收映射

| 验收项 | 实现/验证方式 |
| --- | --- |
| 至少两种后端可用 | 统一能力接口；Docker 集成测试 + remote fake/provider 测试 |
| 两租户互相不可见 | handler/service 双层按 tenant、owner、session 校验；隔离测试 |
| 产物目录外路径拒绝 | 服务端 canonical path 校验，仅接受 `/workspace/output` 下相对路径 |
| 三类产物直接查看 | PPTX 分页组件、HTML unique-origin sandbox iframe、CSV/XLSX 表格 renderer |
| iframe 无主站数据 | 仅 `sandbox="allow-scripts"`，不启用 `allow-same-origin`、表单、弹窗或顶层导航 |
| 超限自动终止 | 复用容器/虚拟机资源限额和每命令 context deadline，取消后终态可查询 |
| 命令可审计 | 每条命令结束写 `sandbox.command_executed` 审计记录 |

## 5. 完成标准

- 新接口、服务、前端入口和 Skill 均有测试；Go 测试、前端类型检查及相关前端测试通过。
- 文档清楚说明本地进程不承载不可信交互式命令；实际工作台支持当前会话持久后端 Docker、CubeSandbox、E2B。
- 不把 sandbox ID、存储 URL、API Key、环境变量或跨租户文件路径返回给浏览器。
