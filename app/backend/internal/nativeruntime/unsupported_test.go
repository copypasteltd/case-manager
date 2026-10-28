//go:build !linux

package nativeruntime

import (
	"context"
	"testing"
)

func TestNativeControllerNeverSubstitutesAWindowsBoundary(t *testing.T) {
	if Check("unavailable") == nil || Serve(context.Background(), "unavailable") == nil {
		t.Fatal("non-Linux host claimed native isolation")
	}
}
