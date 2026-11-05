# 157 · 整批开发与集中验证 / Batch verification

本轮先对全部23编号核对现有实现、未实现能力和外部条件，完成9项声明代码后记录 `freeze-code-code-wave-1.json`，再统一检查和集中修复。该代码清单不代表发布冻结或全部Goal验收。目标结构117；八项Skill继续draft。

## 实际本地结果

| 编号 | 当前检查 | 覆盖与限制 |
| --- | --- | --- |
| V01 | 原生后台选定检查：52条结果记录 | 6个顶层测试：准确Office更新／放弃恢复、发布治理、TG门户授权／独立群披露、真实DOCX／XLSX／PPTX编辑、严格工具清单，以及旧完成上传清理的Web新建／替换。52包含包级结果及子项，不能称为52个独立业务用例。 |
| V02 | Web 494通过 | 当前常规测试脚本；含本轮接口／表单边界。 |
| V03 | 共享移动555通过 | 常规脚本两组240与308，加本轮7项；覆盖共享源码，不等于微信／抖音／Android实际平台验收。 |
| V04 | 运行器60通过、八项draft目录检查通过 | 本地协议、工具边界与包目录；合成模型驱动，不是真实模型或Skill评估。 |
| V05 | 类型检查、Web／微信／抖音构建通过 | 三个当前源码构建目标；没有平台审核或线上发布。 |
| V06 | Android JS构建与194个当前应用模块核对 | 复用仓库已公布四worker Windows兼容流程和无Expo映射检查；本轮不执行Gradle、不生成新APK。 |
| V07 | 实际Web操作、截图及无声录屏 | 隔离合成数据，真实原生HTTP／PG及页面交互；办公室和TG各有独立源码摘要、清理记录和媒体。失败尝试完整保留。 |
| V08 | 范围、发布与历史保留检查 | 23 active、5 excluded；30 AC／72 TC适用分类不代表业务用例通过。旧冻结、旧迁移和旧媒体保持字节不变。 |

当前最终证据由 [独立汇总](evidence/batch-round-157/final-source-check.json) 绑定实际通过的尝试；[完整尝试说明](evidence/batch-round-157/README.md) 保留首轮失败、中断和修复缘由。重新执行采用新attempt名称，不覆盖旧尝试。

## 修复与证据

首轮完整后台命令在HTTP夹具编译失败，其余执行结果保留，不记完整后台通过。后续集中修复新夹具、共享类型和断言、Skill YAML、普通清理查询的活动游标、严格Office工具登记及数据库回执／观察约束、新公司初始化角色、TG准确字段和原有投递状态机。既有授权、不可变台账、精确源检查及投递fencing继续有效。

批次5在桌面／工具上下文更换时原进程消失；仅保留部分日志摘要与中断结果，不推断HTTP测试完成。前两个新157执行器复用了生命周期证据文件，批次2覆盖了批次1生命周期快照；不能把后续快照重建成原始证据。批次3起生命周期按attempt独立存放，旧156及更早证据未受影响。

客户端批次3保留九项成功及原两worker Metro中止的失败步骤；其独立兼容构建成功也单独记录，未篡改原失败。页面复查再发现TG列表把门户授权误标为撤回，已修正Web／移动文案；最终批次4对修正后的全部客户端输入重新完成统一检查与四个构建目标。最终后台复查独立绑定同一完整当前产品源码。

首次Office页面尝试等待了错误入口，第二次操作已完成但录制器拒绝公开run查询参数；按实际UI选择任务／草稿，并仅允许准确UUID参数后重新记录。首次TG录制因页面存在空验证码输入而被录制器拒绝；后续要求其值必须为空、所有截图／帧继续遮盖身份输入，任何可见私密能力仍拒绝录制。失败截图与原记录保留，不算当前通过。

## 媒体与清理

[当前媒体画廊](../media/batch-round-157/README.md) 包含准确配置试运行登记、Office第二版、已永久放弃恢复的旧草稿和准确TG结果片段审核。MP4为实际页面像素按时间采集的无声录屏；GIF为每张真实截图停留2秒的导览，不能称为连续操作录屏。

所有数据合成；不录入真实业务资料或密码、MFA秘密。每次浏览器上下文／页面、自建Vite、临时HTTP与独立原生PG均在检查后关闭；用户原有进程与PG100/API保留。禁用Docker／WSL／虚拟机／模拟器。

## 未执行与保留

| 编号 | 状态 | 说明 |
| --- | --- | --- |
| R01 | 保留历史 | 最后完整后台回归121、完整旧客户端136及此前所有业务／媒体证据保持原来源；不声明在157重跑。 |
| R02 | 保留八个冻结版 | rc.8／APK154继续固定业务153／结构113，不含155–157。当前Android JS通过不会更新该APK。 |
| R03 | 继续开放 | 真实Codex、八项Skill实际评估／安装、TG Bot／群、OSS＋R2、Linux隔离／CI、生产PG、正式Android签名及两小程序配置／发布。资料按用户回复后续提供。 |
| R04 | 继续开放 | 复杂Office／版式、Office合同正文采纳、未知／不确定／复杂派生对象清理、公司完整规则和四端完整正常／失败／冲突业务旅程。 |
| R05 | 用户移出 | 真机、网络性能／实网120GB专项、历史迁移／容灾、AI费用控制不作为门槛；普通文件恢复、正常结构升级、公司财务及AI权限／隔离／审计保留。 |

完整23项见 [编号表](../delivery/05-functional-closure-157.md)。本轮本地结果不构成生产就绪或全Goal完成。

## English

The full declared code wave precedes unified checks and batch debugging. Current local evidence covers selected native HTTP/PG and actual Office binary flows,494 Web cases,555 shared-mobile cases,60 runtime cases, the draft catalog/types, Web/two miniapp builds and Android JS compilation. Final evidence binds actual source bytes and preserves every failed or interrupted attempt.

Recordings contain real authenticated synthetic UI pixels and no audio. GIFs are disclosed screenshot tours. All owned browsers, tabs and transient servers/databases close after checks. Existing rc.8/APK154 remain immutable153/113 artifacts; all23 packages, eight draft Skills and actual integrations/formal acceptance remain open.
