# tdl

<img align="right" src="docs/assets/img/logo.png" height="280" alt="">

> 📥 Telegram Downloader, but more than a downloader

English | <a href="README_zh.md">简体中文</a>

<p>
<img src="https://img.shields.io/github/go-mod/go-version/iyear/tdl?style=flat-square" alt="">
<img src="https://img.shields.io/github/license/iyear/tdl?style=flat-square" alt="">
<img src="https://img.shields.io/github/actions/workflow/status/iyear/tdl/master.yml?branch=master&amp;style=flat-square" alt="">
<img src="https://img.shields.io/github/v/release/iyear/tdl?color=red&amp;style=flat-square" alt="">
<img src="https://img.shields.io/github/downloads/iyear/tdl/total?style=flat-square" alt="">
</p>

#### Features:
- Single file start-up
- Low resource usage
- Take up all your bandwidth
- Faster than official clients
- Download files from (protected) chats
- Forward messages with automatic fallback and message routing
- Upload files to Telegram
- Export messages/members/subscribers to JSON

### Video upload fork

[Download prebuilt binaries](https://github.com/wazk9595/tdl/releases/latest) for Windows, macOS, and Linux (x86_64 / ARM64).
See [INSTALL-video.md](INSTALL-video.md) for installation and FFmpeg requirements.
This fork supports lossless streaming preparation with `--video-mode stream`, reliable thumbnails, and validated video metadata.

```sh
tdl send "folder" --to "https://t.me/c/1234567890/42"
```

Recursively upload a folder to a forum topic with SHA256 deduplication, durable receipts, and remote verification.
Repeat the same command to resume. Videos preserve their codecs by default; incompatible codecs require `--allow-transcode`.
Use `--dry-run` to preview. Previously uploaded files are not imported by filename. Keep the task journal for future runs.

Bulk forwarding from Saved Messages supports inclusive ID ranges, individual IDs, and exclusions:

```sh
tdl forward --from me --ids "100-160" --exclude "150" --to "https://t.me/c/1234567890/42"
```

Albums are expanded as complete groups; excluding any member skips its whole album. Use `--ids "100,160"` for individual IDs,
`--dry-run` to preview, or `--mode clone` to copy. Forwarding deduplicates within one run only; rerunning forwards again.

## Preview

It reaches my proxy's speed limit, and the **speed depends on whether you are a premium**

![](docs/assets/img/preview.gif)

## Documentation

Please refer to the [documentation](https://docs.iyear.me/tdl/).

## Sponsors

![](https://raw.githubusercontent.com/iyear/sponsor/master/sponsors.svg)

## Contributors
<a href="https://github.com/iyear/tdl/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=iyear/tdl&max=750&columns=20" alt="contributors"/>
</a>

## LICENSE

AGPL-3.0 License
