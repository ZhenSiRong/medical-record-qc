/**
 * shot-report.js —— 「首页数据上报」域端到端录屏 / 截图
 *
 * 动线（12 步）：
 *   登录 → 首页数据上报 → 上报 KPI 与批次列表
 *   → 选中 2026-08 批次看「上报前数据校验」→ 导出校验问题清单（CSV）
 *   → 「一致性核对」（应报数 vs 已报数 / 缺失数 / 被改数）
 *   → 「+ 新建批次」选周期 → 创建批次 → 新批次校验结果
 *   → 「导出前检查」弹窗 → 「导出 CSV」（hqmsts_ 国家接口标准）
 *   → 「执行上报」→ 回执回写（平台已接收）→ 字段规范
 *
 * 用法（仓库根目录）：
 *   NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules" \
 *     node tools/shot-report.js
 *   SHOT_VIDEO=1 ... node tools/shot-report.js    # 录屏（1280x800）
 *   SHOT_REPORT_PERIOD=2026-06 ... node tools/shot-report.js  # 强制指定上报周期
 *
 * ── 周期自动挑选（两轮跑法的关键）──────────────────────────────────────
 *   前端 createReport() 对「同一周期已有批次」会短路（直接复用既有批次、不再 POST），
 *   所以第二轮（高清截图）再点「创建批次」不会产生新批次；且批次一旦上报即 SUCCESS，
 *   第二次点「执行上报」只会弹「该批次已接收」。因此脚本**每轮自动挑一个还没被
 *   用过的周期**，保证两轮都能完整演一遍「建批次→上报→回执」。
 *   候选不是写死的：先从库里枚举所有「有已编目病案」的月份，再排除已有批次的周期，
 *   优先取病案数多的月份（导出前检查/校验的问题项更丰富，画面更有信息量）。
 *
 * ── toast 位置（画面瑕疵修正）───────────────────────────────────────────
 *   toast 原本固定在顶部居中（top:16px），会被注入的步骤条 #vb-steps 压住 → 观众
 *   看不到「上报完成：成功 N 条」这类关键提示。改到 top:74px 后又正好盖住 KPI 卡的
 *   「首页上报完成率」标题。最终移到**右下角**：既不挡步骤条，也不挡 KPI 与批次表。
 *
 * 依赖：playwright-core + tools/shot-lib.js（共享库，只读引用，不修改）。
 * 前置：后端 :8080、前端 :8090 已启动（PG 在 5432）。
 */
const { createShooter } = require('./shot-lib');

/* 基准批次：一致性核对与首批校验演示用（库里有真实偏差：应报 10 / 已报 7 / 缺失 3） */
const BASE_PERIOD = process.env.SHOT_REPORT_BASE_PERIOD || '2026-08';
/* 候选新建周期的优先顺序（可选覆盖）；实际会与库里动态枚举到的月份合并 */
const CANDIDATE_PERIODS = (process.env.SHOT_REPORT_PERIODS ||
  '2026-10,2026-06,2026-09,2026-05').split(',').map((s) => s.trim()).filter(Boolean);

const STEP_PLAN = [
  '打开系统并登录',
  '进入「首页数据上报」',
  '查看上报 KPI 与批次列表',
  '选中 2026-08 批次 · 上报前数据校验',
  '导出校验问题清单（CSV）',
  '一致性核对：应报数 vs 已报数',
  '新建上报批次',
  '新批次校验结果与问题明细',
  '导出前检查（必填字段扫描）',
  '导出国家接口标准 CSV',
  '执行上报 → 回执（平台已接收）',
  '病案首页上报字段规范',
];

const log = (name, val) =>
  console.log(name + ' ' + (typeof val === 'object' ? JSON.stringify(val) : val));

