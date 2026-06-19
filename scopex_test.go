package scopex_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zkrebbekx/scopex"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGuardClose(t *testing.T) {
	Convey("Given a Guard wrapping a resource", t, func() {
		closed := 0
		g := scopex.Guarded("res", func(string) error {
			closed++
			return nil
		})

		Convey("When Close is called", func() {
			err := g.Close()

			Convey("Then cleanup runs once with no error", func() {
				So(err, ShouldBeNil)
				So(closed, ShouldEqual, 1)
			})

			Convey("And calling Close again is a no-op", func() {
				So(g.Close(), ShouldBeNil)
				So(closed, ShouldEqual, 1)
			})
		})
	})
}

func TestGuardClosePropagatesError(t *testing.T) {
	Convey("Given a Guard whose cleanup fails", t, func() {
		boom := errors.New("boom")
		g := scopex.Guarded(0, func(int) error { return boom })

		Convey("When Close is called", func() {
			err := g.Close()

			Convey("Then the cleanup error is returned", func() {
				So(err, ShouldEqual, boom)
			})
		})
	})
}

func TestGuardRelease(t *testing.T) {
	Convey("Given a Guard wrapping a resource", t, func() {
		closed := 0
		g := scopex.Guarded(42, func(int) error {
			closed++
			return nil
		})

		Convey("When the resource is Released", func() {
			v := g.Release()

			Convey("Then the value is returned to the caller", func() {
				So(v, ShouldEqual, 42)
			})

			Convey("And subsequent Close does not run cleanup", func() {
				So(g.Close(), ShouldBeNil)
				So(closed, ShouldEqual, 0)
			})
		})
	})
}

func TestAcquire(t *testing.T) {
	Convey("Given an acquisition that succeeds", t, func() {
		g, err := scopex.Acquire(
			func() (string, error) { return "ok", nil },
			func(string) error { return nil },
		)

		Convey("Then a Guard over the value is returned", func() {
			So(err, ShouldBeNil)
			So(g.Value(), ShouldEqual, "ok")
		})
	})

	Convey("Given an acquisition that fails", t, func() {
		fail := errors.New("nope")
		g, err := scopex.Acquire(
			func() (string, error) { return "", fail },
			func(string) error { return nil },
		)

		Convey("Then no Guard is created and the error surfaces", func() {
			So(g, ShouldBeNil)
			So(err, ShouldEqual, fail)
		})
	})
}

func TestScopeClosesLIFO(t *testing.T) {
	Convey("Given a Scope with several registered cleanups", t, func() {
		var order []int
		s := scopex.New()
		s.Defer(func() error { order = append(order, 1); return nil })
		s.Defer(func() error { order = append(order, 2); return nil })
		s.Defer(func() error { order = append(order, 3); return nil })

		Convey("When the Scope is Closed", func() {
			err := s.Close()

			Convey("Then cleanups run last-in-first-out", func() {
				So(err, ShouldBeNil)
				So(order, ShouldResemble, []int{3, 2, 1})
			})

			Convey("And a second Close is a no-op", func() {
				order = nil
				So(s.Close(), ShouldBeNil)
				So(order, ShouldBeEmpty)
			})
		})
	})
}

func TestScopeJoinsErrors(t *testing.T) {
	Convey("Given a Scope where two cleanups fail", t, func() {
		e1 := errors.New("first")
		e2 := errors.New("second")
		s := scopex.New()
		s.Defer(func() error { return e1 })
		s.Defer(func() error { return nil })
		s.Defer(func() error { return e2 })

		Convey("When the Scope is Closed", func() {
			err := s.Close()

			Convey("Then both errors are reported via errors.Join", func() {
				So(errors.Is(err, e1), ShouldBeTrue)
				So(errors.Is(err, e2), ShouldBeTrue)
			})
		})
	})
}

