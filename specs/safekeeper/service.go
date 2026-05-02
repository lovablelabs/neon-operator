package safekeeper

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
)

func Service(sk *v1alpha1.Safekeeper) *corev1.Service {
	return service(sk, Name(sk), "")
}

func HeadlessService(sk *v1alpha1.Safekeeper) *corev1.Service {
	return service(sk, HeadlessName(sk), corev1.ClusterIPNone)
}

func service(sk *v1alpha1.Safekeeper, name, clusterIP string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: sk.Namespace,
			Labels:    labels(sk),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIP,
			Selector:  selectorLabels(sk),
			Ports: []corev1.ServicePort{
				{Name: "pg", Port: 5454, Protocol: corev1.ProtocolTCP},
				{Name: "http", Port: 7676, Protocol: corev1.ProtocolTCP},
			},
		},
	}
}