(async () => {
  const S = await createShooter({ outDir: 'docs/screenshots/report', stepPlan: STEP_PLAN });
  const page = S.page;

  /* ---- 下载事件：导出 CSV / 导出问题清单都靠它落盘 ---- */
  const downloads = [];
  page.on('download', async (d) => {
    let saved = null;
    try { saved = await d.path(); } catch (e) { /* ignore */ }
    downloads.push({ name: d.suggestedFilename(), saved: !!saved });
    log('DOWNLOAD', d.suggestedFilename() + ' saved=' + !!saved);
  });
  /* ---- 原生 confirm：「执行上报」在有校验问题时弹确认框，不接受则静默中止 ---- */
  page.on('dialog', async (d) => {
    log('DIALOG', d.type() + ' | ' + String(d.message()).replace(/\s+/g, ' ').slice(0, 90));
    await d.accept().catch(() => {});
  });

  /* ================= 页面内取数助手 =================
     全部基于**可见文本 / 元素计数**，不靠肉眼看截图判绿。 */
  const extractKpi = () =>
    page.evaluate(() =>
      Array.from(document.querySelectorAll('.card.kpi')).map((c) => ({
        label: (c.querySelector('.k-label') || {}).textContent?.trim() || '',
        value: (c.querySelector('.k-value') || {}).textContent?.replace(/\s+/g, ' ').trim() || '',
      })));

  const validateSummary = () =>
    page.evaluate(() => {
      const m = document.body.innerText.match(/应报\s*(\d+)\s*份，校验通过\s*(\d+)\s*份，异常\s*(\d+)\s*份/);
      return m ? { total: +m[1], pass: +m[2], issues: +m[3] } : null;
    });

  const countIssueTexts = () =>
    page.evaluate(() => {
      const t = document.body.innerText;
      const keys = ['主诊断 ICD 编码缺失', '手术操作编码缺失', '主治医师缺失', '住院费用异常'];
      let n = 0;
      keys.forEach((k) => { n += t.split(k).length - 1; });
      return n;
    });

  /* 一致性核对的 4 个指标：标签 div 的下一个兄弟就是数值 div，按结构取，避免正则误伤文案 */
  const extractConsistency = () =>
    page.evaluate(() => {
      const card = Array.from(document.querySelectorAll('.card')).find((c) =>
        ((c.querySelector('.card-head h3') || {}).textContent || '').includes('上报一致性核对'));
      if (!card) return null;
      const pick = (label) => {
        const el = Array.from(card.querySelectorAll('.muted.small'))
          .find((d) => d.textContent.trim() === label);
        if (!el || !el.nextElementSibling) return null;
        return el.nextElementSibling.textContent.trim();
      };
      const badges = Array.from(card.querySelectorAll('.badge')).map((b) => b.textContent.trim());
      const t = card.innerText;
      return {
        period: pick('应报数（院内应有）') === null ? null : (t.match(/·\s*(\d{4}-\d{2})/) || [null, null])[1],
        expected: pick('应报数（院内应有）'),
        submitted: pick('已报数（上报明细）'),
        missing: pick('缺失数（漏报）'),
        mismatch: pick('上报后被改数'),
        verdict: /上报口径一致/.test(t) ? '一致' : (/存在偏差/.test(t) ? '存在偏差' : '?'),
        metricCards: Array.from(card.querySelectorAll('.muted.small'))
          .filter((d) => /应报数（院内应有）|已报数（上报明细）|缺失数（漏报）|上报后被改数/.test(d.textContent)).length,
        detailRows: badges.filter((s) => s === '缺失' || s === '被改').length,
      };
    });

  const extractExportCheck = () =>
    page.evaluate(() => {
      const m = document.querySelector('.mask .modal');
      if (!m) return null;
      const t = m.innerText;
      const c = t.match(/共\s*(\d+)\s*条，问题病案\s*(\d+)\s*份/);
      return {
        title: ((m.querySelector('.modal-head h3') || {}).textContent || '').trim(),
        total: c ? +c[1] : null,
        problemItems: c ? +c[2] : null,
        problemRows: Array.from(m.querySelectorAll('.modal-body div'))
          .filter((d) => d.style.color && d.style.color.indexOf('var(--danger)') > -1).length,
        okText: /字段完整，可正常导出/.test(t),
        message: (t.match(/存在\s*\d+\s*项问题[^\n]*/) || ['字段完整，可正常导出'])[0],
      };
    });

  const batchTable = () => page.locator('table.data').first();
  const batchRows = () => batchTable().locator('tbody tr').count();
  const rowOf = (p) => batchTable().locator('tbody tr', { hasText: p });
  const rowBadge = (p) => rowOf(p).locator('.badge').first().textContent().catch(() => '');
  const rowNums = (p) =>
    rowOf(p).locator('td.tnum').allTextContents().catch(() => []);

  /* 元素可能被滚出视口 → 点击前先滚进视口（否则 boundingBox 拿到视口外坐标，点空） */
  const ensureVisible = async (locator) => {
    await locator.scrollIntoViewIfNeeded({ timeout: 8000 }).catch(() => {});
    await page.waitForTimeout(260);
  };

  /* 带重试的接口调用：后端偶发进程重启 / 并发抢占会打来 ECONNREFUSED，
     重试即可跨过一次短暂抖动，不必整轮重录。 */
  const apiJson = async (p, tries = 4) => {
    let lastErr = null;
    for (let i = 0; i < tries; i++) {
      try {
        const r = await S.api(p);
        if (r.status && r.status >= 500) throw new Error('HTTP ' + r.status);
        return r.json;
      } catch (e) {
        lastErr = e;
        log('API_RETRY', p + ' try=' + (i + 1) + ' err=' + String(e.message).split('\n')[0]);
        await page.waitForTimeout(2500 + i * 1500);
      }
    }
    throw lastErr || new Error('接口调用失败: ' + p);
  };

  /* ---- 挑一个还没被用过的上报周期：既无批次、又有已编目病案可归集 ----
     候选 = 环境变量给的优先顺序 ∪ 库里动态枚举出的「有已编目病案」的月份；
     排除已有批次的周期；同一优先级内按病案数多的排前面（画面问题项更丰富）。 */
  const pickPeriod = async () => {
    const bj = await apiJson('/report-batches/page?page=1&size=100');
    const used = new Set(((bj.data && bj.data.list) || []).map((b) => b.period));
    const rj = await apiJson('/records/page?page=1&size=100');
    const recs = (rj.data && rj.data.list) || [];
    const REC_PROJ = { NOT_RECYCLED: '未归档', RECYCLED: '待编目', CATALOGED: '已编目', ARCHIVED: '已归档', BORROWED: '已编目', SEALED: '已编目' };
    const d10 = (s) => String(s || '').slice(0, 10);
    /* 库里枚举：有已编目病案的月份 → 病案数 */
    const cnt = new Map();
    recs.forEach((r) => {
      const p = d10(r.dischargeTime).slice(0, 7);
      if (!/^\d{4}-\d{2}$/.test(p)) return;
      if (REC_PROJ[r.status] !== '已编目') return;
      cnt.set(p, (cnt.get(p) || 0) + 1);
    });
    /* 合并候选：显式列表优先，其次库里枚举到的月份（按病案数降序） */
    const merged = [];
    CANDIDATE_PERIODS.forEach((p) => { if (!merged.includes(p)) merged.push(p); });
    Array.from(cnt.keys()).sort((a, b) => cnt.get(b) - cnt.get(a)).forEach((p) => {
      if (!merged.includes(p)) merged.push(p);
    });
    for (const p of merged) {
      if (used.has(p)) continue;
      if ((cnt.get(p) || 0) > 0) return p;
    }
    return null;
  };

  try {
    /* ================= 1. 登录页 ================= */
    await S.open({ loginShot: '01-report-login' });
    /* toast 原本固定顶部居中：top:16px 被步骤条压住 → 移到右下角，
       既露出提示文案，又不遮 KPI 卡与批次表。 */
    await page.addStyleTag({
      content: '.toasts{top:auto !important;bottom:26px !important;right:26px !important;left:auto !important;transform:none !important;}',
    }).catch(() => {});

    const PERIOD = await pickPeriod();
    log('REPORT_TARGET_PERIOD', PERIOD || 'NONE');
    if (!PERIOD) throw new Error('没有可用于演示的上报周期（候选都已有批次或无已编目病案）');

    /* ================= 2. 进入首页数据上报 ================= */
    await S.step(2);
    await S.click(page.locator('button.nav-item:has-text("首页数据上报")'));
    await page.waitForSelector('text=上报批次管理', { timeout: 20000 });
    await page.waitForTimeout(1400);

    /* ================= 3. KPI 与批次列表 ================= */
    await S.step(3);
    await page.waitForTimeout(700);
    const kpi = await extractKpi();
    log('REPORT_KPI_CARDS', kpi.length);
    kpi.forEach((k) => log('  KPI', k.label + ' = ' + k.value));
    log('REPORT_BATCH_ROWS', await batchRows());
    await S.shot('02-report-list');

    /* ================= 4. 选中基准批次 → 上报前数据校验 ================= */
    await S.step(4);
    const baseRow = rowOf(BASE_PERIOD);
    if (await baseRow.count()) {
      await ensureVisible(baseRow.locator('button.link-btn'));
      await S.click(baseRow.locator('button.link-btn'));
    } else {
      log('BASE_ROW_MISSING', BASE_PERIOD);
    }
    await page.waitForTimeout(1200);
    const vsum = await validateSummary();
    log('REPORT_VALIDATE_SUMMARY',
      vsum ? `total=${vsum.total} pass=${vsum.pass} issues=${vsum.issues}` : 'NONE');
    log('REPORT_VALIDATE_ISSUE_ROWS', vsum ? vsum.issues : 0);
    log('REPORT_VALIDATE_ISSUE_TEXTS', await countIssueTexts());
    await page.locator('h3:has-text("上报前数据校验")').scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(400);
    await S.shot('03-report-validate');

    /* ================= 5. 导出校验问题清单 ================= */
    await S.step(5);
    const dl0 = downloads.length;
    const issueBtn = page.locator('button:has-text("导出问题清单")');
    await ensureVisible(issueBtn);
    await S.click(issueBtn);
    await page.waitForTimeout(1900);
    log('REPORT_ISSUE_CSV_OK', downloads.length - dl0);
    log('REPORT_ISSUE_CSV_FILE', downloads.slice(dl0).map((d) => d.name).join(',') || 'NONE');

    /* ================= 6. 一致性核对 ================= */
    await S.step(6);
    const consBtn = page.locator('button:has-text("一致性核对")');
    await ensureVisible(consBtn);
    await S.click(consBtn);
    await page.waitForSelector('text=上报一致性核对', { timeout: 15000 });
    await page.waitForTimeout(1300);
    const cons = await extractConsistency();
    log('REPORT_CONSISTENCY_CARDS', cons ? cons.metricCards : 0);
    log('REPORT_CONSISTENCY_METRICS', cons);
    log('REPORT_CONSISTENCY_DETAIL_ROWS', cons ? cons.detailRows : 0);
    await page.locator('h3:has-text("上报一致性核对")').scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(500);
    await S.shot('04-report-consistency');

    /* ================= 7. 新建批次 ================= */
    await S.step(7);
    const pre = await apiJson('/report-batches/page?page=1&size=100');
    const preCount = ((pre.data && pre.data.list) || []).length;
    const newBtn = page.locator('button:has-text("新建批次")');
    await ensureVisible(newBtn);
    await S.click(newBtn);
    await page.waitForSelector('.mask select', { timeout: 10000 });
    await page.selectOption('.mask select', PERIOD).catch((e) =>
      log('PERIOD_SELECT_FAILED', e.message.split('\n')[0]));
    await page.waitForTimeout(900);
    await S.shot('05-report-create-modal');
    const createBtn = page.locator('.modal-foot button:has-text("创建批次")');
    await ensureVisible(createBtn);
    await S.click(createBtn);
    /* 等新周期那一行真的出现在批次表里（不是占位行） */
    await page.waitForFunction(
      (p) => Array.from(document.querySelectorAll('table.data tbody tr'))
        .some((tr) => tr.innerText.includes(p)),
      PERIOD, { timeout: 20000 }).catch(() => log('NEW_ROW_TIMEOUT', PERIOD));
    await page.waitForTimeout(2400);
    const post = await apiJson('/report-batches/page?page=1&size=100');
    const postList = (post.data && post.data.list) || [];
    const created = postList.length > preCount && postList.some((b) => b.period === PERIOD);
    log('REPORT_BATCH_CREATED', created ? 1 : 0);
    log('REPORT_BATCH_ROWS', await batchRows());
    /* 等真实批次 id 回填（导出前检查 / 执行上报都要 _bid） */
    await page.waitForFunction(
      (p) => {
        const list = (window.MRMS && window.MRMS.state && window.MRMS.state.model.reports) || [];
        const r = list.find((x) => x.id === p);
        return !!(r && r._bid > 0);
      }, PERIOD, { timeout: 20000 }).catch(() => log('BATCH_ID_NOT_BACKFILLED', PERIOD));
    log('REPORT_NEW_BATCH_STATUS', (await rowBadge(PERIOD).catch(() => '')) || 'NONE');
    log('REPORT_NEW_BATCH_TOTAL', JSON.stringify(await rowNums(PERIOD)));
    await batchTable().scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(400);
    await S.shot('06-report-batch-created');

    /* ================= 8. 新批次校验结果 ================= */
    await S.step(8);
    const newRow = rowOf(PERIOD);
    if (await newRow.count()) {
      await ensureVisible(newRow.locator('button.link-btn'));
      await S.click(newRow.locator('button.link-btn'));
    }
    await page.waitForTimeout(1300);
    const vsum2 = await validateSummary();
    log('REPORT_NEW_VALIDATE_SUMMARY',
      vsum2 ? `total=${vsum2.total} pass=${vsum2.pass} issues=${vsum2.issues}` : 'NONE');
    log('REPORT_NEW_VALIDATE_ISSUE_ROWS', vsum2 ? vsum2.issues : 0);
    await page.locator('h3:has-text("上报前数据校验")').scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(400);
    await S.shot('07-report-new-validate');

    /* ================= 9. 导出前检查 ================= */
    await S.step(9);
    const ecBtn = page.locator('button:has-text("导出前检查")');
    await ensureVisible(ecBtn);
    await S.click(ecBtn);
    await page.waitForSelector('.mask .modal', { timeout: 10000 });
    await page.waitForTimeout(2200);
    const ec = await extractExportCheck();
    log('REPORT_EXPORT_CHECK_MODAL', ec ? 1 : 0);
    log('REPORT_EXPORT_CHECK_TOTAL', ec ? ec.total : 0);
    log('REPORT_EXPORT_CHECK_PROBLEMS', ec ? ec.problemItems : 0);
    log('REPORT_EXPORT_CHECK_PROBLEM_ROWS', ec ? ec.problemRows : 0);
    log('REPORT_EXPORT_CHECK_MESSAGE', ec ? ec.message : 'NONE');
    await S.shot('08-report-export-check');

    /* ================= 10. 导出 CSV（hqmsts_ 国家接口标准） ================= */
    await S.step(10);
    const dl1 = downloads.length;
    const csvBtn = page.locator('.modal-foot button:has-text("导出 CSV")');
    await ensureVisible(csvBtn);
    await S.click(csvBtn);
    await page.waitForTimeout(2900);
    const csvFiles = downloads.slice(dl1).map((d) => d.name);
    log('REPORT_CSV_EXPORT_OK', csvFiles.some((n) => /^hqmsts_.*\.csv$/.test(n)) ? 1 : 0);
    log('REPORT_CSV_EXPORT_FILE', csvFiles.join(',') || 'NONE');
    await S.shot('09-report-csv-export');
    await page.locator('.modal-foot button:has-text("关闭")').click({ timeout: 8000 }).catch(() => {});
    await page.waitForTimeout(1000);

    /* ================= 11. 执行上报 → 回执 ================= */
    await S.step(11);
    const submitBtn = page.locator('button:has-text("执行上报")');
    if (await submitBtn.count()) {
      await ensureVisible(submitBtn);
      await S.click(submitBtn);
      const ok = await page.waitForFunction(
        (p) => {
          const tr = Array.from(document.querySelectorAll('table.data tbody tr'))
            .find((r) => r.innerText.includes(p));
          return !!(tr && tr.innerText.includes('已接收'));
        }, PERIOD, { timeout: 30000 }).then(() => 1).catch(() => 0);
      log('REPORT_SUBMIT_OK', ok);
    } else {
      log('REPORT_SUBMIT_OK', 0);
      log('REPORT_SUBMIT_ALREADY', 1);
    }
    log('REPORT_BATCH_FINAL_STATUS', (await rowBadge(PERIOD).catch(() => '')) || 'NONE');
    const kpi2 = await extractKpi();
    log('REPORT_KPI_AFTER', kpi2.map((k) => k.label + '=' + k.value).join(' | '));
    await batchTable().scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(600);
    await S.shot('10-report-submit');

    /* ================= 12. 字段规范 ================= */
    await S.step(12);
    const specCard = page.locator('.card').filter({ has: page.locator('h3:has-text("病案首页上报字段规范")') });
    await specCard.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(800);
    log('REPORT_FIELD_SPEC_ROWS', await specCard.locator('tbody tr').count());
    log('REPORT_FIELD_SPEC_TITLE', await page.locator('h3:has-text("病案首页上报字段规范")').count());
    await S.shot('11-report-field-spec');

    await page.evaluate(() => window.scrollTo(0, 0)).catch(() => {});
    await page.waitForTimeout(700);
    await S.shot('12-report-top');
  } catch (e) {
    log('FLOW_FAILED', e.message.split('\n')[0]);
    try {
      const t = await page.evaluate(() => document.body.innerText.slice(0, 2000));
      console.log('PAGE_TEXT_DUMP ' + t);
    } catch (e2) { /* ignore */ }
    try { await S.shot('99-report-error'); } catch (e3) { /* ignore */ }
  }

  await S.finish();
})().catch((e) => {
  console.error('FATAL ' + e.message);
  process.exit(1);
});
