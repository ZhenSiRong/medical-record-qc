/* ============================================================
   theme-xiayi.js —— 侠医配色 JS 侧补丁（实测色值）
   须在 charts.js 与 store.js 之后、app.js 之前加载
   ============================================================ */
(function () {
  'use strict';

  var XIAYI_BLUE = '#1898f8';   // 交互主色（激活/序列）
  var XIAYI_TEAL = '#009088';   // 品牌青（工具栏/进度）

  // 1) 图表序列首色 → 品牌青（环图/多序列图）
  if (window.MRCharts && Array.isArray(window.MRCharts.SERIES_COLORS)) {
    window.MRCharts.SERIES_COLORS[0] = XIAYI_TEAL;
  }

  // 2) 折线/柱状图默认色 → 交互蓝（闭元常量改不动，改 props 默认值）
  ['MrLineChart', 'MrBarChart'].forEach(function (k) {
    var c = window.MRCharts && window.MRCharts[k];
    if (c && c.props && c.props.color && c.props.color.default === '#1d6ef5') {
      c.props.color.default = XIAYI_BLUE;
    }
  });

  // 3) 状态环图：已编目保持蓝系（侠医选中蓝），其余语义色不动
  if (window.MRMS && window.MRMS.stats && window.MRMS.stats.byStatus) {
    var orig = window.MRMS.stats.byStatus;
    window.MRMS.stats.byStatus = function () {
      var arr = orig();
      arr.forEach(function (x) { if (x.name === '已编目') x.color = XIAYI_BLUE; });
      return arr;
    };
  }
})();
