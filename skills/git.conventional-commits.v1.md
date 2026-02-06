# skill｜conventional commits v1
> 目标：统一 commit message 格式，提升可读性、自动化 changelog 生成、语义化版本管理。参考 Peter 的实践规范。

## 适用范围
- 所有代码仓库
- 所有团队成员的提交

## Commit Message 格式
```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type（必须）
参考统计比例（Peter 的实践）：
- **fix**: 修复 bug（31%，最常用，对应 PATCH 版本）
- **docs**: 仅文档变更（14%）
- **feat**: 新功能（10%，对应 MINOR 版本）
- **chore**: 构建过程或辅助工具的变动（9%）
- **test**: 添加或修改测试（6%）
- **refactor**: 代码重构（5%，既不修复 bug 也不添加功能）
- **perf**: 性能优化
- **style**: 代码格式调整（不影响代码含义的变更）
- **ci**: CI 配置文件和脚本的变更
- **revert**: 回滚之前的 commit

### Scope（可选）
影响范围，例如：api, ui, auth, db, docs, iac, config

### Subject（必须）
- 简短描述（50 字符以内）
- 使用祈使句，现在时态
- 首字母小写
- 结尾不加句号

### Body（可选）
- 详细描述变更的动机和实现细节

### Footer（可选）
- BREAKING CHANGES：以 `BREAKING CHANGE:` 开头
- Issue 引用：如 `Closes #123`

## 规则
- 必须基于 `conventional-commits` 规范
- 严禁模糊的描述（如 “update”, “fix”）
- 大规模变更必须细化为多个原子提交

## 验收标准
✅ 提交信息格式合规
✅ CI/CD 流程能正确解析类型并触发对应流水线
✅ 自动生成的 Changelog 结构清晰
