package pageserver_test

import (
	"testing"

	"oltp.molnett.org/neon-operator/specs/pageserver"
	"oltp.molnett.org/neon-operator/test/fixtures"
	testutils "oltp.molnett.org/neon-operator/test/utils"
)

func TestSpecs(t *testing.T) {
	const (
		clusterName = "test-cluster"
		namespace   = "neon"
		psID        = uint64(0)
	)

	ps := fixtures.NewPageserver("test-pageserver", namespace, clusterName, psID)
	bucket := fixtures.NewBucketCredsSecret(clusterName, namespace)

	cases := []struct {
		name string
		obj  any
	}{
		{"pod", pageserver.Pod(ps, fixtures.DefaultNeonImage)},
		{"pvc", pageserver.PersistentVolumeClaim(ps)},
		{"service", pageserver.Service(ps)},
		{"configmap", pageserver.ConfigMap(ps, bucket)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutils.AssertGolden(t, "testdata/"+tc.name+".yaml", tc.obj)
		})
	}
}
