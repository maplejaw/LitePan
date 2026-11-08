package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/subtitlealign"
)

// requireSubtitleAlignEnabled 在总开关关闭时拒绝整组接口。
func (h *Handler) requireSubtitleAlignEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.subtitleAlign == nil || !h.subtitleAlign.Enabled() {
			writeErr(w, domain.Errorf(domain.CodeValidation, "字幕对齐功能已关闭"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) getSubtitleAlignSettings(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.subtitleAlign != nil) {
		return
	}
	writeOK(w, h.subtitleAlign.GetSettings())
}

func (h *Handler) updateSubtitleAlignSettings(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.subtitleAlign != nil) {
		return
	}
	var req subtitlealign.Settings
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.subtitleAlign.UpdateSettings(r.Context(), req); err != nil {
		writeErr(w, err)
		return
	}
	h.getSubtitleAlignSettings(w, r)
}

func (h *Handler) scanSubtitleAlign(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.subtitleAlign != nil) {
		return
	}
	var req struct {
		Root     string `json:"root"`
		MaxDepth int    `json:"max_depth"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	root, err := subtitleAlignRoot(req.Root)
	if err != nil {
		writeErr(w, err)
		return
	}
	plan, err := h.subtitleAlign.Scan(r.Context(), root, subtitlealign.ScanOptions{MaxDepth: req.MaxDepth})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, plan)
}

// applySubtitleAlign 接收扫描时返回的完整计划，服务端会重新校验每个动作的路径。
func (h *Handler) applySubtitleAlign(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.subtitleAlign != nil) {
		return
	}
	var plan subtitlealign.Plan
	if err := decodeJSON(r, &plan); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.subtitleAlign.Apply(r.Context(), &plan); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, plan)
}

// cleanupSubtitleAlign 按清单清理；dry_run=true 时只做校验。
func (h *Handler) cleanupSubtitleAlign(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.subtitleAlign != nil) {
		return
	}
	var req struct {
		Root   string `json:"root"`
		DryRun bool   `json:"dry_run"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	root, err := subtitleAlignRoot(req.Root)
	if err != nil {
		writeErr(w, err)
		return
	}
	plan, err := h.subtitleAlign.Cleanup(r.Context(), root, req.DryRun)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, plan)
}

func subtitleAlignRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" {
		return "", domain.Errorf(domain.CodeValidation, "请填写要处理的目录")
	}
	if !filepath.IsAbs(root) {
		return "", domain.Errorf(domain.CodeValidation, "目录必须是绝对路径")
	}
	return filepath.Clean(root), nil
}
