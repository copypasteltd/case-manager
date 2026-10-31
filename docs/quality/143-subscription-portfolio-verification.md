# 增量143 · 关键验证与延期 / Focused checks and deferrals

按用户要求优先主功能闭环，选择与完整枚举、私有财务资料和不可变回执有关的检查；没有执行整套回归，也没有将已移出项重新加为门槛。

| 编号 | 实际执行 | 证据与结论 |
|---|---|---|
| V143-01 | TestAgentSubscriptionPortfolio、既有TestAgentSubscriptionReview、TestSubscriptionReviewMCPBoundary | native-reviewed.json，28条pass记录：3个顶层结果、10个新组合审阅子场景、14个原逐笔审阅子场景及1个未启用浏览器钩子返回；不把汇总和空钩子记成新增业务场景 |
| V143-02 | Web Cookie／CSRF、微信／抖音／Android四种实际会话 | 全部订阅、准确来源SHA和缺失账期；真实HTTP／原生耐久PG，接口会话不等于四端完整UI |
| V143-03 | 范围、权限、上限、事务和来源 | 严格日期／查询、跨会话拒绝、完整受众依赖、21项超过发现页、101项整体失败、未知草稿／空集合、实际费用／付款、不可变报告、准确重放／引用完成、审计失败回滚 |
| V143-04 | 当前运行时56项、客户端类型 | runtime-tests-2、typecheck-1通过；新增工具进入运行边界和实际完成回执核对，没有调用真实模型 |
| V143-05 | Web、微信和抖音本地构建 | web-build-1、weapp-build-1、douyin-build-1通过，Node22.23.4，本地API配置；共享Android源码通过类型检查，没有新APK |
| V143-06 | 实际认证Web、完整范围、正文分页和刷新再读 | subscription-portfolio-web/result.json三项通过；原生fixture另行确认预览未生成秘书报告；全部自建上下文／临时服务／PG关闭，无控制台异常或5xx |
| V143-07 | 三张PNG、实际MP4、三截图导览GIF | 当前media.json、visual-review.json绑定实际摘要；MP4为11帧、2247毫秒的实际视口，GIF三张真实截图各两秒，不声称完整交互录屏 |
| V143-08 | 适用性、范围与仓库发布检查 | final-source-check、acceptance-applicability-check、scope-check、repository-check分别留证；适用性核对不执行业务AC／TC |

## 原始尝试与修正

- native-initial保留失败：上限样本遗漏必需vendor_state，严格请求契约拒绝。补齐合成样本后native-final通过，没有放宽业务契约。
- native-final是中间通过来源；复查发现构造101项样本可能超过原隔离证明期限。最终以新租约调用，并准确断言scope／100项上限错误，避免把超时误当数量保护。当前native-reviewed重新执行全部三组选定测试并通过；两个原测试快照均保留。
- runtime-tests-1保留失败：新工具已进入manifest，运行器测试仍固定旧工具集合。补齐准确集合、实际结果工具验证和原回执测试后，runtime-tests-2实际通过56项。原文件快照保留，未删除否定断言。
- 第一轮Web通过，但编码器复制的说明误写为项目创建。原描述、原报告、原素材和脚本快照保留在attempts/media-description-before，明确为非当前来源。更正说明后重新完成Web采集和编码，当前内容只声称订阅查阅／依据翻页，不声称写入财务或创建项目。
- 准备脚本时的编码读取错误及文档精确替换未匹配发生于对应检查之外；没有把这些中止计成业务通过。

## 复用与延期

| 项目 | 当前状态 |
|---|---|
| 全量Go／Web／共享移动回归 | 未重跑；121全后端和136客户端记录保留原出处，基线Web452／移动537是旧数量 |
| 运行时 | 本轮真实56项，明确区别于旧121记录 |
| 移动UI／原生 | 新完整移动交互与新APK延期；APK137为业务136／结构103，不含138–143 |
| 两个冻结版本 | 原rc.1／rc.2标签和下载摘要不动；此次主线功能不是新的冻结包 |
| 开发PG | 原个人开发结构100保留，定向检查和Web使用隔离结构109；当前后端运行前正常升级 |
| 秘书／Skill | 八项仍draft，subscription-review1.4.0的新包摘要需专用部署核验／安装；受控调用不能证明真实模型或Linux隔离 |
| 真实环境 | Codex模型、TG、OSS／R2、目标Linux／HTTPS／PG、真实平台／正式签名未接入，相关工作包继续开放 |
| 当前范围 | 23 active（14 P0／9 P1）、5 excluded；完整GAP-12／18继续开放 |

未启动Docker、WSL、虚拟机、模拟器或实体设备。真机、网络性能／实网120GB、历史数据迁移／容灾和AI费用控制不是门槛。AI授权、隔离、审计、技术限制，普通升级、文件恢复和OSS＋R2继续保留。预览不产生资金或费用记录，组合报告不把不同币种加总。

## 原生复现

```text
python app/scripts/test_subscription_portfolio_native.py <new-attempt>
python app/scripts/run_subscription_portfolio_checks.py runtime-tests typecheck web-build weapp-build douyin-build
python app/scripts/run_subscription_portfolio_browser.py
python app/scripts/render_subscription_portfolio_media.py
python app/scripts/verify_acceptance_applicability.py
python app/scripts/verify_subscription_portfolio.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_repository_publication.py
```

独立Web／原生runner拒绝覆盖旧尝试，复查先归档原记录。私有登录资料、日志与原录屏帧留在被排除的app/.local，最终脚本只读取证据并写自身核对报告，不等于重新执行业务。

English: three selected native HTTP/PG/MCP suites,56 runtime tests, current client types/builds and an authenticated readonly Web journey actually ran. Complete-set limits, exact audience/dependency rights, immutable receipts and atomic audit were checked. Earlier failures and superseded media descriptions retain their original sources. Full regression, mobile UI, a fresh APK and external production acceptance remain deferred; both freezes retain their original identities.

[使用及设计](../development/143-subscription-portfolio.md) · [本轮及全部23项](../delivery/05-functional-closure-143.md)
