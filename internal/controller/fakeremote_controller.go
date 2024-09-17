/*
Copyright 2024.

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
	"encoding/json"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testopv1 "gitlab.com/josvaz/testop/api/v1"
)

// Definitions to manage status conditions
const (
	// typeAvailableFakeRemote represents the available status of the FakeRemote reconciliation
	typeAvailableFakeRemote = "Available"

	// typeFailedFakeRemote represents the failed status of the FakeRemote reconciliation
	typeFailedFakeRemote = "Failed"
)

// Definitions to manage annotations
const (
	// lastAppliedConfigAnnotation stores the latests applied config
	lastAppliedConfigAnnotation = "testop/last-applied-configuration"

	// lastSkippedConfigAnnotation stores the latest skipped config
	lastSkippedConfigAnnotation = "testop/last-skipped-configuration"

	// resourcePolicyAnnotation holds policy info for reconciliation
	resourcePolicyAnnotation = "testop/atlas-resource-policy"

	// reconciliationPolicySkip marks the resource should not be reconciled
	reconciliationPolicySkip = "skip"
)

// FakeRemoteFinalizerLabel marks not to remove a Fake Remote until reconciled
const FakeRemoteFinalizerLabel = "mongodbatlas/finalizer"

// FakeRemoteReconciler reconciles a FakeRemote object
type FakeRemoteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=testop.gitlab.com,resources=fakeremotes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=testop.gitlab.com,resources=fakeremotes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=testop.gitlab.com,resources=fakeremotes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the FakeRemote object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *FakeRemoteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Memcached instance
	// The purpose is check if the Custom Resource for the Kind Memcached
	// is applied on the cluster if not we return nil to stop the reconciliation
	fakeRemote := &testopv1.FakeRemote{}
	err := r.Get(ctx, req.NamespacedName, fakeRemote)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("fake remote resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get fake remote")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status is available
	if fakeRemote.Status.Conditions == nil || len(fakeRemote.Status.Conditions) == 0 {
		meta.SetStatusCondition(&fakeRemote.Status.Conditions, metav1.Condition{
			Type:    typeAvailableFakeRemote,
			Status:  metav1.ConditionUnknown,
			Reason:  "Reconciling",
			Message: "Starting reconciliation",
		}) // mark but no not update in etcd
	}

	// Let's add a finalizer. Then, we can define some operations which should
	// occur before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(fakeRemote, FakeRemoteFinalizerLabel) {
		log.Info("Adding Finalizer for Fake Remote")
		original := fakeRemote.DeepCopy()
		if ok := controllerutil.AddFinalizer(fakeRemote, FakeRemoteFinalizerLabel); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}
		if err := r.Patch(ctx, fakeRemote, client.MergeFrom(original)); err != nil {
			log.Error(err, "Failed to patch in finalizer fo Fake Remote")
			return ctrl.Result{}, err
		}
	}

	return r.reconcile(ctx, fakeRemote)
}

// SetupWithManager sets up the controller with the Manager.
func (r *FakeRemoteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testopv1.FakeRemote{}).
		Complete(r)
}

func (r *FakeRemoteReconciler) reconcile(ctx context.Context, fakeRemote *testopv1.FakeRemote) (ctrl.Result, error) {
	if hasPolicySkip(fakeRemote) {
		return r.skip(ctx, fakeRemote)
	}

	previousSpec, err := getLastAppliedConfig(fakeRemote)
	if err != nil {
		return r.fail(ctx, fakeRemote, err)
	}
	deleting := (fakeRemote.GetDeletionTimestamp() != nil)
	switch {
	case deleting:
		return r.delete(ctx, fakeRemote)
	case previousSpec == nil:
		return r.create(ctx, fakeRemote)
	case !reflect.DeepEqual(previousSpec, fakeRemote.Spec):
		return r.update(ctx, fakeRemote, previousSpec)
	}
	return ctrl.Result{}, nil
}

func (r *FakeRemoteReconciler) create(ctx context.Context, fakeRemote *testopv1.FakeRemote) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	original := fakeRemote.DeepCopy()

	log.Info("Setting Fake Remote initial state...")
	log.Info("Set field", "someOtherField", fakeRemote.Spec.SomeOtherField)
	log.Info("Set list", "dependents", fakeRemote.Spec.Dependents)

	if err := setLastAppliedConfig(fakeRemote); err != nil {
		r.fail(ctx, fakeRemote, err)
	}
	if err := r.apply(ctx, original, fakeRemote, "Set"); err != nil {
		return r.fail(ctx, fakeRemote, err)
	}
	return r.ok(ctx, fakeRemote)
}

func (r *FakeRemoteReconciler) update(ctx context.Context, fakeRemote *testopv1.FakeRemote, previousSpec *testopv1.FakeRemoteSpec) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Skip reconciliation if the spec did not change
	if r.specUnchanged(ctx, fakeRemote) {
		log.Info("Skip update as spec did not change")
		return ctrl.Result{}, nil
	}

	original := fakeRemote.DeepCopy()

	log.Info("Updating Fake Remote state...")
	if fakeRemote.Spec.SomeOtherField != previousSpec.SomeOtherField {
		log.Info("Updated field",
			"old someOtherField", previousSpec.SomeOtherField,
			"new someOtherField", fakeRemote.Spec.SomeOtherField)
	}

	lastSkippedConfig, err := getLastSkippedConfig(fakeRemote)
	if err != nil {
		return r.fail(ctx, fakeRemote, err)
	}
	if areDependentsDisabled(fakeRemote.Spec.Dependents, lastSkippedConfig) {
		log.Info("Dependents are Disabled")
	} else if !reflect.DeepEqual(fakeRemote.Spec.Dependents, previousSpec.Dependents) {
		log.Info("Updated list",
			"old dependents", previousSpec.Dependents,
			"new dependents", fakeRemote.Spec.Dependents)
	}

	if err := setLastAppliedConfig(fakeRemote); err != nil {
		return r.fail(ctx, fakeRemote, err)
	}
	if err := r.apply(ctx, original, fakeRemote, "Updated"); err != nil {
		return r.fail(ctx, fakeRemote, err)
	}
	return r.ok(ctx, fakeRemote)
}

func (r *FakeRemoteReconciler) apply(ctx context.Context, original, fakeRemote *testopv1.FakeRemote, msg string) error {
	log := log.FromContext(ctx)

	if err := r.Patch(ctx, fakeRemote, client.MergeFrom(original)); err != nil {
		log.Error(err, "Failed to patch Fake Remote annotations")
		return err
	}
	meta.SetStatusCondition(&fakeRemote.Status.Conditions, metav1.Condition{
		Type:    typeAvailableFakeRemote,
		Status:  metav1.ConditionTrue,
		Reason:  "ReconcileOK",
		Message: msg,
	})
	if err := r.Status().Update(ctx, fakeRemote); err != nil {
		log.Error(err, "Failed to update Fake Remote status")
		return err
	}

	return nil
}

func (r *FakeRemoteReconciler) ok(ctx context.Context, fakeRemote *testopv1.FakeRemote) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconcile completed OK")
	return ctrl.Result{}, nil
}

func (r *FakeRemoteReconciler) specUnchanged(ctx context.Context, fakeRemote *testopv1.FakeRemote) bool {
	log := log.FromContext(ctx)
	lastConfig, err := getLastAppliedConfig(fakeRemote)
	if err != nil {
		log.Error(err, "Failed to read last applied config")
		return false
	}
	return reflect.DeepEqual(lastConfig, &fakeRemote.Spec)
}

func (r *FakeRemoteReconciler) delete(ctx context.Context, fakeRemote *testopv1.FakeRemote) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	original := fakeRemote.DeepCopy()

	log.Info("Deleting Fake Remote...")
	log.Info("Deleted field", "someOtherField", fakeRemote.Spec.SomeOtherField)
	log.Info("Deleted list", "dependents", fakeRemote.Spec.Dependents)

	controllerutil.RemoveFinalizer(fakeRemote, FakeRemoteFinalizerLabel)
	if err := r.Patch(ctx, fakeRemote, client.MergeFrom(original)); err != nil {
		log.Error(err, "Failed to remove Fake Remote finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *FakeRemoteReconciler) skip(ctx context.Context, fakeRemote *testopv1.FakeRemote) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	nextSkippedConfig, err := computeConfig(&fakeRemote.Spec)
	if err != nil {
		log.Error(err, "Failed to skip Fake Remote")
		return ctrl.Result{}, err
	}
	lastSkippedConfig := fakeRemote.GetAnnotations()[lastSkippedConfigAnnotation]
	if nextSkippedConfig != lastSkippedConfig {
		original := fakeRemote.DeepCopy()
		fakeRemote.GetAnnotations()[lastSkippedConfigAnnotation] = nextSkippedConfig
		if err = r.Patch(ctx, fakeRemote, client.MergeFrom(original)); err != nil {
			log.Error(err, "Failed to skip while updating Fake Remote annotations")
			return ctrl.Result{}, err
		}
		log.Info("Fake Remote Skipped config updated")
	} else {
		log.Info("Fake Remote Skipped config unchanged")
	}
	return ctrl.Result{}, nil
}

func (r *FakeRemoteReconciler) fail(ctx context.Context, fakeRemote *testopv1.FakeRemote, err error) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	meta.SetStatusCondition(&fakeRemote.Status.Conditions, metav1.Condition{
		Type:    typeFailedFakeRemote,
		Status:  metav1.ConditionFalse,
		Reason:  "ReconcileFailed",
		Message: err.Error(),
	})
	if err = r.Status().Update(ctx, fakeRemote); err != nil {
		log.Error(err, "Failed to update Fake Remote status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

func setLastAppliedConfig(fakeRemote *testopv1.FakeRemote) error {
	return setAnnotationConfig(fakeRemote, lastAppliedConfigAnnotation)
}

func getLastAppliedConfig(fakeRemote *testopv1.FakeRemote) (*testopv1.FakeRemoteSpec, error) {
	return getLastConfig(fakeRemote, lastAppliedConfigAnnotation)
}

func getLastSkippedConfig(fakeRemote *testopv1.FakeRemote) (*testopv1.FakeRemoteSpec, error) {
	return getLastConfig(fakeRemote, lastSkippedConfigAnnotation)
}

func getLastConfig(fakeRemote *testopv1.FakeRemote, annotation string) (*testopv1.FakeRemoteSpec, error) {
	previousSpecRaw := fakeRemote.GetAnnotations()[annotation]
	if previousSpecRaw == "" {
		return nil, nil
	}
	fakeRemoteSpec := testopv1.FakeRemoteSpec{}
	err := json.Unmarshal([]byte(previousSpecRaw), &fakeRemoteSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal annotation %q as JSON: %v", annotation, err)
	}
	return &fakeRemoteSpec, nil
}

func hasPolicySkip(fakeRemote *testopv1.FakeRemote) bool {
	return fakeRemote.GetAnnotations()[resourcePolicyAnnotation] == reconciliationPolicySkip
}

func setAnnotationConfig(fakeRemote *testopv1.FakeRemote, annotation string) error {
	previousSpecRaw, err := computeConfig(&fakeRemote.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal annotation %q: %v", annotation, err)
	}
	fakeRemote.GetAnnotations()[annotation] = string(previousSpecRaw)
	return nil
}

func computeConfig(fakeRemoteSpec *testopv1.FakeRemoteSpec) (string, error) {
	raw, err := json.Marshal(fakeRemoteSpec)
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec to JSON: %v", err)
	}
	return string(raw), nil
}

func areDependentsDisabled(specDependents []string, lastSkippedConfig *testopv1.FakeRemoteSpec) bool {
	return len(specDependents) == 0 && lastSkippedConfig != nil && len(lastSkippedConfig.Dependents) == 0
}
