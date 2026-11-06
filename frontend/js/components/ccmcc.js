/* ============================================================
   ccmcc.js —— CC/MCC 严重度分层预览 + 其他诊断录入（ADR-0002 批次 D）

   背景（见 docs/adr/0002-DRG严重度预警边界与CC-MCC表设计.md）：
     ① 后端判读层（service/ccmcc.go 的 JudgeSeverity）返回"本例总分层 +
        逐条其他诊断的严重度/命中前缀/排除原因/来源 + disclaimer"；
     ② ADR §4 强约束：任何返回严重度判断的结果**必须**带免责声明，且
        演示数据（source=demo）**必须显著标注"非官方、不得作为结算依据"**；
     ③ ADR §5：其他诊断的严重度标记是 other_diagnosis_flags，与
        other_diagnosis_codes **按逗号位置一一对应**（0/C/M），解析层
        （service/other_diagnosis.go）对"长度不等"与"重复编码"直接报错。

   本文件只做渲染与录入，不直接调接口：数据由主干经 MRMS.ext（api-store.js
   的 ext 对象）取回后传入。挂载点：MRMS.components.ccmcc。

   加载顺序约束（零构建，无打包器）：本文件须在 js/api-store.js 之后、
   js/components/layout.js 之前由 index.html 以 <script> 引入。
   ============================================================ */
