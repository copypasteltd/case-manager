# 增量167：整批功能与部署闭环

基线 `e36ec01`。先盘点23项编号和未定事项，独立审查/反证后合并5处确认的代码差异；完整实现C01–C07的应用、调用方、用例、工具及文档后，才保存整批源码并统一执行测试。失败保留原始来源与日志，集中修复后仅重测受影响输入。

| 代码差异 | 修复和使用逻辑 |
|---|---|
| AC167-01 | 原账号邀请过期后可重新签发。管理员先查看同一待激活成员的当前邀请，核对版本/邀请ID、填写原因、完成最近MFA，再显式重发；旧邀请立即失效，新ID/新凭证有效24小时，不新增重复账号、不重置已激活/停用账号、不授予角色。 |
| AC167-02 | 初次邀请、重发及明文展示均重新查询原人类会话和数据库MFA，锁定会话直到事务完成；鉴权后先完成的登出/撤销使后续操作停止。永久命令/审计/事件回执不存明文，展示阶段另作当前授权检查。 |
| U120-D01 | Web按1MiB连续切片、Android按64KiB缓冲流式读取全部原件，生成统一`source-sha256-v2:<SHA256>`；重选后在恢复前比较完整内容摘要，服务器在冻结对象全文计算后、扫描/推广前比对该预期摘要。新Web/Android不能恢复旧抽样身份；旧记录保留，不重标为完整校验。 |
| DEP167-01 | 生产打包复制四个固定Python工具，包含`telegram_config.py`。在独立目录移除仓库搜索路径运行安装版CLI，验证真正的包依赖闭包；禁用TG也必须能启动控制器。 |
| DEP167-02 | 受控目录增加`original-cleanup`，固定命令`worker-original-cleanup`、独立`copypaste-retire`身份、`original-cleanup.env`与当前发布指针。延续观察期、精确对象版本、原会话/MFA/重新授权和永久屏障，不改变清理业务。 |

普通升级追加`0133_invitation_reissues.sql`，保留1–132所有迁移。邀请记录增加独立撤销、替代链和原因；历史邀请不回填虚构事件。新接口`POST /v1/users/{id}/invitation-reissues`，输入固定`expected_version/invitation_id/reason`，权限继续`user.invite`，拒绝AI/非交互会话。先授权再幂等重放；响应中的凭证只在独立实时核对后附加。

Web入口「成员与授权 → 查看邀请 → 重新签发邀请」。微信、抖音、Android共享源码入口「我的工作空间 → 管理成员与邀请」，提供分页、创建、查看和重发。邀请码默认隐藏，只在本人显式操作时显示；离开移动页面清除。新成员仍从登录页「接受邀请」设置自己的密码。

完整内容摘要需要先顺序读取一次全部本地原件，耗时随文件大小变化；它没有整件内存缓冲或服务器本地永久副本。浏览器只缓存不可变File对象的摘要，Android仍核对SAF持久授权/来源状态；服务端全文终验阻止原件在读取后发生变化而悄悄提交。完整摘要证明字节身份，不提供杀毒豁免；120 GB文件仍依照既有不可扫描政策及人工豁免/拒绝流程处理。本轮不做120 GB传输性能专项。

旧`web-sample-v1`/`android-saf-v1`记录继续可见，当前客户端要求取消旧上传后以完整身份重新创建。既有Office受控候选继续使用精确`office-artifact-v1`，不改变候选生成/扫描/人工提交规则。普通旧协议的历史云完整性机制保持，不宣称抽样记录已升级。

部署：按[当前原生运行手册](../operations/03-native-release-runbook.md)安装完整`metadata/release-tools/`目录；启用删除服务时将`original-cleanup`加入`deployment.json.units`并提供固定env文件。使用独立数据库/存储删除身份；观察服务继续独立只读身份。OSS要求非空对象版本；R2仍需显式受控独占命名空间承诺。控制器不自动停止或接管旧手工部署的服务，切换前由部署人员核对重复消费者。真实服务器资料由用户后续提供。

源审查见[账号](167-audit/account-lifecycle.md)、[账号重发反证](167-audit/refutation-account.md)、[原会话反证](167-audit/refutation-account-session.md)、[大文件](167-audit/upload120gb.md)、[上传反证](167-audit/refutation-upload.md)、[财务](167-audit/finance-closeout.md)、[部署反证](167-audit/deployment-refutation.md)。这些是源码证据，执行结果另见[统一验证](../quality/167-batch-functional-verification.md)。

[全部23编号的当前余项](../delivery/05-functional-closure-167.md)。全部八项Skill仍draft；真实Codex、Bot、OSS/R2、平台准入、目标Linux/PG、正式签名及公司制度验收仍开放。当前源码是冻结后开发；九个冻结版本与APK164保持业务163/schema129，不包含165–167，Android JS/新APK继续延期。开发PG100/API保持原样。
