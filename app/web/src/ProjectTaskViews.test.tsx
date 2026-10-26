import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { StaticRouter } from 'react-router-dom';
import { ApiError, type Task } from './api';
import ProjectTaskViews from './ProjectTaskViews';

describe('Current task read failures', () => {
  for (const view of ['list', 'board', 'timeline']) it('hides cached task facts and counts in the ' + view + ' view', () => {
    const noop = () => {};
    const html = renderToStaticMarkup(<StaticRouter location={'/projects/test?view=' + view}><ProjectTaskViews
      items={[{ id: 'cached-task', title: 'Previously authorized confidential task', version: 1, state: 'todo' } as Task]}
      state="" search="" pending={false} error={new ApiError(404, 'NOT_FOUND', '当前无权读取此项目', '')}
      hasMore={true} loadingMore={false} refreshing={false} canCreate={false} userName={() => 'Cached employee'}
      onState={noop} onSearch={noop} onLoadMore={noop} onRefresh={noop} onCreate={noop} onSelect={noop}
    /></StaticRouter>);
    expect(html).not.toContain('Previously authorized confidential task');
    expect(html).not.toContain('Cached employee');
    expect(html).not.toContain('已加载'); expect(html).not.toContain('加载更多任务');
    expect(html).not.toContain('没有符合条件的任务'); expect(html).not.toContain('新建任务');
    expect(html).toContain('当前无权读取此项目'); expect(html).toContain('重新读取任务');
  });
});
