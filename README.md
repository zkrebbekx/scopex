# scopex

Composable scoped resource management for Go — a `defer` that **nests,
transfers ownership, joins cleanup errors, and catches leaks in tests**.

`defer` is great for one resource in one function. It gets awkward the moment
you have several dependent resources, conditional cleanup, or you want to build
something up and hand ownership out on success but unwind everything on
failure. `scopex` covers exactly those cases without reaching for `RAII`
folklore or fighting the garbage collector.

Library is zero-dependency (goconvey is test-only). Go 1.24+.

```go
import "github.com/zkrebbekx/scopex"
```

## Guard — one resource, smarter than defer

A `Guard[T]` owns a value plus the func that releases it. `Close` is
idempotent; `Release` hands ownership out so the Guard stops managing it.

```go
g, err := scopex.Acquire(
    func() (*os.File, error) { return os.Open("cfg.json") },
    func(f *os.File) error { return f.Close() },
)
if err != nil {
    return err
}
defer g.Close() // safe to also call earlier; runs once

use(g.Value())
```

Already have the value? Wrap it directly:

```go
g := scopex.Guarded(conn, func(c *Conn) error { return c.Close() })
```

## Scope — many resources, LIFO unwind, ownership transfer

The case plain `defer` handles badly: acquire N things, return them on success,
but tear everything down if any step fails. A `Scope` collects cleanups and
runs them last-in-first-out; `Disarm` cancels them on the success path.

```go
func setup() (*Server, error) {
    s := scopex.New()
    defer s.Close() // unwinds everything UNLESS we Disarm below

    db := scopex.Adopt(s, mustGuard(openDB()))
    tx := scopex.Adopt(s, mustGuard(db.Begin()))
    cache := scopex.Adopt(s, mustGuard(openCache()))

    srv, err := buildServer(db, tx, cache)
    if err != nil {
        return nil, err // s.Close() runs: cache, tx, db — in that order
    }

    s.Disarm() // success: srv owns them now, skip cleanup
    return srv, nil
}
```

`Close` returns every cleanup error joined with `errors.Join`, so nothing is
silently swallowed.

## Panic recovery

A panic already runs deferred `Close` on the way up, so resources are released
either way. `Scope.Recover` goes further: it turns the panic into an error,
runs cleanup, and reports both — so a blown-up handler returns instead of
crashing the process.

```go
func setup() (err error) {
    s := scopex.New()
    defer s.Recover(&err) // errp must be a named return

    db := scopex.Adopt(s, mustGuard(openDB()))
    mightPanic(db) // panic here still rolls back db, returns *PanicError
    s.Disarm()
    return nil
}
```

The recovered value is wrapped in `*PanicError` (with a stack); if it was
itself an `error`, `errors.Is`/`errors.As` see through it.

## Context cancellation

`Scope.CloseOnCancel` ties a Scope's lifetime to a `context.Context`: a watcher
goroutine closes the Scope when the context is done. It returns a stop func so
the goroutine never outlives a manual Close. Close is idempotent and
concurrency-safe, so the cancel-driven close and a manual close race harmlessly.

```go
s := scopex.New()
defer s.Close()
defer s.CloseOnCancel(ctx)() // release early if the request is cancelled
```

## Compile-time variant — `scopex/typed`

The `typed` subpackage is an experimental, stricter take built on phantom
typestate flags (the same technique as
[typestate](https://github.com/zkrebbekx/typestate)). A guard carries its state
in the type, so reading a closed guard does not compile:

```go
g := typed.Open(f, closeFile) // Guard[*os.File, Opened]
data := typed.Value(g)        // ok
g2, _ := typed.Close(g)       // Guard[*os.File, Closed]
_ = typed.Value(g2)           // compile error: Value needs Opened
```

Honest about Go's limits: method sets can't vary by type parameter (so the
state-gated ops are free functions), and Go has no linear types (nothing stops
you reusing the old variable after a transition). The guarantee bites hardest
at function boundaries — a `Guard[T, Closed]` parameter simply can't be read.
Use core `scopex` for collections, idempotent close, and panic recovery; reach
for `typed` when you want the type-level story on one resource.

A typed `Guard` is a value, not a pointer, so leak detection here is a
deterministic registry rather than a finalizer. Under `-tags scopex_debug`,
every `Open` registers and every `Close`/`Release` unregisters; `CheckLeaks`
returns the acquisition stacks of anything still open:

```go
func TestMain(m *testing.M) {
    typed.ResetLeaks()
    code := m.Run()
    if leaks := typed.CheckLeaks(); len(leaks) > 0 {
        fmt.Println(leaks)
        os.Exit(1)
    }
    os.Exit(code)
}
```

## Examples

A runnable tour lives in [`examples/`](./examples) — Guard, Scope
unwind/Disarm, panic recovery, and the typed variant:

```
cd examples && make run
```

## Leak detection (test builds only)

Build with the `scopex_debug` tag and a finalizer is attached to every Guard.
Any Guard garbage-collected while still open — never `Close`d, never
`Release`d — is reported through a hook you choose. In normal builds the
tracking compiles to inlined no-ops, so production pays nothing.

```go
func TestNoLeaks(t *testing.T) {
    scopex.LeakHook(t.Error) // run: go test -tags scopex_debug

    doWork() // forgets to Close a Guard somewhere

    // Finalizers run asynchronously, so a leak is reported only after the
    // Guard is collected. Drive GC and give the finalizer goroutine a moment.
    for i := 0; i < 10; i++ {
        runtime.GC()
        time.Sleep(10 * time.Millisecond)
    }
}
```

Detection is **best-effort**: it relies on the GC reclaiming the leaked Guard,
so treat a report as a strong signal, not the absence of one as proof. Run the
whole suite under the tag with:

```
make testleak   # go test -tags scopex_debug -race ./...
```

## When to use it

- **Use `Guard`** when one resource needs idempotent close or ownership transfer.
- **Use `Scope`** when several resources are built together and must unwind as a
  unit on failure but survive on success.
- **Skip it** for the simple single-resource case where one `defer x.Close()`
  already says everything. `scopex` is progressive enhancement, not a
  replacement for `defer`.

## Design

- Zero allocation on the happy path beyond the slice a `Scope` grows.
- No `panic`-based control flow, no reflection, no codegen.
- Leak detector is strictly opt-in and build-tag isolated.

## License

MIT
