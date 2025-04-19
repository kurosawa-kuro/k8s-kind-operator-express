以下は **曖昧さ・任意ステップを完全排除**した最新版チュートリアルです。  
**helpers.go へコピペするコード**まで全行示してあるので、この手順どおり写経すれば **`ExpressApp` CR 1 つで Node.js/Express イメージを Deployment+Service まで自動生成**できます。

---

## 0 前提バージョン & 作業ディレクトリ

| ツール | バージョン例 |
| --- | --- |
| Ubuntu 22.04 (WSL/VM) |
| Docker 24.0+ |
| kind v0.23.0 (= K8s 1.29) |
| kubectl v1.29.x |
| Go 1.22.x |
| operator‑sdk **v1.39.2** citeturn0search0 |
| AWS CLI v2（ECR 認証用） |

```bash
mkdir -p ~/dev/k8s-kind-operator-express && cd $_
```

---

## 1 Operator SDK をインストール

```bash
curl -LO https://github.com/operator-framework/operator-sdk/releases/download/v1.39.2/operator-sdk_linux_amd64
chmod +x operator-sdk_linux_amd64
sudo mv operator-sdk_linux_amd64 /usr/local/bin/operator-sdk
operator-sdk version   # v1.39.2
```

---

## 2 プロジェクト雛形を生成

```bash
export MODULE=github.com/kurosawa-kuro/express-operator
mkdir express-operator && cd $_

operator-sdk init \
  --domain kurosawa.dev \
  --repo    $MODULE \
  --plugins go/v4
```

---

## 3 API & Controller をスキャフォールド

```bash
operator-sdk create api \
  --group web \
  --version v1alpha1 \
  --kind ExpressApp \
  --resource --controller
```

### 3‑1 CRD スキーマ編集

`api/v1alpha1/expressapp_types.go`

```go
type ExpressAppSpec struct {
    Image       string `json:"image"`               // 必須
    Replicas    *int32 `json:"replicas,omitempty"`  // +kubebuilder:default:=1
    Port        int32  `json:"port,omitempty"`      // +kubebuilder:default:=8000
    MetricsPath string `json:"metricsPath,omitempty"` // +kubebuilder:default:="/metrics"
}
```

```bash
make generate manifests
```

### 3‑2 Reconciler本体（最小形）

`controllers/expressapp_controller.go`  
※自動生成された不要な関数をすべて削除し、以下だけ残す。

```go
func (r *ExpressAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var app webv1alpha1.ExpressApp
    if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    deploy := deploymentFor(&app)
    svc    := serviceFor(&app)

    _ = ctrl.SetControllerReference(&app, deploy, r.Scheme)
    _ = ctrl.SetControllerReference(&app, svc,    r.Scheme)

    if err := reconcilerApply(ctx, r.Client, deploy); err != nil {
        return ctrl.Result{}, err
    }
    if err := reconcilerApply(ctx, r.Client, svc); err != nil {
        return ctrl.Result{}, err
    }
    return ctrl.Result{}, nil
}
```

### 3‑3 **helpers.go を新規追加（コピペするだけ）**

`controllers/helpers.go`

```go
package controllers

import (
	"context"
	"fmt"

	cachev1alpha1 "github.com/kurosawa-kuro/express-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

/*--------------------- 共有ラベル ---------------------*/
func labelsForExpress(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "express-api",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "express-operator",
	}
}

/*--------------------- Deployment --------------------*/
func deploymentFor(app *cachev1alpha1.ExpressApp) *appsv1.Deployment {
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

/*--------------------- Service -----------------------*/
func serviceFor(app *cachev1alpha1.ExpressApp) *corev1.Service {
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

/*----------------- CreateOrUpdate 共通関数 -------------*/
func reconcilerApply(ctx context.Context, c client.Client, obj client.Object) error {
	_, err := controllerutil.CreateOrUpdate(ctx, c, obj, func() error { return nil })
	return err
}
```

この 1 ファイルを追加するだけで、Deployment と Service を自動生成できます。

---

## 4 Operator をビルド & kind へデプロイ

```bash
IMG=986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/express-operator:v0.1.0

make docker-build docker-push IMG=$IMG
make deploy IMG=$IMG
kubectl -n express-operator-system get pods   # → Running
```

---

## 5 Express API イメージを確認

- 既に `/metrics` を実装し、`prom-client` を依存に含む  
  イメージ `container-nodejs-api-8000:v1.0.5` が **ECR に存在する前提** です。  
  （`prom-client` は Node.js 公式の Prometheus クライアント） citeturn1search0  
- **実装済みならビルドや npm install は一切不要。**

---

## 6 `ExpressApp` カスタムリソースを apply

`config/samples/web_v1alpha1_expressapp.yaml`

```yaml
apiVersion: web.kurosawa.dev/v1alpha1
kind: ExpressApp
metadata:
  name: sample-api
  namespace: default
spec:
  image: 986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/container-nodejs-api-8000:v1.0.5
  replicas: 1
  port: 8000
  metricsPath: /metrics
```

```bash
kubectl apply -f config/samples/web_v1alpha1_expressapp.yaml
kubectl get deployment,svc
```

---

## 7 動作確認

```bash
kubectl port-forward svc/sample-api 8080:8000 &
curl http://localhost:8080/healthz   # → ok
curl http://localhost:8080/metrics   # → Prometheus 形式
```

---

### 完了

- **helpers.go を 1 枚追加** → Deployment & Service 自動生成  
- `/metrics` 実装済みイメージを指定すれば **監視もすぐ有効化**  
- CR の値を変えるだけでローリングアップデート・スケール変更が可能  

この手順で「余計なコードゼロ、手戻りゼロ」の Operator チュートリアルが完成です。
