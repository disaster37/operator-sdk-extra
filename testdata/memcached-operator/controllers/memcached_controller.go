/*
Copyright 2023.

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

package controllers

import (
	"context"

	"emperror.dev/errors"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	cachev1alpha1 "github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
)

const (
	MemcachedCondition controller.ConditionName = "MemcachedReady"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	controller.BasicMultiPhaseReconciler
}

func NewMemcachedReconciler(client client.Client, logger *logrus.Entry, recorder record.EventRecorder) (multiPhaseReconciler controller.MultiPhaseReconciler, err error) {
	return controller.NewBasicMultiPhaseReconciler(
		client,
		"memcached",
		"memcached.cache.example.com/finalizer",
		MemcachedCondition,
		logger,
		recorder,
	)
}

//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/finalizers,verbs=update
//+kubebuilder:rbac:groups="core",resources=events,verbs=patch;get;create
//+kubebuilder:rbac:groups="core",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Memcached object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	mc := &v1alpha1.Memcached{}
	data := map[string]any{}

	configMapReconciler, err := NewConfigMapReconciler(
		r.Client,
		r.GetLogger(),
		r.GetRecorder(),
		r.Scheme(),
	)
	if err != nil {
		return res, errors.Wrap(err, "Error when create configMap reconciler")
	}

	deploymentReconciler, err := NewDeploymentReconciler(
		r.Client,
		r.GetLogger(),
		r.GetRecorder(),
		r.Scheme(),
	)

	return r.BasicMultiPhaseReconciler.Reconcile(
		ctx,
		req,
		mc,
		data,
		configMapReconciler,
		deploymentReconciler,
	)

}

// client client.Client, logger *logrus.Entry, recorder record.EventRecorder, scheme *runtime.Scheme, ignoresDiff ...patch.CalculateOption

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Memcached{}).
		Owns(&appv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}