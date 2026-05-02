package safekeeper

import "oltp.molnett.org/neon-operator/api/v1alpha1"

func labels(sk *v1alpha1.Safekeeper) map[string]string {
	return map[string]string{
		"molnett.org/cluster":    sk.Spec.Cluster,
		"molnett.org/component":  "safekeeper",
		"molnett.org/safekeeper": sk.Name,
	}
}

func selectorLabels(sk *v1alpha1.Safekeeper) map[string]string {
	return map[string]string{
		"molnett.org/safekeeper": sk.Name,
	}
}
