package main

import (
	"bytes"
	"testing"
)

func TestPickTrayIconPrefersICOOnWindows(t *testing.T) {
	pngBytes := []byte("png")
	icoBytes := []byte("ico")

	got := pickTrayIcon("windows", pngBytes, icoBytes)
	if !bytes.Equal(got, icoBytes) {
		t.Fatalf("expected windows ico bytes, got %q", string(got))
	}
}

func TestPickTrayIconFallsBackWhenWindowsICOIsMissing(t *testing.T) {
	pngBytes := []byte("png")

	got := pickTrayIcon("windows", pngBytes, nil)
	if !bytes.Equal(got, pngBytes) {
		t.Fatalf("expected png fallback bytes, got %q", string(got))
	}
}

func TestPickTrayIconUsesPNGOnNonWindows(t *testing.T) {
	pngBytes := []byte("png")
	icoBytes := []byte("ico")

	got := pickTrayIcon("linux", pngBytes, icoBytes)
	if !bytes.Equal(got, pngBytes) {
		t.Fatalf("expected png bytes on non-windows, got %q", string(got))
	}
}
