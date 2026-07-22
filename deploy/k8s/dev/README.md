# Sub2API Quota Lease on Kubernetes

这个目录用于 `dev` namespace 的测试环境部署，包含 1 个控制面、1 个调用节点、独立 Postgres/Redis、Service 和示例 Ingress。

## 镜像

默认测试镜像：

```powershell
ghcr.io/minikites8/sub2api:test-codex-quota-lease-demo
```

固定版本部署时，把 `<tag>` 替换成 GitHub Actions 打出的测试镜像标签：

```powershell
kubectl -n dev set image deploy/sub2api-control sub2api=ghcr.io/minikites8/sub2api:<tag>
kubectl -n dev set image statefulset/sub2api-node-us sub2api=ghcr.io/minikites8/sub2api:<tag>
```

## 部署

```powershell
kubectl apply -f deploy/k8s/dev/quota-lease.yaml
kubectl -n dev rollout status statefulset/sub2api-control-postgres
kubectl -n dev rollout status statefulset/sub2api-control-redis
kubectl -n dev rollout status statefulset/sub2api-node-us-postgres
kubectl -n dev rollout status statefulset/sub2api-node-us-redis
kubectl -n dev rollout status deploy/sub2api-control
kubectl -n dev rollout status statefulset/sub2api-node-us
kubectl -n dev get pods,svc,ingress
```

## 入口

示例 Ingress：

- `sub2api-control.dev.example.com`：控制面后台、节点注册、租约管理和诊断接口
- `sub2api-api.dev.example.com`：用户侧 `/v1` API，EO 或测试负载均衡指向这个入口

把 `quota-lease.yaml` 里的 `spec.ingressClassName` 和两个 `host` 改成测试集群实际值。

## 节点注册

控制面后台生成节点注册链接后，把完整 URL 写入 Secret `sub2api-dev-secrets` 的 `NODE_REGISTRATION_URL`。当前正式注册路径格式：

```text
https://sub2api-control.dev.example.com/api/v1/node-leases/nodes/register?registration_token=replace-me
```

节点启动后会向控制面注册，控制面分配并保存节点密钥，后续租约、流水、账号同步、错误日志、用量探测任务都通过控制面接口通讯。

## 节点扩容

```powershell
kubectl -n dev scale statefulset/sub2api-node-us --replicas=2
```

节点 ID 使用 Pod 名称，例如 `sub2api-node-us-0`、`sub2api-node-us-1`。统一入口 Service 是 `sub2api-node`，会选中所有 `app.kubernetes.io/component=gateway-node` 的节点 Pod。

## 存储

控制面和节点使用各自独立的 Postgres/Redis：

- 控制面：`sub2api-control-postgres`、`sub2api-control-redis`
- 节点：`sub2api-node-us-postgres`、`sub2api-node-us-redis`

控制面持久化节点、租约、流水、待上传事件和镜像快照。节点持久化本地镜像、账号数据和运行缓存。

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
- `JWT_SECRET`
- `TOTP_ENCRYPTION_KEY`
- `ADMIN_PASSWORD`
- `QUOTA_LEASE_CONTROL_KEY`
- `NODE_REGISTRATION_URL`

租约配置使用正式环境变量前缀 `GATEWAY_QUOTA_LEASE_*`。旧前缀 `GATEWAY_QUOTA_LEASE_DEMO_*` 可继续读取，用于已有测试环境平滑升级。

节点访问控制面的心跳、镜像同步、流水上传共用 `GATEWAY_QUOTA_LEASE_REMOTE_TIMEOUT_SECONDS`，默认 `15` 秒。

控制面副本当前保持 `1`。多副本控制面需要引入 DB 锁或主节点选举后再开启。
