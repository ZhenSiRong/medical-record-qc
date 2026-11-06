/* ============================================================
   catalog.js —— 病案编目模块
   覆盖：待编目队列、疾病与手术编码录入、ICD 编码库检索、编目质量把控
   ============================================================ */
(function () {
  'use strict';
  var ref = Vue.ref, computed = Vue.computed, reactive = Vue.reactive;

  var Catalog = {
    name: 'Catalog',
    components: {
      MrEmpty: MRCommon.MrEmpty,
      MrProgress: MRCommon.MrProgress,
      MrModal: MRCommon.MrModal
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, St = MRMS.stats, U = MRMS.util;

      var current = ref(null);
      var dictTab = ref('icd10');
      var kw = ref('');
      var search = ref('');

      // 待编目队列：后端状态 RECYCLED（已回收待编目）→ 界面「待编目」
      // 注意：不要筛「已归档」——那是 ARCHIVED（已归档在架），早已编完目，不属于待编目队列。
      var queue = computed(function () {
        return S.model.records
          .filter(function (r) {
            if (r.status !== '待编目') return false;
            if (!search.value.trim()) return true;
            var k = search.value.trim().toLowerCase();
            return (r.mrNo + r.name + r.diagMain + r.dept).toLowerCase().indexOf(k) > -1;
          })
          .sort(function (a, b) { return a.dischargeDate.localeCompare(b.dischargeDate); });
      });

      // 已编目（最近 8 条）
      // ⚠️ 判据用 r.cataloged（= 后端 catalogedAt 是否有值），**不是**当前状态：
      // 编目动作链在质控通过后会调 /archives/shelve 自动上架，病案状态立刻变为
      // ARCHIVED（界面「已归档」）。若按 status === '已编目' 过滤，刚编完目的病案
      // 会立刻从这个列表消失，用户会以为"保存没生效"（实测全库 51 份属此情形）。
      // 本列表是"最近完成的编目"= 编目状态的流水，与病案当前是否在架无关。
      var done = computed(function () {
        return S.model.records
          .filter(function (r) { return r.cataloged; })
          .sort(function (a, b) { return (b.catalogDate || '').localeCompare(a.catalogDate || ''); })
          .slice(0, 8);
      });

      var todayCount = computed(function () {
        return S.model.records.filter(function (r) { return r.catalogDate === U.today(); }).length;
      });

      var ov = computed(function () { return St.overview(); });

      /* ------------------ 编目表单 ------------------ */
      var form = reactive({
        icdCode: '', icdCat: '', icdName: '',
        opCode: '', opName: '',
        quality: '甲级', remark: ''
      });

      function select(r) {
        current.value = r;
        form.icdCode = r.icdCode || '';
        form.icdName = r.diagMain || '';
        form.icdCat = r.icdCat || '';
        form.opCode = r.opCode || '';
        form.opName = r.opName || '';
        form.quality = r.quality || '甲级';
        form.remark = r.remark || '';
      }

      function pickDiag(code) {
        var d = U.icd10Dict.find(function (x) { return x.code === code; });
        if (d) { form.icdCode = d.code; form.icdName = d.name; form.icdCat = d.cat; }
      }
      function pickOp(code) {
        var d = U.icd9Dict.find(function (x) { return x.code === code; });
        if (d) { form.opCode = d.code; form.opName = d.name; }
      }

      function save() {
        if (!current.value) { A.toast('请先从左侧队列选择待编目病案', 'err'); return; }
        if (!form.icdCode) { A.toast('请选择主要诊断的 ICD-10 编码', 'err'); return; }
        A.catalog(current.value.mrNo, {
          icdCode: form.icdCode,
          diagMain: form.icdName,
          icdCat: form.icdCat,
          opCode: form.opCode || '00.0000',
          opName: form.opName || '无手术操作',
          quality: form.quality,
          remark: form.remark
        });
        A.toast('病案 ' + current.value.mrNo + ' 编目完成');
        // 任务16：编目动作链异步生成质量评分，有限重试后在面板展示等级（依据评分口径 甲≥90/乙75-89/丙<75）
        pollQuality(current.value._rid, current.value.mrNo, 3);
        current.value = null;
      }

      /* ------------------ 字典检索 ------------------ */
      var dictList = computed(function () {
        var src = dictTab.value === 'icd10' ? U.icd10Dict : U.icd9Dict;
        var k = kw.value.trim().toLowerCase();
        if (!k) return src;
        return src.filter(function (d) {
          return (d.code + ' ' + d.name).toLowerCase().indexOf(k) > -1;
        });
      });

      function useDict(d) {
        if (!current.value) { A.toast('请先选择待编目病案', 'err'); return; }
        if (dictTab.value === 'icd10') pickDiag(d.code);
        else pickOp(d.code);
        A.toast('已填入：' + d.name);
      }

      /* ============================================================
         扩展能力（2026-10-09 接口批次）——本页接入 4 项：
           任务3  智能预编码推荐（编码辅助，非自动填入）
           任务7  DRG 预检（查表提示，非官方分组器）
           任务16 编目后质量评分等级展示
           任务5  质控工作台与「退回—整改—复核」闭环
         定位：均为「编码/质控辅助」，只提示与流转，不替代编码员判断。
         ============================================================ */
      var E = MRMS.ext;

      /* ---- 任务3：智能预编码推荐（编码辅助，非自动填入）---- */
      var suggestOpen = ref(false);
      var suggestText = ref('');
      var suggestType = ref('ICD10');
      var suggestTarget = ref('icd');   // 'icd' 填主诊断 / 'op' 填主手术
      var suggestLoading = ref(false);
      var suggestList = ref([]);

      // 打开推荐面板：默认取「当前表单已填名称 → 病案登记主诊断 → 备注」作为待分析文本，
      // 减少编码员重复录入；由编码员点击候选后才填入，属“辅助选择”。
      function openSuggest(target) {
        if (!current.value) { A.toast('请先从左侧队列选择待编目病案', 'err'); return; }
        suggestTarget.value = target;
        suggestType.value = target === 'op' ? 'ICD9CM3' : 'ICD10';
        suggestText.value = target === 'op'
          ? (form.opName || form.remark || '')
          : (form.icdName || current.value.diagMain || form.remark || '');
        suggestList.value = [];
        suggestOpen.value = true;
      }

      function runSuggest() {
        var text = (suggestText.value || '').trim();
        if (!text) { A.toast('请输入用于推荐的诊断/手术文本（如 “急性阑尾炎”）', 'err'); return; }
        suggestLoading.value = true;
        E.call(E.suggestCodes(text, suggestType.value)).then(function (list) {
          suggestList.value = list || [];
          if (!suggestList.value.length) A.toast('未匹配到候选编码，请手工检索', 'err');
        }).catch(function () { suggestList.value = []; })
          .then(function () { suggestLoading.value = false; });
      }

      // 点击候选编码：填入对应编码字段并回填名称（编码员确认后才生效）
      function applySuggest(item) {
        if (suggestTarget.value === 'op') {
          form.opCode = item.code;
          form.opName = item.name || '';
        } else {
          pickDiag(item.code);                                  // 复用既有选码逻辑：回填 code/name/cat
          if (!form.icdName && item.name) form.icdName = item.name;
        }
        suggestOpen.value = false;
        A.toast('已填入候选编码：' + item.code + ' ' + (item.name || ''));
      }

      /* ---- 任务7：DRG 预检（查表提示，非官方分组器）---- */
      var drgOpen = ref(false);
      var drgLoading = ref(false);
      var drgResult = ref(null);
      /* DRG 未匹配时的兜底提示文案（抽出为变量，避免模板内转义） */
      var drgFallback = '未匹配到分组规则，请核对主诊断/主手术填写（编码不完整或填写缺失会导致无法入组）。';
      /* 费用格式化：后端返回 float，避免界面出现 13749.521259873729 这类长尾 */
      function fmtCost(v) {
        var n = Number(v || 0);
        return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
      }

      /* ---- ADR-0002 批次 D：CC/MCC 严重度分层预览（disclaimer 是响应契约，不是注释）---- */
      var ccmccOpen = ref(false), ccmccLoading = ref(false), ccmccJudgement = ref(null);
      var ccmccError = ref(''), ccmccEditCodes = ref(''), ccmccEditFlags = ref('');
      var CCMCC_HINT = '严重度分层基于院内演示判读表（Source=demo，非国家医保局官方表），仅供编码质控参考，不作为医保结算或 DRG/DIP 分组依据。';
      /* 当前病案首页：质控工作台把完整首页放在 qcData.frontPage（与 rectify.js 的 fixFp 同源）。 */
      function ccmccFrontPage() {
        return (qcData.value && qcData.value.frontPage) ? qcData.value.frontPage : null;
      }
      function ccmccRecordId() {
        var fp = ccmccFrontPage();
        return (fp && (fp.recordId || fp.id)) || (qcTarget.value ? qcTarget.value.recordId : null);
      }
      function runCcmcc() {
        var fp = ccmccFrontPage();
        ccmccOpen.value = true;
        /* 与质控工作台互斥：两个 mr-modal 同层（同一 z-index），同时打开会按 DOM 顺序叠成一团，
           后渲染的那个压在上面。入口本身在质控工作台里，故打开预览时收起工作台（qcData 仍保留，
           预览所需的首页快照不丢），关掉预览即回到编目页。 */
        qcOpen.value = false;
        ccmccLoading.value = true;
        ccmccError.value = '';
        ccmccJudgement.value = null;
        ccmccEditCodes.value = fp ? (fp.otherDiagnosisCodes || '') : '';
        ccmccEditFlags.value = fp ? (fp.otherDiagnosisFlags || '') : '';
        /* 两种入参形态：优先 recordId（走库）；编目阶段首页尚未落库时退化为直接传编码串判读。 */
        E.call(E.ccmccPreview({
          recordId: ccmccRecordId(),
          mainDiagnosisCode: fp ? (fp.mainDiagnosisCode || '') : '',
          otherDiagnosisCodes: ccmccEditCodes.value,
          otherDiagnosisFlags: ccmccEditFlags.value
        })).then(function (r) {
          /* api-store.js 的 req() 已经 return json.data（第 51 行），E.call 又原样透传，
             故这里的 r **就是** service.CcMccJudgement 本体，再取一层 r.data 必然是
             undefined —— 弹窗会静默落到「暂无判读结果」，而控制台一个错都不报。
             2026-10-27 实测踩中，形如 @@ CC_MCC_PREVIEW_UNWRAP @@ 的层级偏移别再犯。 */
          ccmccJudgement.value = (r && typeof r === 'object') ? r : null;
        }).catch(function (e) {
          ccmccJudgement.value = null;
          ccmccError.value = (e && e.message) ? e.message : '严重度预览失败';
        }).then(function () { ccmccLoading.value = false; });
      }
      function saveCcmccOtherDiag() {
        var rid = ccmccRecordId();
        if (!rid) { ccmccError.value = '缺少病案标识，无法保存'; return; }
        ccmccError.value = '';
        /* 载荷沿用 rectify.js 的「完整快照 + 补丁」姿势：后端按全量白名单赋值，
           补丁外的字段原样回传，不会被写成零值（与 D3 那次数据丢失同源的风险，务必保持）。 */
        E.saveFrontPage(rid, ccmccFrontPage() || {}, {
          otherDiagnosisCodes: ccmccEditCodes.value,
          otherDiagnosisFlags: ccmccEditFlags.value
        }).then(function () {
          /* 保存后必须把本地首页快照重拉一遍再判读。
             qcData.frontPage 是**打开工作台那一刻**的快照，不刷新就 reuse 旧值：
             判读请求会带着旧的标记串去算、编辑框也回填旧值 —— 症状是「保存成功但
             界面毫无变化」，与保存失败长得一模一样（2026-10-27 截图实测踩中：
             库里已是 0,C，弹窗仍显示 C,0，于是误判成"保存没生效"）。
             刷新失败不阻断后续判读：判读本身从库里取，接口侧仍能拿到新值。 */
          return E.getFrontPage(rid).then(function (res) {
            if (res && res.frontPage && qcData.value) qcData.value.frontPage = res.frontPage;
          }).catch(function () { /* 静默降级，见上 */ });
        }).then(function () {
          runCcmcc();
        }).catch(function (e) {
          ccmccError.value = (e && e.message) ? e.message : '保存失败';
        });
      }

      function runDrg() {
        if (!form.icdCode) { A.toast('请先填写主要诊断编码，再做 DRG 预检', 'err'); return; }
        drgLoading.value = true;
        drgResult.value = null;
        drgOpen.value = true;
        // opCode 传空由后端按“无手术”处理；主诊断与主手术都填好时提示更准确
        E.call(E.drgPreview({ diagCode: form.icdCode, opCode: form.opCode || '' })).then(function (r) {
          drgResult.value = r || null;
        }).catch(function () { drgResult.value = null; })
          .then(function () { drgLoading.value = false; });
      }

      /* ---- 任务16：编目后质量评分等级展示 ---- */
      var lastResult = ref(null);
      // 编目动作链（编目→首页→质控）异步执行，质量评分在其后才落库，
      // 故做有限次重试轮询，避免“刚编目完读不到评分”。
      function pollQuality(rid, mrNo, tries) {
        E.quality(rid).then(function (q) {
          lastResult.value = {
            mrNo: mrNo, score: q.score, gradeName: q.gradeName,
            errorCount: q.errorCount, warnCount: q.warnCount,
            vetoed: !!q.vetoed, vetoReason: q.vetoReason || ''
          };
        }).catch(function () {
          if (tries > 0) setTimeout(function () { pollQuality(rid, mrNo, tries - 1); }, 900);
        });
      }

      /* ---- 任务5：质控工作台与退回返修闭环 ---- */
      var qcOpen = ref(false);
      var qcLoading = ref(false);
      var qcTarget = ref(null);
      var qcData = ref(null);
      var qcOpinionOpen = ref(false);
      var qcOpinionText = ref('');
      var qcOpinionMode = ref('return');   // 'return' 退回意见 / 'review_fail' 复核不通过原因
      var qcOpinionItem = ref(null);

      // 处理状态 → 中文标签与颜色（OPEN 橙 / RETURNED 红 / RECTIFIED 蓝 / CLOSED 绿 / REJECTED_BY_QC 红）
      var QC_STATUS = {
        OPEN: { text: '待处理', tone: 'orange' },
        RETURNED: { text: '已退回临床', tone: 'red' },
        RECTIFIED: { text: '已整改待复核', tone: 'blue' },
        CLOSED: { text: '复核通过（已闭环）', tone: 'green' },
        REJECTED_BY_QC: { text: '复核不通过', tone: 'red' }
      };
      function qcStatusText(s) { return (QC_STATUS[s] || {}).text || s || '—'; }
      function qcStatusTone(s) { return (QC_STATUS[s] || {}).tone || 'gray'; }

      function loadQc() {
        if (!qcTarget.value || !qcTarget.value._rid) return;
        qcLoading.value = true;
        E.qcWorkbench(qcTarget.value._rid).then(function (d) {
          qcData.value = d || null;
        }).catch(function (e) {
          qcData.value = null;
          A.toast((e && e.message) || '质控工作台加载失败', 'err');
        }).then(function () { qcLoading.value = false; });
      }

      // 入口：rec 可传入（最近编目行操作），缺省取工作台当前病案
      function openQc(rec) {
        var target = rec || current.value;
        if (!target || !target._rid) { A.toast('请先选择待编目病案', 'err'); return; }
        qcTarget.value = target;
        qcData.value = null;
        qcOpen.value = true;
        loadQc();
      }

      function qcReturnItem(item) {
        qcOpinionMode.value = 'return';
        qcOpinionItem.value = item;
        qcOpinionText.value = '';
        qcOpinionOpen.value = true;
      }
      function qcRectifyItem(item) {
        E.call(E.qcRectify(item.id), '已模拟临床整改，待病案室复核')
          .then(function () { loadQc(); }).catch(function () {});
      }
      function qcReviewPass(item) {
        E.call(E.qcReview(item.id, 'PASS', ''), '复核通过，该质控项已闭环')
          .then(function () { loadQc(); }).catch(function () {});
      }
      function qcReviewFail(item) {
        qcOpinionMode.value = 'review_fail';
        qcOpinionItem.value = item;
        qcOpinionText.value = '';
        qcOpinionOpen.value = true;
      }
      function confirmQcOpinion() {
        var item = qcOpinionItem.value;
        if (!item) return;
        if (qcOpinionMode.value === 'return') {
          E.call(E.qcReturn(item.id, qcOpinionText.value || '请临床补充/修正首页数据'), '已退回临床整改')
            .then(function () { qcOpinionOpen.value = false; loadQc(); }).catch(function () {});
        } else {
          E.call(E.qcReview(item.id, 'FAIL', qcOpinionText.value || '整改未通过，请继续修正'), '已退回临床（复核不通过）')
            .then(function () { qcOpinionOpen.value = false; loadQc(); }).catch(function () {});
        }
      }

      return {
        S: S, U: U, current: current, queue: queue, done: done, search: search,
        todayCount: todayCount, ov: ov, form: form, dictTab: dictTab, kw: kw,
        dictList: dictList, select: select, pickDiag: pickDiag, pickOp: pickOp,
        save: save, useDict: useDict, tone: MRCommon.mrTone,
        // 扩展能力（任务3/5/7/16）
        openSuggest: openSuggest, runSuggest: runSuggest, applySuggest: applySuggest,
        suggestOpen: suggestOpen, suggestText: suggestText, suggestType: suggestType,
        suggestTarget: suggestTarget, suggestLoading: suggestLoading, suggestList: suggestList,
        runDrg: runDrg, drgOpen: drgOpen, drgLoading: drgLoading, drgResult: drgResult,
        drgFallback: drgFallback,
        // ADR-0002 批次 D：CC/MCC 严重度分层预览 + 其他诊断填报
        ccmccOpen: ccmccOpen, ccmccLoading: ccmccLoading, ccmccJudgement: ccmccJudgement,
        ccmccError: ccmccError, ccmccEditCodes: ccmccEditCodes, ccmccEditFlags: ccmccEditFlags,
        CCMCC_HINT: CCMCC_HINT, runCcmcc: runCcmcc, saveCcmccOtherDiag: saveCcmccOtherDiag,
        fmtCost: fmtCost,
        lastResult: lastResult,
        openQc: openQc, loadQc: loadQc, qcOpen: qcOpen, qcLoading: qcLoading,
        qcTarget: qcTarget, qcData: qcData, qcStatusText: qcStatusText, qcStatusTone: qcStatusTone,
        qcReturnItem: qcReturnItem, qcRectifyItem: qcRectifyItem,
        qcReviewPass: qcReviewPass, qcReviewFail: qcReviewFail,
        qcOpinionOpen: qcOpinionOpen, qcOpinionText: qcOpinionText,
        qcOpinionMode: qcOpinionMode, qcOpinionItem: qcOpinionItem, confirmQcOpinion: confirmQcOpinion
      };
    },
    template: [
      '<div>',
      /* ---------------- 指标条 ---------------- */
      '  <div class="grid g-3">',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-orange"><i class="ri-inbox-unarchive-line"></i></div><span class="k-label">待编目队列</span></div>',
      '      <div class="k-value">{{ queue.length }}<small>份</small></div>',
      '      <div class="k-foot">已完成归档、等待赋予疾病与手术编码</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-blue"><i class="ri-edit-2-line"></i></div><span class="k-label">今日编目量</span></div>',
      '      <div class="k-value">{{ todayCount }}<small>份</small></div>',
      '      <div class="k-foot">编目人员：{{ S.currentUser.name }}</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-green"><i class="ri-pie-chart-2-line"></i></div><span class="k-label">编目完成率</span></div>',
      '      <div class="k-value">{{ ov.catalogRate }}<small>%</small></div>',
      '      <div class="k-foot" style="width:100%">',
      '        <mr-progress :value="ov.catalogRate" tone="green"></mr-progress>',
      '      </div>',
      '    </div>',
      '  </div>',

      '  <div class="grid g-1-2 mt-16">',
      /* ---------------- 待编目队列 ---------------- */
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>待编目病案</h3>',
      '        <span class="badge orange no-dot ml-auto">{{ queue.length }}</span>',
      '      </div>',
      '      <div class="card-body tight" style="border-bottom:1px solid var(--border)">',
      '        <div class="search-box">',
      '          <input class="input" v-model="search" placeholder="搜索病案号 / 姓名 / 诊断">',
      '        </div>',
      '      </div>',
      '      <div style="max-height:430px;overflow-y:auto">',
      '        <div v-for="r in queue" :key="r.mrNo"',
      '             @click="select(r)"',
      '             :style="{',
      '               padding: \'11px 16px\',',
      '               borderBottom: \'1px solid #f2f5f9\',',
      '               cursor: \'pointer\',',
      '               background: current && current.mrNo === r.mrNo ? \'#eef4ff\' : \'#fff\',',
      '               borderLeft: \'3px solid \' + (current && current.mrNo === r.mrNo ? \'#1d6ef5\' : \'transparent\')',
      '             }">',
      '          <div class="flex items-center gap-8">',
      '            <span class="mono small">{{ r.mrNo }}</span>',
      '            <span class="ml-auto muted small tnum">{{ r.dischargeDate }}</span>',
      '          </div>',
      '          <div style="font-size:13px;margin-top:2px">{{ r.name }} · {{ r.dept }}</div>',
      '          <div class="muted small">{{ r.diagMain }}</div>',
      '        </div>',
      '        <mr-empty v-if="!queue.length" text="队列已清空，全部病案均已完成编目" icon="ri-medal-line"></mr-empty>',
      '      </div>',
      '    </div>',

      /* ---------------- 编目工作台 ---------------- */
      '    <div class="card">',
      '      <div class="card-head">',
      '        <h3>编目工作台</h3>',
      '        <span class="sub" v-if="current">正在编目：{{ current.mrNo }}</span>',
      '        <span class="sub" v-else>请从左侧选择一份待编目病案</span>',
      /* 任务5：质控工作台入口（工作台顶部） */
      '        <div class="right" v-if="current">',
      '          <button class="btn btn-default btn-sm" @click="openQc()"><i class="ri-shield-check-line"></i> 质控工作台</button>',
      '        </div>',
      '      </div>',
      '      <div class="card-body">',
      '        <div v-if="!current" style="padding:34px 0">',
      '          <mr-empty text="尚未选择病案" icon="ri-folder-open-line"></mr-empty>',
      '        </div>',
      '        <template v-else>',
      '          <div class="alert info mb-12"><span class="ai">i</span>',
      '            <span>{{ current.name }}（{{ current.gender }}/{{ current.age }}）· {{ current.dept }} ·',
      '              出院 {{ current.dischargeDate }} · 主治医师 {{ current.doctor }}</span>',
      '          </div>',
      '          <div class="form-grid">',
      '            <div class="field span-2">',
      '              <div class="flex items-center gap-8">',
      '                <label style="flex:1">主要诊断 · ICD-10 编码<span class="req">*</span></label>',
      /* 任务3：智能预编码推荐（编码辅助） */
      '                <button class="btn btn-default btn-sm" type="button" @click="openSuggest(\'icd\')"><i class="ri-magic-line"></i> 智能推荐</button>',
      '              </div>',
      '              <select class="select" :value="form.icdCode" @change="pickDiag($event.target.value)">',
      '                <option value="">请选择主要诊断</option>',
      '                <option v-for="d in U.icd10Dict" :key="d.code" :value="d.code">',
      '                  {{ d.code }} {{ d.name }}（{{ d.cat }}）',
      '                </option>',
      '              </select>',
      '            </div>',
      '            <div class="field">',
      '              <label>疾病分类</label>',
      '              <input class="input" :value="form.icdCat" readonly style="background:#f7f9fc">',
      '            </div>',
      '            <div class="field">',
      '              <label>主要诊断名称</label>',
      '              <input class="input" :value="form.icdName" readonly style="background:#f7f9fc">',
      '            </div>',
      '            <div class="field span-2">',
      '              <div class="flex items-center gap-8">',
      '                <label style="flex:1">手术操作 · ICD-9-CM-3 编码</label>',
      /* 任务3：智能预编码推荐（手术操作，编码辅助） */
      '                <button class="btn btn-default btn-sm" type="button" @click="openSuggest(\'op\')"><i class="ri-magic-line"></i> 智能推荐</button>',
      '              </div>',
      '              <select class="select" :value="form.opCode" @change="pickOp($event.target.value)">',
      '                <option value="">未选择（无手术请选“无手术操作”）</option>',
      '                <option v-for="d in U.icd9Dict" :key="d.code" :value="d.code">{{ d.code }} {{ d.name }}</option>',
      '              </select>',
      '            </div>',
      '            <div class="field">',
      '              <label>病案质量等级</label>',
      '              <select class="select" v-model="form.quality">',
      '                <option>甲级</option><option>乙级</option><option>丙级</option>',
      '              </select>',
      '            </div>',
      '            <div class="field">',
      '              <label>编目人员</label>',
      '              <input class="input" :value="S.currentUser.name" readonly style="background:#f7f9fc">',
      '            </div>',
      '            <div class="field span-2">',
      '              <label>编目备注</label>',
      '              <textarea class="textarea" v-model="form.remark"',
      '                        placeholder="可记录合并编码、疑难编码的判定依据等"></textarea>',
      '            </div>',
      '          </div>',
      '          <div class="flex gap-8 mt-16" style="align-items:center">',
      /* 任务7：DRG 预检（需先填主诊断） */
      '            <button class="btn btn-default" type="button" @click="runDrg" :disabled="!form.icdCode">',
      '              <i class="ri-stethoscope-line"></i> DRG 预检',
      '            </button>',
      '            <span class="muted small">查表提示，非官方分组器</span>',
      '            <button class="btn btn-default ml-auto" @click="current = null">取消选择</button>',
      '            <button class="btn btn-primary" @click="save">保存编目结果</button>',
      '          </div>',
      '        </template>',
      /* 任务16：编目完成后展示质量评分与等级（vetoed 时红色标出否决原因） */
      '        <div v-if="!current && lastResult" class="alert mt-16"',
      '             :class="lastResult.vetoed ? \'err\' : \'ok\'">',
      '          <span class="ai"><i class="ri-shield-check-line"></i></span>',
      '          <span>',
      '            病案 {{ lastResult.mrNo }} 编目完成 · 质量评分 {{ lastResult.score }} 分 · {{ lastResult.gradeName }}',
      '            <span class="muted small">（错误 {{ lastResult.errorCount }} 项 / 预警 {{ lastResult.warnCount }} 项）</span>',
      '            <template v-if="lastResult.vetoed"><br><b>单项否决：{{ lastResult.vetoReason }}</b></template>',
      '          </span>',
      '        </div>',
      '      </div>',
      '    </div>',
      '  </div>',

      /* ---------------- 编码字典 ---------------- */
      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>ICD 编码库检索</h3>',
      '      <div class="seg ml-auto">',
      '        <button :class="{ on: dictTab === \'icd10\' }" @click="dictTab = \'icd10\'">疾病分类 ICD-10</button>',
      '        <button :class="{ on: dictTab === \'icd9\' }" @click="dictTab = \'icd9\'">手术操作 ICD-9-CM-3</button>',
      '      </div>',
      '    </div>',
      '    <div class="card-body tight" style="border-bottom:1px solid var(--border)">',
      '      <div class="toolbar">',
      '        <div class="search-box" style="max-width:300px;flex:1">',
      '          <input class="input" v-model="kw" placeholder="输入编码或名称片段检索，如 I10 / 肺炎">',
      '        </div>',
      '        <span class="muted small">共 {{ dictList.length }} 条编码</span>',
      '        <span class="muted small ml-auto">点击右侧按钮可将编码填入当前编目表单</span>',
      '      </div>',
      '    </div>',
      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead><tr>',
      '          <th style="width:130px">编码</th>',
      '          <th>名称</th>',
      '          <th v-if="dictTab === \'icd10\'" style="width:150px">疾病分类</th>',
      '          <th style="width:100px">操作</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="d in dictList" :key="d.code">',
      '            <td class="mono">{{ d.code }}</td>',
      '            <td>{{ d.name }}</td>',
      '            <td v-if="dictTab === \'icd10\'">{{ d.cat }}</td>',
      '            <td><button class="link-btn" @click="useDict(d)">填入表单</button></td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '      <mr-empty v-if="!dictList.length" text="未检索到匹配的编码" icon="ri-search-line"></mr-empty>',
      '    </div>',
      '  </div>',

      /* ---------------- 最近编目 ---------------- */
      '  <div class="card mt-16">',
      '    <div class="card-head"><h3>最近完成的编目</h3><span class="sub">最新 8 条</span></div>',
      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead><tr>',
      '          <th>病案号</th><th>患者</th><th>主要诊断</th><th>ICD 编码</th>',
      '          <th class="hide-sm">手术操作</th><th class="hide-sm">质量</th>',
      '          <th class="hide-sm">编目日期</th><th>上报状态</th><th style="width:80px">操作</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="r in done" :key="r.mrNo">',
      '            <td class="mono">{{ r.mrNo }}</td>',
      '            <td class="nowrap">{{ r.name }}</td>',
      '            <td>{{ r.diagMain }}</td>',
      '            <td><span class="tag">{{ r.icdCode }}</span></td>',
      '            <td class="hide-sm">{{ r.opName || "—" }}</td>',
      '            <td class="hide-sm"><span class="badge" :class="tone(r.quality)">{{ r.quality }}</span></td>',
      '            <td class="hide-sm tnum">{{ r.catalogDate }}</td>',
      '            <td><span class="badge" :class="tone(r.reportStatus)">{{ r.reportStatus }}</span></td>',
      /* 任务5：行内质控工作台入口 */
      '            <td><button class="link-btn" @click="openQc(r)"><i class="ri-shield-check-line"></i> 质控</button></td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '    </div>',
      '  </div>',

      /* ---------------- 智能预编码推荐（任务3） ---------------- */
      '  <mr-modal v-if="suggestOpen" title="智能预编码推荐"',
      '            :sub="suggestTarget === \'op\' ? \'手术操作 ICD-9-CM-3\' : \'主要诊断 ICD-10\'"',
      '            @close="suggestOpen = false">',
      '    <div class="alert info"><span class="ai">i</span>',
      '      <span>依据病历文本关键词命中推荐规则给出候选编码。此为<b>编码辅助</b>，最终编码由编码员判断并负责，请勿直接照搬。</span>',
      '    </div>',
      '    <div class="field mt-16">',
      '      <label>用于推荐的病历文本 / 诊断名称</label>',
      '      <textarea class="textarea" v-model="suggestText"',
      '                placeholder="如：急性阑尾炎伴穿孔，腹腔镜阑尾切除术"></textarea>',
      '    </div>',
      '    <div class="flex gap-8 mt-12">',
      '      <button class="btn btn-primary btn-sm" @click="runSuggest" :disabled="suggestLoading">',
      '        <i class="ri-magic-line"></i> {{ suggestLoading ? "识别中…" : "开始推荐" }}',
      '      </button>',
      '    </div>',
      '    <div class="divider"></div>',
      '    <div v-if="!suggestList.length" class="muted small">',
      '      {{ suggestLoading ? "正在匹配候选编码…" : "暂无候选：输入文本后点击“开始推荐”；无命中时请改用下方 ICD 编码库手工检索。" }}',
      '    </div>',
      '    <div v-else>',
      '      <div v-for="c in suggestList" :key="c.code" @click="applySuggest(c)"',
      '           style="padding:10px 12px;border:1px solid var(--border);border-radius:8px;margin-bottom:8px;cursor:pointer">',
      '        <div class="flex items-center gap-8">',
      '          <span class="tag">{{ c.code }}</span>',
      '          <span style="font-weight:600">{{ c.name || "（字典未收录名称）" }}</span>',
      '          <span class="muted small ml-auto">命中「{{ c.matchedKeyword }}」· 权重 {{ c.weight }}</span>',
      '        </div>',
      '      </div>',
      '      <div class="muted small">点击任一候选即填入当前编码字段（填入后可再人工调整）</div>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="suggestOpen = false">关闭</button>',
      '    </template>',
      '  </mr-modal>',

      /* ---------------- DRG 预检（任务7） ---------------- */
      '  <mr-modal v-if="drgOpen" title="DRG 预检提示" sub="查表提示，非官方分组器" @close="drgOpen = false">',
      '    <div v-if="drgLoading" class="muted small">正在匹配分组规则…</div>',
      '    <template v-else>',
      '      <template v-if="drgResult">',
      '        <div v-if="drgResult.matched" class="alert ok">',
      '          <span class="ai"><i class="ri-stethoscope-line"></i></span>',
      '          <span>匹配分组：<b>{{ drgResult.groupCode }}</b> {{ drgResult.groupName }} · 参考权重 <b>{{ drgResult.weight }}</b></span>',
      '        </div>',
      '        <div v-else class="alert warn">',
      '          <span class="ai">!</span>',
      /* 未匹配时后端 hint 即"未匹配…"提示，优先用后端文案，避免与下方 hint 块重复 */
      '          <span>{{ drgResult.hint || drgFallback }}</span>',
      '        </div>',
      /* 风险提示（入歧义组 QY 等）用红色强调；未匹配情形已由上一块展示，此处仅匹配成功时补充 */
      '        <div v-if="drgResult.matched && drgResult.hint" class="alert mt-12"',
      '             :class="drgResult.hint.indexOf(\'歧义组\') > -1 ? \'err\' : \'info\'">',
      '          <span class="ai">!</span><span>{{ drgResult.hint }}</span>',
      '        </div>',
      '        <div class="divider"></div>',
      /* 免责说明必须展示，避免被误认为真实分组结果 */
      '        <div class="small" style="background:var(--warning-bg);border:1px solid #f2ddb4;border-radius:8px;padding:10px 12px;color:#96600c">',
      '          <b>免责说明：</b>{{ drgResult.disclaimer }}',
      '        </div>',
      '      </template>',
      '      <div v-else class="muted small">预检失败，请稍后重试。</div>',
      '    </template>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="drgOpen = false">关闭</button>',
      '    </template>',
      '  </mr-modal>',

      /* ---------------- ADR-0002 批次 D：CC/MCC 严重度分层预览 ---------------- */
      '  <mr-modal v-if="ccmccOpen" title="CC/MCC 严重度分层预览" :sub="\'演示口径 · 非官方判定\'" :wide="true" @close="ccmccOpen = false">',
      '    <div class="alert info"><span class="ai">i</span><span>{{ CCMCC_HINT }}</span></div>',
      '    <div v-if="ccmccLoading" class="muted small">正在判读…</div>',
      '    <template v-else-if="ccmccJudgement">',
      '      <div class="flex items-center gap-8 mt-8">',
      '        <span class="badge" :class="ccmccJudgement.overallSeverity === \'MCC\' ? \'red\' : (ccmccJudgement.overallSeverity === \'CC\' ? \'orange\' : \'\')">',
      '          本例总分层：{{ ccmccJudgement.overallSeverity === \'0\' ? \'无\' : ccmccJudgement.overallSeverity }}',
      '        </span>',
      '        <span class="muted small">MCC {{ ccmccJudgement.mccCount }} 项 · CC {{ ccmccJudgement.ccCount }} 项 · 规则版本 {{ ccmccJudgement.version }}</span>',
      '      </div>',
      '      <div class="divider"></div>',
      '      <div style="font-size:13px;font-weight:600;margin-bottom:8px">其他诊断判读明细（{{ (ccmccJudgement.items || []).length }}）</div>',
      '      <div v-if="!(ccmccJudgement.items || []).length" class="alert ok"><span class="ai"><i class="ri-checkbox-circle-line"></i></span>',
      '        <span>本例未录入其他诊断。</span></div>',
      '      <div v-else>',
      '        <div v-for="(it, i) in ccmccJudgement.items" :key="i"',
      '             style="border:1px solid var(--border);border-radius:8px;padding:10px 12px;margin-bottom:8px">',
      '          <div class="flex items-center gap-8">',
      '            <span class="mono small">{{ it.code || \'—\' }}</span>',
      '            <span class="badge" :class="it.severity === \'MCC\' ? \'red\' : (it.severity === \'CC\' ? \'orange\' : \'\')">{{ it.severity === \'0\' ? \'无\' : it.severity }}</span>',
      '            <span class="muted small">命中前缀：{{ it.matchedPrefix || \'—\' }}</span>',
      '            <span class="badge ml-auto">{{ it.source || \'—\' }}</span>',
      '          </div>',
      '          <div v-if="it.excluded" class="muted small mt-8">排除原因：{{ it.excludeReason }}</div>',
      '          <div v-if="it.remark" class="muted small">{{ it.remark }}</div>',
      '        </div>',
      '      </div>',
      /* 合规红线（ADR §6）：演示数据的显著标注不可省略，文案取自后端 disclaimer，前端不自造 */
      '      <div class="alert err mt-8"><span class="ai">!</span><span>{{ ccmccJudgement.disclaimer }}</span></div>',
      '      <div class="divider"></div>',
      '      <div style="font-size:13px;font-weight:600;margin-bottom:8px">其他诊断填报（编码与标记按位置一一对应）</div>',
      '      <div class="flex gap-8" style="align-items:flex-end">',
      '        <label style="flex:2"><span class="muted small">编码（逗号分隔）</span>',
      '          <input class="input" v-model="ccmccEditCodes" placeholder="I50.900,K80.201" /></label>',
      '        <label style="flex:2"><span class="muted small">标记（0 无 / C CC / M MCC）</span>',
      '          <input class="input" v-model="ccmccEditFlags" placeholder="M,0" /></label>',
      '        <button class="btn btn-default" @click="saveCcmccOtherDiag"><i class="ri-save-line"></i> 保存并重新判读</button>',
      '      </div>',
      '    </template>',
      '    <div v-else-if="ccmccError" class="alert err"><span class="ai">!</span><span>{{ ccmccError }}</span></div>',
      '    <div v-else class="muted small">暂无判读结果。</div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="runCcmcc">重新判读</button>',
      '      <button class="btn btn-default" @click="ccmccOpen = false">关闭</button>',
      '    </template>',
      '  </mr-modal>',

      /* ---------------- 质控工作台（任务5） ---------------- */
      '  <mr-modal v-if="qcOpen" title="质控工作台" :sub="qcTarget ? qcTarget.mrNo : \'\'" :wide="true" @close="qcOpen = false">',
      '    <div v-if="qcLoading" class="muted small">正在加载质控数据…</div>',
      '    <template v-else>',
      '      <template v-if="qcData">',
      /* 首页数据摘要 */
      '        <div class="alert info"><span class="ai">i</span>',
      '          <span>{{ qcData.frontPage.patientName }} · {{ qcData.frontPage.gender }} ·',
      '            {{ qcData.frontPage.deptName }} · 主诊断',
      '            <b>{{ qcData.frontPage.mainDiagnosisCode || "—" }}</b> · 主手术',
      '            <b>{{ qcData.frontPage.mainOperationCode || "—" }}</b> · 费用 {{ fmtCost(qcData.frontPage.totalCost) }} 元</span>',
      '        </div>',
      /* 质量评分与单项否决 */
      '        <div v-if="qcData.quality" class="mt-16" style="font-size:15px">',
      '          质量评分 <b>{{ qcData.quality.score }}</b> 分 · {{ qcData.quality.gradeName }}',
      '          <span class="muted small">（错误 {{ qcData.quality.errorCount }} / 预警 {{ qcData.quality.warnCount }}）</span>',
      '          <div v-if="qcData.quality.vetoed" class="alert err mt-8"><span class="ai">!</span>',
      '            <span>单项否决：{{ qcData.quality.vetoReason }}</span></div>',
      '        </div>',
      '        <div class="divider"></div>',
      '        <div style="font-size:13px;font-weight:600;margin-bottom:8px">质控项（{{ (qcData.qcResults || []).length }}）</div>',
      '        <div v-if="!(qcData.qcResults || []).length" class="alert ok"><span class="ai"><i class="ri-checkbox-circle-line"></i></span>',
      '          <span>本份病案未发现质控问题。</span></div>',
      '        <div v-else>',
      '          <div v-for="q in qcData.qcResults" :key="q.id"',
      '               style="border:1px solid var(--border);border-radius:8px;padding:10px 12px;margin-bottom:8px">',
      '            <div class="flex items-center gap-8">',
      '              <span class="badge" :class="q.errorLevel === \'ERROR\' ? \'red\' : \'orange\'">{{ q.errorLevel }}</span>',
      '              <span class="mono small">{{ q.fieldName }}</span>',
      '              <span class="badge ml-auto" :class="qcStatusTone(q.handleStatus)">{{ qcStatusText(q.handleStatus) }}</span>',
      '            </div>',
      '            <div class="small mt-8">{{ q.errorMsg }}</div>',
      '            <div v-if="q.returnOpinion" class="muted small">退回意见：{{ q.returnOpinion }}</div>',
      '            <div v-if="q.reviewOpinion" class="muted small">复核意见：{{ q.reviewOpinion }}</div>',
      '            <div class="flex gap-8 mt-8">',
      '              <button v-if="q.handleStatus === \'OPEN\'" class="link-btn danger" @click="qcReturnItem(q)">退回临床</button>',
      '              <button v-if="q.handleStatus === \'RETURNED\'" class="link-btn" @click="qcRectifyItem(q)">模拟整改</button>',
      '              <template v-if="q.handleStatus === \'RECTIFIED\'">',
      '                <button class="link-btn" @click="qcReviewPass(q)">复核通过</button>',
      '                <button class="link-btn danger" @click="qcReviewFail(q)">复核不通过</button>',
      '              </template>',
      '              <span v-if="q.handleStatus === \'CLOSED\'" class="muted small">已闭环</span>',
      '              <span v-if="q.handleStatus === \'REJECTED_BY_QC\'" class="muted small">已退回临床（复核不通过）</span>',
      '            </div>',
      '          </div>',
      '        </div>',
      '      </template>',
      '      <div v-else class="muted small">该病案暂无质控数据（请先完成编目以触发质控）。</div>',
      '    </template>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="loadQc">刷新</button>',
      '      <button class="btn btn-default" @click="runCcmcc"><i class="ri-pulse-line"></i> CC/MCC 严重度预览</button>',
      '      <button class="btn btn-default" @click="qcOpen = false">关闭</button>',
      '    </template>',
      '  </mr-modal>',

      /* ---------------- 退回 / 复核意见输入（任务5） ---------------- */
      '  <mr-modal v-if="qcOpinionOpen"',
      '            :title="qcOpinionMode === \'return\' ? \'退回临床整改\' : \'复核不通过\'"',
      '            :sub="qcOpinionItem ? qcOpinionItem.fieldName : \'\'"',
      '            @close="qcOpinionOpen = false">',
      '    <div class="field">',
      '      <label>{{ qcOpinionMode === \'return\' ? \'退回意见\' : \'复核不通过原因\' }}</label>',
      '      <textarea class="textarea" v-model="qcOpinionText"',
      '                placeholder="请填写具体意见，将同步反馈给临床"></textarea>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="qcOpinionOpen = false">取消</button>',
      '      <button class="btn btn-primary" @click="confirmQcOpinion">确认提交</button>',
      '    </template>',
      '  </mr-modal>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Catalog = Catalog;
})();
