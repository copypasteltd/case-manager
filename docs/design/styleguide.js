/* Local review interactions only. No network, storage, or business writes. */
(() => {
  const root = document.documentElement;
  const toggle = document.querySelector('#theme-toggle');
  const feedback = document.querySelector('#sample-feedback');
  const logo = document.querySelector('.brand-logo');
  const updatePalette = () => {
    const styles = getComputedStyle(root);
    document.querySelectorAll('[data-token-value]').forEach(node => {
      node.textContent = styles.getPropertyValue('--cp-' + node.dataset.tokenValue).trim().toUpperCase();
    });
  };
  updatePalette();
  let feedbackTimer;
  const notify = message => {
    clearTimeout(feedbackTimer);
    feedback.textContent = message;
    feedback.hidden = false;
    feedbackTimer = setTimeout(() => { feedback.hidden = true; }, 5000);
  };
  toggle.addEventListener('click', () => {
    const dark = root.dataset.theme !== 'dark';
    root.dataset.theme = dark ? 'dark' : 'light';
    toggle.setAttribute('aria-pressed', String(dark));
    toggle.setAttribute('aria-label', dark ? '切换为浅色主题' : '切换为深色主题');
    logo.src = dark ? 'assets/official-logo-dark.png' : 'assets/official-logo-light.png';
    updatePalette();
    feedback.textContent = dark ? '已切换深色风格样板。' : '已切换浅色风格样板。';
  });
  document.querySelectorAll('.sample-action').forEach(button => button.addEventListener('click', () => {
    notify('「' + button.dataset.sample + '」为交互样例，没有提交业务数据。');
  }));
  document.querySelectorAll('.icon').forEach(icon => icon.setAttribute('aria-hidden', 'true'));
  document.querySelectorAll('.nav-list a,.mobile-nav a').forEach(link => link.addEventListener('click', () => {
    const group = link.closest('nav');
    const peers = group.classList.contains('nav-list') ? document.querySelectorAll('.nav-list a') : group.querySelectorAll('a');
    peers.forEach(peer => { peer.classList.remove('active'); peer.removeAttribute('aria-current'); });
    link.classList.add('active');
    link.setAttribute('aria-current', 'location');
  }));
  const form = document.querySelector('#sample-form');
  const input = document.querySelector('#sample-name');
  const summary = document.querySelector('#error-summary');
  const error = document.querySelector('#sample-name-error');
  const clearError = () => {
    input.removeAttribute('aria-invalid');
    input.setAttribute('aria-describedby', 'sample-name-help');
    error.hidden = true;
    summary.hidden = true;
  };
  input.addEventListener('input', () => { if (input.value.trim()) clearError(); });
  form.addEventListener('submit', event => {
    event.preventDefault();
    if (!input.value.trim()) {
      input.setAttribute('aria-invalid', 'true');
      input.setAttribute('aria-describedby', 'sample-name-help sample-name-error');
      error.hidden = false;
      summary.hidden = false;
      summary.focus();
      notify('示例表单有 1 个字段需要检查。');
    } else {
      clearError();
      notify('格式检查通过；这是样例，没有创建项目。');
    }
  });
})();
