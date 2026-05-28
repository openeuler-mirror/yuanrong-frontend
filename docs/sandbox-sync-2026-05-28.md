# feature/sandbox 同步到 master 留档

## 背景

本次目标是将 `feature/sandbox` 分支中自上次同步后仍有效的 sandbox/frontend 相关能力同步到 `master`。

已知历史信息：

- `master` 上已存在上一次 sandbox 相关同步点：`8410f2d0fa6e68ed16511565e8e12dbded14cc6a`。
- `feature/sandbox` HEAD 为 `bb40347`。
- `master` 当前同步前 HEAD 为 `6ae388b`。
- `feature/sandbox` 与 `master` 的 merge-base 很早，是 `ad754f1`，直接 merge 会把大量旧历史重新带入并造成大量冲突。
- `8410f2d` 在 `master` 上，不在 `feature/sandbox` 上，因此不能简单使用普通 merge-base 作为增量边界。

## 同步策略

本次采用“最终文件净增量”方案，而不是直接 merge 或逐 commit cherry-pick：

```bash
git diff --binary --find-renames 8410f2d..feature/sandbox | git apply --3way
```
选择该方案的原因：

- `feature/sandbox` 历史与 `master` 分叉较早，直接 merge 会出现大量无关冲突。
- `65a875f` 的最终效果已不再存在于 `feature/sandbox` HEAD 中，不需要按 commit 同步。
- `d2070b0c` 的有效内容仍然存在，最终净增量方案会自然包含它的结果，例如 `FunctionProxyID` / `RouteAddress` 相关路由能力。
- 按 `8410f2d..feature/sandbox` 的最终文件差异同步，可以避免重复引入已 squash 到 `master` 的旧改动。

## 主要能力变更

### sandbox API

新增 sandbox 管理 API，主要文件：

- `pkg/frontend/api/sandbox/handler.go`
- `pkg/frontend/api/sandbox/handler_test.go`
- `pkg/frontend/api/api.go`

能力包括：

- sandbox 创建
- sandbox 删除
- sandbox raw runtime 调用透传
- 与 frontend 现有 API 注册体系集成

### posix websocket / webterm

新增 posix websocket 通道与 webterm 行为增强，主要文件：

- `pkg/frontend/posixws/handler.go`
- `pkg/frontend/posixws/message.go`
- `pkg/frontend/posixws/processor.go`
- `pkg/frontend/webui/webterm.go`
- `pkg/frontend/webui/webterm_test.go`
- `pkg/common/faas_common/protobuf/exec_service.proto`

关键点：

- 新增 posix websocket 消息模型和 processor。
- webterm 通过 raw runtime 调用与实例交互。
- `exec_service.proto` 新增 `ExecStdinEof`，用于 cp / stdin 流式交互中的 EOF 表达。

### runtime raw 调用的 RawRequestOption

当前 runtime API 中 raw 调用签名要求携带 `api.RawRequestOption`：

- `CreateInstanceRaw([]byte, api.RawRequestOption)`
- `InvokeByInstanceIdRaw([]byte, api.RawRequestOption)`
- `KillRaw([]byte, api.RawRequestOption)`

本次将 frontend 中所有 raw 调用统一升级：

- `pkg/frontend/common/util/client.go`
- `pkg/frontend/api/functionsystem/handler.go`
- `pkg/frontend/posixws/processor.go`
- 相关 fake client / mock / UT

`RawRequestOption` 当前主要用于传递 `TraceParent`，避免 sandbox/webterm 这类 raw 透传路径丢失 OpenTelemetry 链路上下文。

### FunctionProxyID / RouteAddress

同步了 sandbox 分支中仍有效的实例路由信息：

- `FunctionProxyID` 来源于 runtime allocation。
- frontend 内部转换为 `RouteAddress`。
- 最终写入 runtime invoke option 的 `CreateOpt["YR_ROUTE"]`。

主要涉及：

- `pkg/common/faas_common/types/lease.go`
- `pkg/frontend/common/util/client.go`
- `pkg/frontend/invocation/function_invoke_for_kernel.go`

