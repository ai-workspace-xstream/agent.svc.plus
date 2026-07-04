# 事故复盘：agent.svc.plus Xray 租户客户端同步丢失

## 基本信息

| 项目 | 内容 |
| --- | --- |
| 事故日期 | 2026-07-04 |
| 影响主机 | `tky-proxy.svc.plus` |
| 影响服务 | `agent-svc-plus.service`、`xray.service`、`xray-tcp.service` |
| 相关接口 | `https://accounts.svc.plus/api/agent-server/v1/users` |
| 相关 PR | [`ai-workspace-xstream/agent.svc.plus#7`](https://github.com/ai-workspace-xstream/agent.svc.plus/pull/7) |
| 关键提交 | `569096a fix xray client sync across inbound order` |
| 事故级别 | P1 级配置同步故障，租户客户端丢失 |

## 摘要

`agent.svc.plus` 在同步 Xray 配置时，控制器侧明明返回了 10 个客户端，但节点上落盘的
`/usr/local/etc/xray/config.json` 和 `/usr/local/etc/xray/tcp-config.json` 里只剩下默认的
`admin@svc.plus` 一个客户端。

表面上看，日志一直提示 `xray config synchronized` 且 `clients=10`，但实际渲染后的配置没有把
租户列表写进真正的业务 inbound，导致租户客户端全部丢失。

## 影响范围

- `tky-proxy.svc.plus` 上的 Xray 业务配置被压缩成单客户端
- 仅默认 admin 客户端保留，租户客户端不可用
- `xhttp` 和 `tcp` 两条同步目标都受到影响
- 对应节点的租户连接能力中断，属于面向真实用户的配置回归

## 现场现象

### 控制器侧正常

从节点直接请求控制器接口：

```bash
curl -sk \
  -H "Authorization: Bearer ***" \
  -H "X-Service-Token: ***" \
  -H "X-Agent-ID: tky-proxy.svc.plus" \
  -H "Accept: application/json" \
  https://accounts.svc.plus/api/agent-server/v1/users
```

返回 payload 中包含 10 个客户端，且每个客户端都带有 `ID`、`Email`、`Flow`。

### 节点配置侧异常

落盘后的配置文件里，只有一个客户端：

- `/usr/local/etc/xray/config.json`
- `/usr/local/etc/xray/tcp-config.json`

实际检查结果是：

- `config.json` 的 `inbounds[1].settings.clients` 只有 1 项
- `tcp-config.json` 的 `inbounds[1].settings.clients` 只有 1 项
- 两者都只保留了 `admin@svc.plus`

### 日志表象误导

`agent-svc-plus` 的日志持续显示：

- `xray config synchronized`
- `target: xhttp`
- `target: tcp`
- `clients: 10`

这说明同步循环本身在跑，控制器返回值也正常，但生成器没有把客户端写进真正的业务 inbound。

## 时间线

| 时间 | 事件 |
| --- | --- |
| 2026-07-04 23:08:46 | 日志显示 `clients=10`，但配置文件仍只有 1 个客户端 |
| 2026-07-04 23:31:12 | 修复版二进制重新部署后，首次同步写入 10 个客户端 |
| 2026-07-04 23:31:12 之后 | `config.json` 与 `tcp-config.json` 都恢复为 10 个客户端 |

## 根因分析

### 直接根因

`internal/xrayconfig/generator.go` 里的 `updateClients()` 只检查并更新 `inbounds[0]`。

而生产模板中：

- `inbounds[0]` 是 `api` inbound
- 真正承载租户的 `vless` inbound 在 `inbounds[1]`

因此旧逻辑压根没有命中需要替换客户端列表的那一层。

### 为什么日志会显示 `clients=10`

`agent.svc.plus` 的同步流程里：

1. 控制器接口返回 10 个客户端
2. 同步器记录 `len(clients)=10`
3. 生成器仍然只改了错误的 inbound 层
4. 因此日志看起来正常，但最终配置文件是错的

### 与 PR #7 的关系

[`PR #7`](https://github.com/ai-workspace-xstream/agent.svc.plus/pull/7) 解决的是“配置变化才重启 Xray”的问题。

这次事故属于同一条链路上的另一类缺陷：

- PR #7 让重启变得更智能
- 但生成器仍然依赖了一个脆弱假设：`inbounds[0]` 一定是业务 inbound
- 生产模板顺序变化后，这个假设失效，造成租户客户端被写丢

## 修复内容

### 代码修复

已在 `hotfix/config-change-driven-restart` 分支追加提交 `569096a`，核心修改：

- `updateClients()` 改为遍历所有 `inbounds`
- 命中包含 `settings.clients` 的 inbound 才执行替换
- 保留 `xhttp` 不写 `flow`、`tcp` 继续写 `flow` 的差异化逻辑

### 回归测试

新增测试覆盖：

- `api` inbound 在前
- `vless` inbound 在后
- 仍然能正确渲染出完整客户端列表

## 验证结果

### 本地验证

```bash
cd /Users/shenlan/workspaces/cloud-neutral-toolkit/agent.svc.plus
go test ./...
```

### 节点验证

修复后在 `tky-proxy.svc.plus` 上确认：

- `/usr/local/etc/xray/config.json` 中 `inbounds[1].settings.clients` 为 10 个
- `/usr/local/etc/xray/tcp-config.json` 中 `inbounds[1].settings.clients` 为 10 个
- 第一项和最后一项客户端都与控制器返回值一致

## 过程中额外遇到的部署问题

在现场热更二进制时，还出现过两个与根因无关的部署问题：

1. 误把本机 macOS 构建产物推到 Linux 节点，触发 `Exec format error`
2. 后续一次错误构建导致服务出现 `SEGV`

这两类问题都属于部署验证阶段的额外故障，不是本次租户客户端丢失的直接根因。

## 后续避免办法

1. 不要再把 `inbounds[0]` 视为业务入口的固定假设，所有 Xray 渲染逻辑都应扫描真实的 `settings.clients`
2. 给模板顺序变化增加回归测试，至少覆盖 `api` 在前、`vless` 在后的生产顺序
3. 上线前先做“控制器返回值”和“落盘配置”双验证，不要只看同步日志
4. 任何 AI 辅助修改（包括 Opencode / DeepSeek v4 Pro 之类的改动流）都必须经过：
   - 真实模板渲染验证
   - 目标主机架构验证
   - 配置文件最终结果验证
5. 现场替换二进制前先检查：
   - `uname -m`
   - `file /usr/local/bin/agent-svc-plus`
   - `go env GOOS GOARCH`

## 建议给 PR / issue 的简版说明

`agent.svc.plus` 在同步 Xray 客户端时默认只更新 `inbounds[0]`，但生产模板里 `api` inbound 放在第 0 位，真正的业务 inbound 在第 1 位，导致租户客户端列表没有被写入最终配置，只剩默认 admin 客户端。已修复为遍历所有 inbound 并替换命中的 `settings.clients`，同时补充了 inbound 顺序回归测试。
