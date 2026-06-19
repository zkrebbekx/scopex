//go:build !scopex_debug

// Production stubs for typed leak tracking: empty and inlined away, so a normal
// build pays nothing for the debug-only registry in leak_debug.go.

package typed

func registerLeak() uint64    { return 0 }
func unregisterLeak(_ uint64) {}

// CheckLeaks returns nil outside scopex_debug builds. Build with
// `-tags scopex_debug` to enable leak detection.
func CheckLeaks() []string { return nil }

// ResetLeaks is a no-op outside scopex_debug builds.
func ResetLeaks() {}
