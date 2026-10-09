/* ============================================================
   borrow.js —— 病案借阅模块
   覆盖：借阅申请、审批流转、归还登记、续借、逾期监控
   ============================================================ */
(function () {
  'use strict';
  var ref = Vue.ref, computed = Vue.computed, reactive = Vue.reactive;

  var Borrow = {
    name: 'Borrow',
    components: {
      MrPager: MRCommon.MrPager,
      MrEmpty: MRCommon.MrEmpty,
      MrModal: MRCommon.MrModal
    },
    setup: function () {
      var S = MRMS.state, A = MRMS.actions, U = MRMS.util;

      var filters = reactive({ keyword: '', status: 'all' });
      var page = ref(1);
      var size = ref(10);
      var showApply = ref(false);
      var rejectTarget = ref(null);
      var rejectRemark = ref('');

      // 动态计算逾期：已借出且超过应还日期
      var list = computed(function () {
        return S.model.borrows.map(function (b) {
          var overdueDays = 0;
          var st = b.status;
          if (st === '已借出') {
            var d = U.diffDays(b.dueDate, U.today());
            if (d > 0) { overdueDays = d; st = '已逾期'; }
          }
          return Object.assign({}, b, { effStatus: st, overdueDays: overdueDays });
        }).filter(function (b) {
          if (filters.status !== 'all' && b.effStatus !== filters.status) return false;
          if (filters.keyword.trim()) {
            var k = filters.keyword.trim().toLowerCase();
            return (b.id + b.mrNo + b.patientName + b.borrower + b.borrowerDept + b.reason)
              .toLowerCase().indexOf(k) > -1;
          }
          return true;
        }).sort(function (a, b) { return b.borrowDate.localeCompare(a.borrowDate); });
      });

      var paged = computed(function () {
        var s = (page.value - 1) * size.value;
        return list.value.slice(s, s + size.value);
      });

      var mast = computed(function () {
        var all = list.value;
        return {
          total: S.model.borrows.length,
          pending: all.filter(function (b) { return b.effStatus === '待审批'; }).length,
          out: all.filter(function (b) { return b.effStatus === '已借出'; }).length,
          overdue: all.filter(function (b) { return b.effStatus === '已逾期'; }).length,
          returned: all.filter(function (b) { return b.effStatus === '已归还'; }).length
        };
      });

      /* ------------------ 借阅申请 ------------------ */
      var form = reactive({
        mrNo: '', borrower: '张建国', borrowerDept: '普通外科',
        reason: '临床病例讨论', days: 14, remark: ''
      });

      // 只有已归档及以上的病案才可借阅
      var borrowable = computed(function () {
        return S.model.records.filter(function (r) { return r.status !== '未归档'; });
      });

      function openApply() {
        form.mrNo = '';
        form.borrower = S.currentUser.name;
        form.borrowerDept = '普通外科';
        form.reason = '临床病例讨论';
        form.days = 14;
        form.remark = '';
        showApply.value = true;
      }

      function submitApply() {
        if (!form.mrNo) { A.toast('请选择需要借阅的病案', 'err'); return; }
        var r = S.model.records.find(function (x) { return x.mrNo === form.mrNo; });
        var dup = S.model.borrows.find(function (b) {
          return b.mrNo === form.mrNo && (b.status === '已借出' || b.status === '待审批');
        });
        if (dup) { A.toast('该病案正处于借阅流程中（' + dup.id + '）', 'err'); return; }

        A.addBorrow({
          mrNo: form.mrNo,
          patientName: r ? r.name : '',
          dept: r ? r.dept : '',
          borrower: form.borrower,
          borrowerDept: form.borrowerDept,
          reason: form.reason,
          borrowDate: U.today(),
          remark: form.remark
        });
        showApply.value = false;
        A.toast('借阅申请已提交，等待病案室审批');
      }

      /* ------------------ 流转操作 ------------------ */
      function approve(b) { A.approveBorrow(b.id); A.toast('已批准借阅：' + b.id + '，请提醒按期归还'); }
      function giveBack(b) { A.returnBorrow(b.id); A.toast('病案 ' + b.mrNo + ' 已登记归还'); }
      function renew(b) { A.renewBorrow(b.id, 7); A.toast('已续借 7 天，新应还日期更新'); }
      function openReject(b) { rejectTarget.value = b; rejectRemark.value = ''; }
      function confirmReject() {
        A.rejectBorrow(rejectTarget.value.id, rejectRemark.value || '审批未通过');
        A.toast('已驳回借阅申请');
        rejectTarget.value = null;
      }

      function exportCSV() {
        var rows = list.value.map(function (b) {
          return [b.id, b.mrNo, b.patientName, b.dept, b.borrower, b.borrowerDept,
            b.reason, b.borrowDate, b.dueDate, b.returnDate || '', b.effStatus, b.approver || ''];
        });
        U.exportCSV('病案借阅台账_' + U.today() + '.csv',
          ['借阅单号', '病案号', '患者', '所属科室', '借阅人', '借阅人科室', '借阅事由',
           '借出日期', '应还日期', '归还日期', '状态', '审批人'], rows);
        A.toast('已导出 ' + rows.length + ' 条借阅记录');
      }

      return {
        S: S, U: U, filters: filters, page: page, size: size,
        list: list, paged: paged, mast: mast, form: form, showApply: showApply,
        borrowable: borrowable, openApply: openApply, submitApply: submitApply,
        approve: approve, giveBack: giveBack, renew: renew,
        openReject: openReject, confirmReject: confirmReject,
        rejectTarget: rejectTarget, rejectRemark: rejectRemark,
        exportCSV: exportCSV, tone: MRCommon.mrTone
      };
    },
    template: [
      '<div>',
      '  <div class="grid g-4">',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-blue"><i class="ri-book-2-line"></i></div><span class="k-label">在借病案</span></div>',
      '      <div class="k-value">{{ mast.out }}<small>份</small></div>',
      '      <div class="k-foot">累计借阅 {{ mast.total }} 次，已归还 {{ mast.returned }} 次</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-orange"><i class="ri-time-line"></i></div><span class="k-label">待审批申请</span></div>',
      '      <div class="k-value">{{ mast.pending }}<small>条</small></div>',
      '      <div class="k-foot">需病案室主管在 1 个工作日内处理</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-red"><i class="ri-alarm-warning-line"></i></div><span class="k-label">逾期未还</span></div>',
      '      <div class="k-value">{{ mast.overdue }}<small>份</small></div>',
      '      <div class="k-foot">超过应还日期，需及时催还</div>',
      '    </div>',
      '    <div class="card kpi">',
      '      <div class="k-top"><div class="k-ico bg-green"><i class="ri-loop-left-line"></i></div><span class="k-label">按期归还率</span></div>',
      '      <div class="k-value">{{ mast.returned + mast.out ? Math.round(mast.returned / (mast.returned + mast.out + mast.overdue) * 100) : 100 }}<small>%</small></div>',
      '      <div class="k-foot">以已结束借阅周期为基数计算</div>',
      '    </div>',
      '  </div>',

      '  <div class="card mt-16">',
      '    <div class="card-head">',
      '      <h3>病案借阅登记与审批</h3>',
      '      <span class="sub">按《医疗机构病案管理规定》执行借阅审批与归还登记</span>',
      '      <div class="right">',
      '        <button class="btn btn-default" @click="exportCSV">导出台账</button>',
      '        <button class="btn btn-primary" @click="openApply">+ 借阅申请</button>',
      '      </div>',
      '    </div>',
      '    <div class="card-body tight" style="border-bottom:1px solid var(--border)">',
      '      <div class="toolbar">',
      '        <div class="search-box grow" style="max-width:280px">',
      '          <input class="input" v-model="filters.keyword" placeholder="搜索单号 / 病案号 / 患者 / 借阅人">',
      '        </div>',
      '        <select class="select" style="width:132px" v-model="filters.status">',
      '          <option value="all">全部状态</option>',
      '          <option value="待审批">待审批</option>',
      '          <option value="已借出">已借出</option>',
      '          <option value="已逾期">已逾期</option>',
      '          <option value="已归还">已归还</option>',
      '          <option value="已驳回">已驳回</option>',
      '        </select>',
      '        <span class="muted small ml-auto">命中 {{ list.length }} 条</span>',
      '      </div>',
      '    </div>',
      '    <div class="table-wrap">',
      '      <table class="data">',
      '        <thead><tr>',
      '          <th>借阅单号</th><th>病案号</th><th>患者</th><th>借阅人</th>',
      '          <th class="hide-sm">借阅事由</th><th class="nowrap">借出日期</th>',
      '          <th class="nowrap">应还日期</th><th>状态</th><th style="width:170px">操作</th>',
      '        </tr></thead>',
      '        <tbody>',
      '          <tr v-for="b in paged" :key="b.id">',
      '            <td class="mono">{{ b.id }}</td>',
      '            <td class="mono">{{ b.mrNo }}</td>',
      '            <td class="nowrap">{{ b.patientName }}</td>',
      '            <td class="nowrap">{{ b.borrower }}<div class="muted small">{{ b.borrowerDept }}</div></td>',
      '            <td class="hide-sm">{{ b.reason }}</td>',
      '            <td class="nowrap tnum">{{ b.borrowDate }}</td>',
      '            <td class="nowrap tnum">',
      '              {{ b.dueDate }}',
      '              <div v-if="b.effStatus === \'已逾期\'" class="small" style="color:var(--danger)">',
      '                逾期 {{ b.overdueDays }} 天',
      '              </div>',
      '            </td>',
      '            <td><span class="badge" :class="tone(b.effStatus)">{{ b.effStatus }}</span></td>',
      '            <td>',
      '              <div class="actions">',
      '                <button v-if="b.effStatus === \'待审批\'" class="link-btn" @click="approve(b)">通过</button>',
      '                <button v-if="b.effStatus === \'待审批\'" class="link-btn danger" @click="openReject(b)">驳回</button>',
      '                <button v-if="b.effStatus === \'已借出\'" class="link-btn" @click="giveBack(b)">归还</button>',
      '                <button v-if="b.effStatus === \'已逾期\'" class="link-btn" @click="giveBack(b)">归还</button>',
      '                <button v-if="b.effStatus === \'已借出\'" class="link-btn muted" @click="renew(b)">续借</button>',
      '                <span v-if="b.effStatus === \'已归还\'" class="muted small tnum">{{ b.returnDate }} 已还</span>',
      '                <span v-if="b.effStatus === \'已驳回\'" class="muted small">—</span>',
      '              </div>',
      '            </td>',
      '          </tr>',
      '        </tbody>',
      '      </table>',
      '      <mr-empty v-if="!paged.length" text="没有符合条件的借阅记录" icon="ri-inbox-2-line"></mr-empty>',
      '    </div>',
      '    <mr-pager v-model:page="page" v-model:size="size" :total="list.length"></mr-pager>',
      '  </div>',

      /* ---------------- 借阅申请弹窗 ---------------- */
      '  <mr-modal v-if="showApply" title="病案借阅申请" sub="提交后需经病案室审批" @close="showApply = false">',
      '    <div class="form-grid">',
      '      <div class="field span-2"><label>借阅病案<span class="req">*</span></label>',
      '        <select class="select" v-model="form.mrNo">',
      '          <option value="">请选择病案（仅已归档病案可借阅）</option>',
      '          <option v-for="r in borrowable" :key="r.mrNo" :value="r.mrNo">',
      '            {{ r.mrNo }} · {{ r.name }} · {{ r.dept }} · {{ r.diagMain }}',
      '          </option>',
      '        </select></div>',
      '      <div class="field"><label>借阅人</label>',
      '        <select class="select" v-model="form.borrower">',
      '          <option v-for="d in U.doctors" :key="d" :value="d">{{ d }}</option>',
      '        </select></div>',
      '      <div class="field"><label>借阅人科室</label>',
      '        <select class="select" v-model="form.borrowerDept">',
      '          <option v-for="d in U.departments" :key="d" :value="d">{{ d }}</option>',
      '        </select></div>',
      '      <div class="field"><label>借阅事由</label>',
      '        <select class="select" v-model="form.reason">',
      '          <option>临床病例讨论</option><option>医疗质量检查</option>',
      '          <option>科研数据采集</option><option>医患纠纷取证</option>',
      '          <option>教学查房示教</option><option>医保稽核</option><option>死亡病例讨论</option>',
      '        </select></div>',
      '      <div class="field"><label>借阅期限（天）</label>',
      '        <input class="input" type="number" min="1" max="60" v-model="form.days"></div>',
      '      <div class="field span-2"><label>补充说明</label>',
      '        <textarea class="textarea" v-model="form.remark" placeholder="如涉及具体研究课题、检查批次等"></textarea></div>',
      '    </div>',
      '    <div class="alert warn mt-16"><span class="ai">!</span>',
      '      <span>病案原件一般不得外借；确需借出的，应在规定期限内归还，逾期系统将自动标记并推送催还提醒。</span>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="showApply = false">取消</button>',
      '      <button class="btn btn-primary" @click="submitApply">提交申请</button>',
      '    </template>',
      '  </mr-modal>',

      /* ---------------- 驳回弹窗 ---------------- */
      '  <mr-modal v-if="rejectTarget" title="驳回借阅申请" :sub="rejectTarget.id" @close="rejectTarget = null">',
      '    <div class="field">',
      '      <label>驳回理由</label>',
      '      <textarea class="textarea" v-model="rejectRemark"',
      '                placeholder="请说明驳回原因，将同步反馈给申请人"></textarea>',
      '    </div>',
      '    <template #foot>',
      '      <button class="btn btn-default" @click="rejectTarget = null">取消</button>',
      '      <button class="btn btn-danger" @click="confirmReject">确认驳回</button>',
      '    </template>',
      '  </mr-modal>',
      '</div>'
    ].join('\n')
  };

  window.MRPages = window.MRPages || {};
  window.MRPages.Borrow = Borrow;
})();
