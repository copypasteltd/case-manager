# 增量165实际页面 / Authenticated synthetic demo

完整代码和统一检查后，在隔离合成资料中通过实际Web／共享移动H5界面与原生接口，查看账期固定提醒、版本变化和补充协议的主合同入口。两组各两项业务检查通过；所有自建浏览器、原生测试PG、临时页面服务已关闭。登录、凭据及MFA画面不录制。

| 页面 | 截图 | 实际操作录屏／GIF |
|---|---|---|
| Web固定提醒与版本 | [完整提醒](batch-round-165-functional-wave-web-verified/web-reminder-versions.png) | [MP4](batch-round-165-functional-wave-web-verified/screen-recording.mp4) · [实际操作GIF](batch-round-165-functional-wave-web-verified/interaction.gif) · [截图导览](batch-round-165-functional-wave-web-verified/screenshot-tour.gif) |
| Web补充协议与主合同 | [补充协议入口](batch-round-165-functional-wave-web-verified/web-supplement-parent-entry.png) · [主合同](batch-round-165-functional-wave-web-verified/web-parent-contract.png) | 同一实际录屏 |
| 共享移动固定提醒 | [新版本及送达](batch-round-165-functional-wave-mobile-final/mobile-reminder-versions.png) · [旧七天规则](batch-round-165-functional-wave-mobile-final/mobile-pinned-old-reminder.png) | [MP4](batch-round-165-functional-wave-mobile-final/screen-recording.mp4) · [实际操作GIF](batch-round-165-functional-wave-mobile-final/interaction.gif) · [原始截图导览](batch-round-165-functional-wave-mobile-final/screenshot-tour.gif) |
| 共享移动补充协议与主合同 | [补充协议入口](batch-round-165-functional-wave-mobile-final/mobile-supplement-parent-entry.png) · [稳定主合同画面](batch-round-165-functional-wave-mobile-final/mobile-parent-contract-stable-frame.png) | 稳定图为同一实际录屏末尾原生提帧；[原始侧滑瞬间截图](batch-round-165-functional-wave-mobile-final/mobile-parent-contract.png)不覆盖 |

证据：[Web采集](../../quality/evidence/batch-round-165-functional-wave-web-verified/result.json)、[移动采集](../../quality/evidence/batch-round-165-functional-wave-mobile-final/result.json)、[稳定画面来源](../../quality/evidence/batch-round-165-functional-wave-mobile-final/derived-parent-frame.json)、[准确字节及清理核对](../../quality/evidence/batch-round-165/final-source-check.json)。首次失败、旧查询诊断通过和录屏失败的记录均保留在quality目录，不能替代当前两组源码证据。

The links show authenticated synthetic UI/API sessions, with exact original stills, actual silent MP4 recordings, recording-derived interaction GIFs and separately labelled screenshot tours. The original mobile parent still caught a route slide; the guide uses an unedited stable final video frame with hashed provenance. The old still and video remain unchanged. APK164 does not include source165.

采集后的两处交付工具修正单列于[限定源码记录](../../quality/evidence/batch-round-165/publication-source-adjustment.json)；应用消费者和媒体未变化。最终核对还原两工具原字节来匹配完整采集输入，其余输入必须一致；本次未补录或冒充工具修正后的新采集。

No real device, provider, cloud or production acceptance is inferred. All owned browser contexts, native fixture databases and review servers must close after each check.
