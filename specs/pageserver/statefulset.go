package pageserver

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
)

const storageVolumeName = "pageserver-storage"

const initScript = `echo "id=%d" > /config/identity.toml

echo "{\"host\":\"%s.%s\"," \
     "\"http_host\":\"%s.%s\"," \
     "\"http_port\":9898,\"port\":6400," \
     "\"availability_zone_id\":\"se-ume\"}" > /config/metadata.json

cp /configmap/pageserver.toml /config/pageserver.toml
`

func StatefulSet(ps *v1alpha1.Pageserver, image string) *appsv1.StatefulSet {
	name := Name(ps)
	lbls := labels(ps)

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   storageVolumeName,
			Labels: lbls,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(ps.Spec.StorageConfig.Size),
				},
			},
			StorageClassName: ps.Spec.StorageConfig.StorageClass,
		},
	}

	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ps.Namespace,
			Labels:    lbls,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To(int32(1)),
			ServiceName: HeadlessName(ps),
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels(ps),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: lbls,
				},
				Spec: podSpec(ps, image, name),
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{pvc},
		},
	}
}

func podSpec(ps *v1alpha1.Pageserver, image, serviceName string) corev1.PodSpec {
	return corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser:  ptr.To(int64(1000)),
			RunAsGroup: ptr.To(int64(1000)),
			FSGroup:    ptr.To(int64(1000)),
		},
		InitContainers: []corev1.Container{
			{
				Name:    "setup-config",
				Image:   "busybox:latest",
				Command: []string{"/bin/sh", "-c"},
				Args: []string{
					fmt.Sprintf(initScript,
						ps.Spec.ID,
						serviceName, ps.Namespace,
						serviceName, ps.Namespace),
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "pageserver-config", MountPath: "/configmap"},
					{Name: "config", MountPath: "/config"},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "pageserver",
				Image:           image,
				ImagePullPolicy: corev1.PullAlways,
				Command:         []string{"/usr/local/bin/pageserver"},
				Ports: []corev1.ContainerPort{
					{ContainerPort: 6400},
					{ContainerPort: 9898},
				},
				Env: []corev1.EnvVar{
					{Name: "RUST_LOG", Value: "debug"},
					{Name: "DEFAULT_PG_VERSION", Value: "16"},
					bucketEnv("AWS_ACCESS_KEY_ID", ps),
					bucketEnv("AWS_SECRET_ACCESS_KEY", ps),
					bucketEnv("AWS_REGION", ps),
					bucketEnv("BUCKET_NAME", ps),
					bucketEnv("AWS_ENDPOINT_URL", ps),
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: storageVolumeName, MountPath: "/data/.neon/tenants"},
					{Name: "config", MountPath: "/data/.neon"},
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "pageserver-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: serviceName},
					},
				},
			},
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
	}
}

func bucketEnv(key string, ps *v1alpha1.Pageserver) corev1.EnvVar {
	return corev1.EnvVar{
		Name: key,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ps.Spec.BucketCredentialsSecret.Name,
				},
				Key: key,
			},
		},
	}
}
