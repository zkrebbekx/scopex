// Package scopex provides composable scoped resource management for Go: a
// defer that nests, transfers ownership, aggregates cleanup errors, and — in
// test builds — catches resources you forgot to close.
//
// Two types do the work:
//
//   - [Guard] owns a single resource plus the func that releases it. Close is
//     idempotent; Release hands ownership out so the Guard stops managing it.
//   - [Scope] collects many cleanups and runs them last-in-first-out, joining
//     any errors. Disarm cancels a Scope on the success path, after ownership
//     has moved elsewhere.
//
// The hard case plain defer handles badly — "acquire N things, return them on
// success, but unwind everything if step K fails" — is a Scope plus a deferred
// Close and a Disarm at the end:
//
//	func setup() (*Server, error) {
//		s := scopex.New()
//		defer s.Close() // unwinds everything UNLESS we Disarm
//
//		db := scopex.Adopt(s, mustGuard(openDB()))
//		tx := scopex.Adopt(s, mustGuard(db.Begin()))
//
//		srv, err := buildServer(db, tx)
//		if err != nil {
//			return nil, err // s.Close() runs: tx then db
//		}
//		s.Disarm() // success: srv owns them now
//		return srv, nil
//	}
//
// Leak detection is gated behind the scopex_debug build tag and compiles to
// inlined no-ops otherwise, so production binaries pay nothing. See
// [LeakHook].
//
// Library is zero-dependency (goconvey is a test-only dependency). Go 1.24+.
package scopex

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
)

// Guard owns a resource of type T together with the function that releases
// it. The zero Guard is not usable; construct one with [Guarded] or
// [Acquire].
//
// Close is idempotent and is a no-op once the resource has been Released.
type Guard[T any] struct {
	val      T
	cleanup  func(T) error
	released bool
	closed   bool
}

// Guarded wraps an already-acquired value with the func that releases it.
//
//	g := scopex.Guarded(f, func(f *os.File) error { return f.Close() })
//	defer g.Close()
func Guarded[T any](val T, cleanup func(T) error) *Guard[T] {
	g := &Guard[T]{val: val, cleanup: cleanup}
	track(g)
	return g
}

// Acquire runs acquire and, on success, returns a Guard wrapping the result.
// If acquire fails, no Guard is created and the error is returned as-is.
//
//	g, err := scopex.Acquire(
//		func() (*os.File, error) { return os.Open(path) },
//		func(f *os.File) error { return f.Close() },
//	)
func Acquire[T any](acquire func() (T, error), cleanup func(T) error) (*Guard[T], error) {
	v, err := acquire()
	if err != nil {
		return nil, err
	}
	return Guarded(v, cleanup), nil
}

// Value returns the guarded resource. It remains valid after Release; the
// only change Release makes is that the Guard will no longer clean it up.
func (g *Guard[T]) Value() T { return g.val }

// Close runs the cleanup func exactly once and returns its error. It is a
// no-op (returning nil) if the Guard has already been closed or Released.
func (g *Guard[T]) Close() error {
	if g.closed || g.released {
		return nil
	}
	g.closed = true
	untrack(g)
	return g.cleanup(g.val)
}

// Release transfers ownership of the resource to the caller and returns it.
// After Release the Guard will not clean up, and the resource will not be
// reported as a leak. Use it to hand a resource to a longer-lived owner.
func (g *Guard[T]) Release() T {
	g.released = true
	untrack(g)
	return g.val
}

// Scope collects cleanup funcs and runs them last-in-first-out, which matches
// the order in which dependent resources should be torn down. It is safe for
// concurrent use and must not be copied; obtain one from [New].
type Scope struct {
	mu      sync.Mutex
	closers []func() error
}

// New returns an empty Scope.
func New() *Scope { return &Scope{} }

// Defer registers a cleanup func to run when the Scope is closed. Funcs run in
// reverse registration order.
func (s *Scope) Defer(cleanup func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closers = append(s.closers, cleanup)
}

// Adopt registers g's Close with the Scope and returns the guarded value, so a
// resource can be acquired and scoped in one expression:
//
//	db := scopex.Adopt(s, mustGuard(openDB()))
func Adopt[T any](s *Scope, g *Guard[T]) T {
	s.Defer(g.Close)
	return g.Value()
}

// Close runs every registered cleanup last-in-first-out and returns their
// errors joined with [errors.Join] (nil if all succeeded). Close is idempotent:
// a second call has nothing left to run.
func (s *Scope) Close() error {
	s.mu.Lock()
	closers := s.closers
	s.closers = nil
	s.mu.Unlock()

	var errs []error
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i](); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Disarm cancels all pending cleanups without running them. Call it on the
// success path once ownership of the scoped resources has moved to a
// longer-lived owner, so the deferred Close becomes a no-op.
func (s *Scope) Disarm() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closers = nil
}

// CloseOnCancel ties the Scope's lifetime to ctx: it starts a goroutine that
// calls Close when ctx is done. It returns a stop func that ends the watcher
// without closing the Scope — defer it so the goroutine does not outlive a
// manual Close:
//
//	s := scopex.New()
//	defer s.Close()
//	defer s.CloseOnCancel(ctx)() // close early if the request is cancelled
//
// Close is idempotent and concurrency-safe, so the watcher firing and a manual
// Close racing is harmless: whichever runs first wins, the other is a no-op.
func (s *Scope) CloseOnCancel(ctx context.Context) (stop func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-done:
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// PanicError wraps a value recovered from a panic by [Scope.Recover], together
// with the stack at the point of recovery. If the recovered value was itself
// an error, Unwrap exposes it so errors.Is / errors.As keep working through
// it.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("scopex: recovered panic: %v", e.Value)
}

// Unwrap returns the recovered value if it was an error, else nil.
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// Recover is deferred to make a panicking function clean up and report the
// panic as an error instead of crashing the process. It recovers any in-flight
// panic, always runs Close, and writes the combined result through errp, which
// must point at a named return value:
//
//	func setup() (err error) {
//		s := scopex.New()
//		defer s.Recover(&err)
//
//		db := scopex.Adopt(s, mustGuard(openDB()))
//		mightPanic(db) // panic here still rolls back db, returns *PanicError
//		s.Disarm()
//		return nil
//	}
//
// A recovered panic is wrapped in [*PanicError]; it, any error already in errp,
// and any cleanup errors are joined with [errors.Join]. When there is neither a
// panic nor a cleanup error, errp is left untouched.
func (s *Scope) Recover(errp *error) {
	r := recover()
	cerr := s.Close()
	if r == nil && cerr == nil {
		return
	}
	var perr error
	if r != nil {
		perr = &PanicError{Value: r, Stack: debug.Stack()}
	}
	*errp = errors.Join(perr, *errp, cerr)
}
