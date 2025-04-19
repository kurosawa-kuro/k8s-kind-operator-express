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
```

### 3‑3 helpers.go を新規作成

`controllers/helpers.go`の内容は変更なし（既存の内容を維持）

---

## 4 Operator のビルドとデプロイ

```bash
# ECRのイメージ設定
IMG=986154984217.dkr.ecr.ap-northeast-1.amazonaws.com/express-operator:v0.1.0

# ビルドとデプロイ
make docker-build docker-push IMG=$IMG
make deploy IMG=$IMG

# Podの状態確認
kubectl -n express-operator-system get pods
```

---

## 5 Express APIイメージの準備

Express APIイメージ（`container-nodejs-api-8000:v1.0.4`）は以下の要件を満たす必要があります：

- `/metrics` エンドポイントの実装
- `prom-client`パッケージの導入（Node.js用Prometheusクライアント）
- ECRへのプッシュ済み

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
```

---

## 7 動作確認

```bash
# ポートフォワーディングの設定
kubectl port-forward svc/sample-api 8080:8000 &

# エンドポイントの確認
curl http://localhost:8080/healthz   # 期待値: "ok"
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
