/*
Copyright 2025.import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)the Apache License, Version 2.0 (the "License");
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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Constants for repeated strings
const (
	DaprTrustBundleName = "dapr-trust-bundle"
)

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	SourceSecretName string
	TargetNamespace  string
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// This controller monitors the source secret and destination resources (secret and configmap)
// to ensure they are always in sync with the desired state.
func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Only process resources in the target namespace
	if req.Namespace != r.TargetNamespace {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling resource", "namespace", req.Namespace, "name", req.Name, "sourceSecretName", r.SourceSecretName)

	// Determine what type of resource triggered this reconciliation
	var triggerReason string
	switch req.Name {
	case r.SourceSecretName:
		triggerReason = "source secret event"
	case DaprTrustBundleName:
		triggerReason = "destination resource event (secret or configmap)"
	default:
		triggerReason = "unknown resource event"
	}
	log.Info("Reconciliation triggered", "reason", triggerReason, "resource", req.Name)

	// Always try to reconcile based on the source secret
	// This handles cases where:
	// 1. Source secret is created/updated/deleted
	// 2. Destination secret is modified/deleted (we recreate from source)
	// 3. Destination configmap is modified/deleted (we recreate from source)

	// Fetch the source secret
	sourceSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      r.SourceSecretName,
		Namespace: req.Namespace,
	}, sourceSecret)

	if err != nil {
		if errors.IsNotFound(err) {
			// Source secret was deleted, clean up destination resources
			log.Info("Source secret not found, cleaning up destination resources", "sourceSecret", r.SourceSecretName, "namespace", req.Namespace)
			return r.handleSourceSecretDeletion(ctx, req.Namespace)
		}
		log.Error(err, "Failed to get source secret", "sourceSecret", r.SourceSecretName, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	// Ensure both destination resources exist and have correct content
	log.Info("Ensuring destination resources are properly configured", "sourceSecret", sourceSecret.Name, "namespace", sourceSecret.Namespace)

	// Create or update the target secret
	err = r.createOrUpdateTargetSecret(ctx, sourceSecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create or update the target configmap
	err = r.createOrUpdateTargetConfigMap(ctx, sourceSecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handleSourceSecretDeletion handles the case when the source secret is deleted
func (r *SecretReconciler) handleSourceSecretDeletion(ctx context.Context, namespace string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Delete target secret if it exists
	targetSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      DaprTrustBundleName,
		Namespace: namespace,
	}, targetSecret)

	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get target secret for cleanup", "targetSecret", DaprTrustBundleName, "namespace", namespace)
			return ctrl.Result{}, err
		}
		// Target secret doesn't exist, which is fine
		log.Info("Target secret already deleted or doesn't exist", "targetSecret", DaprTrustBundleName, "namespace", namespace)
	} else {
		// Target secret exists, delete it
		log.Info("Deleting target secret since source was deleted", "targetSecret", DaprTrustBundleName, "namespace", namespace)
		if err := r.Delete(ctx, targetSecret); err != nil {
			log.Error(err, "Failed to delete target secret", "targetSecret", DaprTrustBundleName, "namespace", namespace)
			return ctrl.Result{}, err
		}
		log.Info("Successfully deleted target secret", "targetSecret", DaprTrustBundleName, "namespace", namespace)
	}

	// Delete target configmap if it exists
	targetConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      DaprTrustBundleName,
		Namespace: namespace,
	}, targetConfigMap)

	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get target configmap for cleanup", "targetConfigMap", DaprTrustBundleName, "namespace", namespace)
			return ctrl.Result{}, err
		}
		// Target configmap doesn't exist, which is fine
		log.Info("Target configmap already deleted or doesn't exist", "targetConfigMap", DaprTrustBundleName, "namespace", namespace)
	} else {
		// Target configmap exists, delete it
		log.Info("Deleting target configmap since source was deleted", "targetConfigMap", DaprTrustBundleName, "namespace", namespace)
		if err := r.Delete(ctx, targetConfigMap); err != nil {
			log.Error(err, "Failed to delete target configmap", "targetConfigMap", DaprTrustBundleName, "namespace", namespace)
			return ctrl.Result{}, err
		}
		log.Info("Successfully deleted target configmap", "targetConfigMap", DaprTrustBundleName, "namespace", namespace)
	}

	return ctrl.Result{}, nil
}

// createOrUpdateTargetSecret creates or updates the target secret (DaprTrustBundleName)
func (r *SecretReconciler) createOrUpdateTargetSecret(ctx context.Context, sourceSecret *corev1.Secret) error {
	log := logf.FromContext(ctx)

	targetSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DaprTrustBundleName,
			Namespace: sourceSecret.Namespace,
		},
	}

	// Create or update the target secret
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, targetSecret, func() error {
		// Copy all data from source secret
		if targetSecret.Data == nil {
			targetSecret.Data = make(map[string][]byte)
		}

		// Clear existing data and copy from source
		for key := range targetSecret.Data {
			delete(targetSecret.Data, key)
		}

		for key, value := range sourceSecret.Data {
			// Create a copy of the value
			valueCopy := make([]byte, len(value))
			copy(valueCopy, value)

			// Map keys according to renaming rules
			switch key {
			case "ca.crt":
				// Keep ca.crt as is
				targetSecret.Data["ca.crt"] = valueCopy
			case "tls.key":
				// Rename tls.key to issuer.key
				targetSecret.Data["issuer.key"] = valueCopy
			case "tls.crt":
				// Rename tls.crt to issuer.crt
				targetSecret.Data["issuer.crt"] = valueCopy
			default:
				// Skip all other keys - do nothing
			}
		}

		// Set type to Opaque since we're renaming keys and breaking TLS secret format
		targetSecret.Type = corev1.SecretTypeOpaque

		// Add labels to identify this as managed by our controller
		if targetSecret.Labels == nil {
			targetSecret.Labels = make(map[string]string)
		}
		targetSecret.Labels["app.kubernetes.io/managed-by"] = "dapr-trustbundle-operator"
		targetSecret.Labels["app.kubernetes.io/component"] = DaprTrustBundleName

		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create or update target secret")
		return err
	}

	switch op {
	case controllerutil.OperationResultCreated:
		log.Info("Created destination secret", "secret", targetSecret.Name, "namespace", targetSecret.Namespace)
	case controllerutil.OperationResultUpdated:
		log.Info("Updated destination secret for self-healing", "secret", targetSecret.Name, "namespace", targetSecret.Namespace)
	default:
		log.Info("Destination secret already up to date", "secret", targetSecret.Name, "namespace", targetSecret.Namespace)
	}
	return nil
}

// createOrUpdateTargetConfigMap creates or updates the target configmap (DaprTrustBundleName)
func (r *SecretReconciler) createOrUpdateTargetConfigMap(ctx context.Context, sourceSecret *corev1.Secret) error {
	log := logf.FromContext(ctx)

	targetConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DaprTrustBundleName,
			Namespace: sourceSecret.Namespace,
		},
	}

	// Create or update the target configmap
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, targetConfigMap, func() error {
		// Initialize data map if nil
		if targetConfigMap.Data == nil {
			targetConfigMap.Data = make(map[string]string)
		}

		// Clear existing data
		for key := range targetConfigMap.Data {
			delete(targetConfigMap.Data, key)
		}

		// Only copy ca.crt from the source secret
		if caCrt, exists := sourceSecret.Data["ca.crt"]; exists {
			targetConfigMap.Data["ca.crt"] = string(caCrt)
		}

		// Add labels to identify this as managed by our controller
		if targetConfigMap.Labels == nil {
			targetConfigMap.Labels = make(map[string]string)
		}
		targetConfigMap.Labels["app.kubernetes.io/managed-by"] = "dapr-trustbundle-operator"
		targetConfigMap.Labels["app.kubernetes.io/component"] = DaprTrustBundleName

		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create or update target configmap")
		return err
	}
	switch op {
	case controllerutil.OperationResultCreated:
		log.Info("Created destination configmap", "configmap", targetConfigMap.Name, "namespace", targetConfigMap.Namespace)
	case controllerutil.OperationResultUpdated:
		log.Info("Updated destination configmap for self-healing", "configmap", targetConfigMap.Name, "namespace", targetConfigMap.Namespace)
	default:
		log.Info("Destination configmap already up to date", "configmap", targetConfigMap.Name, "namespace", targetConfigMap.Namespace)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			// Watch three types of secrets in the target namespace:
			// 1. Source secret (e.g., configured via SourceSecretName)
			// 2. Destination secret (DaprTrustBundleName)
			if object.GetNamespace() == r.TargetNamespace {
				return object.GetName() == r.SourceSecretName || object.GetName() == DaprTrustBundleName
			}
			return false
		})).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
				// Watch the destination configmap (DaprTrustBundleName)
				return object.GetName() == DaprTrustBundleName && object.GetNamespace() == r.TargetNamespace
			})),
		).
		Named("secret").
		Complete(r)
}
