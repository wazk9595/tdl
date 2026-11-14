package mediautil

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// VideoInfo describes the selected real video stream, excluding cover art.
type VideoInfo struct {
	Duration                               time.Duration
	Width, Height, Stream                  int
	Codec, PixelFormat, Format, AudioCodec string
	AudioChannels, Rotation                int
	HasAudio, Streaming                    bool
}

var ErrNoVideo = errors.New("no video stream")

func IsVideoFile(name, mime string) bool {
	if IsVideo(mime) {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".qt", ".mkv", ".webm", ".avi", ".flv", ".f4v", ".wmv", ".asf", ".ts", ".mts", ".m2t", ".m2ts", ".mpeg", ".mpg", ".m2v", ".vob", ".3gp", ".3g2", ".ogv", ".ogm", ".rm", ".rmvb", ".divx", ".mxf":
		return true
	}
	return false
}

// ProbeVideo uses a path rather than a pipe: MP4/MOV metadata may be at EOF.
func ProbeVideo(ctx context.Context, path string) (*VideoInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, ffmpegTool("ffprobe"), "-v", "error", "-show_streams", "-show_format", "-of", "json", "-i", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe (install FFmpeg and add it to PATH): %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	info, err := parseVideoInfo(data)
	if err == nil {
		info.Streaming = info.Format == "mov,mp4,m4a,3gp,3g2,mj2" && fastStart(path)
	}
	return info, err
}

