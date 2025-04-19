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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webv1alpha1 "github.com/kurosawa-kuro/express-operator/api/v1alpha1"
)

// ExpressAppReconciler reconciles a ExpressApp object
type ExpressAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=web.kurosawa.dev,resources=expressapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=web.kurosawa.dev,resources=expressapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=web.kurosawa.dev,resources=expressapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ExpressApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ExpressAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var app webv1alpha1.ExpressApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	deploy := deploymentFor(&app)
	svc := serviceFor(&app)

	_ = ctrl.SetControllerReference(&app, deploy, r.Scheme)
	_ = ctrl.SetControllerReference(&app, svc, r.Scheme)

	if err := reconcilerApply(ctx, r.Client, deploy); err != nil {
		return ctrl.Result{}, err
	}
	if err := reconcilerApply(ctx, r.Client, svc); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// SetupWithManager sets up the controller with the Manager.
func (r *ExpressAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webv1alpha1.ExpressApp{}).
		Owns(&appsv1.Deployment{}). // Add this
		Owns(&corev1.Service{}).    // Add this
		Complete(r)
}
