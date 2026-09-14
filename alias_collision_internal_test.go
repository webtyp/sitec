//go:build !wasm

package sitec

import "testing"

// This file lives at the module root, not under tests/, DELIBERATELY: every
// other *_test.go in this repo is package sitec_test (black-box) under
// tests/, but this one is white-box (package sitec) because it needs access
// to unexported functions (aliasFor) and unexported types/constants. This is
// an intentional exception to the tests/ convention in this repo; do not
// "fix" it by moving it there — package sitec inside tests/ would be a
// different, disconnected package with no access to unexported symbols.

func TestAliasFor_NoCollisionKeepsLastSegment(t *testing.T) {
	used := make(map[string]string)
	path := "github.com/veltylabs/staff_manager"
	got := aliasFor(path, used)
	want := "m_staff_manager"
	if got != want {
		t.Errorf("aliasFor(%q) = %q, want %q", path, got, want)
	}
}

func TestAliasFor_CollisionWalksUpOneSegment(t *testing.T) {
	pathA := "github.com/veltylabs/mjosefa-cms/modules/appointment_booking"
	pathB := "github.com/veltylabs/appointment_booking"

	// Order 1: A first, then B
	used1 := make(map[string]string)
	aliasA1 := aliasFor(pathA, used1)
	used1[aliasA1] = pathA
	aliasB1 := aliasFor(pathB, used1)
	used1[aliasB1] = pathB

	if aliasA1 != "m_appointment_booking" {
		t.Errorf("Order 1 pathA alias = %q, want %q", aliasA1, "m_appointment_booking")
	}
	if aliasB1 != "m_veltylabs_appointment_booking" {
		t.Errorf("Order 1 pathB alias = %q, want %q", aliasB1, "m_veltylabs_appointment_booking")
	}

	// Order 2: B first, then A
	used2 := make(map[string]string)
	aliasB2 := aliasFor(pathB, used2)
	used2[aliasB2] = pathB
	aliasA2 := aliasFor(pathA, used2)
	used2[aliasA2] = pathA

	if aliasB2 != "m_appointment_booking" {
		t.Errorf("Order 2 pathB alias = %q, want %q", aliasB2, "m_appointment_booking")
	}
	if aliasA2 != "m_modules_appointment_booking" {
		t.Errorf("Order 2 pathA alias = %q, want %q", aliasA2, "m_modules_appointment_booking")
	}
}

func TestAliasFor_SamePathIsNotACollisionWithItself(t *testing.T) {
	used := make(map[string]string)
	path := "github.com/veltylabs/staff_manager"
	alias1 := aliasFor(path, used)
	used[alias1] = path

	alias2 := aliasFor(path, used)
	if alias2 != alias1 {
		t.Errorf("aliasFor on same path returned %q, want %q", alias2, alias1)
	}
}

func TestAliasFor_ReturnsDifferentAliasesForThreeWayCollision(t *testing.T) {
	used := make(map[string]string)
	paths := []string{
		"a/x/foo",
		"b/y/foo",
		"c/z/foo",
	}

	seen := make(map[string]string)
	for _, p := range paths {
		a := aliasFor(p, used)
		used[a] = p
		if prev, ok := seen[a]; ok {
			t.Errorf("collision detected for alias %q between %q and %q", a, prev, p)
		}
		seen[a] = p
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 distinct aliases, got %d", len(seen))
	}
}
