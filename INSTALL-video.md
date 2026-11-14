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

优先保留原画质并支持边下载边播放：

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

## 更新

本增强版的 `tdl update` 从 `wazk9595/tdl` 的 Release 获取对应平台的安装包并校验 SHA256。
更新命令沿用上游的管理员/root 权限要求。

## English

Download the archive matching your OS and CPU from this fork's Releases page.
Extract tdl into a directory on PATH. Install FFmpeg, including ffprobe, separately.
Use `tdl login -T qr` to sign in, then `tdl up -p "video path" --video-mode stream` for lossless streaming preparation.
For incompatible codecs without an NVIDIA GPU, use `--video-mode compatible --video-encoder libx264`.
Checksums are provided in `tdl_checksums.txt`. Updates stay on the wazk9595/tdl fork.
