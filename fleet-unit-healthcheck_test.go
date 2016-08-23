package main

import (
	"github.com/coreos/fleet/schema"
	"regexp"
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
			schema.UnitState{SystemdActiveState: "activating"},
			"Unit is in activating state.",
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
		{
			"mongo-backup.service",
			false,
		},
		{
			"mongo-backup@1.service",
			true,
		},
	}

	for _, test := range tests {
		if isServiceWhitelisted(test.serviceName, wl) != test.expected {
			t.Errorf("Service %s\t Expected: %t\t", test.serviceName, test.expected)
		}
	}
}

func whitelist() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile(`deployer\.service`),
		regexp.MustCompile(`image-cleaner\.service`),
		regexp.MustCompile(`mongo-backup@\d+\.service`)}
}
