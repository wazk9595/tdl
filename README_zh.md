> [!IMPORTANT]
> 中文文档可能落后于英文文档，如果有问题请先查看英文文档。
> 请使用英文发起新的 Issue, 以便于追踪和搜索

# tdl

<img align="right" src="docs/assets/img/logo.png" height="280" alt="">

> 📥 Telegram Downloader, but more than a downloader

<a href="README.md">English</a> | 简体中文

<p>
<img src="https://img.shields.io/github/go-mod/go-version/iyear/tdl?style=flat-square" alt="">
<img src="https://img.shields.io/github/license/iyear/tdl?style=flat-square" alt="">
<img src="https://img.shields.io/github/actions/workflow/status/iyear/tdl/master.yml?branch=master&amp;style=flat-square" alt="">
<img src="https://img.shields.io/github/v/release/iyear/tdl?color=red&amp;style=flat-square" alt="">
<img src="https://img.shields.io/github/downloads/iyear/tdl/total?style=flat-square" alt="">
</p>

#### 特性：

- 单文件启动
- 低资源占用
- 吃满你的带宽
- 比官方客户端更快
- 支持从受保护的会话中下载文件
- 具有自动回退和消息路由的转发功能
- 支持上传文件至 Telegram
- 导出历史消息/成员/订阅者数据至 JSON 文件

### 此分支的视频上传修复

直接下载：[各平台预编译安装包](https://github.com/wazk9595/tdl/releases/latest)，安装步骤见 [INSTALL-video.md](INSTALL-video.md)。

上传文件夹到指定话题：

```sh
tdl send "视频文件夹完整路径" --to "https://t.me/c/1234567890/42"
```

递归扫描、按内容去重、自动生成视频封面并记录发送回执。默认保留视频原编码；
再次运行同一命令会核对消息并继续未完成文件。加 `--dry-run` 只预览；
不兼容编码只有加 `--allow-transcode` 才会转换。首次运行不会按同名文件导入旧上传历史。

使用 `tdl up -p video.mp4 --video-mode stream` 可保留 H.264/HEVC/AV1 编码，
无损调整 MP4 索引位置后作为视频发送，并在发送前确认 Telegram 保留了封面和流式播放属性。
需要安装 FFmpeg（包含 FFprobe）；其他编码可使用 `--video-mode compatible` 转换。
详细选项和限制见[视频上传说明](docs/content/zh/guide/upload.md)。

## 预览

预览中的速度已经达到了代理的限制，同时**速度取决于你是否是付费用户**

![](docs/assets/img/preview.gif)

## 文档

请参考 [文档](https://docs.iyear.me/tdl/zh/).

## 赞助者

![](https://raw.githubusercontent.com/iyear/sponsor/master/sponsors.svg)

## 贡献者
<a href="https://github.com/iyear/tdl/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=iyear/tdl&max=750&columns=20" alt="contributors"/>
</a>

## 协议

AGPL-3.0 License
