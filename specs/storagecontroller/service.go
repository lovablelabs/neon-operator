package storagecontroller

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
)

func Service(cluster *v1alpha1.Cluster) *corev1.Service {
	name := Name(cluster.Name)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name": name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       Port,
					TargetPort: intstr.FromInt(Port),
				},
			},
		},
	}
}
