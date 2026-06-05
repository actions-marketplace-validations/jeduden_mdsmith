// Package bytelimit guards against oversized inputs. It reads a file —
// from disk or an fs.FS — up to a byte cap, returning an error when the
// file exceeds it.
package bytelimit

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
)

// DefaultMaxInputBytes is the default file-size cap (2 MB, binary).
const DefaultMaxInputBytes int64 = 2 * 1024 * 1024

// ReadFileLimited reads path from disk, returning an error if the file
// exceeds max bytes. When max <= 0 or max == math.MaxInt64 no limit is
// applied (unlimited mode). MaxInt64 is treated as unlimited because the
// +1 sentinel used internally would overflow.
func ReadFileLimited(path string, max int64) ([]byte, error) {
	if max <= 0 || max == math.MaxInt64 {
		return os.ReadFile(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	return readLimited(f, path, max)
}

// ReadFSFileLimited reads name from fsys, returning an error if the file
// exceeds max bytes. When max <= 0 or max == math.MaxInt64 no limit is
// applied (unlimited mode).
func ReadFSFileLimited(fsys fs.FS, name string, max int64) ([]byte, error) {
	if max <= 0 || max == math.MaxInt64 {
		return fs.ReadFile(fsys, name)
	}

	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	return readLimited(f, name, max)
}

// readLimited reads from r up to max+1 bytes. If the read returns more
// than max bytes the file is too large. The +1 sentinel distinguishes
// "exactly at limit" from "truncated".
//
// When the underlying reader is a file, we stat it first to report the
// actual file size in the error message. For non-file readers (or when
// stat fails), we report the truncated read length.
func readLimited(r io.Reader, name string, max int64) ([]byte, error) {
	// Try to get actual file size for a better error message.
	var actualSize int64 = -1
	if st, ok := r.(interface{ Stat() (os.FileInfo, error) }); ok {
		if info, err := st.Stat(); err == nil {
			actualSize = info.Size()
		}
	}

	// Pre-size the read buffer from the stat size (like os.ReadFile) so
	// the common in-cap read is a single allocation rather than
	// io.ReadAll's repeated grow-and-copy. Read through LimitReader(max+1)
	// regardless so a file that grew past the cap since the stat is still
	// flagged as too large.
	data, err := readAllSized(io.LimitReader(r, max+1), actualSize, max)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		reported := actualSize
		if reported < 0 {
			reported = int64(len(data))
		}
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", reported, max)
	}
	return data, nil
}

// readAllSized reads r to EOF. When sizeHint is a usable in-cap file
// size it seeds the buffer so the whole file lands in one allocation
// (mirroring os.ReadFile); otherwise it starts small and grows like
// io.ReadAll. Callers wrap r in a LimitReader, so a file that grew past
// the cap since the stat is still bounded by the +1 sentinel read.
//
// The grow loop is inlined rather than delegating to bytes.Buffer or
// io.ReadAll: both over-reserve (Buffer keeps MinRead headroom; ReadAll
// can leave up to 2x slack) or copy on the way out, whereas the goal
// here is exactly one right-sized sizeHint+1 allocation.
func readAllSized(r io.Reader, sizeHint, max int64) ([]byte, error) {
	capHint := 512
	if sizeHint >= 0 && sizeHint <= max {
		if h := sizeHint + 1; int64(int(h)) == h { // +1 for the EOF read; guard int overflow
			capHint = int(h)
		}
	}
	data := make([]byte, 0, capHint)
	for {
		if len(data) >= cap(data) {
			data = append(data, 0)[:len(data)] // grow, preserve len
		}
		n, err := r.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return data, err
		}
	}
}
