/**
 * tools/shot-stats-quality.js —— 「数据统计」+「质量看板」端到端演示录屏 / 截图
 *
 * 动线 A（数据统计）：登录 → 数据统计 → 顶部 KPI → 月度趋势（近 6 月 / 近 3 月）
 *                     → 疾病分类构成 → 病案流转状态 → 病案质量等级 → 住院天数区间
 *                     → 住院费用区间 → 高频主要诊断 Top 5 → 科室统计明细 → 导出报表
 * 动线 B（质量看板）：登录 → 质量看板 → 四张时效指标卡 → 月度 2 日归档率趋势
 *                     （近 6 / 3 / 12 月）→ 科室归档及时率排名 → 超期未归档催缴清单
 *                     （3 天 / 30 天阈值）→ 导出清单 CSV
 *
 * 用法（仓库根目录）：
 *   NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules" \
 *     node tools/shot-stats-quality.js
 *
 *   默认一轮跑完「录屏 + 高清截图」两遍：
 *     第 1 遍 video:true  视口 1280x800 → 产出 <域>/demo.webm（同名 PNG 为 1280 宽）
 *     第 2 遍 video:false 视口 1680x1050 → 同名 PNG 被高清版覆盖
 *   只跑其中一遍：SHOT_ROUND=video / SHOT_ROUND=stills
 *   只跑某一个域：SHOT_DOMAIN=stats / quality（默认两个都跑）
 *
 * 前置：后端 :8080、前端 :8090 已启动（PG 在 5432）。
 *
 * 断言原则：一律用**页面可见文本 / 元素计数**判定，不靠肉眼看图。
 */
'use strict';

const { createShooter } = require('./shot-lib');

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const OUT_STATS = 'docs/screenshots/stats';
const OUT_QUALITY = 'docs/screenshots/quality';

const STATS_PLAN = [
  '打开系统并登录',
  '进入「数据统计」',
  '查看顶部核心指标（病案总量 / 归档完成率 / 平均住院日 / 例均住院费用）',
  '查看「月度病案量趋势」（近 6 月）',
  '切换「近 3 月」对比趋势',
  '查看「疾病分类（ICD-10 大类）构成」与「病案流转状态」',
  '查看「病案质量等级」与「住院天数区间分布」',
  '查看「住院费用区间分布」与「高频主要诊断 Top 5」',
  '查看「科室病案统计明细」',
  '导出统计报表 CSV',
];

const QUALITY_PLAN = [
  '打开系统并登录',
  '进入「质量看板」',
  '查看归档时效指标卡（出院总数 / 2 日 / 3 日归档率 / 平均归档天数）',
  '查看「月度 2 日归档率趋势」（近 6 月）',
  '切换「近 3 月」与「近 12 月」',
  '查看「科室归档及时率排名」',
  '查看「超期未归档催缴清单」（3 天阈值）',
  '切换「30 天」阈值看超期口径变化',
  '导出超期催缴清单 CSV',
];

