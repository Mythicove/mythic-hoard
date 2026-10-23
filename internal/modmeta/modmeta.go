// Package modmeta 解析 mod jar 内的元数据文件，用于上传时自动填充与冲突标旗。
// 支持：fabric.mod.json、quilt.mod.json、META-INF/neoforge.mods.toml、META-INF/mods.toml。
package modmeta

import (
	"archive/zip"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Meta 是解析出的 mod 元数据（零值字段表示 jar 未提供）。
type Meta struct {
	Loader       string   `json:"loader"` // fabric | quilt | forge | neoforge
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	License      string   `json:"license"`
	GameVersions []string `json:"game_versions,omitempty"`
	IconPath     string   `json:"icon_path,omitempty"` // jar 内相对路径，调用方自行读取
}

// Empty 返回是否什么都没解析到。
func (m Meta) Empty() bool {
	return m.Loader == "" && m.ID == "" && m.Name == ""
}

// 元数据文件在 jar 内的候选路径（按优先级）。
var candidates = []struct {
	path   string
	loader string
}{
	{"fabric.mod.json", "fabric"},
	{"quilt.mod.json", "quilt"},
	{"META-INF/neoforge.mods.toml", "neoforge"},
	{"META-INF/mods.toml", "forge"},
}

// ParseFromZip 从 jar（zip）解析元数据；未识别任何格式时返回零值 Meta（不报错——非 mod jar 也允许上传）。
func ParseFromZip(zr *zip.Reader) Meta {
	for _, cand := range candidates {
		for _, f := range zr.File {
			if f.Name != cand.path {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(rc, 1<<20)) // 元数据文件上限 1MiB
			rc.Close()
			if err != nil {
				continue
			}
			switch cand.loader {
			case "fabric":
				return parseFabric(data)
			case "quilt":
				return parseQuilt(data)
			default:
				return parseModsToml(data, cand.loader)
			}
		}
	}
	return Meta{}
}

// IconBytes 读取 Meta.IconPath 指向的 jar 内文件（上限 2MiB）；无 icon 或读取失败返回 nil。
func IconBytes(zr *zip.Reader, m Meta) []byte {
	if m.IconPath == "" {
		return nil
	}
	for _, f := range zr.File {
		if f.Name != m.IconPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, 2<<20))
		if err != nil {
			return nil
		}
		return data
	}
	return nil
}

type fabricJSON struct {
	ID          string          `json:"id"`
	Version     string          `json:"version"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	License     json.RawMessage `json:"license"` // string 或 {id:...}
	Icon        json.RawMessage `json:"icon"`    // string 或 {"128x": "..."}
	Depends     map[string]any  `json:"depends"`
}

func parseFabric(data []byte) Meta {
	var f fabricJSON
	if err := json.Unmarshal(data, &f); err != nil {
		return Meta{}
	}
	m := Meta{
		Loader: "fabric", ID: f.ID, Version: f.Version,
		Name: f.Name, Description: strings.TrimSpace(f.Description),
		License: licenseString(f.License),
	}
	if mc, ok := f.Depends["minecraft"]; ok {
		if s, ok := mc.(string); ok {
			m.GameVersions = lowerBoundVersions(s)
		}
	}
	m.IconPath = iconString(f.Icon)
	return m
}

type quiltJSON struct {
	QuiltLoader struct {
		ID          string          `json:"id"`
		Version     string          `json:"version"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		License     json.RawMessage `json:"license"`
		Icon        json.RawMessage `json:"icon"`
	} `json:"quilt_loader"`
	Interop struct {
		Depends []struct {
			ID       string `json:"id"`
			Versions any    `json:"versions"`
		} `json:"depends"`
	} `json:"interoperability"`
}

func parseQuilt(data []byte) Meta {
	var q quiltJSON
	if err := json.Unmarshal(data, &q); err != nil {
		return Meta{}
	}
	ql := q.QuiltLoader
	m := Meta{
		Loader: "quilt", ID: ql.ID, Version: ql.Version,
		Name: ql.Name, Description: strings.TrimSpace(ql.Description),
		License: licenseString(ql.License), IconPath: iconString(ql.Icon),
	}
	for _, d := range q.Interop.Depends {
		if d.ID == "minecraft" {
			switch v := d.Versions.(type) {
			case string:
				m.GameVersions = lowerBoundVersions(v)
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						m.GameVersions = append(m.GameVersions, lowerBoundVersions(s)...)
					}
				}
			}
		}
	}
	return m
}

type modsToml struct {
	License string `toml:"license"`
	Mods    []struct {
		ModID       string `toml:"modId"`
		Version     string `toml:"version"`
		DisplayName string `toml:"displayName"`
		Description string `toml:"description"`
	} `toml:"mods"`
	Dependencies map[string][]struct {
		ModID        string `toml:"modId"`
		VersionRange string `toml:"versionRange"`
	} `toml:"dependencies"`
}

func parseModsToml(data []byte, loader string) Meta {
	var t modsToml
	if _, err := toml.Decode(string(data), &t); err != nil {
		return Meta{}
	}
	m := Meta{Loader: loader, License: strings.TrimSpace(t.License)}
	if len(t.Mods) > 0 {
		first := t.Mods[0]
		m.ID = first.ModID
		m.Version = first.Version
		m.Name = first.DisplayName
		m.Description = strings.TrimSpace(first.Description)
	}
	for _, deps := range t.Dependencies {
		for _, d := range deps {
			if d.ModID == "minecraft" {
				m.GameVersions = lowerBoundVersions(d.VersionRange)
			}
		}
	}
	return m
}

// versionRe 匹配 x.y 或 x.y.z 形式的版本号。
var versionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

// lowerBoundVersions 从版本约束提取下界版本号（如 ">=1.20 <1.21"、"[1.20,)" → ["1.20"]）。
// 说明：取首个匹配作为下界近似；精确区间展开留给发布工具链。
func lowerBoundVersions(constraint string) []string {
	m := versionRe.FindString(constraint)
	if m == "" {
		return nil
	}
	return []string{m}
}

// licenseString 兼容 "MIT" 与 {"id":"MIT"} 两种写法。
func licenseString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var obj struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.ID != "" {
			return obj.ID
		}
		return strings.TrimSpace(obj.Name)
	}
	return ""
}

// iconString 兼容 "path.png" 与 {"128x":"path.png"} 两种写法（map 取任一值）。
func iconString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err == nil {
		for _, v := range m {
			return v
		}
	}
	return ""
}
