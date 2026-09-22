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

var _ = Describe("Task Controller", func() {
	const (
		resourceName      = "task-controller-test"
		projectName       = "task-controller-test-project"
		labelName         = "task-controller-test-label"
		resourceNamespace = "default"
	)

	var (
		ctx                context.Context
		fake               *fakeVikunjaServer
		instanceRef        vikunjav1alpha1.InstanceReference
		typeNamespacedName types.NamespacedName
		reconciler         *TaskReconciler
		project            *vikunjav1alpha1.Project
		label              *vikunjav1alpha1.Label
	)

	BeforeEach(func() {
		ctx = context.Background()
		var server *httptest.Server
		server, fake = newFakeVikunjaServer()
		DeferCleanup(server.Close)

		instanceRef = createReadyInstance(ctx, resourceName+"-instance", server)
		typeNamespacedName = types.NamespacedName{Name: resourceName, Namespace: resourceNamespace}
		reconciler = &TaskReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		By("standing up a ready Project for the task to belong to")
		project = &vikunjav1alpha1.Project{
			Name: projectName, Namespace: resourceNamespace,
			Spec: vikunjav1alpha1.ProjectSpec{InstanceRef: instanceRef, Title: "Task Test Project"},
		}
		Expect(k8sClient.Create(ctx, project)).To(Succeed())
		projectReconciler := &ProjectReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		projectNN := types.NamespacedName{Name: projectName, Namespace: resourceNamespace}
		_, err := projectReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: projectNN}) // finalizer
		Expect(err).NotTo(HaveOccurred())
		_, err = projectReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: projectNN}) // create
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, projectNN, project)).To(Succeed())
		Expect(project.Status.ID).NotTo(BeZero())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, project)
			_, _ = projectReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: projectNN})
		})

		By("standing up a ready Label for the task to reference")
		label = &vikunjav1alpha1.Label{
			Name: labelName, Namespace: resourceNamespace,
			Spec: vikunjav1alpha1.LabelSpec{InstanceRef: instanceRef, Title: "bug"},
		}
		Expect(k8sClient.Create(ctx, label)).To(Succeed())
		labelReconciler := &LabelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		labelNN := types.NamespacedName{Name: labelName, Namespace: resourceNamespace}
		_, err = labelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: labelNN}) // finalizer
		Expect(err).NotTo(HaveOccurred())
		_, err = labelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: labelNN}) // create
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, labelNN, label)).To(Succeed())
		Expect(label.Status.ID).NotTo(BeZero())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, label)
			_, _ = labelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: labelNN})
		})
	})

	reconcileOnce := func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
	}

	It("creates the task with labels and assignees, reconciles changes, and deletes it", func() {
		task := &vikunjav1alpha1.Task{
			Name: resourceName, Namespace: resourceNamespace,
			Spec: vikunjav1alpha1.TaskSpec{
				ProjectRef: projectName,
				Title:      "My Task",
				LabelRefs:  []string{labelName},
				Assignees:  []string{testUsernameAlice},
			},
		}
		Expect(k8sClient.Create(ctx, task)).To(Succeed())

		reconcileOnce() // adds finalizer
		reconcileOnce() // creates on the fake instance, attaches label + assignee

		Expect(k8sClient.Get(ctx, typeNamespacedName, task)).To(Succeed())
		Expect(task.Status.ID).NotTo(BeZero())
		Expect(task.Status.Identifier).NotTo(BeEmpty())
		Expect(meta.IsStatusConditionTrue(task.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)).To(BeTrue())

		remote := fake.tasks[task.Status.ID]
		Expect(remote).NotTo(BeNil())
		Expect(remote.Title).To(Equal("My Task"))
		Expect(remote.ProjectID).To(Equal(project.Status.ID))
		Expect(remote.Labels).To(HaveLen(1))
		Expect(remote.Labels[0].ID).To(Equal(label.Status.ID))
		Expect(remote.Assignees).To(HaveLen(1))
		Expect(remote.Assignees[0].Username).To(Equal(testUsernameAlice))

		By("swapping the assignee and clearing the labels")
		task.Spec.Assignees = []string{testUsernameBob}
		task.Spec.LabelRefs = nil
		task.Spec.Done = true
		Expect(k8sClient.Update(ctx, task)).To(Succeed())
		reconcileOnce()

		remote = fake.tasks[task.Status.ID]
		Expect(remote.Done).To(BeTrue())
		Expect(remote.Labels).To(BeEmpty())
		Expect(remote.Assignees).To(HaveLen(1))
		Expect(remote.Assignees[0].Username).To(Equal(testUsernameBob))

		By("deleting the resource")
		Expect(k8sClient.Delete(ctx, task)).To(Succeed())
		reconcileOnce()

		Expect(fake.tasks).NotTo(HaveKey(task.Status.ID))
		err := k8sClient.Get(ctx, typeNamespacedName, task)
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})

	It("reports NotReady while the referenced Project does not exist", func() {
		task := &vikunjav1alpha1.Task{
			Name: resourceName, Namespace: resourceNamespace,
			Spec: vikunjav1alpha1.TaskSpec{ProjectRef: "does-not-exist", Title: "My Task"},
		}
		Expect(k8sClient.Create(ctx, task)).To(Succeed())

		reconcileOnce() // adds finalizer
		reconcileOnce() // fails to resolve the project

		Expect(k8sClient.Get(ctx, typeNamespacedName, task)).To(Succeed())
		cond := meta.FindStatusCondition(task.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vikunjav1alpha1.ReasonDependencyNotReady))
		Expect(task.Status.ID).To(BeZero())

		Expect(k8sClient.Delete(ctx, task)).To(Succeed())
		reconcileOnce()
	})
})
