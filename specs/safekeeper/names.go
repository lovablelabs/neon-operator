package safekeeper

import (
	"fmt"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
)

func Name(sk *v1alpha1.Safekeeper) string {
	return fmt.Sprintf("%s-safekeeper-%d", sk.Spec.Cluster, sk.Spec.ID)
}

func HeadlessName(sk *v1alpha1.Safekeeper) string {
	return Name(sk) + "-headless"
}
