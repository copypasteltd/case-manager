# 主机资源实际检查 · 增量130

以[执行计划](evidence/host-resources/verification-plan.json)及[最终源码复核](evidence/host-resources/final-source-check.json)为准。本轮直接实现CPU、内存、磁盘的采集→签名接收→持久化→四端显示／配置／历史。实际HTTP、PostgreSQL、签名及桥接器均参与检查；Prometheus响应为隔离合成夹具，不代表目标服务器已经安装采集器。

| 编号 | 实际检查 | 结果与边界 |
|---|---|---|
| V130-01 | 三组选定HTTP／PostgreSQL业务套件 | 31条通过记录，零失败／跳过。覆盖真实桥接查询、正常／CPU／内存／磁盘／只读阈值、恢复、配置冲突、缺指标拒绝、原始签名、重放与同序号冲突、迟到、断流过期、停用、撤权和日志重启；原服务状态及Blackbox桥接回归。未执行全量后端。 |
| V130-02 | servicestatus／statusbridge完整单元套件及原生编译 | 82条通过记录、零跳过；Windows及Linux/amd64实际采集器编译通过。固定查询、整数容量／阈值、完整磁盘清单与严格载荷解析。Linux产物交叉构建不等于目标环境执行。 |
| V130-03 | Web408项／共享移动493项与全项目类型 | 全部通过；两端共用12项容量、清单、配置和单调时间有效期业务用例。缺少单调时钟或有效期到期时隐藏旧指标。 |
| V130-04 | Web、微信、抖音、Android当前源码构建 | 三个浏览器／小程序构建通过；当前local配置完整开发APK通过19项内容／签名检查，四个相关JS模块与实际映射源码一致。正式签名／升级／平台发布继续GAP-16/19。 |
| V130-05 | 原生Android及独立JVM套件 | Android28项结果：UP-TO-DATE；JVM28项结果：UP-TO-DATE。保留真实任务结果、原始XML时间和摘要；UP-TO-DATE结果逐字匹配已发布129证据，不冒充本轮重新执行。[准确缓存边界](evidence/host-resources/native-test-scope.json)。 |
| V130-06 | 实际Web及共享Taro H5各四段业务流程 | 真实API正常指标→只读严重告警→恢复；明确MFA停用→未知且隐藏指标；历史及原始签名核对。既有可用性无观测仍未知，与资源健康独立。没有替换业务API响应。 |
| V130-07 | 正常结构升级与开发服务 | 实际96→97升级，原服务／文件／OA／费用／付款／提取历史摘要保持。仅更新已确认归属的API、通知和报表服务，不重启PG、不启用模型／TG／云集成。结束后零隔离测试库。 |
| V130-08 | 6PNG／2MP4／2GIF可视检查及清理 | 实际合成账号登录后的页面；MP4为真实视口像素，核对正常／严重／恢复；GIF明确为三场景截图各2秒导览。移动390×900仅展示长页的当前视口，其余磁盘需下滑，完整指标另由实际API断言核对。所有自建浏览器、上下文、临时服务、页面与隔离库已释放。 |

[后端](evidence/host-resources/backend.json)、[资源核心](evidence/host-resources/host-core.json)、[Web](evidence/host-resources-web/result.json)、[共享H5](evidence/host-resources-mobile/result.json)、[当前开发APK](evidence/host-resources/build.json)、[APK检查](evidence/host-resources/apk-inspection.json)、[四个当前JS模块](evidence/host-resources/android-js-content.json)、[素材](evidence/host-resources/media.json)、[可视复核](evidence/host-resources/visual-review.json)、[正常升级](evidence/host-resources/development-update.json)。APK、工具、私钥、数据库、原始临时截图帧留在忽略目录。

早期尝试保留原状态：[重复夹具注册](evidence/host-resources/attempts/01-fixture-enrollment/README.md)、[Web显式类型引用](evidence/host-resources/attempts/02-web-project-includes/README.md)、[检查脚本重用验证码被正确拒绝](evidence/host-resources/attempts/03-capture-otp-replay/README.md)、[默认Node22.14不满足既有工具链](evidence/host-resources/attempts/04-review-node-version/README.md)。类型引用及验证码检查修复后重跑选定后端、客户端、当前APK及两端实际页面检查。检查使用仓库原生Node22.23.4，无任何容器／WSL／虚拟机／模拟器／真机调用。

随后[APK检查分组登记](evidence/host-resources/attempts/05-apk-inspection-registration/README.md)发现检查器沿用26项规则，未对新分组启用现有Office私有来源DEX检查。仅扩展两处准确分组条件，重新检查同一APK：28项原生结果及19项检查全部通过。此前真实执行摘要不改写；[固定来源复核](evidence/host-resources/inspection-adjustment.json)以已发布129检查器为锚，按固定两行逆补丁重建实际执行时检查器／计划字节，完整组合摘要必须与各原记录相等。所有应用、单元测试、构建、夹具、开发服务和素材字节保持；不因检查器分组修正冒充重新执行完整构建。

最终复核的重建说明曾被同名APK变量覆盖；修正仅涉及复核器输出字段，23项功能复核重新生成。[早期记录](evidence/host-resources/attempts/06-proof-metadata-variable/README.md)保留，应用及实际执行记录不变。

全部129及更早的已发布证据／媒体原字节、96项历史迁移保持。旧121全量后端与runtime56项未重新执行，保留原来源。当前开发APK及缓存XML不代表正式Android发布验收。30AC／72TC适用性未修改，完整旅程继续按[全部23项余项](../delivery/05-functional-closure-130.md)收口；八项Skill仍draft。

资源告警目前为页面评估，并写入原始审计／outbox；尚未实现资源专属断流主动通知与独立外部通道。真实Prometheus／Blackbox／node_exporter安装、获准目标故障／恢复和告警送达继续GAP-23。生产PG、OSS＋R2、模型、TG及平台真实身份继续按原编号验收；不重新引入已移出的费用控制、真机、网络性能、历史迁移和容灾。

The current-source checks cover three real HTTP/PostgreSQL suites, complete client unit suites, controlled native collector ingestion, four-client compilation and the current development APK. Both actual browser flows and all media are independently source-bound. Native test cache outcomes above remain explicit; neither a cross-built collector nor a development APK establishes production acceptance. All prior published evidence remains byte-identical.