### BypassDataSystem

同步了 `BypassDataSystem` 能力：

- 从请求头读取 bypass datasystem 语义。
- 传入 `api.InvokeOptions.BypassDataSystem`。
- 在获取 invoke 结果时避免不必要的数据系统引用处理。

### JWT / auth 调整

同步 sandbox 分支中的 JWT 相关 frontend 改动：

- `pkg/frontend/common/jwtauth/jwtauth.go`
- `pkg/frontend/common/jwtauth/jwtauth_test.go`
- `pkg/frontend/api/api_test.go`

包含 func token auth、永久 token 等相关逻辑和测试更新。

### state 并发安全

`pkg/frontend/state/state.go` 对 `frontendHandlerQueue` 增加锁保护：

- 初始化时加锁，避免重复初始化。
- `GetStateByte` / `DeleteStateByte` 读取 queue 时加读锁。
- queue 未初始化时显式返回错误。

## 冲突处理

初始应用最终净增量后，主要冲突文件包括：

- `pkg/common/faas_common/logger/interfacelogger_test.go`
- `pkg/common/faas_common/logger/log/logger_test.go`
- `pkg/frontend/common/util/client.go`
- `pkg/frontend/invocation/function_invoke_for_kernel.go`
- `pkg/frontend/invocation/function_invoke_for_kernel_test.go`
- `test/test.sh`

处理原则：

- 以 `master` 当前基础设施和测试脚本为准。
- 仅引入 `feature/sandbox` HEAD 的最终有效功能差异。
- 不恢复已经在 feature HEAD 中被反向修改或失效的中间 commit 效果。
- 保持当前 runtime API 可编译。

关键冲突处理：

1. `test/test.sh`
   - 保留 `master` 版本，避免回退当前测试入口行为。

2. `.gitee` PR 模板删除
   - 视为无关删除，已恢复，未纳入本次同步。

3. raw runtime 调用
   - sandbox 新增 raw 调用基于旧签名。
   - 当前 runtime API 需要 `api.RawRequestOption`。
   - 统一升级 interface、实现、mock 和调用点，并补齐 traceparent 传递。

4. `InvokeOptions.IsInterrupted`
   - sandbox 分支尝试写入 `api.InvokeOptions.IsInterrupted`。
   - 当前挂载验证的 runtime API 中不存在该字段。
   - 已去掉对 runtime option 的非法写入，但保留 frontend 内部 `IsInterrupted` 字段和语义。

5. `convert` 签名
   - sandbox 同步后 `convert` 新增 lease 信息参数。
   - 修复 `direct_invoke.go` 旧调用点。
   - 修复相关 gomonkey mock 签名。

6. frontendsdk 与当前 runtime API 字段不一致
   - 当前 runtime `common.Configuration` 不包含 `IamAddress` 和 `SystemAuthDataKey`。
   - 移除 `IamAddress` 写入。
   - `SystemAuthDataKey` 改为 frontend 内部包级变量保存，用于回填 frontend raw STS 配置。

7. fake libruntime
   - `FakeLibruntimeSdkClient.GetAsync/GetEvent` 原本空实现会导致部分 frontend UT 阻塞。
   - 调整为默认执行 callback，避免测试永久等待。

## 验证方式

用户指定验证环境：

- 镜像：`swr.cn-southwest-2.myhuaweicloud.com/yuanrong-dev/compile_arm:2.1`
- PATH：

```bash
PATH="/root/go/bin:/opt/buildtools/golang_go-1.24.1/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
```

验证时使用隔离目录，避免测试脚本修改工作区：

```bash
rm -rf /tmp/yuanrong-frontend-verify
mkdir -p /tmp/yuanrong-frontend-verify
rsync -a --delete --exclude='.git' ./ /tmp/yuanrong-frontend-verify/
```

容器运行时挂载 runtime API：

