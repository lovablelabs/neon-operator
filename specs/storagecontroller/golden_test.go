package storagecontroller_test

import (
	"testing"

	"oltp.molnett.org/neon-operator/specs/storagecontroller"
	"oltp.molnett.org/neon-operator/test/fixtures"
	testutils "oltp.molnett.org/neon-operator/test/utils"
)

func TestSpecs(t *testing.T) {
	cluster := fixtures.NewCluster("test-cluster", "neon")

	cases := []struct {
		name string
		obj  any
	}{
		{"deployment", storagecontroller.Deployment(cluster)},
		{"service", storagecontroller.Service(cluster)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutils.AssertGolden(t, "testdata/"+tc.name+".yaml", tc.obj)
		})
	}
}
