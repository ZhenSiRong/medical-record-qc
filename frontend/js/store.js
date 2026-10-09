/* ============================================================
   store.js —— 数据层
   职责：
     1) 基础字典（科室 / ICD-10 疾病编码 / ICD-9-CM-3 手术编码 / 医师）
     2) 病案、借阅、上报批次的模拟数据生成（确定性伪随机，保证多次打开一致）
     3) 基于 Vue.reactive 的响应式状态 + 业务动作（Action）
     4) localStorage 持久化
   ============================================================ */
(function () {
  'use strict';

  /* ------------------------- 伪随机（种子固定，数据可复现） ------------------------- */
  function mulberry32(a) {
    return function () {
      a |= 0; a = (a + 0x6D2B79F5) | 0;
      var t = Math.imul(a ^ (a >>> 15), 1 | a);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }
  var rnd = mulberry32(20260918);
  function ri(min, max) { return Math.floor(rnd() * (max - min + 1)) + min; }
  function pick(arr) { return arr[Math.floor(rnd() * arr.length)]; }
  function chance(p) { return rnd() < p; }

  /* ---------------------------------- 日期工具 ---------------------------------- */
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
  function diffDays(a, b) {
    return Math.round((new Date(b + 'T00:00:00') - new Date(a + 'T00:00:00')) / 86400000);
  }

  /* ---------------------------------- 基础字典 ---------------------------------- */
  var DEPARTMENTS = [
    '心血管内科', '神经内科', '呼吸内科', '消化内科', '普通外科',
    '骨科', '妇产科', '儿科', '肿瘤科', '泌尿外科'
  ];

  // ICD-10 疾病分类与编码字典
  var ICD10 = [
    { code: 'I10.x00',    name: '原发性高血压',              cat: '循环系统疾病' },
    { code: 'I25.101',    name: '冠状动脉粥样硬化性心脏病',  cat: '循环系统疾病' },
    { code: 'I50.901',    name: '心力衰竭',                  cat: '循环系统疾病' },
    { code: 'I63.901',    name: '脑梗死',                    cat: '神经系统疾病' },
    { code: 'I61.901',    name: '脑出血',                    cat: '神经系统疾病' },
    { code: 'G40.901',    name: '癫痫',                      cat: '神经系统疾病' },
    { code: 'J18.901',    name: '肺炎',                      cat: '呼吸系统疾病' },
    { code: 'J44.101',    name: '慢性阻塞性肺疾病伴急性加重', cat: '呼吸系统疾病' },
    { code: 'J06.900',    name: '急性上呼吸道感染',          cat: '呼吸系统疾病' },
    { code: 'K35.801',    name: '急性阑尾炎',                cat: '消化系统疾病' },
    { code: 'K80.201',    name: '胆囊结石',                  cat: '消化系统疾病' },
    { code: 'K25.901',    name: '胃溃疡',                    cat: '消化系统疾病' },
    { code: 'S72.001',    name: '股骨颈骨折',                cat: '损伤与骨折' },
    { code: 'S06.001',    name: '脑震荡',                    cat: '损伤与骨折' },
    { code: 'S82.601',    name: '踝关节骨折',                cat: '损伤与骨折' },
    { code: 'C34.901',    name: '支气管肺癌',                cat: '肿瘤' },
    { code: 'C50.901',    name: '乳腺恶性肿瘤',              cat: '肿瘤' },
    { code: 'E11.901',    name: '2型糖尿病',                 cat: '内分泌代谢' },
    { code: 'E05.901',    name: '甲状腺功能亢进症',          cat: '内分泌代谢' },
    { code: 'N20.001',    name: '肾结石',                    cat: '泌尿生殖' },
    { code: 'N40.001',    name: '前列腺增生',                cat: '泌尿生殖' },
    { code: 'O80.001',    name: '顺产',                      cat: '妊娠分娩' },
    { code: 'O82.001',    name: '剖宫产',                    cat: '妊娠分娩' },
    { code: 'A41.901',    name: '脓毒症',                    cat: '感染性疾病' }
  ];

  // ICD-9-CM-3 手术操作编码字典
  var ICD9 = [
    { code: '00.0000', name: '无手术操作' },
    { code: '47.0900', name: '阑尾切除术' },
    { code: '51.2300', name: '腹腔镜胆囊切除术' },
    { code: '36.0600', name: '冠状动脉药物洗脱支架植入术' },
    { code: '81.5100', name: '全髋关节置换术' },
    { code: '74.1000', name: '剖宫产术' },
    { code: '55.5101', name: '肾盂切开取石术' },
    { code: '32.4901', name: '肺叶切除术' },
    { code: '45.1300', name: '结肠镜检查' },
    { code: '39.9500', name: '血液透析' },
    { code: '60.2901', name: '经尿道前列腺电切术' }
  ];

  var DOCTORS = ['张建国', '李慧敏', '王振华', '赵雅琴', '陈立新', '刘文博',
                 '周晓东', '孙丽萍', '吴海燕', '郑秋月', '徐明远', '何静怡'];

  var SURNAMES = ['赵', '钱', '孙', '李', '周', '吴', '郑', '王', '冯', '陈',
                  '褚', '卫', '蒋', '沈', '韩', '杨', '朱', '秦', '许', '何',
                  '吕', '施', '张', '孔', '曹', '严', '华', '金', '魏', '陶'];
  var GIVEN1 = ['国', '建', '文', '志', '明', '海', '秀', '淑', '玉', '桂', '德', '春'];
  var GIVEN2 = ['华', '强', '军', '民', '平', '英', '芳', '梅', '兰', '霞', '荣', '杰', '磊', '静'];

  function genName() {
    return pick(SURNAMES) + pick(GIVEN1) + (chance(0.72) ? pick(GIVEN2) : '');
  }

  var ARCHIVE_STATUS = ['未归档', '已归档', '已编目'];

  /* ---------------------------------- 病案数据 ---------------------------------- */
  function genRecords() {
    var list = [];
    var now = new Date();
    var seq = 1;

    // 生成最近 6 个月的出院病案
    for (var offset = 5; offset >= 0; offset--) {
      var monthStart = new Date(now.getFullYear(), now.getMonth() - offset, 1);
      var monthEnd = new Date(now.getFullYear(), now.getMonth() - offset + 1, 0);
      var y = monthStart.getFullYear();
      var m = monthStart.getMonth() + 1;

      var count = ri(16, 23);
      if (offset === 0) count = Math.max(6, Math.round(count * (now.getDate() / 30)));

      for (var i = 0; i < count; i++) {
        var maxDay = offset === 0 ? now.getDate() : monthEnd.getDate();
        var day = ri(1, Math.max(2, maxDay));
        var discharge = new Date(y, m - 1, day);
        var stay = ri(3, 21);
        var admit = addDays(discharge, -stay);

        var icd = pick(ICD10);
        var op = pick(ICD9);
        var dept = pick(DEPARTMENTS);
        // 科室与诊断做一点匹配，数据更合理
        if (icd.cat === '妊娠分娩') dept = '妇产科';
        else if (icd.cat === '损伤与骨折' && chance(0.75)) dept = '骨科';
        else if (icd.cat === '肿瘤' && chance(0.75)) dept = '肿瘤科';

        var age = ri(2, 88);
        if (icd.cat === '妊娠分娩') age = ri(21, 39);
        if (dept === '儿科') age = ri(1, 12);

        // 归档状态：离当前越近，未归档比例越高
        var r = rnd();
        var status;
        if (offset === 0)      status = r < 0.34 ? '未归档' : (r < 0.72 ? '已归档' : '已编目');
        else if (offset === 1) status = r < 0.12 ? '未归档' : (r < 0.30 ? '已归档' : '已编目');
        else                   status = r < 0.04 ? '未归档' : (r < 0.13 ? '已归档' : '已编目');

        var archiveDate = status === '未归档' ? '' : fmt(addDays(discharge, ri(3, 9)));
        var reportStatus = status === '已编目'
          ? (offset <= 1 ? (chance(0.35) ? '待上报' : '已上报') : '已上报')
          : '未上报';

        // 故意保留少量数据缺陷（费用未结算 / 手术编码未填），用于演示上报前的字段校验
        var defect = 0;
        if (status === '已编目') {
          var dd = rnd();
          if (dd < 0.04) defect = 1;        // 住院费用异常
          else if (dd < 0.10) defect = 2;   // 手术操作编码缺失
        }

        list.push({
          id: 'MR' + y + String(seq).padStart(5, '0'),
          mrNo: 'MR' + y + String(seq).padStart(5, '0'),
          inpatientNo: 'ZY' + y + String(ri(1, 9)) + String(seq).padStart(4, '0'),
          name: genName(),
          gender: icd.cat === '妊娠分娩' ? '女' : (chance(0.48) ? '女' : '男'),
          age: age,
          dept: dept,
          admitDate: fmt(admit),
          dischargeDate: fmt(discharge),
          stayDays: stay,
          diagMain: icd.name,
          icdCode: icd.code,
          icdCat: icd.cat,
          opCode: defect === 2 ? '' : op.code,
          opName: defect === 2 ? '' : op.name,
          doctor: pick(DOCTORS),
          fee: defect === 1 ? 0 : +(ri(3200, 128000) + rnd()).toFixed(2),
          pageCount: ri(6, 32),
          quality: chance(0.86) ? '甲级' : (chance(0.78) ? '乙级' : '丙级'),
          status: status,
          archiveDate: archiveDate,
          reportStatus: reportStatus,
          coder: status === '已编目' ? pick(DOCTORS) : '',
          catalogDate: status === '已编目' ? fmt(addDays(discharge, ri(10, 20))) : '',
          remark: ''
        });
        seq++;
      }
    }
    return list;
  }

  /* ---------------------------------- 借阅数据 ---------------------------------- */
  var BORROW_REASONS = ['临床病例讨论', '医疗质量检查', '科研数据采集', '医患纠纷取证',
                        '教学查房示教', '医保稽核', '死亡病例讨论'];

  function genBorrows(records) {
    var list = [];
    var now = new Date();
    var statuses = ['已归还', '已归还', '已归还', '已借出', '已借出', '待审批', '已逾期', '已驳回'];

    for (var i = 0; i < 14; i++) {
      var rec = pick(records.filter(function (r) { return r.status !== '未归档'; })) || records[0];
      var bDate = addDays(now, -ri(1, 70));
      var due = addDays(bDate, 14);
      var st = statuses[i % statuses.length];
      if (st === '已驳回') st = chance(0.5) ? '已驳回' : '待审批';

      var returnDate = st === '已归还' ? fmt(addDays(bDate, ri(2, 16))) : '';
      if (st === '已借出' && diffDays(fmt(due), today()) < 0) st = '已逾期';

      list.push({
        id: 'JY' + String(202600 + i + 1),
        mrNo: rec.mrNo,
        patientName: rec.name,
        dept: rec.dept,
        borrower: pick(DOCTORS),
        borrowerDept: pick(DEPARTMENTS),
        reason: pick(BORROW_REASONS),
        borrowDate: fmt(bDate),
        dueDate: fmt(due),
        returnDate: returnDate,
        status: st,
        approver: st === '待审批' ? '' : '病案室-王主任',
        remark: st === '已驳回' ? '借阅事由不够具体，请补充说明后重新提交' : ''
      });
    }
    return list;
  }

  /* ---------------------------------- 上报批次 ---------------------------------- */
  function genReports(records) {
    var list = [];
    var now = new Date();
    for (var offset = 4; offset >= 0; offset--) {
      var d = new Date(now.getFullYear(), now.getMonth() - offset, 1);
      var period = d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0');
      var total = records.filter(function (r) { return r.dischargeDate.slice(0, 7) === period; }).length;
      var st = offset === 0 ? '待上报' : (offset === 1 ? '已退回' : '已接收');
      list.push({
        id: 'SB' + period.replace('-', ''),
        batchNo: 'SB' + period.replace('-', '') + '01',
        period: period,
        scope: '全院',
        total: total,
        submitted: st === '待上报' ? 0 : total,
        status: st,
        submitTime: st === '待上报' ? '' : period + '-05 09:' + String(ri(10, 55)),
        receiver: st === '已接收' ? '省级卫生健康信息平台' : '',
        note: st === '已退回' ? '第 3、7 条病案手术编码缺失，请补充后重新上报' : (st === '待上报' ? '待月末关账后执行上报' : '')
      });
    }
    return list.reverse();
  }

  /* ---------------------------------- 操作日志 ---------------------------------- */
  function genLogs() {
    var now = new Date();
    return [
      { time: fmt(now) + ' 09:12', user: '张建国', act: '病案归档', target: 'MR202600187', type: 'blue' },
      { time: fmt(now) + ' 10:35', user: '王主任', act: '借阅审批通过', target: 'JY2026007', type: 'green' },
      { time: fmt(now) + ' 11:20', user: '李慧敏', act: '病案编目', target: 'MR202600203', type: 'blue' },
      { time: fmt(now) + ' 14:02', user: '系统', act: '首页数据上报', target: 'SB202608', type: 'orange' }
    ];
  }

  /* ---------------------------------- 状态与持久化 ---------------------------------- */
  var STORAGE_KEY = 'mrms_state_v1';

  function buildInitial() {
    var records = genRecords();
    return {
      records: records,
      borrows: genBorrows(records),
      reports: genReports(records),
      logs: genLogs(),
      catalogDict: ICD10.map(function (x) { return x; })
    };
  }

  function loadState() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      if (raw) {
        var saved = JSON.parse(raw);
        if (saved && Array.isArray(saved.records) && saved.records.length) return saved;
      }
    } catch (e) { /* localStorage 不可用时静默降级为内存数据 */ }
    return buildInitial();
  }

  var state = Vue.reactive({
    currentUser: { name: '王主任', role: '病案室主管', dept: '病案统计科' },
    route: 'dashboard',
    toasts: [],
    sidebarOpen: false,
    model: loadState()
  });

  function persist() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state.model));
    } catch (e) { /* 忽略配额或隐私模式限制 */ }
  }

  /* ---------------------------------- 业务动作 ---------------------------------- */
  var seqCounter = { borrow: 20 };

  function pushLog(act, target, type) {
    state.model.logs.unshift({
      time: today() + ' ' + new Date().toTimeString().slice(0, 5),
      user: state.currentUser.name,
      act: act,
      target: target,
      type: type || 'blue'
    });
    if (state.model.logs.length > 30) state.model.logs.pop();
  }

  var actions = {
    /* --- 全局提示 --- */
    toast: function (msg, type) {
      var id = Date.now() + Math.random();
      state.toasts.push({ id: id, msg: msg, type: type || 'ok' });
      setTimeout(function () {
        var i = state.toasts.findIndex(function (t) { return t.id === id; });
        if (i > -1) state.toasts.splice(i, 1);
      }, 2400);
    },

    /* --- 病案归档 --- */
    addRecord: function (data) {
      var y = new Date().getFullYear();
      var maxSeq = state.model.records.reduce(function (acc, r) {
        var n = parseInt(r.mrNo.slice(-5), 10);
        return isNaN(n) ? acc : Math.max(acc, n);
      }, 0);
      var no = 'MR' + y + String(maxSeq + 1).padStart(5, '0');
      var rec = Object.assign({
        id: no, mrNo: no,
        inpatientNo: 'ZY' + y + String(maxSeq + 1).padStart(4, '0'),
        stayDays: diffDays(data.admitDate, data.dischargeDate),
        pageCount: 8, quality: '甲级',
        status: '未归档', archiveDate: '', reportStatus: '未上报',
        coder: '', catalogDate: '', remark: ''
      }, data);
      state.model.records.unshift(rec);
      pushLog('新增病案', no);
      persist();
      return rec;
    },

    updateRecord: function (id, patch) {
      var rec = state.model.records.find(function (r) { return r.id === id; });
      if (!rec) return;
      Object.assign(rec, patch);
      pushLog('修改病案', id);
      persist();
    },

    removeRecord: function (id) {
      var i = state.model.records.findIndex(function (r) { return r.id === id; });
      if (i > -1) {
        state.model.records.splice(i, 1);
        pushLog('删除病案', id, 'red');
        persist();
      }
    },

    archive: function (ids) {
      var list = Array.isArray(ids) ? ids : [ids];
      list.forEach(function (id) {
        var r = state.model.records.find(function (x) { return x.id === id; });
        if (r && r.status === '未归档') {
          r.status = '已归档';
          r.archiveDate = today();
        }
      });
      pushLog('病案归档', list.length + ' 条');
      persist();
    },

    archiveBatch: function (list) {
      list.forEach(function (id) {
        var r = state.model.records.find(function (x) { return x.id === id; });
        if (r && r.status === '未归档') { r.status = '已归档'; r.archiveDate = today(); }
      });
      pushLog('批量归档', list.length + ' 条');
      persist();
    },

    /* --- 病案编目 --- */
    catalog: function (id, patch) {
      var r = state.model.records.find(function (x) { return x.id === id; });
      if (!r) return;
      Object.assign(r, patch);
      r.status = '已编目';
      if (!r.catalogDate) r.catalogDate = today();
      r.coder = state.currentUser.name;
      r.reportStatus = '待上报';
      pushLog('病案编目', id);
      persist();
    },

    /* --- 借阅 --- */
    addBorrow: function (data) {
      seqCounter.borrow++;
      var id = 'JY' + String(202600 + seqCounter.borrow);
      var due = fmt(addDays(new Date(), 14));
      var rec = Object.assign({
        id: id, dueDate: due, returnDate: '',
        status: '待审批', approver: '', remark: ''
      }, data);
      state.model.borrows.unshift(rec);
      pushLog('提交借阅申请', id, 'orange');
      persist();
      return rec;
    },

    approveBorrow: function (id) {
      var b = state.model.borrows.find(function (x) { return x.id === id; });
      if (b) {
        b.status = '已借出';
        b.approver = state.currentUser.name;
        b.borrowDate = today();
        b.dueDate = fmt(addDays(new Date(), 14));
        pushLog('借阅审批通过', id, 'green');
        persist();
      }
    },

    rejectBorrow: function (id, remark) {
      var b = state.model.borrows.find(function (x) { return x.id === id; });
      if (b) {
        b.status = '已驳回';
        b.approver = state.currentUser.name;
        b.remark = remark || '审批未通过';
        pushLog('借阅审批驳回', id, 'red');
        persist();
      }
    },

    returnBorrow: function (id) {
      var b = state.model.borrows.find(function (x) { return x.id === id; });
      if (b) {
        b.status = '已归还';
        b.returnDate = today();
        pushLog('病案归还', id, 'green');
        persist();
      }
    },

    renewBorrow: function (id, days) {
      var b = state.model.borrows.find(function (x) { return x.id === id; });
      if (b) {
        b.dueDate = fmt(addDays(new Date(b.dueDate + 'T00:00:00'), days || 7));
        b.status = '已借出';
        pushLog('借阅续期', id, 'orange');
        persist();
      }
    },

    /* --- 上报 --- */
    submitReport: function (id) {
      var rep = state.model.reports.find(function (x) { return x.id === id; });
      if (!rep) return;
      rep.status = '已提交';
      rep.submitted = rep.total;
      rep.submitTime = today() + ' ' + new Date().toTimeString().slice(0, 5);
      rep.receiver = '省级卫生健康信息平台';
      rep.note = '数据包已生成并提交，等待平台回执';
      state.model.records.forEach(function (r) {
        if (r.dischargeDate.slice(0, 7) === rep.period && r.status === '已编目') {
          r.reportStatus = '已上报';
        }
      });
      pushLog('执行首页数据上报', rep.batchNo, 'orange');
      persist();
    },

    createReport: function (period) {
      var exist = state.model.reports.find(function (x) { return x.period === period; });
      if (exist) return exist;
      var total = state.model.records.filter(function (r) {
        return r.dischargeDate.slice(0, 7) === period && r.status === '已编目';
      }).length;
      var id = 'SB' + period.replace('-', '');
      var rep = {
        id: id, batchNo: id + '01', period: period, scope: '全院',
        total: total, submitted: 0, status: '待上报',
        submitTime: '', receiver: '', note: '新建批次，待校验后上报'
      };
      state.model.reports.unshift(rep);
      pushLog('新建上报批次', rep.batchNo);
      persist();
      return rep;
    },

    /* --- 数据重置 --- */
    reset: function () {
      state.model = buildInitial();
      persist();
      pushLog('重置演示数据', '全部');
    }
  };

  /* ---------------------------------- 派生统计（纯函数，供各模块复用） ---------------------------------- */
  var stats = {
    // 按条件筛选
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

    // 概览指标
    overview: function () {
      var rs = state.model.records;
      var total = rs.length;
      var archived = rs.filter(function (r) { return r.status !== '未归档'; }).length;
      var cataloged = rs.filter(function (r) { return r.status === '已编目'; }).length;
      var thisMonth = today().slice(0, 7);
      var monthCount = rs.filter(function (r) { return r.dischargeDate.slice(0, 7) === thisMonth; }).length;
      var pendingReport = rs.filter(function (r) { return r.reportStatus === '待上报'; }).length;
      var pendingBorrow = state.model.borrows.filter(function (b) { return b.status === '待审批'; }).length;
      var overdue = state.model.borrows.filter(function (b) { return b.status === '已逾期'; }).length;
      var feeTotal = rs.reduce(function (a, r) { return a + (r.fee || 0); }, 0);
      return {
        total: total,
        archived: archived,
        cataloged: cataloged,
        pendingArchive: total - archived,
        archiveRate: total ? +(archived / total * 100).toFixed(1) : 0,
        catalogRate: total ? +(cataloged / total * 100).toFixed(1) : 0,
        monthCount: monthCount,
        pendingReport: pendingReport,
        pendingBorrow: pendingBorrow,
        overdue: overdue,
        avgFee: total ? Math.round(feeTotal / total) : 0,
        avgStay: total ? +(rs.reduce(function (a, r) { return a + r.stayDays; }, 0) / total).toFixed(1) : 0
      };
    },

    // 月度趋势
    monthly: function (field, months) {
      months = months || 6;
      var arr = [];
      var now = new Date();
      for (var i = months - 1; i >= 0; i--) {
        var d = new Date(now.getFullYear(), now.getMonth() - i, 1);
        var key = d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0');
        arr.push({
          key: key,
          label: (d.getMonth() + 1) + '月',
          value: state.model.records.filter(function (r) {
            return r.dischargeDate.slice(0, 7) === key;
          }).length
        });
      }
      return arr;
    },

    // 科室分布
    byDept: function () {
      var map = {};
      state.model.records.forEach(function (r) { map[r.dept] = (map[r.dept] || 0) + 1; });
      return Object.keys(map).map(function (k) { return { name: k, value: map[k] }; })
        .sort(function (a, b) { return b.value - a.value; });
    },

    // 疾病分类分布
    byCategory: function () {
      var map = {};
      state.model.records.forEach(function (r) { map[r.icdCat] = (map[r.icdCat] || 0) + 1; });
      return Object.keys(map).map(function (k) { return { name: k, value: map[k] }; })
        .sort(function (a, b) { return b.value - a.value; });
    },

    // 状态分布
    byStatus: function () {
      var order = ['已编目', '已归档', '未归档'];
      var colors = { '已编目': '#1d6ef5', '已归档': '#12a15c', '未归档': '#d98b13' };
      return order.map(function (s) {
        return {
          name: s,
          color: colors[s],
          value: state.model.records.filter(function (r) { return r.status === s; }).length
        };
      });
    },

    // 上报进度
    reportProgress: function () {
      var rs = state.model.records.filter(function (r) { return r.status === '已编目'; });
      var done = rs.filter(function (r) { return r.reportStatus === '已上报'; }).length;
      return { total: rs.length, done: done, rate: rs.length ? +(done / rs.length * 100).toFixed(1) : 0 };
    },

    // 数据质量校验（用于上报前的完整性检查）
    validate: function (period) {
      var list = state.model.records.filter(function (r) {
        return r.dischargeDate.slice(0, 7) === period && r.status === '已编目';
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
        period: period,
        total: list.length,
        pass: list.length - issues.length,
        issues: issues,
        rate: list.length ? +((list.length - issues.length) / list.length * 100).toFixed(1) : 100
      };
    },

    doctors: function () { return DOCTORS.slice(); },
    departments: function () { return DEPARTMENTS.slice(); },
    icd10: function () { return ICD10; },
    icd9: function () { return ICD9; }
  };

  /* ---------------------------------- 导出 ---------------------------------- */
  window.MRMS = {
    state: state,
    actions: actions,
    stats: stats,
    util: {
      today: today,
      fmt: fmt,
      addDays: addDays,
      diffDays: diffDays,
      departments: DEPARTMENTS,
      doctors: DOCTORS,
      icd10Dict: ICD10,
      icd9Dict: ICD9,
      persist: persist,
      // 通用导出：将数组导出为 CSV（Excel 可直接打开）
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
    }
  };
})();
