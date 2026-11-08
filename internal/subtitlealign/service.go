package subtitlealign

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"litepan/internal/settings"
)

// 清理阶段的动作类型。
const (
	KindRemove = "remove"
	KindKeep   = "keep"
)

const timeLayout = "2006-01-02 15:04:05"

// skipDirNames 是群晖/Windows 上常见的系统目录，扫描时直接跳过。
var skipDirNames = map[string]bool{
	"@eaDir":                    true,
	"@tmp":                      true,
	"#recycle":                  true,
	".@__thumb":                 true,
	"$RECYCLE.BIN":              true,
	"System Volume Information": true,
}

type Config struct {
	Mode     string   // hardlink | copy
	Fallback bool     // 硬链接不可用时降级为复制
	SubExts  []string // 参与对齐的字幕扩展名
}

type ScanOptions struct {
	MaxDepth int // 0 = 不限；1 = 根目录及其直接子目录
}

type Plan struct {
	ID        string   `json:"id"`
	Root      string   `json:"root"`
	CreatedAt string   `json:"created_at"`
	Actions   []Action `json:"actions"`
}

type Options struct {
	Settings *settings.Service
	Log      *slog.Logger
	// 以下字段仅在未注入 Settings 时作为静态配置，测试用。
	Mode     string
	Fallback bool
	SubExts  []string
}

type Service struct {
	settings *settings.Service
	log      *slog.Logger
	static   Config
}

func New(opts Options) *Service {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		settings: opts.Settings,
		log:      log,
		static:   Config{Mode: opts.Mode, Fallback: opts.Fallback, SubExts: opts.SubExts},
	}
}

// Enabled 返回字幕对齐总开关；未注入设置服务时默认开启。
func (s *Service) Enabled() bool {
	if s == nil || s.settings == nil {
		return true
	}
	return s.settings.Bool(settings.KeySubtitleAlignEnabled)
}

// Settings 是面板读写的一组配置。
type Settings struct {
	Enabled  bool   `json:"enabled"`
	Mode     string `json:"mode"`
	Fallback bool   `json:"fallback"`
	SubExts  string `json:"sub_exts"`
}

func (s *Service) GetSettings() Settings {
	cfg := s.config()
	return Settings{
		Enabled:  s.Enabled(),
		Mode:     cfg.Mode,
		Fallback: cfg.Fallback,
		SubExts:  strings.Join(cfg.SubExts, ","),
	}
}

func (s *Service) UpdateSettings(ctx context.Context, in Settings) error {
	if s.settings == nil {
		return errors.New("设置服务未就绪")
	}
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		mode = ModeHardlink
	}
	if mode != ModeHardlink && mode != ModeCopy {
		return fmt.Errorf("不支持的链接方式: %s", mode)
	}
	exts := splitExts(in.SubExts)
	if len(exts) == 0 {
		exts = DefaultSubtitleExts
	}
	return s.settings.Update(ctx, map[string]string{
		settings.KeySubtitleAlignEnabled:  strconv.FormatBool(in.Enabled),
		settings.KeySubtitleAlignMode:     mode,
		settings.KeySubtitleAlignFallback: strconv.FormatBool(in.Fallback),
		settings.KeySubtitleAlignSubExts:  strings.Join(exts, ","),
	})
}

// config 每次调用时读取最新设置，改配置后无需重启服务。
func (s *Service) config() Config {
	cfg := s.static
	if s.settings != nil {
		if mode := strings.TrimSpace(s.settings.String(settings.KeySubtitleAlignMode)); mode != "" {
			cfg.Mode = mode
		}
		cfg.Fallback = s.settings.Bool(settings.KeySubtitleAlignFallback)
		if raw := strings.TrimSpace(s.settings.String(settings.KeySubtitleAlignSubExts)); raw != "" {
			cfg.SubExts = splitExts(raw)
		}
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeHardlink
	}
	if len(cfg.SubExts) == 0 {
		cfg.SubExts = DefaultSubtitleExts
	}
	return cfg
}

