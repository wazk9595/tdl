package forward

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	pw "github.com/jedib0t/go-pretty/v6/progress"
	"github.com/spf13/viper"
	"go.uber.org/multierr"

	"github.com/iyear/tdl/app/internal/tctx"
	"github.com/iyear/tdl/app/send"
	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/forwarder"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/core/tclient"
	"github.com/iyear/tdl/core/util/tutil"
	"github.com/iyear/tdl/pkg/consts"
	"github.com/iyear/tdl/pkg/prog"
	"github.com/iyear/tdl/pkg/texpr"
	"github.com/iyear/tdl/pkg/tmessage"
)

type Options struct {
	From    []string
	To      string
	Edit    string
	Mode    forwarder.Mode
	Silent  bool
	DryRun  bool
	Single  bool
	Desc    bool
	IDs     string
	Exclude string
}

func Run(ctx context.Context, c *telegram.Client, kvd storage.Storage, opts Options) (rerr error) {
	selection, err := ValidateSelection(opts)
	if err != nil {
		return err
	}
	if opts.To == "-" || opts.Edit == "-" {
		fg := texpr.NewFieldsGetter(nil)

		fields, err := fg.Walk(exprEnv(nil, nil))
		if err != nil {
			return fmt.Errorf("failed to walk fields: %w", err)
		}

		fmt.Print(fg.Sprint(fields, true))
		return nil
	}

	ctx = tctx.WithKV(ctx, kvd)

	pool := dcpool.NewPool(c,
		int64(viper.GetInt(consts.FlagPoolSize)),
		tclient.NewDefaultMiddlewares(ctx, viper.GetDuration(consts.FlagReconnectTimeout))...)
	defer multierr.AppendInvoke(&rerr, multierr.Close(pool))

	ctx = tctx.WithPool(ctx, pool)

	manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(pool.Default(ctx))
	var to *vm.Program
	if selection != nil {
		to, err = resolveDest(ctx, manager, opts.To)
		if err != nil {
			return errors.Wrap(err, "resolve dest peer")
		}
	}
	var dialogs []*tmessage.Dialog
	var batch *batchPlan
	if selection != nil {
		fmt.Println("正在读取指定 ID 范围并整理媒体组……")
		batch, dialogs, err = collectSelected(ctx, pool.Default(ctx), manager, opts, selection)
	} else {
		dialogs, err = collectDialogs(ctx, opts.From, opts.Desc)
	}
	if err != nil {
		return errors.Wrap(err, "collect dialogs")
	}
	if selection == nil {
		to, err = resolveDest(ctx, manager, opts.To)
		if err != nil {
			return errors.Wrap(err, "resolve dest peer")
		}
	}

	if batch != nil {
		fmt.Printf("来源：%s；匹配 %d 条，排除及整组跳过 %d 条（跳过 %d 组），整组补齐 %d 条；本次共 %d 条\n", batch.Source.VisibleName(), batch.Matched, batch.Skipped, batch.SkippedGroups, batch.Expanded, batch.Total)
		for _, id := range batch.IDs {
			if group := batch.Groups[id]; len(group) > 0 {
				ids := make([]string, 0, len(group))
				for _, msg := range group {
					ids = append(ids, strconv.Itoa(msg.ID))
				}
				fmt.Printf("媒体组：%s\n", strings.Join(ids, ","))
			} else {
				fmt.Printf("消息：%d\n", id)
			}
		}
		if batch.Total == 0 {
			fmt.Println("没有需要转发的消息，没有发送。")
			return nil
		}
	}

	edit, err := resolveEdit(opts.Edit)
	if err != nil {
		return errors.Wrap(err, "resolve edit")
	}

	fwProgress := prog.New(pw.FormatNumber)
	fwProgress.SetNumTrackersExpected(totalMessages(dialogs))
	if !viper.GetBool(consts.FlagDisableProgressPS) {
		prog.EnablePS(ctx, fwProgress)
	}

	progress := newProgress(fwProgress)
	fw := forwarder.New(forwarder.Options{
		Pool: pool,
		Iter: newIter(iterOptions{
			manager: manager,
			pool:    pool,
			to:      to,
			edit:    edit,
			dialogs: dialogs,
			batch:   batch,
			mode:    opts.Mode,
			silent:  opts.Silent,
			dryRun:  opts.DryRun,
			grouped: !opts.Single,
			delay:   viper.GetDuration(consts.FlagDelay),
		}),
		Progress: progress,
		Threads:  viper.GetInt(consts.FlagThreads),
	})

	go fwProgress.Render()
	defer prog.Wait(ctx, fwProgress)

	err = fw.Forward(ctx)
	if batch != nil {
		if opts.DryRun {
			fmt.Printf("预览结束：计划 %d 条，没有发送。\n", batch.Total)
		} else {
			fmt.Printf("本次转发：成功 %d 条，失败 %d 条。\n", progress.succeeded, progress.failed)
		}
	}
	return err
}

