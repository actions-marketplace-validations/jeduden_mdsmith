package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ChannelKind is the kind name registered in .mdsmith.yml that
// drives `mdsmith extract` over the per-channel source files.
const ChannelKind = "release-channel"

// ChannelDir holds one Markdown file per distribution channel —
// the single source of truth the install/release tables and the
// website install picker all derive from.
const ChannelDir = "docs/development/release-channels"

// ChannelsDataFile is the Hugo data file the install picker reads.
// It is generated, never edited by hand.
const ChannelsDataFile = "website/data/channels.yaml"

// Channel is the projected, picker-facing shape of one channel.
// The yaml tags define the website/data/channels.yaml schema the
// install-picker partial consumes.
type Channel struct {
	Title     string   `yaml:"title"`
	Summary   string   `yaml:"summary"`
	Mechanism string   `yaml:"mechanism"`
	Artifact  string   `yaml:"artifact"`
	Command   string   `yaml:"command"`
	Audience  string   `yaml:"audience"`
	Platforms []string `yaml:"platforms"`
	URL       string   `yaml:"url"`
	Weight    int      `yaml:"weight"`
}

// channelDoc mirrors the `frontmatter` object that
// `mdsmith extract release-channel <file>` emits. Only the
// frontmatter feeds the picker; the body is documentation.
type channelDoc struct {
	Frontmatter struct {
		Title      string   `json:"title"`
		Summary    string   `json:"summary"`
		Mechanism  string   `json:"mechanism"`
		Artifact   string   `json:"artifact"`
		Command    string   `json:"command"`
		Audience   string   `json:"audience"`
		Platforms  []string `json:"platforms"`
		ChannelURL string   `json:"channelurl"`
		Weight     int      `json:"weight"`
	} `json:"frontmatter"`
}

// channelExtractor is a package-level seam so tests stub the
// shell-out without driving the real mdsmith binary.
var channelExtractor = runChannelExtract

func runChannelExtract(root, relPath string) ([]byte, error) {
	cmd := exec.Command("go", "run", "./cmd/mdsmith", //nolint:gosec // CI-only; args are a constant verb plus a dir-listed filename
		"extract", ChannelKind, relPath, "--format", "json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("mdsmith extract %s: %w (stderr: %s)",
				relPath, err, ee.Stderr)
		}
		return nil, fmt.Errorf("mdsmith extract %s: %w", relPath, err)
	}
	return out, nil
}

// channelFiles lists the channel source basenames (sorted),
// excluding the proto.md schema file.
func channelFiles(root string) ([]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(ChannelDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read channel dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "proto.md" {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files, nil
}

// LoadChannels projects every channel file through
// `mdsmith extract` and returns them sorted by ascending weight.
// Extraction is schema-gated, so a non-conformant channel file
// fails here with the same message `mdsmith check` would print.
func LoadChannels(root string) ([]Channel, error) {
	files, err := channelFiles(root)
	if err != nil {
		return nil, err
	}
	chs := make([]Channel, 0, len(files))
	for _, name := range files {
		rel := filepath.ToSlash(filepath.Join(ChannelDir, name))
		out, err := channelExtractor(root, rel)
		if err != nil {
			return nil, err
		}
		var doc channelDoc
		if err := json.Unmarshal(out, &doc); err != nil {
			return nil, fmt.Errorf("decode %s json: %w", rel, err)
		}
		f := doc.Frontmatter
		ch := Channel{
			Title:     f.Title,
			Summary:   f.Summary,
			Mechanism: f.Mechanism,
			Artifact:  f.Artifact,
			Command:   f.Command,
			Audience:  f.Audience,
			Platforms: f.Platforms,
			URL:       f.ChannelURL,
			Weight:    f.Weight,
		}
		if err := ch.validate(rel); err != nil {
			return nil, err
		}
		chs = append(chs, ch)
	}
	sort.SliceStable(chs, func(i, j int) bool {
		return chs[i].Weight < chs[j].Weight
	})
	return chs, nil
}

// validate fails fast if a required projected field is empty. The
// proto.md schema's CUE constraints catch the same condition under
// `mdsmith check`; this keeps sync-channels self-contained.
func (c Channel) validate(src string) error {
	var missing []string
	for _, p := range []struct{ name, value string }{
		{"title", c.Title},
		{"mechanism", c.Mechanism},
		{"artifact", c.Artifact},
		{"command", c.Command},
		{"audience", c.Audience},
	} {
		if strings.TrimSpace(p.value) == "" {
			missing = append(missing, p.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("channel %s: empty field(s): %s",
			src, strings.Join(missing, ", "))
	}
	return nil
}

// channelsHeader is the do-not-edit banner the data file carries.
const channelsHeader = "# Generated by `mdsmith-release sync-channels` from\n" +
	"# docs/development/release-channels/*.md — do not edit by hand.\n"

// RenderChannelsYAML marshals channels into the website data file
// body, prefixed with the generated-file banner.
func RenderChannelsYAML(chs []Channel) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(channelsHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(chs); err != nil {
		return nil, fmt.Errorf("encode channels yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// renderChannelsFile loads the source and renders the data bytes.
func renderChannelsFile(root string) ([]byte, error) {
	chs, err := LoadChannels(root)
	if err != nil {
		return nil, err
	}
	return RenderChannelsYAML(chs)
}

// SyncChannels regenerates ChannelsDataFile from the source and
// reports whether the on-disk file changed.
func SyncChannels(root string) (bool, error) {
	out, err := renderChannelsFile(root)
	if err != nil {
		return false, err
	}
	path := filepath.Join(root, filepath.FromSlash(ChannelsDataFile))
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, out) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("mkdir website data: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", ChannelsDataFile, err)
	}
	return true, nil
}

// CheckChannels reports whether ChannelsDataFile is out of date
// with respect to the source. A missing file counts as drift.
func CheckChannels(root string) (bool, error) {
	out, err := renderChannelsFile(root)
	if err != nil {
		return false, err
	}
	path := filepath.Join(root, filepath.FromSlash(ChannelsDataFile))
	old, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read %s: %w", ChannelsDataFile, err)
	}
	return !bytes.Equal(old, out), nil
}
