# 增量161全部未完成项 / All remaining packages

截至本轮，**23项正式工作包仍开放（14 P0／9 P1）**。本轮C01–C08声明代码已整批完成；不能把该结论写成全部23项验收完成。下表把本轮已补代码与尚未解决的配置、规则、支持范围及验收放在同一行。

| 编号 | 优先级 | 工作包 | 当前仍未完成／边界 |
|---|---|---|---|
| GAP-01 | P1 | 项目完整管理 | 任务参与人／完成受阻资料、四组完整任务队列、Web关联完整分页已补；全公司角色／目录与跨模块最终验收仍待确认。 |
| GAP-02 | P1 | 合同、采购、报销 | 保留完整普通OA／审批／履行代码；真实审批人、金额阈值、自批／撤回规则及完整公司旅程待确认。 |
| GAP-03 | P1 | 费用、订阅与报表 | 保留增量158–160外汇、退款原本金与结项代码；公司会计口径、财务制度和完整核验待确认。 |
| GAP-04 | P1 | 文件正文、预览与检索 | 保留Office／OCR／DOCX正文代码；复杂结构／版式和真实样本准确性待验收；XLSX／PPTX合同正文采用仍不支持。 |
| GAP-05 | P1 | 文件生命周期 | 共享移动历史恢复、Android固定报告／重扫／例外及独立复核已补；未知原生对象身份、复杂派生清理及公司保留制度仍待处理。 |
| GAP-06 | P1 | 账号生命周期 | 账号／MFA／会话／恢复普通流程已有；真实人员、凭据交付与管理员分工待配置和核验。 |
| GAP-07 | P1 | 统一后台管理 | 新增TG固定预授权及固定范围／通知运行管理；真实模型／Skill、TG和存储配置与整体验收待提供。 |
| GAP-08 | P1 | 工作台与通知 | 新增四组完整任务待办、固定原群片段队列；全局业务制度、异常旅程和真实跨渠道送达仍待验收。 |
| GAP-09 | P0 | 真实Codex秘书接入 | 真实模型、专用凭据、请求→工具→回复／取消／超时／权限变更验收，依赖后续配置。 |
| GAP-10 | P0 | AI原生执行隔离 | 已有原生控制器与限制代码；真实Linux身份、目录／网络／资源隔离、证明及重启回收待安装验收。 |
| GAP-11 | P0 | AI计划与授权 | 新增准确普通跨范围计划执行、一次性TG update_task及固定原群通知；任意收件人、其他直接TG写工具与更广动作仍不支持。 |
| GAP-12 | P0 | 秘书业务工具 | 八类已有界定工具继续保留；更广业务／复杂Office及真实模型完整旅程仍待处理。 |
| GAP-13 | P0 | 八项Skill正式发布 | information-organize1.3.0／project-update1.10.0仍draft，共享定义变更需重核安装摘要；八包真实安装、样本、公司发布与回退验收待做。 |
| GAP-15 | P0 | 股东TG入口 | 一次性TG准确任务更新源码已补，其他七类仍本人入口；真实Bot／股东群／话题／绑定与结果送达待接入。 |
| GAP-16 | P0 | Android构建与发布 | 已补私有正式签名、递增版本、本地互斥及APK+AAB核对工程；真实签名密钥、正式产物及目标兼容验收待提供。 |
| GAP-18 | P1 | 四端业务覆盖 | 四端共享源码同步；本轮统一检查／本地Web与H5留证独立记录；H5恢复区重复加载占位已集中修复，当前只显示一个完成读取的队列；正式Android及全量跨模块UI验收仍开放，旧APK不含新代码。 |
| GAP-19 | P0 | 微信／抖音平台接入与发布准备 | 微信／抖音构建代码保留；真实AppID、域名、主体、平台配置、提审和发布待提供。 |
| GAP-20 | P0 | 实际OSS＋R2 | OSS＋R2真实桶／身份／CORS／分片／复制／失败恢复及原件清理语义核验，待后续配置。 |
| GAP-23 | P0 | 服务监控与告警 | 已有采集／资源／故障／恢复／站内责任代码；真实目标及独立外部告警送达待接入。 |
| GAP-24 | P0 | 生产部署与发布工程 | 已有原生部署／CI／HTTPS／发布回退工程；目标服务器、运行身份与配置待提供，普通升级至127仍需目标实际执行。 |
| GAP-25 | P0 | 生产数据持久化 | 已有持久PG事务与升级代码；生产PG连接、权限、持久性和实际读写验收待接入。 |
| GAP-26 | P0 | 依赖安全与产物 | 保留此前依赖整改证据；目标Linux与正式Android发布产物的冻结、安全核对仍待实际产物。 |
| GAP-28 | P0 | 当前范围完整验收 | 完整代码优先流程和本轮用例已落实；原30AC／72TC按现行范围继续验收，分类／本地合成通过不能替代正式接受。 |

真机验证、网络／实网120GB性能、历史数据迁移与容灾、AI费用控制共5项已移出当前范围；它们不计入未完成验收，也不记作通过。AI授权、隔离、审计与技术限制继续保留。详见[范围99](04-current-scope-and-acceptance.md)。

The23 active packages remain open. The eight declared code tasks finish before unified tests; external configuration, company decisions and unsupported wider capabilities are never inferred. No excluded device/network/migration/DR/AI-cost gate is reintroduced. See the [full code plan](../development/161-batch-code-plan.json), [workflow and constraints](../development/161-batch-functional-closure.md) and [selected evidence](../quality/161-batch-functional-verification.md).
