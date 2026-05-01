package compute_test

import (
	"testing"

	"oltp.molnett.org/neon-operator/specs/compute"
	"oltp.molnett.org/neon-operator/test/fixtures"
	testutils "oltp.molnett.org/neon-operator/test/utils"
)

func TestSpecs(t *testing.T) {
	const (
		projectName = "test-project"
		branchName  = "test-branch"
		namespace   = "neon"
		clusterName = "test-cluster"
	)

	project := fixtures.NewProject(projectName, namespace, clusterName)
	branch := fixtures.NewBranch(branchName, namespace, projectName)

	cases := []struct {
		name string
		obj  any
	}{
		{"deployment", compute.Deployment(branch, project)},
		{"admin_service", compute.AdminService(branch, project)},
		{"postgres_service", compute.PostgresService(branch, project)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutils.AssertGolden(t, "testdata/"+tc.name+".yaml", tc.obj)
		})
	}
}
