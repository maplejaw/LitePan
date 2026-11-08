//go:build windows

package subtitlealign

// 目标部署环境是群晖（Linux），Windows 仅用于本地开发，不做降级判断。
func isFallbackErr(err error) bool {
	return false
}
