package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zkrebbekx/scopex"

	. "github.com/smartystreets/goconvey/convey"
)

// tempFile creates a file and returns a Guard whose cleanup deletes it, so we
// can observe scopex tearing real resources down.
func tempFile(t *testing.T, dir, name string) *scopex.Guard[string] {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return scopex.Guarded(path, os.Remove)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestScopeUnwindsRealFiles(t *testing.T) {
	Convey("Given two files acquired into a Scope", t, func() {
		dir := t.TempDir()
		s := scopex.New()
		a := scopex.Adopt(s, tempFile(t, dir, "a"))
		b := scopex.Adopt(s, tempFile(t, dir, "b"))
		So(exists(a), ShouldBeTrue)
		So(exists(b), ShouldBeTrue)

		Convey("When the Scope is Closed (failure path)", func() {
			So(s.Close(), ShouldBeNil)

			Convey("Then both files are removed", func() {
				So(exists(a), ShouldBeFalse)
				So(exists(b), ShouldBeFalse)
			})
		})

		Convey("When the Scope is Disarmed (success path)", func() {
			s.Disarm()
			So(s.Close(), ShouldBeNil)

			Convey("Then both files survive", func() {
				So(exists(a), ShouldBeTrue)
				So(exists(b), ShouldBeTrue)
			})
		})
	})
}

func TestRecoverCleansRealFile(t *testing.T) {
	Convey("Given a function that panics after acquiring a file", t, func() {
		dir := t.TempDir()
		var path string
		run := func() (err error) {
			s := scopex.New()
			defer s.Recover(&err)
			path = scopex.Adopt(s, tempFile(t, dir, "session"))
			panic("boom")
		}

		Convey("When it runs", func() {
			err := run()

			Convey("Then the panic is captured and the file is gone", func() {
				var pe *scopex.PanicError
				So(errors.As(err, &pe), ShouldBeTrue)
				So(exists(path), ShouldBeFalse)
			})
		})
	})
}
