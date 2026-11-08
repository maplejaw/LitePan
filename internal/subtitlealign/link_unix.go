//go:build !windows

package subtitlealign

import (
	"errors"
	"syscall"
)

// isFallbackErr 只对"这个文件系统做不到硬链接"的情况降级，
// 权限不足等错误必须原样暴露。
func isFallbackErr(err error) bool {
	return errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP)
}
