package fixtures_test

import (
	"testing"

	"oltp.molnett.org/neon-operator/test/fixtures"
)

const (
	testClusterName = "alpha"
	testNamespace   = "neon"
)

func TestNewCluster(t *testing.T) {
	c := fixtures.NewCluster(testClusterName, testNamespace)

	if c.Name != testClusterName || c.Namespace != testNamespace {
		t.Fatalf("ObjectMeta = %s/%s, want %s/%s", c.Namespace, c.Name, testNamespace, testClusterName)
	}
	if c.Spec.NumSafekeepers != 3 {
		t.Errorf("NumSafekeepers = %d, want 3", c.Spec.NumSafekeepers)
	}
	if c.Spec.DefaultPGVersion != 17 {
		t.Errorf("DefaultPGVersion = %d, want 17", c.Spec.DefaultPGVersion)
	}
	if c.Spec.BucketCredentialsSecret == nil || c.Spec.BucketCredentialsSecret.Name != testClusterName+"-bucket-creds" {
		t.Errorf("BucketCredentialsSecret = %+v, want name=%s-bucket-creds", c.Spec.BucketCredentialsSecret, testClusterName)
	}
	if c.Spec.StorageControllerDatabaseSecret == nil || c.Spec.StorageControllerDatabaseSecret.Key != "uri" {
		t.Errorf("StorageControllerDatabaseSecret = %+v, want key=uri", c.Spec.StorageControllerDatabaseSecret)
	}
}

func TestNewProject_LinkedToCluster(t *testing.T) {
	p := fixtures.NewProject("p1", testNamespace, testClusterName)
	if p.Spec.ClusterName != testClusterName {
		t.Errorf("ClusterName = %s, want %s", p.Spec.ClusterName, testClusterName)
	}
	if p.Spec.PGVersion != 17 {
		t.Errorf("PGVersion = %d, want 17", p.Spec.PGVersion)
	}
}

func TestNewBranch_LinkedToProject(t *testing.T) {
	b := fixtures.NewBranch("b1", testNamespace, "p1")
	if b.Spec.ProjectID != "p1" {
		t.Errorf("ProjectID = %s, want p1", b.Spec.ProjectID)
	}
	if b.Spec.PGVersion != 17 {
		t.Errorf("PGVersion = %d, want 17", b.Spec.PGVersion)
	}
}

func TestNewPageserver_LinkedToCluster(t *testing.T) {
	ps := fixtures.NewPageserver("ps0", testNamespace, testClusterName, 1)
	if ps.Spec.Cluster != testClusterName {
		t.Errorf("Cluster = %s, want %s", ps.Spec.Cluster, testClusterName)
	}
	if ps.Spec.ID != 1 {
		t.Errorf("ID = %d, want 1", ps.Spec.ID)
	}
	if ps.Spec.BucketCredentialsSecret == nil || ps.Spec.BucketCredentialsSecret.Name != testClusterName+"-bucket-creds" {
		t.Errorf("BucketCredentialsSecret = %+v, want name=%s-bucket-creds", ps.Spec.BucketCredentialsSecret, testClusterName)
	}
	if ps.Spec.StorageConfig.Size != "1Gi" {
		t.Errorf("StorageConfig.Size = %s, want 1Gi", ps.Spec.StorageConfig.Size)
	}
}

func TestNewSafekeeper_LinkedToCluster(t *testing.T) {
	sk := fixtures.NewSafekeeper("sk0", testNamespace, testClusterName, 1)
	if sk.Spec.Cluster != testClusterName {
		t.Errorf("Cluster = %s, want %s", sk.Spec.Cluster, testClusterName)
	}
	if sk.Spec.ID != 1 {
		t.Errorf("ID = %d, want 1", sk.Spec.ID)
	}
	if sk.Spec.StorageConfig.Size != "1Gi" {
		t.Errorf("StorageConfig.Size = %s, want 1Gi", sk.Spec.StorageConfig.Size)
	}
}

// The relationship invariant: secret fixtures produce names that match
// what the Cluster fixture references. If these drift apart, every test that
// composes a Cluster with its Secrets will fail confusingly.
func TestSecretNamesMatchClusterReferences(t *testing.T) {
	c := fixtures.NewCluster(testClusterName, testNamespace)
	bucket := fixtures.NewBucketCredsSecret(testClusterName, testNamespace)
	storconDB := fixtures.NewStorcondDBSecret(testClusterName, testNamespace)

	if bucket.Name != c.Spec.BucketCredentialsSecret.Name {
		t.Errorf("bucket secret name %s != cluster ref %s", bucket.Name, c.Spec.BucketCredentialsSecret.Name)
	}
	if storconDB.Name != c.Spec.StorageControllerDatabaseSecret.Name {
		t.Errorf("storcon-db secret name %s != cluster ref %s", storconDB.Name, c.Spec.StorageControllerDatabaseSecret.Name)
	}
	if _, ok := storconDB.Data[c.Spec.StorageControllerDatabaseSecret.Key]; !ok {
		t.Errorf("storcon-db secret missing key %q referenced by cluster", c.Spec.StorageControllerDatabaseSecret.Key)
	}
}
