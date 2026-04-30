package fixtures_test

import (
	"testing"

	"oltp.molnett.org/neon-operator/test/fixtures"
	"oltp.molnett.org/neon-operator/utils"
)

func TestNewJWTSecret_RoundTrip(t *testing.T) {
	secret, err := fixtures.NewJWTSecret(testClusterName, testNamespace)
	if err != nil {
		t.Fatalf("NewJWTSecret: %v", err)
	}

	wantName := "cluster-" + testClusterName + "-jwt"
	if secret.Name != wantName {
		t.Errorf("name = %s, want %s", secret.Name, wantName)
	}
	if secret.Namespace != testNamespace {
		t.Errorf("namespace = %s, want %s", secret.Namespace, testNamespace)
	}

	jm, err := utils.NewJWTManagerFromSecret(secret)
	if err != nil {
		t.Fatalf("NewJWTManagerFromSecret: %v", err)
	}

	tok, err := jm.GenerateToken(map[string]any{
		"compute_id": "c1",
		"sub":        "c1",
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := jm.VerifyToken(tok); err != nil {
		t.Errorf("VerifyToken: %v", err)
	}
}
