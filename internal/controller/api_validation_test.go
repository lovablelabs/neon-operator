/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/test/fixtures"
)

var _ = Describe("API validation", func() {
	var namespace string

	BeforeEach(func() {
		namespace = newTestNamespace()
	})

	It("rejects unsupported cluster topology", func() {
		cluster := fixtures.NewCluster("invalid-topology", namespace)
		cluster.Spec.NumSafekeepers = 4

		err := k8sClient.Create(ctx, cluster)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected invalid error, got %v", err)
	})

	It("rejects malformed generated IDs", func() {
		project := fixtures.NewProject("bad-tenant", namespace, "cluster-a")
		project.Spec.TenantID = "not-a-neon-id"
		Expect(apierrors.IsInvalid(k8sClient.Create(ctx, project))).To(BeTrue())

		branch := fixtures.NewBranch("bad-timeline", namespace, "project-a")
		branch.Spec.TimelineID = "not-a-neon-id"
		Expect(apierrors.IsInvalid(k8sClient.Create(ctx, branch))).To(BeTrue())
	})

	It("rejects invalid storage sizes before specs can panic while parsing them", func() {
		sk := fixtures.NewSafekeeper("bad-storage", namespace, "cluster-a", 0)
		sk.Spec.StorageConfig.Size = "tenGi"
		Expect(apierrors.IsInvalid(k8sClient.Create(ctx, sk))).To(BeTrue())
	})

	It("prevents generated IDs from changing after they are set", func() {
		project := fixtures.NewProject("immutable-tenant", namespace, "cluster-a")
		project.Spec.TenantID = "00000000000000000000000000000001"
		Expect(k8sClient.Create(ctx, project)).To(Succeed())

		current := &neonv1alpha1.Project{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: project.Name, Namespace: namespace}, current)).To(Succeed())
		current.Spec.TenantID = "00000000000000000000000000000002"

		err := k8sClient.Update(ctx, current)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected invalid error, got %v", err)
	})
})
