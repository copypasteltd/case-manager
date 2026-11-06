# 增量161：完整代码波次后的统一收口 / Complete code wave and unified closure

本轮先完成C01–C08全部声明实现、接口、普通结构升级、四端共享调用、运行器工具与用例，再建立不可变代码记录，最后统一检查与集中修复。此流程适用于后续每轮；不能把逐模块开发、测试、发版循环称为整轮完成。未提供的公司规则、真实配置和不支持的更广功能继续明确列出。

The entire C01–C08 application/contract/schema/client/runtime/fixture wave precedes the first test batch. Original failures remain recorded; fixes are grouped and affected checks are replayed. Completing this declared wave does not complete all23 work-package acceptances. Current scope remains [revision99](../delivery/04-current-scope-and-acceptance.md).

| 编号 | 本轮实现 | 使用与限制 |
|---|---|---|
| C01 | 普通任务参与人、完成／受阻资料，四组完整个人任务队列 | 当前有效项目成员，最多50位；参与关系不授予权限；未分配队列仅列可编辑且可安排的任务 |
| C02 | Web父任务、里程碑、前置任务完整分页 | 每页20项，明确继续读取；保留尚未读到的原关联，保存时重查，不默认选择首项 |
| C03 | 共享移动端历史内容恢复，Android重扫／例外申请，共享独立复核入口 | 固定历史源→独立复制与校验→本人正式提交新版本；例外不代表扫描通过；申请人不能审批本人例外 |
| C04 | 正式Android签名与递增版本工程入口 | 私有配置及密钥位于仓库外；口令只用受限环境变量名；受控构建互斥；APK+AAB签名、内容、包名及版本独立检查；没有真实签名产物 |
| C05 | TG一次性固定任务更新预授权 | 当前仅update_task；本人独立项目会话完整核对固定字段，绑定群／主题／渠道版本／本人绑定版本／真实近期MFA／运行配置；原群本人准确触发后才排队 |
| C06 | 秘书执行既有普通跨项目／分类文件计划 | 公司私人会话，人工先建立完整普通计划并核对所有可见性变化；模型不能修改源／目标／分类；普通执行后组织授权版本递增，旧运行立即终止 |
| C07 | 秘书将准确已完成结果片段加入原群通知队列 | 仅本人既有TG人工转接结果，重新核对全部来源AI政策；本人选择最多2000个Unicode字符并明确公开；队列回执与送达状态分开 |
| C08 | 接口、数据库约束、运行器工具、八项draft包、双端界面、合成用例及留证工具 | 新建代码全部先写齐；逐包真实模型／原生运行环境安装与正式验收仍开放 |

## 准确使用路径 / Exact ordinary workflows

任务：工作台“任务完整待办”→选择本人／逾期／参与／可安排队列→完整分页→打开准确任务→核对参与人和关联→普通版本提交。查询、打开或读通知均不完成工作。历史参与人因账号变化暂不可用时保留事实；变更参与人需重新核对当前成员。

历史内容：准确文件→历史版本→“恢复为新版本”→本人确认固定来源／当前基础及MFA→读取本人恢复队列→复制、校验到ready→再次明确提交→准确新版本回执。原件和原历史不覆盖，失败／冲突不制造成功记录。Android上传详情可查固定扫描报告，重扫同一源或按现行制度申请；复核人在“扫描例外复核”独立办理。

TG写：先在本人项目私人会话选择project-update并核对完整update_task指令→选择准确原群和本人有效绑定→近期MFA及明确授权→生成最多10分钟的一次性固定指令→本人在原群／主题发送→服务端验证所有原依据并一次性排队→在本人入口看实际执行结果。实际业务渠道始终telegram，不伪装Web；必须显式发布相应TG分类政策。其余七类任务仍保留本人入口转接；本功能不开放任意模型写入，也不自动把结果发到群。

范围变更：先用普通文件范围计划审查完整源／目标／路径、新名称、全部条目、历史、阻塞与权限得失→公司私人information-organize会话输入准确计划→核对全量影响、近期MFA并明确授权→运行器propose_scope_transfer只预览，commit_scope_transfer才调用普通准确执行。提交后保留普通操作、工具和终态回执，组织授权+1；不刷新旧run的权限戳，也不接受其后续工具或自报模型成功。准确工作租约仅可核对既有终态。读取执行结果只返回当前可读目标及最小回执，隐藏旧可见性清单。

固定通知：先完成原TG门户任务→读本人准确结果及原群／主题→选择固定片段并明确公开→加入information-organize请求→propose_notification预览→send_authorized_notification调用原普通披露命令并写入既有reviewed_result投递账本。执行前重查当前权限、原人会话、内容摘要及全部来源AI政策。pending、sent、failed和unknown继续按既有账本处理；不因入队成功声称送达，不自动重发unknown。

## 工程约束 / Engineering constraints

当前结构127，只追加124–127；保留1–123字节。schema123升级124–127；rc.8/schema113升级114–127；开发PG100升级101–127后才可运行本后端。本轮不修改现有开发PG100/API，也不做历史数据迁移或容灾。

TG写配置由部署者独立安装`telegram_write_configuration_sha256`，可由当前profile描述器计算，禁止原地修改运行配置；新配置需新profile。原只读TG配置不自动拥有写入能力。所有模型调用保留原始telegram渠道、固定原人会话、组织／用户版本、范围和一次性激活证据。

正式Android：`python app/scripts/build_android.py --release-config <仓库外私有JSON>`，并提供HTTPS运行配置。私有JSON字段为keystore、alias、store_password_env、key_password_env、certificate_sha256、version_code、version_name、previous_version_code。两个口令字段存环境变量名，且仅接受CP_ANDROID_SECRET_前缀；实际口令不写命令、仓库或证明。配置必须是实际独立签名身份，不能使用示例或自动生成正式密钥。先验previous_version_code由部署方提供，不能当成已核对商店历史；本地证书版本账本防止本受控入口重复版本。实际签名、目标原生兼容与平台接受仍待提供配置。

Eight draft Skills remain draft. information-organize1.3.0 and project-update1.10.0 change package identities; shared reference schema changes require fresh verification of all affected installation digests. No real model, cloud, Bot or platform is invoked by local fixtures. The native confinement proof in selected controller cases is explicitly controlled evidence.

## 证据与当前交付边界 / Evidence and delivery boundary

检查详情、首轮失败、集中修复与当前／保留结果分别记录在[本轮验证](../quality/161-batch-functional-verification.md)。全23项未验收编号见[收口台账](../delivery/05-functional-closure-161.md)，实际界面与录屏见[素材](../media/batch-round-161/README.md)。8个既有冻结版本均保持不可变；APK154仍为业务153／结构113，不包含155–161。本轮没有新APK，也不把本地通过当作生产上线。

最终集中修复 / Final grouped repair: H5历史恢复队列只在读取完成后挂载，避免重复保留加载占位；mobile6验证一个当前队列。类型及两个小程序构建在batch13重跑，后端及Web的准确字节与原检查一致，分别保留原执行及录制工具来源，不制造新的回归执行记录。
