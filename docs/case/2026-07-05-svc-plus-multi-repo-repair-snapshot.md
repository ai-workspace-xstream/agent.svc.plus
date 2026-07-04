# svc.plus 多仓库修复快照

> 用途：迁移 / 部署 / 回滚时，快速定位这次修复所对应的仓库分支、提交和统一 tag。

## 统一 Tag

- `svc-plus-20260705-repair-snapshot`

## 适用范围

本快照覆盖下面 5 个仓库：

- `/Users/shenlan/workspaces/cloud-neutral-toolkit/agent.svc.plus`
- `/Users/shenlan/workspaces/ai-workspace-service/postgresql.svc.plus`
- `/Users/shenlan/workspaces/ai-workspace-service/accounts.svc.plus`
- `/Users/shenlan/workspaces/ai-workspace-service/portal`
- `/Users/shenlan/workspaces/ai-workspace-infra/playbooks`

## 快照清单

| 仓库 | 本地路径 | 分支 | 当前提交 | 状态 | 说明 |
| --- | --- | --- | --- | --- | --- |
| agent.svc.plus | `/Users/shenlan/workspaces/cloud-neutral-toolkit/agent.svc.plus` | `hotfix/config-change-driven-restart` | `569096a36d2e8f8da1c3fd827b48f4e1959a5042` | 有未跟踪文件 | 本次 Xray 客户端同步修复所在仓库 |
| postgresql.svc.plus | `/Users/shenlan/workspaces/ai-workspace-service/postgresql.svc.plus` | `main` | `7aa0cae2a43c90e3c80f07c237f8da5c666b94af` | 干净 | PostgreSQL / stunnel 服务基线 |
| accounts.svc.plus | `/Users/shenlan/workspaces/ai-workspace-service/accounts.svc.plus` | `main` | `ff5a3376e90597d5e61c7d40b1f6bf80a7e4f34f` | 有本地修改 | 账户服务工作区，当前有 overlay 相关未提交改动 |
| portal | `/Users/shenlan/workspaces/ai-workspace-service/portal` | `codex/single-ssh-key-vault` | `20404cc3d2c64a8e88ee0778d0d6010f72e13d18` | 干净 | 门户相关工作区 |
| playbooks | `/Users/shenlan/workspaces/ai-workspace-infra/playbooks` | `fix/postgresql-external-mode` | `d128d9d46dd47a74224f16de513246646be6d9f8` | 有本地修改 | 迁移 / 部署 / 回滚主控 playbooks |

## 建议的使用方式

### 迁移

优先以这个 tag 组合检查目标环境：

1. `agent.svc.plus`
2. `postgresql.svc.plus`
3. `accounts.svc.plus`
4. `portal`
5. `playbooks`

### 部署

当需要复现这次修复时，按上表固定到对应分支和提交，再执行部署 playbook 或主机热更。

### 回滚

如果需要回滚，只要把这 5 个仓库同时回到上表中的提交即可。

建议顺序：

1. 先回滚 `playbooks`
2. 再回滚 `agent.svc.plus`
3. 再回滚 `accounts.svc.plus`
4. 再回滚 `portal`
5. 最后回滚 `postgresql.svc.plus`

## 备注

- `agent.svc.plus` 的本次修复已经合并到 `hotfix/config-change-driven-restart` 分支。
- `playbooks` 和 `accounts.svc.plus` 当前都有本地未提交改动；上表提交号记录的是 HEAD，不包含未提交内容。
- 统一 tag 仅用于帮助跨仓库定位，不替代各仓库自己的发布流程。
