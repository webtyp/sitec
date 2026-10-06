package sitec_test

import "webtyp.com/modfind"

// fakeModules is the test double for modfind.Discoverer: it returns fixed
// modules, so no `go list` runs.
type fakeModules []modfind.Module

func (f fakeModules) Discover(string) ([]modfind.Module, error) { return f, nil }
