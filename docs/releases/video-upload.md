本版提供 Windows、macOS、Linux 的 x86_64 和 ARM64 预编译程序，下载解压后即可使用，无需安装 Go 或编译源码。

### v0.20.4-video.4：按 ID 范围批量转发

```sh
tdl forward --from me --ids "100-160" --exclude "150" --to "话题链接"
```

- 支持单个 ID、多个 ID、闭区间及混合范围；排除项也支持多个 ID 和范围。
- 目标话题链接直接解析并检查，收藏夹中的 ID 空档不会误读为其他聊天中的消息。
- 默认补齐整组，排除相册任意成员时跳过整组；同一组在本次只转发一次。
- 发送失败会返回失败状态，不再把失败项记入本次成功列表；整组准备失败时停止该组，避免静默漏项。
- 普通转发遇到不明网络错误时不会自动改用 clone 重发，减少回执不明导致的重复发送。
- 已在收藏夹实测普通转发与 clone，核对每组三条消息；排除组内一项及范围外的同组成员均正确跳过整组。

加 `--dry-run` 可先查看清单，`--mode clone` 使用复制模式。转发去重仅限本次执行，跨次任务续传尚未实现。

### v0.20.4-video.3：文件夹直接发到话题

```sh
tdl send "视频文件夹完整路径" --to "https://t.me/c/1234567890/42"
```

- 自动检查话题与权限，递归扫描子目录，以 SHA256 去重，不会误跳过同名不同内容的文件。
- 视频默认保留原编码，按需无损调整封装、生成封面并核对流式属性；不兼容编码需明确加 `--allow-transcode`。
- 持久记录服务器媒体编号、固定随机消息编号及发送回执。中断后重复原命令，先核对再继续。
- 发送结果不明时查找对应媒体，不盲目重发；已被远端删除的消息默认报告，不自动补发。
- 批量读取既有消息，避免续传时逐个发起核对请求。原有 `up` 命令保持可用。

已在收藏夹实测视频、照片、文档上传与重复运行；单元测试覆盖断线回执丢失恢复、同名不同内容、
源文件变更、任务锁与远端删除场景。首次使用没有既往任务记录，不会根据远端同名文件导入手动上传历史。
任务记录保存在当前设备，需保留才能续传；本命令逐文件发送，不自动建立媒体组。

### 主要修复

- 用 FFprobe 识别常见视频容器、编码、时长、显示尺寸及旋转信息。
- `--video-mode stream` 保留 H.264/HEVC/AV1 编码，无损调整 MP4 索引位置，支持边下载边播放。
- 修复封面被错误当作未知大小的大文件上传、随后丢失的问题。
- 发送前核对 Telegram 返回的视频类型、封面和流式播放属性；处理失败会明确报错。
- 修复上传失败仍报告成功，以及显式使用 `--rm` 时可能误删失败源文件的问题。
- `tdl update` 使用本 fork 的 Release，避免覆盖为上游原版。

### 选择安装包

| 设备 | 安装包 |
| --- | --- |
| Windows Intel/AMD 64 位 | `tdl_Windows_64bit.zip` |
| Windows ARM64 | `tdl_Windows_arm64.zip` |
| macOS Intel | `tdl_MacOS_64bit.tar.gz` |
| macOS Apple 芯片 | `tdl_MacOS_arm64.tar.gz` |
| Linux x86_64 | `tdl_Linux_64bit.tar.gz` |
| Linux ARM64 | `tdl_Linux_arm64.tar.gz` |

安装包包含详细的 `INSTALL-video.md`，发布附件 `tdl_checksums.txt` 提供 SHA256 校验值。
**上传视频仍需单独安装 FFmpeg（包含 ffprobe）**，并将它们加入 PATH；Windows 也可放在 tdl.exe 同一目录。

```sh
tdl login -T qr
tdl up -p "视频完整路径" --video-mode stream
```

其他编码在没有 NVIDIA 显卡的设备上使用：

```sh
tdl up -p "视频完整路径" --video-mode compatible --video-encoder libx264
```

### 验证范围

18 个常见视频样本已在 Windows 通过处理和解码测试；约 1.8 GB 的完整 HEVC 视频已实际上传，并核对封面及流式播放属性。
macOS/Linux 安装包经过交叉编译；Linux x86_64 在发布流程中检查 CLI 启动。尚未在所有平台实际登录 Telegram 上传。

This fork adds reliable video metadata, sized JPEG thumbnails, lossless faststart remuxing, and validated streaming uploads.
New in video.3: `tdl send FOLDER --to TOPIC_LINK` adds recursive SHA256 deduplication, durable receipts, and verified resume.
New in video.4: ID-based bulk forwarding from Saved Messages, topic links, explicit exclusions, and complete-album handling.
Choose your platform archive, install FFmpeg separately, and see INSTALL-video.md for setup. No Go toolchain is needed.
