# v0.1.0-rc.6 · 功能冻结交付 / Frozen functional delivery

业务源码固定`c49177e6f9f26731ce1a2f2183e2c9020b2c4d3d`（主线149），数据库结构111。新增复制失败→已读→人工重试→实际校验恢复→新未读→准确版本／副本入口，现已纳入四端产物；不再保留“149源码新、APK148旧”的差异。本轮150停止新增业务，只做封包、核对及交付。原五次冻结保持原标签和原字节。

All application business changes through149 are frozen at the exact commit above. Linux/Web/runtime, WeChat, Douyin and the development Android APK are rebuilt from matching inputs. Packaging helpers and current delivery documentation are on main150. This is a complete source/artifact delivery candidate; production integration and formal platform acceptance remain open.

## 固定下载 / Fixed downloads

产物提交 / Artifact commit: `4a35ed903559438501153bcdbc6374ed30b528d8`。每个链接固定提交，五包均小于100MiB。

| 编号 | 产物 / Artifact | 字节 / Bytes |
| --- | --- | ---: |
| F150-01 | [Linux服务端／Web／秘书运行包 · Linux/Web/runtime](https://github.com/copypasteltd/case-manager/raw/4a35ed903559438501153bcdbc6374ed30b528d8/docs/releases/v0.1.0-rc.6/case-manager-v0.1.0-rc.6-linux-amd64.tar.gz) | 31385803 |
| F150-02 | [微信＋抖音 · WeChat/Douyin](https://github.com/copypasteltd/case-manager/raw/4a35ed903559438501153bcdbc6374ed30b528d8/docs/releases/v0.1.0-rc.6/case-manager-v0.1.0-rc.6-miniapps.zip) | 1011269 |
| F150-03 | [Android开发安装包 · Development APK](https://github.com/copypasteltd/case-manager/raw/4a35ed903559438501153bcdbc6374ed30b528d8/docs/releases/v0.1.0-rc.6/case-manager-v0.1.0-rc.6-android-debug.apk) | 43934195 |
| F150-04 | [完整应用源码和文档 · Complete application source/docs](https://github.com/copypasteltd/case-manager/raw/4a35ed903559438501153bcdbc6374ed30b528d8/docs/releases/v0.1.0-rc.6/case-manager-v0.1.0-rc.6-source.zip) | 10597520 |
| F150-05 | [全部演示素材 · All frozen media](https://github.com/copypasteltd/case-manager/raw/4a35ed903559438501153bcdbc6374ed30b528d8/docs/releases/v0.1.0-rc.6/case-manager-v0.1.0-rc.6-media.zip) | 96440777 |

[发布清单](release-manifest.json) · [SHA256SUMS](SHA256SUMS.txt) · [服务器检查](server-build.json) · [Git交付引用](git-publication.json)

| 文件 / File | SHA256 |
| --- | --- |
| case-manager-v0.1.0-rc.6-linux-amd64.tar.gz | `ab971e553fe53f689dd88e28e56b9eac2ea697fda518a9bbf192d19046f990c9` |
| case-manager-v0.1.0-rc.6-miniapps.zip | `77500de25c804c00a1459841e20d3d1ba4d71c7f12a3832aaa0d8bbe3147fb14` |
| case-manager-v0.1.0-rc.6-android-debug.apk | `14412c81d946821059dbd6b8d225fe075e6c2cc9ab05b48f7da941486cfc17fb` |
| case-manager-v0.1.0-rc.6-source.zip | `3b392c09be0b5234270010e468158b78b85b27f42be867b62dec0f1c40a88ac8` |
| case-manager-v0.1.0-rc.6-media.zip | `e66a17831d6f7c68bae8b76efa82b10daf0fbf6b35b72408c3903a459e2dc7b7` |

服务器可信内层清单摘要 / Trusted inner server manifest SHA256: `a5b9a51b3e8d5a29cd916c50d1e9d00d1f624302876de8aa77aa003dd6523e93`。

```sh
git clone --branch v0.1.0-rc.6 git@github.com:copypasteltd/case-manager.git
```

源码ZIP含全部应用源码、测试、锁文件、结构及方案／开发文档，固定Git149；较大历史证明保留在完整Git标签，源码ZIP排除`docs/media/`和`docs/quality/evidence/`。全部媒体在独立ZIP中。源码／媒体包逐Git blob核对，私有配置、数据库、依赖安装、缓存、工具和签名私钥不入包。

The source ZIP contains the complete application and guides. Historical quality evidence remains available in the full Git tag; all media is in the separate media ZIP. Every packaged source/media blob matches frozen Git bytes. Main150 contains packaging tools and these new release instructions.

## 使用与部署 / Usage and deployment

1. 下载并核对SHA256SUMS；源码和媒体解压到新目录。用可信源码检查服务归档：`python3 -B app/deploy/release/bundle.py inspect --archive PACKAGE.tar.gz --manifest-sha256 FULL_INNER_MANIFEST_SHA256 --output app/.local/inspection-UNIQUE`，使用上方完整内层摘要。
2. 按[原生部署手册](../../operations/03-native-release-runbook.md)准备目标Linux、专用身份、持久PostgreSQL、HTTPS和私有配置，再暂存、正常升级、激活并核对就绪。新库应用结构1–111；原开发PG100连接本版前需正常升级101–111。本轮保留既有PG和API，未部署或重启它们。
3. Web登录／MFA后按角色使用项目、任务、交付，合同、采购、报销，费用、付款、退款、订阅，文件／版本、博客、秘书和服务状态。审批、费用和现金分别记录；秘书草稿／计划须显式人工审阅采用。
4. 复制异常从“通知”打开对应版本及副本；已读和排队不代表恢复。有普通文件更新权限时明确提交重试，实际工作器校验成功才产生新恢复未读。组织复制台账继续从Web `/storage-operations`或移动“设置→复制运维”进入；其权限不会授予额外文件内容读取权。
5. 小程序／APK使用本地API `http://127.0.0.1:18080`；连接公司服务前设置真实HTTPS API、平台AppID／合法域名并重建。APK是开发签名，正式签名及平台发布尚待完成。
6. 配置实际OSS＋R2、Codex、原生Linux隔离、TG、监控和平台身份后，按对应工作包验收。八项Skill仍draft。参考[全部23项](../../delivery/05-functional-closure-150.md)，不能据封包认定真实服务已上线。

Verify both outer checksums and the trusted inner manifest, then follow the native Linux/PostgreSQL/HTTPS runbook. Users work within current roles and project scope. Read acknowledgements and queued retries preserve the need for actual replication; verified bytes produce recovery notifications pointing to the exact retained version. Configure real HTTPS/platform identities and rebuild mobile artifacts before server use. Formal signing and actual external-service acceptance remain outstanding.

## 核对与保留证据 / Checks and retained evidence

本轮新构建：服务器1359项准确输入、793项包文件／111项结构；两小程序151项输出；APK10项检查、185项选定源码映射逐内容核对及20页注册。五包展开5262项成员，源码／媒体3324项Git blob一致。真实Gradle任务`> Task :app:testDebugUnitTest UP-TO-DATE`，28项XML为缓存结果，本轮未新执行；没有单独重跑JVM。

精确复用149的22项源绑定检查、15条原生pass记录（12个实质子场景＋1既有通知回归＋1未启用钩子＋1组汇总），Web5／共享移动5及真实Web3项检查、三PNG／2.8秒无声MP4／6秒截图导览GIF。本轮不重新执行业务测试或录制；较早完整后端121、完整客户端136、运行器145的57项仍按原出处保留。

Fresh builds and artifact inspection are distinct from retained business evidence. There is no new full regression, browser capture, device test or target Linux execution in150. The MP4 records actual navigation/read pixels; the GIF is a disclosed screenshot tour. Expanded-byte private-material scanning checks known local private values and high-confidence text patterns, without pixel OCR or a production security sign-off.

用户移出：真机、网络性能／实网120GB、历史迁移／容灾、AI费用控制。四端、真实秘书／TG、AI授权／隔离／审计／技术限制、普通恢复、正常结构升级、持久PG及OSS＋R2继续保留。23个完整工作包仍active（14 P0／9 P1）。

GitHub Releases API此前因现有账号停用返回403，本轮没有再次请求或创建Releases网页；源码标签和固定下载已通过获授权SSH交付。[原始记录](../../quality/evidence/release-candidate-144/github-release-api.json)。The source tag and fixed downloads are delivered independently of the unavailable Releases webpage. No production deployment is claimed.

[中文使用指南](../../README.zh-CN.md) · [English guide](../../README.en.md) · [150交付方案](../../development/150-frozen-release-delivery.md) · [准确核对](../../quality/150-frozen-release-verification.md)
