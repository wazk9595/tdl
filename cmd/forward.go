package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/spf13/cobra"

	"github.com/iyear/tdl/app/forward"
	"github.com/iyear/tdl/core/forwarder"
	"github.com/iyear/tdl/core/logctx"
	"github.com/iyear/tdl/core/storage"
)

func NewForward() *cobra.Command {
	var opts forward.Options

	cmd := &cobra.Command{
		Use:     "forward",
		Short:   "按 ID 范围、消息链接或导出文件批量转发",
		GroupID: groupTools.ID,
		Args:    cobra.NoArgs,
		Example: "tdl forward --from me --ids 100-160 --to https://t.me/c/1234567890/42\ntdl forward --from me --ids 100,160 --to me\ntdl forward --from me --ids 100-160 --exclude 150 --to https://t.me/c/1234567890/42 --mode clone",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := forward.ValidateSelection(opts); err != nil {
				return err
			}
			return tRun(cmd.Context(), func(ctx context.Context, c *telegram.Client, kvd storage.Storage) error {
				return forward.Run(logctx.Named(ctx, "forward"), c, kvd, opts)
			})
		},
	}

	cmd.Flags().StringArrayVar(&opts.From, "from", []string{}, "来源消息链接或导出 JSON；配合 --ids 可用 me（收藏夹）、群 ID 或用户名")
	cmd.Flags().StringVar(&opts.IDs, "ids", "", "消息 ID 或闭区间，可混合：100-160、100,160、100-120,150")
	cmd.Flags().StringVar(&opts.Exclude, "exclude", "", "排除的 ID 或范围，例如 150,155-158；默认跳过涉及的整组相册")
	cmd.Flags().StringVar(&opts.To, "to", "", "目标话题链接、聊天 ID、me 或消息路由表达式")
	cmd.Flags().StringVar(&opts.Edit, "edit", "", "edit message or caption with expression engine. Empty means no edit")
	cmd.Flags().Var(&opts.Mode, "mode", fmt.Sprintf("forward mode: [%s]", strings.Join(forwarder.ModeNames(), ", ")))
	cmd.Flags().BoolVar(&opts.Silent, "silent", false, "send messages silently")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "do not actually send messages, just show how they would be sent")
	cmd.Flags().BoolVar(&opts.Single, "single", false, "do not automatically detect and forward grouped messages")
	cmd.Flags().BoolVar(&opts.Desc, "desc", false, "forward messages in reverse order for each input peer")

	return cmd
}
