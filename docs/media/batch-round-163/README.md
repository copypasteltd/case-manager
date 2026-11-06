# 增量163截图、录制与GIF / Actual screenshots, recordings and GIFs

合成隔离账号；录制只在真实普通登录和MFA完成后开始。Web／共享移动H5均调用本轮原生API／PostgreSQL，读取原登录失效后的实际文件回执并再次读取；没有模型、云或真机验收。所有自建上下文、页面、服务和临时PG已关闭，私有fixture已移除。

| 入口 / Client | 准确截图 / Screenshots | 录屏 / Recording | GIF |
| --- | --- | --- | --- |
| Web | [原授权变化](batch-round-163-functional-wave-web-final/web-original-session-changed.png) · [重新读取](batch-round-163-functional-wave-web-final/web-history-read-receipt.png) | [实际MP4](batch-round-163-functional-wave-web-final/screen-recording.mp4) | [实际操作](batch-round-163-functional-wave-web-final/interaction.gif) · [截图轮播](batch-round-163-functional-wave-web-final/screenshot-tour.gif) |
| Shared mobile H5 | [原授权变化](batch-round-163-functional-wave-mobile-final/mobile-original-session-changed.png) · [重新读取](batch-round-163-functional-wave-mobile-final/mobile-history-read-receipt.png) | [实际MP4](batch-round-163-functional-wave-mobile-final/screen-recording.mp4) | [实际操作](batch-round-163-functional-wave-mobile-final/interaction.gif) · [截图轮播](batch-round-163-functional-wave-mobile-final/screenshot-tour.gif) |

此前Web首轮错误响应等待、移动首轮路由误报，以及中间Web读取成功均保留原输入和结果；当前两端为同一最终采集源码。凭据屏幕不录制，原始帧留在Git外；公开媒体只含合成资料。[验证边界](../../quality/163-batch-functional-verification.md) · [六张实际图复核](../../quality/evidence/batch-round-163/visual-review.json)