/* ------------------------------------------------------------------ */
/* 页面小工具（全部基于可见文本 / 元素计数）                             */
/* ------------------------------------------------------------------ */
function helpers(S) {
  const page = S.page;

  /**
   * 把目标元素滚到视口中央（滚动容器是 body，.header 是 sticky 会盖住顶部）。
   * 这里**不能**静默吞错：locator 匹配不到时 evaluate 会抛异常，
   * 若照旧 .catch(()=>{}) 吞掉，页面会停在原位，截出来的图「看起来正常」
   * 却根本不是那一屏 —— 2026-11-06 首次录统计域时就这样白跑了一轮。
   */
  const bring = async (loc, opt = {}) => {
    const n = await loc.count().catch(() => 0);
    if (!n) {
      console.log('BRING_TARGET_MISSING');
      return false;
    }
    try {
      await loc.evaluate((el, pad) => {
        const r = el.getBoundingClientRect();
        const top = window.scrollY + r.top;
        const centered = top - (window.innerHeight - r.height) / 2;
        /* pad 是序列化传进浏览器的参数；不能引用 Node 侧的 opt —— 那会在
           页面里抛 ReferenceError，且只在「元素不够高」那条分支上才被读到，
           于是表现为「时灵时不灵」。 */
        const y = r.height > window.innerHeight - 140
          ? Math.max(0, top - 110)
          : Math.max(0, centered - (pad || 0));
        window.__vbTargetY = y;
        window.scrollTo({ top: y, behavior: 'smooth' });
      }, opt.shift || 0);
    } catch (e) {
      console.log('BRING_FAILED ' + e.message.split('\n')[0]);
      return false;
    }
    await sleep(opt.wait || 900);
    /* 平滑滚动有时会被别处的滚动打断，落地不到位就补一次瞬时滚动 */
    const landed = await page.evaluate(() => {
      const target = window.__vbTargetY || 0;
      if (Math.abs(window.scrollY - target) > 4) {
        window.scrollTo({ top: target, behavior: 'auto' });
      }
      return Math.round(window.scrollY);
    }).catch(() => -1);
    await sleep(250);
    const y = await page.evaluate(() => Math.round(window.scrollY)).catch(() => -1);
    console.log('SCROLL_Y ' + y + ' target=' + landed);
    return true;
  };

  /**
   * 滚到元素 → 把虚拟光标移到它身上 → 停留（让观众看清数字）→ 截图。
   *
   * 为什么必须移光标：同一行网格里的两张卡片（如「病案质量等级」与
   * 「住院天数区间分布」）滚动后落在同一屏，不移光标的话两张截图会
   * **字节级完全相同**（md5 一致），等于白录一张；移过去之后既区分了帧，
   * 观众也能看出「现在讲的是这张卡」。
   */
  const shotCard = async (loc, name, hold = 1600) => {
    const ok = await bring(loc);
    if (!ok) {
      console.log('SHOT_CARD_MISSING ' + name);
      await S.shot(name);
      return false;
    }
    const box = await loc.boundingBox().catch(() => null);
    if (box) {
      await S.glide(box.x + box.width / 2, box.y + Math.min(box.height / 2, 40));
      await sleep(260);
    }
    const vis = await loc.evaluate((el) => {
      const r = el.getBoundingClientRect();
      return {
        top: Math.round(r.top), bottom: Math.round(r.bottom),
        vh: window.innerHeight,
        inView: r.bottom > 90 && r.top < window.innerHeight - 20
      };
    }).catch(() => ({ inView: false }));
    console.log('SHOT_CARD ' + name + ' inView=' + (vis.inView ? 1 : 0) +
      ' top=' + vis.top + ' bottom=' + vis.bottom + ' vh=' + vis.vh);
    await sleep(hold);
    await S.shot(name);
    return !!vis.inView;
  };

  /** 轮询等待元素数量达到期望值（后端取数是异步的） */
  const waitCount = async (loc, expected, label, timeout = 20000) => {
    const t0 = Date.now();
    let n = -1;
    while (Date.now() - t0 < timeout) {
      n = await loc.count().catch(() => 0);
      if (n === expected) return n;
      await sleep(300);
    }
    console.log('WAIT_TIMEOUT ' + label + ' expect=' + expected + ' actual=' + n);
    return n;
  };

  /**
   * 等待元素数量「稳定下来」而不是等到某个具体值。
   *
   * 为什么要这样：演示库是**活的** —— 另一位同事的编目动线会把病案从「待编目」
   * 推成「已编目」并自动上架（archived_at 被置上），超期清单的行数随之减少。
   * 若把断言写死成 52 行，他那边一跑质控就必然 WAIT_TIMEOUT，画面却是对的，
   * 白白把一次录制判成失败。故这里只要求「有数据且不再变化」。
   */
  const waitStable = async (loc, min, label, timeout = 20000) => {
    const t0 = Date.now();
    let last = -1, stable = 0, n = 0;
    while (Date.now() - t0 < timeout) {
      n = await loc.count().catch(() => 0);
      if (n === last && n >= min) {
        stable++;
        if (stable >= 2) return n;
      } else {
        stable = 0;
      }
      last = n;
      await sleep(400);
    }
    console.log('WAIT_TIMEOUT ' + label + ' min=' + min + ' actual=' + n);
    return n;
  };

  /** 等待某段文本不再显示为占位符「—」 */
  const waitFilled = async (sel, label, timeout = 20000) => {
    const t0 = Date.now();
    while (Date.now() - t0 < timeout) {
      const ok = await page.evaluate((s) => {
        const els = document.querySelectorAll(s);
        if (!els.length) return false;
        for (const e of els) {
          const t = (e.textContent || '').trim();
          if (!t || t === '—' || t.indexOf('加载中') > -1) return false;
        }
        return true;
      }, sel).catch(() => false);
      if (ok) return true;
      await sleep(300);
    }
    console.log('WAIT_TIMEOUT ' + label);
    return false;
  };

  return { bring, shotCard, waitCount, waitStable, waitFilled, page };
}

