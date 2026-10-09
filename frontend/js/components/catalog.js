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
      MrProgress: MRCommon.MrProgress
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, St = MRMS.stats, U = MRMS.util;

      var current = ref(null);
      var dictTab = ref('icd10');
      var kw = ref('');
      var search = ref('');

      // 待编目队列：已归档但尚未编目
      var queue = computed(function () {
        return S.model.records
          .filter(function (r) {
            if (r.status !== '已归档') return false;
            if (!search.value.trim()) return true;
            var k = search.value.trim().toLowerCase();
            return (r.mrNo + r.name + r.diagMain + r.dept).toLowerCase().indexOf(k) > -1;
          })
          .sort(function (a, b) { return a.dischargeDate.localeCompare(b.dischargeDate); });
      });

      // 已编目（最近 8 条）
      var done = computed(function () {
        return S.model.records
          .filter(function (r) { return r.status === '已编目'; })
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

      return {
        S: S, U: U, current: current, queue: queue, done: done, search: search,
        todayCount: todayCount, ov: ov, form: form, dictTab: dictTab, kw: kw,
        dictList: dictList, select: select, pickDiag: pickDiag, pickOp: pickOp,
        save: save, useDict: useDict, tone: MRCommon.mrTone
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
      '              <label>主要诊断 · ICD-10 编码<span class="req">*</span></label>',
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
      '              <label>手术操作 · ICD-9-CM-3 编码</label>',
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
      '          <div class="flex gap-8 mt-16" style="justify-content:flex-end">',
      '            <button class="btn btn-default" @click="current = null">取消选择</button>',
      '            <button class="btn btn-primary" @click="save">保存编目结果</button>',
      '          </div>',
      '        </template>',
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
      '          <th class="hide-sm">编目日期</th><th>上报状态</th>',
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
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '    </div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Catalog = Catalog;
})();
