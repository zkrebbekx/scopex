//go:build scopex_debug

// This file is compiled only under `-tags scopex_debug`. It attaches a
// finalizer to every Guard so that a resource garbage-collected while still
// open (never Closed, never Released) is reported as a leak. Run your tests
// with:
//
//	go test -tags scopex_debug ./...
//
// and register a reporter (e.g. t.Error) with LeakHook.

package scopex

import (
	"fmt"
	"os"
	"runtime"
	"sync"
)

var (
	hookMu sync.RWMutex
	hook   = func(args ...any) {
		fmt.Fprintln(os.Stderr, args...)
	}
)

// LeakHook sets the function called when a Guard is finalized while still
// open. It is only meaningful in scopex_debug builds; in normal builds it is a
// no-op. Pass t.Error to fail the test that leaked:
//
//	scopex.LeakHook(t.Error)
//
// The argument signature matches testing.T.Error / log.Print.
func LeakHook(fn func(args ...any)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	hook = fn
}

func track[T any](g *Guard[T]) {
	stack := captureStack()
	runtime.SetFinalizer(g, func(x *Guard[T]) {
		if x.closed || x.released {
			return
		}
		hookMu.RLock()
		fn := hook
		hookMu.RUnlock()
		fn(fmt.Sprintf("scopex: leaked Guard, never Closed or Released\nacquired at:\n%s", stack))
	})
}

func untrack[T any](g *Guard[T]) {
	runtime.SetFinalizer(g, nil)
}

func captureStack() string {
	var pcs [16]uintptr
	// Skip captureStack, track, Guarded.
	n := runtime.Callers(4, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	var b []byte
	for {
		f, more := frames.Next()
		b = append(b, fmt.Sprintf("\t%s\n\t\t%s:%d\n", f.Function, f.File, f.Line)...)
		if !more {
			break
		}
	}
	return string(b)
}
