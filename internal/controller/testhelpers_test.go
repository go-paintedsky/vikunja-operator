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

// Shared fixtures used across the controller test suite: every test runs
// against the same namespace and fake API token, and the fake Vikunja server
// (see fakevikunja_test.go) always seeds the same three users.
const (
	testNamespace        = "default"
	testToken            = "test-token" //nolint:gosec // test fixture, not a real credential
	testSecretTokenKey   = "token"
	testUsernameOperator = "operator"
	testUsernameAlice    = "alice"
	testUsernameBob      = "bob"
)

// createReadyInstance creates a Secret holding testToken and a VikunjaInstance
// named name pointing at server, reconciles it, and asserts it becomes Ready.
// It returns an InstanceReference other resources' specs can use.
func createReadyInstance(ctx context.Context, name string, server *httptest.Server) vikunjav1alpha1.InstanceReference {
	secretName := name + "-token"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
		StringData: map[string]string{testSecretTokenKey: testToken},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

	instance := &vikunjav1alpha1.VikunjaInstance{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: vikunjav1alpha1.VikunjaInstanceSpec{
			BaseURL:           server.URL,
			APITokenSecretRef: vikunjav1alpha1.SecretKeyReference{Name: secretName, Key: testSecretTokenKey},
		},
	}
	Expect(k8sClient.Create(ctx, instance)).To(Succeed())
	DeferCleanup(func() { _ = k8sClient.Delete(ctx, instance) })

	reconciler := &VikunjaInstanceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	nn := types.NamespacedName{Namespace: testNamespace, Name: name}
	_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
	Expect(err).NotTo(HaveOccurred())

	var updated vikunjav1alpha1.VikunjaInstance
	Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
	Expect(meta.IsStatusConditionTrue(updated.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)).To(BeTrue())

	return vikunjav1alpha1.InstanceReference{Name: name}
}