(function () {
  'use strict';

  var MRMS = window.MRMS;
  if (!MRMS) {
    throw new Error('ccmcc.js 依赖 window.MRMS：请确认本文件在 js/api-store.js 之后、' +
      'js/components/layout.js 之前加载（index.html 的 script 顺序）');
  }

  /* ---------------- HTML 转义 ----------------
     本组件是直接往主干给的容器里写 innerHTML（非 Vue 模板，没有自动转义），
     故所有进入 HTML 的文本必须经 esc()。项目此前没有全局转义工具（其余组件
     全部走 Vue 模板由框架转义），这里定义并统一使用；raw 文本展示一律走
     textContent 或 esc()，不允许拼接未转义内容。 */
  function esc(v) {
    return String(v === null || v === undefined ? '' : v)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  /* ---------------- 严重度口径 ----------------
     后端同一份 JSON 里出现两种"无"：service/ccmcc.go 的 SeverityNone = "0"
     （与标记串口径一致），service/other_diagnosis.go 的 SeverityLevelNone =
     "NONE"（展示口径）。前端必须两种都认，否则界面会出现空白徽标。 */
  var SEV = {
    MCC: { text: 'MCC', tone: 'red' },
    CC: { text: 'CC', tone: 'orange' },
    NONE: { text: '无', tone: 'gray' }
  };
  function normSeverity(v) {
    var s = String(v === null || v === undefined ? '' : v).trim().toUpperCase();
    if (s === 'MCC' || s === 'M') return 'MCC';
    if (s === 'CC' || s === 'C') return 'CC';
    return 'NONE';
  }
  function sevText(v) { return SEV[normSeverity(v)].text; }
  function sevTone(v) { return SEV[normSeverity(v)].tone; }

  // 标记串取值（ADR §5 锁定：0 无 / C CC / M MCC），只认这三种，其余按"无"处理。
  function normFlag(v) {
    var s = String(v === null || v === undefined ? '' : v).trim().toUpperCase();
    if (s === 'M' || s === 'MCC') return 'M';
    if (s === 'C' || s === 'CC') return 'C';
    return '0';
  }

  // 命中规则的来源 → 界面表述（demo 必须显著区别于 official，ADR §4/§6）
  var SOURCE_TEXT = { demo: '演示数据（demo）', official: '官方表（official）' };
  function sourceText(s) {
    var k = String(s || '').trim().toLowerCase();
    return SOURCE_TEXT[k] || (k ? k : '—');
  }

  // disclaimer 缺失时的兜底提示（ADR §4：免责声明是接口契约的一部分；
  // 后端若未返回，界面也不能出现"无免责说明"的空白——那等于暗示结论可用于结算）。
  var FALLBACK_DISCLAIMER = '本判断基于院内演示用 CC/MCC 判读表（非国家医保局官方表），' +
    '仅供编码质控参考，不作为医保结算或 DRG/DIP 分组依据。';

  /* ============================================================
     一、严重度分层预览
     renderSeverityPanel(el, judgement)
       judgement（后端 CcMccJudgement）：
         { mainDiagnosisCode, version, items:[{code,severity,matchedPrefix,
           excluded,excludeReason,source,remark}],
           overallSeverity, mccCount, ccCount, sources, demoOnly, disclaimer }
     ============================================================ */
  function renderSeverityPanel(el, judgement) {
    if (!el) return;
    var j = judgement || null;

    // 无判读结果：与"判读完成但没有其他诊断"区分开——前者是没取数，后者是没数据。
    if (!j) {
      el.innerHTML = '<div class="empty"><span class="big"><i class="ri-inbox-2-line"></i></span>' +
        '尚未获取严重度判读结果（请先在编目工作台执行判读）</div>';
      return;
    }

    var items = Array.isArray(j.items) ? j.items : [];
    var mcc = Number(j.mccCount) || 0;
    var cc = Number(j.ccCount) || 0;
    var overall = normSeverity(j.overallSeverity);

    var html = [];
    html.push('<div class="ccmcc-panel">');

    /* ---- 顶部：总分层徽标 + 计数 ---- */
    html.push('<div class="flex items-center gap-8" style="flex-wrap:wrap">');
    html.push('  <span style="font-size:13px;font-weight:600">本例总分层</span>');
    html.push('  <span class="badge ' + SEV[overall].tone + '">' + SEV[overall].text + '</span>');
    html.push('  <span class="muted small">MCC ' + mcc + ' 项 · CC ' + cc +
      ' 项 · 其他诊断 ' + items.length + ' 项</span>');
    html.push('  <span class="muted small ml-auto">主诊断 <b>' +
      esc(j.mainDiagnosisCode || '—') + '</b> · 判读版本 ' + esc(j.version || '—') +
      ' · 来源 ' + esc(((j.sources || []).map(sourceText).join('、')) || '—') + '</span>');
    html.push('</div>');

    /* ---- 逐项明细 ---- */
    if (!items.length) {
      // 明确空态：种子首页的其他诊断当前全空，静默空白会被当成"功能坏了"。
      html.push('<div class="empty mt-12"><span class="big"><i class="ri-inbox-2-line"></i></span>' +
        '本例未录入其他诊断</div>');
      html.push('<div class="muted small mt-8">请在下方「其他诊断录入」中补充编码与严重度标记后重新判读：' +
        '无其他诊断时不存在 CC/MCC，总分层为「无」属正常结果。</div>');
    } else {
      html.push('<div class="table-wrap mt-12">');
      html.push('  <table class="data">');
      html.push('    <thead><tr>');
      html.push('      <th style="width:150px">编码</th>');
      html.push('      <th style="width:120px">判定</th>');
      html.push('      <th style="width:110px">命中前缀</th>');
      html.push('      <th>排除原因</th>');
      html.push('      <th style="width:150px">来源</th>');
      html.push('    </tr></thead>');
      html.push('    <tbody>');
      items.forEach(function (it) {
        it = it || {};
        var sev = normSeverity(it.severity);
        html.push('      <tr>');
        html.push('        <td class="mono">' + esc(it.code || '—') +
          (it.remark ? '<div class="muted small">' + esc(it.remark) + '</div>' : '') + '</td>');
        html.push('        <td><span class="badge ' + SEV[sev].tone + '">' + SEV[sev].text + '</span>' +
          (it.excluded ? '<span class="muted small" style="margin-left:6px">已排除</span>' : '') + '</td>');
        html.push('        <td class="mono">' + esc(it.matchedPrefix || '—') + '</td>');
        html.push('        <td class="small">' + esc(it.excludeReason || '—') + '</td>');
        html.push('        <td class="small">' + esc(sourceText(it.source)) + '</td>');
        html.push('      </tr>');
      });
      html.push('    </tbody>');
      html.push('  </table>');
      html.push('</div>');
    }

    /* ---- 免责块 ----
       demoOnly=true（命中的全是演示规则）⇒ 红色显著块，文案原样取后端 disclaimer；
       disclaimer 为空时用兜底文案，绝不留白。非演示结论同理保留免责说明，只是弱化展示。 */
    var disclaimer = String(j.disclaimer || '').trim() || FALLBACK_DISCLAIMER;
    if (j.demoOnly) {
      html.push('<div class="alert err mt-12"><span class="ai">!</span><span>' +
        '<b>非官方口径 · 不得作为结算依据：</b>' + esc(disclaimer) + '</span></div>');
    } else if (disclaimer) {
      html.push('<div class="muted small mt-12"><b>免责说明：</b>' + esc(disclaimer) + '</div>');
    }

    html.push('</div>');
    el.innerHTML = html.join('\n');
  }

  /* ============================================================
     二、其他诊断录入
     renderOtherDiagEditor(el, opts)
       opts = {
         codes:    'I50.900,K80.201',   // other_diagnosis_codes（逗号分隔）
         flags:    'M,0',               // other_diagnosis_flags（0/C/M，按位对齐）
         readonly: false,               // true ⇒ 全部禁用
         onChange: function (codesStr, flagsStr) {}
       }
     onChange 回传的两个串**由同一份行数组序列化**，槽位数恒等（后端解析层的硬约束）。
     ============================================================ */

  // 容错切分：空串 → 1 个空槽（保留一个可输入的行）；尾部逗号 → 末尾空槽；空格由后续 trim 处理。
  function splitSlots(s) {
    return String(s === null || s === undefined ? '' : s).split(',');
  }

  // codes / flags → 行数组。长度取两者槽位数的较大者，缺失侧按"空编码 / 无标记"补齐，
  // 避免"3 个编码 + 2 个标记"被静默截断成 2 行（那正是解析层要拦的错位）。
  function parseRows(codesStr, flagsStr) {
    var codes = splitSlots(codesStr);
    var flags = splitSlots(flagsStr);
    var n = Math.max(codes.length, flags.length);
    var rows = [];
    for (var i = 0; i < n; i++) {
      rows.push({
        code: String(codes[i] === undefined ? '' : codes[i]).trim(),
        flag: normFlag(flags[i])
      });
    }
    return rows;
  }

  function serializeCodes(rows) {
    return rows.map(function (r) { return String(r.code || '').trim(); }).join(',');
  }
  function serializeFlags(rows) {
    return rows.map(function (r) { return normFlag(r.flag); }).join(',');
  }

  // 重复编码（去空格、转大写后判定）：后端解析层直接拒绝（ADR §5 硬约束），
  // 保存时才报 400 用户无从下手，故录入时就地提示（只提示，不拦截编辑）。
  function duplicateCodes(rows) {
    var seen = {}, dup = [];
    rows.forEach(function (r) {
      var c = String(r.code || '').trim().toUpperCase();
      if (!c) return;
      if (seen[c]) { if (dup.indexOf(c) === -1) dup.push(c); return; }
      seen[c] = 1;
    });
    return dup;
  }

  var FLAG_OPTIONS = [
    { value: '0', text: '无' },
    { value: 'C', text: 'CC（并发症/合并症）' },
    { value: 'M', text: 'MCC（主要并发症/合并症）' }
  ];

  function renderOtherDiagEditor(el, opts) {
    if (!el) return;
    opts = opts || {};
    var readonly = !!opts.readonly;

    // 状态挂在 el 上：重复渲染（主干刷新数据）时替换状态即可，事件委托只绑一次，
    // 且输入过程中不整体重绘，避免输入框失焦。
    var st = {
      rows: parseRows(opts.codes, opts.flags),
      readonly: readonly,
      onChange: typeof opts.onChange === 'function' ? opts.onChange : null
    };
    el.__ccmccEditor = st;

    var html = [];
    html.push('<div class="ccmcc-editor">');
    html.push('  <div class="muted small">编码与严重度标记<b>按逗号位置一一对应</b>' +
      '（第 i 个编码 ↔ 第 i 个标记，取值 0 无 / C CC / M MCC）。' +
      '编码中不要输入逗号（它是分隔符），同一编码不要重复录入。</div>');
    html.push('  <div class="table-wrap mt-8">');
    html.push('    <table class="data">');
    html.push('      <thead><tr>');
    html.push('        <th style="width:48px">#</th>');
    html.push('        <th>其他诊断编码（ICD-10）</th>');
    html.push('        <th style="width:220px">严重度标记</th>');
    html.push('        <th style="width:80px">操作</th>');
    html.push('      </tr></thead>');
    html.push('      <tbody>');
    if (!st.rows.length) {
      html.push('        <tr><td colspan="4" class="muted small">暂无其他诊断，请点下方「添加一行」录入</td></tr>');
    }
    st.rows.forEach(function (r, i) {
      html.push('        <tr>');
      html.push('          <td class="tnum muted">' + (i + 1) + '</td>');
      html.push('          <td><input class="input" data-k="code" data-i="' + i + '"' +
        ' value="' + esc(r.code) + '" placeholder="如 I50.900"' +
        (readonly ? ' disabled' : '') + '></td>');
      html.push('          <td><select class="select" data-k="flag" data-i="' + i + '"' +
        (readonly ? ' disabled' : '') + '>');
      FLAG_OPTIONS.forEach(function (o) {
        html.push('            <option value="' + o.value + '"' +
          (normFlag(r.flag) === o.value ? ' selected' : '') + '>' + o.text + '</option>');
      });
      html.push('          </select></td>');
      html.push('          <td><button class="link-btn danger" data-act="del" data-i="' + i + '"' +
        (readonly ? ' disabled' : '') + '>删除</button></td>');
      html.push('        </tr>');
    });
    html.push('      </tbody>');
    html.push('    </table>');
    html.push('  </div>');
    html.push('  <div data-role="warn" style="display:none"></div>');
    html.push('  <div class="flex gap-8 mt-8" style="align-items:center;flex-wrap:wrap">');
    html.push('    <button class="btn btn-default btn-sm" data-act="add"' +
      (readonly ? ' disabled' : '') + '><i class="ri-add-line"></i> 添加一行</button>');
    if (readonly) html.push('    <span class="muted small">只读视图（当前账号无编辑权限或该首页已上报）</span>');
    html.push('  </div>');
    html.push('  <div class="muted small mt-8" data-role="raw"></div>');
    html.push('</div>');

    el.innerHTML = html.join('\n');
    paintEditor(el);
    bindEditor(el);
  }

  // 只重绘"提示区 + 当前串"，不动表格——保证正在输入的框不失焦。
  function paintEditor(el) {
    var st = el.__ccmccEditor;
    if (!st) return;
    var raw = el.querySelector('[data-role="raw"]');
    if (raw) {
      raw.textContent = '当前：codes="' + serializeCodes(st.rows) + '" · flags="' +
        serializeFlags(st.rows) + '"（两串槽位数 ' + st.rows.length + '，恒等长）';
    }
    var warn = el.querySelector('[data-role="warn"]');
    if (!warn) return;
    var dup = duplicateCodes(st.rows);
    warn.innerHTML = dup.length
      ? '<div class="alert warn mt-8"><span class="ai">!</span><span>存在重复编码：' +
        esc(dup.join('、')) + '。后端按逗号位置对齐，重复编码会被拒绝保存，请合并或删除后再提交。</span></div>'
      : '';
    warn.style.display = dup.length ? '' : 'none';
  }

  function emitChange(el) {
    var st = el.__ccmccEditor;
    if (!st || !st.onChange) return;
    st.onChange(serializeCodes(st.rows), serializeFlags(st.rows));
  }

  // 事件委托：容器内所有输入/按钮共用三个监听，重绘表格后无需重新绑定。
  function bindEditor(el) {
    if (el.__ccmccBound) return;
    el.__ccmccBound = true;

    function indexOf(target) {
      var i = parseInt(target.getAttribute('data-i'), 10);
      return isNaN(i) ? -1 : i;
    }

    // 文本输入：同步状态 → 刷提示 → 回调。逗号是分隔符，输入即剔除，防止槽位错位。
    el.addEventListener('input', function (e) {
      var st = el.__ccmccEditor;
      var t = e.target;
      if (!st || st.readonly || t.getAttribute('data-k') !== 'code') return;
      var i = indexOf(t);
      if (i < 0 || !st.rows[i]) return;
      var v = String(t.value || '').replace(/[,，]/g, '');
      if (v !== t.value) t.value = v;
      st.rows[i].code = v;
      paintEditor(el);
      emitChange(el);
    });

    // 下拉（严重度标记）：change 事件
    el.addEventListener('change', function (e) {
      var st = el.__ccmccEditor;
      var t = e.target;
      if (!st || st.readonly || t.getAttribute('data-k') !== 'flag') return;
      var i = indexOf(t);
      if (i < 0 || !st.rows[i]) return;
      st.rows[i].flag = normFlag(t.value);
      emitChange(el);
    });

    el.addEventListener('click', function (e) {
      var st = el.__ccmccEditor;
      var t = e.target;
      if (!st || st.readonly || !t.getAttribute) return;
      var act = t.getAttribute('data-act');
      if (!act) return;
      if (act === 'add') {
        st.rows.push({ code: '', flag: '0' });
        renderRows(el, st);
        emitChange(el);
        var inputs = el.querySelectorAll('input[data-k="code"]');
        if (inputs.length) inputs[inputs.length - 1].focus();
      } else if (act === 'del') {
        var i = indexOf(t);
        if (i < 0 || !st.rows[i]) return;
        st.rows.splice(i, 1);
        renderRows(el, st);
        emitChange(el);
      }
    });
  }

  // 行数变化时才整体重绘表格（增删行），状态与 opts 保持不变。
  function renderRows(el, st) {
    var opts = {
      codes: serializeCodes(st.rows),
      flags: serializeFlags(st.rows),
      readonly: st.readonly,
      onChange: st.onChange
    };
    renderOtherDiagEditor(el, opts);
  }

  MRMS.components = MRMS.components || {};
  MRMS.components.ccmcc = {
    renderSeverityPanel: renderSeverityPanel,
    renderOtherDiagEditor: renderOtherDiagEditor
  };
})();
