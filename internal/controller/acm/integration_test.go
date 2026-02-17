// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	plrv1 "github.com/stolostron/multicloud-operators-placementrule/pkg/apis/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clrapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ramendr/ramen/internal/controller/acm"
	"github.com/ramendr/ramen/internal/controller/util"
)

var _ = Describe("Dual-Backend Selection", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter)))
	})

	Context("With ACM CRDs present", func() {
		It("should detect ACM and instantiate ACM backends", func() {
			scheme := runtime.NewScheme()
			Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())

			policyCRD := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "policies.policy.open-cluster-management.io",
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "policy.open-cluster-management.io",
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Kind:     "Policy",
						Plural:   "policies",
						Singular: "policy",
					},
					Scope: apiextensionsv1.ClusterScoped,
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name:    "v1",
							Served:  true,
							Storage: true,
							Schema: &apiextensionsv1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
									Type: "object",
								},
							},
						},
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policyCRD).
				Build()

			detected, err := acm.DetectACM(ctx, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(detected).To(BeTrue())

			// Verify ACM backend constructors return valid instances
			log := logf.Log.WithName("test-acm-backends")
			secretProp := acm.NewACMSecretPropagator(fakeClient, fakeClient, ctx, log)
			Expect(secretProp).NotTo(BeNil())

			volSyncProp := acm.NewACMVolSyncSecretPropagator(fakeClient)
			Expect(volSyncProp).NotTo(BeNil())

			addonDeployer := acm.NewACMAddonDeployer(fakeClient, log)
			Expect(addonDeployer).NotTo(BeNil())

			placementAdapter := acm.NewACMPlacementAdapter(fakeClient)
			Expect(placementAdapter).NotTo(BeNil())
			Expect(placementAdapter.SupportsPlacementRule()).To(BeTrue())
		})
	})

	Context("Without ACM CRDs", func() {
		It("should not detect ACM and instantiate OCM backends", func() {
			scheme := runtime.NewScheme()
			Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(ocmworkv1.AddToScheme(scheme)).To(Succeed())

			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "ramen-system",
				},
				Data: map[string][]byte{
					"AWS_ACCESS_KEY_ID":     []byte("TESTKEY"),
					"AWS_SECRET_ACCESS_KEY": []byte("TESTSECRET"),
				},
			}

			clusterNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sourceSecret, clusterNS).
				Build()

			// Verify ACM is NOT detected
			detected, err := acm.DetectACM(ctx, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(detected).To(BeFalse())

			// Verify OCM backends work
			log := logf.Log.WithName("test-ocm-backends")
			propagator := acm.NewOCMSecretPropagator(fakeClient, fakeClient, ctx, log)
			Expect(propagator).NotTo(BeNil())

			// Propagate a secret and verify ManifestWork is created (not Policy)
			err = propagator.PropagateSecretToCluster(
				"test-secret", "ramen-system", "test-cluster", "target-ns",
				util.SecretFormatRamen, "",
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify a ManifestWork was created
			mw := &ocmworkv1.ManifestWork{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "test-secret-ramen-secret-mw",
				Namespace: "test-cluster",
			}, mw)
			Expect(err).NotTo(HaveOccurred())

			// Verify the ManifestWork contains the secret
			Expect(mw.Spec.Workload.Manifests).To(HaveLen(1))

			targetSecret := &corev1.Secret{}
			err = json.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, targetSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetSecret.Name).To(Equal("test-secret"))
			Expect(targetSecret.Namespace).To(Equal("target-ns"))
		})
	})

	Context("PlacementRule on OCM hub", func() {
		It("should reject PlacementRule", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			adapter := acm.NewOCMPlacementAdapter(fakeClient)
			Expect(adapter.SupportsPlacementRule()).To(BeFalse())

			_, err := adapter.GetPlacementObject(ctx, corev1.ObjectReference{
				Kind: "PlacementRule",
				Name: "my-placement-rule",
			}, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PlacementRule is not supported"))
		})
	})

	Context("PlacementRule on ACM hub", func() {
		It("should accept PlacementRule", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(plrv1.AddToScheme(scheme)).To(Succeed())
			Expect(clrapiv1beta1.AddToScheme(scheme)).To(Succeed())

			placementRule := &plrv1.PlacementRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-placement-rule",
					Namespace: "default",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(placementRule).
				Build()

			adapter := acm.NewACMPlacementAdapter(fakeClient)
			Expect(adapter.SupportsPlacementRule()).To(BeTrue())

			obj, err := adapter.GetPlacementObject(ctx, corev1.ObjectReference{
				Kind: "PlacementRule",
				Name: "my-placement-rule",
			}, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).NotTo(BeNil())

			pr, ok := obj.(*plrv1.PlacementRule)
			Expect(ok).To(BeTrue())
			Expect(pr.Name).To(Equal("my-placement-rule"))
		})
	})

	Context("Placement on ACM hub", func() {
		It("should accept Placement", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(plrv1.AddToScheme(scheme)).To(Succeed())
			Expect(clrapiv1beta1.AddToScheme(scheme)).To(Succeed())

			placement := &clrapiv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-placement",
					Namespace: "default",
				},
				Spec: clrapiv1beta1.PlacementSpec{},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(placement).
				Build()

			adapter := acm.NewACMPlacementAdapter(fakeClient)

			obj, err := adapter.GetPlacementObject(ctx, corev1.ObjectReference{
				Kind: "Placement",
				Name: "my-placement",
			}, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).NotTo(BeNil())

			p, ok := obj.(*clrapiv1beta1.Placement)
			Expect(ok).To(BeTrue())
			Expect(p.Name).To(Equal("my-placement"))
		})
	})

	Context("Placement on OCM hub", func() {
		It("should accept Placement", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(clrapiv1beta1.AddToScheme(scheme)).To(Succeed())

			placement := &clrapiv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-placement",
					Namespace: "default",
				},
				Spec: clrapiv1beta1.PlacementSpec{},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(placement).
				Build()

			adapter := acm.NewOCMPlacementAdapter(fakeClient)

			obj, err := adapter.GetPlacementObject(ctx, corev1.ObjectReference{
				Kind: "Placement",
				Name: "my-placement",
			}, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).NotTo(BeNil())

			p, ok := obj.(*clrapiv1beta1.Placement)
			Expect(ok).To(BeTrue())
			Expect(p.Name).To(Equal("my-placement"))
		})
	})
})
