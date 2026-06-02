/*
Copyright 2022.

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

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller/remote"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/samples/elasticsearch-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	roleFinalizer shared.FinalizerName = "role.elasticsearchapi.k8s.webcenter.fr/finalizer"
	roleName      string               = "role"
)

type RoleReconciler struct {
	controller.Controller
	remote.RemoteReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	reconcilerAction remote.RemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler]
	name             string
}

func NewRoleReconciler(c client.Client, logger *logrus.Entry, recorder record.EventRecorder) controller.Controller {
	return &RoleReconciler{
		Controller: controller.NewController(),
		RemoteReconciler: remote.NewRemoteReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
			c,
			roleName,
			roleFinalizer,
			logger,
			recorder,
		),
		reconcilerAction: newRoleReconciler(c, recorder),
		name:             roleName,
	}
}

//+kubebuilder:rbac:groups=elasticsearchapi.k8s.webcenter.fr,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=elasticsearchapi.k8s.webcenter.fr,resources=roles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=elasticsearchapi.k8s.webcenter.fr,resources=roles/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=patch;get;create
//+kubebuilder:rbac:groups="elasticsearch.k8s.webcenter.fr",resources=elasticsearches,verbs=get

func (r *RoleReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := &elasticsearchapicrd.Role{}
	data := map[string]any{}
	return r.RemoteReconciler.Reconcile(ctx, req, o, data, r.reconcilerAction)
}

func (r *RoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elasticsearchapicrd.Role{}).
		Complete(r)
}
