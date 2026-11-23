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
	"context"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	"github.com/joe-bresee/gitsync/internal/apply"
)

var _ = Describe("ConfigSync Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		configsync := &configsv1alpha1.ConfigSync{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ConfigSync")
			err := k8sClient.Get(ctx, typeNamespacedName, configsync)
			if err != nil && errors.IsNotFound(err) {
				resource := &configsv1alpha1.ConfigSync{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: configsv1alpha1.ConfigSyncSpec{
						Targets: []configsv1alpha1.TargetRef{
							{
								Namespace: "default",
								Name:      "test-target",
								Type:      "Deployment",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &configsv1alpha1.ConfigSync{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ConfigSync")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ConfigSyncReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})

		It("creates and updates a target from a Git source", func() {
			previousDryRun := apply.DryRunEnabled
			apply.DryRunEnabled = false
			defer func() { apply.DryRunEnabled = previousDryRun }()

			repoPath := GinkgoT().TempDir()
			repo, err := git.PlainInit(repoPath, false)
			Expect(err).NotTo(HaveOccurred())
			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())

			manifestPath := filepath.Join(repoPath, "config.yaml")
			writeManifest := func(value string) {
				Expect(os.WriteFile(manifestPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: reconciled-target\ndata:\n  value: "+value+"\n"), 0o600)).To(Succeed())
				_, err = worktree.Add("config.yaml")
				Expect(err).NotTo(HaveOccurred())
				_, err = worktree.Commit("update config", &git.CommitOptions{
					Author: &object.Signature{Name: "test", Email: "test@example.com"},
				})
				Expect(err).NotTo(HaveOccurred())
			}
			writeManifest("one")

			name := types.NamespacedName{Name: "git-target-sync", Namespace: "default"}
			resource := &configsv1alpha1.ConfigSync{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: configsv1alpha1.ConfigSyncSpec{
					Source: configsv1alpha1.SourceSpec{Git: &configsv1alpha1.GitSource{
						RepoURL: repoPath, Path: "config.yaml", AuthMethod: "none",
					}},
					Targets: []configsv1alpha1.TargetRef{{Namespace: "default", Name: "reconciled-target", Type: "ConfigMap"}},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}()

			reconciler := &ConfigSyncReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			target := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "reconciled-target", Namespace: "default"}, target)).To(Succeed())
			Expect(target.Data["value"]).To(Equal("one"))

			writeManifest("two")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "reconciled-target", Namespace: "default"}, target)).To(Succeed())
			Expect(target.Data["value"]).To(Equal("two"))

			Expect(k8sClient.Delete(ctx, target)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "reconciled-target", Namespace: "default"}, target)).To(Succeed())
			Expect(target.Data["value"]).To(Equal("two"))
		})
	})
})
