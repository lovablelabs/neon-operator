package pageserver

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
)

func Service(ps *v1alpha1.Pageserver) *corev1.Service {
	return service(ps, Name(ps), "")
}

func HeadlessService(ps *v1alpha1.Pageserver) *corev1.Service {
	return service(ps, HeadlessName(ps), corev1.ClusterIPNone)
}

func service(ps *v1alpha1.Pageserver, name, clusterIP string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ps.Namespace,
			Labels:    labels(ps),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIP,
			Selector:  selectorLabels(ps),
			Ports: []corev1.ServicePort{
				{Name: "pg", Port: 6400, Protocol: corev1.ProtocolTCP},
				{Name: "http", Port: 9898, Protocol: corev1.ProtocolTCP},
			},
		},
	}
}
