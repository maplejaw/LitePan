package strm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"litepan/internal/domain"
)

func newQuietStrmService() *Service {
	return &Service{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestRunSubtitleAlignInvokesAligner(t *testing.T) {
	s := newQuietStrmService()
	var got *domain.StrmTask
	s.SetSubtitleAligner(func(_ context.Context, task *domain.StrmTask) error {
		got = task
		return nil
	})

	task := &domain.StrmTask{ID: 7, Name: "电影"}
	s.runSubtitleAlign(context.Background(), task)
	if got != task {
		t.Fatalf("对齐器未被调用")
	}
}

func TestRunSubtitleAlignWithoutAligner(t *testing.T) {
	s := newQuietStrmService()
	// 未注入对齐器时直接返回，不应 panic。
	s.runSubtitleAlign(context.Background(), &domain.StrmTask{ID: 1})
}

func TestRunSubtitleAlignSwallowsError(t *testing.T) {
	s := newQuietStrmService()
	s.SetSubtitleAligner(func(context.Context, *domain.StrmTask) error {
		return errors.New("boom")
	})
	// 对齐失败只记录日志，不能影响任务本身（方法无返回值）。
	s.runSubtitleAlign(context.Background(), &domain.StrmTask{ID: 1})
}
