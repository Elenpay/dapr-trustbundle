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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Constants for test values
const (
	DaprSystemNamespace          = "dapr-system"
	sourceSecretName            = "dapr-trust-bundle-cert-manager"
	testCACertKey               = "ca.crt"
	testTLSKey                  = "tls.key"
	testTLSCert                 = "tls.crt"
	testIssuerKey               = "issuer.key"
	testIssuerCert              = "issuer.crt"
)

var _ = Describe("SecretReconciler", func() {
	var (
		ctx        context.Context
		reconciler *SecretReconciler
		fakeClient client.Client
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		reconciler = &SecretReconciler{
			Client:           fakeClient,
			Scheme:           scheme,
			SourceSecretName: sourceSecretName,
			TargetNamespace:  DaprSystemNamespace,
		}
	})

	Describe("Key Renaming Logic", func() {
		It("should rename keys correctly when creating destination secret", func() {
			namespace := DaprSystemNamespace

			// Create source secret with TLS certificate format
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					testCACertKey:  []byte("test-ca-certificate"),
					testTLSKey:     []byte("test-private-key"),
					testTLSCert:    []byte("test-tls-certificate"),
					"extra.data": []byte("should-be-filtered"),
				},
			}

			Expect(fakeClient.Create(ctx, sourceSecret)).To(Succeed())

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify destination secret was created with correct key renaming
			destSecret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destSecret)
			Expect(err).NotTo(HaveOccurred())

			// Check key renaming according to README:
			// ca.crt → ca.crt (unchanged)
			// tls.key → issuer.key
			// tls.crt → issuer.crt
			// All other keys are filtered out
			Expect(destSecret.Data).To(HaveKey(testCACertKey))
			Expect(destSecret.Data).To(HaveKey(testIssuerKey))
			Expect(destSecret.Data).To(HaveKey(testIssuerCert))
			Expect(destSecret.Data).NotTo(HaveKey(testTLSKey))
			Expect(destSecret.Data).NotTo(HaveKey(testTLSCert))
			Expect(destSecret.Data).NotTo(HaveKey("extra.data"))

			// Check values are copied correctly
			Expect(string(destSecret.Data[testCACertKey])).To(Equal("test-ca-certificate"))
			Expect(string(destSecret.Data[testIssuerKey])).To(Equal("test-private-key"))
			Expect(string(destSecret.Data[testIssuerCert])).To(Equal("test-tls-certificate"))

			// Check secret type is changed to Opaque
			Expect(destSecret.Type).To(Equal(corev1.SecretTypeOpaque))

			// Check management labels
			Expect(destSecret.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "dapr-trustbundle-operator"))
			Expect(destSecret.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", DaprTrustBundleName))
		})

		It("should create configmap with only ca.crt", func() {
			namespace := DaprSystemNamespace

			// Create source secret
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					testCACertKey:  []byte("test-ca-certificate"),
					testTLSKey:     []byte("test-private-key"),
					testTLSCert:    []byte("test-tls-certificate"),
					"other.data": []byte("should-be-ignored"),
				},
			}

			Expect(fakeClient.Create(ctx, sourceSecret)).To(Succeed())

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify configmap was created with only ca.crt
			destConfigMap := &corev1.ConfigMap{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destConfigMap)
			Expect(err).NotTo(HaveOccurred())

			// Check that only ca.crt is present
			Expect(destConfigMap.Data).To(HaveKey(testCACertKey))
			Expect(destConfigMap.Data).To(HaveLen(1))
			Expect(destConfigMap.Data[testCACertKey]).To(Equal("test-ca-certificate"))

			// Check management labels
			Expect(destConfigMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "dapr-trustbundle-operator"))
			Expect(destConfigMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", DaprTrustBundleName))
		})

		It("should handle missing ca.crt gracefully", func() {
			namespace := DaprSystemNamespace

			// Create source secret without ca.crt
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					testTLSKey:  []byte("test-private-key"),
					testTLSCert: []byte("test-tls-certificate"),
				},
			}

			Expect(fakeClient.Create(ctx, sourceSecret)).To(Succeed())

			// Reconcile should not fail even without ca.crt
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify destination secret was created with available keys
			destSecret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destSecret)
			Expect(err).NotTo(HaveOccurred())

			Expect(destSecret.Data).NotTo(HaveKey(testCACertKey))
			Expect(destSecret.Data).To(HaveKey(testIssuerKey))
			Expect(destSecret.Data).To(HaveKey(testIssuerCert))

			// Verify configmap has no data since ca.crt is missing
			destConfigMap := &corev1.ConfigMap{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(destConfigMap.Data).NotTo(HaveKey(testCACertKey))
			Expect(destConfigMap.Data).To(BeEmpty())
		})

		It("should update destination resources when source secret is updated", func() {
			namespace := DaprSystemNamespace

			// Create initial source secret
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					testCACertKey:  []byte("initial-ca-certificate"),
					testTLSKey:     []byte("initial-private-key"),
					testTLSCert:    []byte("initial-tls-certificate"),
				},
			}

			Expect(fakeClient.Create(ctx, sourceSecret)).To(Succeed())

			// Initial reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Update source secret
			sourceSecret.Data[testCACertKey] = []byte("updated-ca-certificate")
			sourceSecret.Data[testTLSCert] = []byte("updated-tls-certificate")
			Expect(fakeClient.Update(ctx, sourceSecret)).To(Succeed())

			// Reconcile again
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify destination secret was updated
			destSecret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destSecret)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(destSecret.Data[testCACertKey])).To(Equal("updated-ca-certificate"))
			Expect(string(destSecret.Data[testIssuerCert])).To(Equal("updated-tls-certificate"))
			Expect(string(destSecret.Data[testIssuerKey])).To(Equal("initial-private-key")) // unchanged

			// Verify configmap was updated
			destConfigMap := &corev1.ConfigMap{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(destConfigMap.Data[testCACertKey]).To(Equal("updated-ca-certificate"))
		})
	})

	Describe("Self-Healing Logic", func() {
		It("should recreate destination secret when it's deleted", func() {
			namespace := DaprSystemNamespace

			// Create source secret
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					testCACertKey: []byte("test-ca-certificate"),
					testTLSKey:    []byte("test-private-key"),
				},
			}
			Expect(fakeClient.Create(ctx, sourceSecret)).To(Succeed())

			// Initial reconcile to create destination secret
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
			}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify destination secret exists
			destSecret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destSecret)
			Expect(err).NotTo(HaveOccurred())

			// Delete destination secret (simulating external deletion)
			Expect(fakeClient.Delete(ctx, destSecret)).To(Succeed())

			// Reconcile again when destination secret is deleted
			// This simulates the operator receiving an event for the deleted destination secret
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      DaprTrustBundleName,
					Namespace: namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify destination secret was recreated with self-healing
			destSecret = &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(destSecret.Data).To(HaveKey(testCACertKey))
			Expect(destSecret.Data).To(HaveKey(testIssuerKey))
		})
	})

	Describe("Cleanup Logic", func() {
		It("should delete destination resources when source secret is deleted", func() {
			namespace := DaprSystemNamespace

			// Create source secret
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					testCACertKey: []byte("test-ca-certificate"),
				},
			}
			Expect(fakeClient.Create(ctx, sourceSecret)).To(Succeed())

			// Initial reconcile to create destination resources
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sourceSecretName,
					Namespace: namespace,
				},
			}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify destination resources exist
			destSecret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destSecret)
			Expect(err).NotTo(HaveOccurred())

			destConfigMap := &corev1.ConfigMap{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destConfigMap)
			Expect(err).NotTo(HaveOccurred())

			// Delete source secret
			Expect(fakeClient.Delete(ctx, sourceSecret)).To(Succeed())

			// Reconcile after source secret deletion
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify destination resources were deleted
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destSecret)
			Expect(err).To(HaveOccurred())

			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      DaprTrustBundleName,
				Namespace: namespace,
			}, destConfigMap)
			Expect(err).To(HaveOccurred())
		})
	})
})
