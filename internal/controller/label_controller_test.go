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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vikunjav1alpha1 "github.com/go-paintedsky/vikunja-operator/api/v1alpha1"
)

var _ = Describe("Label Controller", func() {
	const (
		resourceName      = "label-controller-test"
		resourceNamespace = "default"
	)

	var (
		ctx                context.Context
		fake               *fakeVikunjaServer
		instanceRef        vikunjav1alpha1.InstanceReference
		typeNamespacedName types.NamespacedName
		reconciler         *LabelReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		var server *httptest.Server
		server, fake = newFakeVikunjaServer()
		DeferCleanup(server.Close)

		instanceRef = createReadyInstance(ctx, resourceName+"-instance", server)
		typeNamespacedName = types.NamespacedName{Name: resourceName, Namespace: resourceNamespace}
		reconciler = &LabelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	reconcileOnce := func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
	}

	It("creates, updates and deletes the label on the Vikunja instance", func() {
		label := &vikunjav1alpha1.Label{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: resourceNamespace},
			Spec: vikunjav1alpha1.LabelSpec{
				InstanceRef: instanceRef,
				Title:       "bug",
				HexColor:    "d73a4a",
			},
		}
		Expect(k8sClient.Create(ctx, label)).To(Succeed())

		reconcileOnce() // adds finalizer
		reconcileOnce() // creates on the fake instance

		Expect(k8sClient.Get(ctx, typeNamespacedName, label)).To(Succeed())
		Expect(label.Status.ID).NotTo(BeZero())
		Expect(meta.IsStatusConditionTrue(label.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)).To(BeTrue())

		remote := fake.labels[label.Status.ID]
		Expect(remote).NotTo(BeNil())
		Expect(remote.Title).To(Equal("bug"))

		label.Spec.Description = "Something isn't working"
		Expect(k8sClient.Update(ctx, label)).To(Succeed())
		reconcileOnce()

		Expect(fake.labels[label.Status.ID].Description).To(Equal("Something isn't working"))

		Expect(k8sClient.Delete(ctx, label)).To(Succeed())
		reconcileOnce()

		Expect(fake.labels).NotTo(HaveKey(label.Status.ID))
		err := k8sClient.Get(ctx, typeNamespacedName, label)
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})
})
