package modmeta

import (
	"archive/zip"
	"bytes"
	"testing"
)

func buildJar(t *testing.T, files map[string]string) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

func TestParseFabric(t *testing.T) {
	zr := buildJar(t, map[string]string{
		"fabric.mod.json": `{
			"schemaVersion": 1,
			"id": "mymod",
			"version": "1.2.3",
			"name": "My Mod",
			"description": "A cool mod\n",
			"license": {"id": "MIT"},
			"icon": {"128x": "assets/mymod/icon.png"},
			"depends": {"fabricloader": ">=0.14.0", "minecraft": ">=1.20 <1.21"}
		}`,
		"assets/mymod/icon.png": "fakepng",
	})
	m := ParseFromZip(zr)
	if m.Loader != "fabric" || m.ID != "mymod" || m.Name != "My Mod" || m.Version != "1.2.3" {
		t.Fatalf("basic: %+v", m)
	}
	if m.Description != "A cool mod" || m.License != "MIT" {
		t.Fatalf("desc/license: %+v", m)
	}
	if len(m.GameVersions) != 1 || m.GameVersions[0] != "1.20" {
		t.Fatalf("game versions: %+v", m.GameVersions)
	}
	if m.IconPath != "assets/mymod/icon.png" {
		t.Fatalf("icon: %+v", m)
	}
	icon := IconBytes(zr, m)
	if string(icon) != "fakepng" {
		t.Fatalf("icon bytes: %q", icon)
	}
}

func TestParseQuilt(t *testing.T) {
	zr := buildJar(t, map[string]string{
		"quilt.mod.json": `{
			"schema_version": 1,
			"quilt_loader": {
				"id": "qmod", "version": "0.1.0", "name": "Quilt Mod",
				"description": "quilt desc", "license": "Apache-2.0", "icon": "icon.png"
			},
			"interoperability": {"depends": [{"id": "minecraft", "versions": ">=1.19.4"}]}
		}`,
	})
	m := ParseFromZip(zr)
	if m.Loader != "quilt" || m.ID != "qmod" || m.License != "Apache-2.0" {
		t.Fatalf("quilt: %+v", m)
	}
	if len(m.GameVersions) != 1 || m.GameVersions[0] != "1.19.4" {
		t.Fatalf("quilt versions: %+v", m.GameVersions)
	}
	if m.IconPath != "icon.png" {
		t.Fatalf("quilt icon: %+v", m)
	}
}

func TestParseForgeToml(t *testing.T) {
	zr := buildJar(t, map[string]string{
		"META-INF/mods.toml": `
modLoader="javafml"
loaderVersion="[40,)"
license="GPL-3.0"
[[mods]]
modId="forgemod"
version="2.0"
displayName="Forge Mod"
description='''
Multi-line forge desc
'''
[[dependencies.forgemod]]
modId="minecraft"
versionRange="[1.20.1,)"
`,
	})
	m := ParseFromZip(zr)
	if m.Loader != "forge" || m.ID != "forgemod" || m.Name != "Forge Mod" {
		t.Fatalf("forge: %+v", m)
	}
	if m.License != "GPL-3.0" || m.Version != "2.0" {
		t.Fatalf("forge license/version: %+v", m)
	}
	if !bytes.Contains([]byte(m.Description), []byte("Multi-line forge desc")) {
		t.Fatalf("forge desc: %q", m.Description)
	}
	if len(m.GameVersions) != 1 || m.GameVersions[0] != "1.20.1" {
		t.Fatalf("forge versions: %+v", m.GameVersions)
	}
}

func TestParseNeoforge(t *testing.T) {
	zr := buildJar(t, map[string]string{
		"META-INF/neoforge.mods.toml": `
license="MIT"
[[mods]]
modId="neomod"
version="3.0"
displayName="Neo Mod"
[[dependencies.neomod]]
modId="minecraft"
versionRange="[1.21,)"
`,
	})
	m := ParseFromZip(zr)
	if m.Loader != "neoforge" || m.ID != "neomod" {
		t.Fatalf("neo: %+v", m)
	}
	if len(m.GameVersions) != 1 || m.GameVersions[0] != "1.21" {
		t.Fatalf("neo versions: %+v", m.GameVersions)
	}
}

func TestParseFabricPriority(t *testing.T) {
	// fabric.mod.json 优先于 mods.toml（同时存在时按候选顺序）
	zr := buildJar(t, map[string]string{
		"fabric.mod.json":    `{"id":"f","name":"F"}`,
		"META-INF/mods.toml": "license=\"X\"\n[[mods]]\nmodId=\"t\"\n",
	})
	m := ParseFromZip(zr)
	if m.Loader != "fabric" {
		t.Fatalf("priority: %+v", m)
	}
}

func TestParseNoMeta(t *testing.T) {
	zr := buildJar(t, map[string]string{"some/class.class": "bytes"})
	m := ParseFromZip(zr)
	if !m.Empty() {
		t.Fatalf("expected empty, got %+v", m)
	}
	if IconBytes(zr, m) != nil {
		t.Fatal("icon should be nil")
	}
}

func TestParseBrokenMeta(t *testing.T) {
	// 损坏的 JSON 不应崩溃，返回零值
	zr := buildJar(t, map[string]string{"fabric.mod.json": "{not json"})
	if m := ParseFromZip(zr); !m.Empty() {
		t.Fatalf("broken: %+v", m)
	}
}