func parseVideoInfo(data []byte) (*VideoInfo, error) {
	var probe struct {
		Streams []struct {
			Index       int    `json:"index"`
			Codec       string `json:"codec_name"`
			Type        string `json:"codec_type"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			PixelFormat string `json:"pix_fmt"`
			Channels    int    `json:"channels"`
			Duration    string `json:"duration"`
			SAR         string `json:"sample_aspect_ratio"`
			Disposition struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
			Tags struct {
				Rotate string `json:"rotate"`
			} `json:"tags"`
			SideData []struct {
				Rotation float64 `json:"rotation"`
			} `json:"side_data_list"`
		} `json:"streams"`
		Format struct {
			Name     string `json:"format_name"`
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}
	info := &VideoInfo{Format: probe.Format.Name, Stream: -1}
	for _, s := range probe.Streams {
		if s.Type == "audio" && !info.HasAudio {
			info.HasAudio, info.AudioCodec, info.AudioChannels = true, s.Codec, s.Channels
		}
		if s.Type != "video" || s.Disposition.AttachedPic != 0 || info.Stream >= 0 {
			continue
		}
		if s.Width <= 0 || s.Height <= 0 {
			return nil, errors.New("invalid video dimensions")
		}
		info.Stream, info.Width, info.Height, info.Codec, info.PixelFormat = s.Index, s.Width, s.Height, s.Codec, s.PixelFormat
		// Telegram expects display dimensions, including non-square pixels and rotation.
		if parts := strings.Split(s.SAR, ":"); len(parts) == 2 {
			n, _ := strconv.ParseFloat(parts[0], 64)
			d, _ := strconv.ParseFloat(parts[1], 64)
			if n > 0 && d > 0 {
				info.Width = int(math.Round(float64(info.Width) * n / d))
			}
		}
		rotation, _ := strconv.ParseFloat(s.Tags.Rotate, 64)
		for _, side := range s.SideData {
			rotation = side.Rotation
		}
		info.Rotation = (int(math.Round(rotation))%360 + 360) % 360
		if info.Rotation == 90 || info.Rotation == 270 {
			info.Width, info.Height = info.Height, info.Width
		}
		duration, _ := strconv.ParseFloat(s.Duration, 64)
		if duration <= 0 {
			duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
		}
		if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) || duration > float64(math.MaxInt64)/float64(time.Second) {
			return nil, errors.New("invalid video duration")
		}
		info.Duration = time.Duration(duration * float64(time.Second))
	}
	if info.Stream < 0 {
		return nil, ErrNoVideo
	}
	return info, nil
}

// fastStart checks top-level MP4 atoms without reading the media payload.
func fastStart(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	for offset := int64(0); offset+8 <= stat.Size(); {
		var header [16]byte
		if _, err := f.ReadAt(header[:8], offset); err != nil {
			return false
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		min := int64(8)
		if size == 1 {
			if _, err := f.ReadAt(header[8:], offset+8); err != nil {
				return false
			}
			size = int64(binary.BigEndian.Uint64(header[8:]))
			min = 16
		}
		if size == 0 {
			size = stat.Size() - offset
		}
		if size < min || size > stat.Size()-offset {
			return false
		}
		switch string(header[4:8]) {
		case "moov":
			return true
		case "mdat":
			return false
		}
		offset += size
	}
	return false
}

// PreparedVideo owns temporary output only; Close never removes the source.
type PreparedVideo struct {
	Path, Name, Thumbnail, Dir string
	Info                       *VideoInfo
}

func (v *PreparedVideo) Close() error {
	if v.Dir == "" {
		return nil
	}
	return os.RemoveAll(v.Dir)
}

func copyVideo(v *VideoInfo) bool {
	return v.Codec == "h264" && v.PixelFormat == "yuv420p" && v.Rotation == 0
}
func copyAudio(v *VideoInfo) bool {
	return !v.HasAudio || (v.AudioCodec == "aac" && v.AudioChannels <= 2)
}

// PrepareVideo preserves compatible MP4, remuxes compatible codecs, and converts
// other codecs to H.264/AAC. Original mode only probes and attaches video metadata.
func PrepareVideo(ctx context.Context, path, mode, encoder string) (_ *PreparedVideo, rerr error) {
	info, err := ProbeVideo(ctx, path)
	if err != nil {
		return nil, err
	}
	if mode == "stream" && ((info.Codec != "h264" && info.Codec != "hevc" && info.Codec != "av1") || !copyAudio(info)) {
		return nil, errors.New("stream mode requires H.264/HEVC/AV1 with AAC audio; use compatible mode for other codecs")
	}
	v := &PreparedVideo{Path: path, Name: filepath.Base(path), Info: info}
	defer func() {
		if rerr != nil {
			_ = v.Close()
		}
	}()
	v.Dir, err = os.MkdirTemp("", "tdl-video-*")
	if err != nil {
		return nil, err
	}
	if (mode == "compatible" && !(info.Streaming && copyVideo(info) && copyAudio(info))) || (mode == "stream" && !info.Streaming) {
		v.Path = filepath.Join(v.Dir, "video.mp4")
		v.Name = strings.TrimSuffix(v.Name, filepath.Ext(v.Name)) + ".mp4"
		args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
		if !copyVideo(info) && encoder == "h264_nvenc" {
			args = append(args, "-hwaccel", "auto")
		}
		args = append(args, "-i", path, "-map", fmt.Sprintf("0:%d", info.Stream), "-map", "0:a:0?", "-sn", "-dn", "-map_metadata", "-1")
		if copyVideo(info) || mode == "stream" {
			args = append(args, "-c:v", "copy")
		} else {
			args = append(args, "-c:v", encoder, "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw*sar/2)*2:trunc(ih/2)*2,setsar=1")
			if encoder == "h264_nvenc" {
				args = append(args, "-preset", "p7", "-tune", "hq", "-rc", "vbr", "-cq", "19", "-b:v", "22M", "-maxrate", "30M", "-bufsize", "60M", "-profile:v", "high")
			} else {
				args = append(args, "-preset", "medium", "-crf", "19")
			}
		}
		if copyAudio(info) {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2")
		}
		args = append(args, "-movflags", "+faststart", "-f", "mp4", v.Path)
		if err := runFFmpeg(ctx, args...); err != nil {
			return nil, err
		}
		v.Info, err = ProbeVideo(ctx, v.Path)
		if err != nil {
			return nil, err
		}
		if (mode == "compatible" && !copyVideo(v.Info)) || !copyAudio(v.Info) || !v.Info.Streaming || (mode == "stream" && v.Info.Codec != info.Codec) {
			return nil, errors.New("converted video failed compatibility verification")
		}
	}
	// A real thumbnail makes the video card visible in Telegram clients.
	v.Thumbnail = filepath.Join(v.Dir, "thumb.jpg")
	if err := runFFmpeg(ctx, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-ss", fmt.Sprint(math.Min(30, v.Info.Duration.Seconds()/10)), "-i", v.Path, "-map", fmt.Sprintf("0:%d", v.Info.Stream), "-frames:v", "1", "-vf", "scale=320:320:force_original_aspect_ratio=decrease", "-q:v", "5", "-update", "1", v.Thumbnail); err != nil {
		return nil, fmt.Errorf("video thumbnail: %w", err)
	}
	return v, nil
}

func runFFmpeg(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, ffmpegTool("ffmpeg"), args...)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func ffmpegTool(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	// Windows package-manager links may be missing from an inherited PATH.
	// Also allow portable FFmpeg beside tdl.exe without changing system PATH.
	if runtime.GOOS == "windows" {
		exe, _ := os.Executable()
		for _, dir := range []string{filepath.Dir(exe), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links")} {
			path := filepath.Join(dir, name+".exe")
			if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
				return path
			}
		}
	}
	return name
}
