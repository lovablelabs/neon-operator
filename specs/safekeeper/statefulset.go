package safekeeper

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/specs/storagebroker"
)

const storageVolumeName = "safekeeper-storage"

func StatefulSet(sk *v1alpha1.Safekeeper, image string) *appsv1.StatefulSet {
	name := Name(sk)
	lbls := labels(sk)

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   storageVolumeName,
			Labels: lbls,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(sk.Spec.StorageConfig.Size),
				},
			},
			StorageClassName: sk.Spec.StorageConfig.StorageClass,
		},
	}

	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: sk.Namespace,
			Labels:    lbls,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To(int32(1)),
			ServiceName: HeadlessName(sk),
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels(sk),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: lbls,
				},
				Spec: podSpec(sk, image, name),
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{pvc},
		},
	}
}

func podSpec(sk *v1alpha1.Safekeeper, image, serviceName string) corev1.PodSpec {
	return corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser:  ptr.To(int64(1000)),
			RunAsGroup: ptr.To(int64(1000)),
			FSGroup:    ptr.To(int64(1000)),
		},
		Containers: []corev1.Container{
			{
				Name:    "safekeeper",
				Image:   image,
				Command: []string{"/usr/local/bin/safekeeper"},
				Args: []string{
					fmt.Sprintf("--id=%d", sk.Spec.ID),
					"--broker-endpoint=" + storagebroker.URL(sk.Spec.Cluster),
					"--listen-pg=0.0.0.0:5454",
					"--listen-http=0.0.0.0:7676",
					fmt.Sprintf("--advertise-pg=%s:5454", serviceName),
					"--datadir", "/data",
				},
				Ports: []corev1.ContainerPort{
					{ContainerPort: 5454},
					{ContainerPort: 7676},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: storageVolumeName, MountPath: "/data"},
				},
			},
		},
	}
}