```bash
docker run --rm \
  -v /tmp/yuanrong-frontend-verify:/workspace \
  -v "$PWD/../yuanrong/api/go":/api/go:ro \
  -w /workspace \
  swr.cn-southwest-2.myhuaweicloud.com/yuanrong-dev/compile_arm:2.1 \
  bash -lc 'export GOPATH=/root/go; export PATH="/root/go/bin:/opt/buildtools/golang_go-1.24.1/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"; sh test/test.sh faasfrontend'
```

注意事项：

- 需要设置 `GOPATH=/root/go`，否则 `protoc-gen-go` 可能安装到容器默认 GOPATH，不在指定 PATH 中。
- `test/faasfrontend/test.sh` 会执行 `go mod tidy` 和修改 `frontendsdk` import，因此必须在 `/tmp/yuanrong-frontend-verify` 这类隔离副本中运行。
- 用户明确要求只关注 frontend 测试，不处理 common 或其他组件测试失败。

## 验证结果

完整运行 `sh test/test.sh faasfrontend` 后，全量 frontend 脚本仍因历史/环境敏感用例 exit 1，但本次同步直接相关包均已通过：

```text
ok frontend/pkg/frontend/api/functionsystem
ok frontend/pkg/frontend/api/sandbox
ok frontend/pkg/frontend/common/util
ok frontend/pkg/frontend/invocation
ok frontend/pkg/frontend/posixws
ok frontend/pkg/frontend/state
ok frontend/pkg/frontend/webui
```

这些包覆盖了本次核心同步路径：

- sandbox API
- raw function system handler
- raw option / traceparent 传递
- invocation convert / lease routing
- posix websocket
- webterm
- state queue 安全访问
- util runtime client raw 调用与结果获取

全量 frontend 中剩余失败主要集中在非本次同步核心路径，例如：

- `frontend/pkg/frontend/api`
- `frontend/pkg/frontend/api/auth`
- `frontend/pkg/frontend/api/datasystem`
- `frontend/pkg/frontend/api/job`
- `frontend/pkg/frontend/api/lease`
- `frontend/pkg/frontend/clusterhealth`
- `frontend/pkg/frontend/config`
- `frontend/pkg/frontend/frontendsdk`
- `frontend/pkg/frontend/middleware`
- `frontend/pkg/frontend/watcher`
- `frontend/pkg/frontend/wisecloud`

这些失败包含永久 token 断言、反射 mock 签名、etcd/config 环境依赖、超时、全局状态污染等，不属于本次 sandbox raw / websocket / invocation 同步的直接功能面。

## 本次未做的事情

- 未直接 merge `feature/sandbox`，避免引入旧分叉历史。
- 未逐 commit cherry-pick，避免重复同步已 squash 内容。
- 未处理 common/其他组件测试，因为用户明确要求只关注 frontend。
- 未强行保留当前 runtime API 不存在的 `api.InvokeOptions.IsInterrupted` 字段。
- 未修改全量 frontend 中与本次同步无关的历史不稳定测试。

## 后续快速恢复上下文

如果后续需要继续排查，可优先查看：

- `pkg/frontend/api/sandbox/handler.go`
- `pkg/frontend/posixws/processor.go`
- `pkg/frontend/api/functionsystem/handler.go`
- `pkg/frontend/common/util/client.go`
- `pkg/frontend/invocation/function_invoke_for_kernel.go`
- `pkg/frontend/webui/webterm.go`
- `pkg/frontend/state/state.go`

如果要复跑核心验证，可使用：

```bash
rm -rf /tmp/yuanrong-frontend-verify
mkdir -p /tmp/yuanrong-frontend-verify
rsync -a --delete --exclude='.git' ./ /tmp/yuanrong-frontend-verify/
docker run --rm \
  -v /tmp/yuanrong-frontend-verify:/workspace \
  -v "$PWD/../yuanrong/api/go":/api/go:ro \
  -w /workspace \
  swr.cn-southwest-2.myhuaweicloud.com/yuanrong-dev/compile_arm:2.1 \
  bash -lc 'export GOPATH=/root/go; export PATH="/root/go/bin:/opt/buildtools/golang_go-1.24.1/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"; sh test/test.sh faasfrontend'
```
