package strmscrape

import (
	"context"
	"strings"
	"testing"

	"litepan/internal/settings"
)

func TestScrapeEnabledDefaultsTrue(t *testing.T) {
	svc := &Service{}
	if !svc.Enabled() {
		t.Fatal("未注入设置服务时应默认开启")
	}
}

func TestRunAsyncRejectsWhenScrapeDisabled(t *testing.T) {
	svc := &Service{settings: newScrapeSettings(t, map[string]string{
		settings.KeyStrmScrapeEnabled: "false",
	})}
	if svc.Enabled() {
		t.Fatal("开关为 false 时 Enabled 应为 false")
	}
	err := svc.RunAsync(context.Background(), RunRequest{StrmTaskID: 1})
	if err == nil || !strings.Contains(err.Error(), "已关闭") {
		t.Fatalf("关闭后 RunAsync 应拒绝，实际 err=%v", err)
	}
}

func TestMarkNormalRejectsWhenScrapeDisabled(t *testing.T) {
	svc := &Service{settings: newScrapeSettings(t, map[string]string{
		settings.KeyStrmScrapeEnabled: "false",
	})}
	_, err := svc.MarkNormal(context.Background(), MarkNormalRequest{StrmTaskID: 1, ItemID: "x"})
	if err == nil || !strings.Contains(err.Error(), "已关闭") {
		t.Fatalf("关闭后 MarkNormal 应拒绝，实际 err=%v", err)
	}
}
