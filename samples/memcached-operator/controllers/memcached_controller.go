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

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8scontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/multiphase"
	cachecrd "github.com/disaster37/operator-sdk-extra/v2/samples/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
)

const (
	mainFinalizer shared.FinalizerName = "memcached.cache.example.com/finalizer"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	controller.Controller
	multiphase.MultiPhaseReconciler[*cachecrd.Memcached]
	multiphase.MultiPhaseReconcilerAction[*cachecrd.Memcached]
	name            string
	stepReconcilers []multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, client.Object]
}

func NewMemcachedReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.Controller {
	configMapStep := newConfigMapReconciler(c, recorder)
	deploymentStep := newDeploymentReconciler(c, recorder)

	return &MemcachedReconciler{
		Controller: controller.NewController(),
		MultiPhaseReconciler: multiphase.NewMultiPhaseReconciler[*cachecrd.Memcached](
			c,
			"memcached",
			mainFinalizer,
			logger,
			recorder,
		),
		MultiPhaseReconcilerAction: multiphase.NewMultiPhaseReconcilerAction[*cachecrd.Memcached](
			c,
			controller.ReadyCondition,
			recorder,
		),
		name: "memcached",
		stepReconcilers: []multiphase.MultiPhaseStepReconcilerAction[*cachecrd.Memcached, client.Object]{
			multiphase.NewObjectMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap, client.Object](configMapStep),
			multiphase.NewObjectMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *appv1.Deployment, client.Object](deploymentStep),
		},
	}
}

//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/finalizers,verbs=update
//+kubebuilder:rbac:groups="core",resources=events,verbs=patch;get;create
//+kubebuilder:rbac:groups="core",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

func (h *MemcachedReconciler) Client() client.Client {
	return h.MultiPhaseReconcilerAction.Client()
}

func (h *MemcachedReconciler) Recorder() record.EventRecorder {
	return h.MultiPhaseReconcilerAction.Recorder()
}

func (r *MemcachedReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &cachecrd.Memcached{}
	data := map[string]any{}

	return r.MultiPhaseReconciler.Reconcile(ctx, req, o, data, r, r.stepReconcilers...)
}

func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachecrd.Memcached{}).
		Owns(&appv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(k8scontroller.Options{
			RateLimiter: controller.DefaultControllerRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}
