package subtitlealign

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupMovieDir(t *testing.T) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "movie")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "movie.srt"), "subtitle")
	writeFile(t, filepath.Join(dir, "movie - 4K.strm"), "http://example/4k")
	writeFile(t, filepath.Join(dir, "movie.strm"), "http://example/base")
	return root, dir
}

func findLink(actions []Action, alias string) *Action {
	for i := range actions {
		if actions[i].Alias == alias {
			return &actions[i]
		}
	}
	return nil
}

func TestScanApplyCleanup(t *testing.T) {
	root, dir := setupMovieDir(t)
	ctx := context.Background()
	svc := New(Options{Mode: ModeHardlink, Fallback: true})

	plan, err := svc.Scan(ctx, root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	link := findLink(plan.Actions, "movie - 4K.srt")
	if link == nil || link.Kind != KindLink {
		t.Fatalf("未生成 link 动作: %+v", plan.Actions)
	}

	if err := svc.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if link.Status != "done" || link.Mode != ModeHardlink {
		t.Fatalf("动作未成功: %+v", link)
	}

	alias := filepath.Join(dir, "movie - 4K.srt")
	if !sameFile(filepath.Join(dir, "movie.srt"), alias) {
		t.Fatal("别名不是源字幕的硬链接")
	}
	manifest, err := readManifest(root)
	if err != nil || len(manifest.Entries) != 1 {
		t.Fatalf("清单异常: %v %+v", err, manifest)
	}

	// 重复执行应当幂等：不重复写清单，动作退化为 skipped。
	plan2, err := svc.Scan(ctx, root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(ctx, plan2); err != nil {
		t.Fatal(err)
	}
	again := findLink(plan2.Actions, "movie - 4K.srt")
	if again == nil || again.Kind != KindExists || again.Mode != ModeHardlink {
		t.Fatalf("重复扫描未识别为已存在: %+v", again)
	}
	if again.Status != "skipped" {
		t.Fatalf("重复执行未跳过: %+v", again)
	}
	manifest, _ = readManifest(root)
	if len(manifest.Entries) != 1 {
		t.Fatalf("重复执行后清单条目 = %d, want 1", len(manifest.Entries))
	}

	// 清理：删除别名，保留源字幕。
	cleanupPlan, err := svc.Cleanup(ctx, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(alias); !os.IsNotExist(err) {
		t.Fatalf("别名未被删除: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "movie.srt")); err != nil {
		t.Fatalf("源字幕被误删: %v", err)
	}
	manifest, _ = readManifest(root)
	if len(manifest.Entries) != 0 {
		t.Fatalf("清理后清单条目 = %d, want 0 (%+v)", len(manifest.Entries), cleanupPlan.Actions)
	}
}

func TestApplyDoesNotOverwriteConflict(t *testing.T) {
	root, dir := setupMovieDir(t)
	ctx := context.Background()
	alias := filepath.Join(dir, "movie - 4K.srt")
	writeFile(t, alias, "user's own subtitle")

	svc := New(Options{Mode: ModeHardlink, Fallback: true})
	plan, err := svc.Scan(ctx, root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	act := findLink(plan.Actions, "movie - 4K.srt")
	if act == nil || act.Kind != KindConflict {
		t.Fatalf("未标记冲突: %+v", act)
	}
	data, err := os.ReadFile(alias)
	if err != nil || string(data) != "user's own subtitle" {
		t.Fatalf("冲突文件被覆盖: %q %v", data, err)
	}
	manifest, _ := readManifest(root)
	if len(manifest.Entries) != 0 {
		t.Fatalf("冲突条目不应写入清单: %+v", manifest.Entries)
	}
}

func TestCleanupKeepsReplacedAlias(t *testing.T) {
	root, dir := setupMovieDir(t)
	ctx := context.Background()
	svc := New(Options{Mode: ModeHardlink, Fallback: true})

	plan, err := svc.Scan(ctx, root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}

	// 模拟用户把别名替换成自己的文件。
	alias := filepath.Join(dir, "movie - 4K.srt")
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	writeFile(t, alias, "replaced by user")

	if _, err := svc.Cleanup(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(alias)
	if err != nil || string(data) != "replaced by user" {
		t.Fatalf("用户替换的别名被删除: %q %v", data, err)
	}
	manifest, _ := readManifest(root)
	if len(manifest.Entries) != 1 {
		t.Fatalf("保留的条目应从清单中保留, got %d", len(manifest.Entries))
	}
}

func TestCleanupDryRunKeepsFiles(t *testing.T) {
	root, dir := setupMovieDir(t)
	ctx := context.Background()
	svc := New(Options{Mode: ModeHardlink, Fallback: true})

	plan, err := svc.Scan(ctx, root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "movie - 4K.srt")

	result, err := svc.Cleanup(ctx, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(alias); err != nil {
		t.Fatalf("dry-run 删除了文件: %v", err)
	}
	act := findLink(result.Actions, "movie - 4K.srt")
	if act == nil || act.Kind != KindRemove {
		t.Fatalf("dry-run 未报告待删除: %+v", result.Actions)
	}
}

func TestCopyMode(t *testing.T) {
	root, dir := setupMovieDir(t)
	ctx := context.Background()
	svc := New(Options{Mode: ModeCopy})

	plan, err := svc.Scan(ctx, root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "movie - 4K.srt")
	if sameFile(filepath.Join(dir, "movie.srt"), alias) {
		t.Fatal("copy 模式不应产生硬链接")
	}
	if !filesEqual(filepath.Join(dir, "movie.srt"), alias) {
		t.Fatal("copy 模式内容不一致")
	}
	if _, err := svc.Cleanup(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(alias); !os.IsNotExist(err) {
		t.Fatal("copy 模式的别名未被清理")
	}
}

func TestScanClassifiesExistingAlias(t *testing.T) {
	root, dir := setupMovieDir(t)
	source := filepath.Join(dir, "movie.srt")
	alias := filepath.Join(dir, "movie - 4K.srt")

	// 硬链接形式的已存在别名。
	if err := os.Link(source, alias); err != nil {
		t.Fatal(err)
	}
	svc := New(Options{Mode: ModeHardlink, Fallback: true})
	plan, err := svc.Scan(context.Background(), root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	act := findLink(plan.Actions, "movie - 4K.srt")
	if act == nil || act.Kind != KindExists || act.Mode != ModeHardlink {
		t.Fatalf("硬链接别名未被识别: %+v", act)
	}

	// 同内容的独立文件也应识别为已存在（复制方式）。
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	writeFile(t, alias, "subtitle")
	plan, err = svc.Scan(context.Background(), root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	act = findLink(plan.Actions, "movie - 4K.srt")
	if act == nil || act.Kind != KindExists || act.Mode != ModeCopy {
		t.Fatalf("复制别名未被识别: %+v", act)
	}

	// 不同内容仍然是冲突。
	writeFile(t, alias, "other subtitle")
	plan, err = svc.Scan(context.Background(), root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	act = findLink(plan.Actions, "movie - 4K.srt")
	if act == nil || act.Kind != KindConflict {
		t.Fatalf("冲突未被识别: %+v", act)
	}
}

func TestCleanupFindsNestedManifests(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "tasks", "movie")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nested, "movie.srt"), "subtitle")
	writeFile(t, filepath.Join(nested, "movie - 4K.strm"), "http://example/4k")

	ctx := context.Background()
	svc := New(Options{Mode: ModeHardlink, Fallback: true})
	plan, err := svc.Scan(ctx, nested, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(nested, "movie - 4K.srt")
	if _, err := os.Stat(alias); err != nil {
		t.Fatalf("别名未创建: %v", err)
	}

	// 从上层目录清理，应当发现子目录里的清单。
	result, err := svc.Cleanup(ctx, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(alias); !os.IsNotExist(err) {
		t.Fatalf("嵌套清单中的别名未被清理: %v", err)
	}
	if len(result.Actions) != 1 || result.Actions[0].Status != "done" {
		t.Fatalf("清理结果异常: %+v", result.Actions)
	}
	if _, err := os.Stat(filepath.Join(nested, "movie.srt")); err != nil {
		t.Fatalf("源字幕被误删: %v", err)
	}
}

func TestValidateActionRejectsEscapes(t *testing.T) {
	root := filepath.FromSlash("/media")
	ok := &Action{
		Dir:    filepath.Join(root, "movie"),
		Source: "movie.srt",
		Alias:  "movie - 4K.srt",
	}
	if err := validateAction(root, ok); err != nil {
		t.Fatalf("合法动作被拒绝: %v", err)
	}

	bad := []*Action{
		{Dir: filepath.FromSlash("/etc"), Source: "a.srt", Alias: "b.srt"},
		{Dir: filepath.Join(root, "movie"), Source: "../a.srt", Alias: "b.srt"},
		{Dir: filepath.Join(root, "movie"), Source: "a.srt", Alias: "../b.srt"},
		{Dir: filepath.Join(root, "movie"), Source: "", Alias: "b.srt"},
	}
	for _, a := range bad {
		if err := validateAction(root, a); err == nil {
			t.Errorf("越界动作未被拒绝: %+v", a)
		}
	}
}

func TestScanRespectsDepthAndHiddenDirs(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(deep, "b.srt"), "s")
	writeFile(t, filepath.Join(deep, "b - 4K.strm"), "http://example")

	hidden := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(hidden, ".hidden.srt"), "s")
	writeFile(t, filepath.Join(hidden, ".hidden - 4K.strm"), "http://example")

	svc := New(Options{})
	limited, err := svc.Scan(context.Background(), root, ScanOptions{MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.Actions) != 0 {
		t.Fatalf("MaxDepth=1 不应扫到 a/b: %+v", limited.Actions)
	}

	full, err := svc.Scan(context.Background(), root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Actions) != 1 {
		t.Fatalf("未命中深层目录或未跳过隐藏目录: %+v", full.Actions)
	}
}
