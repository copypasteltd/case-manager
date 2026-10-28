# 本地原生开发与验证

增量102的本地财务入口：报表页 → 选择汇率版本 → 发布人工汇率版本。实际开发组织不会自动设为HKD或写入演示报价；只有具备权限的员工确认基础币种、区间、来源和口径后才发布。正常升级追加迁移81并保留旧数据，见[开发说明](102-finance-fx-valuation.md)。

日期：2026-10-23。源码位于app/，文档位于docs/。当前实现边界见[实施台账](../delivery/02-implementation-ledger.md)。以下为Windows本地开发流程，不能用开发服务托管生产。

## 1. 工具与路径

本地已验证Node22.14.0/npm10.9.2、Go1.27.3、PG18.6；JDK21与Android SDK可读，未连接Android真机。Go/PG在app/.tools/，数据/随机开发凭证在app/.local/，均已忽略。

禁止调度Docker、WSL、虚拟机或设备模拟器。中文工作区下PG使用Windows DOS路径别名，实际目录仍为app/.local/postgres；这不是虚拟化。脚本只建立未占用的盘符，记录归属，停止本集群后仅移除完全匹配的自建别名；不改动用户已有别名/其他PG服务。

Go原生下载与官方SHA校验脚本为app/scripts/bootstrap_go.py；PG二进制来自EDB官方发布。工具版本升级须重新核验与跑实际检查。不要提交.env、.local、密钥、签名文件或生成二进制。

## 2. 已安装工具后的启动

以下命令均从仓库根目录执行：

```powershell
python app/scripts/dev_database.py start
python app/scripts/run_backend.py migrate
python app/scripts/seed_development.py
npm ci --prefix app
npm run contract --prefix app
```

seed只允许本机development且空组织，创建3个随机口令的开发身份：system.admin、project.manager、project.member；重复执行不会覆盖已有组织。口令只在忽略的app/.local/development-accounts.json中，本工具不打印口令。系统管理员要求MFA，项目权限是另一个角色。

分别运行两个原生前台进程：

```powershell
python app/scripts/run_backend.py api
```

```powershell
npm run dev --prefix app
```

Web为http://localhost:5178；本地API配置为127.0.0.1:18080，Vite代理/v1和/health。启动前确认端口归属，不停掉其他项目进程。API环境不传迁移连接；生产API若收到迁移凭证将拒绝启动。

停止API/Vite用所属终端中断；停止本开发PG运行python app/scripts/dev_database.py stop。不要按进程名批量停止node、postgres或浏览器。

## 3. 生产初始化边界

生产服务器、域名、TLS、独立PG、OSS/R2、平台与模型配置尚未提供，当前未部署。迁移用单独身份/连接；API用最小权限应用身份。生产环境禁止迁移凭证进入API进程。

platform bootstrap --organization … --login … --name …只接受标准输入中的初始密码，创建单个系统管理员，不赋予业务角色。生产密码经受控管道输入，不能放在命令行、日志或Git。已经初始化的组织会拒绝再次bootstrap。

base_currency未业务确认时保持未设置；正式财务流程不得自行默认为HKD。模型、Telegram、云文件能力在未完成适配/验证前不假装成功。

## 4. 验证

```powershell
python app/scripts/test_backend.py -v
python app/scripts/verify_backend.py
npm run test --workspace @copypaste/web --prefix app
npm run build:web --prefix app
node app/scripts/check_web.mjs
node app/scripts/check_administration.mjs
node app/scripts/check_projects.mjs
node app/scripts/check_knowledge.mjs
node app/scripts/check_files.mjs
```

后端脚本仅连接显式本机测试管理身份，创建cp_test_前缀随机数据库，应用仍使用非特权连接。清理前校验固定前缀/名称长度；只删除自己创建的数据库。未传CP_TEST_ADMIN_URL时数据库测试会标记跳过，不能据此称集成测试通过。

浏览器脚本要求前述API/Vite运行且开发身份存在，使用本机原生Edge，不连接个人浏览器。它向开发库创建带「开发验证」前缀的项目/任务；不清空业务表。截图和脱敏结果在docs/quality/evidence/web/；finally关闭全部自建上下文及浏览器。

管理、项目协作和知识脚本分别保存在对应evidence子目录，使用带验证标记的合成数据。知识脚本验证固定修订发布、评论版本、冲突保留、只读隔离、历史恢复、主题和移动导航焦点。verify_backend.py保存脱敏结果、源码摘要及执行期间源码是否变化，完整原始输出仍只在忽略目录；报告中的父测试和子场景不能重复计数。

