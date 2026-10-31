# 增量138 · 已完成、未提交上传的受控清理

本轮补齐新文件和替换版本两类人工上传：内容已经完成上传、甚至已经复制成正式候选，但用户没有提交为正式文件。目录中的管理员现在可以盘点、选定一个上传会话、建立固定清单，观察后明确批准，再查看每个对象的执行回执。首冻和第二候选保持不可变；这是 main 上的后续源码，结构104，不包含在现有 rc.2 下载或 APK137 中。

## 使用流程

1. 以获授权员工登录 Web、微信、抖音或 Android，进入具体资料目录，展开“已完成上传草稿”。需要当前目录读取和 `file.retention.manage`；替换上传还要核对当前正式文件的保留管理权限。秘书与非人工渠道不能操作。
2. 查看新文件／替换上传、上传修订、已知对象数、保留截止时间和保护原因。盘点包括上传原件及这个会话记录的所有已完成晋升候选，不只处理最后一个候选指针。
3. 选择一个合格候选并填写清理原因，建立固定目录修订、上传修订和来源摘要的观察清单。一次最多64个对象；每次只选择一个上传会话。来源不明、超出数量或状态不确定时继续保护。
4. 完成至少24小时观察期，在初次批准窗口内核对不可逆影响、填写依据并完成近期 MFA。服务器重新核对目录、替换文件、来源、制度、会话、作业及保留约束，再原子写入批准、永久对象屏障、永久上传停用标记及执行任务。批准不会自行变成云端成功。
5. 独立原件清理 worker 使用既有专用用途身份按固定来源逐项处理。失败可受控重试；原授权失效后，可明确重新授权全部停止的剩余项。已经确认的对象不重复执行。停止清单只停止剩余操作，上传停用标记和已确认结果永久保留。

上传队列显示“已批准清理，不可继续提交”，隐藏继续／正式提交入口。服务端和数据库同时拦截后续提交、上传会话改写、新晋升和上传尝试；旧 worker 不能通过重新生成目标键恢复这次上传。需要再次上传时重新建立会话。

## 来源与保留约束

| 编号 | 约束 | 实现含义 |
|---|---|---|
| U138-01 | 新文件使用目录身份 | 尚未提交时没有正式文件行，不能拿目录 UUID 冒充文件；替换上传保留真实文件及当前修订 |
| U138-02 | 完成观察时间不可变 | 上传首次固定来源、晋升首次完成时由数据库记录；已有旧记录在正常升级104时以升级时刻开始保守观察，不推断历史完成日期 |
| U138-03 | 保留期取最新已知完成 | `draft` 已发布制度的保留时长，从原上传及全部完成候选中最新的观察时刻计算；还须超过上传有效期加最大签名有效期 |
| U138-04 | 已知完成集合固定 | 精确上传尝试键、原生来源、晋升文件／版本键、原来源摘要、目标位置、完成时间和候选指针均参与核对；未完成或不确定晋升继续保护 |
| U138-05 | 写入静止 | 活动作业、最近仍有会话作业更新、其他不确定上传尝试、秘书恢复来源、正式对象重合、其他会话复用及有效保留约束阻止建单／执行 |
| U138-06 | 云端身份不外露 | 客户端只拿对象角色、次序、结果和修订，不拿桶名、对象键、原生来源、会话凭据或清理能力 |
| U138-07 | OSS／R2要求沿用132 | OSS必须有非空原生版本；R2必须显式证明独占不可变受管目录，不声称 R2 DeleteObject 支持 CAS |
| U138-08 | 不制造容量或业务事实 | 这些会话已过期，逻辑预留本来已退出；没有正式版本计费事实可释放，不新增或删除正式文件、费用、资金或历史元数据 |

原有历史正式原件、已正式提交上传草稿继续走文件入口；本轮的替换上传清单只进入目录入口，避免在文件历史清理工作台混入没有正式版本的对象。同一供应商可有多个对象，客户端逐项显示“上传原件／晋升候选原件”和次序。

## 接口与代码边界

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/v1/folders/{id}/completed-draft-retirement-candidates` | 当前人有权查看的目录候选，分页游标绑定身份和目录 |
| GET | `/v1/folders/{id}/completed-draft-retirement-plans` | 目录清单历史，再逐份核对当前权限 |
| POST | `/v1/folders/{id}/completed-draft-retirement-plans` | 固定 `expected_folder_version`、`expected_upload_version`、`target_id` 和 `reason` |
| GET／POST | 既有 `object-retirement-plans`、`object-retirement-items` 路径 | 复用详情、批准、取消、重试、重新授权、授权历史和不可变回执 |

新增普通迁移 `0104_completed_upload_retirements.sql`；已应用的1–103保持字节不变。`object_retirement_plans` 增加目录作用域，允许尚无文件身份的清单；`uncommitted_draft` 支持1–64项，旧历史原件仍严格两项、已提交草稿仍一项。共享客户端保持文件和目录两种独立的外部范围核对，不能因放宽对象数而让旧文件清单接受目录身份。

部署前正常升级到104。当前个人开发库仍100，现有用户进程不自动重启或升级。全新实例正常应用1–104；旧103实例应用104后，旧已完成来源开始新的保守保留期。原件清理继续使用独立 worker、既有用途身份和配置，不把删除权限交给普通 API 或员工客户端。

## English implementation and usage

Directory-scoped retirement now covers both new-file and replacement human uploads whose completed originals have not been formally committed. The inventory pins the original upload plus every known completed promotion candidate, including older candidates. New uploads have no formal file identity; replacements require current file authority as well as folder authority.

Open a specific folder, expand **Completed upload drafts**, select an eligible upload and provide a reason. A fixed plan binds folder/upload revisions and source evidence. After retention and at least24 hours of observation, a human with a live session and recent MFA may approve. Approval atomically creates permanent object barriers, a permanent upload fence and independent worker jobs. Retry, cancellation, exact remaining-item reauthorization and immutable receipts reuse the existing retirement workflow. Cancellation never enables the upload again.

Schema104 records immutable first-observed completion times. Existing completed rows conservatively start at the upgrade clock; new rows use their actual first completion observation. Retention starts at the latest completion in the selected set. Active/recent jobs, uncertain transfers, agent recovery sources, formal/shared objects, holds, unavailable cleanup backends or missing native evidence remain protected. At most64 exact objects may enter one plan. OSS requires native versions; R2 requires an explicitly attested exclusive immutable managed namespace. No R2 delete-CAS claim is made.

The change preserves formal file heads/history and creates no artificial quota credit: these expired sessions already stopped reserving logical upload capacity. Client responses never expose private storage coordinates. Apply normal schema upgrades before running this backend. Both frozen candidates and their artifact branches remain unchanged; rc.2 and APK137 contain business136/schema103, not this main/schema104 increment.

[实际检查和延期](../quality/138-completed-upload-retirement-verification.md) · [本轮及全部23项](../delivery/05-functional-closure-138.md) · [既有冻结下载](../releases/v0.1.0-rc.2/README.md)。
