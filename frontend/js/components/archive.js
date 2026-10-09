/* ============================================================
   archive.js —— 病案归档模块
   覆盖：病案登记、检索筛选、批量归档、详情查看、导出
   ============================================================ */
(function () {
  'use strict';
  var ref = Vue.ref, computed = Vue.computed, reactive = Vue.reactive, watch = Vue.watch;

  var Archive = {
    name: 'Archive',
    components: {
      MrPager: MRCommon.MrPager,
      MrEmpty: MRCommon.MrEmpty,
      MrModal: MRCommon.MrModal
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, St = MRMS.stats, U = MRMS.util;

      var filters = reactive({
        keyword: '', dept: 'all', status: 'all', from: '', to: ''
      });
      var page = ref(1);
      var size = ref(10);
      var selected = ref([]);
      var detail = ref(null);
      var showAdd = ref(false);

      var list = computed(function () {
        return St.filter({
          keyword: filters.keyword, dept: filters.dept,
          status: filters.status, from: filters.from, to: filters.to
        }).sort(function (a, b) { return b.dischargeDate.localeCompare(a.dischargeDate); });
      });

      var paged = computed(function () {
        var start = (page.value - 1) * size.value;
        return list.value.slice(start, start + size.value);
      });

      watch(
        function () {
          return [filters.keyword, filters.dept, filters.status, filters.from, filters.to];
        },
        function () { page.value = 1; }
      );

      var allChecked = computed(function () {
        return paged.value.length > 0 &&
          paged.value.every(function (r) { return selected.value.indexOf(r.mrNo) > -1; });
      });

      function toggleAll(e) {
        var ids = paged.value.map(function (r) { return r.mrNo; });
        if (e.target.checked) {
          ids.forEach(function (id) { if (selected.value.indexOf(id) === -1) selected.value.push(id); });
        } else {
          selected.value = selected.value.filter(function (id) { return ids.indexOf(id) === -1; });
        }
      }
      function toggleOne(no) {
        var i = selected.value.indexOf(no);
        if (i > -1) selected.value.splice(i, 1); else selected.value.push(no);
      }

      function resetFilters() {
        filters.keyword = ''; filters.dept = 'all'; filters.status = 'all';
        filters.from = ''; filters.to = '';
      }

      /* ---------------- 新增病案 ---------------- */
      var form = reactive({
        name: '', gender: '男', age: 45, dept: '心血管内科',
        admitDate: U.today(), dischargeDate: U.today(),
        diagMain: '', icdCode: '', icdCat: '',
        opCode: '00.0000', opName: '无手术操作',
        doctor: '张建国', fee: 8600, pageCount: 12, quality: '甲级',
        // 缺口 D3 条件必填真字段（2026-10-09）：登记时就采集，避免首页建好后无处可填。
        externalCauseCode: '', externalCauseName: '',
        newbornBirthWeight: '', newbornGestationalWeeks: ''
      });

      // 条件必填提示：主诊断 S/T（损伤中毒）→ 外部原因编码；P（新生儿）→ 出生体重 + 孕周。
      // 这里只做提示，不拦截提交（与质控规则的 ERROR 判定分离，登记环节允许先建后补）。
      var isInjury = computed(function () { return /^[ST]/.test(String(form.icdCode || '').trim().toUpperCase()); });
      var isNewborn = computed(function () { return /^P/.test(String(form.icdCode || '').trim().toUpperCase()); });
      var condHint = computed(function () {
        if (isInjury.value) return '主要诊断为损伤中毒编码：外部原因编码（ICD-10 第二十章 V01–Y98）为条件必填，建议现在填写。';
        if (isNewborn.value) return '主要诊断为新生儿编码：出生体重与孕周为条件必填（两条独立规则），建议现在填写。';
        return '';
      });
      // 体重/孕周留空 → 提交 null（后端把 0 视为「空」，故不能填 0 占位）
      var weightWarn = computed(function () {
        var v = U.intOrNull(form.newbornBirthWeight);
        return v !== null && (v < 500 || v > 6000) ? '出生体重 ' + v + ' 克超出常见范围（500–6000 克），请确认单位为克' : '';
      });
      var weeksWarn = computed(function () {
        var v = U.intOrNull(form.newbornGestationalWeeks);
        return v !== null && (v < 20 || v > 45) ? '孕周 ' + v + ' 周超出常见范围（20–45 周），请确认单位为周' : '';
      });

      function openAdd() {
        form.name = ''; form.gender = '男'; form.age = 45;
        form.diagMain = ''; form.icdCode = ''; form.icdCat = '';
        form.admitDate = U.today(); form.dischargeDate = U.today();
        form.opCode = '00.0000'; form.opName = '无手术操作';
        form.fee = 8600; form.pageCount = 12; form.quality = '甲级';
        form.externalCauseCode = ''; form.externalCauseName = '';
        form.newbornBirthWeight = ''; form.newbornGestationalWeeks = '';
        showAdd.value = true;
      }

      function onIcdChange(code) {
        var d = U.icd10Dict.find(function (x) { return x.code === code; });
        if (d) { form.diagMain = d.name; form.icdCode = d.code; form.icdCat = d.cat; }
      }
      function onOpChange(code) {
        var d = U.icd9Dict.find(function (x) { return x.code === code; });
        if (d) { form.opName = d.name; }
      }

      function submitAdd() {
        if (!form.name.trim()) { A.toast('请填写患者姓名', 'err'); return; }
        if (!form.icdCode) { A.toast('请选择主要诊断', 'err'); return; }
        if (form.dischargeDate < form.admitDate) { A.toast('出院日期不能早于入院日期', 'err'); return; }
        A.addRecord({
          name: form.name.trim(), gender: form.gender, age: +form.age, dept: form.dept,
          admitDate: form.admitDate, dischargeDate: form.dischargeDate,
          diagMain: form.diagMain, icdCode: form.icdCode, icdCat: form.icdCat,
          opCode: form.opCode, opName: form.opName, doctor: form.doctor,
          fee: +form.fee, pageCount: +form.pageCount, quality: form.quality,
          // 缺口 D3：字符串字段未填传 ''，体重/孕周未填传 null（intOrNull，绝不用 0 占位）
          externalCauseCode: form.externalCauseCode.trim(),
          externalCauseName: form.externalCauseName.trim(),
          newbornBirthWeight: U.intOrNull(form.newbornBirthWeight),
          newbornGestationalWeeks: U.intOrNull(form.newbornGestationalWeeks)
        });
        showAdd.value = false;
        // 范围超出仅提示不拦截：随成功提示一并带出，不阻断登记
        var soft = weightWarn.value || weeksWarn.value;
        A.toast('病案登记成功，已进入待归档队列' + (soft ? '（提示：' + soft + '）' : ''));
      }

      /* ---------------- 归档操作 ---------------- */
      function archiveOne(r) {
        A.archive(r.mrNo);
        A.toast('病案 ' + r.mrNo + ' 已归档');
      }
      function archiveSelected() {
        if (!selected.value.length) { A.toast('请先勾选需要归档的病案', 'err'); return; }
        var targets = pendingSelected();
        if (!targets.length) { A.toast('所选病案均已归档，无需重复操作', 'err'); return; }
        A.archiveBatch(targets);
        selected.value = [];
        A.toast('已批量归档 ' + targets.length + ' 份病案');
      }
      // 取「选中且尚未归档」的病案号集合
      function pendingSelected() {
        return selected.value.filter(function (no) {
          var r = S.model.records.find(function (x) { return x.mrNo === no; });
          return r && r.status === '未归档';
        });
      }

      function removeOne(r) {
        if (confirm('确定删除病案 ' + r.mrNo + '（' + r.name + '）吗？删除后不可恢复。')) {
          A.removeRecord(r.mrNo);
          A.toast('病案已删除');
        }
      }

      function exportCSV() {
        var rows = list.value.map(function (r) {
          return [r.mrNo, r.inpatientNo, r.name, r.gender, r.age, r.dept,
            r.admitDate, r.dischargeDate, r.stayDays, r.diagMain, r.icdCode,
            r.opName, r.doctor, r.fee, r.pageCount, r.quality, r.status,
            r.archiveDate, r.reportStatus];
        });
        U.exportCSV('病案归档清单_' + U.today() + '.csv',
          ['病案号', '住院号', '姓名', '性别', '年龄', '科室', '入院日期', '出院日期',
           '住院天数', '主要诊断', 'ICD编码', '手术操作', '主治医师', '住院费用',
           '病案页数', '质量等级', '归档状态', '归档日期', '上报状态'], rows);
        A.toast('已导出 ' + rows.length + ' 条记录');
      }

      return {
        S: S, U: U, filters: filters, page: page, size: size,
        list: list, paged: paged, selected: selected, detail: detail,
        showAdd: showAdd, form: form, allChecked: allChecked,
        toggleAll: toggleAll, toggleOne: toggleOne, resetFilters: resetFilters,
        openAdd: openAdd, submitAdd: submitAdd, onIcdChange: onIcdChange, onOpChange: onOpChange,
        condHint: condHint, weightWarn: weightWarn, weeksWarn: weeksWarn,
        archiveOne: archiveOne, archiveSelected: archiveSelected, pendingSelected: pendingSelected,
        removeOne: removeOne, exportCSV: exportCSV, tone: MRCommon.mrTone
      };
    },
    template: [
      '<div>',
      '  <div class="card">',
      '    <div class="card-head">',
      '      <h3>病案归档管理</h3>',
      '      <span class="sub">住院病案的登记、归档与流转跟踪</span>',
      '      <div class="right">',
      '        <button class="btn btn-default" @click="exportCSV">导出清单</button>',
      '        <button class="btn btn-default" :disabled="!selected.length" @click="archiveSelected">',
      '          批量归档<span v-if="selected.length">（{{ pendingSelected().length }}）</span>',
      '        </button>',
      '        <button class="btn btn-primary" @click="openAdd">+ 病案登记</button>',
      '      </div>',
      '    </div>',
      '    <div class="card-body tight" style="border-bottom:1px solid var(--border)">',
      '      <div class="toolbar">',
      '        <div class="search-box grow" style="max-width:280px">',
      '          <input class="input" v-model="filters.keyword" placeholder="搜索病案号 / 姓名 / 诊断 / 医师">',
      '        </div>',
      '        <select class="select" style="width:132px" v-model="filters.dept">',
      '          <option value="all">全部科室</option>',
      '          <option v-for="d in U.departments" :key="d" :value="d">{{ d }}</option>',
      '        </select>',
      '        <select class="select" style="width:126px" v-model="filters.status">',
      '          <option value="all">全部状态</option>',
      '          <option value="未归档">未归档</option>',
      '          <option value="待编目">待编目</option>',
      '          <option value="已归档">已归档</option>',
      '          <option value="已编目">已编目</option>',
      '        </select>',
      '        <input class="input" type="date" style="width:146px" v-model="filters.from" title="出院开始日期">',
      '        <span class="muted">至</span>',
      '        <input class="input" type="date" style="width:146px" v-model="filters.to" title="出院结束日期">',
      '        <button class="btn btn-default btn-sm" @click="resetFilters">重置</button>',
      '        <span class="muted small ml-auto">命中 {{ list.length }} 条</span>',
      '      </div>',
      '    </div>',

      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead>',
      '          <tr>',
      '            <th style="width:38px"><input type="checkbox" :checked="allChecked" @change="toggleAll"></th>',
      '            <th>病案号</th>',
      '            <th>患者</th>',
      '            <th class="hide-sm">科室</th>',
      '            <th class="nowrap">出院日期</th>',
      '            <th class="hide-sm nowrap">住院天数</th>',
      '            <th>主要诊断</th>',
      '            <th class="hide-sm">主治医师</th>',
      '            <th>归档状态</th>',
      '            <th class="hide-sm">上报状态</th>',
      '            <th style="width:150px">操作</th>',
      '          </tr>',
      '        </thead>',
      '        <tbody>',
      '          <tr v-for="r in paged" :key="r.mrNo">',
      '            <td><input type="checkbox" :checked="selected.indexOf(r.mrNo) > -1" @change="toggleOne(r.mrNo)"></td>',
      '            <td class="mono">{{ r.mrNo }}</td>',
      '            <td class="nowrap">{{ r.name }}<span class="muted small"> {{ r.gender }}/{{ r.age }}</span></td>',
      '            <td class="hide-sm">{{ r.dept }}</td>',
      '            <td class="nowrap tnum">{{ r.dischargeDate }}</td>',
      '            <td class="hide-sm tnum">{{ r.stayDays }} 天</td>',
      '            <td><span class="nowrap">{{ r.diagMain }}</span> <span class="tag">{{ r.icdCode }}</span></td>',
      '            <td class="hide-sm">{{ r.doctor }}</td>',
      '            <td><span class="badge" :class="tone(r.status)">{{ r.status }}</span></td>',
      '            <td class="hide-sm"><span class="badge" :class="tone(r.reportStatus)">{{ r.reportStatus }}</span></td>',
      '            <td>',
      '              <div class="actions">',
      '                <button class="link-btn" @click="detail = r">详情</button>',
      '                <button class="link-btn" v-if="r.status === \'未归档\'" @click="archiveOne(r)">归档</button>',
      '                <button class="link-btn danger" @click="removeOne(r)">删除</button>',
      '              </div>',
      '            </td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '      <mr-empty v-if="!paged.length" text="没有符合条件的病案记录" icon="ri-search-line"></mr-empty>',
      '    </div>',
      '    <mr-pager v-model:page="page" v-model:size="size" :total="list.length"></mr-pager>',
      '  </div>',

      /* ------------------ 详情抽屉 ------------------ */
      '  <template v-if="detail">',
      '    <div class="drawer-mask" @click="detail = null"></div>',
      '    <div class="drawer">',
      '      <div class="drawer-head">',
      '        <h3>病案详情</h3>',
      '        <span class="badge" :class="tone(detail.status)">{{ detail.status }}</span>',
      '        <button class="btn btn-default btn-sm ml-auto" @click="detail = null">关闭</button>',
      '      </div>',
      '      <div class="drawer-body">',
      '        <div class="alert info mb-12"><span class="ai">i</span>',
      '          <span>病案号为住院患者的唯一标识，归档后纸质病案按号序上架，电子病案同步至数据库。</span>',
      '        </div>',
      '        <dl class="desc-list">',
      '          <dt>病案号</dt><dd class="mono">{{ detail.mrNo }}</dd>',
      '          <dt>住院号</dt><dd class="mono">{{ detail.inpatientNo }}</dd>',
      '          <dt>患者姓名</dt><dd>{{ detail.name }}（{{ detail.gender }} / {{ detail.age }} 岁）</dd>',
      '          <dt>就诊科室</dt><dd>{{ detail.dept }}</dd>',
      '          <dt>入院日期</dt><dd class="tnum">{{ detail.admitDate }}</dd>',
      '          <dt>出院日期</dt><dd class="tnum">{{ detail.dischargeDate }}（住院 {{ detail.stayDays }} 天）</dd>',
      '          <dt>主要诊断</dt><dd>{{ detail.diagMain }} <span class="tag">{{ detail.icdCode }}</span></dd>',
      '          <dt>疾病分类</dt><dd>{{ detail.icdCat }}</dd>',
      '          <dt>手术操作</dt><dd>{{ detail.opName || "—" }} <span class="tag" v-if="detail.opCode">{{ detail.opCode }}</span></dd>',
      '          <dt>主治医师</dt><dd>{{ detail.doctor }}</dd>',
      '          <dt>住院费用</dt><dd class="tnum">¥ {{ detail.fee.toLocaleString() }}',
      '            <span v-if="!detail.fee" class="badge red" style="margin-left:6px">费用未结算</span></dd>',
      '          <dt>病案页数</dt><dd class="tnum">{{ detail.pageCount }} 页</dd>',
      '          <dt>质量等级</dt><dd><span class="badge" :class="tone(detail.quality)">{{ detail.quality }}</span></dd>',
      '          <dt>归档日期</dt><dd class="tnum">{{ detail.archiveDate || "—" }}</dd>',
      '          <dt>编目人员</dt><dd>{{ detail.coder || "—" }}</dd>',
      '          <dt>编目日期</dt><dd class="tnum">{{ detail.catalogDate || "—" }}</dd>',
      '          <dt>上报状态</dt><dd><span class="badge" :class="tone(detail.reportStatus)">{{ detail.reportStatus }}</span></dd>',
      /* ---------- 缺口 D3：条件必填字段（两行展示 4 个值） ---------- */
      '          <dt>外部原因</dt>',
      '          <dd v-if="detail.externalCauseCode || detail.externalCauseName">',
      '            <span class="tag" v-if="detail.externalCauseCode">{{ detail.externalCauseCode }}</span>',
      '            {{ detail.externalCauseName || "" }}</dd>',
      '          <dd v-else class="muted">未采集（损伤中毒病案为条件必填）</dd>',
      '          <dt>新生儿信息</dt>',
      '          <dd v-if="detail.newbornBirthWeight !== null || detail.newbornGestationalWeeks !== null">',
      '            出生体重 {{ detail.newbornBirthWeight !== null ? detail.newbornBirthWeight + " 克" : "未采集" }}',
      '            · 孕周 {{ detail.newbornGestationalWeeks !== null ? detail.newbornGestationalWeeks + " 周" : "未采集" }}</dd>',
      '          <dd v-else class="muted">未采集（新生儿病案为条件必填）</dd>',
      '        </dl>',
      '      </div>',
      '      <div class="drawer-foot">',
      '        <button class="btn btn-default" @click="detail = null">关闭</button>',
      '        <button v-if="detail.status === \'未归档\'" class="btn btn-primary"',
      '                @click="archiveOne(detail); detail = null">确认归档</button>',
      '      </div>',
      '    </div>',
      '  </template>',

      /* ------------------ 新增病案 ------------------ */
      '  <mr-modal v-if="showAdd" title="病案登记" sub="新增住院病案基础信息" wide @close="showAdd = false">',
      '    <div class="form-grid">',
      '      <div class="field"><label>患者姓名<span class="req">*</span></label>',
      '        <input class="input" v-model="form.name" placeholder="请输入姓名"></div>',
      '      <div class="field"><label>性别 / 年龄</label>',
      '        <div class="flex gap-8">',
      '          <select class="select" style="width:86px" v-model="form.gender">',
      '            <option>男</option><option>女</option>',
      '          </select>',
      '          <input class="input" type="number" min="0" max="120" v-model="form.age" style="flex:1">',
      '        </div>',
      '      </div>',
      '      <div class="field"><label>就诊科室</label>',
      '        <select class="select" v-model="form.dept">',
      '          <option v-for="d in U.departments" :key="d" :value="d">{{ d }}</option>',
      '        </select></div>',
      '      <div class="field"><label>主治医师</label>',
      '        <select class="select" v-model="form.doctor">',
      '          <option v-for="d in U.doctors" :key="d" :value="d">{{ d }}</option>',
      '        </select></div>',
      '      <div class="field"><label>入院日期<span class="req">*</span></label>',
      '        <input class="input" type="date" v-model="form.admitDate"></div>',
      '      <div class="field"><label>出院日期<span class="req">*</span></label>',
      '        <input class="input" type="date" v-model="form.dischargeDate"></div>',
      '      <div class="field span-2"><label>主要诊断（ICD-10）<span class="req">*</span></label>',
      '        <select class="select" :value="form.icdCode" @change="onIcdChange($event.target.value)">',
      '          <option value="">请选择主要诊断</option>',
      '          <option v-for="d in U.icd10Dict" :key="d.code" :value="d.code">{{ d.code }} {{ d.name }}</option>',
      '        </select></div>',
      '      <div class="field span-2"><label>手术操作（ICD-9-CM-3）</label>',
      '        <select class="select" :value="form.opCode" @change="onOpChange($event.target.value)">',
      '          <option v-for="d in U.icd9Dict" :key="d.code" :value="d.code">{{ d.code }} {{ d.name }}</option>',
      '        </select></div>',
      '      <div class="field"><label>住院费用（元）</label>',
      '        <input class="input" type="number" min="0" v-model="form.fee"></div>',
      '      <div class="field"><label>病案页数</label>',
      '        <input class="input" type="number" min="1" v-model="form.pageCount"></div>',
      '      <div class="field"><label>质量等级</label>',
      '        <select class="select" v-model="form.quality">',
      '          <option>甲级</option><option>乙级</option><option>丙级</option>',
      '        </select></div>',
      /* ---------- 缺口 D3：条件必填真字段（损伤中毒 / 新生儿） ---------- */
      '      <div class="field"><label>外部原因编码</label>',
      '        <input class="input" v-model="form.externalCauseCode" placeholder="ICD-10 第二十章，如 V01.x00">',
      '        <div class="muted small">损伤中毒（主诊断 S/T）时条件必填</div></div>',
      '      <div class="field"><label>外部原因名称</label>',
      '        <input class="input" v-model="form.externalCauseName" placeholder="与编码配对，如 行人被机动车碰撞">',
      '        <div class="muted small">不参与质控校验，仅随编码留存</div></div>',
      '      <div class="field"><label>新生儿出生体重（克）</label>',
      '        <input class="input" type="number" min="0" v-model="form.newbornBirthWeight" placeholder="未采集请留空">',
      '        <div class="muted small" :style="{ color: weightWarn ? \'var(--warning)\' : \'\' }">',
      '          {{ weightWarn || "留空按未采集提交（null，不会写 0）" }}</div></div>',
      '      <div class="field"><label>新生儿孕周（周）</label>',
      '        <input class="input" type="number" min="0" v-model="form.newbornGestationalWeeks" placeholder="未采集请留空">',
      '        <div class="muted small" :style="{ color: weeksWarn ? \'var(--warning)\' : \'\' }">',
      '          {{ weeksWarn || "留空按未采集提交（null，不会写 0）" }}</div></div>',
      '    </div>',
      '    <div class="alert warn mt-16" v-if="condHint"><span class="ai">!</span>',
      '      <span>{{ condHint }}仅作提示，不阻断登记（可在病案室首页维护或质控退回后补填）。</span>',
      '    </div>',
      '    <div class="muted small mt-8">',
      '      出生体重 500–6000 克、孕周 20–45 周为常见范围，超出仅提示不拦截；',
      '      两项留空一律提交 null——后端把 0 视为「空」，故不要用 0 占位。',
      '    </div>',
      '    <div class="alert warn mt-16"><span class="ai">!</span>',
      '      <span>登记完成后病案进入“未归档”队列，需在出院后 7 个工作日内完成归档。</span>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="showAdd = false">取消</button>',
      '      <button class="btn btn-primary" @click="submitAdd">保存并登记</button>',
      '    </template>',
      '  </mr-modal>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Archive = Archive;
})();
