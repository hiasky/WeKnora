# 课题四：知识网络与引导式学习原型

实际启用、体验、接口调用与演示步骤见[使用指南](guided-learning-user-guide.md)。

## 1. 目标与范围

本原型把长期记忆中已经存在的“回答引用了哪些文档”投影到 Wiki 链接图上，回答三个问题：用户已经反复使用哪些知识、证据是什么、下一步可看什么。第一版刻意不让 LLM 直接评判用户水平，也不引入第二套知识图谱。

可运行范围：开启长期记忆与 Wiki 的知识库；用户完成若干带知识库引用的问答后，进入 Wiki 图谱即可看到掌握度和推荐节点。

## 2. 定义

### 2.1 知识节点

节点直接采用 `wiki_pages`：身份是 `(tenant_id, knowledge_base_id, slug)`。选择 Wiki 页面而不是 Neo4j 实体的原因是：页面已有稳定 slug、别名、来源文档、引用证据、访问控制和可点击正文；同时包含实体、概念、摘要、综合和对比等适合学习的粒度。

页面的 `source_refs` 提供 `knowledge_id|title`，把节点与原始资料建立确定性映射；`in_links/out_links` 构成引导用网络。页面重命名由 Wiki 机制更新反向链接，原始文档变更则由 Wiki ingest 重建页面来源。

### 2.2 掌握度

原型的“掌握度”准确说是**有证据的使用熟悉度**，不是考试能力：

```text
evidence_hits(page) = memory_wiki_affinity.hits(page.slug)
                    + sum(memory_doc_affinity.hits for page.source_knowledge_ids)
mastery_score(page) = min(100, evidence_hits(page) * 10)
```

`memory_wiki_affinity.hits` 在成功的 `wiki_read_page` 把页面实际放入模型上下文时增长；一次回答内重复读取同页只计一次，搜索命中、失败和因预算省略均不计。`memory_doc_affinity.hits` 只在回答实际采用某原始文档作为来源时增长。两类证据在导出中分别呈现，分数可重算、可解释、与 LLM 的主观自评无关。十次封顶只是 UI 标尺，不声称用户达到教育学意义上的完全掌握。

局限：阅读/引用并不等于理解，而且一个页面可能继承整篇文档的点击证据。后续应加入独立证据并分别展示：用户主动打开并停留（弱）、能够正确回答检索增强生成的题目（强）、间隔复习后仍答对（最强）。不要把这些信号先交给模型再只保存一个不可解释分数。

### 2.3 下一步推荐

第一版采用局部 frontier 策略：掌握度为 0、且与至少一个有掌握证据节点直接相连的页面进入推荐集；理由明确显示为“与已掌握节点 X 直接相连”。这能保证推荐处在用户当前知识边界附近。

Wiki 链接不表达先修方向，所以本版将边视为无向关系，不声称推荐是严格先修顺序。后续可在页面元数据增加 `prerequisites`，并用“先修均达阈值、本人尚未掌握、概念中心性适中、近期未拒绝”的多目标排序。

## 3. 数据流与接口

```text
带引用的回答
  -> MemoryService.RecordAnswerSources
  -> memory_doc_affinity (tenant + subject + knowledge)
  -> Wiki source_refs 映射到页面
  -> 确定性掌握度
  -> Wiki 邻接 frontier
  -> 图谱亮度、蓝色已使用环、橙色虚线推荐环

Wiki-first 回答则走 `wiki_read_page -> read_pages -> memory_wiki_affinity -> slug 精确映射`，不依赖原始文档引用卡片。
```

接口均沿用 Wiki 的 `Viewer + KBAccessRead`，用户身份只从请求上下文解析，不接受 `tenant_id` 或 `subject_id` 参数：

- `GET /api/v1/knowledgebase/:kb_id/wiki/graph`：节点新增 `mastery_score`、`evidence_hits`、`recommended`、`recommendation_reason`；
- `GET /api/v1/knowledgebase/:kb_id/wiki/learning-profile`：查看当前用户画像与定义；
- `GET /api/v1/knowledgebase/:kb_id/wiki/learning-profile/export`：下载 JSON；
- `DELETE /api/v1/knowledgebase/:kb_id/wiki/learning-profile`：只删除当前用户在该 KB 的文档使用证据，不影响 Wiki、其他 KB 或其他用户。

画像是源数据的可重算投影，不额外持久化冗余分数，避免页面更新后遗留孤儿画像，也让删除语义完整。

## 4. 可视化

- 节点颜色继续表示 Wiki 页面类型；
- 节点填充透明度随掌握度提高，避免用颜色同时表达两个维度；
- 蓝色实线外环表示达到常用资料阈值；
- 橙色虚线外环表示建议下一步学习；
- 鼠标悬停显示分数、引用次数或推荐理由；图例只在相应节点存在时出现。

## 5. 验证方案

### 5.1 离线可重复测试

单元测试固定一张小图和文档引用次数，验证：分数可复算且封顶；只有已掌握节点的一跳未掌握邻居进入 frontier；断开的热门节点不被推荐；租户/用户标识不从 API 输入。运行：

```bash
go test ./internal/application/service -run 'TestComputeGraphSubset|TestMasteryScore' -count=1
```

### 5.2 小规模试用

招募 10–20 位用户，对每个启用 Wiki 的 KB 交叉随机两周：A 组显示普通图，B 组显示学习覆盖层，第二周交换。主要指标是“推荐页面打开后 7 天内被回答引用的比例”；次要指标是推荐点击率、每会话探索的新节点数、用户对推荐理由的可信度（5 分量表）。记录覆盖层曝光而非敏感画像正文。

有效门槛可预注册为：B 相比 A 的推荐后有效引用率提升至少 15%，且“理由可信”均值不低于 3.5/5。另抽样检查至少 100 个推荐，人工标注“与当前主题相关”，报告 Precision@5 和 95% bootstrap 置信区间。

### 5.3 防退化检查

- 证据为 0 时分数必须为 0，禁止模型补分；
- 删除画像后重新请求所有分数为 0，且其他用户不变；
- 页面来源跨 KB 时不计入当前 KB；
- 超过十次引用保持 100，避免重度用户破坏尺度；
- 明确把该指标称为使用熟悉度，不能在产品文案中宣称真实能力认证。

## 6. 后续闭环

下一阶段可增加 3–5 道带出处的低风险检索题。答题结果保存为追加式 evidence（题目版本、作答、判定规则、时间），使用项目分析/IRT 或简单 Beta-Binomial 置信区间估计掌握度；间隔复习对置信度衰减而不是直接改写历史证据。用户可纠正推荐、标记“已会/不感兴趣”，这些反馈同样必须可查看、导出、删除。
