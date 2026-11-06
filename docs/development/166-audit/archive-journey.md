# J-10 归档／恢复源码核对 / Archive journey source review

审查对象：增量165提交 `ddf6a70cc2de5fba5d01f64d581c4a228052c1d7`。本报告是单需求的只读源码审查，没有运行测试、数据库、构建或浏览器，没有改动应用、旧迁移或历史证据。结论为 **partial**：主归档链已有完整实现，但订阅取消生效日在项目结项核查中采用错误时区。以下不是全项目或正式业务验收。

## 规范与可证明范围

- [产品旅程](../../requirements/01-product-and-journeys.md)（第45行） J-10：核对任务、验收、合同费用后归档；只读、受控恢复、不隐式停订阅、不删除原件、新操作留历史。
- [项目需求](../../requirements/03-projects-and-knowledge.md)（第11行） PM-05及23–29行：未生效的订阅取消不能默认为完成；缺少权限不能显示为零；准确版本／完整依据、人工近期MFA、固定附件、同事务复盘与收据；重开保留旧历史。Q-18未决，不实现豁免。
- [结项协议](../44-project-closeout-and-archive.md)（第7行）第1–4节：十三类核查、明确生效取消、当前依据和历史阅读；[移动协议](../79-mobile-project-closeout-and-archive.md)（第9行）接入相同原始状态机。
- [订阅协议](../37-subscriptions-billing-and-forecast.md)（第15行）明确IANA账期时区，23／27行规定未来取消待生效、归档前已生效。香港的界面展示时区不替代订阅自身日历。

| 子要求 | 源码证据与已读调用链 | 裁定 |
|---|---|---|
| 十三类来源和完整权限核查 | `project/closeout_reads.go:19–128`调用 `project_closeout_readable_groups`、`project_closeout_snapshot`；最新 `0122_closeout_refund_units.sql:46–89`列入十三类，重用合同／采购核查及准确费用／退款本金；`0046_project_closeout_and_archive.sql:70–86`按类和实际对象重新授权 | 在已读路径 implemented，仍不等于公司财务口径或全部真实样本验收 |
| 不完整、旧依据及阻塞不归档 | `project/closeout_mutations.go:25–106`核对版本、指纹、完整可读、阻塞及固定文件；`0046...sql:96–115`重新计算相同完整依据、核对实际近期会话MFA及延迟一致性 | 主链 implemented；订阅时区偏差另见F-166-J10-01 |
| 归档不制造费用、不隐式停订阅／合同、不删原件 | `ArchiveProject`完整25–153行只写 `project_archives`、固定附件和项目状态，再输出原子普通事件；`pin_project_archive_file`在0046:118–123建立保留引用 | 已读归档写集 implemented；没有据此证明所有云对象生命周期 |
| 普通项目写入只读与受控重开 | `project/tasks.go:50–55`统一拒绝closing/archived/cancelled；项目资料更新 `service.go:283`；交付变更调用同一 `projectWritable`。`service.go:352–438`拒绝旧archive transition、重开独立 `project.reopen`、原版本、原因、新项目版本及普通事件 | 已读普通项目路径 implemented；未穷举每个后台财务纠错入口 |
| 历史依据保留及当前授权 | `closeout_reads.go:129–260`同时核对当时分类、当前源权限／共享分摊，限制后隐藏完整历史；`0046...sql:122–127`历史与固定附件禁止修改／删除 | 在已读路径 implemented |
| Web及共享移动消费者 | `web/ProjectCloseout.tsx:44–110`读取同一预览／历史、独立复盘与MFA／重新核查；`mobile/.../closeout.tsx:42–233`当前身份重读、十三类、原阶段命令、两项确认／MFA及真实归档回执核对，未另造归档状态机 | 代码存在；本轮未执行页面，四端最终验收 undecidable |

## F-166-J10-01：项目核查用香港日历判定任意订阅取消生效

**源码判定：contradicted，业务影响为错误放行或错误阻塞项目结项。尚未执行反例，应由独立复核和整批用例确认。**

最新定义 `app/backend/migrations/0122_closeout_refund_units.sql:83` 使用：

```sql
s.cancel_effective_on > (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date
```

相反，普通订阅链明确采用自身时区：`subscription_calendar.go:49`接受有效IANA时区；`subscription_reads.go:48–58`以订阅时区求日期，仅无时区草稿回退香港；`subscription_mutations.go:344–399`在取消和订阅归档时复用该日期，未来生效则拒绝归档；`subscription_reads.go:76–79`同样投影 `cancellation_pending`。

可由普通命令构造的反例：在 `2026-11-06T00:30:00Z`，香港为11月6日，`America/Los_Angeles`仍为11月5日。创建该时区的项目订阅草稿，直接取消（普通命令允许draft→cancelled），生效日11月6日，固定供应方已取消状态、核对时间及正式证据；未启用，故无预测账期。订阅本身仍显示取消待生效且拒绝自身归档，项目结项谓词却把其视作已生效，其他条件满足时不再阻塞进入closing／人工归档。反方向在 `2026-11-05T12:30:00Z`，`Pacific/Kiritimati`已为11月6日而香港仍为11月5日，也会额外阻塞已经生效的取消。

已尝试的反证：

1. 项目核查确实允许 `cancelled`，不要求必须先成为订阅自身的 `archived`，所以普通订阅归档的正确守卫不能抵消此处错误。
2. 取消允许draft状态，且不要求已有生成账期；不存在必然阻塞所有此类记录的forecast。供应方固定取消证据与生效日是分别检查的字段，不能用前者代替后者。
3. 归档准备、项目状态守卫、延迟一致性都复用 `project_closeout_items/snapshot`，没有另一个按订阅时区求生效日的后置检查。
4. 按 `FUNCTION project_closeout_items`及 `cancel_effective_on`搜索全部130迁移；0046、0089及0122有该谓词，最新定义是0122。未找到后续替换。
5. 人员位于香港、前端采用香港展示时间，均未禁止创建其他有效IANA时区的订阅；不能据此裁定该普通路径不可达。

整批实现建议：追加迁移，保留1–130原件，将当前项目结项的生效日统一为订阅明确时区；对无时区的可取消草稿使用与普通 `subscriptionDate` 相同的香港回退。避免单纯切成UTC或只改前端文案。前置用例同时覆盖香港日期领先／落后、普通创建→取消待生效→核查／阶段拒绝、已生效取消不额外阻塞、空时区草稿的现有回退，以及无权核查不泄露计数；业务代码及所有前置用例完成后再统一执行。

## 搜索及限制

实际搜索包含：`project_closeout_snapshot`、`project_closeout_readable_groups`、`guard_project_archive_state`、`project_archives`、`projectWritable`、`cancel_effective_on`、`subscriptionDate`、`CANCELLATION_NOT_EFFECTIVE`、全部迁移中的 `FUNCTION project_closeout_items`。初次猜测单数文件名不存在后，通过 `rg --files`定位真实 `03-projects-and-knowledge.md`；没有将不存在的猜测文件当作规范。

读取了上述指定函数／区间，0046的归档结构／守卫／保留引用、0122当前十三类、相关订阅命令与IANA校验，以及Web／移动消费者的阶段、归档和重新核查实现。没有逐条审计所有合同履约／财务公式、所有文件删除／云对象命令、所有AI工具或原生平台运行时；完整公司角色、Q-18、财务制度、真实业务资料及目标配置仍未提供，本报告不补造这些决定。旧测试与媒体未重放，也不将未发现第二个偏差写成J-10全面完成。
