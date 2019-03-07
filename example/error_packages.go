package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	raven "github.com/getsentry/raven-go"
	goErrors "github.com/go-errors/errors"
	pingcapErrors "github.com/pingcap/errors"
	pkgErrors "github.com/pkg/errors"
)

//==============================
// https://github.com/pkg/errors
//==============================

func pkgBar() error {
	return pkgErrors.New("this is bad from pkgErrors")
}

func pkgFoo() error {
	return pkgBar()
}

//==================================
// https://github.com/pingcap/errors
//==================================

func pingcapBar() error {
	return pingcapErrors.New("this is bad from pingcapErrors")
}

func pingcapFoo() error {
	return pingcapBar()
}

//====================================
// https://github.com/go-errors/errors
//====================================

func goErrorsBar() error {
	return goErrors.New(goErrors.Errorf("this is bad from goErrors"))
}

func goErrorsFoo() error {
	return goErrorsBar()
}

//==============================

func main() {
	pkgErr := pkgFoo()
	pkgStacktrace := raven.GetOrNewStacktrace(pkgErr, 0, 3, []string{})
	spew.Dump(pkgStacktrace)
	spew.Dump(len(pkgStacktrace.Frames))

	fmt.Print("\n\n\n")

	pingcapErr := pingcapFoo()
	pingcapStacktrace := raven.GetOrNewStacktrace(pingcapErr, 0, 3, []string{})
	spew.Dump(pingcapStacktrace)
	spew.Dump(len(pingcapStacktrace.Frames))

	fmt.Print("\n\n\n")

	goErrorsErr := goErrorsFoo()
	goErrorsStacktrace := raven.GetOrNewStacktrace(goErrorsErr, 0, 3, []string{})
	spew.Dump(goErrorsStacktrace)
	spew.Dump(len(goErrorsStacktrace.Frames))
}
