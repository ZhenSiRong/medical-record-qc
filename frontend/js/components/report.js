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
      MrProgress: MRCommon.MrProgress,
      MrRoleNotice: MRCommon.MrRoleNotice
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, St = MRMS.stats, U = MRMS.util;

      /* ---------------- 角色能力门控（2026-10-09，本文件归属本次改动） ----------------
         本页后端权限已逐个核对（router.go 上报段 + branch_ext.go 任务8）：
           ADMIN only：POST /report-batches（新建批次）、POST /report-batches/:id/submit（上报）、
                       GET  /report-batches/:id/export（hqmsts_ CSV 导出）
           登录可见 ：GET /report-batches/:id/export-check（导出前检查）、
                       GET /report-consistency（一致性核对）
         故只对上面 3 个 ADMIN 接口对应的控件置灰，其余保持可用——
         尤其「导出前检查」和「一致性核对」不是 ADMIN 接口，不能一起灰掉
         （那会把临床本来能用的能力误判为无权限）。 */
      var canManageReport = computed(function () { return U.canManageReport(); });
      var canExportReport = computed(function () { return U.canExportReport(); });
      var reportHint = U.denyHint('report.manage');
      var exportHint = U.denyHint('report.export');

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
        if (!canManageReport.value) { A.toast(reportHint, 'err'); return; }
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
        if (!canManageReport.value) { A.toast(reportHint, 'err'); return; }
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

      /* ============================================================
         扩展能力（2026-10-09 接口批次）——本页接入 2 项：
           任务10 上报一致性核对（上报口径 vs 院内实际）
           任务8  导出前检查 + 符合国家接口标准的 hqmsts_ CSV 导出
         ============================================================ */
      var E = MRMS.ext;

      /* ---- 任务10：上报一致性核对 ---- */
      var consistency = ref(null);
      var consistencyLoading = ref(false);

      function runConsistency() {
        if (!selected.value) { A.toast('请先选择上报批次', 'err'); return; }
        consistencyLoading.value = true;
        E.reportConsistency(selected.value.period).then(function (d) {
          consistency.value = d || null;
        }).catch(function (e) {
          consistency.value = null;
          A.toast((e && e.message) || '一致性核对失败', 'err');
        }).then(function () { consistencyLoading.value = false; });
      }
      function closeConsistency() { consistency.value = null; }

      /* ---- 任务8：导出前检查 + 导出 CSV ---- */
      var showExportCheck = ref(false);
      var exportCheck = ref(null);
      var exportCheckLoading = ref(false);

      function runExportCheck() {
        var rep = selected.value;
        if (!rep) { A.toast('请先选择上报批次', 'err'); return; }
        if (!rep._bid) { A.toast('批次尚未落库完成，请稍后重试', 'err'); return; }
        exportCheck.value = null;
        exportCheckLoading.value = true;
        showExportCheck.value = true;
        E.reportExportCheck(rep._bid).then(function (d) {
          exportCheck.value = d || null;
        }).catch(function (e) {
          exportCheck.value = null;
          A.toast((e && e.message) || '导出前检查失败', 'err');
        }).then(function () { exportCheckLoading.value = false; });
      }

      // 导出符合国家接口标准的 CSV（文件名以 hqmsts_ 开头）
      // GET /report-batches/:id/export 在 admin 组（ADMIN only），临床调用必 403
      function downloadCsv() {
        if (!canExportReport.value) { A.toast(exportHint, 'err'); return; }
        var rep = selected.value;
        if (!rep) { A.toast('请先选择上报批次', 'err'); return; }
        if (!rep._bid) { A.toast('批次尚未落库完成，请稍后重试', 'err'); return; }
        var fname = 'hqmsts_' + rep.period.replace('-', '') + '.csv';
        E.call(E.downloadReportExport(rep._bid, fname)).then(function () {
          A.toast('已导出符合国家接口标准的 CSV（文件名以 hqmsts_ 开头）');
        }).catch(function () {});
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
        // 角色能力门控（模板里用 canManageReport / canExportReport 与提示文案）
        canManageReport: canManageReport, canExportReport: canExportReport,
        reportHint: reportHint, exportHint: exportHint,
        showCreate: showCreate, newPeriod: newPeriod, periodOptions: periodOptions,
        createBatch: createBatch, doSubmit: doSubmit, exportPackage: exportPackage,
        exportIssues: exportIssues, fieldSpec: fieldSpec, tone: MRCommon.mrTone,
        // 扩展能力（任务10 / 任务8）
        runConsistency: runConsistency, closeConsistency: closeConsistency,
        consistency: consistency, consistencyLoading: consistencyLoading,
        runExportCheck: runExportCheck, downloadCsv: downloadCsv,
        showExportCheck: showExportCheck, exportCheck: exportCheck, exportCheckLoading: exportCheckLoading
      };
    },
    template: [
      '<div>',
      /* 角色提示条：本页 ADMIN-only 操作对临床置灰的原因说明。
         注意措辞只列真的会 403 的三项——「导出前检查」「一致性核对」登录即可用。 */
      '  <mr-role-notice v-if="!canManageReport"',
      '    text="当前账号为临床医师：上报批次的新建与提交、以及国家接口标准 CSV 导出需病案室权限，相关按钮已置灰（悬停可见原因）；导出前检查、上报一致性核对与各类清单查看不受影响。"',
      '  ></mr-role-notice>',
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
      /* 任务10：一致性核对入口（核对当前选中批次的上报周期）
         —— GET /report-consistency 只挂 auth 组，临床可用，**不门控** */
      '          <button class="btn btn-default" @click="runConsistency"><i class="ri-git-compare-line"></i> 一致性核对</button>',
      /* 新建批次 = POST /report-batches（ADMIN only）→ 置灰 + 原因 */
      '          <button class="btn btn-primary" :disabled="!canManageReport"',
      '                  :title="canManageReport ? \'\' : reportHint" @click="showCreate = true">+ 新建批次</button>',
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
      /* 导出问题清单 / 导出数据包 = 前端本地 CSV/JSON 导出，不调后端接口，两角色都保留 */
      '            <button class="btn btn-default btn-sm" @click="exportIssues">导出问题清单</button>',
      '            <button class="btn btn-default btn-sm" @click="exportPackage">导出数据包</button>',
      /* 任务8：导出前检查 = GET /:id/export-check（登录可见，不门控）；导出 CSV = ADMIN only */
      '            <button class="btn btn-default btn-sm" @click="runExportCheck"><i class="ri-checkbox-multiple-line"></i> 导出前检查</button>',
      '            <button class="btn btn-default btn-sm" :disabled="!canExportReport"',
      '                    :title="canExportReport ? \'\' : exportHint" @click="downloadCsv">',
      '              <i class="ri-download-2-line"></i> 导出 CSV</button>',
      /* 执行上报 = POST /report-batches/:id/submit（ADMIN only）→ 置灰 + 原因 */
      '            <button class="btn btn-primary btn-sm ml-auto" :disabled="!canManageReport"',
      '                    :title="canManageReport ? \'\' : reportHint" @click="doSubmit">',
      '              {{ selected.status === \'已接收\' ? "已完成上报" : "执行上报" }}',
      '            </button>',
      '          </div>',
      '        </template>',
      '        <mr-empty v-else text="请选择左侧的上报批次" icon="ri-file-list-3-line"></mr-empty>',
      '      </div>',
      '    </div>',
      '  </div>',

      /* ---------------- 一致性核对（任务10） ---------------- */
      '  <div v-if="consistencyLoading || consistency" class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>上报一致性核对<span v-if="consistency" style="margin-left:6px;font-weight:600">· {{ consistency.period }}</span></h3>',
      '      <span class="badge no-dot" style="margin-left:8px" v-if="consistency"',
      '            :class="consistency.consistent ? \'green\' : \'red\'">',
      '        {{ consistency.consistent ? "上报口径一致" : "存在偏差" }}',
      '      </span>',
      '      <button class="link-btn ml-auto" @click="closeConsistency">收起</button>',
      '    </div>',
      '    <div class="card-body">',
      '      <div v-if="consistencyLoading" class="muted small">正在核对上报数据与院内原始数据…</div>',
      '      <template v-else-if="consistency">',
      '        <div class="flex gap-12" style="flex-wrap:wrap">',
      '          <div style="flex:1;min-width:150px;padding:12px 14px;border:1px solid var(--border);border-radius:8px">',
      '            <div class="muted small">应报数（院内应有）</div>',
      '            <div class="tnum" style="font-size:22px;font-weight:700">{{ consistency.expected }}</div>',
      '          </div>',
      '          <div style="flex:1;min-width:150px;padding:12px 14px;border:1px solid var(--border);border-radius:8px">',
      '            <div class="muted small">已报数（上报明细）</div>',
      '            <div class="tnum" style="font-size:22px;font-weight:700">{{ consistency.submitted }}</div>',
      '          </div>',
      '          <div style="flex:1;min-width:150px;padding:12px 14px;border:1px solid var(--border);border-radius:8px">',
      '            <div class="muted small">缺失数（漏报）</div>',
      '            <div class="tnum" :style="{ fontSize: \'22px\', fontWeight: 700, color: consistency.missing ? \'var(--danger)\' : \'\' }">{{ consistency.missing }}</div>',
      '          </div>',
      '          <div style="flex:1;min-width:150px;padding:12px 14px;border:1px solid var(--border);border-radius:8px">',
      '            <div class="muted small">上报后被改数</div>',
      '            <div class="tnum" :style="{ fontSize: \'22px\', fontWeight: 700, color: consistency.mismatch ? \'var(--danger)\' : \'\' }">{{ consistency.mismatch }}</div>',
      '          </div>',
      '        </div>',
      '        <div v-if="!consistency.consistent" class="alert err mt-16"><span class="ai">!</span>',
      '          <span>上报口径与院内实际不一致：存在漏报或上报后被改数据。评审现场核查会抽取 ≥20% 上报数据，偏差 ≥10% 会被判为错误数据并扣分，请务必核对后再上报。</span>',
      '        </div>',
      '        <div v-else class="alert ok mt-16"><span class="ai"><i class="ri-checkbox-circle-line"></i></span>',
      '          <span>上报数据与院内原始数据一致，无漏报、无上报后改动。</span>',
      '        </div>',
      '        <template v-if="(consistency.detail || []).length">',
      '          <div class="divider"></div>',
      '          <div style="font-size:13px;font-weight:600;margin-bottom:8px">偏差明细（{{ consistency.detail.length }}）</div>',
      '          <div style="max-height:240px;overflow-y:auto">',
      '            <div v-for="(d, i) in consistency.detail" :key="i"',
      '                 style="padding:8px 0;border-bottom:1px dashed #eef1f6">',
      '              <div class="flex items-center gap-8">',
      '                <span class="badge" :class="d.kind === \'MISSING\' ? \'orange\' : \'red\'">{{ d.kind === \'MISSING\' ? "缺失" : "被改" }}</span>',
      '                <span class="mono small">{{ d.recordNo || "—" }}</span>',
      '                <span class="small">{{ d.patientName }} · {{ d.deptName }}</span>',
      '              </div>',
      '              <div class="small muted">{{ d.detail }}</div>',
      '            </div>',
      '          </div>',
      '        </template>',
      '      </template>',
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
      '      <button class="btn btn-primary" :disabled="!canManageReport"',
      '              :title="canManageReport ? \'\' : reportHint" @click="createBatch">创建批次</button>',
      '    </template>',
      '  </mr-modal>',

      /* ---------------- 导出前检查（任务8） ---------------- */
      '  <mr-modal v-if="showExportCheck" title="导出前检查"',
      '            sub="扫描批次内病案首页必填字段完整性" :wide="true" @close="showExportCheck = false">',
      '    <div v-if="exportCheckLoading" class="muted small">正在扫描批次内病案首页…</div>',
      '    <template v-else>',
      '      <template v-if="exportCheck">',
      '        <div class="alert" :class="exportCheck.problemCount ? \'warn\' : \'ok\'">',
      '          <span class="ai">!</span>',
      '          <span>{{ exportCheck.message }}（共 {{ exportCheck.total }} 条，问题病案 {{ exportCheck.problemItems }} 份）</span>',
      '        </div>',
      '        <div v-if="!(exportCheck.items || []).length" class="alert ok mt-12">',
      '          <span class="ai"><i class="ri-checkbox-circle-line"></i></span>',
      '          <span>必填字段完整，可直接导出符合国家接口标准的 CSV。</span>',
      '        </div>',
      '        <div v-else class="mt-12" style="max-height:300px;overflow-y:auto">',
      '          <div v-for="(p, i) in exportCheck.items" :key="i"',
      '               style="padding:8px 0;border-bottom:1px dashed #eef1f6">',
      '            <div class="flex items-center gap-8">',
      '              <span class="mono small">{{ p.recordNo || ("#" + p.recordId) }}</span>',
      '              <span class="small">{{ p.patientName }}</span>',
      '            </div>',
      '            <div class="small" style="color:var(--danger)">{{ (p.issues || []).join("；") }}</div>',
      '          </div>',
      '        </div>',
      '      </template>',
      '      <div v-else class="muted small">检查失败，请稍后重试。</div>',
      '    </template>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="showExportCheck = false">关闭</button>',
      '      <button class="btn btn-primary" :disabled="!canExportReport"',
      '              :title="canExportReport ? \'\' : exportHint" @click="downloadCsv">',
      '        <i class="ri-download-2-line"></i> 导出 CSV</button>',
      '    </template>',
      '  </mr-modal>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Report = Report;
})();
