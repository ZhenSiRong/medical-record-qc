/* ============================================================
   report.js —— 病案首页数据上报模块
   覆盖：批次管理、上报前数据校验、执行上报、数据包导出、字段规范说明
   ============================================================ */
(function () {
  'use strict';
  var ref = Vue.ref, computed = Vue.computed, watch = Vue.watch;

  var Report = {
    name: 'Report',
    components: {
      MrEmpty: MRCommon.MrEmpty,
      MrModal: MRCommon.MrModal,
      MrProgress: MRCommon.MrProgress
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, St = MRMS.stats, U = MRMS.util;

      var selected = ref(null);
      var showCreate = ref(false);
      var newPeriod = ref(U.today().slice(0, 7));

      var batches = computed(function () {
        return S.model.reports.slice().sort(function (a, b) { return b.period.localeCompare(a.period); });
      });

      watch(batches, function (v) {
        if (!v.length) { selected.value = null; return; }
        // 保持当前选中批次（若仍然存在）
        if (selected.value) {
          var still = v.find(function (x) { return x.id === selected.value.id; });
          if (still) { selected.value = still; return; }
        }
        // 默认选中近期有数据的批次，保证校验面板有内容可看
        var withData = v.filter(function (x) { return x.total > 0; });
        selected.value = withData.length ? withData[0] : v[0];
      }, { immediate: true });

      var check = computed(function () {
        return selected.value ? St.validate(selected.value.period) : { total: 0, pass: 0, issues: [], rate: 100 };
      });

      var mast = computed(function () {
        var bs = batches.value;
        var pr = St.reportProgress();
        return {
          pending: pr.total - pr.done,
          rate: pr.rate,
          accepted: bs.filter(function (b) { return b.status === '已接收'; }).length,
          rejected: bs.filter(function (b) { return b.status === '已退回'; }).length
        };
      });

      // 可选月份：近 8 个月
      var periodOptions = computed(function () {
        var out = [];
        var now = new Date();
        for (var i = 0; i < 8; i++) {
          var d = new Date(now.getFullYear(), now.getMonth() - i, 1);
          out.push(d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0'));
        }
        return out;
      });

      function periodRecords(period) {
        return S.model.records.filter(function (r) {
          return r.dischargeDate.slice(0, 7) === period && r.status === '已编目';
        });
      }

      function createBatch() {
        if (!newPeriod.value) { A.toast('请选择上报周期', 'err'); return; }
        if (!periodRecords(newPeriod.value).length) {
          A.toast('该周期暂无可上报的已编目病案', 'err');
          return;
        }
        var rep = A.createReport(newPeriod.value);
        selected.value = rep;
        showCreate.value = false;
        A.toast('已创建上报批次 ' + rep.batchNo);
      }

      function doSubmit() {
        var rep = selected.value;
        if (!rep) { A.toast('请先选择上报批次', 'err'); return; }
        if (rep.status === '已接收') { A.toast('该批次已接收，无需重复上报', 'err'); return; }
        if (!check.value.total) { A.toast('该批次没有可上报数据', 'err'); return; }
        if (check.value.issues.length) {
          if (!confirm('校验发现 ' + check.value.issues.length + ' 条病案存在字段缺失，仍要上报吗？')) return;
        }
        A.submitReport(rep.id);
        A.toast('批次 ' + rep.batchNo + ' 已提交至省级平台');
      }

      function exportPackage() {
        var rep = selected.value;
        if (!rep) { A.toast('请先选择上报批次', 'err'); return; }
        var rows = periodRecords(rep.period);
        var pkg = {
          batchNo: rep.batchNo,
          period: rep.period,
          scope: rep.scope,
          generateTime: U.today() + ' ' + new Date().toTimeString().slice(0, 8),
          recordCount: rows.length,
          standard: '国家卫生健康委员会《住院病案首页数据采集接口标准》',
          records: rows.map(function (r) {
            return {
              mrNo: r.mrNo, inpatientNo: r.inpatientNo, name: r.name,
              gender: r.gender, age: r.age, dept: r.dept,
              admitDate: r.admitDate, dischargeDate: r.dischargeDate, stayDays: r.stayDays,
              diagMain: r.diagMain, icdCode: r.icdCode, icdCategory: r.icdCat,
              operationCode: r.opCode, operationName: r.opName,
              doctor: r.doctor, totalFee: r.fee, pageCount: r.pageCount,
              qualityLevel: r.quality, reportStatus: r.reportStatus
            };
          })
        };
        U.exportJSON('病案首页上报数据包_' + rep.batchNo + '.json', pkg);
        A.toast('数据包已导出，共 ' + rows.length + ' 条病案首页记录');
      }

      function exportIssues() {
        var iss = check.value.issues;
        if (!iss.length) { A.toast('当前批次未发现校验问题', 'err'); return; }
        U.exportCSV('上报校验问题清单_' + check.value.period + '.csv',
          ['病案号', '患者姓名', '问题描述'],
          iss.map(function (i) { return [i.mrNo, i.name, i.errs.join('；')]; }));
        A.toast('校验问题清单已导出');
      }

      // 首页核心字段规范
      var fieldSpec = [
        { key: 'mrNo', name: '病案号', req: '必填', note: '住院患者唯一标识，院内唯一' },
        { key: 'inpatientNo', name: '住院号', req: '必填', note: '与 HIS 住院流水号一致' },
        { key: 'name', name: '患者姓名', req: '必填', note: '与身份证一致，不含空格' },
        { key: 'gender', name: '性别', req: '必填', note: '值域：男 / 女' },
        { key: 'age', name: '年龄', req: '必填', note: '按入院时实际年龄取整' },
        { key: 'admitDate', name: '入院日期', req: '必填', note: '格式 YYYY-MM-DD' },
        { key: 'dischargeDate', name: '出院日期', req: '必填', note: '格式 YYYY-MM-DD' },
        { key: 'diagMain', name: '主要诊断', req: '必填', note: '按 ICD-10 规范填写' },
        { key: 'icdCode', name: '主要诊断编码', req: '必填', note: 'ICD-10 六位扩展码' },
        { key: 'opName', name: '手术操作名称', req: '条件必填', note: '无手术填“无手术操作”' },
        { key: 'opCode', name: '手术操作编码', req: '条件必填', note: 'ICD-9-CM-3 编码' },
        { key: 'doctor', name: '主治医师', req: '必填', note: '签署首页的医师姓名' },
        { key: 'fee', name: '住院总费用', req: '必填', note: '单位：元，保留两位小数' }
      ];

      return {
        S: S, U: U, batches: batches, selected: selected, check: check, mast: mast,
        showCreate: showCreate, newPeriod: newPeriod, periodOptions: periodOptions,
        createBatch: createBatch, doSubmit: doSubmit, exportPackage: exportPackage,
        exportIssues: exportIssues, fieldSpec: fieldSpec, tone: MRCommon.mrTone
      };
    },
    template: [
      '<div>',
      '  <div class="grid g-4">',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-orange"><i class="ri-file-upload-line"></i></div><span class="k-label">待上报病案</span></div>',
      '      <div class="k-value">{{ mast.pending }}<small>份</small></div>',
      '      <div class="k-foot">已完成编目但尚未提交至上报平台</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-green"><i class="ri-pie-chart-2-line"></i></div><span class="k-label">首页上报完成率</span></div>',
      '      <div class="k-value">{{ mast.rate }}<small>%</small></div>',
      '      <div class="k-foot"><mr-progress :value="mast.rate" tone="green"></mr-progress></div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-blue"><i class="ri-inbox-archive-line"></i></div><span class="k-label">平台已接收批次</span></div>',
      '      <div class="k-value">{{ mast.accepted }}<small>批</small></div>',
      '      <div class="k-foot">上报批次已被省级平台确认接收</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-red"><i class="ri-arrow-go-back-line"></i></div><span class="k-label">被退回批次</span></div>',
      '      <div class="k-value">{{ mast.rejected }}<small>批</small></div>',
      '      <div class="k-foot">需按退回意见补充数据后重新上报</div>',
      '    </div>',
      '  </div>',

      '  <div class="grid g-2-1 mt-16">',
      /* ---------------- 批次列表 ---------------- */
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>上报批次管理</h3>',
      '        <span class="sub">按自然月组织上报数据</span>',
      '        <div class="right">',
      '          <button class="btn btn-primary" @click="showCreate = true">+ 新建批次</button>',
      '        </div>',
      '      </div>',
      '      <div class="table-wrap">',
      '        <table class="data">',
      '          <thead><tr>',
      '            <th>批次号</th><th>上报周期</th><th>应报</th><th>已报</th>',
      '            <th>状态</th><th class="hide-sm">提交时间</th><th style="width:80px">操作</th>',
      '          </tr></thead>',
      '          <tbody>',
      '            <tr v-for="b in batches" :key="b.id"',
      '                :style="{ background: selected && selected.id === b.id ? \'#eef4ff\' : \'\' }">',
      '              <td class="mono">{{ b.batchNo }}</td>',
      '              <td>{{ b.period }}</td>',
      '              <td class="tnum">{{ b.total }}</td>',
      '              <td class="tnum">{{ b.submitted }}</td>',
      '              <td><span class="badge" :class="tone(b.status)">{{ b.status }}</span></td>',
      '              <td class="hide-sm tnum small">{{ b.submitTime || "—" }}</td>',
      '              <td><button class="link-btn" @click="selected = b">选择</button></td>',
      '            </tr>',
      '          </tbody>',
      '        </table>',
      '        <mr-empty v-if="!batches.length" text="暂无上报批次" icon="ri-archive-2-line"></mr-empty>',
      '      </div>',
      '    </div>',

      /* ---------------- 校验面板 ---------------- */
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>上报前数据校验</h3>',
      '        <span class="badge blue no-dot ml-auto" v-if="selected">{{ selected.batchNo }}</span>',
      '      </div>',
      '      <div class="card-body">',
      '        <template v-if="selected">',
      '          <div v-if="selected.note" class="alert"',
      '               :class="selected.status === \'已退回\' ? \'err\' : (selected.status === \'已接收\' ? \'ok\' : \'warn\')">',
      '            <span class="ai">!</span><span>{{ selected.note }}</span>',
      '          </div>',
      '          <div class="mt-16">',
      '            <div class="flex items-center gap-12">',
      '              <span style="font-size:24px;font-weight:700" class="tnum">{{ check.rate }}%</span>',
      '              <div style="flex:1">',
      '                <mr-progress :value="check.rate" :tone="check.rate >= 98 ? \'green\' : \'orange\'"></mr-progress>',
      '                <div class="muted small mt-8">',
      '                  应报 {{ check.total }} 份，校验通过 {{ check.pass }} 份，异常 {{ check.issues.length }} 份',
      '                </div>',
      '              </div>',
      '            </div>',
      '          </div>',
      '          <div class="divider"></div>',
      '          <div style="font-size:13px;font-weight:600;margin-bottom:8px">校验问题明细</div>',
      '          <div v-if="!check.issues.length" class="alert ok"><span class="ai"><i class="ri-checkbox-circle-line"></i></span>',
      '            <span>全部病案首页字段完整，可直接上报。</span></div>',
      '          <div v-else style="max-height:200px;overflow-y:auto">',
      '            <div v-for="i in check.issues" :key="i.mrNo"',
      '                 style="padding:8px 0;border-bottom:1px dashed #eef1f6">',
      '              <div class="flex items-center gap-8">',
      '                <span class="mono small">{{ i.mrNo }}</span>',
      '                <span class="small">{{ i.name }}</span>',
      '              </div>',
      '              <div class="small" style="color:var(--danger)">{{ i.errs.join("；") }}</div>',
      '            </div>',
      '          </div>',
      '          <div class="flex gap-8 mt-16" style="flex-wrap:wrap">',
      '            <button class="btn btn-default btn-sm" @click="exportIssues">导出问题清单</button>',
      '            <button class="btn btn-default btn-sm" @click="exportPackage">导出数据包</button>',
      '            <button class="btn btn-primary btn-sm ml-auto" @click="doSubmit">',
      '              {{ selected.status === \'已接收\' ? "已完成上报" : "执行上报" }}',
      '            </button>',
      '          </div>',
      '        </template>',
      '        <mr-empty v-else text="请选择左侧的上报批次" icon="ri-file-list-3-line"></mr-empty>',
      '      </div>',
      '    </div>',
      '  </div>',

      /* ---------------- 字段规范 ---------------- */
      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>病案首页上报字段规范</h3>',
      '      <span class="sub">依据国家卫生健康委员会《住院病案首页数据采集接口标准》整理</span>',
      '    </div>',
      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead><tr>',
      '          <th style="width:170px">数据项</th><th style="width:110px">必填性</th><th>填报说明</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="f in fieldSpec" :key="f.key">',
      '            <td><span class="mono small">{{ f.key }}</span> · {{ f.name }}</td>',
      '            <td><span class="badge" :class="f.req === \'必填\' ? \'red\' : \'orange\'">{{ f.req }}</span></td>',
      '            <td class="text-2">{{ f.note }}</td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '    </div>',
      '  </div>',

      /* ---------------- 新建批次 ---------------- */
      '  <mr-modal v-if="showCreate" title="新建上报批次" sub="选择上报周期" @close="showCreate = false">',
      '    <div class="field">',
      '      <label>上报周期</label>',
      '      <select class="select" v-model="newPeriod">',
      '        <option v-for="p in periodOptions" :key="p" :value="p">{{ p }}</option>',
      '      </select>',
      '    </div>',
      '    <div class="alert info mt-16"><span class="ai">i</span>',
      '      <span>批次创建后，系统将自动归集该周期内所有已完成编目的病案，并执行首页字段完整性校验。</span>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="showCreate = false">取消</button>',
      '      <button class="btn btn-primary" @click="createBatch">创建批次</button>',
      '    </template>',
      '  </mr-modal>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Report = Report;
})();
