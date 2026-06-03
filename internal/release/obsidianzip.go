package release

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// obsidianZipFiles is the exact set of files the Obsidian release zip
// ships, stored flat (base name only, no dist/ prefix). It mirrors the
// five files Obsidian loads from a plugin directory and the
// upload-artifact contract in ci.yml / release.yml. manifest.json
// doubles as the version source.
var obsidianZipFiles = []string{
	"main.js",
	"manifest.json",
	"styles.css",
	"mdsmith.wasm",
	"wasm_exec.js",
}

// PackageObsidian builds the Obsidian plugin release zip from a built
// dist directory. It reads the plugin version from
// <distDir>/manifest.json (JSON `version` field) and writes
// <outDir>/mdsmith-obsidian-<version>.zip containing exactly the five
// files in obsidianZipFiles, stored flat. It returns the created zip's
// path.
//
// The zip is built with archive/zip rather than shelling out to the
// `zip` binary so the step is pure Go, cross-platform, and unit
// testable — the rule in docs/development/release-tooling.md. A missing
// manifest.json, a manifest with no version, or any missing required
// file is an error.
func PackageObsidian(distDir, outDir string) (string, error) {
	version, err := readObsidianVersion(filepath.Join(distDir, "manifest.json"))
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	zipPath := filepath.Join(outDir, "mdsmith-obsidian-"+version+".zip")
	if err := writeObsidianZip(distDir, zipPath); err != nil {
		return "", err
	}

	info, err := os.Stat(zipPath)
	if err != nil {
		return "", err
	}
	// Parity with the old `ls -l`: a one-line confirmation of the
	// artifact path and size.
	fmt.Printf("packaged %s (%d bytes)\n", zipPath, info.Size())
	return zipPath, nil
}

// readObsidianVersion reads the plugin version from a manifest.json at
// path. It reuses jsonVersionRE (the same matcher build-npm uses) so a
// missing file, a manifest without a "version" field, or an empty
// version all surface as a clear error.
func readObsidianVersion(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sub := jsonVersionRE.FindSubmatch(body)
	if sub == nil {
		return "", fmt.Errorf("%s: no version field found", path)
	}
	return string(sub[2]), nil
}

// writeObsidianZip writes the five obsidianZipFiles from distDir into a
// new zip at zipPath, each stored under its base name. A missing
// required file is an error and leaves no zip behind.
func writeObsidianZip(distDir, zipPath string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range obsidianZipFiles {
		data, err := os.ReadFile(filepath.Join(distDir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("add %s to zip: %w", name, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("write %s to zip: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalize zip: %w", err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", zipPath, err)
	}
	return nil
}
