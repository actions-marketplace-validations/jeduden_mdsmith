//go:build windows

package main_test

// unixProcessAlive is a Windows stub. The timeout/process-group test that
// calls it skips on Windows at runtime; the stub only satisfies the
// compiler so the shared hardening test file builds on all platforms.
func unixProcessAlive(int) bool { return false }
