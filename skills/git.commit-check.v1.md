# skill｜git commit check (gitleaks) v1
> 目标：在提交/推送前阻止 secrets 进入 git 历史。默认：本地 pre-commit 扫 staged；pre-push 全仓复检；CI 再复检一次。

## 适用范围
- 所有代码仓库（尤其：Cloud Run / Vercel / infra / IaC）
- 所有会接触 token、证书、DB 账号、API Key 的变更

## 全局规则
- 默认阻断提交：检测到疑似 secret => 退出码非 0 => commit/push 失败
- 输出必须可操作：指出报告路径、下一步动作
- 误报必须可控：只允许在 `gitleaks.toml` 做最小 allowlist（路径/regex）
- 误报放行必须由项目负责人审核，且永不把真实 secret 写进 allowlist（只 allow “确定无害”的模式）

## 依赖
- gitleaks v8+（必须支持 `detect --pipe`）
- bash / sh
- git

## 安装/启用（仓库内）
1) 放置脚本（建议路径）：
- `scripts/hooks/run-gitleaks.sh`
- `scripts/hooks/pre-commit`
- `scripts/hooks/pre-push`
- `scripts/hooks/install-hooks.sh`
- `config/gitleaks.toml`

2) 一键安装 hooks：
```bash
bash scripts/hooks/install-hooks.sh
```

## Hook 行为设计
### pre-commit（快）
只扫描 staged patch（避免扫全仓导致慢）
命令（参考）：
```bash
git diff --cached -U0 | gitleaks detect -v --no-git --pipe --redact --config config/gitleaks.toml
```

### pre-push（稳）
推送前全仓扫描（兜底）
命令（参考）：
```bash
gitleaks detect -v --redact --config config/gitleaks.toml
```

## 输出与报告
- 报告目录：`.git/gitleaks/report.json`
- 发现泄露时输出：
  - “Potential secrets detected”
  - 报告路径
  - 处理动作（见 skills/git.secret-incident-response.v1.md）

## 可配置项（环境变量）
- GITLEAKS_BIN：默认 gitleaks
- GITLEAKS_CONFIG：默认 config/gitleaks.toml
- GITLEAKS_REPORT_DIR：默认 .git/gitleaks
- GITLEAKS_MODE：staged|full

## CI 复检（必须）
任何人都能绕过本地 hook，CI 必须复检：
```bash
gitleaks detect -v --redact --config config/gitleaks.toml
```

## 失败处理入口
统一转交到：skills/git.secret-incident-response.v1.md

## 验收标准
✅ 提交包含假 key 时，commit 被阻止并生成 report.json
✅ 误报可通过 allowlist 精准放行
✅ push 前全仓复检能挡住“漏网之鱼”
✅ CI 中同样会失败（防绕过）
