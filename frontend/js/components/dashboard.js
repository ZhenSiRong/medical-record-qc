/* ============================================================
   dashboard.js —— 工作台（数据总览）
   ============================================================ */
(function () {
  'use strict';
  var computed = Vue.computed;

  var Dashboard = {
    name: 'Dashboard',
    components: {
      MrLineChart: MRCharts.MrLineChart,
      MrDonutChart: MRCharts.MrDonutChart,
      MrProgress: MRCommon.MrProgress
    },
    setup: function () {
      var S = MRMS.state, St = MRMS.stats;

      var ov = computed(function () { return St.overview(); });

      var kpis = computed(function () {
        var o = ov.value;
        return [
          {
            label: '住院病案总数', value: o.total, unit: '份', icon: 'ri-hospital-line', bg: 'bg-blue',
            foot: '近 6 个月出院病例累计'
          },
          {
            label: '归档完成率', value: o.archiveRate, unit: '%', icon: 'ri-archive-line', bg: 'bg-green',
            foot: '已归档 ' + o.archived + ' 份 / 待归档 ' + o.pendingArchive + ' 份'
          },
          {
            label: '编目完成率', value: o.catalogRate, unit: '%', icon: 'ri-price-tag-3-line', bg: 'bg-purple',
            foot: '已完成疾病与手术编码 ' + o.cataloged + ' 份'
          },
          {
            label: '本月出院病案', value: o.monthCount, unit: '份', icon: 'ri-calendar-check-line', bg: 'bg-orange',
            foot: '平均住院日 ' + o.avgStay + ' 天'
          }
        ];
      });

      var trend = computed(function () { return St.monthly('discharge', 6); });
      var statusDist = computed(function () { return St.byStatus(); });
      var deptRank = computed(function () { return St.byDept().slice(0, 7); });
      var deptMax = computed(function () {
        var d = deptRank.value;
        return d.length ? d[0].value : 1;
      });
      var reportPr = computed(function () { return St.reportProgress(); });
      var logs = computed(function () { return S.model.logs.slice(0, 5); });

      var todos = computed(function () {
        var o = ov.value;
        var toCatalog = S.model.records.filter(function (r) { return r.status === '已归档'; }).length;
        return [
          { label: '待归档病案', value: o.pendingArchive, route: 'archive', tone: 'orange', hint: '出院后 7 日内需完成归档' },
          { label: '待编目病案', value: toCatalog, route: 'catalog', tone: 'blue', hint: '已完成归档，待赋予 ICD 编码' },
          { label: '待审批借阅', value: o.pendingBorrow, route: 'borrow', tone: 'orange', hint: '借阅申请等待审批处理' },
          { label: '逾期未还', value: o.overdue, route: 'borrow', tone: 'red', hint: '已超过归还期限，需催还' },
          { label: '待上报病案', value: o.pendingReport, route: 'report', tone: 'blue', hint: '首页数据尚未提交至上报平台' }
        ];
      });

      function go(route) { S.route = route; }

      return {
        S: S, ov: ov, kpis: kpis, trend: trend, statusDist: statusDist,
        deptRank: deptRank, deptMax: deptMax, reportPr: reportPr,
        todos: todos, logs: logs, go: go, tone: MRCommon.mrTone
      };
    },
    template: [
      '<div>',
      /* ---------------- KPI 区 ---------------- */
      '  <div class="grid g-4">',
      '    <div class="card kpi" v-for="k in kpis" :key="k.label">',
      '      <div class="k-top">',
      '        <div class="k-ico" :class="k.bg"><i :class="k.icon"></i></div>',
      '        <span class="k-label">{{ k.label }}</span>',
      '      </div>',
      '      <div class="k-value">{{ k.value }}<small>{{ k.unit }}</small></div>',
      '      <div class="k-foot">{{ k.foot }}</div>',
      '    </div>',
      '  </div>',

      /* ---------------- 趋势 + 状态分布 ---------------- */
      '  <div class="grid g-2-1 mt-16">',
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>月度出院病案归档趋势</h3>',
      '        <span class="sub">按出院日期统计</span>',
      '        <div class="right">',
      '          <span class="badge blue no-dot">近 6 个月</span>',
      '        </div>',
      '      </div>',
      '      <div class="card-body">',
      '        <mr-line-chart :data="trend" unit=" 份"></mr-line-chart>',
      '      </div>',
      '    </div>',
      '    <div class="card">',
      '      <div class="card-head"><h3>病案流转状态</h3></div>',
      '      <div class="card-body">',
      '        <mr-donut-chart :data="statusDist" center-label="病案总数" :size="170"></mr-donut-chart>',
      '      </div>',
      '    </div>',
      '  </div>',

      /* ---------------- 科室排名 + 待办 ---------------- */
      '  <div class="grid g-2-1 mt-16">',
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>科室病案量排名</h3>',
      '        <span class="sub">Top 7</span>',
      '      </div>',
      '      <div class="card-body tight">',
      '        <div class="rank-row" v-for="(d, i) in deptRank" :key="d.name">',
      '          <span class="idx" :class="\'top\' + (i + 1)">{{ i + 1 }}</span>',
      '          <span class="nm">{{ d.name }}</span>',
      '          <span class="bar"><i :style="{ width: (d.value / deptMax * 100) + \'%\' }"></i></span>',
      '          <span class="vl tnum">{{ d.value }}</span>',
      '        </div>',
      '      </div>',
      '    </div>',
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>待办事项</h3>',
      '        <span class="sub">共 {{ todos.reduce(function(a,t){return a + t.value}, 0) }} 项</span>',
      '      </div>',
      '      <div class="card-body tight">',
      '        <div v-for="t in todos" :key="t.label"',
      '             class="flex items-center gap-12"',
      '             style="padding:9px 0;border-bottom:1px dashed #eef1f6;cursor:pointer"',
      '             @click="go(t.route)">',
      '          <span class="badge no-dot" :class="t.tone" style="min-width:36px;justify-content:center">{{ t.value }}</span>',
      '          <span style="flex:1">',
      '            <span style="display:block;font-size:13px">{{ t.label }}</span>',
      '            <span class="muted small">{{ t.hint }}</span>',
      '          </span>',
      '          <span class="muted">›</span>',
      '        </div>',
      '      </div>',
      '    </div>',
      '  </div>',

      /* ---------------- 上报进度 + 动态 ---------------- */
      '  <div class="grid g-2-1 mt-16">',
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>病案首页数据上报进度</h3>',
      '        <span class="sub">以已编目病案为基数</span>',
      '      </div>',
      '      <div class="card-body">',
      '        <div class="flex items-center gap-12">',
      '          <span style="font-size:26px;font-weight:700" class="tnum">{{ reportPr.rate }}%</span>',
      '          <div style="flex:1">',
      '            <mr-progress :value="reportPr.rate" tone="green"></mr-progress>',
      '            <div class="muted small mt-8">已上报 {{ reportPr.done }} 份 / 应上报 {{ reportPr.total }} 份</div>',
      '          </div>',
      '        </div>',
      '        <div class="divider"></div>',
      '        <div class="flex gap-12" style="flex-wrap:wrap">',
      '          <div style="flex:1;min-width:120px">',
      '            <div class="muted small">平均住院费用</div>',
      '            <div style="font-size:17px;font-weight:600" class="tnum">¥ {{ ov.avgFee.toLocaleString() }}</div>',
      '          </div>',
      '          <div style="flex:1;min-width:120px">',
      '            <div class="muted small">平均住院日</div>',
      '            <div style="font-size:17px;font-weight:600" class="tnum">{{ ov.avgStay }} 天</div>',
      '          </div>',
      '          <div style="flex:1;min-width:120px">',
      '            <div class="muted small">丙级病案</div>',
      '            <div style="font-size:17px;font-weight:600" class="tnum">',
      '              {{ S.model.records.filter(function(r){ return r.quality === \'丙级\' }).length }} 份',
      '            </div>',
      '          </div>',
      '        </div>',
      '      </div>',
      '    </div>',
      '    <div class="card">',
      '      <div class="card-head"><h3>最近操作动态</h3></div>',
      '      <div class="card-body">',
      '        <div class="timeline">',
      '          <div class="tl-item" :class="l.type" v-for="(l, i) in logs" :key="i">',
      '            <div class="tl-title">{{ l.user }} · {{ l.act }}</div>',
      '            <div class="tl-time">{{ l.target }} · {{ l.time }}</div>',
      '          </div>',
      '        </div>',
      '      </div>',
      '    </div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Dashboard = Dashboard;
})();
