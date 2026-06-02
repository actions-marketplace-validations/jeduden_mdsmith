package release

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOsRunner_RunCommand drives the production Runner against
// real, harmless binaries: a zero-exit command returns nil, a
// non-zero exit returns *exec.ExitError, and a missing binary
// returns a not-found error before any process starts.
func TestOsRunner_RunCommand(t *testing.T) {
	r := osRunner{}

	t.Run("zero exit returns nil", func(t *testing.T) {
		// `true` exits 0; cwd "" inherits the test process cwd.
		require.NoError(t, r.RunCommand("", "true"))
	})

	t.Run("non-zero exit returns ExitError", func(t *testing.T) {
		err := r.RunCommand("", "false")
		require.Error(t, err)
		var exitErr *exec.ExitError
		assert.True(t, errors.As(err, &exitErr),
			"a non-zero exit must surface as *exec.ExitError, got %T", err)
	})

	t.Run("runs in the requested directory", func(t *testing.T) {
		// `test -f marker` succeeds only when cmd.Dir is honoured:
		// the marker exists only inside the temp dir.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o644))
		assert.NoError(t, r.RunCommand(dir, "test", "-f", "marker"))
		// From a different cwd the same relative test fails.
		other := t.TempDir()
		assert.Error(t, r.RunCommand(other, "test", "-f", "marker"))
	})

	t.Run("missing binary returns an error", func(t *testing.T) {
		err := r.RunCommand("", "mdsmith-no-such-binary-on-path-xyz")
		require.Error(t, err)
		assert.ErrorIs(t, err, exec.ErrNotFound)
	})
}

// TestOsHTTPGetter_Get stands up an in-process httptest server to
// cover every branch of the production HTTP surface: a 200 returns
// the status and the fully-read body, a non-2xx status is returned
// verbatim with a nil error (callers decide per-asset), a
// truncated body surfaces a wrapped read error, and a refused
// connection returns status 0 with a transport error.
func TestOsHTTPGetter_Get(t *testing.T) {
	g := osHTTPGetter{}

	t.Run("200 returns status and body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("HELLO-BODY"))
		}))
		defer srv.Close()
		status, body, err := g.Get(srv.URL)
		require.NoError(t, err)
		assert.Equal(t, 200, status)
		assert.Equal(t, "HELLO-BODY", string(body))
	})

	t.Run("non-200 returns status and body with nil error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		}))
		defer srv.Close()
		status, body, err := g.Get(srv.URL)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Contains(t, string(body), "missing")
	})

	t.Run("truncated body surfaces a read error", func(t *testing.T) {
		// Promise 100 bytes via Content-Length but write 5 and slam
		// the connection shut, so io.ReadAll trips on a short read.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(200)
			_, _ = w.Write([]byte("short"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
				}
			}
		}))
		defer srv.Close()
		status, body, err := g.Get(srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read body")
		assert.Equal(t, 200, status, "the status is still surfaced on a read failure")
		assert.Nil(t, body)
	})

	t.Run("refused connection returns status 0 and a transport error", func(t *testing.T) {
		// Bind a port, learn its address, then close the listener so
		// the subsequent dial is refused without touching the network.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		url := "http://" + ln.Addr().String()
		require.NoError(t, ln.Close())
		status, body, err := g.Get(url)
		require.Error(t, err)
		assert.Equal(t, 0, status)
		assert.Nil(t, body)
	})
}
