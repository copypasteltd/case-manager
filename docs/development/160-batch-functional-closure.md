# 增量160：整轮功能代码与统一验证

本轮先完成C01–C06全部应用代码、四端共享契约、正常升级脚本、原生服务配置和验证用例，再冻结整轮来源，随后统一验证和集中修复。开发中没有运行逐模块测试或发布。检查结果以独立证据记录为准。

| 编号 | 已实现代码 | 业务边界 |
|---|---|---|
| C01 | 项目／采购结项按退款原付款本金核对，独立显示本金币种 | 实际收到的外币现金、原付款本金、费用币种分别保留；不重写旧账或历史结项 |
| C02 | 已知终态上传的准确位置盘点、人工授权、独立HEAD观察工作进程、不可变回执；已放弃秘书草稿纳入普通清理授权 | 只观察已记录位置；404不证明分片不存在；不补造上传成功、扫描或内容校验；全量观察成功后重新起算保留期，再经普通清理流程 |
| C03 | 九类个人业务待办的完整分页与准确业务入口 | 固定完整当前可见集合；任何集合／权限变化使续页失效；单组超过5000项明确拒绝而不截断；读取不结清业务 |
| C04 | Web与共享移动端人工拟定1–10项资料整理计划，统一准确来源／目标／回执校验 | 同项目同分类；预览、人工批准与提交相互独立；不扩大AI授权范围 |
| C05 | 共享移动端合同签署、生效、计划节点、完成／终止；采购订单、验收、拒收、原验收行退货、取消、差异提案／重审／启用、完成；报销结清与商业历史／草稿归档 | 复用普通后端命令、固定完整商业修订及文件、本人验证和独立审批；不将审批自动变成费用、现金或交付 |
| C06 | 新契约、schema122–123、原生HEAD服务、整轮用例、四端来源核对、双语入口及证据工具 | 原生Windows／PG检查与合成浏览器演示不能替代真实云／模型／平台／生产验收 |

## 原生观察与清理逻辑

前端读取已过期且无正式版本的已知会话，逐项审阅全部已记录上传尝试和晋级候选位置。提交固定目录／会话版本、来源摘要及本人近期验证，生成五分钟内有效的不可变请求。独立工作进程使用专用 `ORIGINAL_OBSERVER` 身份，只调用HEAD；每次读取前及保存回执前重新核对原人类会话、目录和来源、任务租约及所有保护。授权变化后停止，已读取的部分结果不作为成功来源。

只有全部位置取得准确大小、ETag和适用原生版本的回执才能成为新的普通清理来源。OSS需要非空原生版本；R2需要明确确认独占且不可变的管理空间。首次观察时间重新起算保留期；原上传状态、字节校验、正式引用、活跃任务／已签发许可、放弃记录、保留策略及人工清理批准均独立保留。未观察到、失败、未知对象及未提供真实云配置仍明确开放。

部署增加 `worker-original-observation`、独立服务账号及专用私有环境样例。API只需要位置元数据；不能借用清理／写入身份作为观察凭据。生产安装和真实IAM尚未执行。

## 全部工作包复核

23项仍是14P0／9P1。下表按全量原编号复核，本轮代码子任务完成不等于整个工作包验收完成。原始未完成说明保留在JSON计划 `original_remaining`，5项移出范围及8个draft Skills保持不变。

| 编号 | 工作包 | 本轮处理 | 正式验收 |
|---|---|---|---|
| GAP-01 | 项目完整管理 | C01 corrects current project foreign-refund blockers. Full company project closeout adoption remains open. | 保持开放 |
| GAP-02 | 合同、采购、报销 | C05 adds shared mobile contract/purchase execution and claim settlement/history parity using existing ordinary commands. Company policy and full journeys remain open. | 保持开放 |
| GAP-03 | 费用、订阅与报表 | C01 separates refund original principal from actual cash at closeout. Existing source facts/reversals are preserved; company accounting adoption remains open. | 保持开放 |
| GAP-04 | 文件正文、预览与检索 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-05 | 文件生命周期 | C02 adds exact known terminal metadata observation and abandoned-agent completed-draft access; new retention clock, native identity and all ordinary cleanup gates remain. Unknown coordinates/objects and real cloud acceptance remain open. | 保持开放 |
| GAP-06 | 账号生命周期 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-07 | 统一后台管理 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-08 | 工作台与通知 | C03 offers complete current authorized inventory for all nine groups, without completing or acknowledging business work. External notices remain open. | 保持开放 |
| GAP-09 | 真实Codex秘书接入 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-10 | AI原生执行隔离 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-11 | AI计划与授权 | C04 adds manual exact-scope organization-plan creation to shared mobile. Wider cross-project AI execution and TG write preauthorization remain unsupported/open. | 保持开放 |
| GAP-12 | 秘书业务工具 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-13 | 八项Skill正式发布 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-15 | 股东TG入口 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-16 | Android构建与发布 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-18 | 四端业务覆盖 | C03–C05 add full inventory, organization preparation and OA execution shared source. No real-device/emulator/APK or full acceptance claim. | 保持开放 |
| GAP-19 | 微信／抖音平台接入与发布准备 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-20 | 实际OSS＋R2 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-23 | 服务监控与告警 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-24 | 生产部署与发布工程 | C02/C06 include a separate native HEAD-only service and strict dedicated storage credentials. Target deployment remains deferred. | 保持开放 |
| GAP-25 | 生产数据持久化 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-26 | 依赖安全与产物 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |
| GAP-28 | 当前范围完整验收 | Reviewed as part of the full23 active register. Existing implementation is retained; remaining company policy, actual integration or formal acceptance in original_remaining is not invented or relabeled. | 保持开放 |

## 验证和交付约束

证据目录：`docs/quality/evidence/batch-round-160/`。首次冻结、首次失败、集中修复及受影响检查重跑分别留存，不覆盖旧失败。浏览器使用独立合成数据；录制在登录后开始，排除凭据，结束后关闭全部自建上下文、临时Web／H5／API和测试数据库。当前开发PG100/API保持原状。

本轮没有新APK或新冻结标签；八个旧冻结及rc.8/APK154仍精确对应business153/schema113，当前源代码schema123应通过正常升级部署。沿用的旧Android／广泛回归结果不会标成160重跑。本轮不做真机验证、不做网络性能／实网120GB、不做历史迁移与容灾、不做AI费用控制。真实服务器／云／Codex／TG／小程序配置继续等待用户后续提供。

新增升级122–123基于schema121。若部署基线是rc.8/schema113，应先顺序完成114–123；现有开发schema100若后续启用新服务器，则需101–123。升级前按部署指南停止对应写服务并使用独立升级账户；本轮未对现有开发数据库或API执行升级。
