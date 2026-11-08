package subtitlealign

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// manifestName 放在扫描根目录下，随媒体目录一起移动。
const manifestName = ".litepan-subtitle-align.json"

// Manifest 记录本工具创建过的别名，是清理时唯一可信的依据。
type Manifest struct {
	Version int             `json:"version"`
	Entries []ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	Alias      string `json:"alias"`  // 相对根目录
	Source     string `json:"source"` // 相对根目录
	Mode       string `json:"mode"`   // hardlink | copy
	CreatedAt  string `json:"created_at"`
	SourceSize int64  `json:"source_size"`
	SourceMod  int64  `json:"source_mod_unix"`
}

func manifestPath(root string) string {
	return filepath.Join(root, manifestName)
}

func readManifest(root string) (*Manifest, error) {
	return readManifestAt(manifestPath(root))
}

func readManifestAt(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Manifest{Version: 1, Entries: []ManifestEntry{}}, nil
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Entries == nil {
		m.Entries = []ManifestEntry{}
	}
	return &m, nil
}

// manifestRef 是一份清单及其所在目录。
type manifestRef struct {
	Dir string
	M   *Manifest
}

// findManifests 递归查找目录树中的清单。
// 自动对齐按任务输出目录写入清单，清理时只读根目录会漏掉子目录里的记录。
func findManifests(root string) ([]manifestRef, error) {
	var out []manifestRef
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || skipDirNames[d.Name()]) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != manifestName {
			return nil
		}
		m, readErr := readManifestAt(path)
		if readErr != nil {
			return nil
		}
		out = append(out, manifestRef{Dir: filepath.Dir(path), M: m})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func writeManifest(root string, m *Manifest) error {
	m.Version = 1
	if m.Entries == nil {
		m.Entries = []ManifestEntry{}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(root, "."+manifestName+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, manifestPath(root))
}