func collectDialogs(ctx context.Context, input []string, desc bool) ([]*tmessage.Dialog, error) {
	var dialogs []*tmessage.Dialog

	for _, p := range input {
		var (
			d   []*tmessage.Dialog
			err error
		)

		switch {
		case strings.HasPrefix(p, "http"):
			d, err = tmessage.Parse(tmessage.FromURL(ctx, tctx.Pool(ctx), tctx.KV(ctx), []string{p}))
			if err != nil {
				return nil, errors.Wrap(err, "parse from url")
			}
		default:
			d, err = tmessage.Parse(tmessage.FromFile(ctx, tctx.Pool(ctx), tctx.KV(ctx), []string{p}, false))
			if err != nil {
				return nil, errors.Wrap(err, "parse from file")
			}
		}

		if desc {
			for _, dd := range d {
				for i, j := 0, len(dd.Messages)-1; i < j; i, j = i+1, j-1 {
					dd.Messages[i], dd.Messages[j] = dd.Messages[j], dd.Messages[i]
				}
			}
		}

		dialogs = append(dialogs, d...)
	}

	return dialogs, nil
}

// resolveDest parses the input string and returns a vm.Program. It can be a CHAT, a text or a file based on expression engine.
func resolveDest(ctx context.Context, manager *peers.Manager, input string) (*vm.Program, error) {
	compile := func(i string) (*vm.Program, error) {
		// we pass empty peer and message to enable type checking
		return expr.Compile(i, expr.Env(exprEnv(nil, nil)))
	}

	// default
	if input == "" || isSaved(input) {
		return compile(`""`)
	}
	if strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://") {
		target, err := send.ParseTarget(input)
		if err != nil {
			return nil, err
		}
		resolved, err := send.Resolve(ctx, tctx.Pool(ctx).Default(ctx), manager, target)
		if err != nil {
			return nil, err
		}
		fmt.Printf("目标：%s\n", resolved.Destination.Name)
		return compile(fmt.Sprintf(`{Peer: %q, Thread: %d}`, strconv.FormatInt(resolved.Destination.PeerID, 10), resolved.Destination.Topic))
	}

	// file
	if exp, err := os.ReadFile(input); err == nil {
		return compile(string(exp))
	}

	// chat
	if _, err := tutil.GetInputPeer(ctx, manager, input); err == nil {
		// convert to const string
		return compile(fmt.Sprintf(`"%s"`, input))
	}

	// text
	return compile(input)
}

// resolveEdit returns nil if input is empty, otherwise it returns a vm.Program. It can be a text or a file based on expression engine.
func resolveEdit(input string) (*vm.Program, error) {
	compile := func(i string) (*vm.Program, error) {
		// we pass empty peer and message to enable type checking
		return expr.Compile(i, expr.Env(exprEnv(nil, nil)), expr.AsKind(reflect.String))
	}

	// no edit, nil program
	if input == "" {
		return nil, nil
	}

	// file
	if exp, err := os.ReadFile(input); err == nil {
		return compile(string(exp))
	}

	// text
	return compile(input)
}

func totalMessages(dialogs []*tmessage.Dialog) int {
	var total int
	for _, d := range dialogs {
		total += len(d.Messages)
	}
	return total
}
