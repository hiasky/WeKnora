---
name: presentation-generator
description: 生成可下载和页面内预览的 PowerPoint 演示文稿。当用户要求制作 PPT、幻灯片、路演材料、汇报课件或把知识库内容整理为演示文稿时使用。
---

# Presentation Generator

先根据用户材料确定受众、目的和叙事顺序，再调用脚本生成 `.pptx`。如果关键信息已足够，不要为了风格偏好阻塞生成；使用清晰、克制的默认版式。

## 输入组织

传给 `scripts/create_presentation.py` 的 stdin 必须是 JSON：

```json
{
  "title": "季度业务复盘",
  "subtitle": "2026 Q2",
  "author": "可选",
  "output_file": "quarterly-review.pptx",
  "slides": [
    {
      "title": "核心结论",
      "bullets": ["收入同比增长 18%", "重点客户留存率 94%"],
      "notes": "可选的演讲者备注"
    },
    {
      "title": "下一步",
      "bullets": ["扩大华东试点", "优化交付周期"]
    }
  ]
}
```

约束：

- `slides` 应包含 1–50 页内容页；脚本会自动添加标题页。
- 每页聚焦一个观点，通常使用 3–6 个简短要点；不要把长篇正文塞进幻灯片。
- `output_file` 只能是文件名，扩展名必须为 `.pptx`。脚本始终写入 `WEKNORA_SKILL_OUTPUT_DIR`，不要尝试写其他目录。
- 需要引用知识库内容时，先完成检索和事实核对，再把精炼后的文字传入脚本。

## 执行

调用 `execute_skill_script`：

```json
{
  "skill_name": "presentation-generator",
  "script_path": "scripts/create_presentation.py",
  "input": "{...上述 JSON...}"
}
```

成功结果会返回生成文件的绝对路径、页数和大小。最终回答应明确告诉用户演示文稿已生成，可在产物区直接预览或下载；不要把沙箱内部路径当作下载链接。
