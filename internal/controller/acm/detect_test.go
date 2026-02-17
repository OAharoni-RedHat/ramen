// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/ramendr/ramen/internal/controller/acm"
)

var _ = Describe("DetectACM", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
	})

	It("should detect ACM when Policy CRD exists", func() {
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
	})

	It("should not detect ACM when Policy CRD does not exist", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		detected, err := acm.DetectACM(ctx, fakeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(detected).To(BeFalse())
	})

	It("should return error on client failure", func() {
		expectedErr := fmt.Errorf("connection refused")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(
					ctx context.Context,
					client client.WithWatch,
					key client.ObjectKey,
					obj client.Object,
					opts ...client.GetOption,
				) error {
					return expectedErr
				},
			}).
			Build()

		detected, err := acm.DetectACM(ctx, fakeClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
		Expect(detected).To(BeFalse())
	})
})
