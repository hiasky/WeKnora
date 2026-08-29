# WeKnora 可视化沙箱工作台操作指南

本文介绍如何配置和使用 WeKnora 的可视化沙箱工作台，包括交互式终端、文件管理、产物预览以及演示文稿生成。

## 1. 使用前需要准备什么

工作台使用的是 Agent 当前会话绑定的沙箱，因此需要满足以下条件：

1. 管理员已经在当前空间配置并启用了至少一个沙箱；
2. 当前 Agent 已选择该沙箱；
3. 当前对话使用“智能推理”或其他启用了沙箱工具的 Agent；
4. Agent 至少在本会话中执行过一次沙箱命令或 Skill，以建立会话与沙箱的绑定。

工作台目前适用于会话持久化的 Docker、CubeSandbox 和 E2B 后端。工作台不会在 WeKnora 宿主机上直接执行用户输入的命令。

## 2. 管理员：配置沙箱

如果空间中已经有可用沙箱，可以跳到第 3 节。

### 2.1 进入沙箱设置

1. 登录 WeKnora；
2. 打开“设置”；
3. 进入“沙箱”或“沙箱后端”；
4. 确认“允许 Skill 脚本在沙箱中运行”已经开启；
5. 点击“添加沙箱”。

### 2.2 选择后端

#### Docker

适合本机开发和快速体验。

通常需要确认：

- WeKnora 服务能够访问 Docker daemon；
- Docker 镜像使用默认的 `wechatopenai/weknora-sandbox:main`，或使用包含 `/workspace/input`、`/workspace/output` 的兼容镜像；
- 容器资源限制已配置，例如 CPU、内存、进程数和空闲回收时间；
- Docker 网络模式符合部署安全要求。

#### CubeSandbox

适合使用腾讯 CubeSandbox 的部署。

通常需要填写：

- Cube API 地址；
- CubeProxy 地址；
- Sandbox Domain；
- API Key（服务启用认证时）；
- 模板 ID、TTL 和 DNS 等配置。

#### E2B

适合 E2B Cloud 或兼容 E2B 协议的自建控制面。

通常需要填写：

- E2B API Key；
- API 地址；
- Sandbox Domain；
- 自建集群的数据面 Proxy URL；
- 模板 ID 和 Sandbox TTL。

### 2.3 检查连接

保存配置前，建议先执行连接检查。

- 普通检查用于验证地址、凭证和模板配置；
- 完整检查会创建一个临时沙箱、执行测试命令，然后销毁沙箱；
- 完整检查可能消耗少量远端沙箱时长。

看到“沙箱内执行”检查成功后，再保存配置。

## 3. 管理员：安装演示文稿 Skill

要让 Agent 直接生成 PPTX，需要安装 `presentation-generator` Skill。

> 当前版本的“技能”页面没有“预置技能”列表。仓库中的 `skills/preloaded/` 是 Skill 源码目录，不会自动显示为可点击安装的技能。页面目前支持上传 ZIP，或者从 GitHub、GitLab、skills.sh 等远程来源安装。

1. 打开“设置 → Skills”或“技能”；
2. 选择刚才配置的沙箱；
3. 在“安装技能”区域选择一个 **Skill 安装模型**；
4. 将仓库内的 `skills/dist/presentation-generator.zip` 拖到上传区域，或点击上传区域选择该文件；
5. 点击安装，并等待依赖检查和沙箱镜像快照完成；
6. 在已安装技能列表中确认 `presentation-generator` 的状态为“就绪”。

ZIP 内的 `SKILL.md` 必须位于压缩包根目录。本项目提供的安装包结构如下：

```text
presentation-generator.zip
├── SKILL.md
├── requirements.txt
└── scripts/
    └── create_presentation.py
```

如果你修改了 Skill 源码，可以在仓库根目录重新生成安装包：

```powershell
New-Item -ItemType Directory -Force skills/dist | Out-Null
Compress-Archive -Path skills/preloaded/presentation-generator/* `
  -DestinationPath skills/dist/presentation-generator.zip -Force
