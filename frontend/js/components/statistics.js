/* ============================================================
   statistics.js —— 数据统计模块
   覆盖：病案量趋势、疾病谱分布、科室排名、费用与住院日结构、质量等级
   ============================================================ */
(function () {
  'use strict';
  var ref = Vue.ref, computed = Vue.computed;

  var Statistics = {
    name: 'Statistics',
    components: {
      MrLineChart: MRCharts.MrLineChart,
      MrBarChart: MRCharts.MrBarChart,
      MrDonutChart: MRCharts.MrDonutChart,
      MrEmpty: MRCommon.MrEmpty
    },
    setup: function () {
      var S = MRMS.state, St = MRMS.stats, U = MRMS.util;

      var range = ref(6);

      var trend = computed(function () { return St.monthly('discharge', range.value); });
      var catDist = computed(function () { return St.byCategory(); });
      var statusDist = computed(function () { return St.byStatus(); });
      var deptRank = computed(function () { return St.byDept(); });
      var deptMax = computed(function () {
        var d = deptRank.value;
        return d.length ? d[0].value : 1;
      });

      // 费用区间分布
      var feeBuckets = computed(function () {
        var buckets = [
          { label: '<1万', min: 0, max: 10000 },
          { label: '1-3万', min: 10000, max: 30000 },
          { label: '3-5万', min: 30000, max: 50000 },
          { label: '5-10万', min: 50000, max: 100000 },
          { label: '>10万', min: 100000, max: Infinity }
        ];
        return buckets.map(function (b) {
          return {
            label: b.label,
            value: S.model.records.filter(function (r) { return r.fee >= b.min && r.fee < b.max; }).length
          };
        });
      });

      // 住院天数区间分布
      var stayBuckets = computed(function () {
        var buckets = [
          { label: '1-3天', min: 0, max: 4 },
          { label: '4-7天', min: 4, max: 8 },
          { label: '8-14天', min: 8, max: 15 },
          { label: '15天以上', min: 15, max: Infinity }
        ];
        return buckets.map(function (b) {
          return {
            label: b.label,
            value: S.model.records.filter(function (r) { return r.stayDays >= b.min && r.stayDays < b.max; }).length
          };
        });
      });

      // 质量等级分布
      var qualityDist = computed(function () {
        var colors = { '甲级': '#12a15c', '乙级': '#2b9fd8', '丙级': '#d98b13' };
        return ['甲级', '乙级', '丙级'].map(function (q) {
          return {
            name: q + '病案',
            color: colors[q],
            value: S.model.records.filter(function (r) { return r.quality === q; }).length
          };
        });
      });

      // 科室明细表
      var deptTable = computed(function () {
        var total = S.model.records.length || 1;
        return St.byDept().map(function (d) {
          var rs = S.model.records.filter(function (r) { return r.dept === d.name; });
          var archived = rs.filter(function (r) { return r.status !== '未归档'; }).length;
          var fee = rs.reduce(function (a, r) { return a + r.fee; }, 0);
          var stay = rs.reduce(function (a, r) { return a + r.stayDays; }, 0);
          return {
            name: d.name,
            count: d.value,
            percent: +(d.value / total * 100).toFixed(1),
            avgStay: rs.length ? +(stay / rs.length).toFixed(1) : 0,
            avgFee: rs.length ? Math.round(fee / rs.length) : 0,
            archiveRate: rs.length ? +(archived / rs.length * 100).toFixed(1) : 0
          };
        });
      });

      var ov = computed(function () { return St.overview(); });

      // 疾病谱 Top5
      var topDisease = computed(function () {
        var map = {};
        S.model.records.forEach(function (r) {
          map[r.diagMain] = (map[r.diagMain] || 0) + 1;
        });
        return Object.keys(map).map(function (k) { return { name: k, value: map[k] }; })
          .sort(function (a, b) { return b.value - a.value; }).slice(0, 5);
      });
      var topMax = computed(function () {
        return topDisease.value.length ? topDisease.value[0].value : 1;
      });

      function exportReport() {
        U.exportCSV('病案统计报表_' + U.today() + '.csv',
          ['科室', '病案量', '占比(%)', '平均住院日(天)', '平均费用(元)', '归档完成率(%)'],
          deptTable.value.map(function (d) {
            return [d.name, d.count, d.percent, d.avgStay, d.avgFee, d.archiveRate];
          }));
        MRMS.actions.toast('统计报表已导出');
      }

      return {
        S: S, U: U, range: range, trend: trend, catDist: catDist, statusDist: statusDist,
        deptRank: deptRank, deptMax: deptMax, feeBuckets: feeBuckets, stayBuckets: stayBuckets,
        qualityDist: qualityDist, deptTable: deptTable, ov: ov,
        topDisease: topDisease, topMax: topMax, exportReport: exportReport
      };
    },
    template: [
      '<div>',
      '  <div class="card">',
      '    <div class="card-head">',
      '      <h3>病案数据统计</h3>',
      '      <span class="sub">基于全部住院病案实时汇总</span>',
      '      <div class="right">',
      '        <div class="seg">',
      '          <button :class="{ on: range === 3 }" @click="range = 3">近 3 月</button>',
      '          <button :class="{ on: range === 6 }" @click="range = 6">近 6 月</button>',
      '        </div>',
      '        <button class="btn btn-default" @click="exportReport">导出报表</button>',
      '      </div>',
      '    </div>',
      '    <div class="card-body">',
      '      <div class="grid g-4">',
      '        <div><div class="muted small">病案总量</div>',
      '          <div style="font-size:22px;font-weight:700" class="tnum">{{ ov.total }} 份</div></div>',
      '        <div><div class="muted small">归档完成率</div>',
      '          <div style="font-size:22px;font-weight:700" class="tnum">{{ ov.archiveRate }}%</div></div>',
      '        <div><div class="muted small">平均住院日</div>',
      '          <div style="font-size:22px;font-weight:700" class="tnum">{{ ov.avgStay }} 天</div></div>',
      '        <div><div class="muted small">例均住院费用</div>',
      '          <div style="font-size:22px;font-weight:700" class="tnum">¥ {{ ov.avgFee.toLocaleString() }}</div></div>',
      '      </div>',
      '    </div>',
      '  </div>',

      '  <div class="grid g-2 mt-16">',
      '    <div class="card">',
      '      <div class="card-head"><h3>月度病案量趋势</h3><span class="sub">按出院日期归集</span></div>',
      '      <div class="card-body"><mr-bar-chart :data="trend" unit=" 份"></mr-bar-chart></div>',
      '    </div>',
      '    <div class="card">',
      '      <div class="card-head"><h3>疾病分类（ICD-10 大类）构成</h3></div>',
      '      <div class="card-body"><mr-donut-chart :data="catDist" center-label="病案总数" :size="180"></mr-donut-chart></div>',
      '    </div>',
      '  </div>',

      '  <div class="grid g-3 mt-16">',
      '    <div class="card">',
      '      <div class="card-head"><h3>病案流转状态</h3></div>',
      '      <div class="card-body"><mr-donut-chart :data="statusDist" center-label="合计" :size="150"></mr-donut-chart></div>',
      '    </div>',
      '    <div class="card">',
      '      <div class="card-head"><h3>病案质量等级</h3></div>',
      '      <div class="card-body"><mr-donut-chart :data="qualityDist" center-label="质控结果" :size="150"></mr-donut-chart></div>',
      '    </div>',
      '    <div class="card">',
      '      <div class="card-head"><h3>住院天数区间分布</h3></div>',
      '      <div class="card-body"><mr-bar-chart :data="stayBuckets" color="#7250e0" height="180"></mr-bar-chart></div>',
      '    </div>',
      '  </div>',

      '  <div class="grid g-2-1 mt-16">',
      '    <div class="card">',
      '      <div class="card-head"><h3>住院费用区间分布</h3></div>',
      '      <div class="card-body"><mr-bar-chart :data="feeBuckets" color="#2187b8" height="200" unit=" 份"></mr-bar-chart></div>',
      '    </div>',
      '    <div class="card">',
      '      <div class="card-head"><h3>高频主要诊断 Top 5</h3></div>',
      '      <div class="card-body tight">',
      '        <div class="rank-row" v-for="(d, i) in topDisease" :key="d.name">',
      '          <span class="idx" :class="\'top\' + (i + 1)">{{ i + 1 }}</span>',
      '          <span class="nm" style="width:auto;flex:1">{{ d.name }}</span>',
      '          <span class="bar" style="max-width:110px"><i :style="{ width: (d.value / topMax * 100) + \'%\' }"></i></span>',
      '          <span class="vl tnum">{{ d.value }}</span>',
      '        </div>',
      '      </div>',
      '    </div>',
      '  </div>',

      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>科室病案统计明细</h3>',
      '      <span class="sub">共 {{ deptTable.length }} 个科室</span>',
      '    </div>',
      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead><tr>',
      '          <th>科室</th><th>病案量</th><th>占比</th>',
      '          <th>平均住院日</th><th>例均费用</th><th>归档完成率</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="d in deptTable" :key="d.name">',
      '            <td>{{ d.name }}</td>',
      '            <td class="tnum">{{ d.count }}</td>',
      '            <td class="tnum">{{ d.percent }}%</td>',
      '            <td class="tnum">{{ d.avgStay }} 天</td>',
      '            <td class="tnum">¥ {{ d.avgFee.toLocaleString() }}</td>',
      '            <td>',
      '              <div class="flex items-center gap-8">',
      '                <span style="width:44px" class="tnum">{{ d.archiveRate }}%</span>',
      '                <mr-progress :value="d.archiveRate"',
      '                             :tone="d.archiveRate >= 90 ? \'green\' : (d.archiveRate >= 70 ? \'\' : \'orange\')"',
      '                             style="flex:1;max-width:150px"></mr-progress>',
      '              </div>',
      '            </td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '    </div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Statistics = Statistics;
})();
