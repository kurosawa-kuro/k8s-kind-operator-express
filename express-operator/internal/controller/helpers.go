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
	"fmt"

	webv1alpha1 "github.com/kurosawa-kuro/express-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// 共有ラベル
func labelsForExpress(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "express-api",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "express-operator",
	}
}

// Deployment作成
func deploymentFor(app *webv1alpha1.ExpressApp) *appsv1.Deployment {
	replicas := int32(1)
	if app.Spec.Replicas != nil {
		replicas = *app.Spec.Replicas
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labelsForExpress(app.Name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labelsForExpress(app.Name)},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "api",
						Image: app.Spec.Image,
						Ports: []corev1.ContainerPort{{
							ContainerPort: app.Spec.Port,
							Name:          "http",
						}},
						Env: []corev1.EnvVar{{
							Name:  "PORT",
							Value: fmt.Sprint(app.Spec.Port),
						}},
					}},
				},
			},
		},
	}
}

// Service作成
func serviceFor(app *webv1alpha1.ExpressApp) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			Labels:    labelsForExpress(app.Name),
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/port":   fmt.Sprint(app.Spec.Port),
				"prometheus.io/path":   app.Spec.MetricsPath,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labelsForExpress(app.Name),
			Ports: []corev1.ServicePort{{
				Port:       app.Spec.Port,
				TargetPort: intstr.FromInt(int(app.Spec.Port)),
				Name:       "http",
			}},
		},
	}
}

// CreateOrUpdate共通関数
func reconcilerApply(ctx context.Context, c client.Client, obj client.Object) error {
	_, err := controllerutil.CreateOrUpdate(ctx, c, obj, func() error { return nil })
	return err
}