/* ------------------------------------------------------------------ */
/* 动线 A：数据统计                                                     */
/* ------------------------------------------------------------------ */
async function runStats(video) {
  const S = await createShooter({
    outDir: OUT_STATS,
    stepPlan: STATS_PLAN,
    video,
    recordName: 'demo',
  });
  const H = helpers(S);
  const page = S.page;

  /* 登录页一帧 */
  await S.open({ loginShot: '01-stats-login' });

  /* 进入「数据统计」 */
  await S.step(2);
  await S.click(page.locator('button.nav-item:has-text("数据统计")').first());
  await H.waitFilled('.grid.g-4 > div', 'STATS_KPI');
  await sleep(1400);
  await S.shot('02-stats-kpi');

  /* ---- 顶部 KPI 断言：4 个指标块 + 取值不为空 ---- */
  const kpiBlocks = page.locator('.grid.g-4 > div');
  const kpiN = await kpiBlocks.count();
  const kpiText = (await page.locator('.grid.g-4').first().innerText().catch(() => ''))
    .replace(/\s+/g, ' ').trim();
  console.log('STATS_KPI_CARDS ' + kpiN);
  console.log('STATS_KPI_TEXT ' + kpiText);
  const uiTotal = (kpiText.match(/(\d+)\s*份/) || [])[1] || '?';
  const uiArchiveRate = (kpiText.match(/归档完成率\s*([\d.]+)%/) || [])[1] || '?';

  /* ---- 卡片与图表容器计数 ---- */
  const cardN = await page.locator('.card').count();
  const holderN = await page.locator('.chart-holder').count();
  console.log('STATS_CARD_COUNT ' + cardN);
  console.log('STATS_CHART_HOLDERS ' + holderN);

  /* 图表是否真的画出了图形（bar=rect，donut=circle，line=path） */
  const chartShapes = await page.evaluate(() => {
    const out = [];
    document.querySelectorAll('.chart-holder').forEach((h, i) => {
      const svg = h.querySelector('svg');
      const shapes = svg ? svg.querySelectorAll('rect,circle,path,polyline').length : 0;
      const title = (h.closest('.card') || {}).querySelector ? h.closest('.card').querySelector('h3') : null;
      out.push({ i: i, shapes: shapes, title: title ? title.textContent.trim() : '?' });
    });
    return out;
  });
  chartShapes.forEach((c) => console.log('STATS_CHART ' + c.i + ' shapes=' + c.shapes + ' title=' + c.title));
  const renderedCharts = chartShapes.filter((c) => c.shapes > 1).length;
  console.log('STATS_CHARTS_RENDERED ' + renderedCharts + '/' + chartShapes.length);

  /* ---- 月度病案量趋势：近 6 月 ---- */
  await S.step(4);
  const trendCard = page.locator('.card:has-text("月度病案量趋势")').first();
  const trendBars = trendCard.locator('.chart-holder svg rect');
  await H.waitStable(trendBars, 3, 'STATS_TREND_6M');
  await H.shotCard(trendCard, '03-stats-trend-6m', 2000);
  console.log('STATS_TREND_BARS_6M ' + (await trendBars.count()));

  /* ---- 口径一致性核对：质量等级环图合计 应与「病案总量」一致 ----
     环图只统计 quality ∈ {甲级,乙级,丙级} 的记录；无首页数据（未编目）的记录
     quality 为空串被过滤掉 ⇒ 环图合计 < 病案总量。差额即被漏掉的记录数。 */
  const gradeTotal = await page.evaluate(() => {
    const c = [...document.querySelectorAll('.card')]
      .find((x) => (x.querySelector('h3') || {}).textContent === '病案质量等级');
    const t = c && c.querySelector('.chart-holder svg text');
    return t ? parseInt(t.textContent.trim(), 10) : -1;
  });
  const uiTotalN = parseInt(uiTotal, 10);
  console.log('STATS_GRADE_DONUT_TOTAL ' + gradeTotal);
  console.log('STATS_GRADE_GAP ' + (uiTotalN - gradeTotal) +
    ' (质量等级环图合计 ' + gradeTotal + ' vs 病案总量 ' + uiTotalN + ')');

  /* ---- Top5 第一名诊断名为空？（无首页数据的记录 diagMain 为空串） ---- */
  const top5Rows = await page.evaluate(() =>
    [...document.querySelectorAll('.rank-row')].map((r) => {
      const nm = r.querySelector('.nm');
      return { name: nm ? nm.textContent.trim() : '', txt: r.textContent.replace(/\s+/g, ' ').trim() };
    }));
  const emptyNamed = top5Rows.filter((r) => !r.name);
  console.log('STATS_TOP5_ROWS_TEXT ' + top5Rows.map((r) => r.txt).join(' | '));
  console.log('STATS_TOP5_EMPTY_NAME ' + emptyNamed.length +
    (emptyNamed.length ? ' first=' + emptyNamed[0].txt : ''));

  /* ---- 切换「近 3 月」 ---- */
  await S.step(5);
  await S.click(page.locator('button:text-is("近 3 月")').first());
  await H.waitStable(trendBars, 2, 'STATS_TREND_3M');
  await sleep(1200);
  await S.shot('04-stats-trend-3m');
  console.log('STATS_TREND_BARS_3M ' + (await trendBars.count()));
  console.log('STATS_RANGE_SWITCH_ON ' +
    (await page.locator('button:text-is("近 3 月").on').count()));

  /* ---- 疾病分类构成 + 病案流转状态 ---- */
  await S.step(6);
  const catCard = page.locator('.card:has-text("疾病分类（ICD-10 大类）构成")').first();
  await H.shotCard(catCard, '05-stats-donut-category', 2000);
  const statusCard = page.locator('.card:has-text("病案流转状态")').first();
  await H.shotCard(statusCard, '06-stats-donut-status', 2000);
  console.log('STATS_DONUT_LEGEND_ITEMS ' + (await page.locator('.chart-legend .li').count()));

  /* ---- 病案质量等级 + 住院天数区间 ---- */
  await S.step(7);
  const qCard = page.locator('.card:has-text("病案质量等级")').first();
  await H.shotCard(qCard, '07-stats-donut-quality', 2000);
  const stayCard = page.locator('.card:has-text("住院天数区间分布")').first();
  await H.shotCard(stayCard, '08-stats-bars-stay', 2000);
  console.log('STATS_STAY_BARS ' + (await stayCard.locator('.chart-holder svg rect').count()));

  /* ---- 住院费用区间 + 高频主要诊断 Top 5 ---- */
  await S.step(8);
  const feeCard = page.locator('.card:has-text("住院费用区间分布")').first();
  await H.shotCard(feeCard, '09-stats-bars-fee', 2000);
  console.log('STATS_FEE_BARS ' + (await feeCard.locator('.chart-holder svg rect').count()));
  const top5 = page.locator('.card:has-text("高频主要诊断 Top 5")').first();
  await H.shotCard(top5, '10-stats-top5', 2000);
  console.log('STATS_TOP5_ROWS ' + (await top5.locator('.rank-row').count()));

  /* ---- 科室病案统计明细 ---- */
  await S.step(9);
  const deptCard = page.locator('.card:has-text("科室病案统计明细")').first();
  await H.shotCard(deptCard, '11-stats-dept-table', 2000);
  console.log('STATS_DEPT_TABLE_ROWS ' + (await deptCard.locator('tbody tr').count()));
  console.log('STATS_DEPT_TABLE_HEADERS ' + (await deptCard.locator('thead th').count()));

  /* ---- 导出报表（只验证是否真的触发下载，不落盘到仓库） ---- */
  await S.step(10);
  let dlName = null;
  page.on('download', (d) => { dlName = d.suggestedFilename(); });
  await H.bring(page.locator('button:text-is("导出报表")').first());
  await S.click(page.locator('button:text-is("导出报表")').first());
  await sleep(1500);
  await S.shot('12-stats-export');
  console.log('STATS_EXPORT_DOWNLOAD ' + (dlName || 'NONE'));
  console.log('STATS_EXPORT_TOAST ' + (await page.locator('.toast').count()));

  /* ---- 与后端接口交叉核对（UI 数字必须来自真接口） ---- */
  const recs = await S.api('/records/page?page=1&size=100');
  const recTotal = (recs.json && recs.json.data && recs.json.data.total) || 0;
  console.log('STATS_API_RECORD_TOTAL ' + recTotal + ' http=' + recs.status);
  console.log('STATS_KPI_TOTAL_MATCH ' + (String(recTotal) === String(uiTotal) ? 1 : 0) +
    ' ui=' + uiTotal + ' api=' + recTotal);

  /* ---- 两个域的「归档率」口径是否一致 ----
     统计页「归档完成率」= status != 未归档 的占比（把 待编目/已编目/已归档 都算作已归档）；
     质量看板「已归档 N 份」= 后端 archived_at IS NOT NULL 的条数（真正上架归档过的）。
     两个数差得很远 ⇒ 同屏出现会让观众以为有一个是错的。 */
  const trueArchived = (recs.json && recs.json.data && recs.json.data.list || [])
    .filter((r) => r.archivedAt).length;
  console.log('STATS_TRUE_ARCHIVED_AT ' + trueArchived);
  console.log('STATS_ARCHIVE_RATE_CALIBER gap=' + (recTotal - trueArchived) +
    ' (统计页口径 ' + uiArchiveRate + '% vs 真实 archived_at ' +
    trueArchived + '/' + recTotal + ')');

  await S.finish();
}

