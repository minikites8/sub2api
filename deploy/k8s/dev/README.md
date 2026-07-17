# Sub2API Quota Lease Demo on Kubernetes

这个目录用于 `dev` namespace 的测试环境部署，包含控制面独立 Postgres/Redis、3 个节点池各自独立 Postgres/Redis、Service 和示例 Ingress。

## 镜像

默认镜像：

```powershell
ghcr.io/minikites8/sub2api:test-codex-quota-lease-demo
```

当前分支提交对应的固定测试镜像：

```powershell
ghcr.io/minikites8/sub2api:test-codex-quota-lease-demo-a415251
```

固定镜像部署：

```powershell
kubectl -n dev set image deploy/sub2api-control sub2api=ghcr.io/minikites8/sub2api:test-codex-quota-lease-demo-a415251
kubectl -n dev set image statefulset/sub2api-node-us sub2api=ghcr.io/minikites8/sub2api:test-codex-quota-lease-demo-a415251
kubectl -n dev set image statefulset/sub2api-node-sg sub2api=ghcr.io/minikites8/sub2api:test-codex-quota-lease-demo-a415251
kubectl -n dev set image statefulset/sub2api-node-jp sub2api=ghcr.io/minikites8/sub2api:test-codex-quota-lease-demo-a415251
```

## 部署

```powershell
kubectl apply -f deploy/k8s/dev/quota-lease-demo.yaml
kubectl -n dev rollout status statefulset/sub2api-control-postgres
kubectl -n dev rollout status statefulset/sub2api-control-redis
kubectl -n dev rollout status statefulset/sub2api-node-us-postgres
kubectl -n dev rollout status statefulset/sub2api-node-us-redis
kubectl -n dev rollout status statefulset/sub2api-node-sg-postgres
kubectl -n dev rollout status statefulset/sub2api-node-sg-redis
kubectl -n dev rollout status statefulset/sub2api-node-jp-postgres
kubectl -n dev rollout status statefulset/sub2api-node-jp-redis
kubectl -n dev rollout status deploy/sub2api-control
kubectl -n dev rollout status statefulset/sub2api-node-us
kubectl -n dev rollout status statefulset/sub2api-node-sg
kubectl -n dev rollout status statefulset/sub2api-node-jp
kubectl -n dev get pods,svc,ingress
```

## 入口

示例 Ingress：

- `sub2api-control.dev.example.com`：控制面后台和节点租约管理接口
- `sub2api-api.dev.example.com`：用户侧 `/v1` API，请把 EO 或测试负载均衡指到这个入口

把 `quota-lease-demo.yaml` 里的 `spec.ingressClassName` 和两个 `host` 改成测试集群实际值。

## 节点扩容

```powershell
kubectl -n dev scale statefulset/sub2api-node-us --replicas=2
kubectl -n dev scale statefulset/sub2api-node-sg --replicas=2
kubectl -n dev scale statefulset/sub2api-node-jp --replicas=2
```

节点 ID 使用 Pod 名称，例如 `sub2api-node-us-0`、`sub2api-node-sg-0`、`sub2api-node-jp-0`。节点启动后会向控制面注册，控制面分配节点密钥，后续租约、流水、账号任务都走控制面接口。

## 节点池

- `sub2api-node-us`：美国测试节点池
- `sub2api-node-sg`：新加坡测试节点池
- `sub2api-node-jp`：日本测试节点池

统一入口 Service 是 `sub2api-node`，会选中所有 `app.kubernetes.io/component=gateway-node` 的节点 Pod。

节点池使用 `DEPLOYMENT_ROLE=node`，HTTP 层只开放健康检查和用户侧网关入口。控制面使用 `DEPLOYMENT_ROLE=control`，提供后台页面、节点注册、租约、流水和账号任务管理。

每个角色连接自己的存储服务：

- 控制面：`sub2api-control-postgres`、`sub2api-control-redis`
- US 节点池：`sub2api-node-us-postgres`、`sub2api-node-us-redis`
- SG 节点池：`sub2api-node-sg-postgres`、`sub2api-node-sg-redis`
- JP 节点池：`sub2api-node-jp-postgres`、`sub2api-node-jp-redis`

## 配置点

默认测试管理员：

```text
admin@sub2api.local
sub2api-dev-admin-password
```

关键 Secret 位于 `sub2api-dev-secrets`：

- `CONTROL_DATABASE_PASSWORD`
- `CONTROL_REDIS_PASSWORD`
- `NODE_US_DATABASE_PASSWORD`
- `NODE_US_REDIS_PASSWORD`
- `NODE_SG_DATABASE_PASSWORD`
- `NODE_SG_REDIS_PASSWORD`
- `NODE_JP_DATABASE_PASSWORD`
- `NODE_JP_REDIS_PASSWORD`
- `JWT_SECRET`
- `TOTP_ENCRYPTION_KEY`
- `ADMIN_PASSWORD`
- `QUOTA_LEASE_CONTROL_KEY`

控制面副本保持 `1`。当前 quota lease demo 的租约状态在控制面进程内存中，多副本会分散状态。
