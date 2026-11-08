package subtitlealign

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	data, err := os.ReadFile(manifestPath(root))
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
