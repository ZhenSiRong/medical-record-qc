/* ============================================================
   quality.js —— 质量看板模块（归档时效专项）
   依据《病案管理质量控制指标（2021 版）》：
     出院病案 2 日归档率、归档及时率（目标 ≥90%）为病案科刚性考核指标，
     也是本次调研中「催办低效」痛点的量化抓手。
   覆盖：指标卡、月度 2 日归档率趋势、科室及时率排名、超期未归档催缴清单。
   数据来源：MRMS.ext.archiveTimeliness / archiveOverdue（后端真接口）。
   ============================================================ */
(function () {
  'use strict';
  var ref = Vue.ref, computed = Vue.computed, onMounted = Vue.onMounted;

  var Quality = {
    name: 'Quality',
    components: {
      MrBarChart: MRCharts.MrBarChart,
      MrEmpty: MRCommon.MrEmpty,
      MrProgress: MRCommon.MrProgress
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, E = MRMS.ext, U = MRMS.util;

      /* ------------------------------ 页面状态 ------------------------------ */
      var months = ref(6);        // 趋势窗口：近 3 / 6 / 12 月
      var overdueDays = ref(3);   // 超期阈值：默认 3 天（医院归档时限惯例）
      var timeliness = ref(null); // archiveTimeliness 原始返回
      var overdue = ref(null);    // archiveOverdue 原始返回

      // rate2 / rate3 后端为小数（0.0115），统一 ×100 保留 1 位
      function pct(v) {
        if (v === null || v === undefined) return 0;
        return +(v * 100).toFixed(1);
      }
      // 取日期前 10 位，后端 dischargeTime 形如 2026-09-20T09:00:00+08:00
      function d10(s) { return s ? String(s).slice(0, 10) : ''; }

      /* ------------------------------ 派生指标 ------------------------------ */
      var overallRaw = computed(function () {
        return timeliness.value ? timeliness.value.overall : null;
      });
      var loaded = computed(function () { return !!overallRaw.value; });
      var ov = computed(function () { return overallRaw.value || {}; });

      var rate2 = computed(function () { return pct(ov.value.rate2); });
      var rate3 = computed(function () { return pct(ov.value.rate3); });

      // 2 日归档率配色：达标绿；接近达标橙；严重不达标红
      var rate2Color = computed(function () {
        var o = overallRaw.value;
        if (!o) return '';
        if (o.met) return 'var(--success)';
        return pct(o.rate2) >= 70 ? 'var(--warning)' : 'var(--danger)';
      });

      // 月度趋势：只取 2 日归档率，值以百分数呈现给柱状图
      var trend = computed(function () {
        var t = (timeliness.value && timeliness.value.trend) || [];
        return t.map(function (x) {
          return { label: monthLabel(x.month), value: pct(x.rate2) };
        });
      });
      function monthLabel(m) {
        if (!m) return '';
        var s = String(m);
        var i = s.indexOf('-');
        return i > -1 ? (Number(s.slice(i + 1)) + '月') : s;
      }

      // 科室排名：按 2 日归档率降序
      var deptRank = computed(function () {
        var d = (timeliness.value && timeliness.value.byDept) || [];
        return d.slice().sort(function (a, b) {
          return (b.rate2 || 0) - (a.rate2 || 0);
        }).map(function (x) {
          var rate = pct(x.rate2);
          return {
            name: x.deptName,
            total: x.total || 0,
            archived: x.archived || 0,
            rate: rate,
            color: rate >= 90 ? 'var(--success)' : (rate >= 70 ? 'var(--warning)' : 'var(--danger)')
          };
        });
      });

      var overdueList = computed(function () {
        return (overdue.value && overdue.value.list) || [];
      });

      /* ------------------------------ 数据加载 ------------------------------ */
      function loadTimeliness() {
        E.archiveTimeliness(months.value).then(function (d) {
          timeliness.value = d || {};
        }, function (e) {
          A.toast((e && e.message) || '归档及时率加载失败', 'err');
        });
      }

      function loadOverdue() {
        E.archiveOverdue(overdueDays.value).then(function (d) {
          overdue.value = d || {};
        }, function (e) {
          A.toast((e && e.message) || '超期未归档清单加载失败', 'err');
        });
      }

      function pickMonths(m) {
        if (m === months.value) return;
        months.value = m;
        loadTimeliness(); // 切换窗口需重新向后端取数
      }

      function pickDays(d) {
        if (d === overdueDays.value) return;
        overdueDays.value = d;
        loadOverdue();    // 阈值变化需重取超期清单
      }

      /* ------------------------------ 导出催缴清单 ------------------------------ */
      function exportOverdue() {
        var rows = overdueList.value.map(function (r) {
          return [r.recordNo, r.patientName, r.deptName, r.doctorName,
            d10(r.dischargeTime), r.elapsedDays, r.overdueDays, r.status];
        });
        if (!rows.length) { A.toast('当前无超期病案可导出', 'err'); return; }
        U.exportCSV('超期未归档催缴清单_' + overdueDays.value + '天_' + U.today() + '.csv',
          ['病案号', '患者', '科室', '主治医师', '出院时间', '已出院天数', '超期天数', '状态'], rows);
        A.toast('已导出 ' + rows.length + ' 条超期病案，请分发至责任科室催办');
      }

      onMounted(function () {
        loadTimeliness();
        loadOverdue();
      });

      return {
        S: S, U: U,
        months: months, overdueDays: overdueDays,
        ov: ov, loaded: loaded, rate2: rate2, rate3: rate3, rate2Color: rate2Color,
        trend: trend, deptRank: deptRank, overdueList: overdueList,
        pickMonths: pickMonths, pickDays: pickDays, exportOverdue: exportOverdue,
        d10: d10, tone: MRCommon.mrTone
      };
    },
    template: [
      '<div>',
      /* ---------------- 政策依据说明 ---------------- */
      '  <div class="alert info">',
      '    <span class="ai"><i class="ri-information-line"></i></span>',
      '    <span>依据《病案管理质量控制指标（2021版）》：出院病案 2 日归档率与归档及时率（目标 ≥90%）为病案科刚性考核指标，未达标需按科室追溯并催办。</span>',
      '  </div>',

      /* ---------------- 四张指标卡 ---------------- */
      '  <div class="grid g-4 mt-16">',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-blue"><i class="ri-file-list-3-line"></i></div><span class="k-label">出院病案总数</span></div>',
      '      <div class="k-value">{{ loaded ? ov.total : \'—\' }}<small>份</small></div>',
      '      <div class="k-foot">已归档 {{ loaded ? ov.archived : \'—\' }} 份</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico" :class="loaded && ov.met ? \'bg-green\' : \'bg-orange\'"><i class="ri-time-line"></i></div><span class="k-label">2 日归档率</span></div>',
      '      <div class="k-value" :style="{ color: loaded ? rate2Color : \'\' }">{{ loaded ? rate2 : \'—\' }}<small>%</small></div>',
      '      <div class="k-foot">{{ loaded ? (ov.met ? \'已达标 ≥90%\' : \'未达标，目标 ≥90%\') : \'加载中…\' }}</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-cyan"><i class="ri-calendar-check-line"></i></div><span class="k-label">3 日归档率</span></div>',
      '      <div class="k-value">{{ loaded ? rate3 : \'—\' }}<small>%</small></div>',
      '      <div class="k-foot">出院 3 日内完成归档的病案占比</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-orange"><i class="ri-hourglass-line"></i></div><span class="k-label">平均归档天数</span></div>',
      '      <div class="k-value">{{ loaded ? ov.avgDays : \'—\' }}<small>天</small></div>',
      '      <div class="k-foot">病案从出院到完成归档的平均耗时</div>',
      '    </div>',
      '  </div>',

      /* ---------------- 月度 2 日归档率趋势 ---------------- */
      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>月度 2 日归档率趋势</h3>',
      '      <span class="sub">按出院日期归集，观察归档时效的逐月变化</span>',
      '      <div class="right">',
      '        <div class="seg">',
      '          <button :class="{ on: months === 3 }" @click="pickMonths(3)">近 3 月</button>',
      '          <button :class="{ on: months === 6 }" @click="pickMonths(6)">近 6 月</button>',
      '          <button :class="{ on: months === 12 }" @click="pickMonths(12)">近 12 月</button>',
      '        </div>',
      '      </div>',
      '    </div>',
      '    <div class="card-body">',
      '      <mr-bar-chart v-if="trend.length" :data="trend" color="#1898f8" unit="%"></mr-bar-chart>',
      '      <mr-empty v-else text="暂无月度归档率数据"></mr-empty>',
      '    </div>',
      '  </div>',

      /* ---------------- 科室归档及时率排名 ---------------- */
      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>科室归档及时率排名</h3>',
      '      <span class="sub">按 2 日归档率降序，共 {{ deptRank.length }} 个科室</span>',
      '    </div>',
      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead><tr>',
      '          <th style="width:64px">排名</th><th>科室</th>',
      '          <th>出院总数</th><th>已归档</th><th>2 日归档率</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="(d, i) in deptRank" :key="d.name">',
      '            <td class="tnum">{{ i + 1 }}</td>',
      '            <td>{{ d.name }}</td>',
      '            <td class="tnum">{{ d.total }}</td>',
      '            <td class="tnum">{{ d.archived }}</td>',
      '            <td>',
      '              <div class="flex items-center gap-8">',
      '                <span class="tnum" :style="{ color: d.color, fontWeight: 600, width: \'52px\' }">{{ d.rate }}%</span>',
      '                <mr-progress :value="d.rate"',
      '                             :tone="d.rate >= 90 ? \'green\' : (d.rate >= 70 ? \'orange\' : \'\')"',
      '                             style="flex:1;max-width:150px"></mr-progress>',
      '              </div>',
      '            </td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '      <mr-empty v-if="!deptRank.length" text="暂无科室归档数据"></mr-empty>',
      '    </div>',
      '  </div>',

      /* ---------------- 超期未归档催缴清单（本页重点） ---------------- */
      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>超期未归档催缴清单</h3>',
      '      <span class="sub">出院超过 {{ overdueDays }} 天仍未归档 · 共 {{ overdueList.length }} 份</span>',
      '      <div class="right">',
      '        <div class="seg">',
      '          <button :class="{ on: overdueDays === 3 }" @click="pickDays(3)">3 天</button>',
      '          <button :class="{ on: overdueDays === 7 }" @click="pickDays(7)">7 天</button>',
      '          <button :class="{ on: overdueDays === 15 }" @click="pickDays(15)">15 天</button>',
      '          <button :class="{ on: overdueDays === 30 }" @click="pickDays(30)">30 天</button>',
      '        </div>',
      '        <button class="btn btn-default" :disabled="!overdueList.length" @click="exportOverdue">导出清单 CSV</button>',
      '      </div>',
      '    </div>',
      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead><tr>',
      '          <th>病案号</th><th>患者</th><th>科室</th><th>主治医师</th>',
      '          <th class="nowrap">出院时间</th><th>已出院天数</th><th>超期天数</th><th>状态</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="r in overdueList" :key="r.recordId || r.recordNo">',
      '            <td class="mono">{{ r.recordNo }}</td>',
      '            <td class="nowrap">{{ r.patientName }}</td>',
      '            <td>{{ r.deptName }}</td>',
      '            <td class="nowrap">{{ r.doctorName }}</td>',
      '            <td class="nowrap tnum">{{ d10(r.dischargeTime) }}</td>',
      '            <td class="tnum">{{ r.elapsedDays }} 天</td>',
      '            <td class="tnum" style="color:var(--danger);font-weight:700">{{ r.overdueDays }} 天</td>',
      '            <td><span class="badge" :class="tone(r.status)">{{ r.status }}</span></td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '      <mr-empty v-if="!overdueList.length" text="暂无超期未归档病案"></mr-empty>',
      '    </div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Quality = Quality;
})();
