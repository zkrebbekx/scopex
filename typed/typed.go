// Package typed is an experimental, compile-time-checked variant of the
// scopex Guard built on phantom typestate flags — the same technique as
// github.com/zkrebbekx/typestate.
//
// A [Guard] carries a phantom state type, [Opened] or [Closed]. The
// state-restricted operations are free functions that accept only the state
// they require, so reading the value of a closed guard does not compile:
//
//	g := typed.Open(f, func(f *os.File) error { return f.Close() }) // Guard[*os.File, Opened]
//	data := typed.Value(g)                                          // ok
//	g2, err := typed.Close(g)                                       // Guard[*os.File, Closed]
//	_ = typed.Value(g2)                                             // compile error: needs Opened
//
// Two Go limitations shape the design, and you should know both:
//
//   - Method sets cannot vary by type parameter, so a method defined on
//     Guard[T, S] exists for every S. The state gate therefore lives in free
//     functions ([Value], [Close], [Release]), not methods.
//   - Go has no linear/affine types, so nothing stops you from reusing the
//     pre-transition variable after [Close]. Follow the rebind idiom — assign
//     the transition's result and let the old name fall out of use — and the
//     guarantee holds where it matters most: at function boundaries, where a
//     parameter of type Guard[T, Closed] simply cannot be read.
//
// For dynamic collections, idempotent Close, leak detection, and panic
// recovery, use the main scopex package; this one trades those for a stricter
// type-level story on a single resource.
package typed

// Opened is the phantom state of a guard that still owns its resource.
type Opened struct{}

// Closed is the phantom state of a guard whose resource has been released.
type Closed struct{}

// Guard owns a resource of type T in phantom state S (either [Opened] or
// [Closed]). Construct one with [Open]; transition it with [Close] or
// [Release].
type Guard[T any, S any] struct {
	val     T
	cleanup func(T) error
	id      uint64 // leak-tracking id; 0 and unused outside scopex_debug
}

// Open wraps an acquired value and its cleanup func, yielding an Opened guard.
func Open[T any](val T, cleanup func(T) error) Guard[T, Opened] {
	return Guard[T, Opened]{val: val, cleanup: cleanup, id: registerLeak()}
}

// Value returns the resource. It accepts only an Opened guard, so reading a
// Closed guard is a compile-time error.
func Value[T any](g Guard[T, Opened]) T { return g.val }

// Close runs the cleanup func and returns the resulting Closed guard and the
// cleanup error. The returned guard cannot be passed to [Value].
func Close[T any](g Guard[T, Opened]) (Guard[T, Closed], error) {
	unregisterLeak(g.id)
	err := g.cleanup(g.val)
	return Guard[T, Closed]{val: g.val, id: g.id}, err
}

// Release transfers the resource to the caller without running cleanup, and
// returns the value alongside a Closed guard.
func Release[T any](g Guard[T, Opened]) (T, Guard[T, Closed]) {
	unregisterLeak(g.id)
	return g.val, Guard[T, Closed]{val: g.val, id: g.id}
}
