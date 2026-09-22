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

var _ = Describe("Project Controller", func() {
	const (
		resourceName      = "project-controller-test"
		resourceNamespace = "default"
	)

	var (
		ctx                context.Context
		server             *httptest.Server
		fake               *fakeVikunjaServer
		instanceRef        vikunjav1alpha1.InstanceReference
		typeNamespacedName types.NamespacedName
		reconciler         *ProjectReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		server, fake = newFakeVikunjaServer()
		DeferCleanup(server.Close)

		instanceRef = createReadyInstance(ctx, resourceName+"-instance", server)
		typeNamespacedName = types.NamespacedName{Name: resourceName, Namespace: resourceNamespace}
		reconciler = &ProjectReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	reconcileOnce := func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
	}

	It("creates, updates and deletes the project on the Vikunja instance", func() {
		By("creating the Project resource")
		project := &vikunjav1alpha1.Project{
			Name: resourceName, Namespace: resourceNamespace,
			Spec: vikunjav1alpha1.ProjectSpec{
				InstanceRef: instanceRef,
				Title:       "My Project",
				HexColor:    "ff0000",
			},
		}
		Expect(k8sClient.Create(ctx, project)).To(Succeed())

		By("reconciling to add the finalizer, then to create the remote project")
		reconcileOnce() // adds finalizer
		reconcileOnce() // creates on the fake instance

		Expect(k8sClient.Get(ctx, typeNamespacedName, project)).To(Succeed())
		Expect(project.Status.ID).NotTo(BeZero())
		Expect(meta.IsStatusConditionTrue(project.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)).To(BeTrue())
		Expect(project.Finalizers).To(ContainElement(vikunjav1alpha1.Finalizer))

		remote := fake.projects[project.Status.ID]
		Expect(remote).NotTo(BeNil())
		Expect(remote.Title).To(Equal("My Project"))
		Expect(remote.HexColor).To(Equal("ff0000"))

		By("updating the spec and reconciling again")
		project.Spec.Description = "updated description"
		Expect(k8sClient.Update(ctx, project)).To(Succeed())
		reconcileOnce()

		Expect(fake.projects[project.Status.ID].Description).To(Equal("updated description"))

		By("deleting the resource")
		Expect(k8sClient.Delete(ctx, project)).To(Succeed())
		reconcileOnce()

		Expect(fake.projects).NotTo(HaveKey(project.Status.ID))
		err := k8sClient.Get(ctx, typeNamespacedName, project)
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})

	It("reports NotReady while the referenced VikunjaInstance does not exist", func() {
		project := &vikunjav1alpha1.Project{
			Name: resourceName, Namespace: resourceNamespace,
			Spec: vikunjav1alpha1.ProjectSpec{
				InstanceRef: vikunjav1alpha1.InstanceReference{Name: "does-not-exist"},
				Title:       "My Project",
			},
		}
		Expect(k8sClient.Create(ctx, project)).To(Succeed())

		reconcileOnce() // adds finalizer
		reconcileOnce() // fails to resolve the instance

		Expect(k8sClient.Get(ctx, typeNamespacedName, project)).To(Succeed())
		cond := meta.FindStatusCondition(project.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vikunjav1alpha1.ReasonInstanceNotFound))
		Expect(project.Status.ID).To(BeZero())

		Expect(k8sClient.Delete(ctx, project)).To(Succeed())
		reconcileOnce()
	})
})
