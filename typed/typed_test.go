package typed_test

import (
	"errors"
	"testing"

	"github.com/zkrebbekx/scopex/typed"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOpenAndValue(t *testing.T) {
	Convey("Given an Opened guard", t, func() {
		g := typed.Open("res", func(string) error { return nil })

		Convey("When Value is read", func() {
			v := typed.Value(g)

			Convey("Then it returns the resource", func() {
				So(v, ShouldEqual, "res")
			})
		})
	})
}

func TestCloseTransitions(t *testing.T) {
	Convey("Given an Opened guard", t, func() {
		closed := 0
		g := typed.Open(7, func(int) error { closed++; return nil })

		Convey("When Close is called", func() {
			_, err := typed.Close(g)

			Convey("Then cleanup runs and no error is returned", func() {
				So(err, ShouldBeNil)
				So(closed, ShouldEqual, 1)
			})
		})
	})
}

func TestCloseReturnsCleanupError(t *testing.T) {
	Convey("Given a guard whose cleanup fails", t, func() {
		boom := errors.New("boom")
		g := typed.Open(0, func(int) error { return boom })

		Convey("When Close is called", func() {
			_, err := typed.Close(g)

			Convey("Then the cleanup error surfaces", func() {
				So(err, ShouldEqual, boom)
			})
		})
	})
}

func TestReleaseTransfersWithoutCleanup(t *testing.T) {
	Convey("Given an Opened guard", t, func() {
		closed := 0
		g := typed.Open(42, func(int) error { closed++; return nil })

		Convey("When Release is called", func() {
			v, _ := typed.Release(g)

			Convey("Then the value is returned and cleanup did not run", func() {
				So(v, ShouldEqual, 42)
				So(closed, ShouldEqual, 0)
			})
		})
	})
}
