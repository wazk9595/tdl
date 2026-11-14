package send

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"github.com/iyear/tdl/core/util/mediautil"
)

func CanonicalRoot(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	stat, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("请选择文件夹：%s", root)
	}
	return filepath.Clean(root), nil
}

func TaskID(root, namespace string, account int64, dest Destination) string {
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	value := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%d", root, namespace, account, dest.PeerID, dest.Topic)
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

func HashFile(ctx context.Context, path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	buffer := make([]byte, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		n, err := f.Read(buffer)
		if n > 0 {
			_, _ = h.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", 0, err
		}
	}
	after, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", 0, fmt.Errorf("文件在读取时发生变化：%s", path)
	}
	return hex.EncodeToString(h.Sum(nil)), after.Size(), nil
}

func ScanFolder(ctx context.Context, root string) (Scan, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return Scan{}, err
	}
	scan := Scan{Root: canonical}
	seen := map[string]bool{}
	err = filepath.WalkDir(canonical, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			scan.Errors = append(scan.Errors, fmt.Sprintf("%s: %v", path, walkErr))
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			scan.Ignored++
			return nil
		}
		if d.IsDir() {
			if path != canonical && (d.Name() == ".git" || d.Name() == ".tdl") {
				scan.Ignored++
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		extension := strings.ToLower(filepath.Ext(name))
		if name == ".ds_store" || name == "thumbs.db" || name == "desktop.ini" || extension == ".thumb" || extension == ".part" || extension == ".crdownload" {
			scan.Ignored++
			return nil
		}
		info, err := d.Info()
		if err != nil {
			scan.Errors = append(scan.Errors, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		if !info.Mode().IsRegular() {
			scan.Ignored++
			return nil
		}
		scan.Count++
		hash, size, err := HashFile(ctx, path)
		if err != nil {
			scan.Errors = append(scan.Errors, err.Error())
			return ctx.Err()
		}
		if seen[hash] {
			scan.Duplicates++
			return nil
		}
		seen[hash] = true
		mime, err := mimetype.DetectFile(path)
		if err != nil {
			scan.Errors = append(scan.Errors, err.Error())
			return nil
		}
		kind := kindDocument
		if mediautil.IsVideoFile(path, mime.String()) {
			kind = kindVideo
		} else if mediautil.IsImage(mime.String()) && mime.String() != "image/webp" && mime.String() != "image/gif" {
			kind = kindPhoto
		}
		scan.Files = append(scan.Files, File{Path: path, Name: d.Name(), Hash: hash, Size: size, Kind: kind})
		return nil
	})
	return scan, err
}
