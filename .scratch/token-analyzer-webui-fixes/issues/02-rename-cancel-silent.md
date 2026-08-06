# 02 — 重命名取消不报错

**What to build:** 会话管理行内编辑中，按 Esc 或失焦取消编辑时静默恢复原名称，不显示任何错误提示；仅「保存但名称为空」时显示「显示名不能为空」并恢复。Enter 保存、非法字符拒绝、成功刷新分组的既有行为保持不变。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] Esc 取消（输入为空或非空）均无错误提示、名称恢复
- [ ] 失焦取消（非 Enter 触发）无错误提示
- [ ] Enter 保存空名仍显示「显示名不能为空」
- [ ] Enter 保存合法名仍正常改名并刷新分组