func splitExts(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '，' || r == '；' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Scan 只读扫描，不修改任何文件。
func (s *Service) Scan(ctx context.Context, root string, opts ScanOptions) (*Plan, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("不是目录: %s", root)
	}

	cfg := s.config()
	plan := newPlan(root)
	seq := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if rel != "." {
			if strings.HasPrefix(d.Name(), ".") || skipDirNames[d.Name()] {
				return filepath.SkipDir
			}
			if opts.MaxDepth > 0 && pathDepth(rel) > opts.MaxDepth {
				return filepath.SkipDir
			}
		}

		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return nil
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		for _, a := range BuildActions(path, names, cfg.SubExts) {
			seq++
			a.ID = fmt.Sprintf("a%d", seq)
			s.classify(&a)
			plan.Actions = append(plan.Actions, a)
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, context.Canceled) {
		return nil, walkErr
	}
	return plan, nil
}

// classify 在扫描阶段确认别名是否已存在，避免重复应用时把已完成项显示成待创建。
// Apply 仍会再检查一次，覆盖扫描与执行之间的变化。
func (s *Service) classify(a *Action) {
	if a.Kind != KindLink {
		return
	}
	src := filepath.Join(a.Dir, a.Source)
	dst := filepath.Join(a.Dir, a.Alias)
	if _, err := os.Stat(dst); err != nil {
		return
	}
	switch {
	case sameFile(src, dst):
		a.Kind = KindExists
		a.Mode = ModeHardlink
		a.Reason = "别名已存在（同一 inode）"
	case filesEqual(src, dst):
		a.Kind = KindExists
		a.Mode = ModeCopy
		a.Reason = "别名已存在且内容一致"
	default:
		a.Kind = KindConflict
		a.Reason = "别名已存在且内容不同，未覆盖"
	}
}

