# 增量162：整轮固定TG项目操作闭环 / Complete project-grant code wave

本轮统一复核23个开放编号后，一次完成C01–C06代码、接口、四端入口、运行器、SQL和用例，再开始统一检查。完整计划在[代码清单](162-batch-code-plan.json)。公司规则、复杂格式、真实服务与正式验收仍按编号开放；不把单轮源码完成等同于总目标完成。

## 使用逻辑 / Workflow

1. 在本人项目私密秘书会话选择并核对既有准确项目操作。支持全部16项：任务新建／资料／状态／前置关系，里程碑新建／资料／状态，交付新建／资料／准确修订提交，风险新建／更新，问题新建／更新，项目资料／阶段。
2. 核对完整固定参数、原群／话题、本人数字身份绑定、近期MFA和原因。建立一次性预授权只保存准确运行与证据，不加入任务队列。
3. 本人在所选原群发送生成的固定指令，现行原登录、权限／分类AI政策、来源、群／话题／绑定版本、部署配置与有效期全部重新检查后才入队。有效期不超过10分钟。普通群消息不补写字段。
4. 模型只可执行原授权的一项工具及全部准确参数；其他项目工具即使安装在同一运行配置也会被控制器拒绝。创建、变更及准确交付提交均复用普通业务命令、原子事务、幂等回执与审计。交付提交不代表独立人工验收。
5. Web的TG入口与微信／抖音／Android共享TG入口可打开“我的固定预授权”，完整分页并按状态查询本人当前有权查看的元数据，跳转准确指令／结果。读取不执行操作；旧权限下的资料不显示。
6. 未触发授权可填写原因并明确取消，复用普通任务取消命令。若触发／执行已经并发推进，按实际版本和取消请求处理，不能声称已完成业务被撤销。群内公开结果仍须独立的原群披露审核。

`pending_trigger`表示目前可核对的待触发授权；`invalidated`表示原登录／群配置／权限或准确业务依据已变化；`expired`表示触发到期；`stopped`表示未触发执行已终止；`consumed`仅表示触发已使用。`run_state`独立呈现真实运行结果。当前依据验证失败不修改不可变授权原文。

## 工程约束 / Engineering boundaries

追加普通迁移128，不改动1–127。TG实际业务渠道保持telegram，原交互登录只用于验证原人类授权；不会把审计或普通权限渠道伪装成Web。交付提交专门重检原人类会话与已激活TG授权，且不授予其他交互身份能力。

运行器／MCP／原生证明三个工具目录均同步至18定义（2项读取＋16项既有变更）；真正每次写权限仍绑定一项准确意图。project-update1.11.0及其余七包保持draft。共享执行边界文档改变全部八包摘要；目标安装须重新核对，现有配置不会自动启用新目录。

清单按不可变建立时刻和ID分页，游标绑定组织／用户／身份及组织授权版本／当前渠道／当前登录／筛选条件。候选失去普通读取权时隐藏且继续推进游标；空页带下一页不表示完整清单为空。返回信息不含会话正文、密码、Bot令牌或模型私有内容。

其他七类Skill沿用既有本人门户衔接；不增加任意收件人、任意群写入、付款、独立验收或生产控制。复杂Office、未知外部对象身份和真实公司制度不通过猜测补足。

## Evidence limits

All declared implementation and fixture work precedes unified native HTTP/PostgreSQL, client, runtime and source checks. Failed attempts and the exact tested inputs are retained; repairs are grouped and only affected checks repeat. Synthetic Web/H5 media masks fixed trigger codes and excludes login/MFA screens. No actual Bot/model/cloud/native-Linux installation, formal signing, new APK or full cross-module acceptance is inferred. Development PG100/API and all eight frozen releases remain unchanged; rc.8/APK154 still represent153/schema113. The current source requires ordinary schema128 upgrades in a separately configured environment.

The complete23-package register remains open (14P0/9P1), with five previously excluded items and eight draft Skills preserved. See [current scope](../delivery/04-current-scope-and-acceptance.md).

当前本地结果：原生31项测试结果、Web522、共享移动550、运行器66通过；类型、Web及微信／抖音构建通过。Web日期显示在媒体复核后补充香港时分秒，仅重跑受影响的类型／Web构建和录制。参见[统一验证与保留边界](../quality/162-batch-functional-verification.md)及[截图／录屏／GIF](../media/batch-round-162/README.md)。

Current source-bound checks passed:31 native test entries,522 Web tests,550 shared-mobile tests and66 runtime tests. The late Web timestamp display repair repeats only typechecking, Web building and the affected capture; unaffected results remain explicitly retained. Recorded cancellation never executes the authorized business command. The synthetic captures establish this local path only.
