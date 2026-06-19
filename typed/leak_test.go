//go:build scopex_debug

package typed_test

import (
	"testing"

	"github.com/zkrebbekx/scopex/typed"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLeakDetection(t *testing.T) {
	Convey("Given a fresh leak registry", t, func() {
		typed.ResetLeaks()

		Convey("When a guard is opened and never closed", func() {
			_ = typed.Open(1, func(int) error { return nil })

			Convey("Then CheckLeaks reports it", func() {
				leaks := typed.CheckLeaks()
				So(len(leaks), ShouldEqual, 1)
				So(leaks[0], ShouldContainSubstring, "leaked Guard")
			})
		})

		Convey("When a guard is opened and Closed", func() {
			g := typed.Open(1, func(int) error { return nil })
			_, _ = typed.Close(g)

			Convey("Then nothing leaks", func() {
				So(typed.CheckLeaks(), ShouldBeEmpty)
			})
		})

		Convey("When a guard is opened and Released", func() {
			g := typed.Open(1, func(int) error { return nil })
			_, _ = typed.Release(g)

			Convey("Then nothing leaks", func() {
				So(typed.CheckLeaks(), ShouldBeEmpty)
			})
		})
	})
}
