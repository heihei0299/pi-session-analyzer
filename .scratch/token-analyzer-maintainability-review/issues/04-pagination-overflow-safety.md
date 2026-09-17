# 04: 让 HTTP 分页参数在溢出场景安全失败

**What to build:** 让 sessions 和 requests 的 HTTP 分页在正常输入、缺失配对参数、超大整数和空结果集下都保持稳定，不因索引计算而 panic。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] page 和 size 必须成对提供，并且只能是正整数；非法组合返回稳定 client error。
- [x] 超大但可被整数解析的 page/size 不会造成乘法或加法溢出。
- [x] 超出结果集的 page 返回空 rows，不产生负 slice boundary 或 handler panic。
- [x] sessions 和 requests 都遵守同一套分页边界策略。
- [x] 默认排序、显式排序、total、page、size 和重复查询顺序保持既有契约。
- [x] 回归测试覆盖最大整数、溢出组合、空页、最后一页和重复分页查询。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：新增共享 `paginationBounds`，在计算索引前避免整数溢出，并让越界页稳定返回空 rows；HTTP sessions/requests 回归覆盖非法组合、正常/最后一页、排序、重复查询和大整数。
- 验证：`TOKEN_ANALYZER_DB= go test ./internal/server ./internal/query`，39 个测试通过。
- Review：完整 Standards/Spec 双轴 Review 已通过；分页契约与边界增量复核关闭。
- Commit：`316fc41 fix(query): make pagination overflow-safe`。
- 未解决边界问题：无。
