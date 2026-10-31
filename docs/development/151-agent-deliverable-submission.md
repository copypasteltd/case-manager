# 增量151：秘书提交固定交付修订 / Exact deliverable submission

已在Web／微信／抖音／Android共享入口实现人工准确授权的交付提交。此能力补齐已有的创建、修改草稿之后的待验收提交；不自动作出验收决定。

## 使用顺序

1. 进入同项目秘书会话，选择 project-update 运行配置与“提交固定交付修订待验收”。运行配置须由公司另行安装、验证和启用；本地受控配置仅用于合成检查。
2. 选择实际可提交交付，核对基础版本和原修订，选择1–20个不同正式附件版本，填写交付说明及原因。公司级附件及同项目附件沿用正常业务规则；选择不授权读取二进制正文。
3. 核对后勾选确认，将完整提交加入本次请求，再明确发送。任一表单变化都会清除勾选。
4. 秘书只能使用完整六字段原始授权执行。后端重新检查原登录、项目／交付／里程碑、当前受众及文件的普通与AI权限、扫描或例外、复制状态和固定元数据。
5. 实际成功才返回新修订ID、交付版本和操作回执。界面明确待独立人员验收；重复调用复用原回执。新提交可来自草稿、退回或已验收交付，旧修订及验收记录继续保留。
6. 原依据、权限、原登录或运行状态发生变化即失败，不自动刷新授权。验收由原有交付页的独立人员按最近MFA和完整文件核验办理。

## 开发约束

- 新迁移112只增加此工具与准确绑定校验，不改变迁移1–111的字节。
- 原交付提交业务服务被复用。仅准确存储的同一运行／人／会话／原登录／参数可使用原交互登录；实际AI身份、运行ID、普通及AI权限、审计继续保留。没有通用人类身份转换或验收权限。
- Before 固定 submission_files：文件／版本ID、序号、原文件名、SHA256、字节数、媒体类型、扫描状态和例外引用。有效扫描例外保持waived，不表述为clean。
- MCP、受控原生边界、运行观察、Skill输入和完成工具、OpenAPI、四端共享类型与回执验证同时更新。project-update1.9.0仍draft，真实模型和正式Skill验收保持开放。
- 示例只使用原生临时PG及合成对象运输；不使用Docker、WSL、虚拟机或模拟器。原PG100／API不变。

English: An exact six-field human intent fixes the deliverable/base revision, ordered formal file versions, prose and reason. The existing business command creates one immutable submitted revision with atomic audit and a replayable receipt. The original live session is used solely for this stored intent; the actor remains an AI run. Current audience/file permissions and metadata are rechecked. Human acceptance remains separate and MFA-bound. Real model/Skill, cloud and production adoption are not established by these local proofs.

参见[检查与限制](../quality/151-agent-deliverable-submission.md)、[全部待办](../delivery/05-functional-closure-151.md)。
