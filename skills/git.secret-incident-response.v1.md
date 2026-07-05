# skill｜git secret incident response (auto-fix playbook) v1
> 目标：当 gitleaks 发现泄露时，提供“自动止血 + 可回滚 + 可审计”的标准处理流程。
> 原则：先撤销/轮换，再清理历史，最后加规则；不要反过来。

## 触发条件
- 本地 hook / CI 中 `gitleaks detect` 失败
- 或发现敏感信息已进入仓库（哪怕没有报警）

## 输入
- gitleaks 报告：`.git/gitleaks/report.json`（或 CI artifact）
- 泄露类型：API key / token / 私钥 / DB 密码 / 证书
- 泄露范围：仅未提交(staged) / 已提交 but 未推送 / 已推送到远端

## 全局规则（强制）
- 任何真实 secret 一律视为已泄露：必须 rotate/revoke
- 不允许“先 allowlist 让它过”来掩盖问题
- 清理历史是最后手段，但一旦推送过就必须执行

---

## 处理流程（按优先级执行）

### Step 0：立即止血（30 秒内）
1) 取消暂存/撤回改动（避免误提交）：
```bash
git restore --staged .
```

2) 在本地删除/替换泄露内容（改为读取 Secret Manager / env / vault）

3) 重新 staged 后再跑 gitleaks（确保干净）：
```bash
git add -A
git diff --cached -U0 | gitleaks detect -v --no-git --pipe --redact --config config/gitleaks.toml
```

### Step 1：撤销/轮换密钥（必须）
根据类型选择动作（示例）：
- GitHub token：revoke + 重新生成
- GCP SA key：禁用/删除 key，改用 Workload Identity（如可）
- DB 密码：立即改密码 + 强制最小权限 + 限制来源 IP
- TLS 私钥：重新签发证书

产物：记录 “rotated_at / rotated_by / secret_id / scope”。

### Step 2：判断是否进入 git 历史（分支决策）
情况 A：仅 staged / 未 commit
- 不需要清理历史，只需修复内容 + rotate 即可

情况 B：已 commit 但未 push
- 可以用 reset/squash 直接抹掉 commit：
```bash
git reset --soft HEAD~1
# 删除泄露内容后重新 commit
```
仍需 rotate（因为本地也可能被复制、日志、shell history 等）

情况 C：已 push 到远端（高危）
- 必须清理历史：推荐 git filter-repo（比 filter-branch 稳、快）
- 删除文件：
```bash
git filter-repo --path path/to/leaked.file --invert-paths
```
- 或按文本替换（需要准备 replacement rules）：
```bash
git filter-repo --replace-text replacements.txt
```
- 强制推送并通知协作者重拉：
```bash
git push --force --all
git push --force --tags
```

注意：即便清理了 git 历史，也必须 rotate，因为泄露已经发生。

### Step 3：误报处理（允许，但必须最小化）
仅当你确认它不是 secret（例如测试样例、公开字符串）：
1) 在 config/gitleaks.toml 里加 allowlist（最小范围）
2) 优先 path allowlist
3) 其次 regex allowlist（要尽量具体）
4) 重新运行 gitleaks 验证通过

### Step 4：加固（防再犯）
- CI 中强制 gitleaks（不可跳过）
- 将 secrets 全部迁移到 Secret Manager（或 Vault）
- 禁止在 docs/example 中出现真实 key（用 EXAMPLE_ 前缀）

## 自动化脚本建议（可选增强）
你可以在仓库提供一个统一入口脚本：
`scripts/security/secret-incident.sh`

功能：
- 自动打开 report.json
- 提示泄露文件/规则
- 输出对应处理命令模板（reset/filter-repo/allowlist）
- 生成一份 incident 记录（markdown）

## 验收标准
✅ 泄露发生时：提交/CI 必然失败
✅ 30 秒内可止血：撤回 staged 并定位报告
✅ 轮换动作有记录
✅ 已 push 泄露：历史清理可复现且协作指引明确
✅ 误报 allowlist 不扩大攻击面
