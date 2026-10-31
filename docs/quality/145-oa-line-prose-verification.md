# 增量145：定向核验与明确延期

## 当前已执行

| 编号 | 检查 | 当前结果与范围 |
| --- | --- | --- |
| 1 | 原生HTTP／PG及MCP | 2组，30子例＋2组汇总，共32条通过；真实四种会话渠道下采购／报销明细采用、未知／重复／商业字段拒绝、旧原文、撤销／权限／来源重查及回执失败回滚。原生PG18.6持久运行，无虚拟化；模型隔离证明为受控fixture |
| 2 | Web逐明细逻辑 | 11项通过；准确ID、同名行、字段／长度／NUL边界、明确采用及其他商业事实／顺序完整保留；不是全部452项旧套件重跑 |
| 3 | 共享移动逻辑 | 11项通过及相关编译；同一共享依据和收据判断；不是全部537项旧套件重跑，也不代表真实移动平台交互 |
| 4 | Skill运行器 | 当前57项通过，含新增明细键、总字段上限、重复／越界字段及合同拒绝明细键。实际模型未调用；八项包仍draft |
| 5 | 客户端类型与构建 | 当前类型、Web、微信和抖音本地API配置构建通过；不代表AppID、HTTPS域名、审核或正式产物通过 |
| 6 | 实际Web | 本人登录及MFA；原文／建议在390／768／1440宽检查通过；采购与报销各实际采用一次、准确保存读回；真正选择首条同名报销明细并发送固定授权，排队结果仍authorized，未虚构后续推理 |
| 7 | 演示媒体 | 三张实际PNG，约4.67秒／23帧无声实际MP4，六秒三截图GIF导览。PNG及视频首／中／尾和GIF三场景取样查看，未称完整连续播放 |
| 8 | 清理 | 本轮自建浏览器上下文／页面、Web服务、原生fixture和临时PG均关闭，私有handoff删除；保留用户原有服务 |

公共来源、命令、摘要、退出状态与计数见[证据目录](evidence/oa-line-prose/)、[当前Web](evidence/oa-line-prose-web/result.json)、[测试计数](evidence/oa-line-prose/selected-test-counts.json)和[总核验](evidence/oa-line-prose/final-source-check.json)。原生私有日志留在Git外，只公开不可逆摘要；不公开fixture口令、MFA秘密、模型／云凭据。

## 失败与复用

1. 第一次严格Skill编译因条件数组分支缺失type而失败：[原记录](evidence/oa-line-prose/runtime-tests-1.json)、[修复说明](evidence/oa-line-prose/attempts/01-schema-strict-types/reason.json)。补明确array类型后当前运行器57项及所有相关检查重跑；没有删除或改写首次失败。
2. 初次原生32条已通过，但绑定修复前生成器／API源；保留[native-reviewed](evidence/oa-line-prose/native-reviewed.json)，之后以[native-final](evidence/oa-line-prose/native-final.json)重新执行32条，不把旧摘要换成新源码。
3. 首次浏览器在缩放后立即测量失败，原截图、来源、关闭记录留在[第二次尝试说明](evidence/oa-line-prose/attempts/02-viewport-settling/reason.json)及同目录browser/native。等待字体及两帧布局完成后390／768／1440均无横向溢出；没有放宽断言或改CSS。当前捕获来源为独立新摘要，旧记录不冒充成功。
4. 全量后端证据仍121；Web452／共享移动537全套仍136；旧143及更早媒体保留原字节。当前APK144仍固定业务143／结构109，不包含145，不将其称为145重建。三个冻结候选及全部下载不改。

5. 首次仓库检查误把PNG／GIF／MP4纳入文本空白子集，按二进制字节出现的空白模式误报。原报告留在[发布子集修正](evidence/oa-line-prose/attempts/03-publication-text-selection/reason.json)；只修正文本子集，所有二进制仍经完整凭据／路径／大小扫描与媒体摘要检查，应用和媒体未改。

## 明确延期／仍开放

依据用户“确保主功能闭环、适当跳过部分验证”的授权，本轮不跑全部后端／客户端回归，不追加Taro H5完整旅程，不重建APK、Linux服务器包或正式平台产物。已有开发PG100、API服务和真实业务记录未更新。

真实模型／目标Linux／Skill安装评测、TG群与Bot、OSS＋R2、平台AppID／密钥／域名、正式Android签名／SDK／产物、实际监控与外部告警、生产PG与原生CI／部署仍分别在23工作包中开放。GitHub Releases API403的已有账号问题保留于144证据，未重复请求；源码与冻结资产通过SSH已发布。

真机、网络性能／实网120GB、历史迁移／容灾、AI费用控制仍是移出项，不称“已测试通过”，不因此启动模拟器。AI权限／隔离／审计／技术资源限制、正常结构升级、普通文件恢复与生产PG仍保留。

## English evidence notes

Current selected checks ran32 native HTTP/PG/MCP result records,11 Web logic cases,11 shared mobile cases and57 runtime cases, plus types/Web/two miniapp builds. The actual authenticated Web capture adopted selected purchase and claim line prose, preserved all other facts and queued a separate exactly selected claim request. Controlled MCP does not establish real Codex, model or target Linux acceptance.

The first strict-schema compile failure and first immediate-resize capture failure are retained with their sources. The schema was corrected and current checks rerun; waiting for fonts/two frames resolved layout measurement without waiving the bound. Current PNGs, silent MP4 and the disclosed three-screenshot GIF have separate hashes and sampled visual inspection. Every owned context, tab and temporary service/database closed.

Full regression, additional mobile UI and a newAPK are explicitly deferred. APK144/rc.3 remain business143/schema109; main145/schema110 is subsequent source. All23 packages and eight draft Skills remain open; excluded scopes are never counted as passed.
