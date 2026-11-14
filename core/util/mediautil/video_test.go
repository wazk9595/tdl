package mediautil

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeMetadata(t *testing.T) {
	info, err := parseVideoInfo([]byte(`{"streams":[{"index":0,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}},{"index":1,"codec_type":"audio","codec_name":"aac","channels":2},{"index":2,"codec_type":"video","codec_name":"hevc","width":1920,"height":1080,"pix_fmt":"yuv420p10le","duration":"N/A","side_data_list":[{"rotation":-90}]}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"2.125"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if info.Stream != 2 || info.Codec != "hevc" || info.Width != 1080 || info.Height != 1920 || info.Duration != 2125*time.Millisecond || !info.HasAudio {
		t.Fatalf("wrong metadata: %+v", info)
	}
	for _, bad := range []string{`{"streams":[{"codec_type":"video","width":0,"height":720}]}`, `{"streams":[{"codec_type":"video","width":1280,"height":720,"duration":"NaN"}]}`, `not json`} {
		if _, err := parseVideoInfo([]byte(bad)); err == nil {
			t.Fatal("accepted invalid metadata")
		}
	}
	if _, err := parseVideoInfo([]byte(`{"streams":[{"codec_type":"audio","codec_name":"aac"}]}`)); !errors.Is(err, ErrNoVideo) {
		t.Fatalf("audio-only: %v", err)
	}
}

// Run with TDL_VIDEO_FIXTURES=<directory>. These are real FFmpeg-generated
// containers/codecs; converted output must decode and the source must survive.
func TestVideoCompatibility(t *testing.T) {
	dir := os.Getenv("TDL_VIDEO_FIXTURES")
	if dir == "" {
		t.Skip("set TDL_VIDEO_FIXTURES to run FFmpeg integration cases")
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.IsDir() || !IsVideoFile(file.Name(), "") {
			continue
		}
		t.Run(file.Name(), func(t *testing.T) {
			path := filepath.Join(dir, file.Name())
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			video, err := PrepareVideo(ctx, path, "compatible", "h264_nvenc")
			if err != nil {
				t.Fatal(err)
			}
			temp := video.Dir
			defer video.Close()
			if video.Info.Codec != "h264" || video.Info.Width <= 0 || video.Info.Height <= 0 || !video.Info.Streaming || (video.Info.HasAudio && video.Info.AudioCodec != "aac") {
				t.Fatalf("incompatible output: %+v", video.Info)
			}
			if file.Name() == "11-rotated.mp4" && (video.Info.Width != 360 || video.Info.Height != 640) {
				t.Fatalf("lost rotation: %+v", video.Info)
			}
			if thumb, err := os.Stat(video.Thumbnail); err != nil || thumb.Size() == 0 {
				t.Fatalf("missing thumbnail: %v", err)
			}
			cmd := exec.CommandContext(ctx, ffmpegTool("ffmpeg"), "-v", "error", "-xerror", "-i", video.Path, "-f", "null", "-")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("decode: %v: %s", err, out)
			}
			if err := video.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(temp); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("temporary directory not cleaned")
			}
			after, err := os.Stat(path)
			if err != nil || after.Size() != before.Size() {
				t.Fatal("source changed")
			}
		})
	}
}