文件脚本将真实接口创建的目录与本地元数据夹具分开标记。prepare_storage_browser_fixture.py仅接受本任务本机开发库、匹配前缀的项目/目录及实际开发创建人；拒绝生产。它不创建任何云对象，不把扫描或副本伪造为已验证。120GB仅为数值与布局样本；文件结果和截图保存在docs/quality/evidence/files/。

上传控制接口和原生worker-upload已接线，但开发环境默认禁用云存储，未启动真实云执行器。用途身份、单独进程环境及运行方法见[上传执行器文档](12-upload-worker-and-cloud-identities.md)。固定源摘要、恢复检查点及扫描/重扫已有[独立执行链](13-upload-integrity-and-scanning.md)，默认扫描器关闭会阻断。[正式候选/提交](14-file-promotion-and-commit.md)、[扫描例外审核](15-scan-waivers-and-review.md)、[双云副本](16-cross-cloud-replication.md)及[下载授权](17-download-authorization-and-streaming.md)已有隔离实现；[Web 上传及审核页面](18-upload-queue-and-review-clients.md)已接入，[历史内容恢复](19-history-restoration.md)提供独立恢复执行器、任务/提交与Web界面，真实云、移动端与真实120GB传输仍需继续完成，不以元数据按钮或单元流测试充当上传可用证据。

中断后的PG可能需要恢复。启动脚本只启动一次本任务集群，再轮询同一个进程的就绪状态；等待超时不代表进程停止，不会因此再次启动。status区分「进程在运行」与「数据库已就绪」；应先查实际状态和本集群日志。

单元/集成/浏览器通过只代表对应代码分支；真实120GB、2TB迁移、双云恢复、平台审核与真机、香港链路、性能和完整业务验收均需独立证据。

审批制度的隔离测试可执行 `python app/scripts/test_backend.py -run '^TestWorkflowDefinitions$' -timeout=3m -v`。`node app/scripts/check_workflow_definitions.mjs` 依赖已运行的 API/Vite 和现有管理员 MFA 记录，填写、排序后取消示例表单，不发布任何公司制度；记录保存在 `docs/quality/evidence/workflow-definitions/`，finally 关闭全部自建浏览器。该制度表单检查不构成申请执行证据；申请检查有独立脚本及隔离场景。

往来资料检查：`python app/scripts/test_backend.py -run '^TestBusinessParties$' -timeout=3m -v` 运行隔离数据库场景。`node app/scripts/check_parties.mjs` 创建明确标记的虚构项目与供应方，使用临时项目范围角色，真实操作修订/冲突/归档/恢复；结束撤销临时授权并关闭全部自建浏览器。协议与边界见[往来资料](22-business-parties.md)。

## 5. 契约与样式

修改app/scripts/build_openapi.py后先生成docs/development/openapi.json及后端嵌入副本，再执行npm run contract --prefix app。禁止手改生成的api.d.ts。新增接口先补精确Schema，再实现确定性业务约束。

docs/design/tokens.json和tokens.css为已评审风格源；python app/scripts/sync_design.py同步到app/packages/design和品牌资源。Web引用真实官网标识，不通过模型重绘Logo。当前功能不需要额外位图素材，尚未调用图像生成。

上传/审核客户端检查：在原生 API/Vite 运行且已有开发账号及管理员 MFA 记录时，于 app/ 执行 node scripts/check_uploads.mjs。它不重新初始化账号；准备专用前缀项目/目录内的元数据会话，只为当前测试浏览器的管理员会话调旧 MFA 时间以验证再认证；不会发布有效例外制度、创建扫描报告或访问云。结果与截图位于 docs/quality/evidence/uploads/，所有浏览器在 finally 中关闭。实现协议见[上传队列与审核客户端](18-upload-queue-and-review-clients.md)。

历史恢复客户端检查：原生API/Vite运行时，于app/执行 node scripts/check_history_restores.mjs。只为专用前缀项目/目录建立不可用元数据及排队任务；无对象、扫描报告或复制任务，不伪造ready候选。检查真实取消保留历史、其他用户隔离与双主题五宽度，结果位于 docs/quality/evidence/history-restores/；浏览器finally关闭。worker-restore-small/large需要独立用途身份，当前未启动真实云执行器。

