本版提供 Windows、macOS、Linux 的 x86_64 和 ARM64 预编译程序，下载解压后即可使用，无需安装 Go 或编译源码。

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
Choose your platform archive, install FFmpeg separately, and see INSTALL-video.md for setup. No Go toolchain is needed.
