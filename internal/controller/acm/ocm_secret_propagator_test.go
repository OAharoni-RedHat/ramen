// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ramendr/ramen/internal/controller/acm"
	"github.com/ramendr/ramen/internal/controller/util"
)

var _ = Describe("OCM Secret Propagator", func() {
	var (
		ctx         context.Context
		testScheme  *runtime.Scheme
		fakeClient  client.Client
		propagator  acm.SecretPropagator
		clusterName string
		secretName  string
		sourceNS    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter)))
		log := logf.Log.WithName("test-ocm-secret-propagator")

		testScheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(testScheme)).To(Succeed())
		Expect(ocmworkv1.AddToScheme(testScheme)).To(Succeed())

		clusterName = "managed-cluster-1"
		secretName = "my-s3-secret"
		sourceNS = "ramen-system"

		sourceSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: sourceNS,
			},
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("AKIAIOSFODNN7EXAMPLE"),
				"AWS_SECRET_ACCESS_KEY": []byte("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			},
		}

		// Create cluster namespace (ManifestWork goes in the managed cluster namespace)
		clusterNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		}

		fakeClient = fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(sourceSecret, clusterNS).
			Build()

		propagator = acm.NewOCMSecretPropagator(fakeClient, fakeClient, ctx, log)
	})

	It("should propagate a secret in Ramen format", func() {
		targetNS := "ramen-ops"
		err := propagator.PropagateSecretToCluster(
			secretName, sourceNS, clusterName, targetNS,
			util.SecretFormatRamen, "",
		)
		Expect(err).NotTo(HaveOccurred())

		// Verify ManifestWork was created in the cluster namespace
		mw := &ocmworkv1.ManifestWork{}
		mwName := secretName + "-" + string(util.SecretFormatRamen) + "-secret-mw"
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name: mwName, Namespace: clusterName,
		}, mw)
		Expect(err).NotTo(HaveOccurred())

		// Verify ManifestWork contains the secret
		Expect(mw.Spec.Workload.Manifests).To(HaveLen(1))

		targetSecret := &corev1.Secret{}
		err = json.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, targetSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(targetSecret.Name).To(Equal(secretName))
		Expect(targetSecret.Namespace).To(Equal(targetNS))
		Expect(targetSecret.Data).To(HaveKey("AWS_ACCESS_KEY_ID"))
		Expect(targetSecret.Data).To(HaveKey("AWS_SECRET_ACCESS_KEY"))
	})

	It("should propagate a secret in Velero format", func() {
		veleroNS := "velero"
		err := propagator.PropagateSecretToCluster(
			secretName, sourceNS, clusterName, "ramen-ops",
			util.SecretFormatVelero, veleroNS,
		)
		Expect(err).NotTo(HaveOccurred())

		// Verify ManifestWork was created
		mw := &ocmworkv1.ManifestWork{}
		mwName := secretName + "-" + string(util.SecretFormatVelero) + "-secret-mw"
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name: mwName, Namespace: clusterName,
		}, mw)
		Expect(err).NotTo(HaveOccurred())

		// Verify ManifestWork contains a secret with Velero credential format
		Expect(mw.Spec.Workload.Manifests).To(HaveLen(1))

		targetSecret := &corev1.Secret{}
		err = json.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, targetSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(targetSecret.Name).To(Equal(util.GenerateVeleroSecretName(secretName)))
		Expect(targetSecret.Namespace).To(Equal(veleroNS))
		Expect(targetSecret.Data).To(HaveKey("cloud"))

		cloudData := string(targetSecret.Data["cloud"])
		Expect(cloudData).To(ContainSubstring("[default]"))
		Expect(cloudData).To(ContainSubstring("aws_access_key_id = AKIAIOSFODNN7EXAMPLE"))
		Expect(cloudData).To(ContainSubstring("aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"))
	})

	It("should remove a secret ManifestWork", func() {
		// First propagate the secret
		err := propagator.PropagateSecretToCluster(
			secretName, sourceNS, clusterName, "ramen-ops",
			util.SecretFormatRamen, "",
		)
		Expect(err).NotTo(HaveOccurred())

		// Verify it exists
		mw := &ocmworkv1.ManifestWork{}
		mwName := secretName + "-" + string(util.SecretFormatRamen) + "-secret-mw"
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name: mwName, Namespace: clusterName,
		}, mw)
		Expect(err).NotTo(HaveOccurred())

		// Now remove it
		err = propagator.RemoveSecretFromCluster(
			secretName, clusterName, sourceNS,
			util.SecretFormatRamen,
		)
		Expect(err).NotTo(HaveOccurred())

		// Verify it was deleted
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name: mwName, Namespace: clusterName,
		}, mw)
		Expect(err).To(HaveOccurred())
	})

	It("should return error for a missing source secret", func() {
		err := propagator.PropagateSecretToCluster(
			"nonexistent-secret", sourceNS, clusterName, "ramen-ops",
			util.SecretFormatRamen, "",
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get source secret"))
	})
})
