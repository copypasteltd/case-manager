# 增量141 · 秘书运行配置关键验证 / Focused runtime operations evidence

## 本轮实际执行

| 编号 | 检查 | 真实范围 |
|---|---|---|
| V141-01 | 原生Windows耐久PostgreSQL，迁移1–107；两组实际HTTP／执行控制器测试 | 最终26条通过记录＝23个执行过的业务场景＋两父测试＋一个未启用浏览器保持分支的记账记录；不是26个业务用例 |
| V141-02 | 独立权限与四端会话 | Web实际Cookie／Origin／CSRF，WeChat／Douyin／Android实际Bearer；独立reader／manager、近期MFA、准确版本、明确核对、权限撤销后重放拒绝 |
| V141-03 | 实际任务变化与停用 | 普通HTTP创建／取消真实run，旧影响摘要冲突；实际IssueExecution及ExecutionScope，停用后失去执行权限，新任务返回503 RUNTIME_UNAVAILABLE；不调用真实模型 |
| V141-04 | 数据与事务 | 应用仍无profile表INSERT／UPDATE／DELETE，不能伪造journal；回执不可变、旧配置不能复活；注入audit错误后配置／回执／事件整体回滚 |
| V141-05 | 当前客户端 | 完整类型检查、Web及微信／抖音构建，Node22.23.4、local API profile；不是新Android包或正式小程序发布 |
| V141-06 | 实际认证Web | 配置台账→准确详情及一个排队任务影响→明确停用→重载＋独立HTTP读回；保持原run而不伪造取消；Go夹具另查一回执／审计、两历史 |
| V141-07 | 媒体 | 三张实际PNG、登录后视口MP4、三截图各两秒GIF；GIF不是操作录屏。完成后关闭所有自有浏览器／服务／夹具／PG |

最终原生测试耗时约19.097秒，-count=1明确不复用Go测试结果。所选原执行控制器11项场景重跑；证明／供应商为受控替身，不等于Linux隔离或真实Codex。新功能12个业务场景；单独浏览器保持分支只在实际Web演示时启用。

## 保留的失败及修正

1. attempts/initial-mfa-fixture：初始测试对不需要MFA的agent_user使用MFA登录辅助流程，得到空challenge。仅将隔离测试角色明确设为需要MFA；保留原测试源码及失败记录。
2. attempts/profile-lock-privilege：详情／停用准备曾对只读profile表使用FOR SHARE／FOR UPDATE，实际HTTP失败。移除应用侧行锁，沿用组织锁及受控触发器中的部署行锁，**没有授予应用UPDATE权限**。同次测试已经终止，但私有清理句柄重名使报告写出中断；保存实际终结测试事件重建诊断和原源码，不将其标为来源绑定通过。清理原PG已结束。
3. attempts/http-status-expectations及query-status-contract：最初负向断言错误地预期404／400／409；按既有契约修正为缺少近期MFA403、无效输入422、配置停用后新任务503；签名游标错误仍400。未改变产品契约来迎合测试。
4. attempts/before-builder-line-ending-preservation：保留一次完整通过；随后恢复OpenAPI构建器原有LF字节风格以缩小差异，最终相同两组测试在准确当前源码上再次实际执行。所有旧迁移和140及更早证明／媒体不改写。

5. attempts/capture-route-guard：首次捕获因复用助手仍只允许/telegram页面而主动拒绝，未提交业务；保存原来源、错误和已关闭原生PG证明。只将专属捕获允许页修正为/agent-runtime且拒绝查询参数，凭据可见性保护保留。
6. attempts/before-runtime-navigation-label：首次完整Web成功与PNG／MP4／GIF保留；视觉核对后补新导航标题，随后客户端检查和浏览器／媒体在最终源码上重新执行。

范围检查最初在功能证明尚未生成时缺少前置文件；按功能证明→范围→发布的顺序完成。范围与验收分类继续由原独立检查核对，没有改写检查器或取消门槛。

7. attempts/before-source-scope-import-correction：实际Web业务和像素再次通过，但最终核对发现新捕获／构建脚本仍导入旧来源模块，build_source摘要不匹配141清单。保留全部成功业务／媒体与未通过的最终来源报告；改为明确导入141来源模块后，客户端检查及浏览器／媒体再次实际执行，不用旧清单摘要冒充当前完整输入。

原始凭据、MFA秘密、临时日志、帧和数据库只留忽略目录；共享证据只有允许的摘要、合成标识、终结结果及准确源码快照。

## 延期与保留出处 / Deferred and retained

- 全量后端／运行器保留121出处，完整Web452／共享移动537项保留136出处，本轮不把它们称为新执行。
- Android仅共享源码更新；APK137固定业务136／结构103，不含138–141。完整移动UI、新原生／APK、真平台身份和正式签名未完成。
- 开发PG100不升级、不重启既有服务；新后端需正常升级至107。新建空PG完整迁移通过不代表已有生产库升级验收。
- 八Skill仍draft；安装／发布核验与真实模型、Linux隔离、TG、OSS／R2、监控、HTTPS、CI及生产PG继续开放。AI费用控制保持移出。
- 两冻结标签不变、23工作包14 P0／9 P1、五项移出不计通过；本轮不是全目标或生产发布验收。

## 可复核材料

[原生结果](evidence/agent-runtime-operations/native-final.json) · [Web结果](evidence/agent-runtime-operations-web/result.json) · [Web持久读回](evidence/agent-runtime-operations-web/business-proof.json) · [媒体摘要](evidence/agent-runtime-operations/media.json) · [来源检查](evidence/agent-runtime-operations/final-source-check.json) · [适用范围](evidence/agent-runtime-operations/scope-check.json) · [验收分类](evidence/agent-runtime-operations/acceptance-applicability-check.json)

本轮脚本在app/scripts：test_agent_runtime_ops_native.py、run_agent_runtime_ops_checks.py、run_agent_runtime_ops_browser.py、render_agent_runtime_ops_media.py、verify_agent_runtime_ops.py。测试及捕获会拒绝覆盖既有证据；已有尝试应先保留原来源。最终检查可导入verify_agent_runtime_ops.verify()只读执行；发布检查核对实际暂存文件。
