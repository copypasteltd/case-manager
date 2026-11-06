# GAP-01候选反证：共享移动任务看板／基础时间线

反证人：GAP-03独立审查上下文，未撰写原GAP-01候选。基线`930cdde97b9d5d2e85d892664b0857a10a811739`；仅阅读文档和源码，没有测试、构建、浏览器、服务或应用代码改动。

**裁定：保留为确认的首发代码补齐项，范围仅为共享移动端任务列表之外的看板／基础时间线查阅；不要求复制桌面布局、高级甘特、拖拽、自动排期或新增写协议。**

## 尝试推翻与核对结果

1. **候选是否把Web局部增量误当所有端的等同布局要求？**

   `docs/development/06-clients-and-pages.md:5`明确能力可按工作场景区分，不要求桌面配置原样搬到小程序，因此不能要求所有视觉结构等同。但同文件`:7`不允许界面简化改变能力，`:14`规定项目／任务查阅在微信、抖音、Android均必需，`:16`规定任务处理。任务看板／基本日期与依赖视图是业务查阅，不是只能在桌面处理的管理配置；适配为窄屏分组／可读日期列表可以符合要求，不必照搬横向桌面表格。

2. **PM-04是否只对Web生效，移动端有正式首发豁免？**

   `docs/requirements/03-projects-and-knowledge.md:12`要求“列表、看板、基础时间线与筛选”，基本时间线须看期限与依赖，未限定Web。原始`docs/01-requirements-and-scope.md:127`也要求项目详情提供列表／看板与基础时间线。`docs/requirements/01-product-and-journeys.md:57`规定所有详细需求默认首发必需，移动免除桌面复杂配置与120GB上传；`:59`要求跨平台降级可见并有验收用例。在这些首发要求中，未找到接受移动长期缺少任务看板／基础时间线的条款。

3. **增量68的Web-only说明是否已经取消移动要求？**

   `docs/development/68-project-task-board-and-timeline.md:3`说明该轮只补Web呈现；`:17`明确“本轮范围是Web任务视图”，并继续保留多端补齐／首发验收。`docs/requirements/03-projects-and-knowledge.md:17`同样记录Web已实现范围。这些是既有实现范围披露，证明当时未完成所有端，未修改PM-04或用户的四端首发范围。已披露未实现不等于用户撤销功能，也不能从该说明推定已获首发豁免。

4. **是否已经有未被候选发现的共享移动实现？**

   `app/mobile/src/pages/project/task-forms.tsx:24–46`中`ProjectTaskList→TaskList`读取项目／成员和同一任务API，只有标题／状态筛选、Row列表与游标加载，没有视图选择、六状态分组呈现或基本日期／依赖时间线。相对的`app/web/src/ProjectTaskViews.tsx:34,48–49,55–87`有list/board/timeline、六状态分组、日期区间／单端／未排期及当前依赖读取，确认服务端已有基础数据和详情入口可复用。

## 检索与未读边界

- `rg -n 'PM-04|看板|时间线|甘特' docs/requirements -g '*.md'`定位PM-04及Web增量说明；逐段阅读原始§5.5、详细要求、平台矩阵及首发默认规则。
- 阅读`docs/development/68-project-task-board-and-timeline.md:3–26`及移动任务列表实际函数；通过`rg --files app/web/src -g '*Task*'`找到当前Web任务视图，再读取实际分派／日期／依赖分支。首次猜测的`ProjectTasks.tsx`、`68-project-kanban-and-timeline.md`不存在，未将该路径错误视作实现不存在的证明，随后按目录发现正确文件。
- 在`app/mobile/src/pages/project`、`app/mobile/src/platform/task-input.ts`、`app/mobile/src/app.config.ts`检索`board|timeline|TaskBoard|TaskTimeline|看板|时间线`无任务视图实现；查看当前`TaskList`完整函数确认无间接视图分派。
- 反证不审查项目其余生命周期、不声称全部移动源码无任意时间线，也不要求PM-04三种呈现具有相同桌面几何结构。服务端任务／依赖权限与现有写动作应继续复用；新移动视图仍须明确已加载范围、无权／读取失败隐藏和日期未知语义。

结论：未发现足以推翻候选的首发范围豁免。保留该窄范围代码差异，实际四端/公司业务验收仍另列，不能以新增界面代替。
