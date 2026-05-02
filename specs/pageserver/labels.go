package pageserver

import "oltp.molnett.org/neon-operator/api/v1alpha1"

func labels(ps *v1alpha1.Pageserver) map[string]string {
	return map[string]string{
		"molnett.org/cluster":    ps.Spec.Cluster,
		"molnett.org/component":  "pageserver",
		"molnett.org/pageserver": ps.Name,
	}
}

func selectorLabels(ps *v1alpha1.Pageserver) map[string]string {
	return map[string]string{
		"molnett.org/pageserver": ps.Name,
	}
}