保留界面检查：在已有开发账号及管理员 MFA 记录的原生 API/Vite 环境，于 app/ 执行 node scripts/check_retention.mjs。只为专用元数据项目临时绑定保留角色，通过真实页面建立/释放保留锁并留存评估，finally 撤销授权且关闭所有浏览器。不会发布公司保留制度或访问云；详见[保留与引用保护](20-retention-and-reference-protection.md)。

合同草稿使用 `node scripts/check_contracts.mjs`，在虚构项目内临时分配 contract_manager 和 party_manager，验证 nullable 草稿、资料选择、固定名称、修订历史、并发冲突、补充协议及回收恢复；结束撤销两项授权并关闭全部浏览器。正文正向引用来自隔离 PG 的受控提供商测试，真实浏览器只验证无可选文件分支。开发库已应用至 0030，实际全量证据以报告时间及源码摘要为准。

申请检查：于 app/ 执行 node scripts/check_requests.mjs，使用虚构合同和三项临时项目授权，验证草稿/缺项阻断/明确刷新/并发冲突/修订历史/权限隔离；不会发布公司制度或伪造正文。结束撤销授权并关闭全部浏览器。正向两人审批执行来自 TestSequentialApprovals 的 17 个隔离 PG 叶场景。详细协议见[申请快照](24-request-snapshots-and-sequential-approvals.md)。

采购/报销检查：于 app/ 执行 `python scripts/run_spend_browser.py`，脚本拥有独立原生测试库、临时 Vite 与 Edge 上下文，验证实际录入、准确金额、审批、退回修改重提撤回、疑似重复、归档和撤权。夹具的 `CP_BROWSER_MOBILE_FIXTURE` 只是既有受控业务夹具名称，不会调用模拟器。结果位于 `docs/quality/evidence/spend-documents/`；脚本结束撤销临时权限、关闭全部所属浏览器和服务并销毁夹具。详见[采购报销协议](33-purchase-and-claim-approval-drafts.md)。

两种小程序编译使用 `python scripts/verify_mobile.py`，明确指定本机测试 API，不推断生产端点。脚本将输出写入忽略的本地日志，记录命令完成/超时、来源摘要及结果；超时停止自己启动的进程树。原生后端完整回归、小程序编译和多浏览器夹具宜顺序运行；本机已出现并行大编译期间临时数据库清理超过45秒的情况，须确认清理并重跑，不能据业务断言通过忽略套件失败。完整回归期间不修改其纳入摘要的源码。

合同签署正向浏览器链在 app/ 运行 `python scripts/run_business_browser.py`。该脚本显式启动 TestContractSignatureAndActivation 的本机隔离数据库，保存仅供此夹具的临时身份到忽略目录，另起 5189 端口 Vite 并运行真实登录/送审/审批/签署/生效。原生开发 API 与 5178 工作空间保持独立。脚本 finally 请求夹具正常释放，使原生测试清理自建数据库，关闭全部检查浏览器并停止所属 Vite；结果记录真实清理退出码。它使用受控正式文件提供商，不访问真实云、不启动 Docker/WSL，也不向开发公司发布制度。残留夹具文件须先检查所属进程，不覆盖正在使用的文件。


签署浏览器链现包含合同收付款计划编辑、错误合计保留、准确审批快照及生效自动节点；仍通过自建原生数据库和独立 5189 Vite 执行。开发 API 已加载0030迁移，不向开发公司发布测试审批制度。协议见[合同计划](26-contract-payment-schedules.md)。

采购履行检查：在 app/ 运行 `python scripts/run_spend_browser.py --fulfillment`。沿用独立原生业务夹具，验证订单、通过/拒收、超收输入保留、取消余量、双会话冲突、准确来源退货与取消后的退货。临时授权、Edge、Vite和数据库均由脚本清理。结果位于 `docs/quality/evidence/purchase-fulfillment/`；协议见[采购履行](34-purchase-orders-acceptance-and-returns.md)。


实际费用检查：在 app/ 运行 `python scripts/run_spend_browser.py --costs`。使用同一原生隔离夹具，验证不完整草稿、固定供应方/凭证、双会话修订冲突、不平衡拒绝及准确确认、采购/报销/生效合同共同关联、全额冲销、历史及月度原币分摊钻取。记录位于 `docs/quality/evidence/actual-costs/`，源码在检查期间必须保持一致；失败或中途改码后重新运行才可形成当前通过证据。脚本负责临时角色、全部自有浏览器/标签、Vite及数据库清理。协议见[唯一实际费用](35-canonical-actual-costs-and-incurred-reports.md)。