```

“来源地址”输入框只接受受支持的远程地址，不能填写本机的 `skills/preloaded/presentation-generator` 路径。将代码推送到远程仓库后，也可以填写指向该 Skill 目录或 ZIP 文件的 GitHub/GitLab 地址进行安装。

该 Skill 会安装 `python-pptx`。安装完成后，新会话会使用包含该 Skill 的沙箱镜像。

如果安装策略设置为“仅新会话生效”，请新建一个对话进行测试。旧会话会继续使用其原来的沙箱镜像。

## 4. 给 Agent 选择沙箱与 Skill

1. 打开“智能体”；
2. 新建或编辑一个 Agent；
3. 找到“Skills 与沙箱”配置区域；
4. 选择要使用的沙箱；
5. 在技能列表中勾选 `presentation-generator`；
6. 根据需要启用 `shell_exec` 等沙箱工具；
7. 保存 Agent。

如果 Agent 没有选择沙箱，即使勾选了 Skill，脚本也无法执行。

## 5. 打开沙箱工作台

1. 使用已经配置好的 Agent 新建对话；
2. 先让 Agent 执行一次需要沙箱的任务，例如：

   ```text
   请在沙箱中运行 pwd，并列出 /workspace 目录。
   ```

3. 等待命令完成；
4. 点击对话顶部的会话标题旁边的“…”菜单；
5. 点击“沙箱工作台”。

工作台从页面右侧打开，包含“终端”和“文件”两个页签。

如果提示“沙箱不可用”或“不支持终端”，请检查：

- 当前 Agent 是否选择了沙箱；
- 当前会话是否已经执行过一次沙箱任务；
- 沙箱配置是否被禁用或删除；
- 远端沙箱是否已过期且无法恢复；
- 当前账号是否拥有这个会话。

## 6. 使用终端

### 6.1 执行命令

1. 打开“终端”页签；
2. 在底部 `$` 后输入命令；
3. 按 Enter 或点击“运行”；
4. stdout 和 stderr 会追加显示在终端区域；
5. 命令结束后会显示状态和退出码。

示例：

```bash
pwd
ls -la /workspace
find /workspace/output -maxdepth 2 -type f
python --version
du -h /workspace/output/*
```

普通终端命令使用非 root 沙箱账户运行。工作目录只能位于 `/workspace` 下。

### 6.2 中断命令

运行中的命令右上角会显示“中断”按钮。

1. 点击“中断”；
2. 服务端取消本命令的执行上下文；
3. 沙箱后端终止对应执行；
4. 终端显示 `canceled` 或失败状态。

例如下面的命令可以用来测试中断：

```bash
for i in $(seq 1 100); do echo "step $i"; sleep 1; done
```

### 6.3 超时和资源限制

- 工作台命令默认最长运行 5 分钟；
- 服务端允许的单条命令上限为 30 分钟；
- Docker 容器同时受沙箱配置中的 CPU、内存和进程数限制；
- CubeSandbox/E2B 同时受远端模板和实例资源策略限制；
- 超时或资源超限后，命令会被终止并显示失败状态。

每条终端命令都会记录为 `sandbox.command_executed`，空间管理员可在审计日志中按该动作筛选。

## 7. 使用文件管理器

文件管理器只允许访问：

```text
/workspace/output
```

这是 Agent 和 Skill 生成产物的标准目录。

### 7.1 浏览目录

1. 打开“文件”页签；
2. 单击文件夹名称进入目录；
3. 点击左上角返回按钮回到上一级；
4. 点击刷新按钮重新获取目录内容。

服务端会拒绝以下路径：

```text
/etc/passwd
/workspace/input/secret.pdf
../input/secret.pdf
..\input\secret.pdf
```

即使通过浏览器开发者工具手动修改请求，也无法访问产物目录以外的文件。

### 7.2 上传文件

1. 进入目标目录；
2. 点击“上传”；
3. 选择文件；
4. 上传完成后列表自动刷新。

单个文件当前最大为 50 MiB。上传文件会写入当前会话的沙箱，不会进入其他租户或其他会话的沙箱。

### 7.3 下载文件

点击文件右侧的下载图标。下载请求会携带当前登录凭证，并再次校验空间与会话归属。

### 7.4 重命名文件

1. 点击文件右侧的编辑图标；
2. 输入新文件名；
3. 确认后等待列表刷新。

新名称只能是文件名，不能包含 `/`、`\` 或上级目录路径。目前只支持重命名文件，不支持重命名文件夹。

### 7.5 删除文件或目录

1. 点击删除图标；
2. 在确认框中确认；
3. 删除完成后列表自动刷新。

不能删除 `/workspace/output` 根目录。

## 8. 让 Agent 生成 PPTX

### 8.1 简单示例

在使用了 `presentation-generator` Skill 的 Agent 对话中输入：

```text
请生成一份 6 页的 PPT，主题是 WeKnora 可视化沙箱工作台。
受众是研发团队，内容包括背景、总体架构、交互式终端、文件管理、
产物预览和安全设计。请生成可下载的 PPTX。
```

Agent 会：

1. 读取 `presentation-generator` Skill；
2. 整理标题、每页主题和要点；
3. 在沙箱中运行生成脚本；
4. 将 PPTX 写到 `/workspace/output`；
5. 回答完成时收集该文件；
6. 在回答的产物区提供预览和下载。

### 8.2 基于知识库生成

```text
请先检索当前知识库中关于 2026 年产品规划的内容，核对关键数字，
然后生成一份面向管理层的 8 页 PPTX。最后一页列出下一步行动。
```

Agent 应先检索和核对内容，再生成演示文稿。生成脚本本身不会替 Agent 判断事实是否正确。

## 9. 查看三类产物

Agent 回答完成后，回答工具栏会出现产物文件入口。点击文件可以直接预览。

### 演示文稿

- `.pptx` 被标记为 `presentation`；
- 前端使用 PPTX 预览组件按页浏览；
- 可以同时下载原始 PPTX。

### 网页

- `.html`/`.htm` 被标记为 `web`；
- 在受限 iframe 中运行；
- iframe 仅启用脚本执行，不启用 `allow-same-origin`；
- 网页无法读取主站 DOM、Cookie、localStorage 或登录状态；
- 可以切换到源码视图检查 HTML。

### 表格

- `.csv`、`.xls`、`.xlsx` 被标记为 `spreadsheet`；
- 前端将内容转换成表格视图；
- 多工作表文件会显示各工作表内容；
- 仍可下载原始文件。

## 10. 租户与会话隔离说明

每次工作台请求都会校验：

1. 当前登录身份所属的租户；
2. 当前会话所属的租户；
3. 当前用户是否是会话 owner；
4. 会话绑定的沙箱配置；
5. 沙箱内部路径是否位于允许范围。

因此：

- 两个租户打开终端时会使用不同的会话沙箱绑定；
- 同一租户的普通成员也不能修改其他成员的会话沙箱；
- 管理员对渠道会话的只读观察权限不会自动变成工作台写权限；
- 浏览器不会收到远端 sandbox ID、对象存储真实地址或环境变量密钥。

## 11. 常见问题

### 找不到“沙箱工作台”

- 工作台位于对话顶部标题旁的“…”菜单；
- 嵌入式聊天页面不展示主站会话菜单；
- 确认前端已经更新到包含该功能的版本。

### 提示工作台不可用

- 检查空间是否配置了沙箱；
- 检查 Agent 是否选择了该沙箱；
- 先让 Agent 在当前会话执行一次沙箱任务；
- 检查沙箱配置是否已被管理员禁用；
- 检查远端 API、凭证、模板和网络连接。

### 终端没有立即显示内容

部分命令本身会缓存输出。Python 程序可使用无缓冲模式：

```bash
python -u script.py
```

也可以让程序主动刷新 stdout。

### 文件列表为空

确认程序将文件写入：

```text
/workspace/output
```

Skill 中建议读取环境变量：

```bash
echo "$WEKNORA_SKILL_OUTPUT_DIR"
```

不要把需要展示的产物只写在 `/tmp` 或技能安装目录。

### PPTX 没有出现在回答的产物区

- 确认生成脚本成功退出；
- 确认文件位于 `/workspace/output`；
- 确认文件没有超过产物收集大小限制；
- 等待回答完成后的产物收集阶段；
- 刷新会话或从工作台“文件”页确认文件是否存在。

### HTML 页面不能访问主站接口

这是预期的安全行为。HTML 产物运行在隔离 iframe 中，不继承 WeKnora 登录状态。若页面需要数据，应把必要数据直接写入产物，或访问明确允许的公开接口。

## 12. 推荐的首次验收流程

管理员或测试人员可以按以下顺序快速验收：

1. 配置 Docker 或远端沙箱并完成连接检查；
2. 安装 `presentation-generator`；
3. 新建 Agent，选择沙箱并启用该 Skill；
4. 新建会话，让 Agent 执行 `pwd && ls -la /workspace`；
5. 打开工作台，运行持续输出命令并测试“中断”；
6. 在文件页上传一个 CSV，测试下载、重命名和删除；
7. 尝试请求 `/etc/passwd` 或 `../input`，确认服务端拒绝；
8. 让 Agent 生成 PPTX，并在页面中预览和下载；
9. 生成 HTML，确认 iframe 无法读取主站 Cookie 和 DOM；
10. 生成 CSV/XLSX，确认以表格视图展示；
11. 在审计日志中筛选 `sandbox.command_executed`；
12. 使用另一个租户重复打开终端，确认双方文件和命令互不可见。
