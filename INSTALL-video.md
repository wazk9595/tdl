# tdl 视频上传增强版安装

下载入口：https://github.com/wazk9595/tdl/releases/latest

不需要安装 Go，也不需要自己编译。按系统和处理器选择压缩包：

| 设备 | 文件 |
| --- | --- |
| Windows Intel/AMD 64 位 | tdl_Windows_64bit.zip |
| Windows ARM64 | tdl_Windows_arm64.zip |
| macOS Intel | tdl_MacOS_64bit.tar.gz |
| macOS Apple 芯片 | tdl_MacOS_arm64.tar.gz |
| Linux x86_64 | tdl_Linux_64bit.tar.gz |
| Linux ARM64 | tdl_Linux_arm64.tar.gz |

每个包包含 tdl、许可证和说明。发布页的 `tdl_checksums.txt` 提供 SHA256 校验值。
FFmpeg 不包含在安装包内；上传视频还需要 `ffmpeg` 和 `ffprobe`。

## Windows

解压到固定目录，例如 `C:\tdl`，把该目录加入用户 PATH。
安装 FFmpeg 并将其 bin 目录加入 PATH；也可以把 `ffmpeg.exe`、`ffprobe.exe` 放在 `tdl.exe` 同一目录。
重新打开终端检查：

```powershell
tdl version
ffmpeg -version
ffprobe -version
```

## macOS / Linux

解压对应的 tar.gz，把 tdl 放入 PATH 中的目录，例如：

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 tdl "$HOME/.local/bin/tdl"
export PATH="$HOME/.local/bin:$PATH"
tdl version
```

把 export 那一行加入所用 shell 的配置文件，供后续终端使用。
通过系统包管理器安装 FFmpeg，并确认 `ffmpeg`、`ffprobe` 均可调用。

## 登录及上传

每台设备可独立扫码登录：

```sh
tdl login -T qr
```

上传整个文件夹到话题，只需一个目录和一个链接：

```sh
tdl send "视频文件夹完整路径" --to "https://t.me/c/1234567890/42"
```

命令会递归扫描子文件夹，按 SHA256 跳过内容完全相同的副本。视频按视频发送并附带封面，
照片按照片发送，其他内容按文件发送。默认保留视频原编码，必要时无损调整封装以支持流式播放。
客户端能否解码 HEVC/AV1 仍取决于设备；需要广泛兼容时应使用 H.264/AAC。

任务会记录在当前设备的 `.tdl/tasks`。中断后运行相同命令，会先核对原有回执，再继续未完成文件；
新加入或内容改变的文件会上传。原文件始终保留。第一次使用该命令没有旧任务记录，
不会把群里已有的同名文件认作已完成；更换文件夹位置、账号、目标或丢失任务记录会创建新任务。

先检查链接、权限和文件数量，不实际上传：

```sh
tdl send "视频文件夹完整路径" --to "https://t.me/c/1234567890/42" --dry-run
```

不兼容的编码会列为待处理，不会自动重编码。明确允许转换时使用：

```sh
tdl send "视频文件夹完整路径" --to "https://t.me/c/1234567890/42" --allow-transcode
```

转码默认检测 NVIDIA 显卡；有显卡使用 NVENC，没有时使用 CPU。可通过 `--video-encoder` 明确指定。
图片需要保留原始字节时加 `--original-images`。`--to me` 可上传到自己的收藏夹。
本命令按文件逐条发送，不创建媒体组。

远端消息已删除时默认只报告；确认需要补发才加 `--resend-missing`。
发送回执丢失时先按媒体编号查找对应消息；无法确认时暂停该文件，核查后可加 `--retry-uncertain`，
复用原随机消息编号重试。Telegram 对该编号的去重期限由服务器决定，因此不能保证无限期去重。
保留任务目录才能续传；`--task-dir` 用于自定义位置，必须放在待上传目录之外。

原有单文件上传命令仍可使用。优先保留原画质并支持边下载边播放：

```sh
tdl up -p "视频完整路径" --video-mode stream
```

stream 模式不要求 NVIDIA 显卡；它保留 H.264/HEVC/AV1 编码并无损调整封装，音轨需兼容 AAC。
其他编码可转换成 H.264/AAC；没有 NVIDIA 显卡时明确选 CPU 编码器：

```sh
tdl up -p "视频完整路径" --video-mode compatible --video-encoder libx264
```

发送前会检查 Telegram 返回的视频类型、封面及所需流式播放属性。默认保留源文件。
转换或换封装会选择首个非封面视频流和首条音轨；`original` 模式用于保留原始字节，但不保证流式播放。

## 从收藏夹批量转发

```powershell
# ID 100 到 160，包含两端
tdl forward --from me --ids "100-160" --to "话题链接"
# 只选 ID 100 和 160
tdl forward --from me --ids "100,160" --to "话题链接"
# 同一范围排除 ID 150
tdl forward --from me --ids "100-160" --exclude "150" --to "话题链接"
```

默认补齐整组相册；排除项属于相册时跳过整组。范围与多个 ID 可混合，排除项也支持范围。
先加 `--dry-run` 可预览最终消息清单；需要去掉转发来源时加 `--mode clone`。
转发只在本次执行内去重，再次运行会重新转发；文件夹上传的任务记录不适用于此命令。

## 更新

本增强版的 `tdl update` 从 `wazk9595/tdl` 的 Release 获取对应平台的安装包并校验 SHA256。
更新命令沿用上游的管理员/root 权限要求。

## English

Download the archive matching your OS and CPU from this fork's Releases page.
Extract tdl into a directory on PATH. Install FFmpeg, including ffprobe, separately.
Use `tdl login -T qr` to sign in, then `tdl up -p "video path" --video-mode stream` for lossless streaming preparation.
Use `tdl send "folder" --to "https://t.me/c/1234567890/42"` for recursive, content-deduplicated uploads with durable receipts.
Repeat the same command to verify and resume. Existing manual uploads are not imported by filename. Use `--dry-run` to preview,
and `--allow-transcode` to explicitly permit incompatible codecs to be converted. Keep the task journal for future runs.
For incompatible codecs without an NVIDIA GPU, use `--video-mode compatible --video-encoder libx264`.
Checksums are provided in `tdl_checksums.txt`. Updates stay on the wazk9595/tdl fork.
