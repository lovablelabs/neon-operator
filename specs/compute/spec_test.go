package compute

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/utils"
)

func TestComputeHookNotifyRequest_JSON(t *testing.T) {
	expectedReq := `{"tenant_id":"test-tenant-123","stripe_size":8,"shards":[{"node_id":1,"shard_number":0},` +
		`{"node_id":2,"shard_number":1}]}`
	tests := []struct {
		name     string
		request  ComputeHookNotifyRequest
		expected string
	}{
		{
			name: "complete request",
			request: ComputeHookNotifyRequest{
				TenantID:   "test-tenant-123",
				StripeSize: func(x uint32) *uint32 { return &x }(8),
				Shards: []ComputeHookNotifyRequestShard{
					{NodeID: 1, ShardNumber: 0},
					{NodeID: 2, ShardNumber: 1},
				},
			},
			expected: expectedReq,
		},
		{
			name: "minimal request",
			request: ComputeHookNotifyRequest{
				TenantID: "minimal-tenant",
				Shards:   []ComputeHookNotifyRequestShard{},
			},
			expected: `{"tenant_id":"minimal-tenant","shards":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.request)
			if err != nil {
				t.Errorf("failed to marshal request: %v", err)
			}

			if string(jsonData) != tt.expected {
				t.Errorf("expected JSON %s, got %s", tt.expected, string(jsonData))
			}

			// Test unmarshaling
			var unmarshaled ComputeHookNotifyRequest
			err = json.Unmarshal(jsonData, &unmarshaled)
			if err != nil {
				t.Errorf("failed to unmarshal request: %v", err)
			}

			if unmarshaled.TenantID != tt.request.TenantID {
				t.Errorf("expected tenant_id %s, got %s", tt.request.TenantID, unmarshaled.TenantID)
			}
		})
	}
}

func TestGenerateComputeSpec_UsesCredentialsSecretHash(t *testing.T) {
	t.Parallel()

	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})

	publicDER, err := x509.MarshalPKIXPublicKey(privateKey.Public())
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 to scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := neonv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add neonv1alpha1 to scheme: %v", err)
	}

	branch := &neonv1alpha1.Branch{
		ObjectMeta: metav1.ObjectMeta{Name: "probe-branch", Namespace: "neon"},
		Spec:       neonv1alpha1.BranchSpec{ProjectID: "probe-project", TimelineID: "timeline-1", PGVersion: 17},
	}
	project := &neonv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "probe-project", Namespace: "neon"},
		Spec:       neonv1alpha1.ProjectSpec{ClusterName: "probe-cluster", TenantID: "tenant-1", PGVersion: 17},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "probe-branch-compute-node",
			Namespace: "neon",
			Labels: map[string]string{
				"molnett.org/component": "compute",
				"neon.tenant_id":        "tenant-1",
				"neon.timeline_id":      "timeline-1",
			},
			Annotations: map[string]string{
				"neon.compute_id":   "probe-branch",
				"neon.cluster_name": "probe-cluster",
			},
		},
	}
	jwksSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-probe-cluster-jwt", Namespace: "neon"},
		Data: map[string][]byte{
			"private.pem": privatePEM,
			"public.pem":  publicPEM,
		},
	}
	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: CredentialsSecretName("probe-branch"), Namespace: "neon"},
		Data: map[string][]byte{
			CredentialsUsernameKey: []byte("cloud_admin"),
			CredentialsPasswordKey: []byte("cloud_admin"),
			CredentialsPasswordMD5: []byte(utils.PostgresMD5Hash("cloud_admin", "cloud_admin")),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		branch,
		project,
		deployment,
		jwksSecret,
		credentialsSecret,
	).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	spec, err := GenerateComputeSpec(context.Background(), logger, k8sClient, &ComputeHookNotifyRequest{
		TenantID: "tenant-1",
		Shards:   []ComputeHookNotifyRequestShard{{NodeID: 1, ShardNumber: 0}},
	}, "probe-branch")
	if err != nil {
		t.Fatalf("GenerateComputeSpec returned error: %v", err)
	}

	if len(spec.Spec.Cluster.Roles) == 0 {
		t.Fatal("expected roles in generated spec")
	}

	var got *string
	for _, role := range spec.Spec.Cluster.Roles {
		if role.Name == "cloud_admin" {
			got = role.EncryptedPassword
		}
	}
	if got == nil {
		t.Fatal("expected cloud_admin role in spec")
	}

	want := utils.PostgresMD5Hash("cloud_admin", "cloud_admin")
	if *got != want {
		t.Fatalf("unexpected cloud_admin encrypted password: got %s want %s", *got, want)
	}
}
