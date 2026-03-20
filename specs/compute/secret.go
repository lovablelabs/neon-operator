package compute

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/utils"
)

const (
	CredentialsUsernameKey = "username"
	CredentialsPasswordKey = "password"
	CredentialsPasswordMD5 = "password_md5"
	CredentialsUsername    = "cloud_admin"

	BranchCredentialsSecretAnnotation = "neon.oltp.molnett.org/credentials-secret"
)

func CredentialsSecretName(branchName string) string {
	return fmt.Sprintf("%s-compute-credentials", branchName)
}

func CredentialsSecret(branch *neonv1alpha1.Branch, project *neonv1alpha1.Project) (*corev1.Secret, error) {
	data, err := BuildCredentialsData(nil)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      CredentialsSecretName(branch.Name),
			Namespace: branch.Namespace,
			Labels: map[string]string{
				"molnett.org/cluster":   project.Spec.ClusterName,
				"molnett.org/component": "compute",
				"molnett.org/branch":    branch.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}, nil
}

// BuildCredentialsData returns a valid credentials payload.
// If existingData contains a non-empty password, it is preserved and dependent
// fields are healed around it. Otherwise a new password is generated.
func BuildCredentialsData(existingData map[string][]byte) (map[string][]byte, error) {
	password := ""
	if existingData != nil {
		password = string(existingData[CredentialsPasswordKey])
	}

	if password == "" {
		generatedPassword, err := utils.GeneratePassword(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate compute password: %w", err)
		}
		password = generatedPassword
	}

	username := CredentialsUsername
	passwordMD5 := utils.PostgresMD5Hash(username, password)

	return map[string][]byte{
		CredentialsUsernameKey: []byte(username),
		CredentialsPasswordKey: []byte(password),
		CredentialsPasswordMD5: []byte(passwordMD5),
	}, nil
}

func CredentialsDataIsValid(data map[string][]byte) bool {
	username := string(data[CredentialsUsernameKey])
	password := string(data[CredentialsPasswordKey])
	passwordMD5 := string(data[CredentialsPasswordMD5])

	if username != CredentialsUsername {
		return false
	}

	if len(password) == 0 || len(passwordMD5) == 0 {
		return false
	}

	return utils.PostgresMD5Hash(username, password) == passwordMD5
}
