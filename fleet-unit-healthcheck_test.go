package main

import (
	"github.com/coreos/fleet/schema"
	"testing"
)

func TestCheck(t *testing.T) {

	fleetUnitHC := &fleetUnitHealthChecker{whitelist()}
	var tests = []struct {
		unit   schema.UnitState
		errMsg string
	}{
		{
			schema.UnitState{SystemdActiveState: "active"},
			"",
		},
		{
			schema.UnitState{SystemdActiveState: "failed"},
			"Unit is in failed state.",
		},
		{
			schema.UnitState{Name: "vulcan.service", SystemdActiveState: "inactive"},
			"Unit is in inactive state.",
		},
		{
			schema.UnitState{Name: "deployer.service", SystemdActiveState: "inactive"},
			"",
		},
	}

	for _, test := range tests {
		actual := fleetUnitHC.Check(test.unit)
		if (actual == nil && test.errMsg != "") || (actual != nil && actual.Error() != test.errMsg) {
			t.Errorf("Test case: %#v\nResult: %#v", test, actual)
		}
	}

}

func TestIsServiceWhitelisted(t *testing.T) {

	wl := whitelist()
	var tests = []struct {
		serviceName string
		expected    bool
	}{
		{
			"deployer",
			false,
		},
		{
			"deployer.service",
			true,
		},
		{
			"deployer.timer",
			false,
		},
		{
			"vulcan.service",
			false,
		},
	}

	for _, test := range tests {
		if isServiceWhitelisted(test.serviceName, wl) != test.expected {
			t.Errorf("Service %s\t Expected: %t\t", test.serviceName, test.expected)
		}
	}
}

func whitelist() []string {
	return []string{"deployer.service", "image-cleaner.service"}
}
