package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetLanguage(t *testing.T) {
	require := require.New(t)

	require.Equal("java", GetLanguage("foo.java", []byte(`
	package foo;
	`)))

	require.Equal("java", GetLanguage("", []byte(`
	// -*- java -*-
	package foo;
	import foo.bar;
	class Foo {
		public Foo() { }
		public int foo() {
			return 0;
		}
	}
	`)))

	require.Equal("cpp", GetLanguage("", []byte(`
	// -*- C++ -*-
	package foo;
	import foo.bar;
	class Foo {
		public Foo() { }
		public int foo() {
			return 0;
		}
	}
	`)))

	require.Equal("csharp", GetLanguage("", []byte(`
	// -*- C# -*-
	package foo;
	import foo.bar;
	class Foo {
		public Foo() { }
		public int foo() {
			return 0;
		}
	}
	`)))
}
