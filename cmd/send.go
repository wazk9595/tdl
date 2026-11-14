package cmd

import (
	"context"
	"fmt"

	"github.com/gotd/td/telegram"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/iyear/tdl/app/send"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/pkg/consts"
)

func NewSend() *cobra.Command {
	var opts send.Options
	cmd := &cobra.Command{
		Use: "send FOLDER --to TOPIC_LINK", Short: "按话题链接上传文件夹，自动去重、记录进度并核对结果", GroupID: groupTools.ID, Args: cobra.ExactArgs(1),
		Example: "tdl send \"D:\\Videos\" --to https://t.me/c/1234567890/42\ntdl send ./videos --to me --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := send.ParseTarget(opts.To); err != nil {
				return err
			}
			root, err := send.CanonicalRoot(args[0])
			if err != nil {
				return err
			}
			opts.Folder = root
			if opts.Retries < 0 || opts.Retries > 5 {
				return fmt.Errorf("--retries 必须在 0 到 5 之间")
			}
			if opts.Encoder != "auto" && opts.Encoder != "h264_nvenc" && opts.Encoder != "libx264" {
				return fmt.Errorf("--video-encoder 请选择 auto、h264_nvenc 或 libx264")
			}
			if viper.GetInt(consts.FlagThreads) < 1 || viper.GetInt(consts.FlagLimit) < 1 {
				return fmt.Errorf("--threads 和 --limit 必须大于 0")
			}
			return tRun(cmd.Context(), func(ctx context.Context, c *telegram.Client, kvd storage.Storage) error {
				return send.Run(ctx, c, kvd, opts, cmd.OutOrStdout())
			})
		},
	}
	cmd.Flags().StringVar(&opts.To, "to", "", "Telegram 话题链接；me 表示自己的收藏夹")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "仅检查目标和扫描文件，不上传或写入任务记录")
	cmd.Flags().BoolVar(&opts.AllowTranscode, "allow-transcode", false, "明确允许将不兼容的视频转换为 H.264/AAC")
	cmd.Flags().StringVar(&opts.Encoder, "video-encoder", "auto", "转码编码器：auto、h264_nvenc 或 libx264")
	cmd.Flags().BoolVar(&opts.OriginalImages, "original-images", false, "图片按原始文件发送，保留原图字节")
	cmd.Flags().IntVar(&opts.Retries, "retries", 2, "发送前失败的自动重试次数；发送结果不明时先核对")
	cmd.Flags().BoolVar(&opts.ResendMissing, "resend-missing", false, "明确允许重新发送已从 Telegram 删除的任务消息")
	cmd.Flags().BoolVar(&opts.RetryUncertain, "retry-uncertain", false, "核对后仍找不到回执时，允许使用原消息编号重试发送")
	cmd.Flags().StringVar(&opts.TaskDir, "task-dir", "", "任务记录目录，默认保存在用户 .tdl/tasks 下")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "以 JSON 输出最终核对报告")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
