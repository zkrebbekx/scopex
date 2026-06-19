//go:build scopex_debug

// Leak tracking for the typed Guard. A typed Guard is a value, not a pointer,
// so there is nothing to attach a finalizer to. Instead each Open registers an
// id with the alloc-site stack, and Close/Release unregister it. Whatever is
// still registered when you call CheckLeaks was never closed — a deterministic
// check, no GC required. Enable with `-tags scopex_debug`.

package typed

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	leakSeq  atomic.Uint64
	leakMu   sync.Mutex
	leakOpen = map[uint64]string{} // id -> acquisition stack
)

func registerLeak() uint64 {
	id := leakSeq.Add(1)
	stack := captureStack()
	leakMu.Lock()
	leakOpen[id] = stack
	leakMu.Unlock()
	return id
}

func unregisterLeak(id uint64) {
	leakMu.Lock()
	delete(leakOpen, id)
	leakMu.Unlock()
}

// CheckLeaks returns one report per guard opened but never Closed or Released.
// Each report includes the acquisition stack. In normal builds it returns nil.
// Call it at the end of a test (after ResetLeaks at the start) to fail on
// leaked guards:
//
//	func TestMain(m *testing.M) {
//		typed.ResetLeaks()
//		code := m.Run()
//		if leaks := typed.CheckLeaks(); len(leaks) > 0 {
//			fmt.Println(leaks)
//			os.Exit(1)
//		}
//		os.Exit(code)
//	}
func CheckLeaks() []string {
	leakMu.Lock()
	defer leakMu.Unlock()
	var out []string
	for _, stack := range leakOpen {
		out = append(out, fmt.Sprintf("typed: leaked Guard, never Closed or Released\nacquired at:\n%s", stack))
	}
	return out
}

// ResetLeaks clears the leak registry. In normal builds it is a no-op.
func ResetLeaks() {
	leakMu.Lock()
	leakOpen = map[uint64]string{}
	leakMu.Unlock()
}

func captureStack() string {
	var pcs [16]uintptr
	// Skip captureStack, registerLeak, Open.
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
