package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// runRenderScoopManifest is the entry point for the
// `render-scoop-manifest` subcommand. It reads version and
// checksums-file arguments, extracts the Windows hash, and writes
// the Scoop JSON manifest to stdout.
func runRenderScoopManifest(_ string, args []string) int {
	fs := flag.NewFlagSet("render-scoop-manifest", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith-release render-scoop-manifest <version> <checksums-file>\n\n"+
				"Emit the Scoop bucket/mdsmith.json manifest for <version> to\n"+
				"stdout, reading the mdsmith-windows-amd64.exe SHA-256 from\n"+
				"<checksums-file>. <version> must be a bare semver (no leading v).\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr,
			"mdsmith-release: render-scoop-manifest"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	version := fs.Arg(0)
	checksumsPath := fs.Arg(1)

	f, err := os.Open(checksumsPath) //nolint:gosec // path is the CLI argument
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: open checksums: %v\n", err)
		return 1
	}
	defer func() { _ = f.Close() }()

	hash, err := release.ParseChecksumFor(f, "mdsmith-windows-amd64.exe")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: %v\n", err)
		return 1
	}

	fmt.Print(release.RenderScoopManifest(version, hash))
	return 0
}

// runRenderWingetManifest is the entry point for the
// `render-winget-manifest` subcommand. It reads version and
// checksums-file arguments, extracts the Windows hash, and writes
// the three WinGet YAML manifest files to a target directory.
func runRenderWingetManifest(_ string, args []string) int {
	fs := flag.NewFlagSet("render-winget-manifest", flag.ContinueOnError)
	outDir := fs.String("out", "",
		"directory to write the three WinGet manifest files into (required)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith-release render-winget-manifest --out <dir> <version> <checksums-file>\n\n"+
				"Write the three WinGet YAML manifests for <version> under <dir>:\n"+
				"  <dir>/jeduden.mdsmith.yaml                (version manifest)\n"+
				"  <dir>/jeduden.mdsmith.installer.yaml      (installer manifest)\n"+
				"  <dir>/jeduden.mdsmith.locale.en-US.yaml   (locale manifest)\n\n"+
				"<version> must be a bare semver (no leading v).\n"+
				"The SHA-256 of mdsmith-windows-amd64.exe is read from <checksums-file>.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr,
			"mdsmith-release: render-winget-manifest"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 || *outDir == "" {
		fs.Usage()
		return 2
	}
	version := fs.Arg(0)
	checksumsPath := fs.Arg(1)

	f, err := os.Open(checksumsPath) //nolint:gosec // path is the CLI argument
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: open checksums: %v\n", err)
		return 1
	}
	defer func() { _ = f.Close() }()

	hash, err := release.ParseChecksumFor(f, "mdsmith-windows-amd64.exe")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: %v\n", err)
		return 1
	}

	manifests := release.RenderWingetManifests(version, hash)
	names := []string{
		"jeduden.mdsmith.yaml",
		"jeduden.mdsmith.installer.yaml",
		"jeduden.mdsmith.locale.en-US.yaml",
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: mkdir %s: %v\n", *outDir, err)
		return 1
	}

	for i, name := range names {
		path := *outDir + "/" + name
		if err := os.WriteFile(path, []byte(manifests[i]), 0o644); err != nil { //nolint:gosec // 0644 is intentional
			fmt.Fprintf(os.Stderr, "mdsmith-release: write %s: %v\n", path, err)
			return 1
		}
	}

	fmt.Printf("wrote %d WinGet manifests to %s\n", len(names), *outDir)
	return 0
}
