/*
Copyright 2026.

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
	"context"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vikunjav1alpha1 "github.com/go-paintedsky/vikunja-operator/api/v1alpha1"
)

var _ = Describe("VikunjaInstance Controller", func() {
	const resourceName = "vikunjainstance-controller-test"

	var (
		ctx                context.Context
		server             *httptest.Server
		typeNamespacedName types.NamespacedName
		reconciler         *VikunjaInstanceReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		server, _ = newFakeVikunjaServer()
		DeferCleanup(server.Close)

		typeNamespacedName = types.NamespacedName{Name: resourceName, Namespace: testNamespace}
		reconciler = &VikunjaInstanceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	It("becomes Ready when the token is valid", func() {
		secret := &corev1.Secret{
			Name: resourceName + "-token", Namespace: testNamespace,
			StringData: map[string]string{testSecretTokenKey: testToken},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		instance := &vikunjav1alpha1.VikunjaInstance{
			Name: resourceName, Namespace: testNamespace,
			Spec: vikunjav1alpha1.VikunjaInstanceSpec{
				BaseURL:           server.URL,
				APITokenSecretRef: vikunjav1alpha1.SecretKeyReference{Name: secret.Name, Key: testSecretTokenKey},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, typeNamespacedName, instance)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)).To(BeTrue())
		Expect(instance.Status.Version).To(Equal("test"))
		Expect(instance.Status.TokenOwner).To(Equal(testUsernameOperator))
	})

	It("becomes NotReady when the token is rejected by the instance", func() {
		secret := &corev1.Secret{
			Name: resourceName + "-bad-token", Namespace: testNamespace,
			StringData: map[string]string{testSecretTokenKey: "wrong-token"},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		instance := &vikunjav1alpha1.VikunjaInstance{
			Name: resourceName, Namespace: testNamespace,
			Spec: vikunjav1alpha1.VikunjaInstanceSpec{
				BaseURL:           server.URL,
				APITokenSecretRef: vikunjav1alpha1.SecretKeyReference{Name: secret.Name, Key: testSecretTokenKey},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, typeNamespacedName, instance)).To(Succeed())
		cond := meta.FindStatusCondition(instance.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vikunjav1alpha1.ReasonInstanceNotReady))
	})
})
