import { Component, type ReactNode } from 'react';

export class RouteBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() { return { failed: true }; }

  render() {
    if (this.state.failed) return <section className="panel empty" role="alert">
      <h2>暂时无法载入此页面</h2>
      <p>请检查网络后重新载入。已保存的内容可以继续在工作空间中查看。</p>
      <button className="button primary" onClick={() => window.location.reload()}>重新载入页面</button>
    </section>;
    return this.props.children;
  }
}
