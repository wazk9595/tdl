package send

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/spf13/viper"

	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/core/tclient"
	"github.com/iyear/tdl/pkg/consts"
)

type Options struct {
	Folder, To, TaskDir, Encoder                                                string
	DryRun, JSON, AllowTranscode, OriginalImages, ResendMissing, RetryUncertain bool
	Retries                                                                     int
}

func Run(ctx context.Context, c *telegram.Client, kvd storage.Storage, opts Options, out io.Writer) (rerr error) {
	var outputMu sync.Mutex
	emit := func(format string, args ...any) {
		if !opts.JSON {
			outputMu.Lock()
			defer outputMu.Unlock()
			_, _ = fmt.Fprintf(out, format+"\n", args...)
		}
	}
	target, err := ParseTarget(opts.To)
	if err != nil {
		return err
	}
	pool := dcpool.NewPool(c, int64(viper.GetInt(consts.FlagPoolSize)), tclient.NewDefaultMiddlewares(ctx, viper.GetDuration(consts.FlagReconnectTimeout))...)
	defer func() {
		if err := pool.Close(); rerr == nil {
			rerr = err
		}
	}()
	client := pool.Default(ctx)
	manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(client)
	backend, err := Resolve(ctx, client, manager, target)
	if err != nil {
		return err
	}
	backend.AllowTranscode = opts.AllowTranscode
	backend.Encoder = opts.Encoder
	backend.Threads = viper.GetInt(consts.FlagThreads)
	backend.OriginalImages = opts.OriginalImages
	emit("目标：%s", backend.Destination.Name)
	dir := opts.TaskDir
	if dir == "" {
		dir = filepath.Join(consts.DataDir, "tasks")
	}
	root, err := CanonicalRoot(opts.Folder)
	if err != nil {
		return err
	}
	dir, err = taskDirectory(root, dir)
	if err != nil {
		return err
	}
	emit("正在扫描文件并核对内容哈希……")
	scan, err := ScanFolder(ctx, root)
	if err != nil {
		return err
	}
	emit("扫描 %d 个文件，重复跳过 %d 个，唯一文件 %d 个，忽略 %d 项", scan.Count, scan.Duplicates, len(scan.Files), scan.Ignored)
	ns := viper.GetString(consts.FlagNamespace)
	task := Task{Version: 1, Root: scan.Root, Namespace: ns, AccountID: backend.AccountID, Destination: backend.Destination, Created: time.Now(), Entries: map[string]*Entry{}}
	task.ID = TaskID(task.Root, ns, task.AccountID, task.Destination)
	_, statErr := os.Stat(filepath.Join(dir, task.ID+".db"))
	if statErr == nil {
		emit("发现已有任务记录；将重新核对已发送消息，继续未完成部分。")
	} else if os.IsNotExist(statErr) {
		emit("首次任务：没有既往发送记录，不会仅凭远端同名文件跳过。")
	} else {
		return statErr
	}
	if opts.DryRun {
		summary := Summary{TaskID: task.ID, Destination: task.Destination.Name, Scanned: scan.Count, Duplicates: scan.Duplicates, Ignored: scan.Ignored, Expected: len(scan.Files), Issues: scan.Errors}
		if opts.JSON {
			if err := json.NewEncoder(out).Encode(summary); err != nil {
				return err
			}
		} else {
			emit("预览结束，没有上传，也没有写入任务记录。")
			for _, issue := range scan.Errors {
				emit("扫描问题：%s", issue)
			}
		}
		if len(scan.Errors) > 0 {
			return fmt.Errorf("存在 %d 个扫描问题", len(scan.Errors))
		}
		return nil
	}
	store, err := OpenStore(dir, task)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); rerr == nil {
			rerr = err
		}
	}()
	if err := backend.CheckExisting(ctx, store.Snapshot(), scan); err != nil {
		return err
	}
	summary, runErr := Execute(ctx, store, scan, backend, EngineOptions{Concurrency: viper.GetInt(consts.FlagLimit), Retries: opts.Retries, ResendMissing: opts.ResendMissing, RetryUncertain: opts.RetryUncertain, Emit: emit})
	if opts.JSON {
		if err := json.NewEncoder(out).Encode(summary); err != nil {
			return err
		}
	} else {
		emit("核对通过：%d/%d（视频 %d，照片 %d，其他文件 %d）", summary.Verified, summary.Expected, summary.Videos, summary.Photos, summary.Documents)
		emit("重复跳过：%d；转码文件：%d；待处理：%d", summary.Duplicates, summary.Transcoded, summary.Unresolved)
		for _, issue := range summary.Issues {
			emit("待处理：%s", issue)
		}
		emit("原文件保留。再次运行同一命令可核对并继续。")
	}
	return runErr
}

// Keep journal files outside the input tree, including through directory links.
func taskDirectory(root, dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	parent := abs
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			abs = resolved
			break
		}
		if !os.IsNotExist(err) || filepath.Dir(parent) == parent {
			return "", err
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
	rel, err := filepath.Rel(root, abs)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("任务记录目录必须位于待上传文件夹之外，请调整 --task-dir 或选择更具体的上传文件夹")
	}
	return abs, nil
}
