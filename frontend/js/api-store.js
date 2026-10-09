/* ============================================================
   api-store.js —— REST 适配层数据层（替换 store.js 的 localStorage 模拟）
   对外导出与原 store.js 同构的 window.MRMS（state/actions/stats/util），
   8 个页面组件零改动；数据来自 mrs-backend(Go) 真接口，状态投影见
   prototype/BASELINE.md。
   脚本顺序约束：本文件须在组件脚本之前加载（index.html 已保证）。
   ============================================================ */
(function () {
  'use strict';

  /* ------------------------- 后端地址与 HTTP 封装 ------------------------- */
  // 与后端同源部署（go:embed / docker）时为空串；8090 静态联调时指向 8080
  var API = (location.port === '8090' ? location.protocol + '//' + location.hostname + ':8080' : '');
  function token() { return sessionStorage.getItem('mrms_token') || ''; }
  function setToken(t) {
    if (t) sessionStorage.setItem('mrms_token', t);
    else sessionStorage.removeItem('mrms_token');
  }

  var state = Vue.reactive({
    currentUser: { name: '', role: '', dept: '' },
    route: 'login',
    toasts: [],
    sidebarOpen: false,
    model: { records: [], borrows: [], reports: [], logs: [] }
  });

  async function req(method, path, body) {
    var opt = { method: method, headers: { 'Content-Type': 'application/json' } };
    if (token()) opt.headers.Authorization = 'Bearer ' + token();
    if (body !== undefined) opt.body = JSON.stringify(body);
    var res;
    try {
      res = await fetch(API + '/api' + path, opt);
    } catch (e) {
      throw new Error('后端不可达，请先启动 mrs-backend（go run 或 docker compose up）');
    }
    var json = null;
    try { json = await res.json(); } catch (e) { /* 空响应体 */ }
    if (res.status === 401) {
      setToken('');
      state.route = 'login';
      throw new Error((json && json.msg) || '登录已失效');
    }
    if (res.status === 403) {
      throw { forbidden: true, message: (json && json.msg) || '无权限执行该操作（需要病案室管理员）' };
    }
    if (!res.ok || !json || json.code !== 0) {
      throw new Error((json && json.msg) || ('请求失败 HTTP ' + res.status));
    }
    return json.data;
  }

  // 分页拉全量（后端 size 上限 100；前端全量内存模型）
  async function fetchAll(path, max) {
    var out = [], page = 1, total = Infinity;
    var sep = path.indexOf('?') > -1 ? '&' : '?';
    while (out.length < total && page <= (max || 30)) {
      var data = await req('GET', path + sep + 'page=' + page + '&size=100');
      total = data.total || 0;
      out = out.concat(data.list || []);
      page++;
    }
    return out;
  }

  /* ------------------------- 状态投影（后端枚举 → 界面中文词） ------------------------- */
  // 注意：RECYCLED（已回收）与 ARCHIVED（已归档在架）是两个**不同**的后端状态，
  // 早期版本把两者都投影成「已归档」，导致：
  //   ①「待编目队列」按「已归档」筛选时，把已归档在架的病案也装了进来（业务上它们早已编完目）；
  //   ② 界面看不出"待编目"与"在架"的区别。
  // 现将 RECYCLED 正名为「待编目」（它就队列里的东西），ARCHIVED 保留「已归档」。
  var REC_PROJ = {
    NOT_RECYCLED: '未归档', RECYCLED: '待编目',
    CATALOGED: '已编目', ARCHIVED: '已归档', BORROWED: '已编目', SEALED: '已编目'
  };
  var BOR_PROJ = {
    APPLYING: '待审批', APPROVED: '已借出', /* 待借出中间态被「通过」链折叠 */
    BORROWED: '已借出', OVERDUE: '已逾期', RETURNED: '已归还', REJECTED: '已驳回'
  };
  var BAT_PROJ = { PENDING: '待上报', SUCCESS: '已接收', FAILED: '已退回', PARTIAL: '已退回' };

  function d10(s) { return s ? String(s).slice(0, 10) : ''; }
  function dt16(s) { return s ? String(s).slice(0, 16).replace('T', ' ') : ''; }
  function ageOf(birth) {
    if (!birth) return 0;
    var b = new Date(birth), n = new Date();
    var a = n.getFullYear() - b.getFullYear();
    return a > 0 ? a : 0;
  }
  function diffDays(a, b) {
    return Math.round((new Date(b + 'T00:00:00') - new Date(a + 'T00:00:00')) / 86400000);
  }

  /* ------------------------- 字典（响应式，bootstrap 后回填） ------------------------- */
  var CAT_MAP = {
    'I10.x00': '循环系统疾病', 'I25.101': '循环系统疾病', 'I50.901': '循环系统疾病',
    'I63.901': '神经系统疾病', 'I61.901': '神经系统疾病', 'G40.901': '神经系统疾病',
    'J18.901': '呼吸系统疾病', 'J44.101': '呼吸系统疾病', 'J06.900': '呼吸系统疾病',
    'K35.801': '消化系统疾病', 'K80.201': '消化系统疾病', 'K25.901': '消化系统疾病',
    'S72.001': '损伤与骨折', 'S06.001': '损伤与骨折', 'S82.601': '损伤与骨折',
    'C34.901': '肿瘤', 'C50.901': '肿瘤',
    'E11.901': '内分泌代谢', 'E05.901': '内分泌代谢',
    'N20.001': '泌尿生殖', 'N40.001': '泌尿生殖',
    'O80.001': '妊娠分娩', 'O82.001': '妊娠分娩',
    'A41.901': '感染性疾病'
  };
  var departments = Vue.reactive([]);
  var doctors = Vue.reactive([]);
  var icd10Dict = Vue.reactive([]);
  var icd9Dict = Vue.reactive([]);
  var deptIdByName = {};
  var icd9NameByCode = {};

  /* ------------------------- 映射：后端实体 → 组件数据形状 ------------------------- */
  var fpByRecord = {};   // recordId -> 首页
  var ridByMrNo = {};    // 病案号 -> 后端 id
  var qualityByMrNo = {}; // 编目动作质控推导的精确等级

  function mapRecord(r) {
    var fp = fpByRecord[r.id] || {};
    var icdCode = fp.mainDiagnosisCode || '';
    var stay = (r.admitTime && r.dischargeTime) ? diffDays(d10(r.admitTime), d10(r.dischargeTime)) : 0;
    var quality = qualityByMrNo[r.recordNo] ||
      (fp.status === 'QC_PASSED' ? '甲级' : fp.status === 'QC_REJECTED' ? '丙级' : fp.status === 'WAIT_QC' ? '乙级' : '');
    var reportStatus = fp.status === 'REPORTED' ? '已上报' : fp.status === 'QC_PASSED' ? '待上报' : '未上报';
    return {
      id: r.recordNo, mrNo: r.recordNo,
      inpatientNo: r.hospitalNo || '',
      name: r.patientName, gender: r.gender || '',
      age: ageOf(r.birthDate),
      dept: r.deptName || '',
      admitDate: d10(r.admitTime), dischargeDate: d10(r.dischargeTime),
      stayDays: stay > 0 ? stay : 0,
      diagMain: fp.dischargeDiagnosis || '',
      icdCode: icdCode,
      icdCat: CAT_MAP[icdCode] || '其他',
      opCode: fp.mainOperationCode || '',
      opName: icd9NameByCode[fp.mainOperationCode] || '',
      doctor: r.doctorName || '',
      fee: Number(fp.totalCost || 0),
      pageCount: null,
      quality: quality,
      status: REC_PROJ[r.status] || '未归档',
      archiveDate: d10(r.archivedAt),
      reportStatus: reportStatus,
      coder: fp.recordId ? '病案室' : '',
      catalogDate: d10(r.catalogedAt),
      // 「是否编过目」以 catalogedAt 为准，**不能用当前状态**：
      // 编目动作链在质控通过后会自动上架，病案随即变成 ARCHIVED（界面「已归档」），
      // 若用 status==='已编目' 判断，刚编完目的病案会立刻从「最近完成的编目」与
      // 「编目完成数」里消失，用户会以为没保存成功（实测：全库 51 份处于该状态）。
      cataloged: !!r.catalogedAt,
      remark: r.remark || '',
      // 缺口 D3 条件必填字段：供病案详情抽屉展示（null 表示未采集，不显示为 0）
      externalCauseCode: fp.externalCauseCode || '',
      externalCauseName: fp.externalCauseName || '',
      newbornBirthWeight: intOrNull(fp.newbornBirthWeight),
      newbornGestationalWeeks: intOrNull(fp.newbornGestationalWeeks),
      _rid: r.id, _status: r.status
    };
  }

  function mapBorrow(b) {
    var rec = ridByMrNo[b.recordNo] ? state.model.records.find(function (x) { return x._rid === b.recordId; }) : null;
    return {
      id: String(b.id),
      mrNo: b.recordNo,
      patientName: rec ? rec.name : (b.remark || ''),
      dept: rec ? rec.dept : '',
      borrower: b.borrowerName,
      borrowerDept: b.borrowerDept || '',
      reason: b.reason,
      borrowDate: d10(b.applyTime),
      dueDate: d10(b.dueTime),
      returnDate: d10(b.returnTime),
      status: BOR_PROJ[b.status] || '待审批',
      approver: b.handlerName || '',
      remark: b.remark || '',
      _rid: b.id, _status: b.status
    };
  }

  function mapBatch(b) {
    return {
      id: b.period, batchNo: b.batchNo, period: b.period,
      scope: '全院',
      total: b.totalCount, submitted: b.successCount,
      status: BAT_PROJ[b.status] || '待上报',
      submitTime: dt16(b.submitTime),
      receiver: b.status === 'SUCCESS' ? '省级卫生健康信息平台' : '',
      note: b.remark || (b.status === 'PENDING' ? '待月末关账后执行上报' : (b.status === 'FAILED' || b.status === 'PARTIAL') ? '存在回执失败明细，请在后端排查后重新上报' : ''),
      _bid: b.id, _status: b.status
    };
  }

  var OP_LABELS = [
    ['/api/archives/recycle', '病案回收归档'], ['/api/archives/shelve', '归档上架'],
    ['/api/archives/seal', '病案封存'], ['/api/records', '病案登记'],
    ['/api/catalogs', '病案编目'], ['/api/borrows', '借阅审批'],
    ['/api/front-pages', '首页数据'], ['/api/qc-results', '质控修正'],
    ['/api/report-batches', '首页数据上报'], ['/api/icd-dicts', '编码库维护'],
    ['/api/users', '账号管理'], ['/api/auth/login', '登录系统']
  ];
  function mapLog(l) {
    var op = l.operation || '';
    var act = op;
    for (var i = 0; i < OP_LABELS.length; i++) {
      if (op.indexOf(OP_LABELS[i][0]) > -1) { act = OP_LABELS[i][1]; break; }
    }
    return {
      time: dt16(l.createdAt),
      user: l.operatorName || '系统',
      act: act,
      target: l.module || '',
      type: op.indexOf('POST') === 0 ? 'green' : op.indexOf('DELETE') === 0 ? 'red' : 'blue'
    };
  }

  /* ------------------------- 拉取与回填 ------------------------- */
  async function refetchRecords() {
    var recs = await fetchAll('/records/page');
    var fps = await fetchAll('/front-pages/page');
    fpByRecord = {};
    fps.forEach(function (f) { fpByRecord[f.recordId] = f; });
    ridByMrNo = {};
    recs.forEach(function (r) { ridByMrNo[r.recordNo] = r.id; });
    var mapped = recs.map(mapRecord);
    state.model.records.splice(0, state.model.records.length);
    mapped.forEach(function (m) { state.model.records.push(m); });
  }
  async function refetchBorrows() {
    var list = await fetchAll('/borrow-ledger/page');
    var mapped = list.map(mapBorrow);
    state.model.borrows.splice(0, state.model.borrows.length);
    mapped.forEach(function (m) { state.model.borrows.push(m); });
  }
  async function refetchReports() {
    var list = await fetchAll('/report-batches/page');
    var byPeriod = {}; // 同期多批取最新
    list.forEach(function (b) {
      var old = byPeriod[b.period];
      if (!old || b.id > old.id) byPeriod[b.period] = b;
    });
    var mapped = Object.keys(byPeriod).sort().reverse().map(function (p) { return mapBatch(byPeriod[p]); });
    state.model.reports.splice(0, state.model.reports.length);
    mapped.forEach(function (m) { state.model.reports.push(m); });
  }
  async function refetchLogs() {
    var data = await req('GET', '/oper-logs/page?size=30');
    var mapped = (data.list || []).map(mapLog);
    state.model.logs.splice(0, state.model.logs.length);
    mapped.forEach(function (m) { state.model.logs.push(m); });
  }
  async function refetchDicts() {
    var d = await req('GET', '/depts/page?size=100');
    deptIdByName = {};
    var dn = [];
    (d.list || []).forEach(function (x) { deptIdByName[x.deptName] = x.id; dn.push(x.deptName); });
    departments.splice(0, departments.length); dn.forEach(function (x) { departments.push(x); });

    var doc = await req('GET', '/doctors/page?size=100');
    var dnames = (doc.list || []).map(function (x) { return x.doctorName; });
    doctors.splice(0, doctors.length); dnames.forEach(function (x) { doctors.push(x); });

    var i10 = await req('GET', '/icd-dicts/page?type=ICD10&size=100');
    var i9 = await req('GET', '/icd-dicts/page?type=ICD9CM3&size=100');
    icd9NameByCode = {};
    var i9Arr = (i9.list || []).map(function (x) { icd9NameByCode[x.code] = x.name; return { code: x.code, name: x.name }; });
    var i10Arr = (i10.list || []).map(function (x) {
      return { code: x.code, name: x.name, cat: CAT_MAP[x.code] || '其他' };
    });
    icd10Dict.splice(0, icd10Dict.length); i10Arr.forEach(function (x) { icd10Dict.push(x); });
    icd9Dict.splice(0, icd9Dict.length); i9Arr.forEach(function (x) { icd9Dict.push(x); });
  }

  function pushLocalLog(act, target, type) {
    var d = new Date();
    var hh = String(d.getHours()).padStart(2, '0'), mm = String(d.getMinutes()).padStart(2, '0');
    state.model.logs.unshift({
      time: fmtDate(new Date()) + ' ' + hh + ':' + mm,
      user: state.currentUser.name || '系统',
      act: act, target: target, type: type || 'blue'
    });
    if (state.model.logs.length > 30) state.model.logs.pop();
  }

  function fmtDate(d) {
    var m = String(d.getMonth() + 1).padStart(2, '0'), dd = String(d.getDate()).padStart(2, '0');
    return d.getFullYear() + '-' + m + '-' + dd;
  }

  async function bootstrap() {
    await Promise.all([refetchDicts(), refetchRecords()]);
    await Promise.all([refetchBorrows(), refetchReports(), refetchLogs()]);
  }

  async function login(username, password) {
    var data = await req('POST', '/auth/login', { username: username, password: password });
    setToken(data.token);
    applyUser(data.user);
    await bootstrap();
    state.route = 'dashboard';
  }

  function applyUser(u) {
    state.currentUser = {
      name: u.realName || u.username,
      role: u.roleCode === 'ADMIN' ? '病案室主管' : '临床医师',
      dept: u.deptName || '',
      // roleCode 为后端原始角色码（ADMIN / DEPT_USER），
      // 供「我的待整改」等按角色区分可见性与视角的页面使用（避免用中文角色名做判断）。
      roleCode: u.roleCode || ''
    };
  }

  async function tryResume() {
    if (!token()) { state.route = 'login'; return; }
    try {
      var me = await req('GET', '/auth/me');
      applyUser(me);
      await bootstrap();
      state.route = 'dashboard';
    } catch (e) {
      state.route = 'login';
    }
  }

  // 顺带修复（2026-10-09，本文件归属本次改动）：本文件内部多处以 `toast(...)` 裸调用提示，
  // 但作用域内并没有名为 toast 的函数（只有 actions.toast），因此这些调用运行到就抛
  // `ReferenceError: toast is not defined`——症状是「接口调用成功、成功提示不显示」，
  // 而 catch 里的 fail(e) 又会二次抛错，把失败提示也一起吞掉（静默 bug）。
  // 这里补一个别名指向 actions.toast，避免逐个改 17 处调用点。
  // 函数声明会提升；actions 在本文件稍后初始化，而这些 toast 调用都发生在初始化之后。
  function toast(msg, type) { return actions.toast(msg, type); }

  function fail(e) {
    var msg = e && e.forbidden ? e.message : (e && e.message) || '操作失败';
    toast(msg, 'err');
  }

  /* ============================================================
     首页保存载荷（缺口 D3 前置修复）：防「部分字段提交」把其它字段清空
     后端 PUT /front-pages/:recordId 是「显式白名单 + 全量赋值」语义
     （见 mrs-backend/internal/handler/frontpage.go 的 Save：
      tx.Model(&existing).Updates(map[string]any{ ... 白名单 ... })），
     **未出现在载荷中的白名单字段会被写成零值**。
     因此任何首页保存都必须先把 GET /front-pages/:recordId 读回的快照按下面
     三张清单全量回传，再叠加本次要改的字段；否则 otherDiagnosisCodes / extJson /
     birthDate / D3 新增的 externalCauseCode 等会被静默清空。
     ============================================================ */
  // string：未采集写 ''（Go string 的零值）
  var FP_TEXT = [
    'patientName', 'gender', 'hospitalNo', 'deptName', 'doctorName',
    'admitDiagnosis', 'dischargeDiagnosis', 'mainDiagnosisCode', 'otherDiagnosisCodes',
    'mainOperationCode', 'otherOperationCodes', 'extJson',
    'externalCauseCode', 'externalCauseName'
  ];
  // 数值：未采集写 0
  var FP_NUM = ['actualDays', 'totalCost'];
  // *time.Time：未采集写 null
  var FP_TIME = ['birthDate', 'admitTime', 'dischargeTime'];
  // 三态字段（*int）：null = 未采集/不适用，整数 = 已采集。
  // 绝不写 0 占位——质控引擎的 isBlank 把「指向 0 的指针」判为「空」，
  // 用 0 占位等于把「未采集」伪装成「已填 0」，会让规则语义与数据不一致。
  var FP_INT_OR_NULL = ['newbornBirthWeight', 'newbornGestationalWeeks'];

  // 空串 / null / 非数字 → null；其余 → 整数（四舍五入）。
  function intOrNull(v) {
    if (v === null || v === undefined) return null;
    if (typeof v === 'string' && v.trim() === '') return null;
    var n = Number(v);
    return isFinite(n) ? Math.round(n) : null;
  }

  // 快照 + 本次改动 → 可直接 PUT 的全量载荷
  function frontPagePayload(fp, patch) {
    var base = fp || {};
    var body = {};
    FP_TEXT.forEach(function (k) {
      body[k] = base[k] === null || base[k] === undefined ? '' : String(base[k]);
    });
    FP_NUM.forEach(function (k) { body[k] = Number(base[k]) || 0; });
    FP_TIME.forEach(function (k) { body[k] = base[k] || null; });
    FP_INT_OR_NULL.forEach(function (k) { body[k] = intOrNull(base[k]); });

    Object.keys(patch || {}).forEach(function (k) {
      if (!Object.prototype.hasOwnProperty.call(body, k)) return; // 非白名单字段后端也会忽略
      var v = patch[k];
      if (FP_TEXT.indexOf(k) > -1) body[k] = v === null || v === undefined ? '' : String(v);
      else if (FP_NUM.indexOf(k) > -1) body[k] = Number(v) || 0;
      else if (FP_TIME.indexOf(k) > -1) body[k] = v || null;
      else if (FP_INT_OR_NULL.indexOf(k) > -1) body[k] = intOrNull(v);
    });
    return body;
  }

  /* ------------------------- 业务动作（组件同步风格，内部 async） ------------------------- */
  var actions = {
    toast: function (msg, type) {
      var id = Date.now() + Math.random();
      state.toasts.push({ id: id, msg: msg, type: type || 'ok' });
      setTimeout(function () {
        var i = state.toasts.findIndex(function (t) { return t.id === id; });
        if (i > -1) state.toasts.splice(i, 1);
      }, 2600);
    },

    login: function (username, password) {
      return login(username, password).catch(function (e) { fail(e); throw e; });
    },
    logout: function () {
      setToken('');
      state.route = 'login';
    },

    reset: function () {
      toast('已接入真实数据库，重置不可用', 'err');
    },

    /* --- 病案归档 --- */
    addRecord: function (data) {
      (async function () {
        try {
          var noOp = !data.opCode || data.opCode === '00.0000';
          var created = await req('POST', '/records', {
            patientName: data.name, gender: data.gender,
            birthDate: data.age ? new Date(new Date().getFullYear() - Number(data.age), 0, 1).toISOString().slice(0, 10) : null,
            hospitalNo: data.inpatientNo || '',
            deptId: deptIdByName[data.dept] || 0, deptName: data.dept,
            doctorName: data.doctor,
            admitTime: data.admitDate ? data.admitDate + 'T09:00:00+08:00' : null,
            dischargeTime: data.dischargeDate ? data.dischargeDate + 'T09:00:00+08:00' : null
          });
          await req('PUT', '/front-pages/' + created.id, {
            patientName: data.name, gender: data.gender,
            hospitalNo: created.hospitalNo, deptName: data.dept, doctorName: data.doctor,
            admitTime: data.admitDate ? data.admitDate + 'T09:00:00+08:00' : null,
            dischargeTime: data.dischargeDate ? data.dischargeDate + 'T09:00:00+08:00' : null,
            actualDays: diffDays(data.admitDate, data.dischargeDate),
            admitDiagnosis: data.diagMain, dischargeDiagnosis: data.diagMain,
            mainDiagnosisCode: data.icdCode,
            mainOperationCode: noOp ? '' : data.opCode,
            totalCost: Number(data.fee) || 0,
            // 缺口 D3 条件必填真字段：登记表单透传。字符串字段未填传 ''，
            // 出生体重/孕周未填传 null（用 intOrNull 归一，绝不用 0 占位）。
            externalCauseCode: data.externalCauseCode || '',
            externalCauseName: data.externalCauseName || '',
            newbornBirthWeight: intOrNull(data.newbornBirthWeight),
            newbornGestationalWeeks: intOrNull(data.newbornGestationalWeeks)
          });
          await refetchRecords();
          pushLocalLog('新增病案', created.recordNo);
          toast('病案已登记：' + created.recordNo);
        } catch (e) { fail(e); }
      })();
    },

    updateRecord: function (mrNo, patch) {
      (async function () {
        try {
          var rec = state.model.records.find(function (r) { return r.mrNo === mrNo; });
          if (!rec || !rec._rid) return;
          var body = {};
          if (patch.remark !== undefined) body.remark = patch.remark;
          if (patch.dept) { body.deptName = patch.dept; body.deptId = deptIdByName[patch.dept] || 0; }
          if (patch.doctor !== undefined) body.doctorName = patch.doctor;
          await req('PUT', '/records/' + rec._rid, body);
          Object.assign(rec, patch);
          pushLocalLog('修改病案', mrNo);
        } catch (e) { fail(e); }
      })();
    },

    removeRecord: function () {
      toast('病案依法不得删除（医疗机构病历管理规定）', 'err');
    },

    archive: function (mrNo) { actions.archiveBatch([mrNo]); },

    archiveBatch: function (mrNos) {
      var list = Array.isArray(mrNos) ? mrNos : [mrNos];
      (async function () {
        try {
          var ids = list.map(function (mr) {
            var rec = state.model.records.find(function (r) { return r.mrNo === mr; });
            return rec && rec._rid;
          }).filter(Boolean);
          if (!ids.length) { toast('所选病案均不可归档', 'err'); return; }
          await req('POST', '/archives/recycle', { recordIds: ids });
          await refetchRecords();
          pushLocalLog('病案回收归档', ids.length + ' 条', 'green');
          toast('已归档 ' + ids.length + ' 条病案');
        } catch (e) { fail(e); }
      })();
    },

    /* --- 病案编目：编目 + 首页 + 自动质控 + 自动上架（动作链） --- */
    catalog: function (mrNo, patch) {
      (async function () {
        try {
          var rec = state.model.records.find(function (r) { return r.mrNo === mrNo; });
          if (!rec || !rec._rid) { toast('病案不存在', 'err'); return; }
          var rid = rec._rid;
          var noOp = !patch.opCode || patch.opCode === '00.0000';

          await req('POST', '/catalogs', {
            recordId: rid, catalogType: 'DIAGNOSIS',
            icdCode: patch.icdCode, icdName: patch.diagMain, isMain: true
          });
          if (!noOp) {
            await req('POST', '/catalogs', {
              recordId: rid, catalogType: 'OPERATION',
              icdCode: patch.opCode, icdName: patch.opName || '', isMain: true
            });
          }
          var fp = await req('GET', '/front-pages/' + rid).catch(function () { return null; });
          // 缺口 D3 数据丢失点修复：GET 回来的首页快照（fp.frontPage）必须先全量回填，
          // 再叠加本次编目要改的字段——后端 PUT 是「全量赋值」，未传的白名单字段（含
          // externalCauseCode / externalCauseName / newbornBirthWeight /
          // newbornGestationalWeeks，以及此前的 otherDiagnosisCodes / extJson / birthDate）
          // 会被直接写成零值。fp 为空（该病案尚无首页）时退化为纯新增，语义不变。
          var fpBase = (fp && fp.frontPage) ? fp.frontPage : null;
          await req('PUT', '/front-pages/' + rid, frontPagePayload(fpBase, {
            patientName: rec.name, gender: rec.gender,
            hospitalNo: rec.inpatientNo, deptName: rec.dept, doctorName: rec.doctor,
            admitTime: rec.admitDate ? rec.admitDate + 'T09:00:00+08:00' : null,
            dischargeTime: rec.dischargeDate ? rec.dischargeDate + 'T09:00:00+08:00' : null,
            actualDays: rec.stayDays,
            admitDiagnosis: patch.diagMain + '待查', dischargeDiagnosis: patch.diagMain,
            mainDiagnosisCode: patch.icdCode,
            mainOperationCode: noOp ? '' : patch.opCode,
            totalCost: rec.fee || 0
            // 注：原载荷里的 remark 不是 med_record_front_page 的列（不在后端白名单），
            // 传了也会被忽略，故此处不再传。
          }));
          var qc = await req('POST', '/front-pages/' + rid + '/qc');
          var hasErr = (qc.results || []).some(function (x) { return x.errorLevel === 'ERROR'; });
          var hasWarn = (qc.results || []).some(function (x) { return x.errorLevel === 'WARN'; });
          var quality = hasErr ? '丙级' : hasWarn ? '乙级' : '甲级';
          qualityByMrNo[mrNo] = quality;

          if (qc.status === 'QC_PASSED') {
            var shelves = await req('GET', '/shelves/page?size=50');
            var shelf = (shelves.list || []).find(function (s) { return s.usedCount < s.capacity; });
            if (shelf) {
              await req('POST', '/archives/shelve', { recordIds: [rid], shelfId: shelf.id });
            }
          }
          await refetchRecords();
          pushLocalLog('病案编目', mrNo);
          toast('编目完成，质控等级：' + quality + (qc.status === 'QC_PASSED' ? '（已自动上架）' : '（质控退回，请修正首页）'));
        } catch (e) { fail(e); }
      })();
    },

    /* --- 借阅 --- */
    addBorrow: function (data) {
      (async function () {
        try {
          var rec = state.model.records.find(function (r) { return r.mrNo === data.mrNo; });
          if (!rec || !rec._rid) { toast('病案不存在', 'err'); return; }
          var due = new Date(); due.setDate(due.getDate() + 14);
          await req('POST', '/borrows', {
            recordId: rec._rid,
            reason: data.reason || data.remark || '临床诊疗',
            dueTime: due.toISOString()
          });
          await refetchBorrows();
          pushLocalLog('提交借阅申请', data.mrNo, 'orange');
          toast('借阅申请已提交，待病案室审批');
        } catch (e) { fail(e); }
      })();
    },

    approveBorrow: function (id) {
      (async function () {
        try {
          await req('PUT', '/borrows/' + id + '/approve', { result: 'APPROVED', opinion: '同意' });
          await req('PUT', '/borrows/' + id + '/lend');
          await refetchBorrows();
          pushLocalLog('借阅审批通过', id, 'green');
          toast('已审批通过并借出');
        } catch (e) { fail(e); }
      })();
    },

    rejectBorrow: function (id, remark) {
      (async function () {
        try {
          await req('PUT', '/borrows/' + id + '/approve', { result: 'REJECTED', opinion: remark || '审批未通过' });
          await refetchBorrows();
          pushLocalLog('借阅审批驳回', id, 'red');
          toast('已驳回该借阅申请');
        } catch (e) { fail(e); }
      })();
    },

    returnBorrow: function (id) {
      (async function () {
        try {
          await req('PUT', '/borrows/' + id + '/return');
          await refetchBorrows();
          pushLocalLog('病案归还', id, 'green');
          toast('归还核销完成，病案回到在架');
        } catch (e) { fail(e); }
      })();
    },

    renewBorrow: function (id, days) {
      (async function () {
        try {
          await req('PUT', '/borrows/' + id + '/renew', { days: days || 7 });
          await refetchBorrows();
          pushLocalLog('借阅续期', id, 'orange');
          toast('已续借 ' + (days || 7) + ' 天');
        } catch (e) { fail(e); }
      })();
    },

    /* --- 上报 --- */
    submitReport: function (periodId) {
      (async function () {
        try {
          var rep = state.model.reports.find(function (x) { return x.id === periodId; });
          if (!rep || !rep._bid) { toast('批次不存在', 'err'); return; }
          var batch = await req('POST', '/report-batches/' + rep._bid + '/submit');
          await Promise.all([refetchReports(), refetchRecords()]);
          pushLocalLog('执行首页数据上报', batch.batchNo, 'orange');
          toast('上报完成：成功 ' + batch.successCount + ' 条，失败 ' + batch.failCount + ' 条',
            batch.failCount > 0 ? 'err' : 'ok');
        } catch (e) { fail(e); }
      })();
    },

    createReport: function (period) {
      // 同步返回占位批次（report.js 紧接着读 batchNo），真实批次异步回填
      var exist = state.model.reports.find(function (x) { return x.id === period; });
      if (exist) return exist;
      var total = state.model.records.filter(function (r) {
        return r.dischargeDate.slice(0, 7) === period && r.cataloged && r.reportStatus !== '已上报';
      }).length;
      var rep = {
        id: period, batchNo: 'SB' + period.replace('-', '') + '…', period: period, scope: '全院',
        total: total, submitted: 0, status: '待上报',
        submitTime: '', receiver: '', note: '新建批次，待校验后上报', _bid: 0
      };
      state.model.reports.unshift(rep);
      (async function () {
        try {
          var ids = state.model.records.filter(function (r) {
            return r.dischargeDate.slice(0, 7) === period && r.cataloged && r.reportStatus !== '已上报';
          }).map(function (r) { return r._rid; });
          var batch = await req('POST', '/report-batches', { period: period, recordIds: ids });
          await refetchReports();
          var real = state.model.reports.find(function (x) { return x.id === period; });
          if (real) real.batchNo = batch.batchNo;
          pushLocalLog('新建上报批次', batch.batchNo);
          toast('上报批次已创建：' + batch.batchNo + '（' + batch.totalCount + ' 条）');
        } catch (e) {
          var i = state.model.reports.findIndex(function (x) { return x.id === period; });
          if (i > -1) state.model.reports.splice(i, 1);
          fail(e);
        }
      })();
      return rep;
    }
  };

  /* ------------------------- 派生统计（与原 store.js 同构，内存计算） ------------------------- */
  var stats = {
    filter: function (opt) {
      opt = opt || {};
      return state.model.records.filter(function (r) {
        if (opt.dept && opt.dept !== 'all' && r.dept !== opt.dept) return false;
        if (opt.status && opt.status !== 'all' && r.status !== opt.status) return false;
        if (opt.reportStatus && opt.reportStatus !== 'all' && r.reportStatus !== opt.reportStatus) return false;
        if (opt.cat && opt.cat !== 'all' && r.icdCat !== opt.cat) return false;
        if (opt.keyword) {
          var k = opt.keyword.trim().toLowerCase();
          var hay = (r.mrNo + r.name + r.diagMain + r.icdCode + r.doctor + r.inpatientNo).toLowerCase();
          if (hay.indexOf(k) === -1) return false;
        }
        if (opt.from && r.dischargeDate < opt.from) return false;
        if (opt.to && r.dischargeDate > opt.to) return false;
        return true;
      });
    },
    overview: function () {
      var rs = state.model.records;
      var total = rs.length;
      var archived = rs.filter(function (r) { return r.status !== '未归档'; }).length;
      var cataloged = rs.filter(function (r) { return r.cataloged; }).length;
      var n = new Date();
      var thisMonth = n.getFullYear() + '-' + String(n.getMonth() + 1).padStart(2, '0');
      var monthCount = rs.filter(function (r) { return r.dischargeDate.slice(0, 7) === thisMonth; }).length;
      var pendingReport = rs.filter(function (r) { return r.reportStatus === '待上报'; }).length;
      var pendingBorrow = state.model.borrows.filter(function (b) { return b.status === '待审批'; }).length;
      var overdue = state.model.borrows.filter(function (b) { return b.status === '已逾期'; }).length;
      var feeTotal = rs.reduce(function (a, r) { return a + (r.fee || 0); }, 0);
      var stayTotal = rs.reduce(function (a, r) { return a + (r.stayDays || 0); }, 0);
      return {
        total: total, archived: archived, cataloged: cataloged,
        pendingArchive: total - archived,
        archiveRate: total ? +(archived / total * 100).toFixed(1) : 0,
        catalogRate: total ? +(cataloged / total * 100).toFixed(1) : 0,
        monthCount: monthCount,
        pendingReport: pendingReport, pendingBorrow: pendingBorrow, overdue: overdue,
        avgFee: total ? Math.round(feeTotal / total) : 0,
        avgStay: total ? +(stayTotal / total).toFixed(1) : 0
      };
    },
    monthly: function (field, months) {
      months = months || 6;
      var arr = [], n = new Date();
      for (var i = months - 1; i >= 0; i--) {
        var d = new Date(n.getFullYear(), n.getMonth() - i, 1);
        var key = d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0');
        arr.push({
          key: key,
          label: (d.getMonth() + 1) + '月',
          value: state.model.records.filter(function (r) { return r.dischargeDate.slice(0, 7) === key; }).length
        });
      }
      return arr;
    },
    byDept: function () {
      var map = {};
      state.model.records.forEach(function (r) { map[r.dept] = (map[r.dept] || 0) + 1; });
      return Object.keys(map).map(function (k) { return { name: k, value: map[k] }; })
        .sort(function (a, b) { return b.value - a.value; });
    },
    byCategory: function () {
      var map = {};
      state.model.records.forEach(function (r) { map[r.icdCat] = (map[r.icdCat] || 0) + 1; });
      return Object.keys(map).map(function (k) { return { name: k, value: map[k] }; })
        .sort(function (a, b) { return b.value - a.value; });
    },
    byStatus: function () {
      // 键名必须与 REC_PROJ 的投影值一致（RECYCLED→待编目、ARCHIVED→已归档），
      // 否则该状态会被静默算成 0：历史上「已归档」(ARCHIVED) 就从未进入此图。
      var order = ['待编目', '已归档', '已编目', '未归档'];
      var colors = { '待编目': '#d98b13', '已归档': '#12a15c', '已编目': '#1d6ef5', '未归档': '#8a94a6' };
      return order.map(function (s) {
        return {
          name: s, color: colors[s],
          value: state.model.records.filter(function (r) { return r.status === s; }).length
        };
      });
    },
    reportProgress: function () {
      // 「已编目病案」以 catalogedAt 为准（编过目即算，含已自动上架的）
      var rs = state.model.records.filter(function (r) { return r.cataloged; });
      var done = rs.filter(function (r) { return r.reportStatus === '已上报'; }).length;
      return { total: rs.length, done: done, rate: rs.length ? +(done / rs.length * 100).toFixed(1) : 0 };
    },
    validate: function (period) {
      var list = state.model.records.filter(function (r) {
        return r.dischargeDate.slice(0, 7) === period && r.cataloged;
      });
      var issues = [];
      list.forEach(function (r) {
        var errs = [];
        if (!r.icdCode) errs.push('主诊断 ICD 编码缺失');
        if (!r.opCode) errs.push('手术操作编码缺失');
        if (!r.doctor) errs.push('主治医师缺失');
        if (!r.fee || r.fee <= 0) errs.push('住院费用异常');
        if (errs.length) issues.push({ mrNo: r.mrNo, name: r.name, errs: errs });
      });
      return {
        period: period, total: list.length,
        pass: list.length - issues.length, issues: issues,
        rate: list.length ? +((list.length - issues.length) / list.length * 100).toFixed(1) : 100
      };
    }
  };

  /* ------------------------- 日期工具与导出（与原 util 同构） ------------------------- */
  function fmt(d) {
    var y = d.getFullYear();
    var m = String(d.getMonth() + 1).padStart(2, '0');
    var dd = String(d.getDate()).padStart(2, '0');
    return y + '-' + m + '-' + dd;
  }
  function addDays(d, n) {
    var x = new Date(d.getTime());
    x.setDate(x.getDate() + n);
    return x;
  }
  function today() { return fmt(new Date()); }

  var util = {
    today: today, fmt: fmt, addDays: addDays, diffDays: diffDays,
    // 三态整数解析（'' / null / 非数字 → null，其余 → 整数）：
    // 新生儿出生体重/孕周等「可空且 0 视为空」的字段在前端统一用它归一，避免 0 占位。
    intOrNull: intOrNull,
    departments: departments, doctors: doctors, icd10Dict: icd10Dict, icd9Dict: icd9Dict,
    persist: function () { /* 服务端为事实来源，本地不再持久化 */ },
    exportCSV: function (filename, headers, rows) {
      var csv = '\ufeff' + headers.join(',') + '\n' +
        rows.map(function (r) {
          return r.map(function (c) {
            var s = c === null || c === undefined ? '' : String(c);
            return '"' + s.replace(/"/g, '""') + '"';
          }).join(',');
        }).join('\n');
      var blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
      var a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(a.href);
    },
    exportJSON: function (filename, obj) {
      var blob = new Blob([JSON.stringify(obj, null, 2)], { type: 'application/json' });
      var a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(a.href);
    },
    importJSON: function (file, cb) {
      var reader = new FileReader();
      reader.onload = function () {
        try { cb(JSON.parse(reader.result)); } catch (e) { cb(null); }
      };
      reader.readAsText(file);
    }
  };

  stats.doctors = function () { return doctors.slice(); };
  stats.departments = function () { return departments.slice(); };
  stats.icd10 = function () { return icd10Dict.slice(); };
  stats.icd9 = function () { return icd9Dict.slice(); };

  /* ============================================================
     ext —— 功能扩展批次（2026-10-09）新增能力的统一调用入口
     与 actions 分离，避免污染既有接口；页面用 MRMS.ext.xxx() 调用。
     ============================================================ */
  var ext = {
    /* ---- 任务 3：智能编码辅助（规则版预编码）---- */
    suggestCodes: function (text, type) {
      return req('POST', '/code-suggest', { text: text, type: type || '' });
    },
    suggestionRulesPage: function (params) {
      var qs = Object.keys(params || {}).map(function (k) { return k + '=' + encodeURIComponent(params[k]); }).join('&');
      return req('GET', '/code-suggestion-rules/page' + (qs ? '?' + qs : ''));
    },

    /* ---- 任务 7：DRG/DIP 预分组（查表版）---- */
    drgPreview: function (payload) {
      return req('POST', '/drg-preview', payload);
    },
    drgRulesPage: function (params) {
      var qs = Object.keys(params || {}).map(function (k) { return k + '=' + encodeURIComponent(params[k]); }).join('&');
      return req('GET', '/drg-group-rules/page' + (qs ? '?' + qs : ''));
    },

    /* ---- 任务 16：病历质量评分 ---- */
    quality: function (recordId) {
      return req('GET', '/front-pages/' + recordId + '/quality');
    },

    /* ---- 任务 5：质控工作台与退回返修闭环 ---- */
    qcWorkbench: function (recordId) {
      return req('GET', '/front-pages/' + recordId + '/qc-workbench');
    },
    qcReturn: function (qcResultId, opinion) {
      return req('POST', '/qc-results/' + qcResultId + '/return', { opinion: opinion || '' });
    },
    // opinion 为可选的整改说明（缺口 D4）；不传时仍提交 {}，与后端既有契约一致。
    qcRectify: function (qcResultId, opinion) {
      return req('POST', '/qc-results/' + qcResultId + '/rectify', { opinion: opinion || '' });
    },
    qcReview: function (qcResultId, result, opinion) {
      return req('POST', '/qc-results/' + qcResultId + '/review', { result: result, opinion: opinion || '' });
    },
    // 缺口 D4：临床端「我的待整改」清单（按登录用户角色限定数据范围：临床=本科室，病案室=全院督办）
    myPendingQc: function () {
      return req('GET', '/qc-results/my-pending');
    },

    /* ---- 缺口 D3（2026-10-09）：首页数据读取 / 修正 / 重新质控 ----
       权限现实（见 router.go：PUT 与 /qc 都在 admin 组）：
         GET  /front-pages/:recordId      任意登录用户（auth）→ 临床可只读查看
         PUT  /front-pages/:recordId      ADMIN only
         POST /front-pages/:recordId/qc   ADMIN only
       调用方必须先判 MRMS.state.currentUser.roleCode === 'ADMIN' 再调写接口，
       否则必然 403。 */
    getFrontPage: function (recordId) {
      return req('GET', '/front-pages/' + recordId);
    },
    // frontPage 为 GET 回来的完整快照，patch 为本次改动；内部按全量白名单拼载荷，
    // 避免「只传本次改的字段」把其它字段写成零值（reason 写入变更留痕）。
    // reason 走查询参数而非 X-Change-Reason 请求头：后端 CORS 只放行
    // Authorization/Content-Type（middleware/cors.go），自定义头在 8090 跨源联调时会
    // 触发预检失败；handler 的 changeReason 同时支持 ?reason=，故用查询参数。
    saveFrontPage: function (recordId, frontPage, patch, reason) {
      var body = frontPagePayload(frontPage, patch);
      // 两个分支都直接写成请求字面量而非先存变量：frontend_calls_test.go 只对
      // 「方法 + 字面量路径」这类写法做静态校验，路径进变量会让「前端调用的路由
      // 是否存在」这条契约测试静默失去覆盖（此处刻意保留可静态校验的写法）。
      if (reason) return req('PUT', '/front-pages/' + recordId + '?reason=' + encodeURIComponent(reason), body);
      return req('PUT', '/front-pages/' + recordId, body);
    },
    // 保存后首页状态一律置 WAIT_QC，故修正后必须紧接着重跑质控拿结论。
    runQc: function (recordId) {
      return req('POST', '/front-pages/' + recordId + '/qc');
    },

    /* ---- 任务 17：归档锁定与变更留痕 ---- */
    revisions: function (recordId) {
      return req('GET', '/front-pages/' + recordId + '/revisions');
    },

    /* ---- 任务 23：归档及时率与超期催缴 ---- */
    archiveTimeliness: function (months) {
      return req('GET', '/archive-timeliness?months=' + (months || 6));
    },
    archiveOverdue: function (days) {
      return req('GET', '/archive-overdue?days=' + (days || 3));
    },

    /* ---- 任务 10：上报一致性核对 ---- */
    reportConsistency: function (period) {
      return req('GET', '/report-consistency' + (period ? '?period=' + period : ''));
    },
    reportConsistencySummary: function () {
      return req('GET', '/report-consistency/summary');
    },

    /* ---- 任务 8：上报导出（hqmsts_ CSV）与导出前检查 ---- */
    reportExportCheck: function (batchId) {
      return req('GET', '/report-batches/' + batchId + '/export-check');
    },
    reportExportURL: function (batchId) {
      // 导出走浏览器直接下载（带 token 的 URL 不能直接给 <a>，故用 fetch + Blob）
      return API + '/api/report-batches/' + batchId + '/export';
    },
    downloadReportExport: function (batchId, filename) {
      var self = this;
      return fetch(self.reportExportURL(batchId), {
        headers: { Authorization: 'Bearer ' + token() }
      }).then(function (res) {
        if (!res.ok) throw new Error('导出失败 HTTP ' + res.status);
        return res.blob();
      }).then(function (blob) {
        var a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = filename || ('hqmsts_export.csv');
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(a.href);
      });
    },

    /* 便捷包装：带 toast 的调用（页面直接用，不必自己 catch） */
    call: function (promise, okMsg) {
      return promise.then(function (d) {
        if (okMsg) actions.toast(okMsg);
        return d;
      }).catch(function (e) {
        actions.toast((e && e.message) || '操作失败', 'err');
        throw e;
      });
    }
  };

  window.MRMS = { state: state, actions: actions, stats: stats, util: util, ext: ext };

  // 启动：有 token 则恢复会话并拉数，否则停在登录页
  tryResume();
})();
