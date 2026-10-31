# 增量142 · 关键验证与延期 / Focused checks and deferrals

本轮按用户要求优先功能闭环，保留与目录授权、固定计划和真实建项直接相关的验证；未执行整套回归，不用历史通过结果充作当前重跑。

| 编号 | 实际执行 | 证据与结论 |
|---|---|---|
| V142-01 | 选定TestAgentProjectCatalog与既有TestAgentProjectTools；新建原生耐久PG | native-reviewed.json通过。13条pass记录包括两个顶层结果、10个实际目录业务子场景及一个未启用浏览器钩子返回；不把汇总和钩子算成新增业务场景 |
| V142-02 | Web Cookie／CSRF、微信、抖音、Android四种实际会话 | 逐一查询人员和模板、固定非本人负责人／成员／模板／日期、人工批准、实际创建、幂等读回；DB核对模板目录、任务、成员与日期 |
| V142-03 | 权限和来源失效 | 元数据白名单、草稿隐藏、严格筛选、私人会话归属、公司级权限、完整模板正文、分页、工具观察与提案、候选改变、退役、审计故障回滚；受控租约／隔离证明不等于真实模型或Linux隔离 |
| V142-04 | 当前运行时测试56项 | runtime-tests-3.json实际通过；工具描述、HTTP边界及全部现有运行时用例。没有调用真实模型 |
| V142-05 | 当前客户端类型及Web／微信／抖音本地构建 | typecheck-2、web-build-1、weapp-build-1、douyin-build-1通过；Node22.23.4、本地API配置。共享Android源码通过类型检查，本轮未构建APK |
| V142-06 | 真实认证Web操作、保存、批准、建项、刷新读回 | project-catalog-web/result.json三项检查通过；原生fixture独立核对持久项目。1个浏览器上下文及全部自建临时服务／PG关闭，无控制台异常或5xx |
| V142-07 | 三张PNG、实际MP4及截图导览GIF | media.json及visual-review.json绑定具体摘要；MP4为9帧、约2秒的实际视口操作；GIF为三张真实截图各两秒，不声称完整交互录屏 |
| V142-08 | 范围、适用性、发布内容核对 | scope-check、acceptance-applicability-check、final-source-check、repository-check分别记录；适用性检查不执行或通过AC／TC业务验收 |

四种会话接口检查不等于四端完整UI旅程。Web使用隔离合成业务和实际本地服务；没有真实Codex、Bot群、OSS／R2、目标Linux、正式平台或生产环境操作。未启动Docker、WSL、虚拟机或模拟器，保留用户原有服务和页面。

## 保留的尝试与修复

- native-initial.json保留实际失败：首个工具观察与PG jsonb重放按原始字节比较，键顺序不同造成测试假失败；改用规范JSON语义比较。初始测试源码快照保留在attempts/initial-contract-checks。随后native-final通过；最终补正退役来源读取场景的会话归属、补全OpenAPI路径枚举后，native-reviewed重新执行并通过，不复用旧源码通过声明。
- runtime-tests-1.json保留实际失败：新增工具已进入Skill／MCP，运行时HTTP允许工具名单尚未加入，真实配置描述会拒绝。已补入边界名单；旧源码快照保留，runtime-tests-2通过；最终源码在runtime-tests-3再次通过56项。
- 准备runner时一次变量提取错误在任何检查执行前终止，未产生“已执行”或“通过”证据。已修正CLIENT_SCOPES／SCOPES定义。所有后续验证记录保留其真实来源和结果，不覆盖失败。
- typecheck-1保留首次源码通过事实；随后严格UUID、100字节类型输入约束与接口类型修正触发typecheck-2和当前构建，分别留证。

## 明确复用与延期

| 项目 | 当前状态 |
|---|---|
| 全量Go／Web／移动测试 | 未重跑；保留121全量后端与原客户端记录的原出处。基线中的Web452／移动537不是本轮执行数量 |
| 运行时 | 本轮实际56项；不再把旧121运行时记录表述为当前执行 |
| 移动UI／原生 | 新完整移动交互旅程、原生打包延期；APK137仍业务136、结构103，不含138–142 |
| 两个冻结候选 | rc.1和rc.2标签及下载摘要保持；此次主线功能提交不移动标签，也不重新声明冻结包包含新功能 |
| 开发数据库 | 原个人开发PG100保留；选定检查和Web使用自建隔离PG108。当前主线使用前需正常升级，不自动改动用户数据库 |
| 真实模型／Skill | 八项包仍draft；共同schema变更导致八项摘要改变。旧配置证明不能作为新包安装或真实模型评测通过依据 |
| 当前范围 | 23 active（14 P0／9 P1），5 excluded；完整GAP-12／18及生产验收继续开放 |

## Reproduce with native tools

在本地原生依赖准备完成后，可按需执行：

```text
python app/scripts/test_project_catalog_native.py <new-attempt-name>
python app/scripts/run_project_catalog_checks.py runtime-tests typecheck web-build weapp-build douyin-build
python app/scripts/run_project_catalog_browser.py
python app/scripts/render_project_catalog_media.py
python app/scripts/verify_acceptance_applicability.py
python app/scripts/verify_project_catalog.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_repository_publication.py
```

浏览器及媒体脚本拒绝覆盖已有最终尝试；复查时应先审阅并归档原始记录，不能覆盖已发布证据。日志、合成登录凭据和原始录屏帧保持在被Git排除的app/.local。最终核对脚本只读检查已有证据，CLI仅写自身核对记录；不代表重新执行业务或正式验收。

English: selected native HTTP/PG business checks,56 current runtime tests, current client types/builds and one authenticated Web journey were actually executed. Earlier failures and successful intermediate sources remain available. Full regression, complete mobile UI and new native artifacts were deferred; external model/Linux/cloud/platform/production acceptance remains open. The two frozen candidates and retained APK keep their original provenance.
