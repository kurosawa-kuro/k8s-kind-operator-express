## 0 前提バージョンとディレクトリ

| ツール | バージョン例 |
| --- | --- |
| Ubuntu 22.04 (WSL/VM) |
| Docker 24.0+ |
| kind v0.23.0 (Kubernetes 1.29) |
| kubectl v1.29.x |
| Go 1.22.x |
| operator‑sdk **v1.39.2** citeturn1search0 |
| AWS CLI v2（ECR 認証用） |

```bash
mkdir -p ~/dev/k8s-kind-operator-express && cd $_
```

---

## 1 Operator SDK のインストール

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

## 3 API & Controller スケルトン

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
    Image string `json:"image"`              // 必須
    // +kubebuilder:default:=1
    Replicas *int32 `json:"replicas,omitempty"`
    // +kubebuilder:default:=8000
    Port int32 `json:"port,omitempty"`
    // +kubebuilder:default:="/metrics"
    MetricsPath string `json:"metricsPath,omitempty"`
}
```

```bash
make generate manifests
```

### 3‑2 Reconciler 実装（最小）

`controllers/expressapp_controller.go` ※不要箇所は削除済み

```go
func (r *ExpressAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var app webv1alpha1.ExpressApp
    if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    deploy := deploymentFor(&app)
    svc    := serviceFor(&app)

    if err := ctrl.SetControllerReference(&app, deploy, r.Scheme); err != nil {
        return ctrl.Result{}, err
    }
    if err := ctrl.SetControllerReference(&app, svc, r.Scheme); err != nil {
        return ctrl.Result{}, err
    }

    if err := reconcilerApply(ctx, r.Client, deploy); err != nil {
        return ctrl.Result{}, err
    }
    if err := reconcilerApply(ctx, r.Client, svc); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

> `deploymentFor`, `serviceFor`, `reconcilerApply` は公式 memcached‑operator サンプルをコピーして OK。

---

## 4 Operator をビルド & kind にデプロイ

```bash
IMG=986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/express-operator:v0.1.0

make docker-build docker-push IMG=$IMG
make deploy IMG=$IMG
kubectl -n express-operator-system get pods   # → Running
```

---

## 5 Express API イメージの準備

**API ソース変更が必要な場合のみ** `/metrics` ルートを追加して再ビルドします。  

```bash
npm i prom-client express-prom-bundle        # 追加ライブラリ  citeturn2search0
```

`src/index.ts`（抜粋）

```ts
import promBundle from 'express-prom-bundle';
app.use(promBundle({ promClient: { collectDefaultMetrics: {} } }));
```

```bash
TAG=v1.0.5
docker build -t container-nodejs-api-8000:$TAG .
docker tag  container-nodejs-api-8000:$TAG \
  986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/container-nodejs-api-8000:$TAG
docker push 986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/container-nodejs-api-8000:$TAG
```

---

## 6 `ExpressApp` CR を apply

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
curl http://localhost:8080/metrics   # → Prometheus 形式のメトリクス
```

---

### 完了
