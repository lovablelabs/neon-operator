package compute

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/utils"
)

func TestCredentialsSecret(t *testing.T) {
	t.Parallel()

	branch := &neonv1alpha1.Branch{ObjectMeta: metav1.ObjectMeta{Name: "probe-branch", Namespace: "neon"}}
	project := &neonv1alpha1.Project{Spec: neonv1alpha1.ProjectSpec{ClusterName: "probe-cluster"}}

	secret, err := CredentialsSecret(branch, project)
	if err != nil {
		t.Fatalf("CredentialsSecret returned error: %v", err)
	}

	if secret.Name != "probe-branch-compute-credentials" {
		t.Fatalf("unexpected secret name: %s", secret.Name)
	}

	if got := string(secret.Data[CredentialsUsernameKey]); got != "cloud_admin" {
		t.Fatalf("unexpected username: %s", got)
	}

	password := string(secret.Data[CredentialsPasswordKey])
	if len(password) == 0 {
		t.Fatal("expected generated non-empty password")
	}

	gotMD5 := string(secret.Data[CredentialsPasswordMD5])
	wantMD5 := utils.PostgresMD5Hash("cloud_admin", password)
	if gotMD5 != wantMD5 {
		t.Fatalf("unexpected password md5 hash: got %s want %s", gotMD5, wantMD5)
	}

	if !CredentialsDataIsValid(secret.Data) {
		t.Fatal("expected generated credentials data to be valid")
	}
}

func TestCredentialsDataIsValid(t *testing.T) {
	t.Parallel()

	valid := map[string][]byte{
		CredentialsUsernameKey: []byte(CredentialsUsername),
		CredentialsPasswordKey: []byte("abc123"),
		CredentialsPasswordMD5: []byte(utils.PostgresMD5Hash(CredentialsUsername, "abc123")),
	}
	if !CredentialsDataIsValid(valid) {
		t.Fatal("expected valid credentials data")
	}

	invalidHash := map[string][]byte{
		CredentialsUsernameKey: []byte(CredentialsUsername),
		CredentialsPasswordKey: []byte("abc123"),
		CredentialsPasswordMD5: []byte("broken"),
	}
	if CredentialsDataIsValid(invalidHash) {
		t.Fatal("expected invalid hash to fail validation")
	}

	invalidUser := map[string][]byte{
		CredentialsUsernameKey: []byte("other"),
		CredentialsPasswordKey: []byte("abc123"),
		CredentialsPasswordMD5: []byte(utils.PostgresMD5Hash(CredentialsUsername, "abc123")),
	}
	if CredentialsDataIsValid(invalidUser) {
		t.Fatal("expected invalid username to fail validation")
	}
}

func TestBuildCredentialsData(t *testing.T) {
	t.Parallel()

	t.Run("preserves existing non-empty password", func(t *testing.T) {
		t.Parallel()

		healed, err := BuildCredentialsData(map[string][]byte{
			CredentialsUsernameKey: []byte("wrong"),
			CredentialsPasswordKey: []byte("keep-me"),
			CredentialsPasswordMD5: []byte("broken"),
		})
		if err != nil {
			t.Fatalf("BuildCredentialsData returned error: %v", err)
		}

		if string(healed[CredentialsPasswordKey]) != "keep-me" {
			t.Fatalf("expected password to be preserved, got %q", string(healed[CredentialsPasswordKey]))
		}
		if string(healed[CredentialsUsernameKey]) != CredentialsUsername {
			t.Fatalf("expected username %q, got %q", CredentialsUsername, string(healed[CredentialsUsernameKey]))
		}
		if !CredentialsDataIsValid(healed) {
			t.Fatal("expected healed credentials data to be valid")
		}
	})

	t.Run("generates password when existing password is missing", func(t *testing.T) {
		t.Parallel()

		healed, err := BuildCredentialsData(map[string][]byte{
			CredentialsUsernameKey: []byte(CredentialsUsername),
			CredentialsPasswordMD5: []byte("broken"),
		})
		if err != nil {
			t.Fatalf("BuildCredentialsData returned error: %v", err)
		}

		if len(healed[CredentialsPasswordKey]) == 0 {
			t.Fatal("expected generated non-empty password")
		}
		if !CredentialsDataIsValid(healed) {
			t.Fatal("expected generated credentials data to be valid")
		}
	})
}
