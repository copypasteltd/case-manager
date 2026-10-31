# 增量146 · v0.1.0-rc.4冻结交付 / Frozen delivery

按用户要求停止扩展业务，以现有主要功能冻结交付。准确业务源码为 `294bf0509a7c87f63d9c829d5317fe33fb76a6f3`（增量145），正常数据库结构110。新包包含145的采购／报销逐明细文案选择、建议、核对及人工采用；原三次冻结的源码标签、产物与证据不移动。

This release packages the complete existing business source through145. Packaging tools and delivery records live in main146; the source tag and convenience archive fix the exact145 snapshot. It is a frozen functional candidate with explicit remaining integration and acceptance work.

## 1. 交付及使用顺序

| 编号 | 交付 | 内容与使用逻辑 |
| --- | --- | --- |
| F146-01 | Linux amd64服务器包 | 三个Go程序、Web、独立秘书运行器／依赖、八项draft Skill、systemd模板、110项结构清单；按原生部署手册暂存、正常升级、激活、核对就绪 |
| F146-02 | 微信／抖音小程序ZIP | 两个平台分别构建；员工查阅项目／文件／状态及审批；接入真实AppID、合法域名和HTTPS API后重建、提审 |
| F146-03 | Android开发APK | 全部当前共享业务模块及原生上传桥接；含145新增页面逻辑；当前为本地API／开发签名，正式签名发布仍待完成 |
| F146-04 | 完整应用源码／开发文档ZIP | 指定145提交全部app源码、测试、锁文件、110项迁移、需求／开发／使用文档；逐项对应Git原始字节 |
| F146-05 | 完整演示媒体ZIP | 同一145提交全部docs/media截图、录屏和GIF；本轮复用原媒体，不重录或修改旧证明 |

源码与媒体ZIP可解压到同一个新目录。源码ZIP不重复装入大型历史 `docs/quality/evidence`；完整仓库及全部原始证明使用 `git clone --branch v0.1.0-rc.4 git@github.com:copypasteltd/case-manager.git`。本轮146封装脚本与发版说明位于main，不能声称属于先前145构建输入。

从[下载页](../releases/v0.1.0-rc.4/README.md)取得包及SHA256SUMS；先核对压缩包摘要，再用可信源码 `app/deploy/release/bundle.py inspect` 核对服务器内层清单。安装见[原生发布手册](../operations/03-native-release-runbook.md)。开发PG仍100，运行新后端前正常升级101–110；新安装应用1–110。本轮不升级或替换既有开发服务。

移动产物明确使用 `http://127.0.0.1:18080`，部署前设置实际HTTPS API及平台身份并重建；不会自动连接公司服务器。凭据、数据库、依赖安装、缓存、私有签名材料不进入Git或便利归档。

## 2. 主要功能与操作逻辑

| 编号 | 模块 | 操作顺序 |
| --- | --- | --- |
| M146-01 | 账号、项目、任务与交付 | 登录／MFA→按当前角色进入项目→创建任务／里程碑→履行与交付→结项／归档 |
| M146-02 | 合同、采购、报销及审批 | 建立准确范围草稿→核对资料／版本→送审→分步审批→采购／合同履行；退回与冲突保留准确版本 |
| M146-03 | 费用、现金、退款与订阅 | 人工登记实际费用→实际收付款→核销／退款→汇兑、分摊和报表；审批不生成现金记录 |
| M146-04 | 文件、版本、正文与博客 | 上传→校验／扫描→明确提交→版本预览／检索／关联；Office创建编辑、历史恢复及清理遵守独立权限与保留规则 |
| M146-05 | 工作台、通知及服务状态 | 查看本人业务／资源／存储待办→按权限处置→刷新核对持久回执；已读不消除真实故障 |
| M146-06 | 秘书及TG业务代码 | 会话选择资料／Skill→核对固定计划→授权执行→查看结果和审计；文案建议需人明确采用，真实Codex／Linux／TG仍待接入 |

145的行文案入口：秘书会话→OA请求→选择采购／报销及准确已有明细→起草→逐项核对原文／建议→人工采用；金额、数量、供应商、凭证、顺序和未选行保持原值。采用只保存普通业务修订，后续审批、下单、记账、付款仍单独执行。见[功能说明](145-oa-line-prose.md)。

## 3. 本轮检查与明确延期

服务器／Web／秘书运行器、两小程序、Android开发包重新构建；检查实际包结构、签名、内嵌JS及源码映射、完整源码字节、归档摘要和私密材料。精选三组原生HTTP／耐久PG主流程；完整回归、额外浏览器／移动UI旅程不重跑。Android原生任务若命中缓存，保留实际任务状态与原XML时间，不记为新执行。

145定向原生／运行器／Web／移动逻辑与真实页面录制按原出处复用；136完整客户端和121完整后端回归保持历史身份。实际条数与文件摘要见[本轮核验](../quality/146-frozen-release-verification.md)。所有自建临时服务清理，禁止Docker／WSL／虚拟机／模拟器，不操作用户原有进程。

仍23个工作包（14 P0／9 P1）和八项draft Skill。真实Linux、生产PG、OSS＋R2、Codex、TG、平台身份、正式Android签名与完整公司规则／旅程／验收保留。真机、网络性能／实网120GB专项、历史数据迁移／容灾、AI费用控制继续移出；不把取消或延期记为通过。

GitHub Releases API此前返回403、现有凭据账号被停用；本轮保留该原始事实，不重复调用相同凭据。源码标签、五项产物通过已授权SSH推送至独立产物分支，固定Git提交链接下载；Releases网页未创建。没有执行生产部署。

[下载／摘要](../releases/v0.1.0-rc.4/README.md) · [全部23项待办](../delivery/05-functional-closure-146.md)

服务器封包首次发现原构建器的裸Node版本读取用了Windows父进程路径（22.14.0），封存前置检查拒绝该次产物；[失败记录](../quality/evidence/release-candidate-146/attempts/01-server-node-resolution/reason.json)。随后在进程级明确Node22.23.4并重新构建；每项实际构建输入与145 Git字节对应，待提交146发版脚本／文档使整个工作区为dirty，原始标志如实保留，不伪装为干净工作区。
