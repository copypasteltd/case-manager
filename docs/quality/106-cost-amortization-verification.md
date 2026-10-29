# 增量106：费用摊销验证 / Cost amortization verification

当前源码：后端224个顶层／1355条结果，Web172、共享移动273、运行时55；微信／抖音各20页、Android开发APK及18+4项静态检查、两个Linux交叉编译。26条JVM按实际缓存结果记录。修复移动编译目标对BigInt运算的转换问题后，8项客户端命令重新执行，并新增旧目标编译回归。新增8 PNG／2静音MP4／2 GIF；原199份媒体字节保留。

| 编号 | 范围 | 证据及结论 |
| --- | --- | --- |
| V-01 | 完整后端 | 224个顶层／1355条结果，当前Go／SQL／嵌入合约前后一致。见[完整批次](evidence/cost-amortization-backend/result.json)及[私有日志归档摘要](evidence/cost-amortization-backend/raw-log-archive.json)。 |
| V-02 | 算法与事务 | 闰日、单日、120月、原币精度和逐份额守恒；权限、MFA重放、审计回滚、并发发布、不可变／延迟约束、历史／停用、范围／CSV及冲销报价日。 |
| V-03 | 四端 | Web172／移动273／运行时55；微信／抖音各20页。[8项重新执行](evidence/cost-amortization/client-checks.json)保存命令时间、日志摘要与最终输入。回归覆盖旧目标编译后的BigInt校验，最终Web与H5均重新录制。 |
| V-04 | 浏览器 | [Web](evidence/cost-amortization-web/result.json)及[Taro H5](evidence/cost-amortization-mobile/result.json)操作真实原生PG／API，使用合成数据，覆盖预览、MFA发布、历史与独立报表；自建页面、上下文、服务和临时库关闭。 |
| V-05 | 媒体 | 8 PNG／2 MP4／2 GIF实际像素，登录／MFA输入结束后录制，无音轨。[编码与采集](evidence/cost-amortization/media.json)及[视觉检查](evidence/cost-amortization/visual-review.json)。原199份按[保留记录](evidence/cost-amortization/reused-evidence.json)核验，合计211份。 |
| V-06 | Android | [开发APK](evidence/cost-amortization/build.json)、[18项检查](evidence/cost-amortization/apk-inspection.json)、[4项功能内容](evidence/cost-amortization/android-feature-content.json)；未执行设备。26条JVM为缓存结果，见[实际任务状态](evidence/cost-amortization/android-test-scope.json)。 |
| V-07 | Linux | 两个ELF64 amd64目标[交叉编译](evidence/cost-amortization/linux-build.json)，未执行、部署；不替代105原生隔离／真实模型验收。 |
| V-08 | 开发环境 | 正常升级schema83，保持历史／使用记录，仅更新已核验归属的API、通知与报表Worker。[更新](evidence/cost-amortization/development-update.json)及[只读核对](evidence/cost-amortization/development-project-resources.json)；临时库0，外部集成未启用；此前超时残留按[名称、OID、创建时间及空闲连接归属核对后清理](evidence/cost-amortization/attempts/07-resource-cleanup/cleanup-result.json)，未使用强制断连。 |
| V-09 | 依赖与范围 | 锁未变，沿用103审计70项（6 high／57 moderate／7 low），不声明新扫描。23项工作包、8项draft Skill、5项排除保持；完整GAP-03未完成。 |

初次类型检查发现共享测试依赖清单缺项；首次服务冷启动超出45秒窗口，[诊断](evidence/cost-amortization/attempts/01-build-input-and-startup/status.json)保留。第二次浏览器在文本定位阶段停止并正确清理，见[原结果](evidence/cost-amortization/attempts/02-text-locator/result.json)。修正定位、显式加载详情样式后重新检查。数据库空服务日期补强后的全量回归使用新源码执行，[早期批次](evidence/cost-amortization/attempts/04-before-date-binding/batch-progress.json)单独保留。随后一批20个测试有19个通过，TestAgentFileText在结束后的临时库删除阶段超时；[失败批次与原日志摘要](evidence/cost-amortization/attempts/05-fixture-cleanup-timeout/status.json)保留，不混作最终通过。随后H5真实页面暴露BigInt幂运算被旧目标转换为Math.pow的兼容问题；[失败页面与中止产物](evidence/cost-amortization/attempts/06-bigint-build-compatibility/status.json)保留。改为整数乘法，补旧目标编译回归后，8项客户端命令与两套实际页面重新执行。最终结果来自相同源码的实际执行及零残留检查。

Checks: backend 224 top-level tests / 1355 results, Web 172, mobile 273, runtime 55; WeChat and Douyin builds with 20 pages each, a development APK with 18 + 4 static checks, and 2 Linux cross-builds. The 26 JVM results retain their actual cached status. All 8 client commands were rerun after fixing BigInt operator lowering; a regression also executes the compiled legacy-target validator. New media: 8 screenshots, 2 silent MP4s and 2 GIFs; 199 earlier media retain exact bytes. No device, live cloud/model or production acceptance is claimed.

不做真机、模拟器、网络性能／实网120GB、历史迁移／容灾或AI费用控制。当前能力是可重列的经营费用分期视图；公司制度、跨币种核销、手续费／汇差、完整四端旅程和外部接入继续按[23项清单](../delivery/03-goal-gap-assessment.md)推进。
