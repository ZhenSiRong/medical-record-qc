/* ============================================================
   rectify.js —— 「我的待整改」/「整改督办」（缺口 D4：质控退回的医生端）
   背景：质控项被病案室退回（handle_status=RETURNED / REJECTED_BY_QC）后，
   原先只有病案室自己在编目工作台点「模拟整改」，临床上并不知道要改什么。
   本页给临床医师一个真实入口：看到本科室待整改项 → 提交整改 → 病案室复核。

   权限与数据范围（口径见后端 handler/qc_ext.go 的 MyPending 注释）：
     - 临床医师（DEPT_USER）：只能看到本科室病案的待整改项，可提交整改；
     - 病案室（ADMIN）：本院全部待整改项（督办视角，只读列表，不做提交）。
   数据来源：MRMS.ext.myPendingQc / qcRectify（后端真接口）。

   缺口 D3（2026-10-09）「修正首页数据」真闭环：
     原来临床只能填一句说明（qcRectify），首页数据本身没被修正，病案室复核时问题依旧。
     现在每行（及提交弹窗内）都提供「修正首页数据」入口：
       GET /front-pages/:recordId 回填 → 编辑 4 个条件必填字段 → PUT（全量回传，防字段被清空）
       → POST /front-pages/:recordId/qc 重新质控 → 结果反馈 → 可继续提交整改（带修正说明）。
     权限现实：PUT 与 /qc 都是 ADMIN only（router.go 的 admin 组），GET 是任意登录用户。
     因此病案室（ADMIN）得到可编辑表单；临床（DEPT_USER）得到**只读**视图 + 明确说明
     「修正需病案室权限」，绝不会出现一个必然 403 的保存按钮。
   ============================================================ */
