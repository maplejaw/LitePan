package subtitlealign

import (
	"path/filepath"
	"sort"
	"strings"
)

// 动作类型。
const (
	KindLink     = "link"     // 将创建别名
	KindExists   = "exists"   // 别名已存在且与源字幕一致
	KindSkip     = "skip"     // 无需处理：视频本体，别名与源字幕同名
	KindConflict = "conflict" // 目标已存在且不是同一份内容
	KindError    = "error"    // 执行或扫描错误
)

// 当前只支持 STRM 媒体库。
const videoExt = ".strm"

// DefaultSubtitleExts 是默认参与对齐的字幕扩展名（小写，含点）。
var DefaultSubtitleExts = []string{".srt", ".ass", ".ssa", ".vtt", ".sub", ".smi"}

// Action 描述一条待执行或已执行的对齐动作。
type Action struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Dir    string `json:"dir"`
	Video  string `json:"video"`
	Source string `json:"source"`
	Alias  string `json:"alias"`
	Mode   string `json:"mode,omitempty"`
	Reason string `json:"reason,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// WorkName 返回目录名，作为这个目录对应作品的名称。
func WorkName(dir string) string {
	return filepath.Base(filepath.Clean(dir))
}

// BuildActions 根据一个目录内的文件名生成对齐动作。
// names 必须只包含直接子文件的文件名（不含目录），顺序不影响结果。
//
// 规则：
//  1. 源字幕：去掉扩展名后与目录名相等（忽略大小写）。
//  2. 版本视频：扩展名为 .strm，且去掉扩展名后以目录名开头（忽略大小写）。
//  3. 别名：视频名 + 源字幕扩展名。
func BuildActions(dir string, names []string, subExts []string) []Action {
	work := WorkName(dir)
	if work == "" || work == "." || work == string(filepath.Separator) {
		return nil
	}

	exts := normalizeExts(subExts)
	var sources []string
	for _, name := range names {
		if isSourceSubtitle(name, work, exts) {
			sources = append(sources, name)
		}
	}
	if len(sources) == 0 {
		return nil
	}
	sort.Strings(sources)

	var actions []Action
	claimed := make(map[string]bool)
	for _, name := range names {
		if !isVersionVideo(name, work) {
			continue
		}
		stem := stemOf(name)
		for _, src := range sources {
			alias := stem + filepath.Ext(src)
			if strings.EqualFold(alias, src) {
				actions = append(actions, Action{
					Kind:   KindSkip,
					Dir:    dir,
					Video:  name,
					Source: src,
					Alias:  alias,
					Reason: "视频本体，别名与源字幕同名",
				})
				continue
			}
			if claimed[alias] {
				continue
			}
			claimed[alias] = true
			actions = append(actions, Action{
				Kind:   KindLink,
				Dir:    dir,
				Video:  name,
				Source: src,
				Alias:  alias,
			})
		}
	}

	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].Video != actions[j].Video {
			return actions[i].Video < actions[j].Video
		}
		return actions[i].Source < actions[j].Source
	})
	return actions
}

func isSourceSubtitle(name, work string, exts map[string]bool) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if !exts[ext] {
		return false
	}
	return strings.EqualFold(stemOf(name), work)
}

func isVersionVideo(name, work string) bool {
	if !strings.EqualFold(filepath.Ext(name), videoExt) {
		return false
	}
	return prefixedBy(stemOf(name), work)
}

// prefixedBy 判断 name 是否以 folder 开头（忽略大小写）。
func prefixedBy(name, folder string) bool {
	if len(name) < len(folder) {
		return false
	}
	return strings.EqualFold(name[:len(folder)], folder)
}

func stemOf(name string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}

func normalizeExts(exts []string) map[string]bool {
	if len(exts) == 0 {
		exts = DefaultSubtitleExts
	}
	out := make(map[string]bool, len(exts))
	for _, ext := range exts {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		out[ext] = true
	}
	return out
}
