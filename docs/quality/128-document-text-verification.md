# PDF／OCR实际检查 · 增量128

以[执行计划](evidence/document-text/verification-plan.json)及[最终源码检查](evidence/document-text/final-source-check.json)为准。本轮执行的是实际原生PDF／OCR与隔离合成业务，未调用容器、WSL、虚拟机、模拟器、真实模型、TG或真实OSS／R2。

| 编号 | 当前检查 | 覆盖与边界 |
|---|---|---|
| V128-01 | 六组选定HTTP／PostgreSQL套件，43条通过记录、零失败／跳过 | PDF／PNG／JPEG、DOCX／Office、搜索、秘书读取；显式引擎、固定源、完整页、旧终态、当前撤权／扫描及处理期撤回。不是全量后端回归。 |
| V128-02 | 实际Poppler／Tesseract和原生完整性检查，20条通过记录、零失败／跳过 | 文字＋扫描混合页面、两页PDF、PNG／JPEG、加密／损坏／21页拒绝、取消／临时清理、路径／模型／重复资产／工具摘要。合成输入位于[夹具](fixtures/document-text/README.md)。 |
| V128-03 | Web390项、共享移动476项 | 完整现有单元套件，新增完整页数／引擎依据、旧格式拒绝伪造OCR证据、固定版本比较和OCR说明。 |
| V128-04 | 全项目类型、Web／微信／抖音／Android JS构建 | 当前源码构建；Android仅新JS与四个准确原生模块，不生成新APK、不作正式签名／发布承诺。 |
| V128-05 | 实际Web及共享Taro H5，各4项流程检查 | 当前OCR命中、PDF／图片预览与原文摘要、准确PDF比较、同证据秘书资料读取。实际HTTP／PG／原生提取，云输入合成且没有替换API响应。 |
| V128-06 | 开发库95→96与既有原生服务 | 普通结构升级，旧文件／OA／财务／历史提取列保持，数据库未重启，服务只替换已确认归属的旧进程；外部集成保持停用。 |
| V128-07 | 6PNG／2MP4／2GIF逐项核对 | MP4来自本人验证后的实际视口帧，GIF为明确三张截图各2秒导览。没有密码／验证码／真实业务数据。浏览器／上下文／临时服务／数据库关闭后检查端口释放。 |

原生依赖按[独立清单](evidence/document-text/native-dependencies.json)披露，工具及模型留在忽略目录。清单固定4个工具与435个资产；当前原生安装含54个包，字体回退、目标Linux动态依赖和生产隔离尚未完成正式验收。OCR可能误读、遗漏和转换简繁体，不保证逐字正确或版式重建。

[Web实际证据](evidence/document-text-web/result.json)、[共享H5实际证据](evidence/document-text-mobile/result.json)、[素材和探测](evidence/document-text/media.json)、[可视检查](evidence/document-text/visual-review.json)、[原生模块检查](evidence/document-text/android-js-content.json)、[正常升级](evidence/document-text/development-update.json)。

保留边界：旧127所有证据／素材和95及以前迁移字节保持；旧121完整后端、runtime56项和开发APK保留原来源。本轮未执行新的全后端／runtime／正式APK验收，不能换成128产物。探索性失败和较早源码通过记录分别保存在[第一次尝试](evidence/document-text/attempts/01-before-final-source/README.md)、[移动测试入口修正前](evidence/document-text/attempts/02-before-mobile-test-registration/README.md)、[并发页面启动碰撞](evidence/document-text/attempts/03-review-startup-collision/README.md)及[原生临时目录准备前](evidence/document-text/attempts/04-native-temp-preparation/README.md)，均不重标为最终通过。最终两端页面检查按顺序执行；原生构建先创建声明的独立TEMP／TMP目录，再执行同一源码与命令。

当前[范围](../delivery/04-current-scope-and-acceptance.md)和AC30／TC72适用性保持；全部23项余项见[编号清单](../delivery/05-functional-closure-128.md)。不做真机、网络性能／实网120GB、历史迁移／容灾或AI费用控制；正常文件恢复、生产PG及双存储保留。

仓库[发布核对](evidence/document-text/repository-check.json)对完整候选文件执行凭据模式、已知私有值、链接、历史依据、素材和正文空白检查；暂存核对额外逐字节比较索引与工作文件。独立文件读取按原顺序返回，每批最多8路，缩短Windows逐个打开文件的等待；异常继续使检查失败。该发布脚本以自身SHA记录在发布结果中，独立于上述业务源码执行摘要。

合成PDF作为二进制输入保留全部内部空白，与既有 `*.pdf binary` 属性一致；全部文件和凭据扫描继续包含它们，源码空白格式检查只接收文本。首个候选检查中的[PDF分类错误记录](evidence/document-text/attempts/05-pdf-publication-classification/README.md)原样保留，修正分类没有改动夹具或任何业务执行输入。
