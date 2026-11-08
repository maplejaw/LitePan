package subtitlealign

import (
	"path/filepath"
	"reflect"
	"testing"
)

func summarize(actions []Action) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.Kind+"|"+a.Video+"|"+a.Source+"|"+a.Alias)
	}
	return out
}

func TestBuildActions(t *testing.T) {
	cases := []struct {
		name    string
		dir     string
		names   []string
		subExts []string
		want    []string
	}{
		{
			name: "多版本生成别名，带语言标记的字幕不作为源",
			dir:  filepath.FromSlash("/media/流浪地球2"),
			names: []string{
				"流浪地球2 - 4K.strm",
				"流浪地球2-1080p.strm",
				"流浪地球2.strm",
				"流浪地球2.srt",
				"流浪地球2.zh.srt",
				"Sample.strm",
			},
			want: []string{
				"link|流浪地球2 - 4K.strm|流浪地球2.srt|流浪地球2 - 4K.srt",
				"link|流浪地球2-1080p.strm|流浪地球2.srt|流浪地球2-1080p.srt",
				"skip|流浪地球2.strm|流浪地球2.srt|流浪地球2.srt",
			},
		},
		{
			name:  "没有源字幕时整个目录跳过",
			dir:   filepath.FromSlash("/media/电影名"),
			names: []string{"电影名 - 4K.strm", "电影名.zh.srt"},
			want:  nil,
		},
		{
			name:  "忽略大小写",
			dir:   filepath.FromSlash("/media/movie"),
			names: []string{"MOVIE - 4K.STRM", "movie.srt"},
			want: []string{
				"link|MOVIE - 4K.STRM|movie.srt|MOVIE - 4K.srt",
			},
		},
		{
			name:  "非 strm 视频被忽略",
			dir:   filepath.FromSlash("/media/movie"),
			names: []string{"movie - 4K.mkv", "movie.srt"},
			want:  nil,
		},
		{
			name:  "不以目录名开头的 strm 被忽略",
			dir:   filepath.FromSlash("/media/movie"),
			names: []string{"other.strm", "Sample.strm", "movie.srt"},
			want:  nil,
		},
		{
			name:  "同前缀的续集会被当作版本（已确认目录规范不会出现）",
			dir:   filepath.FromSlash("/media/沙丘"),
			names: []string{"沙丘2 - 4K.strm", "沙丘.srt"},
			want: []string{
				"link|沙丘2 - 4K.strm|沙丘.srt|沙丘2 - 4K.srt",
			},
		},
		{
			name:  "多个字幕扩展名各生成一份",
			dir:   filepath.FromSlash("/media/movie"),
			names: []string{"movie - 4K.strm", "movie.ass", "movie.srt"},
			want: []string{
				"link|movie - 4K.strm|movie.ass|movie - 4K.ass",
				"link|movie - 4K.strm|movie.srt|movie - 4K.srt",
			},
		},
		{
			name:  "视频本体只产生 skip",
			dir:   filepath.FromSlash("/media/movie"),
			names: []string{"movie.strm", "movie.srt"},
			want: []string{
				"skip|movie.strm|movie.srt|movie.srt",
			},
		},
		{
			name:  "目录名本身带点",
			dir:   filepath.FromSlash("/media/Movie.2024"),
			names: []string{"Movie.2024.1080p.strm", "Movie.2024.srt"},
			want: []string{
				"link|Movie.2024.1080p.strm|Movie.2024.srt|Movie.2024.1080p.srt",
			},
		},
		{
			name:    "自定义扩展名不带点也能识别",
			dir:     filepath.FromSlash("/media/movie"),
			names:   []string{"movie - 4K.strm", "movie.SRT"},
			subExts: []string{"SRT"},
			want: []string{
				"link|movie - 4K.strm|movie.SRT|movie - 4K.SRT",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(BuildActions(tc.dir, tc.names, tc.subExts))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %v\nwant %v", got, tc.want)
			}
		})
	}
}

func TestWorkName(t *testing.T) {
	cases := map[string]string{
		filepath.FromSlash("/media/movie"):  "movie",
		filepath.FromSlash("/media/movie/"): "movie",
		filepath.FromSlash("/media/流浪地球2"):  "流浪地球2",
	}
	for dir, want := range cases {
		if got := WorkName(dir); got != want {
			t.Errorf("WorkName(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestPrefixedBy(t *testing.T) {
	cases := []struct {
		name, folder string
		want         bool
	}{
		{"movie - 4K", "movie", true},
		{"MOVIE-1080p", "movie", true},
		{"movie", "movie", true},
		{"movies", "movie", true}, // 纯前缀规则，不含边界判断
		{"mov", "movie", false},
		{"other", "movie", false},
		{"沙丘2 - 4K", "沙丘", true},
	}
	for _, tc := range cases {
		if got := prefixedBy(tc.name, tc.folder); got != tc.want {
			t.Errorf("prefixedBy(%q, %q) = %v, want %v", tc.name, tc.folder, got, tc.want)
		}
	}
}
