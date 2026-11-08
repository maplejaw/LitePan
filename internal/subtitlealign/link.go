package subtitlealign

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	ModeHardlink = "hardlink"
	ModeCopy     = "copy"
)

// linkWithFallback 创建别名，返回实际使用的方式。
// 硬链接遇到跨文件系统/不支持的情况时降级为复制；权限等错误不降级。
func linkWithFallback(source, alias, mode string, fallback bool) (string, error) {
	if mode == "" {
		mode = ModeHardlink
	}
	switch mode {
	case ModeCopy:
		if err := copyFile(source, alias); err != nil {
			return "", err
		}
		return ModeCopy, nil
	case ModeHardlink:
		err := os.Link(source, alias)
		if err == nil {
			return ModeHardlink, nil
		}
		if errors.Is(err, fs.ErrExist) || !fallback || !isFallbackErr(err) {
			return "", err
		}
		if cerr := copyFile(source, alias); cerr != nil {
			return "", cerr
		}
		return ModeCopy, nil
	default:
		return "", fmt.Errorf("不支持的链接方式: %s", mode)
	}
}

// copyFile 先写临时文件再原子改名，避免出现半截文件。
func copyFile(source, alias string) error {
	if _, err := os.Stat(alias); err == nil {
		return fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dir := filepath.Dir(alias)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(alias)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, alias)
}

// sameFile 判断两个路径是否指向同一个 inode（硬链接）。
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// filesEqual 按大小和内容比较。字幕文件很小，直接读全量。
func filesEqual(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil || ai.Size() != bi.Size() {
		return false
	}
	ab, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return string(ab) == string(bb)
}
