//go:build !scopex_debug

// This file holds the production stubs for leak tracking. They are empty and
// inline away to nothing, so a normal build pays zero cost for the debug-only
// leak detector in leak_debug.go.

package scopex

// LeakHook is a no-op outside scopex_debug builds. Build with
// `-tags scopex_debug` to enable leak detection. See [Guard].
func LeakHook(_ func(args ...any)) {}

func track[T any](_ *Guard[T])   {}
func untrack[T any](_ *Guard[T]) {}
