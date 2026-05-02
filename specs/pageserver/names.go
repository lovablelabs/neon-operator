package pageserver

import (
	"fmt"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
)

func Name(ps *v1alpha1.Pageserver) string {
	return fmt.Sprintf("%s-pageserver-%d", ps.Spec.Cluster, ps.Spec.ID)
}

func HeadlessName(ps *v1alpha1.Pageserver) string {
	return Name(ps) + "-headless"
}
