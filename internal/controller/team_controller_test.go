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

var _ = Describe("Team Controller", func() {
	const (
		resourceName      = "team-controller-test"
		resourceNamespace = "default"
	)

	var (
		ctx                context.Context
		fake               *fakeVikunjaServer
		instanceRef        vikunjav1alpha1.InstanceReference
		typeNamespacedName types.NamespacedName
		reconciler         *TeamReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		var server *httptest.Server
		server, fake = newFakeVikunjaServer()
		DeferCleanup(server.Close)

		instanceRef = createReadyInstance(ctx, resourceName+"-instance", server)
		typeNamespacedName = types.NamespacedName{Name: resourceName, Namespace: resourceNamespace}
		reconciler = &TeamReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	reconcileOnce := func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
	}

	It("creates the team, reconciles membership, and deletes the team", func() {
		team := &vikunjav1alpha1.Team{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: resourceNamespace},
			Spec: vikunjav1alpha1.TeamSpec{
				InstanceRef: instanceRef,
				Name:        "Sample Team",
				Members: []vikunjav1alpha1.TeamMemberSpec{
					{Username: testUsernameAlice, Admin: true},
					{Username: testUsernameBob},
				},
			},
		}
		Expect(k8sClient.Create(ctx, team)).To(Succeed())

		reconcileOnce() // adds finalizer
		reconcileOnce() // creates the team and reconciles members

		Expect(k8sClient.Get(ctx, typeNamespacedName, team)).To(Succeed())
		Expect(team.Status.ID).NotTo(BeZero())
		Expect(meta.IsStatusConditionTrue(team.Status.Conditions, vikunjav1alpha1.ConditionTypeReady)).To(BeTrue())

		remote := fake.teams[team.Status.ID]
		Expect(remote).NotTo(BeNil())
		byUsername := map[string]bool{}
		for _, m := range remote.Members {
			byUsername[m.Username] = m.Admin
		}
		// The token owner ("operator") is kept as an implicit admin member
		// alongside the two members declared in spec.
		Expect(byUsername).To(HaveLen(3))
		Expect(byUsername[testUsernameAlice]).To(BeTrue())
		Expect(byUsername[testUsernameBob]).To(BeFalse())
		Expect(byUsername).To(HaveKey(testUsernameOperator))

		By("removing bob and promoting alice's admin flag off, while adding no one new")
		team.Spec.Members = []vikunjav1alpha1.TeamMemberSpec{{Username: testUsernameAlice, Admin: false}}
		Expect(k8sClient.Update(ctx, team)).To(Succeed())
		reconcileOnce()

		remote = fake.teams[team.Status.ID]
		byUsername = map[string]bool{}
		for _, m := range remote.Members {
			byUsername[m.Username] = m.Admin
		}
		Expect(byUsername).To(HaveLen(2))
		Expect(byUsername[testUsernameAlice]).To(BeFalse())
		Expect(byUsername).NotTo(HaveKey(testUsernameBob))
		Expect(byUsername).To(HaveKey(testUsernameOperator)) // never removed: it's the token owner

		Expect(k8sClient.Delete(ctx, team)).To(Succeed())
		reconcileOnce()

		Expect(fake.teams).NotTo(HaveKey(team.Status.ID))
		err := k8sClient.Get(ctx, typeNamespacedName, team)
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})
})
