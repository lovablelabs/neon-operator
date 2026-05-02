package pageserver

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/specs/storagebroker"
	"oltp.molnett.org/neon-operator/specs/storagecontroller"
)

func ConfigMap(ps *v1alpha1.Pageserver, bucketSecret *corev1.Secret) *corev1.ConfigMap {
	pageserverToml := fmt.Sprintf(`
control_plane_api = "%s/upcall/v1/"
listen_pg_addr = "0.0.0.0:6400"
listen_http_addr = "0.0.0.0:9898"
broker_endpoint = "%s"
pg_distrib_dir='/usr/local/'
[remote_storage]
bucket_name = "%s"
bucket_region = "%s"
prefix_in_bucket = "pageserver"
endpoint = "%s"
`,
		storagecontroller.URL(ps.Spec.Cluster),
		storagebroker.URL(ps.Spec.Cluster),
		string(bucketSecret.Data["BUCKET_NAME"]),
		string(bucketSecret.Data["AWS_REGION"]),
		string(bucketSecret.Data["AWS_ENDPOINT_URL"]),
	)

	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name(ps),
			Namespace: ps.Namespace,
			Labels:    labels(ps),
		},
		Data: map[string]string{
			"pageserver.toml": pageserverToml,
		},
	}
}
