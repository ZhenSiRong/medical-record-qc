/* ============================================================
   app.js —— 应用启动入口
   1) 全局注册组件
   2) 挂载应用
   ============================================================ */
(function () {
  'use strict';

  var app = Vue.createApp(window.MRApp);

  // 通用组件
  Object.keys(window.MRCommon).forEach(function (k) {
    if (k !== 'mrTone') app.component(k, window.MRCommon[k]);
  });

  // 图表组件
  Object.keys(window.MRCharts).forEach(function (k) {
    if (k !== 'SERIES_COLORS') app.component(k, window.MRCharts[k]);
  });

  // 业务页面
  Object.keys(window.MRPages).forEach(function (k) {
    app.component(k, window.MRPages[k]);
  });

  // 全局错误兜底，便于排查
  app.config.errorHandler = function (err, instance, info) {
    console.error('[MRMS] 组件异常：', info, err);
  };

  app.mount('#app');

  // 控制台提示
  console.log('%c病案管理系统 MRMS v1.0 %c Vue 3 · 纯前端演示版 ',
    'background:#1d6ef5;color:#fff;padding:3px 8px;border-radius:4px 0 0 4px;font-weight:600',
    'background:#14233c;color:#c9d6e8;padding:3px 8px;border-radius:0 4px 4px 0');
})();
