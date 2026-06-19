package scopex_test

import (
	"errors"
	"fmt"

	"github.com/zkrebbekx/scopex"
)

// A single Guard behaves like a smarter defer: idempotent Close, plus the
// option to Release ownership.
func ExampleGuard() {
	g := scopex.Guarded("conn", func(c string) error {
		fmt.Println("closing", c)
		return nil
	})
	defer func() { _ = g.Close() }()

	fmt.Println("using", g.Value())
	// Output:
	// using conn
	// closing conn
}

// A Scope unwinds many resources LIFO if construction fails, but Disarm hands
// them off intact when it succeeds.
func ExampleScope() {
	build := func(fail bool) error {
		s := scopex.New()
		defer func() { _ = s.Close() }()

		scopex.Adopt(s, scopex.Guarded("db", func(string) error {
			fmt.Println("rollback db")
			return nil
		}))
		scopex.Adopt(s, scopex.Guarded("tx", func(string) error {
			fmt.Println("rollback tx")
			return nil
		}))

		if fail {
			return errors.New("build failed")
		}
		s.Disarm() // success: ownership moves on, skip cleanup
		return nil
	}

	fmt.Println("-- failure path --")
	_ = build(true)
	fmt.Println("-- success path --")
	_ = build(false)
	// Output:
	// -- failure path --
	// rollback tx
	// rollback db
	// -- success path --
}
