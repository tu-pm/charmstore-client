// Copyright 2015 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	qt "github.com/frankban/quicktest"
)

// TODO factor the RunSuite function into a separate public repo
// somewhere.

// RunSuite runs all methods on the given value that have the
// prefix "Test". The signature of the test methods must be
// func(*quicktest.C).
//
// If suite is is a pointer, the value pointed to is copied
// before any methods are invoked on it; a new copy
// is made for each test.
//
// If there is an Init(*quicktest.C) method, it will be
// invoked before each test method runs.
func RunSuite(c *qt.C, suite interface{}) {
	sv := reflect.ValueOf(suite)
	st := sv.Type()
	init, hasInit := st.MethodByName("Init")
	if hasInit && !isValidMethod(init) {
		c.Fatal("wrong signature for Init, must be Init(*quicktest.C)")
	}
	for i := 0; i < st.NumMethod(); i++ {
		m := st.Method(i)
		if !isTestMethod(m) {
			continue
		}
		c.Run(m.Name, func(c *qt.C) {
			if !isValidMethod(m) {
				c.Fatalf("wrong signature for %s, must be %s(*quicktest.C)", m.Name, m.Name)
			}

			sv := sv
			if st.Kind() == reflect.Ptr {
				sv1 := reflect.New(st.Elem())
				sv1.Elem().Set(sv.Elem())
				sv = sv1
			}
			args := []reflect.Value{sv, reflect.ValueOf(c)}
			if hasInit {
				init.Func.Call(args)
			}
			m.Func.Call(args)
		})
	}
}

var cType = reflect.TypeOf(&qt.C{})

func isTestMethod(m reflect.Method) bool {
	if !strings.HasPrefix(m.Name, "Test") {
		return false
	}
	r, n := utf8.DecodeRuneInString(m.Name[4:])
	if n > 0 && unicode.IsLower(r) {
		return false
	}
	return true
}

func isValidMethod(m reflect.Method) bool {
	if m.Type.NumIn() != 2 || m.Type.NumOut() != 0 {
		return false
	}
	if m.Type.In(1) != cType {
		return false
	}
	return true
}
