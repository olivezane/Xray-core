package aead

import (
	"reflect"
	"testing"
)

func requireEqual(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func requireNil(t *testing.T, got any) {
	t.Helper()
	if !isNil(got) {
		t.Fatalf("got %v, want nil", got)
	}
}

func requireNotNil(t *testing.T, got any) {
	t.Helper()
	if isNil(got) {
		t.Fatal("got nil")
	}
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