(function () {
  'use strict';
  var ref = Vue.ref, computed = Vue.computed, reactive = Vue.reactive, onMounted = Vue.onMounted;

  var Rectify = {
    name: 'Rectify',
    components: {
      MrEmpty: MRCommon.MrEmpty,
      MrModal: MRCommon.MrModal
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, E = MRMS.ext, U = MRMS.util;

      /* ------------------------------ 页面状态 ------------------------------ */
      var loading = ref(false);
      var list = ref([]);
      var scope = ref('');       // DEPT 本科室 / ALL 全院督办 / NONE 未设科室
      var deptName = ref('');
      var hint = ref('');
      var failed = ref(false);

      var submitOpen = ref(false);
      var submitItem = ref(null);
      var submitNote = ref('');
      var submitting = ref(false);

      /* ---------------- 缺口 D3：修正首页数据（真闭环）状态 ---------------- */
      var fixOpen = ref(false);
      var fixItem = ref(null);          // 当前整改项（含 recordId）
      var fixLoading = ref(false);      // 正在 GET 首页数据
      var fixSaving = ref(false);       // 正在 PUT + 重新质控
      var fixErr = ref('');             // 取数失败 / 首页不存在
      var fixFp = ref(null);            // GET 回来的首页快照（全字段回传，防未传字段被写成零值）
      var fixQc = ref(null);            // GET 回来的历史质控项
      var fixResult = ref(null);        // 最近一次「保存 + 重新质控」的返回 {status, results, quality}
      var fixPending = ref(true);       // 重跑质控后本项是否仍在待整改清单（决定可否提交整改）
      var fixSummary = ref('');         // 本次修正内容摘要（带入整改说明）
      var fixForm = reactive({
        externalCauseCode: '', externalCauseName: '',
        newbornBirthWeight: '', newbornGestationalWeeks: ''
      });

      // 病案室视角（roleCode 由 api-store.applyUser 回填，取值 ADMIN / DEPT_USER）
      var isAdmin = computed(function () { return S.currentUser.roleCode === 'ADMIN'; });
      // PUT /front-pages/:recordId 与 /qc 均为 ADMIN only：非病案室只给只读视图 + 权限说明，
      // 不渲染保存按钮（避免做一个必然 403 的入口）。
      var canEdit = computed(function () { return isAdmin.value; });
      // 退回超过 3 天仍未整改：仅作提示，不改变后端状态
      var OVERDUE_DAYS = 3;

      /* ------------------------------ 展示映射 ------------------------------ */
      // 首页字段名 → 中文，便于临床理解"要改哪一项"（未收录时原样展示后端字段名）
      var FIELD_LABEL = {
        PatientName: '患者姓名', Gender: '性别', BirthDate: '出生日期', HospitalNo: '住院号',
        DeptName: '科室', DoctorName: '医师',
        AdmitTime: '入院时间', DischargeTime: '出院时间', ActualDays: '实际住院天数',
        AdmitDiagnosis: '入院诊断', DischargeDiagnosis: '出院诊断',
        MainDiagnosisCode: '主要诊断编码', OtherDiagnosisCodes: '其他诊断编码',
        MainOperationCode: '主要手术编码', OtherOperationCodes: '其他手术编码',
        TotalCost: '住院总费用', ExternalCauseCode: '外部原因编码',
        ExternalCauseName: '外部原因名称',
        NewbornBirthWeight: '新生儿出生体重', NewbornGestationalWeeks: '新生儿孕周'
      };
      function fieldLabel(f) { return FIELD_LABEL[f] || f || '—'; }

      var STATUS = {
        RETURNED: { text: '已退回', tone: 'red' },
        REJECTED_BY_QC: { text: '复核不通过', tone: 'orange' }
      };
      function statusText(s) { return (STATUS[s] || {}).text || s || '—'; }
      function statusTone(s) { return (STATUS[s] || {}).tone || 'gray'; }

      function d16(s) { return s ? String(s).slice(0, 16).replace('T', ' ') : '—'; }

      // 最近一次"病案室反馈"时间：复核不通过（REJECTED_BY_QC）取复核时间，
      // 否则取退回时间。依据：后端 Review(FAIL) 只更新 reviewed_at，不刷新 returned_at，
      // 若一律用 returned_at 会把"刚复核不通过"的项误判成已等待很久（或 0 天）。
      function sinceOf(r) {
        if (!r) return null;
        if (r.handleStatus === 'REJECTED_BY_QC' && r.reviewedAt) return r.reviewedAt;
        return r.returnedAt || r.reviewedAt || null;
      }

      // 已等待天数（以上述最近反馈时间起算），用于临床排优先级、病案室看督办时长
      function waitedDays(r) {
        var since = sinceOf(r);
        if (!since) return 0;
        var d = U.diffDays(String(since).slice(0, 10), U.today());
        return d > 0 ? d : 0;
      }

      /* ------------------------------ 派生指标 ------------------------------ */
      var recordCount = computed(function () {
        var seen = {}, n = 0;
        list.value.forEach(function (r) { if (!seen[r.recordId]) { seen[r.recordId] = 1; n++; } });
        return n;
      });
      var overdueCount = computed(function () {
        return list.value.filter(function (r) { return waitedDays(r) >= OVERDUE_DAYS; }).length;
      });
      // 督办视角：按科室聚合待整改条数（谁家积压最多，一眼可见）
      var deptSummary = computed(function () {
        var map = {};
        list.value.forEach(function (r) {
          var k = r.deptName || '未填科室';
          map[k] = (map[k] || 0) + 1;
        });
        return Object.keys(map).map(function (k) { return { name: k, count: map[k] }; })
          .sort(function (a, b) { return b.count - a.count; });
      });

      /* ------------------------------ 数据加载 ------------------------------ */
      function load() {
        loading.value = true;
        failed.value = false;
        return E.myPendingQc().then(function (d) {
          d = d || {};
          list.value = d.list || [];
          scope.value = d.scope || '';
          deptName.value = d.deptName || '';
          hint.value = d.hint || '';
        }, function (e) {
          list.value = [];
          failed.value = true;
          A.toast((e && e.message) || '待整改清单加载失败', 'err');
        }).then(function () { loading.value = false; });
      }

      /* ================= 缺口 D3：修正首页数据（真闭环） ================= */
      // 软校验与提示（只提示不拦截）：
      //  主诊断 S/T → 外部原因编码条件必填；P → 出生体重 + 孕周（两条独立规则）
      var diagCode = computed(function () {
        return String((fixFp.value && fixFp.value.mainDiagnosisCode) || '').trim().toUpperCase();
      });
      var isInjury = computed(function () { return /^[ST]/.test(diagCode.value); });
      var isNewborn = computed(function () { return /^P/.test(diagCode.value); });
      var weightVal = computed(function () { return U.intOrNull(fixForm.newbornBirthWeight); });
      var weeksVal = computed(function () { return U.intOrNull(fixForm.newbornGestationalWeeks); });

      // 条件必填项中仍然为空的（口径与后端 qc_rule.condition_expr 对齐）
      var missingFields = computed(function () {
        var out = [];
        if (isInjury.value && !String(fixForm.externalCauseCode || '').trim()) {
          out.push('外部原因编码');
        }
        if (isNewborn.value) {
          if (weightVal.value === null) out.push('新生儿出生体重（克）');
          if (weeksVal.value === null) out.push('新生儿孕周（周）');
        }
        return out;
      });

      var injuryHint = computed(function () {
        if (!isInjury.value) return '';
        var s = '主诊断 ' + (diagCode.value || '（未填）') +
          ' 属损伤中毒病案：外部原因编码（ICD-10 第二十章 V01–Y98）为条件必填。';
        if (!String(fixForm.externalCauseCode || '').trim()) s += '当前为空，质控会判为「必填字段为空」。';
        // 后端条件引擎只支持单前缀 LIKE 'S%'，T 编码属已知漏报：如实说明，避免用户困惑
        if (/^T/.test(diagCode.value)) {
          s += '注：T 编码暂未被后端条件规则覆盖（引擎仅支持 S% 单前缀），留空也可能质控通过，仍建议按标准补填。';
        }
        return s;
      });

      var newbornHint = computed(function () {
        if (!isNewborn.value) return '';
        var s = '主诊断 ' + diagCode.value + ' 属新生儿病案：出生体重与孕周为条件必填（两条独立规则，互不顶替）。';
        if (missingFields.value.length) s += '当前尚缺：' + missingFields.value.join('、') + '。';
        return s;
      });

      // 范围软提示（不阻断提交）
      var weightWarn = computed(function () {
        var v = weightVal.value;
        if (v === null) return '';
        return (v < 500 || v > 6000)
          ? '出生体重 ' + v + ' 克超出常见范围（500–6000 克），请确认单位为克' : '';
      });
      var weeksWarn = computed(function () {
        var v = weeksVal.value;
        if (v === null) return '';
        return (v < 20 || v > 45)
          ? '孕周 ' + v + ' 周超出常见范围（20–45 周），请确认单位为周' : '';
      });
      // 非数字输入：按「未填」提交（仍不拦截，但要明说，避免用户以为提交成功）
      function badNum(v) {
        var s = String(v === null || v === undefined ? '' : v).trim();
        return s !== '' && !isFinite(Number(s));
      }
      var numWarn = computed(function () {
        var out = [];
        if (badNum(fixForm.newbornBirthWeight)) out.push('出生体重');
        if (badNum(fixForm.newbornGestationalWeeks)) out.push('孕周');
        return out;
      });
      // 已上报的首页后端禁止修改（409）；提前在界面上说明，别等用户点了才报错
      var fpLocked = computed(function () {
        return !!fixFp.value && fixFp.value.status === 'REPORTED';
      });
      // 同一份病案上其他待整改项（GET 回来的 qcResults）：修正时一并可见，避免只盯一项
      var otherPending = computed(function () {
        var cur = fixItem.value;
        return (fixQc.value || []).filter(function (x) {
          return x.id !== (cur && cur.id) &&
            (x.handleStatus === 'RETURNED' || x.handleStatus === 'REJECTED_BY_QC');
        });
      });

      function reviewFlag(x) { return x === 'ERROR' ? 'red' : 'orange'; }
      function qcMsgs(qc) {
        var arr = (qc && qc.results) || [];
        var msgs = arr.map(function (x) { return fieldLabel(x.fieldName) + '：' + (x.errorMsg || '—'); });
        return msgs.length ? msgs.join('；') : '无明细';
      }

      // 打开弹窗：先 GET 首页数据并回填（GET 任意登录用户可读，故临床也能看到真实值）
      function openFix(item) {
        if (!item || !item.recordId) { A.toast('该质控项缺少病案标识，无法读取首页数据', 'err'); return; }
        submitOpen.value = false;   // 两个弹窗互斥，避免叠两层遮罩
        fixItem.value = item;
        fixFp.value = null;
        fixQc.value = null;
        fixResult.value = null;
        fixErr.value = '';
        fixSummary.value = '';
        fixPending.value = true;    // 本项此刻就在待整改清单里（除非重跑质控后消失）
        fixForm.externalCauseCode = '';
        fixForm.externalCauseName = '';
        fixForm.newbornBirthWeight = '';
        fixForm.newbornGestationalWeeks = '';
        fixOpen.value = true;
        fixLoading.value = true;
        E.getFrontPage(item.recordId).then(function (d) {
          var fp = (d && d.frontPage) || null;
          fixFp.value = fp;
          fixQc.value = (d && d.qcResults) || [];
          if (!fp) { fixErr.value = '该病案尚无首页数据，无法在此修正。'; return; }
          fixForm.externalCauseCode = fp.externalCauseCode || '';
          fixForm.externalCauseName = fp.externalCauseName || '';
          // 三态回填：null/未采集 → 空输入框（绝不能回填成 0）
          fixForm.newbornBirthWeight =
            (fp.newbornBirthWeight === null || fp.newbornBirthWeight === undefined) ? '' : String(fp.newbornBirthWeight);
          fixForm.newbornGestationalWeeks =
            (fp.newbornGestationalWeeks === null || fp.newbornGestationalWeeks === undefined) ? '' : String(fp.newbornGestationalWeeks);
        }, function (e) {
          fixErr.value = (e && e.message) || '首页数据读取失败';
        }).then(function () { fixLoading.value = false; });
      }

      // 构建整改说明（把本次修正内容 / 仍缺项 / 权限说明带给病案室复核）
      function buildNote() {
        var lines = [];
        var qc = fixResult.value;
        if (fixSummary.value && qc) lines.push(fixSummary.value + '。');
        if (qc) {
          lines.push(qc.status === 'QC_PASSED'
            ? '重新质控结果：通过（QC_PASSED）。'
            : '重新质控结果：仍未通过（QC_REJECTED），剩余问题：' + qcMsgs(qc) + '。');
        }
        if (missingFields.value.length) lines.push('首页仍缺：' + missingFields.value.join('、') + '。');
        if (!canEdit.value) {
          lines.push('当前账号无首页数据修改权限（PUT /front-pages 仅病案室可用）：' +
            '请在电子病历系统中补充后由病案室同步，或联系病案室代改。');
        }
        return lines.join('');
      }

      // 保存 + 紧跟重新质控（保存后后端状态自动置 WAIT_QC）
      function saveFix() {
        var item = fixItem.value;
        if (!item) return;
        if (!canEdit.value) { A.toast('首页数据修正需病案室权限，请联系病案室代改', 'err'); return; }
        if (fpLocked.value) { A.toast('首页数据已上报（REPORTED），后端禁止修改', 'err'); return; }
        if (!fixFp.value) { A.toast(fixErr.value || '未取到该病案的首页数据，无法保存', 'err'); return; }
        fixSaving.value = true;
        // 出生体重/孕周留空 → null；填了数字 → 整数（U.intOrNull 归一，绝不用 0 占位）
        var edit = {
          externalCauseCode: String(fixForm.externalCauseCode || '').trim(),
          externalCauseName: String(fixForm.externalCauseName || '').trim(),
          newbornBirthWeight: weightVal.value,
          newbornGestationalWeeks: weeksVal.value
        };
        var reason = '整改闭环：修正首页数据（' + (item.recordNo || '') + ' 质控项 #' + item.id +
          ' ' + fieldLabel(item.fieldName) + '）';
        var saved = false;
        // fixFp 为 GET 回来的完整快照，saveFrontPage 内部按全量白名单拼载荷，
        // 未在 edit 中出现的字段（otherDiagnosisCodes / extJson / birthDate 等）原样回传，
        // 不会被后端「全量赋值」写成零值。
        E.saveFrontPage(item.recordId, fixFp.value, edit, reason).then(function () {
          saved = true;
          fixFp.value = Object.assign({}, fixFp.value, edit, { status: 'WAIT_QC' });
          return E.runQc(item.recordId);
        }).then(function (qc) {
          return afterQc(item, qc);
        }).catch(function (e) {
          A.toast((e && e.message) || (saved
            ? '首页数据已保存，但重新质控失败，请在质控工作台重试'
            : '首页数据保存失败'), 'err');
        }).then(function () { fixSaving.value = false; });
      }

      function afterQc(item, qc) {
        fixResult.value = qc || null;
        var passed = !!qc && qc.status === 'QC_PASSED';
        // 后端 Qc 会把首页状态改成 QC_PASSED / QC_REJECTED，同步刷新弹窗里的状态展示
        if (qc && qc.status) fixFp.value = Object.assign({}, fixFp.value, { status: qc.status });
        // 修正内容摘要：带入「提交整改」说明，让病案室复核时知道改了什么
        var parts = [];
        var code = String(fixForm.externalCauseCode || '').trim();
        var name = String(fixForm.externalCauseName || '').trim();
        if (code) parts.push('外部原因编码 ' + code + (name ? '（' + name + '）' : ''));
        if (weightVal.value !== null) parts.push('出生体重 ' + weightVal.value + ' 克');
        if (weeksVal.value !== null) parts.push('孕周 ' + weeksVal.value + ' 周');
        fixSummary.value = parts.length
          ? '已在系统内修正首页数据：' + parts.join('；')
          : '已修正首页数据（外部原因 / 新生儿字段未填）';
        A.toast(passed ? '首页数据已修正且质控通过' : '首页数据已保存，质控仍未通过，请按下方问题继续修正',
          passed ? 'ok' : 'err');
        // 重新质控是增量 upsert（缺口 D9）：命中未消除则保留同一 id 与退回痕迹，
        // 修正后不再命中则该行被删除。故本项是否还在清单里只能以重取清单为准，不做猜测。
        return load().then(function () {
          var fresh = null;
          list.value.forEach(function (x) { if (x.id === item.id) fresh = x; });
          fixPending.value = !!fresh;
          if (fresh) fixItem.value = fresh;     // 用最新一行刷新弹窗里的问题描述
          if (!fresh && passed) A.toast('该质控项已不再命中，已从待整改清单移除');
        });
      }

      /* ------------------------------ 提交整改 ------------------------------ */
      function openSubmit(item) {
        if (isAdmin.value) {
          A.toast('病案室为督办视角，整改提交请由临床医师账号完成', 'err');
          return;
        }
        submitItem.value = item;
        // 若刚在本次会话中修正过同一份病案，把修正内容预填进说明（临床仍可编辑）
        var same = fixItem.value && fixItem.value.recordId === item.recordId && fixSummary.value;
        submitNote.value = same ? buildNote() : '';
        submitOpen.value = true;
      }

      // 从「修正首页数据」弹窗跳到「提交整改」，带上修正说明
      function goSubmit() {
        var item = fixItem.value;
        if (!item) return;
        fixOpen.value = false;
        submitItem.value = item;
        submitNote.value = buildNote();
        submitOpen.value = true;
      }

      function confirmSubmit() {
        var item = submitItem.value;
        if (!item) return;
        submitting.value = true;
        E.call(E.qcRectify(item.id, submitNote.value.trim()), '整改已提交，等待病案室复核')
          .then(function () {
            submitOpen.value = false;
            load(); // 已提交项从待整改清单消失（后端置 RECTIFIED，不再属于临床待办）
          }, function () { /* toast 已由 ext.call 弹出 */ })
          .then(function () { submitting.value = false; });
      }

      onMounted(load);

      return {
        S: S, U: U,
        loading: loading, list: list, scope: scope, deptName: deptName, hint: hint, failed: failed,
        isAdmin: isAdmin, canEdit: canEdit,
        recordCount: recordCount, overdueCount: overdueCount, deptSummary: deptSummary,
        fieldLabel: fieldLabel, statusText: statusText, statusTone: statusTone,
        d16: d16, sinceOf: sinceOf, waitedDays: waitedDays, overdueDays: OVERDUE_DAYS,
        load: load, openSubmit: openSubmit, confirmSubmit: confirmSubmit,
        submitOpen: submitOpen, submitItem: submitItem, submitNote: submitNote, submitting: submitting,
        // 缺口 D3「修正首页数据」
        fixOpen: fixOpen, fixItem: fixItem, fixLoading: fixLoading, fixSaving: fixSaving,
        fixErr: fixErr, fixFp: fixFp, fixQc: fixQc, fixResult: fixResult,
        fixPending: fixPending, fixForm: fixForm, fpLocked: fpLocked,
        diagCode: diagCode, isInjury: isInjury, isNewborn: isNewborn,
        injuryHint: injuryHint, newbornHint: newbornHint, missingFields: missingFields,
        weightWarn: weightWarn, weeksWarn: weeksWarn, numWarn: numWarn,
        otherPending: otherPending,
        openFix: openFix, saveFix: saveFix, goSubmit: goSubmit,
        reviewFlag: reviewFlag
      };
    },
    template: [
      '<div>',
      /* ---------------- 角色指引 ---------------- */
      '  <div class="alert info" v-if="isAdmin">',
      '    <span class="ai"><i class="ri-information-line"></i></span>',
      '    <span>病案室督办视角：以下为<b>全院</b>已退回、等待临床整改的质控项。可按科室催办；',
      '      整改由临床医师在「我的待整改」页提交，提交后本页该项消失并回到质控工作台等待复核。',
      '      病案室可直接点「修正首页数据」代改（首页数据修正接口仅病案室可用），保存后系统自动重新质控。</span>',
      '  </div>',
      '  <div class="alert info" v-else>',
      '    <span class="ai"><i class="ri-information-line"></i></span>',
      '    <span>以下为病案室退回<b>本科室</b>病案的质控项。请在电子病历系统中完成补充/修正后，',
      '      点击「提交整改」说明处理情况，病案室将据此复核；复核不通过的会再次退回本页。',
      '      <b>首页数据修正接口（PUT /front-pages）仅病案室可用</b>，故此处「修正首页数据」为',
      '      <b>只读</b>查看：可在弹窗内看到系统当前已采集的值与仍缺的字段，再据此填写整改说明；',
      '      实际改动需由病案室代改（或在电子病历系统补充后由病案室同步）。</span>',
      '  </div>',
      '  <div class="alert err mt-8" v-if="hint">',
      '    <span class="ai">!</span><span>{{ hint }}</span>',
      '  </div>',

      /* ---------------- 指标卡 ---------------- */
      '  <div class="grid g-3 mt-16">',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-orange"><i class="ri-error-warning-line"></i></div>',
      '        <span class="k-label">{{ isAdmin ? "全院待整改质控项" : "本科室待整改质控项" }}</span></div>',
      '      <div class="k-value">{{ loading ? "—" : list.length }}<small>项</small></div>',
      '      <div class="k-foot">{{ isAdmin ? "全院已退回待临床处理" : (deptName || "本科室") + " 已退回待处理" }}</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-blue"><i class="ri-file-list-3-line"></i></div><span class="k-label">涉及病案</span></div>',
      '      <div class="k-value">{{ loading ? "—" : recordCount }}<small>份</small></div>',
      '      <div class="k-foot">一份病案可能命中多条质控项</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico" :class="overdueCount ? \'bg-red\' : \'bg-green\'">',
      '        <i class="ri-hourglass-line"></i></div><span class="k-label">退回超 {{ overdueDays }} 天未整改</span></div>',
      '      <div class="k-value" :style="{ color: overdueCount ? \'var(--danger)\' : \'\' }">{{ loading ? "—" : overdueCount }}<small>项</small></div>',
      '      <div class="k-foot">建议优先处理积压最久的项</div>',
      '    </div>',
      '  </div>',

      /* ---------------- 科室分布（仅督办视角需要） ---------------- */
      '  <div class="card mt-16" v-if="isAdmin && deptSummary.length">',
      '    <div class="card-head">',
      '      <h3>待整改科室分布</h3>',
      '      <span class="sub">按科室聚合，共 {{ deptSummary.length }} 个科室有待整改项</span>',
      '    </div>',
      '    <div class="card-body flex gap-8" style="flex-wrap:wrap">',
      '      <span class="badge" :class="d.count >= 3 ? \'red\' : \'orange\'" v-for="d in deptSummary" :key="d.name">',
      '        {{ d.name }} · {{ d.count }} 项</span>',
      '    </div>',
      '  </div>',

      /* ---------------- 待整改清单 ---------------- */
      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>{{ isAdmin ? "全院待整改质控项" : "我的待整改质控项" }}</h3>',
      '      <span class="sub">共 {{ list.length }} 项 · 按退回时间升序（最久未处理的排最前）</span>',
      '      <div class="right">',
      '        <button class="btn btn-default" @click="load">刷新</button>',
      '      </div>',
      '    </div>',
      '    <div class="table-wrap">',
      '      <div v-if="loading" class="muted small" style="padding:16px">正在加载待整改清单…</div>',
      '      <div v-else-if="failed" class="alert err" style="margin:16px">',
      '        <span class="ai">!</span><span>清单加载失败，请确认后端服务与登录状态后重试。</span>',
      '      </div>',
      '      <table class="data" v-else-if="list.length">',
      '        <thead><tr>',
      '          <th>病案号</th><th>患者</th><th>科室</th><th>质控项</th><th>问题描述</th>',
      '          <th>退回 / 复核意见</th><th class="nowrap">退回 / 复核时间</th><th>已等待</th><th>状态</th><th>操作</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="r in list" :key="r.id">',
      '            <td class="mono">{{ r.recordNo || "—" }}</td>',
      '            <td class="nowrap">{{ r.patientName || "—" }}</td>',
      '            <td>{{ r.deptName || "—" }}</td>',
      '            <td>',
      '              <div>{{ fieldLabel(r.fieldName) }}</div>',
      '              <div class="muted small" v-if="r.ruleName">{{ r.ruleName }}</div>',
      '            </td>',
      '            <td>',
      '              <span class="badge" :class="r.errorLevel === \'ERROR\' ? \'red\' : \'orange\'">{{ r.errorLevel }}</span>',
      '              <span class="small" style="margin-left:6px">{{ r.errorMsg || "—" }}</span>',
      '            </td>',
      '            <td class="small">',
      '              <div>{{ r.returnOpinion || "（病案室未填退回意见）" }}</div>',
      '              <div v-if="r.reviewOpinion" style="color:var(--danger)">',
      '                复核意见：{{ r.reviewOpinion }}</div>',
      '            </td>',
      '            <td class="nowrap tnum">',
      '              <div>{{ d16(r.returnedAt) }}</div>',
      '              <div class="muted small" v-if="r.reviewedAt">复核 {{ d16(r.reviewedAt) }}</div>',
      '            </td>',
      '            <td class="tnum" :style="{ color: waitedDays(r) >= overdueDays ? \'var(--danger)\' : \'\', fontWeight: waitedDays(r) >= overdueDays ? 700 : 400 }">',
      '              {{ waitedDays(r) }} 天</td>',
      '            <td><span class="badge" :class="statusTone(r.handleStatus)">{{ statusText(r.handleStatus) }}</span></td>',
      '            <td class="nowrap">',
      '              <div class="actions">',
      '                <button class="link-btn" @click="openFix(r)">修正首页数据</button>',
      '                <span class="muted small" v-if="!canEdit">（需病案室权限）</span>',
      '                <button v-if="!isAdmin" class="link-btn" @click="openSubmit(r)">提交整改</button>',
      '                <span v-else class="muted small">待临床整改</span>',
      '              </div>',
      '            </td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '      <mr-empty v-else',
      '                :text="isAdmin ? \'全院当前没有待整改的质控项\' : \'本科室当前没有待整改的质控项\'"></mr-empty>',
      '    </div>',
      '    <div class="card-body" v-if="!loading && !failed && !list.length">',
      '      <div class="alert ok"><span class="ai"><i class="ri-checkbox-circle-line"></i></span>',
      '        <span v-if="isAdmin">病案室暂无督办事项：所有已退回质控项均已由临床提交整改或已闭环。</span>',
      '        <span v-else>太棒了，本科室没有待整改项。当病案室退回质控项时，会出现在这里并可直接提交整改。</span>',
      '      </div>',
      '      <div class="muted small mt-8">',
      '        质控退回闭环：病案室编目质控 → 退回临床（RETURNED）→ 本页提交整改（RECTIFIED）→ 病案室复核 → 闭环（CLOSED）。',
      '        若复核不通过，该项目会回到本页继续整改。',
      '      </div>',
      '    </div>',
      '  </div>',

      /* ---------------- 提交整改（可选说明） ---------------- */
      '  <mr-modal v-if="submitOpen" title="提交整改"',
      '            :sub="submitItem ? (submitItem.recordNo + \' · \' + fieldLabel(submitItem.fieldName)) : \'\'"',
      '            @close="submitOpen = false">',
      '    <div v-if="submitItem">',
      '      <div class="alert info"><span class="ai">i</span>',
      '        <span>{{ submitItem.patientName }} · {{ submitItem.deptName }} · {{ submitItem.errorLevel }}</span>',
      '      </div>',
      '      <div class="mt-8 small"><b>问题描述：</b>{{ submitItem.errorMsg || "—" }}</div>',
      '      <div class="small"><b>退回意见：</b>{{ submitItem.returnOpinion || "（病案室未填退回意见）" }}</div>',
      '      <div class="small" v-if="submitItem.reviewOpinion" style="color:var(--danger)">',
      '        <b>复核不通过原因：</b>{{ submitItem.reviewOpinion }}</div>',
      '      <div class="muted small" v-if="submitItem.reviewedAt">上次复核时间：{{ d16(submitItem.reviewedAt) }}</div>',
      '      <div class="divider"></div>',
      /* 缺口 D3：提交弹窗内也提供「修正首页数据」入口 */
      '      <div class="flex items-center gap-8" style="flex-wrap:wrap">',
      '        <button class="btn btn-default btn-sm" @click="openFix(submitItem)">修正首页数据</button>',
      '        <span class="muted small">{{ canEdit ? "保存后自动重新质控，可据此再提交整改" : "只读查看 4 个条件必填字段（修正需病案室权限）" }}</span>',
      '      </div>',
      '      <div class="divider"></div>',
      '      <div class="field">',
      '        <label>整改说明（可选）</label>',
      '        <textarea class="textarea" v-model="submitNote"',
      '                  placeholder="例如：已在电子病历系统补充外部原因编码 V01.x00，请复核"></textarea>',
      '      </div>',
      '      <div class="muted small">说明将记录为变更留痕，供病案室复核时查看；提交后该项状态变为「已整改待复核」。</div>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="submitOpen = false">取消</button>',
      '      <button class="btn btn-primary" :disabled="submitting" @click="confirmSubmit">',
      '        {{ submitting ? "提交中…" : "确认提交" }}</button>',
      '    </template>',
      '  </mr-modal>',

      /* ---------------- 缺口 D3：修正首页数据（真闭环） ---------------- */
      '  <mr-modal v-if="fixOpen" title="修正首页数据"',
      '            :sub="fixItem ? (fixItem.recordNo + \' · \' + fieldLabel(fixItem.fieldName)) : \'\'"',
      '            wide @close="fixOpen = false">',
      '    <div v-if="fixItem">',
      '      <div class="alert info"><span class="ai">i</span>',
      '        <span>{{ fixItem.patientName || "—" }} · {{ fixItem.deptName || "—" }} · 质控项：',
      '          {{ fieldLabel(fixItem.fieldName) }}（{{ fixItem.errorLevel }}）',
      '          <span v-if="fixItem.ruleName">· {{ fixItem.ruleName }}</span></span>',
      '      </div>',
      '      <div class="small mt-8"><b>质控问题：</b>{{ fixItem.errorMsg || "—" }}</div>',
      '      <div class="small"><b>退回意见：</b>{{ fixItem.returnOpinion || "（病案室未填退回意见）" }}</div>',
      '      <div class="small" v-if="fixItem.reviewOpinion" style="color:var(--danger)">',
      '        <b>复核不通过原因：</b>{{ fixItem.reviewOpinion }}</div>',
      '      <div class="divider"></div>',

      /* 权限说明：如实告知临床为何只能看 */
      '      <div class="alert warn" v-if="!canEdit"><span class="ai">!</span>',
      '        <span>首页数据修正接口（<b>PUT /front-pages</b>）<b>仅病案室（ADMIN）可用</b>，',
      '          当前账号为「{{ S.currentUser.role || "临床医师" }}」，故此处为<b>只读</b>视图。',
      '          请在电子病历系统中补充后由病案室同步，或联系病案室代改；',
      '          下方列出的「仍缺」字段可直接带入整改说明，供病案室按图索骥。</span>',
      '      </div>',
      '      <div class="alert info" v-else><span class="ai">i</span>',
      '        <span>保存后首页状态自动变为「待质控」，系统会<b>紧接着重新质控</b>并把结论显示在下方。',
      '          未修改的字段会按读取到的原值一并回传，不会被清空。</span>',
      '      </div>',

      '      <div v-if="fixLoading" class="muted small mt-12">正在读取该病案的首页数据…</div>',
      '      <div v-else-if="fixErr" class="alert err mt-12">',
      '        <span class="ai">!</span><span>{{ fixErr }}为避免用空表单覆盖已有首页，取不到数据时不提供保存。</span>',
      '      </div>',

      '      <template v-else-if="fixFp">',
      '        <div class="alert warn mt-12" v-if="fpLocked"><span class="ai">!</span>',
      '          <span>该首页已上报（status=REPORTED），后端禁止修改，保存将返回 409。</span>',
      '        </div>',
      '        <div class="alert warn mt-8" v-if="otherPending.length"><span class="ai">!</span>',
      '          <span>该病案还有 {{ otherPending.length }} 项待整改质控项：',
      '            <span v-for="(x, i) in otherPending" :key="x.id"><span v-if="i">；</span>',
      '              {{ fieldLabel(x.fieldName) }}（{{ x.errorMsg || "—" }}）</span>。',
      '            修正后请重新质控确认是否一并消除。</span>',
      '        </div>',

      /* 4 个条件必填字段 */
      '        <div class="form-grid mt-8">',
      '          <div class="field"><label>外部原因编码</label>',
      '            <input class="input" v-model="fixForm.externalCauseCode" :disabled="!canEdit"',
      '                   placeholder="ICD-10 第二十章，如 V01.x00">',
      '            <div class="muted small">损伤中毒（主诊断 S/T）时条件必填，编码为质控校验目标</div></div>',
      '          <div class="field"><label>外部原因名称</label>',
      '            <input class="input" v-model="fixForm.externalCauseName" :disabled="!canEdit"',
      '                   placeholder="与编码配对，如 行人在道路上被机动车碰撞">',
      '            <div class="muted small">不参与质控校验，仅随编码留存</div></div>',
      '          <div class="field"><label>新生儿出生体重（克）</label>',
      '            <input class="input" type="number" min="0" v-model="fixForm.newbornBirthWeight"',
      '                   :disabled="!canEdit" placeholder="未采集请留空">',
      '            <div class="muted small" :style="{ color: weightWarn ? \'var(--warning)\' : \'\' }">',
      '              {{ weightWarn || "留空提交 null（后端把 0 视为「空」，不要用 0 占位）" }}</div></div>',
      '          <div class="field"><label>新生儿孕周（周）</label>',
      '            <input class="input" type="number" min="0" v-model="fixForm.newbornGestationalWeeks"',
      '                   :disabled="!canEdit" placeholder="未采集请留空">',
      '            <div class="muted small" :style="{ color: weeksWarn ? \'var(--warning)\' : \'\' }">',
      '              {{ weeksWarn || "留空提交 null（后端把 0 视为「空」，不要用 0 占位）" }}</div></div>',
      '        </div>',

      /* 条件必填高亮提示（只提示不拦截） */
      '        <div class="alert warn mt-12" v-if="injuryHint"><span class="ai">!</span>',
      '          <span>{{ injuryHint }}</span></div>',
      '        <div class="alert warn mt-8" v-if="newbornHint"><span class="ai">!</span>',
      '          <span>{{ newbornHint }}</span></div>',
      '        <div class="alert warn mt-8" v-if="numWarn.length"><span class="ai">!</span>',
      '          <span>以下输入不是有效数字，将按「未填」提交：{{ numWarn.join("、") }}（不阻断保存，请确认后重填）。</span></div>',
      '        <div class="muted small mt-8">',
      '          软校验：出生体重 500–6000 克、孕周 20–45 周仅作范围提示，<b>不阻断保存</b>；',
      '          两项留空一律提交 null。当前仍缺：{{ missingFields.length ? missingFields.join("、") : "无" }}。',
      '        </div>',

      /* 只读上下文：说明这些字段会原样回传 */
      '        <div class="divider"></div>',
      '        <div class="muted small">以下首页字段本次按原值一并回传（只读，避免被后端全量赋值清空）：</div>',
      '        <dl class="desc-list mt-8">',
      '          <dt>患者</dt><dd>{{ fixFp.patientName || "—" }}（{{ fixFp.gender || "—" }}）</dd>',
      '          <dt>住院号 / 科室</dt><dd class="mono">{{ fixFp.hospitalNo || "—" }} · {{ fixFp.deptName || "—" }}</dd>',
      '          <dt>出院诊断</dt><dd>{{ fixFp.dischargeDiagnosis || "—" }}',
      '            <span class="tag" v-if="fixFp.mainDiagnosisCode">{{ fixFp.mainDiagnosisCode }}</span></dd>',
      '          <dt>主要手术编码</dt><dd class="mono">{{ fixFp.mainOperationCode || "—" }}</dd>',
      '          <dt>其他诊断 / 手术编码</dt><dd class="mono">{{ fixFp.otherDiagnosisCodes || "—" }} / {{ fixFp.otherOperationCodes || "—" }}</dd>',
      '          <dt>首页状态</dt><dd><span class="badge" :class="fixFp.status === \'REPORTED\' ? \'orange\' : \'gray\'">{{ fixFp.status || "—" }}</span>',
      '            <span class="muted small" style="margin-left:6px">修正保存后自动变 WAIT_QC</span></dd>',
      '        </dl>',

      /* 重新质控结果反馈 */
      '        <div v-if="fixResult" class="mt-16">',
      '          <div class="alert" :class="fixResult.status === \'QC_PASSED\' ? \'ok\' : \'err\'">',
      '            <span class="ai">{{ fixResult.status === "QC_PASSED" ? "✓" : "!" }}</span>',
      '            <span v-if="fixResult.status === \'QC_PASSED\'">数据已修正且质控通过（QC_PASSED）。',
      '              <b v-if="!fixPending">该质控项已不再命中，已从待整改清单移除，无需再提交整改。</b>',
      '              <b v-else>该质控项仍在待整改清单（质控已通过，但该项需人工提交整改或由病案室复核闭环）。</b></span>',
      '            <span v-else>首页数据已保存，但质控仍未通过（QC_REJECTED）。',
      '              这不阻止提交整改，但请知悉问题依旧存在，建议继续按下方逐条修正。</span>',
      '          </div>',
      '          <ul class="small" v-if="fixResult.results && fixResult.results.length"',
      '              style="margin:8px 0 0 18px">',
      '            <li v-for="(x, i) in fixResult.results" :key="i">',
      '              <span class="badge" :class="reviewFlag(x.errorLevel)">{{ x.errorLevel }}</span>',
      '              {{ fieldLabel(x.fieldName) }}：{{ x.errorMsg || "—" }}</li>',
      '          </ul>',
      '          <div class="muted small mt-8" v-else>本次质控未产生新的问题项。</div>',
      '        </div>',
      '      </template>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="fixOpen = false">关闭</button>',
      '      <span class="muted small" v-if="!canEdit">只读视图 · 修正需病案室权限</span>',
      '      <button v-if="!canEdit" class="btn btn-primary" @click="goSubmit">带入说明并提交整改</button>',
      '      <button v-if="canEdit && fixFp && !fpLocked" class="btn btn-primary"',
      '              :disabled="fixSaving || fixLoading" @click="saveFix">',
      '        {{ fixSaving ? "保存并重新质控…" : "保存并重新质控" }}</button>',
      '    </template>',
      '  </mr-modal>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Rectify = Rectify;
})();
