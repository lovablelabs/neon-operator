package controlplane

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"oltp.molnett.org/neon-operator/test/fakes"
	"oltp.molnett.org/neon-operator/test/fixtures"
	"oltp.molnett.org/neon-operator/utils"
)

func TestNotifyAttach_HappyPathWithFakes(t *testing.T) {
	const (
		clusterName = "test-cluster"
		tenantID    = "test-tenant-123"
		computeID   = "test-compute"
	)

	jwtSecret, err := fixtures.NewJWTSecret(clusterName, "neon")
	if err != nil {
		t.Fatalf("NewJWTSecret: %v", err)
	}
	jm, err := utils.NewJWTManagerFromSecret(jwtSecret)
	if err != nil {
		t.Fatalf("NewJWTManagerFromSecret: %v", err)
	}

	fakeCompute := fakes.NewCompute(jm)
	defer fakeCompute.Close()

	scheme := createTestScheme()
	objs := []client.Object{
		createTestDeployment("123456789"),
		createTestService(computeID+"-admin", "neon", tenantID),
		createTestProject(),
		createTestBranch(),
		jwtSecret,
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	mux := http.NewServeMux()
	mux.Handle("/notify-attach", notifyAttach(createTestLogger(), k8sClient, fakeCompute.URL()))

	body := `{"tenant_id":"test-tenant-123","shards":[{"node_id":1,"shard_number":0}]}`
	req := httptest.NewRequest(http.MethodPost, "/notify-attach", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", w.Code, w.Body.String())
	}

	configures := fakeCompute.Configures()
	if len(configures) != 1 {
		t.Fatalf("fake compute received %d /configure calls, want 1", len(configures))
	}
	call := configures[0]
	if call.VerifyErr != nil {
		t.Fatalf("fake compute could not verify JWT: %v", call.VerifyErr)
	}
	if call.Token == "" {
		t.Errorf("expected bearer token, got empty")
	}

	wantClaims := map[string]any{
		"compute_id": computeID,
		"sub":        computeID,
		"aud":        "compute",
		"iss":        "neon-operator",
	}
	for k, want := range wantClaims {
		got := call.Claims[k]
		// `aud` arrives as []string per JWT spec; everything else is the raw value.
		if k == "aud" {
			arr, ok := got.([]string)
			if !ok || len(arr) != 1 || arr[0] != want {
				t.Errorf("claim %q = %v, want [%v]", k, got, want)
			}
			continue
		}
		if got != want {
			t.Errorf("claim %q = %v, want %v", k, got, want)
		}
	}

	roles, ok := call.Claims["roles"].([]any)
	if !ok || len(roles) == 0 {
		t.Errorf("claim roles missing or wrong type: %v", call.Claims["roles"])
	} else if roles[0] != "compute_ctl:admin" {
		t.Errorf("first role = %v, want compute_ctl:admin", roles[0])
	}

	if !bytes.Contains(call.Body, []byte(`"format_version"`)) {
		t.Errorf("/configure body missing compute spec; got %q", call.Body)
	}
}
