package raven

import (
	"runtime"
	"strings"
	"testing"
)

var functionNameTests = []struct {
	skip int
	pack string
	name string
}{
	{0, "github.com/cupcake/raven-go", "TestFunctionName"},
	{1, "testing", "tRunner"},
	{2, "runtime", "goexit"},
	{100, "", ""},
}

func TestFunctionName(t *testing.T) {
	for _, test := range functionNameTests {
		pc, _, _, _ := runtime.Caller(test.skip)
		pack, name := functionName(pc)

		if pack != test.pack {
			t.Errorf("incorrect package; got %s, want %s", pack, test.pack)
		}
		if name != test.name {
			t.Errorf("incorrect function; got %s, want %s", name, test.name)
		}
	}
}

func TestStacktrace(t *testing.T) {
	st := trace()
	if st == nil {
		t.Error("got nil stacktrace")
	}
	if len(st.Frames) == 0 {
		t.Error("got zero frames")
	}

	f := st.Frames[len(st.Frames)-1]
	filepath := "src/github.com/cupcake/raven-go/stacktrace_test.go"
	if f.Filename != filepath {
		t.Errorf("incorrect Filename; got %s, want %s", f.Filename, filepath)
	}
	if !strings.HasSuffix(f.AbsolutePath, filepath) {
		t.Error("incorrect AbsolutePath:", f.AbsolutePath)
	}
	if f.Function != "trace" {
		t.Error("incorrect Function:", f.Function)
	}
	if f.Module != "github.com/cupcake/raven-go" {
		t.Error("incorrect Module:", f.Module)
	}
	if f.Lineno < 50 {
		t.Error("incorrect Lineno:", f.Lineno)
	}
	if f.ContextLine != "\treturn NewStacktrace(0, 2, []string{\"github.com/cupcake/raven-go\"})" {
		t.Errorf("incorrect ContextLine: %#v", f.ContextLine)
	}
	if len(f.PreContext) != 2 || f.PreContext[0] != "// a" || f.PreContext[1] != "func trace() *Stacktrace {" {
		t.Errorf("incorrect PreContext %#v", f.PreContext)
	}
	if len(f.PostContext) != 2 || f.PostContext[0] != "\t// b" || f.PostContext[1] != "}" {
		t.Errorf("incorrect PostContext %#v", f.PostContext)
	}
	if !*f.InApp {
		t.Error("expected InApp to be true")
	}

	if st.Culprit() != "github.com/cupcake/raven-go.trace" {
		t.Error("incorrect Culprit:", st.Culprit())
	}
}

// a
func trace() *Stacktrace {
	return NewStacktrace(0, 2, []string{"github.com/cupcake/raven-go"})
	// b
}