// Apply 按计划创建别名，并把成功的条目写入清单。
func (s *Service) Apply(ctx context.Context, plan *Plan) error {
	if plan == nil {
		return errors.New("计划为空")
	}
	if plan.Root == "" {
		return errors.New("计划缺少根目录")
	}
	cfg := s.config()
	manifest, err := readManifest(plan.Root)
	if err != nil {
		return fmt.Errorf("读取清单失败: %w", err)
	}
	index := make(map[string]int, len(manifest.Entries))
	for i, e := range manifest.Entries {
		index[e.Alias] = i
	}

	for i := range plan.Actions {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		a := &plan.Actions[i]
		if a.Kind != KindLink {
			a.Status = "skipped"
			continue
		}
		if err := validateAction(plan.Root, a); err != nil {
			a.Kind = KindError
			a.Status = "failed"
			a.Error = err.Error()
			continue
		}
		src := filepath.Join(a.Dir, a.Source)
		dst := filepath.Join(a.Dir, a.Alias)

		if _, statErr := os.Stat(dst); statErr == nil {
			switch {
			case sameFile(src, dst):
				a.Status = "skipped"
				a.Mode = ModeHardlink
				a.Reason = "别名已存在（同一 inode）"
			case filesEqual(src, dst):
				a.Status = "skipped"
				a.Mode = ModeCopy
				a.Reason = "别名已存在且内容一致"
			default:
				a.Kind = KindConflict
				a.Status = "skipped"
				a.Reason = "别名已存在且内容不同，未覆盖"
			}
			continue
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			a.Kind = KindError
			a.Status = "failed"
			a.Error = statErr.Error()
			continue
		}

		mode, linkErr := linkWithFallback(src, dst, cfg.Mode, cfg.Fallback)
		if linkErr != nil {
			a.Kind = KindError
			a.Status = "failed"
			a.Error = linkErr.Error()
			continue
		}
		a.Mode = mode
		a.Status = "done"

		relAlias, errAlias := filepath.Rel(plan.Root, dst)
		relSource, errSource := filepath.Rel(plan.Root, src)
		if errAlias != nil || errSource != nil {
			continue
		}
		entry := ManifestEntry{
			Alias:     relAlias,
			Source:    relSource,
			Mode:      mode,
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		if info, statErr := os.Stat(src); statErr == nil {
			entry.SourceSize = info.Size()
			entry.SourceMod = info.ModTime().Unix()
		}
		if idx, ok := index[relAlias]; ok {
			manifest.Entries[idx] = entry
		} else {
			index[relAlias] = len(manifest.Entries)
			manifest.Entries = append(manifest.Entries, entry)
		}
	}

	return writeManifest(plan.Root, manifest)
}

// Cleanup 按清单清理本工具创建的别名。dryRun=true 时只检查不删除，等价于校验。
func (s *Service) Cleanup(ctx context.Context, root string, dryRun bool) (*Plan, error) {
	root = filepath.Clean(root)
	manifest, err := readManifest(root)
	if err != nil {
		return nil, fmt.Errorf("读取清单失败: %w", err)
	}

	plan := newPlan(root)
	kept := make([]ManifestEntry, 0, len(manifest.Entries))

	for i, e := range manifest.Entries {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		aliasAbs := filepath.Join(root, e.Alias)
		sourceAbs := filepath.Join(root, e.Source)
		act := Action{
			ID:     fmt.Sprintf("c%d", i+1),
			Dir:    filepath.Dir(e.Alias),
			Source: filepath.Base(e.Source),
			Alias:  filepath.Base(e.Alias),
			Mode:   e.Mode,
		}

		if _, statErr := os.Stat(aliasAbs); errors.Is(statErr, fs.ErrNotExist) {
			act.Kind = KindSkip
			act.Reason = "别名已不存在，从清单移除"
			plan.Actions = append(plan.Actions, act)
			continue
		}

		if _, statErr := os.Stat(sourceAbs); statErr != nil {
			act.Kind = KindKeep
			act.Reason = "源字幕不存在，保留别名"
			kept = append(kept, e)
			plan.Actions = append(plan.Actions, act)
			continue
		}

		keepReason := ""
		if e.Mode == ModeHardlink {
			if !sameFile(sourceAbs, aliasAbs) {
				keepReason = "别名已不是源字幕的硬链接（可能被替换），保留"
			}
		} else if !filesEqual(sourceAbs, aliasAbs) {
			keepReason = "别名内容已被修改，保留"
		}
		if keepReason != "" {
			act.Kind = KindKeep
			act.Reason = keepReason
			kept = append(kept, e)
			plan.Actions = append(plan.Actions, act)
			continue
		}

		act.Kind = KindRemove
		if dryRun {
			act.Status = "pending"
			act.Reason = "校验通过，待删除"
		} else if removeErr := os.Remove(aliasAbs); removeErr != nil {
			act.Kind = KindError
			act.Status = "failed"
			act.Error = removeErr.Error()
			kept = append(kept, e)
		} else {
			act.Status = "done"
			act.Reason = "已删除"
		}
		plan.Actions = append(plan.Actions, act)
	}

	if !dryRun {
		manifest.Entries = kept
		if err := writeManifest(root, manifest); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

func newPlan(root string) *Plan {
	return &Plan{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Root:      root,
		CreatedAt: time.Now().Format(timeLayout),
		Actions:   []Action{},
	}
}

func pathDepth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// validateAction 防止客户端回传的计划里出现越界路径或带分隔符的文件名。
func validateAction(root string, a *Action) error {
	if !pathWithin(root, filepath.Clean(a.Dir)) {
		return fmt.Errorf("目录不在根目录内: %s", a.Dir)
	}
	for _, name := range []string{a.Source, a.Alias} {
		if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
			return fmt.Errorf("非法文件名: %s", name)
		}
	}
	return nil
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
