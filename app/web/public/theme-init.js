// Runs before the first paint. Appearance is optional and never blocks login.
(function () {
  var preference = 'system';
  try {
    var stored = localStorage.getItem('cp:theme');
    if (stored === 'light' || stored === 'dark') preference = stored;
  } catch (_) {}
  var dark = false;
  try { dark = preference === 'dark' || (preference === 'system' && matchMedia('(prefers-color-scheme: dark)').matches); } catch (_) {}
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
})();
