// Package main is a runnable tour of scopex: a single Guard, a Scope that
// unwinds on failure and Disarms on success, panic recovery, and the
// compile-time-checked typed variant. Run it with `make run`.
package main

import (
	"errors"
	"fmt"

	"github.com/zkrebbekx/scopex"
	"github.com/zkrebbekx/scopex/typed"
)

// conn is a stand-in for any resource that must be released — a file, a DB
// handle, a network connection.
type conn struct{ name string }

func dial(name string) (*conn, error) {
	fmt.Printf("  open  %s\n", name)
	return &conn{name: name}, nil
}

func (c *conn) close() error {
	fmt.Printf("  close %s\n", c.name)
	return nil
}

func main() {
	guardDemo()
	scopeDemo(false)
	scopeDemo(true)
	panicDemo()
	typedDemo()
}

// guardDemo: one resource, defer Close. Close is idempotent, so an explicit
// early Close plus the deferred one is fine.
func guardDemo() {
	fmt.Println("== Guard ==")
	g := scopex.Guarded(&conn{name: "cache"}, (*conn).close)
	defer func() { _ = g.Close() }()
	fmt.Printf("  using %s\n", g.Value().name)
}

// scopeDemo: build several dependent resources. If construction fails the
// Scope unwinds them LIFO; on success Disarm hands ownership onward.
func scopeDemo(fail bool) {
	fmt.Printf("== Scope (fail=%v) ==\n", fail)
	_, err := func() (*conn, error) {
		s := scopex.New()
		defer func() { _ = s.Close() }()

		db := scopex.Adopt(s, mustGuard(dial("db")))
		tx := scopex.Adopt(s, mustGuard(dial("tx")))
		_ = db
		_ = tx

		if fail {
			return nil, errors.New("build failed") // s.Close unwinds tx, db
		}
		s.Disarm() // success: caller owns them now
		return tx, nil
	}()
	if err != nil {
		fmt.Println("  ->", err)
	} else {
		fmt.Println("  -> ok, resources handed off")
	}
}

// panicDemo: a panic in the body is turned into an error and the resources are
// still released, via the deferred Recover.
func panicDemo() {
	fmt.Println("== Recover ==")
	err := func() (err error) {
		s := scopex.New()
		defer s.Recover(&err)
		scopex.Adopt(s, mustGuard(dial("session")))
		panic("handler blew up")
	}()
	var pe *scopex.PanicError
	if errors.As(err, &pe) {
		fmt.Printf("  recovered: %v\n", pe.Value)
	}
}

// typedDemo: the phantom-state guard. Reading g2 (Closed) with typed.Value
// would not compile — see the commented line.
func typedDemo() {
	fmt.Println("== typed (compile-time) ==")
	g := typed.Open(&conn{name: "stream"}, (*conn).close)
	fmt.Printf("  using %s\n", typed.Value(g).name)

	g2, _ := typed.Close(g)
	_ = g2
	// _ = typed.Value(g2) // compile error: Value needs Guard[_, Opened]
	fmt.Println("  closed; Value(g2) would not compile")
}

func mustGuard(c *conn, err error) *scopex.Guard[*conn] {
	if err != nil {
		panic(err)
	}
	return scopex.Guarded(c, (*conn).close)
}
