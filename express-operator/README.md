# express-operator
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## 0 前提バージョン & 作業ディレクトリ

| ツール | バージョン |
| --- | --- |
| Ubuntu 22.04 (WSL/VM) | - |
| Docker | 24.0+ |
| kind | v0.23.0 (= K8s 1.29) |
| kubectl | v1.29.x |
| Go | 1.22.x |
| operator‑sdk | v1.39.2 |
| AWS CLI | v2 |

# 環境クリーンアップ（やり直す場合）

```bash
# kindクラスターの削除
kind delete cluster --name express-operator

# 作業ディレクトリのクリーンアップ（必要な場合）
cd ~/dev
rm -rf k8s-kind-operator-express
```

# kindクラスターの作成

まず、クラスター設定ファイルを作成します：

`kind-config.yaml`
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30080  # NodePortで公開するポート
        hostPort: 8000       # ホストマシンのポート
```

```bash
# 作業ディレクトリの作成
mkdir -p ~/dev/k8s-kind-operator-express && cd $_

# kindクラスターの作成
kind create cluster --name express-operator --config kind-config.yaml

# クラスター情報の確認
kubectl cluster-info --context kind-express-operator
```

---

## 1 Operator SDK のインストール

```bash
# Operator SDKのダウンロードとインストール
curl -LO https://github.com/operator-framework/operator-sdk/releases/download/v1.39.2/operator-sdk_linux_amd64
chmod +x operator-sdk_linux_amd64
sudo mv operator-sdk_linux_amd64 /usr/local/bin/operator-sdk
operator-sdk version   # バージョン確認
```

---

## 2 プロジェクト雛形を生成

```bash
# モジュール名の設定とプロジェクト作成
export MODULE=github.com/kurosawa-kuro/express-operator
mkdir express-operator && cd $_

operator-sdk init \
  --domain kurosawa.dev \
  --repo $MODULE \
  --plugins go/v4
```

---

## 3 API & Controller をスキャフォールド

```bash
# APIとControllerの生成
operator-sdk create api \
  --group web \
  --version v1alpha1 \
  --kind ExpressApp \
  --resource --controller
```

### 3‑1 CRD スキーマ編集

`api/v1alpha1/expressapp_types.go`

```go
type ExpressAppSpec struct {
    Image       string `json:"image"`                // 必須
    Replicas    *int32 `json:"replicas,omitempty"`  // +kubebuilder:default:=1
    Port        int32  `json:"port,omitempty"`      // +kubebuilder:default:=8000
    MetricsPath string `json:"metricsPath,omitempty"` // +kubebuilder:default:="/metrics"
}
```

```bash
# マニフェストの生成
make generate manifests
```

### 3‑2 Reconciler本体（最小形）

`controllers/expressapp_controller.go`  
※自動生成された不要な関数をすべて削除し、以下だけ残します。

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

func (r *ExpressAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&webv1alpha1.ExpressApp{}).
        Owns(&appsv1.Deployment{}).
        Owns(&corev1.Service{}).
        Complete(r)
}
```

### 3‑3 helpers.go を新規作成

`controllers/helpers.go`

```go
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
                    ImagePullSecrets: []corev1.LocalObjectReference{
                        {
                            Name: "ecr-secret",
                        },
                    },
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
    _, err := controllerutil.CreateOrUpdate(ctx, c, obj, func() error {
        return nil
    })
    return err
}
```

---

## 4 Operator のビルドとデプロイ

```bash
# ECRのイメージ設定
IMG=986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/express-operator:v0.1.0

# ECRへのログイン（ローカルマシン用）
aws ecr get-login-password --region ap-northeast-1 | docker login --username AWS --password-stdin 986154984217.dkr.ecr.ap-northeast-1.amazonaws.com

# ビルドとデプロイ
make docker-build docker-push IMG=$IMG
make deploy IMG=$IMG

# Podの状態確認
kubectl -n express-operator-system get pods

# ECRシークレットの作成（express-operator-system名前空間用）
kubectl create secret docker-registry ecr-secret \
  --docker-server=986154984217.dkr.ecr.ap-northeast-1.amazonaws.com \
  --docker-username=AWS \
  --docker-password=$(aws ecr get-login-password --region ap-northeast-1) \
  -n express-operator-system

# デプロイメントにイメージプルシークレットを追加
kubectl patch deployment express-operator-controller-manager -n express-operator-system \
  -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"ecr-secret"}]}}}}'

# Podの状態を再確認
kubectl -n express-operator-system get pods
```

---

## 5 Express APIイメージの準備

Express APIイメージ（`container-nodejs-api-8000:v1.0.4`）は以下の要件を満たす必要があります：

- `/metrics` エンドポイントの実装
- `prom-client`パッケージの導入（Node.js用Prometheusクライアント）
- ECRへのプッシュ済み

**重要**: Express APIイメージも同様にECR認証が必要です。以下の手順で`default`名前空間にもシークレットを作成してください：

```bash
# default名前空間にECRのシークレットを作成
kubectl create secret docker-registry ecr-secret \
  --docker-server=986154984217.dkr.ecr.ap-northeast-1.amazonaws.com \
  --docker-username=AWS \
  --docker-password=$(aws ecr get-login-password --region ap-northeast-1)
```

---

## 6 ExpressAppカスタムリソースのデプロイ

`config/samples/web_v1alpha1_expressapp.yaml`

```yaml
apiVersion: web.kurosawa.dev/v1alpha1
kind: ExpressApp
metadata:
  name: sample-api
  namespace: default
spec:
  image: 986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/container-nodejs-api-8000:v1.0.4
  replicas: 1
  port: 8000
  metricsPath: /metrics
```

```bash
# カスタムリソースのデプロイと確認
kubectl apply -f config/samples/web_v1alpha1_expressapp.yaml
kubectl get deployment,svc

# アプリケーションの動作確認
kubectl port-forward svc/sample-api 8080:8000 &
curl http://localhost:8080/healthz   # 期待値: {"status":"ok","timestamp":"..."}
curl http://localhost:8080/metrics   # Prometheusメトリクスの確認
```

---

## 注意事項

1. ECRへのアクセス権限が必要です
2. イメージのプル時に認証エラーが発生した場合は、以下のコマンドでECRにログインしてください：
```bash
aws ecr get-login-password --region ap-northeast-1 | docker login --username AWS --password-stdin 986154984217.dkr.ecr.ap-northeast-1.amazonaws.com
```

3. Kubernetesクラスタ内でECRイメージをプルする場合は、適切なイメージプルシークレットの設定が必要です：
```bash
kubectl create secret docker-registry ecr-secret \
  --docker-server=986154984217.dkr.ecr.ap-northeast-1.amazonaws.com \
  --docker-username=AWS \
  --docker-password=$(aws ecr get-login-password --region ap-northeast-1)

kubectl patch deployment sample-api -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"ecr-secret"}]}}}}'
```

4. トラブルシューティング
   - Podが起動しない場合は、`kubectl describe pod <pod-name>`で詳細を確認
   - Operatorのログは`kubectl logs -n express-operator-system -l control-plane=controller-manager -c manager`で確認
   - イメージプルエラーは`kubectl get events --sort-by='.lastTimestamp'`で確認

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/express-operator:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/express-operator/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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