func TestScopeAdoptAndDisarm(t *testing.T) {
	Convey("Given a Scope that has Adopted a Guard", t, func() {
		closed := 0
		s := scopex.New()
		v := scopex.Adopt(s, scopex.Guarded("db", func(string) error {
			closed++
			return nil
		}))

		Convey("Then Adopt returns the underlying value", func() {
			So(v, ShouldEqual, "db")
		})

		Convey("When the Scope is Closed on the failure path", func() {
			So(s.Close(), ShouldBeNil)

			Convey("Then the adopted resource is cleaned up", func() {
				So(closed, ShouldEqual, 1)
			})
		})

		Convey("When the Scope is Disarmed on the success path", func() {
			s.Disarm()
			So(s.Close(), ShouldBeNil)

			Convey("Then no cleanup runs", func() {
				So(closed, ShouldEqual, 0)
			})
		})
	})
}

func TestScopeRecoverFromPanic(t *testing.T) {
	Convey("Given a function whose body panics under a Scope", t, func() {
		cleaned := 0
		run := func() (err error) {
			s := scopex.New()
			defer s.Recover(&err)
			s.Defer(func() error { cleaned++; return nil })
			panic("kaboom")
		}

		Convey("When it is called", func() {
			err := run()

			Convey("Then the panic becomes a *PanicError", func() {
				var pe *scopex.PanicError
				So(errors.As(err, &pe), ShouldBeTrue)
				So(pe.Value, ShouldEqual, "kaboom")
				So(len(pe.Stack), ShouldBeGreaterThan, 0)
			})

			Convey("And cleanup still ran", func() {
				So(cleaned, ShouldEqual, 1)
			})
		})
	})
}

func TestScopeRecoverUnwrapsErrorPanic(t *testing.T) {
	Convey("Given a panic carrying an error value", t, func() {
		sentinel := errors.New("sentinel")
		run := func() (err error) {
			s := scopex.New()
			defer s.Recover(&err)
			panic(sentinel)
		}

		Convey("When recovered", func() {
			err := run()

			Convey("Then errors.Is finds the original error through the wrapper", func() {
				So(errors.Is(err, sentinel), ShouldBeTrue)
			})
		})
	})
}

func TestScopeRecoverNoPanicLeavesErr(t *testing.T) {
	Convey("Given no panic and clean cleanup", t, func() {
		run := func() (err error) {
			s := scopex.New()
			defer s.Recover(&err)
			s.Defer(func() error { return nil })
			return nil
		}

		Convey("When called", func() {
			err := run()

			Convey("Then err stays nil", func() {
				So(err, ShouldBeNil)
			})
		})
	})

	Convey("Given no panic but a failing cleanup", t, func() {
		boom := errors.New("cleanup failed")
		run := func() (err error) {
			s := scopex.New()
			defer s.Recover(&err)
			s.Defer(func() error { return boom })
			return nil
		}

		Convey("When called", func() {
			err := run()

			Convey("Then the cleanup error surfaces", func() {
				So(errors.Is(err, boom), ShouldBeTrue)
			})
		})
	})
}

func TestCloseOnCancel(t *testing.T) {
	Convey("Given a Scope bound to a cancellable context", t, func() {
		cleaned := make(chan struct{}, 1)
		s := scopex.New()
		s.Defer(func() error { cleaned <- struct{}{}; return nil })
		ctx, cancel := context.WithCancel(context.Background())
		stop := s.CloseOnCancel(ctx)
		defer stop()

		Convey("When the context is cancelled", func() {
			cancel()

			Convey("Then the Scope is closed by the watcher", func() {
				select {
				case <-cleaned:
				case <-time.After(time.Second):
					t.Fatal("cleanup did not run on cancel")
				}
			})
		})
	})

	Convey("Given a bound Scope closed manually before cancel", t, func() {
		count := 0
		s := scopex.New()
		s.Defer(func() error { count++; return nil })
		ctx, cancel := context.WithCancel(context.Background())
		stop := s.CloseOnCancel(ctx)

		Convey("When stop and Close run, then the context cancels", func() {
			stop()
			So(s.Close(), ShouldBeNil)
			cancel()
			time.Sleep(20 * time.Millisecond) // let any stray watcher run

			Convey("Then cleanup ran exactly once (idempotent)", func() {
				So(count, ShouldEqual, 1)
			})
		})
	})
}
