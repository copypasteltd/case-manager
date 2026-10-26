# 官方资料与核实记录

核实日期：2026-10-22。本文记录与方案直接相关的来源、查阅范围和限制。产品状态和价格可能变化，正式采购与实现前应复核所选版本、地区和账号。

## 1. 来源与结论

| ID | 官方来源 | 本次核实内容及使用边界 |
| --- | --- | --- |
| S-01 | [CopyPaste 官网](https://copypaste.hk/) | 原生 HTTPS 成功读取，页面介绍 AI、网络和支付协作；项目档案主线是据此提出的设计判断，未核实内部组织或经营数据 |
| S-02 | [OpenAI API 支持地区](https://developers.openai.com/api/docs/supported-countries) | 实际打开并查阅列表；未列出 Hong Kong / China。账号、实际地区与服务提供资格须进一步确认 |
| S-03 | [Codex SDK](https://learn.chatgpt.com/docs/codex-sdk) | 可程序化控制本地 Codex 线程；官方介绍服务端 TypeScript 库。具体事件、取消和原生状态恢复需按锁定版本 PoC |
| S-04 | [Codex app-server](https://learn.chatgpt.com/docs/app-server) | 自定义客户端协议、线程/回合、事件和交互；文档标注 app-server 命令与 WebSocket 的实验限制。不能据集成能力推断生产 SLA |
| S-05 | [Codex 认证](https://learn.chatgpt.com/docs/auth) | API key 与 ChatGPT 登录/企业自动化凭据具有不同适用范围和计费；公司共享认证方案需单独确定 |
| S-06 | [Codex 非交互模式](https://learn.chatgpt.com/docs/non-interactive-mode) | 后台有界任务、结构化/事件式处理是候选集成方式；业务持久化仍由平台负责 |
| S-07 | [Codex as a platform](https://developers.openai.com/blog/codex-as-a-platform) | harness、SDK/app-server 和模型服务是不同层；自有运行时不等于本地模型推理 |
| S-08 | [OpenAI 数据控制](https://developers.openai.com/api/docs/guides/your-data) | API 数据使用、应用状态/日志保留和区域设置有各自限制；本方案不承诺未经确认的零保留 |
| S-09 | [OSS 上传方式](https://www.alibabacloud.com/help/en/oss/user-guide/upload-objects-to-oss/) | 普通上传和分片/断点上传能力不同，120 GB 应走后者；客户端直传仍需后端授权和完成校验 |
| S-10 | [OSS SDK Quick Start](https://www.alibabacloud.com/help/en/oss/user-guide/oss-sdk-quick-start) | 官方提供 Go 等 SDK；推荐 OSS 原生适配，不假设与 R2 所有 S3 行为一致 |
| S-11 | [OSS 跨区域复制](https://www.alibabacloud.com/help/en/oss/user-guide/cross-region-replication-overview/) | 原生 CRR 描述 OSS 桶间异步复制，本方案不将其当作 OSS→R2 原生同步 |
| S-12 | [R2 S3 兼容](https://developers.cloudflare.com/r2/api/s3/api/) | 部分 S3 API 不支持，包括所列版本/复制相关能力；用应用级版本、专用复制流程规避依赖 |
| S-13 | [R2 限制](https://developers.cloudflare.com/r2/platform/limits/) | 120 GB 在 Multipart 对象限制内，分片数量有限；入口与工具也可能有独立大小限制 |
| S-14 | [R2 上传与 Multipart](https://developers.cloudflare.com/r2/objects/upload-objects/) | 分片可重试；有分片大小/数量约束及未完成任务生命周期；默认 7 天中止需与任务恢复策略一致 |
| S-15 | [R2 数据位置](https://developers.cloudflare.com/r2/reference/data-location/) | Location Hint 是尽力提示，jurisdiction 是不同机制；不承诺具体城市或未经核实的两地要求 |
| S-16 | [R2 Bucket Lock](https://developers.cloudflare.com/r2/buckets/bucket-locks/) | 可阻止覆盖/删除，管理员可改规则；应隔离管理凭据，不能声称防最高权限账户失陷的绝对 WORM |
| S-17 | [R2 预签名 URL](https://developers.cloudflare.com/r2/api/s3/presigned-urls/) | 签名是短时持有凭证，S3 API 域名与自定义域名边界不同；严格受限文件需实时鉴权策略 |
| S-18 | [R2 价格](https://developers.cloudflare.com/r2/pricing/) | Standard USD 0.015/GB-month、免费额度与操作单独计费；R2 无出流量费不代表全链路免费 |
| S-19 | [PostgreSQL PITR](https://www.postgresql.org/docs/current/continuous-archiving.html) | 连续 WAL 与物理基准备份支持时间点恢复；pg_dump 不能替代物理基准备份参与 WAL 回放 |
| S-20 | [PostgreSQL RLS](https://www.postgresql.org/docs/current/ddl-rowsecurity.html) | 超级用户/特权角色和表所有者的绕过行为；启用 RLS 需要相应数据库角色与连接池设计 |
| S-21 | [Taro 平台介绍](https://docs.taro.zone/)、[Taro RN](https://docs.taro.zone/docs/react-native) | 官方资料列出微信、抖音、RN 路线；不同端需要适配，页面启动不证明原生上传和发布就绪 |
| S-22 | [抖音 tt.uploadFile](https://partner.open-douyin.com/docs/resource/zh-CN/mini-app/develop/api/network/http/tt-upload-file) | 实际打开，线上 HTTPS、域名配置、文件/前后台错误等均有平台要求；不据此承诺 120 GB 小程序上传 |
| S-23 | [Telegram Bot API](https://core.telegram.org/bots/api) | HTTPS Webhook、secret_token、更新身份/话题和回调机制；公司业务权限仍需自己实现 |
| S-24 | [Telegram Bot FAQ](https://core.telegram.org/bots/faq) | 隐私模式影响群消息接收，发送有限流与文件边界；采用分阶段消息和门户大文件入口 |
| S-25 | [Prometheus 概览](https://prometheus.io/docs/introduction/overview/) | 可复用指标/告警体系；本方案建议外部探测和状态过期处理，未选定实际托管服务 |

OpenAI 原 developers.openai.com/codex 文档入口在本次访问中重定向至上述 learn.chatgpt.com 页面，本文使用实际落地的官方地址。

## 2. 查阅限制与待补验证

- 公司首页的网页抓取工具读取失败，原生 HTTPS 返回 200 并读取到正文；未据搜索中的同名公司推断你司实体情况。
- Taro 当前文档部分网页直读超时，已查询到官方当前 RN/平台介绍内容，并阅读官方文档镜像的流程说明；具体 Taro/RN/原生依赖版本组合仍须 PoC 冻结，不引用旧镜像的版本表作为当前兼容保证。
- [微信网络能力入口](https://developers.weixin.qq.com/miniprogram/dev/framework/ability/network.html) 本次未成功读取，不能把未读取页面当作已核实的上传限额或主体规则。微信正式域名、上传/预览、主体与发布要求进入 P0 核验。
- 未登录你司云账号、未读取服务器、未测试香港实际访问链路、未查询平台主体资格，也未上传真实 120 GB 文件。本套文档中的相关指标均是设计目标或待验证条件。
- 未创建 Telegram Bot、加入群或向任何员工/股东发送消息。本轮没有打开交互式浏览器标签页，也未启动任何本地虚拟化环境。

## 3. 事实、估计与设计判断

已核实事实包括官方能力/限制和用户提供的 3 人、境内服务器、香港访问、2 TB、120 GB、首发平台、业务范围与大文件分工。

设计判断包括模块化单体、独立 PostgreSQL、统一费用台账、应用级版本、任务幂等、渠道隔离和单主跨云副本。这些是为满足当前需求提出的工程方案，不是某个供应商保证的现成功能。

容量估计包括版本系数 1.5、月新增 100 GB、20 台/50 个探测对象、AI 使用量和资源规格。工作量/周期、云费用区间和候选 RPO/RTO 同样是可讨论的规划参数，后续用盘点、PoC 与报价替换。

法规、合同资料的保留期、跨境资料处理和账号服务资格应由公司负责人结合实际制度及供应商确认；本规划未作具体法律或财务合规结论。
