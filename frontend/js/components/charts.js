/* ============================================================
   charts.js —— 轻量 SVG 图表组件
   不依赖任何第三方图表库，全部基于 Vue 模板 + 计算属性生成 SVG，
   保证在完全离线的环境下也能正常渲染。
   包含：折线/面积图、柱状图、环形图
   ============================================================ */
(function () {
  'use strict';

  var CYAN = '#1d6ef5';
  var SERIES_COLORS = ['#1d6ef5', '#12a15c', '#d98b13', '#7250e0', '#2187b8',
                       '#dc4b4b', '#0ea5a5', '#e07b9a', '#7a8fb5', '#8fbf3f'];

  /* ------------------------------ 折线 / 面积图 ------------------------------ */
  var MrLineChart = {
    name: 'MrLineChart',
    props: {
      data: { type: Array, default: function () { return []; } },
      color: { type: String, default: CYAN },
      unit: { type: String, default: '' },
      height: { type: Number, default: 230 }
    },
    computed: {
      geom: function () {
        var W = 620, H = this.height;
        var padL = 40, padR = 14, padT = 18, padB = 30;
        var plotW = W - padL - padR;
        var plotH = H - padT - padB;
        var vals = this.data.map(function (d) { return d.value; });
        var max = Math.max.apply(null, vals.concat([1]));
        var nice = Math.ceil(max / 5) * 5 || 5;
        var n = this.data.length;
        var stepX = n > 1 ? plotW / (n - 1) : 0;

        var pts = this.data.map(function (d, i) {
          return {
            x: padL + stepX * i,
            y: padT + plotH - (d.value / nice) * plotH,
            label: d.label,
            value: d.value
          };
        });

        var linePath = pts.map(function (p, i) {
          return (i === 0 ? 'M' : 'L') + p.x.toFixed(1) + ',' + p.y.toFixed(1);
        }).join(' ');

        var areaPath = pts.length
          ? linePath + ' L' + pts[pts.length - 1].x.toFixed(1) + ',' + (padT + plotH) +
            ' L' + pts[0].x.toFixed(1) + ',' + (padT + plotH) + ' Z'
          : '';

        // 水平网格线与 Y 轴刻度
        var ticks = [];
        for (var i = 0; i <= 4; i++) {
          var v = nice / 4 * i;
          ticks.push({
            y: padT + plotH - (v / nice) * plotH,
            text: Math.round(v * 10) / 10
          });
        }

        return {
          W: W, H: H, padL: padL, plotH: plotH, baseY: padT + plotH,
          pts: pts, linePath: linePath, areaPath: areaPath, ticks: ticks
        };
      }
    },
    template: [
      '<div class="chart-holder">',
      '  <svg :viewBox="\'0 0 \' + geom.W + \' \' + geom.H" style="width:100%;height:auto;display:block">',
      '    <defs>',
      '      <linearGradient :id="gid" x1="0" y1="0" x2="0" y2="1">',
      '        <stop offset="0%" :stop-color="color" stop-opacity="0.28"/>',
      '        <stop offset="100%" :stop-color="color" stop-opacity="0.02"/>',
      '      </linearGradient>',
      '    </defs>',
      '    <g>',
      '      <line v-for="(t, i) in geom.ticks" :key="\'g\'+i"',
      '            :x1="geom.padL" :x2="geom.W - 14" :y1="t.y" :y2="t.y"',
      '            stroke="#eef2f8" stroke-width="1"/>',
      '      <text v-for="(t, i) in geom.ticks" :key="\'gt\'+i"',
      '            :x="geom.padL - 8" :y="t.y + 3.5" text-anchor="end"',
      '            font-size="10.5" fill="#9aa6b8">{{ t.text }}</text>',
      '    </g>',
      '    <path :d="geom.areaPath" :fill="\'url(#\' + gid + \')\'"/>',
      '    <path :d="geom.linePath" fill="none" :stroke="color" stroke-width="2.4"',
      '          stroke-linecap="round" stroke-linejoin="round"/>',
      '    <g v-for="(p, i) in geom.pts" :key="\'p\'+i">',
      '      <circle :cx="p.x" :cy="p.y" r="4.2" fill="#fff" :stroke="color" stroke-width="2.2">',
      '        <title>{{ p.label }}：{{ p.value }}{{ unit }}</title>',
      '      </circle>',
      '      <text :x="p.x" :y="p.y - 11" text-anchor="middle" font-size="11"',
      '            font-weight="600" fill="#3b4657">{{ p.value }}</text>',
      '      <text :x="p.x" :y="geom.H - 8" text-anchor="middle" font-size="11"',
      '            fill="#8d99ab">{{ p.label }}</text>',
      '    </g>',
      '  </svg>',
      '</div>'
    ].join('\n'),
    data: function () { return { gid: 'lg' + Math.random().toString(36).slice(2, 8) }; }
  };

  /* ------------------------------ 柱状图 ------------------------------ */
  var MrBarChart = {
    name: 'MrBarChart',
    props: {
      data: { type: Array, default: function () { return []; } },
      color: { type: String, default: CYAN },
      color2: { type: String, default: '' },
      height: { type: Number, default: 230 },
      unit: { type: String, default: '' }
    },
    computed: {
      geom: function () {
        var W = 620, H = this.height;
        var padL = 40, padR = 14, padT = 20, padB = 30;
        var plotW = W - padL - padR;
        var plotH = H - padT - padB;
        var vals = this.data.map(function (d) { return d.value; });
        var max = Math.max.apply(null, vals.concat([1]));
        var nice = Math.ceil(max / 5) * 5 || 5;
        var n = this.data.length || 1;
        var slot = plotW / n;
        var bw = Math.min(46, slot * 0.58);

        var bars = this.data.map(function (d, i) {
          var h = (d.value / nice) * plotH;
          return {
            x: padL + slot * i + (slot - bw) / 2,
            y: padT + plotH - h,
            w: bw,
            h: Math.max(h, 1.5),
            label: d.label,
            value: d.value,
            cx: padL + slot * i + slot / 2
          };
        });

        var ticks = [];
        for (var i = 0; i <= 4; i++) {
          var v = nice / 4 * i;
          ticks.push({ y: padT + plotH - (v / nice) * plotH, text: Math.round(v * 10) / 10 });
        }
        return { W: W, H: H, padL: padL, bars: bars, ticks: ticks, bw: bw };
      }
    },
    template: [
      '<div class="chart-holder">',
      '  <svg :viewBox="\'0 0 \' + geom.W + \' \' + geom.H" style="width:100%;height:auto;display:block">',
      '    <defs>',
      '      <linearGradient :id="gid" x1="0" y1="0" x2="0" y2="1">',
      '        <stop offset="0%" :stop-color="color"/>',
      '        <stop offset="100%" :stop-color="color" stop-opacity="0.62"/>',
      '      </linearGradient>',
      '    </defs>',
      '    <line v-for="(t, i) in geom.ticks" :key="\'g\'+i" :x1="geom.padL" :x2="geom.W - 14"',
      '          :y1="t.y" :y2="t.y" stroke="#eef2f8" stroke-width="1"/>',
      '    <text v-for="(t, i) in geom.ticks" :key="\'gt\'+i" :x="geom.padL - 8" :y="t.y + 3.5"',
      '          text-anchor="end" font-size="10.5" fill="#9aa6b8">{{ t.text }}</text>',
      '    <g v-for="(b, i) in geom.bars" :key="\'b\'+i">',
      '      <rect :x="b.x" :y="b.y" :width="b.w" :height="b.h" rx="4"',
      '            :fill="color2 && i === geom.bars.length - 1 ? color2 : (\'url(#\' + gid + \')\')">',
      '        <title>{{ b.label }}：{{ b.value }}{{ unit }}</title>',
      '      </rect>',
      '      <text :x="b.cx" :y="b.y - 6" text-anchor="middle" font-size="11"',
      '            font-weight="600" fill="#3b4657">{{ b.value }}</text>',
      '      <text :x="b.cx" :y="geom.H - 8" text-anchor="middle" font-size="11" fill="#8d99ab">{{ b.label }}</text>',
      '    </g>',
      '  </svg>',
      '</div>'
    ].join('\n'),
    data: function () { return { gid: 'bg' + Math.random().toString(36).slice(2, 8) }; }
  };

  /* ------------------------------ 环形图 ------------------------------ */
  var MrDonutChart = {
    name: 'MrDonutChart',
    props: {
      data: { type: Array, default: function () { return []; } },
      size: { type: Number, default: 190 },
      thickness: { type: Number, default: 24 },
      centerLabel: { type: String, default: '合计' }
    },
    computed: {
      ring: function () {
        var R = 60;
        var C = 2 * Math.PI * R;
        var total = this.data.reduce(function (a, d) { return a + d.value; }, 0) || 1;
        var acc = 0;
        var segs = this.data.map(function (d, i) {
          var len = (d.value / total) * C;
          var seg = {
            color: d.color || SERIES_COLORS[i % SERIES_COLORS.length],
            len: len,
            gap: C - len,
            offset: -acc,
            name: d.name,
            value: d.value,
            percent: (d.value / total * 100).toFixed(1)
          };
          acc += len;
          return seg;
        });
        return { segs: segs, C: C, R: R, total: total };
      }
    },
    template: [
      '<div class="chart-holder" style="display:flex;gap:20px;align-items:center;flex-wrap:wrap">',
      '  <svg :viewBox="\'0 0 160 160\'" :style="{ width: size + \'px\', height: size + \'px\', flexShrink: 0 }">',
      '    <g transform="rotate(-90 80 80)">',
      '      <circle cx="80" cy="80" :r="ring.R" fill="none" stroke="#f1f4f9"',
      '              :stroke-width="thickness"/>',
      '      <circle v-for="(s, i) in ring.segs" :key="\'s\'+i" cx="80" cy="80" :r="ring.R"',
      '              fill="none" :stroke="s.color" :stroke-width="thickness"',
      '              :stroke-dasharray="s.len + \' \' + s.gap"',
      '              :stroke-dashoffset="s.offset" stroke-linecap="butt">',
      '        <title>{{ s.name }}：{{ s.value }}（{{ s.percent }}%）</title>',
      '      </circle>',
      '    </g>',
      '    <text x="80" y="75" text-anchor="middle" font-size="22" font-weight="700"',
      '          fill="#1f2733">{{ ring.total }}</text>',
      '    <text x="80" y="93" text-anchor="middle" font-size="10.5" fill="#8d99ab">{{ centerLabel }}</text>',
      '  </svg>',
      '  <div class="chart-legend" style="flex:1;min-width:150px;flex-direction:column;gap:9px;margin-top:0">',
      '    <div class="li" v-for="(s, i) in ring.segs" :key="\'l\'+i">',
      '      <span class="dot" :style="{ background: s.color }"></span>',
      '      <span style="flex:1">{{ s.name }}</span>',
      '      <span class="vl">{{ s.value }}</span>',
      '      <span class="muted small" style="width:44px;text-align:right">{{ s.percent }}%</span>',
      '    </div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  window.MRCharts = {
    MrLineChart: MrLineChart,
    MrBarChart: MrBarChart,
    MrDonutChart: MrDonutChart,
    SERIES_COLORS: SERIES_COLORS
  };
})();
