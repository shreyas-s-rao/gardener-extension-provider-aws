// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mutator_test

import (
	"context"
	"time"

	calicov1alpha1 "github.com/gardener/gardener-extension-networking-calico/pkg/apis/calico/v1alpha1"
	ciliumv1alpha1 "github.com/gardener/gardener-extension-networking-cilium/pkg/apis/cilium/v1alpha1"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	"github.com/gardener/gardener-extension-provider-aws/pkg/admission/mutator"
	"github.com/gardener/gardener-extension-provider-aws/pkg/aws"
)

var _ = Describe("Shoot mutator", func() {

	Describe("#Mutate", func() {
		const namespace = "garden-dev"

		var (
			shootMutator extensionswebhook.Mutator
			shoot        *gardencorev1beta1.Shoot
			oldShoot     *gardencorev1beta1.Shoot
			ctx          = context.TODO()
			now          = metav1.Now()
		)

		BeforeEach(func() {
			shootMutator = mutator.NewShootMutator()
			scheme := runtime.NewScheme()
			Expect(gardencorev1beta1.AddToScheme(scheme)).To(Succeed())
			Expect(shootMutator.(inject.Scheme).InjectScheme(scheme)).To(Succeed())

			shoot = &gardencorev1beta1.Shoot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: namespace,
				},
				Spec: gardencorev1beta1.ShootSpec{
					Kubernetes: gardencorev1beta1.Kubernetes{
						Version: "1.24.7",
					},
					SeedName: pointer.String("aws"),
					Provider: gardencorev1beta1.Provider{
						Type: aws.Type,
					},
					Region: "us-west-1",
					Networking: gardencorev1beta1.Networking{
						Nodes: pointer.String("10.250.0.0/16"),
						Type:  "calico",
					},
				},
			}

			oldShoot = &gardencorev1beta1.Shoot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: namespace,
				},
				Spec: gardencorev1beta1.ShootSpec{
					Kubernetes: gardencorev1beta1.Kubernetes{
						Version: "1.24.7",
					},
					SeedName: pointer.String("aws"),
					Provider: gardencorev1beta1.Provider{
						Type: aws.Type,
					},
					Region: "us-west-1",
					Networking: gardencorev1beta1.Networking{
						Nodes: pointer.String("10.250.0.0/16"),
						Type:  "calico",
					},
				},
			}
		})

		Context("Mutate shoot networking providerconfig for type calico", func() {

			It("should return without mutation when shoot is in scheduled to new seed phase", func() {
				shoot.Status.LastOperation = &gardencorev1beta1.LastOperation{
					Description:    "test",
					LastUpdateTime: metav1.Time{Time: metav1.Now().Add(time.Second * -1000)},
					Progress:       0,
					Type:           gardencorev1beta1.LastOperationTypeReconcile,
					State:          gardencorev1beta1.LastOperationStateProcessing,
				}
				shoot.Status.SeedName = pointer.String("gcp")
				shootExpected := shoot.DeepCopy()
				err := shootMutator.Mutate(ctx, shoot, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot).To(DeepEqual(shootExpected))
			})

			It("should return without mutation when shoot is in migration or restore phase", func() {
				shoot.Status.LastOperation = &gardencorev1beta1.LastOperation{
					Description:    "test",
					LastUpdateTime: metav1.Time{Time: metav1.Now().Add(time.Second * -1000)},
					Progress:       0,
					Type:           gardencorev1beta1.LastOperationTypeMigrate,
					State:          gardencorev1beta1.LastOperationStateProcessing,
				}
				shoot.Status.SeedName = pointer.String("aws")
				shootExpected := shoot.DeepCopy()
				err := shootMutator.Mutate(ctx, shoot, shoot)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot).To(DeepEqual(shootExpected))
			})

			It("should return without mutation when shoot is in deletion phase", func() {
				shoot.DeletionTimestamp = &now
				shootExpected := shoot.DeepCopy()
				err := shootMutator.Mutate(ctx, shoot, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot).To(DeepEqual(shootExpected))
			})

			It("should return without mutation when shoot specs have not changed", func() {
				shootWithAnnotations := shoot.DeepCopy()
				shootWithAnnotations.Annotations = map[string]string{"foo": "bar"}
				shootExpected := shootWithAnnotations.DeepCopy()

				err := shootMutator.Mutate(ctx, shootWithAnnotations, shoot)
				Expect(err).To(BeNil())
				Expect(shootWithAnnotations).To(DeepEqual(shootExpected))
			})

			It("should disable overlay for a new shoot", func() {
				err := shootMutator.Mutate(ctx, shoot, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot.Spec.Networking.ProviderConfig).To(Equal(&runtime.RawExtension{
					Object: &calicov1alpha1.NetworkConfig{
						Overlay: &calicov1alpha1.Overlay{
							Enabled: false,
						},
					},
				}))
			})

			It("should take overlay field value from old shoot when unspecified in new shoot", func() {
				oldShoot.Spec.Networking.ProviderConfig = &runtime.RawExtension{
					Raw: []byte(`{"overlay":{"enabled":true}}`),
					Object: &calicov1alpha1.NetworkConfig{
						Overlay: &calicov1alpha1.Overlay{
							Enabled: true,
						},
					},
				}
				err := shootMutator.Mutate(ctx, shoot, oldShoot)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot.Spec.Networking.ProviderConfig).To(Equal(&runtime.RawExtension{
					Object: &calicov1alpha1.NetworkConfig{
						Overlay: &calicov1alpha1.Overlay{
							Enabled: true,
						},
					},
				}))
			})
		})

		Context("Mutate shoot networking providerconfig for type cilium", func() {

			BeforeEach(func() {
				shoot.Spec.Networking.Type = "cilium"
				oldShoot.Spec.Networking.Type = "cilium"
			})

			It("should return without mutation when shoot is in scheduled to new seed phase", func() {
				shoot.Status.LastOperation = &gardencorev1beta1.LastOperation{
					Description:    "test",
					LastUpdateTime: metav1.Time{Time: metav1.Now().Add(time.Second * -1000)},
					Progress:       0,
					Type:           gardencorev1beta1.LastOperationTypeReconcile,
					State:          gardencorev1beta1.LastOperationStateProcessing,
				}
				shoot.Status.SeedName = pointer.String("gcp")
				shootExpected := shoot.DeepCopy()
				err := shootMutator.Mutate(ctx, shoot, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot).To(DeepEqual(shootExpected))
			})

			It("should return without mutation when shoot is in migration or restore phase", func() {
				shoot.Status.LastOperation = &gardencorev1beta1.LastOperation{
					Description:    "test",
					LastUpdateTime: metav1.Time{Time: metav1.Now().Add(time.Second * -1000)},
					Progress:       0,
					Type:           gardencorev1beta1.LastOperationTypeMigrate,
					State:          gardencorev1beta1.LastOperationStateProcessing,
				}
				shoot.Status.SeedName = pointer.String("aws")
				shootExpected := shoot.DeepCopy()
				err := shootMutator.Mutate(ctx, shoot, shoot)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot).To(DeepEqual(shootExpected))
			})

			It("should return without mutation when shoot is in deletion phase", func() {
				shoot.DeletionTimestamp = &now
				shootExpected := shoot.DeepCopy()
				err := shootMutator.Mutate(ctx, shoot, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot).To(DeepEqual(shootExpected))
			})

			It("should return without mutation when shoot specs have not changed", func() {
				shootWithAnnotations := shoot.DeepCopy()
				shootWithAnnotations.Annotations = map[string]string{"foo": "bar"}
				shootExpected := shootWithAnnotations.DeepCopy()

				err := shootMutator.Mutate(ctx, shootWithAnnotations, shoot)
				Expect(err).To(BeNil())
				Expect(shootWithAnnotations).To(DeepEqual(shootExpected))
			})

			It("should disable overlay for a new shoot", func() {
				err := shootMutator.Mutate(ctx, shoot, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot.Spec.Networking.ProviderConfig).To(Equal(&runtime.RawExtension{
					Object: &ciliumv1alpha1.NetworkConfig{
						Overlay: &ciliumv1alpha1.Overlay{
							Enabled: false,
						},
					},
				}))
			})

			It("should take overlay field value from old shoot when unspecified in new shoot", func() {
				oldShoot.Spec.Networking.ProviderConfig = &runtime.RawExtension{
					Raw: []byte(`{"overlay":{"enabled":true}}`),
					Object: &ciliumv1alpha1.NetworkConfig{
						Overlay: &ciliumv1alpha1.Overlay{
							Enabled: true,
						},
					},
				}
				err := shootMutator.Mutate(ctx, shoot, oldShoot)
				Expect(err).NotTo(HaveOccurred())
				Expect(shoot.Spec.Networking.ProviderConfig).To(Equal(&runtime.RawExtension{
					Object: &ciliumv1alpha1.NetworkConfig{
						Overlay: &ciliumv1alpha1.Overlay{
							Enabled: true,
						},
					},
				}))
			})
		})
	})
})