/* ------------------------------------------------------------------ */
/* 动线 B：质量看板                                                     */
/* ------------------------------------------------------------------ */
async function runQuality(video) {
  const S = await createShooter({
    outDir: OUT_QUALITY,
    stepPlan: QUALITY_PLAN,
    video,
    recordName: 'demo',
  });
  const H = helpers(S);
  const page = S.page;

  await S.open({ loginShot: '01-quality-login' });

  await S.step(2);
  await S.click(page.locator('button.nav-item:has-text("质量看板")').first());
  await H.waitFilled('.card.kpi .k-value', 'QUALITY_KPI');
  await sleep(1400);
  await S.shot('02-quality-kpi');

  /* ---- 四张指标卡 ---- */
  const kpiN = await page.locator('.card.kpi').count();
  const kpiVals = await page.locator('.card.kpi .k-value').allInnerTexts();
  const kpiLabels = await page.locator('.card.kpi .k-label').allInnerTexts();
  console.log('QUALITY_KPI_CARDS ' + kpiN);
  console.log('QUALITY_KPI_LABELS ' + kpiLabels.map((s) => s.trim()).join(' | '));
  console.log('QUALITY_KPI_VALUES ' + kpiVals.map((s) => s.replace(/\s+/g, ' ').trim()).join(' | '));

  /* ---- 月度 2 日归档率趋势：近 6 月 ---- */
  await S.step(4);
  const trendCard = page.locator('.card:has-text("月度 2 日归档率趋势")').first();
  const trendBars = trendCard.locator('.chart-holder svg rect');
  await H.waitStable(trendBars, 2, 'QUALITY_TREND_6M');
  await H.shotCard(trendCard, '03-quality-trend-6m', 2000);
  console.log('QUALITY_TREND_BARS_6M ' + (await trendBars.count()));
  console.log('QUALITY_TREND_EMPTY ' + (await page.locator('text=暂无月度归档率数据').count()));

  /* ---- 近 3 月 / 近 12 月 ---- */
  await S.step(5);
  await S.click(page.locator('button:text-is("近 3 月")').first());
  await H.waitStable(trendBars, 1, 'QUALITY_TREND_3M');
  await sleep(1200);
  await S.shot('04-quality-trend-3m');
  console.log('QUALITY_TREND_BARS_3M ' + (await trendBars.count()));
  console.log('QUALITY_TREND_SWITCH_ON ' + (await page.locator('button:text-is("近 3 月").on').count()));

  await S.click(page.locator('button:text-is("近 12 月")').first());
  await H.waitStable(trendBars, 3, 'QUALITY_TREND_12M');
  await sleep(1200);
  await S.shot('05-quality-trend-12m');
  console.log('QUALITY_TREND_BARS_12M ' + (await trendBars.count()));
  console.log('QUALITY_TREND_SWITCH_ON_12 ' + (await page.locator('button:text-is("近 12 月").on').count()));

  /* ---- 科室归档及时率排名 ---- */
  await S.step(6);
  const deptCard = page.locator('.card:has-text("科室归档及时率排名")').first();
  await H.shotCard(deptCard, '06-quality-dept-rank', 2000);
  console.log('QUALITY_DEPT_RANK_ROWS ' + (await deptCard.locator('tbody tr').count()));
  console.log('QUALITY_DEPT_RANK_HEADERS ' + (await deptCard.locator('thead th').count()));

  /* ---- 超期未归档催缴清单（3 天） ---- */
  await S.step(7);
  const odCard = page.locator('.card:has-text("超期未归档催缴清单")').first();
  const odRows = odCard.locator('tbody tr');
  await H.waitStable(odRows, 5, 'QUALITY_OVERDUE_3D');
  await H.shotCard(odCard, '07-quality-overdue-3d', 2000);
  const uiCount3D = await odRows.count();
  console.log('QUALITY_OVERDUE_ROWS ' + uiCount3D);
  console.log('QUALITY_OVERDUE_HEADERS ' + (await odCard.locator('thead th').count()));
  console.log('QUALITY_OVERDUE_SUB ' +
    (await odCard.locator('.card-head .sub').first().innerText().catch(() => '')).replace(/\s+/g, ' ').trim());
  console.log('QUALITY_OVERDUE_EMPTY ' + (await page.locator('text=暂无超期未归档病案').count()));

  /* ---- 状态列是否漏了状态枚举投影 ----
     后端 /archive-overdue 返回的是**原始状态枚举**（NOT_RECYCLED / CATALOGED …），
     前端 quality.js 直接 `{{ r.status }}` 渲染，没有走 api-store 的 REC_PROJ 投影，
     于是这一列显示的是英文枚举而非中文状态，且 badge 全部落在 gray 兜底色上。 */
  const odStatuses = await odCard.locator('tbody tr td:last-child .badge').allInnerTexts();
  const uniqStatuses = [...new Set(odStatuses.map((s) => s.trim()))];
  const rawEnum = uniqStatuses.filter((s) => /^[A-Z_]+$/.test(s));
  console.log('QUALITY_OVERDUE_STATUS_VALUES ' + uniqStatuses.join(','));
  console.log('QUALITY_OVERDUE_STATUS_RAW_ENUM ' + rawEnum.length +
    (rawEnum.length ? ' -> ' + rawEnum.join(',') : ''));

  /* ---- 切换 30 天阈值 ---- */
  await S.step(8);
  await S.click(page.locator('button:text-is("30 天")').first());
  await H.waitStable(odRows, 5, 'QUALITY_OVERDUE_30D');
  await sleep(1200);
  await S.shot('08-quality-overdue-30d');
  const uiCount30D = await odRows.count();
  console.log('QUALITY_OVERDUE_ROWS_30D ' + uiCount30D);
  console.log('QUALITY_OVERDUE_SWITCH_ON ' + (await page.locator('button:text-is("30 天").on').count()));

  /* ---- 导出催缴清单 ---- */
  await S.step(9);
  let dlName = null;
  page.on('download', (d) => { dlName = d.suggestedFilename(); });
  await H.bring(page.locator('button:text-is("导出清单 CSV")').first());
  await S.click(page.locator('button:text-is("导出清单 CSV")').first());
  await sleep(1500);
  await S.shot('09-quality-export');
  console.log('QUALITY_EXPORT_DOWNLOAD ' + (dlName || 'NONE'));
  console.log('QUALITY_EXPORT_TOAST ' + (await page.locator('.toast').count()));

  /* ---- 与后端接口交叉核对：UI 行数必须与后端 count 一致 ----
     注意两点：
     ① 不能拿「52」这种写死值去比 —— 演示库是活的，同事跑一次归档动线
        就会让 archived_at 变化、行数减少。真正该断言的是 **UI 与后端自洽**。
     ② 必须**同口径**比：核对时页面停在「30 天」档，UI 行数就该跟 days=30 的
        接口比；早先版本拿 30 天的 UI 去比 days=3 的接口，恒不相等（假红）。 */
  const od3 = await S.api('/archive-overdue?days=3');
  const apiCount3 = (od3.json && od3.json.data && od3.json.data.count) || 0;
  const od30 = await S.api('/archive-overdue?days=30');
  const apiCount30 = (od30.json && od30.json.data && od30.json.data.count) || 0;
  console.log('QUALITY_API_OVERDUE_3 ' + apiCount3 + ' http=' + od3.status);
  console.log('QUALITY_API_OVERDUE_30 ' + apiCount30 + ' http=' + od30.status);
  console.log('QUALITY_OVERDUE_3D_UI_VS_API ' + (uiCount3D === apiCount3 ? 1 : 0) +
    ' ui=' + uiCount3D + ' api=' + apiCount3);
  console.log('QUALITY_OVERDUE_30D_UI_VS_API ' + (uiCount30D === apiCount30 ? 1 : 0) +
    ' ui=' + uiCount30D + ' api=' + apiCount30);
  console.log('QUALITY_OVERDUE_30D_LE_3D ' + (apiCount30 <= apiCount3 ? 1 : 0) +
    ' (30天阈值行数应不多于3天阈值)');
  const tl = await S.api('/archive-timeliness?months=6');
  const t = (tl.json && tl.json.data && tl.json.data.trend) || [];
  console.log('QUALITY_API_TREND_MONTHS ' + t.length);

  await S.finish();
}

/* ------------------------------------------------------------------ */
(async () => {
  const round = (process.env.SHOT_ROUND || 'both').toLowerCase();
  const domain = (process.env.SHOT_DOMAIN || 'both').toLowerCase();
  const order = [];
  if (round === 'both' || round === 'video') order.push(true);
  if (round === 'both' || round === 'stills') order.push(false);

  for (const video of order) {
    console.log('=== ROUND ' + (video ? 'VIDEO(1280x800)' : 'STILLS(1680x1050)') +
      ' domain=' + domain + ' ===');
    if (domain !== 'quality') await runStats(video);
    if (domain !== 'stats') await runQuality(video);
  }
  console.log('ALL_DONE');
})().catch((e) => {
  console.error('FATAL ' + (e && e.message));
  process.exit(1);
});
