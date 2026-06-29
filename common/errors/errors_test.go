package errors_test

import (
	"io"
	"strings"
	"testing"

	. "github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/log"
)

func TestError(t *testing.T) {
	err := New("TestError")
	if v := GetSeverity(err); v != log.Severity_Info {
		t.Error("severity: ", v)
	}

	err = New("TestError2").Base(io.EOF)
	if v := GetSeverity(err); v != log.Severity_Info {
		t.Error("severity: ", v)
	}

	err = New("TestError3").Base(io.EOF).AtWarning()
	if v := GetSeverity(err); v != log.Severity_Warning {
		t.Error("severity: ", v)
	}

	err = New("TestError4").Base(io.EOF).AtWarning()
	err = New("TestError5").Base(err)
	if v := GetSeverity(err); v != log.Severity_Warning {
		t.Error("severity: ", v)
	}
	if v := err.Error(); !strings.Contains(v, "EOF") {
		t.Error("error: ", v)
	}
}

func TestErrorMessage(t *testing.T) {
	err := New("a").Base(New("b"))
	s := err.Error()
	if !strings.Contains(s, ": a >") || !strings.HasSuffix(s, ": b") {
		t.Error("unexpected error format: ", s)
	}
}
