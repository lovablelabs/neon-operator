package safekeeper_test

import (
	"testing"

	"oltp.molnett.org/neon-operator/specs/safekeeper"
	"oltp.molnett.org/neon-operator/test/fixtures"
	testutils "oltp.molnett.org/neon-operator/test/utils"
)

func TestSpecs(t *testing.T) {
	const (
		clusterName = "test-cluster"
		namespace   = "neon"
		skID        = uint32(0)
	)

	sk := fixtures.NewSafekeeper("test-safekeeper", namespace, clusterName, skID)

	cases := []struct {
		name string
		obj  any
	}{
		{"pod", safekeeper.Pod(sk, fixtures.DefaultNeonImage)},
		{"pvc", safekeeper.PersistentVolumeClaim(sk)},
		{"service", safekeeper.Service(sk)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutils.AssertGolden(t, "testdata/"+tc.name+".yaml", tc.obj)
		})
	}
}
