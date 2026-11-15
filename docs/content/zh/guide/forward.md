---
title: "转发"
weight: 35
---

# 转发

具有自动回退和消息路由的转发功能

## 按消息 ID 批量选择（本分支）

```powershell
# 收藏夹 ID 100 到 160，包含两端
tdl forward --from me --ids "100-160" --to "话题链接"
# 只选择 ID 100 和 160
tdl forward --from me --ids "100,160" --to "话题链接"
# 同一范围排除 ID 150
tdl forward --from me --ids "100-160" --exclude "150" --to "话题链接"
# 范围、单个 ID 和多个排除范围混合，使用复制模式
tdl forward --from me --ids "100-120,150,160-180" --exclude "110-115,170" --to "话题链接" --mode clone
```

- `--from me` 指收藏夹，也可用 `self`、`saved`；其他聊天填一个群 ID 或用户名。
- ID 是所选来源聊天中的消息 ID。空档、已删除消息及服务消息不会发送；不会读取其他聊天中的同号消息。
- 默认整组转发：选中相册任意一条，会补齐当前可读取的同组消息，即使成员位于范围外；重复 ID、重叠范围和同组多个 ID 在本次只处理一次。
- **排除项优先：只要相册中包含被排除的 ID，就跳过整组。** 排除 ID 位于范围外时也适用。`--single` 禁用整组处理，此时只排除指定的单条消息。
- 默认按 ID 从小到大发送，`--desc` 反转各组或独立消息的顺序，组内顺序保持不变。
- `--to` 可直接填写话题链接，例如 `https://t.me/c/1234567890/42`，会先检查目标。使用 `--ids` 时必须明确指定目标；`--to me` 表示收藏夹。
- 加 `--dry-run` 只读取并显示实际消息 ID、媒体组及排除统计，不发送。加 `--mode clone` 使用复制模式。
- 转发失败或整组准备失败会返回非零退出码；失败消息不会被记作本次成功，组内准备失败不会静默漏项。
- 去重仅作用于本次执行，**再次运行会再次转发**，尚未提供跨次转发任务的进度记录。发送结果不明时先核对目标，再决定是否重跑。

一行命令将消息从 `https://t.me/telegram/193` 转发到 `收藏夹`：

{{< command >}}
tdl forward --from https://t.me/telegram/193
{{< /command >}}

## 自定义来源

{{< include "snippets/link.md" >}}

您可以从链接和[导出的JSON文件](/zh/guide/download/#从-json-下载)转发消息：

{{< command >}}
tdl forward \
--from https://t.me/telegram/193 \
--from https://t.me/telegram/195 \
--from tdl-export.json \
--from tdl-export2.json
{{< /command >}}

## 自定义目标

{{< include "snippets/chat.md" >}}

### 特定聊天

转发到特定的聊天：

{{< command >}}
tdl forward --from tdl-export.json --to CHAT
{{< /command >}}

### 消息路由

通过基于 [expr](/zh/reference/expr) 的路由将消息转发至不同的聊天

列出所有可用的字段：

{{< command >}}
tdl forward --from tdl-export.json --to -
{{< /command >}}

如果消息包含 `foo`，则转发到 `CHAT1`，否则转发到 `收藏夹`：

{{< hint info >}}
表达式必须返回一个**字符串**或者**结构体**作为目标 CHAT，空字符串表示转发到 `收藏夹`。
{{< /hint >}}

{{< command >}}
tdl forward --from tdl-export.json \
--to 'Message.Message contains "foo" ? "CHAT1" : ""'
{{< /command >}}

转发含有 `foo` 的消息到 `CHAT1`，否则转发到 `CHAT2` 中 ID 为 4 的消息/主题：

{{< command >}}
tdl forward --from tdl-export.json \
--to 'Message.Message contains "foo" ? "CHAT1" : { Peer: "CHAT2", Thread: 4 }'
{{< /command >}}

如果表达式较复杂，你可以传递文件名：

{{< details "router.txt" >}}
你可以像写 `switch` 一样编写表达式：

```javascript
Message.Message contains "foo" ? "CHAT1" :
From.ID == 123456 ? "CHAT2" :
Message.Views > 30 ? { Peer: "CHAT3", Thread: 101 } :
""
```

{{< /details >}}

{{< command >}}
tdl forward --from tdl-export.json --to router.txt
{{< /command >}}

## 模式

消息转发采取自动降级策略

可用模式：
- `direct`（默认）
- `clone`

### Direct

优先使用官方的转发API。

如果聊天或消息不允许使用官方转发API，将自动降级为 `clone` 模式。

{{< command >}}
tdl forward --from tdl-export.json --mode direct
{{< /command >}}

### Clone

通过复制方式转发消息，将不包含转发来源的标头。

将自动忽略一些无法复制的消息内容，例如投票、发票等

{{< command >}}
tdl forward --from tdl-export.json --mode clone
{{< /command >}}

## 编辑

使用[表达式引擎](/reference/expr)编辑转发前的消息。

{{< hint info >}}
- 你必须传递合并照片的第一条消息才能编辑标题。
- 你可以传递任何合并文档的消息以编辑相应的评论。
{{< /hint >}}

你可以在表达式中引用原始消息的相关字段。

列出所有可用字段：
{{< command >}}
tdl forward --from tdl-export.json --edit -
{{< /command >}}

在原始消息后附加 `测试转发消息`：
{{< command >}}
tdl forward --from tdl-export.json --edit 'Message.Message + " 测试转发消息"'
{{< /command >}}

以[HTML](https://core.telegram.org/bots/api#html-style)格式编写带有样式的消息：
{{< command >}}
tdl forward --from tdl-export.json --edit \
'Message.Message + `<b>粗体</b> <a href="https://example.com">链接</a>`'
{{< /command >}}

如果表达式较复杂，可以传递文件名：

{{< details "edit.txt" >}}
```javascript
repeat(Message.Message, 2) + `
<a href="https://www.google.com">谷歌</a>
<a href="https://www.bing.com">必应</a>
<b>粗体</b>
<i>斜体</i>
<code>代码</code>
<tg-spoiler>剧透</tg-spoiler>
<pre><code class="language-go">
package main

import "fmt"

func main() {
    fmt.Println("hello world")
}
</code></pre>
` + From.VisibleName
```
{{< /details >}}

{{< command >}}
tdl forward --from tdl-export.json --edit edit.txt
{{< /command >}}

## 试运行

只打印进度而不实际发送消息，可以用于调试消息路由的效果。

{{< command >}}
tdl forward --from tdl-export.json --dry-run
{{< /command >}}

## 静默发送

发送消息而不通知其他成员。

{{< command >}}
tdl forward --from tdl-export.json --silent
{{< /command >}}

## 取消分组检测

默认情况下，tdl 将自动探测到分组消息并将它们转发为合并的消息。

你可以通过 `--single` 禁用此行为，将其作为单个消息转发。

{{< command >}}
tdl forward --from tdl-export.json --single
{{< /command >}}

## 反序

对每个来源的消息进行反序转发。

{{< command >}}
tdl forward --from tdl-export.json --desc
{{< /command >}}
