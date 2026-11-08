package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"litepan/internal/subtitlealign"
)

func newSubtitleAlignTestHandler() *Handler {
	return &Handler{
		subtitleAlign: subtitlealign.New(subtitlealign.Options{
			Mode:     subtitlealign.ModeHardlink,
			Fallback: true,
		}),
	}
}

func TestSubtitleAlignScanApplyHTTP(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "movie")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"movie.srt":       "subtitle",
		"movie - 4K.strm": "http://example/4k",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := newSubtitleAlignTestHandler()

	scanBody, err := json.Marshal(map[string]any{"root": root, "max_depth": 0})
	if err != nil {
		t.Fatal(err)
	}
	scanRec := httptest.NewRecorder()
	h.scanSubtitleAlign(scanRec, httptest.NewRequest(http.MethodPost, "/scan", bytes.NewReader(scanBody)))
	if scanRec.Code != http.StatusOK {
		t.Fatalf("scan status = %d, body = %s", scanRec.Code, scanRec.Body.String())
	}
	var scanResp struct {
		Success bool               `json:"success"`
		Data    subtitlealign.Plan `json:"data"`
	}
	if err := json.Unmarshal(scanRec.Body.Bytes(), &scanResp); err != nil {
		t.Fatal(err)
	}
	if !scanResp.Success || len(scanResp.Data.Actions) != 1 {
		t.Fatalf("扫描结果异常: %+v", scanResp)
	}
	if scanResp.Data.Actions[0].Alias != "movie - 4K.srt" {
		t.Fatalf("别名为 %q", scanResp.Data.Actions[0].Alias)
	}

	// 把扫描结果原样回传给 apply，验证计划 JSON 往返可用。
	applyBody, err := json.Marshal(scanResp.Data)
	if err != nil {
		t.Fatal(err)
	}
	applyRec := httptest.NewRecorder()
	h.applySubtitleAlign(applyRec, httptest.NewRequest(http.MethodPost, "/apply", bytes.NewReader(applyBody)))
	if applyRec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", applyRec.Code, applyRec.Body.String())
	}
	var applyResp struct {
		Success bool               `json:"success"`
		Data    subtitlealign.Plan `json:"data"`
	}
	if err := json.Unmarshal(applyRec.Body.Bytes(), &applyResp); err != nil {
		t.Fatal(err)
	}
	if !applyResp.Success || len(applyResp.Data.Actions) != 1 || applyResp.Data.Actions[0].Status != "done" {
		t.Fatalf("应用结果异常: %+v", applyResp)
	}

	sourceInfo, err := os.Stat(filepath.Join(dir, "movie.srt"))
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, err := os.Stat(filepath.Join(dir, "movie - 4K.srt"))
	if err != nil {
		t.Fatalf("别名未创建: %v", err)
	}
	if !os.SameFile(sourceInfo, aliasInfo) {
		t.Fatal("别名不是硬链接")
	}
}

func TestSubtitleAlignScanRejectsRelativeRoot(t *testing.T) {
	h := newSubtitleAlignTestHandler()
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"root":"relative/path"}`))
	h.scanSubtitleAlign(rec, httptest.NewRequest(http.MethodPost, "/scan", body))
	if rec.Code == http.StatusOK {
		t.Fatalf("相对路径被接受: %s", rec.Body.String())
	}
}
