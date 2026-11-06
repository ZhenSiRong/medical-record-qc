/**
 * 归档域 + 借阅域 端到端演示（单脚本两段产物）
 *
 * 归档域（docs/screenshots/archive/）：登录 → 列表 → 详情抽屉 → 登记表单(填不提交)
 *        → 批量归档入口(勾选展示,不真写库) → 导出清单
 * 借阅域（docs/screenshots/borrow/）：登录 → 台账概览 → 提交申请(真写库)
 *        → 审批通过(含借出) → 续借 → 归还 → 逾期视角 → 导出台账
 *
 * 定位要点（踩过的坑，都写在代码旁）：
 *   1. **表格要按「数据表」定位**：`table.data tbody tr`。用 `table tbody tr` 会把空态行算进去。
 *   2. **归档页工具栏有两个 select**：第 1 个是科室、第 2 个才是状态。
 *      用 `select.first()` 找状态会拿到科室 → 选不中。
 *   3. **详情是 `.drawer` 抽屉，不是 mr-modal**：`.mask` 关闭逻辑对它无效，
 *      要点 `.drawer-head` 里的「关闭」按钮；抽屉有全屏 `.drawer-mask`，不关会挡住后续点击。
 *   4. **原生 select 在 headless 下点不出下拉列表**：只 glide（人类化移动）+ selectOption，
 *      不要 click select 打开下拉，否则后续鼠标事件会被原生 popup 吞掉。
 *   5. **行内按钮必须定位到「那一行」**：台账里可能同时存在别人的待审批单，
 *      全局 `text=通过` 会点到别人的行。用关键词搜索框把列表过滤到只剩目标行再点。
 *   6. **「续借」只在「已借出」行渲染**，所以续借必须排在归还之前。
 *
 * 运行（仓库根目录）：
 *   NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules" \
 *     "C:/Users/12743/.workbuddy/binaries/node/versions/22.22.2-6/node.exe" tools/shot-archive-borrow.js
 *   SHOT_VIDEO=1 ... # 同上，加环境变量即录视频（archive/demo.webm 与 borrow/demo.webm）
 */
const { createShooter } = require('./shot-lib');

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** 页面可见文本（找不到元素时的诊断出口） */
const textOf = (page) => page.evaluate(() => document.body.innerText.slice(0, 2000));

/** 断言：输出 "NAME 1|0"（1=通过，0=失败），detail 仅在失败时打印 */
function assert(name, ok, detail) {
  console.log(name + ' ' + (ok ? 1 : 0) + (ok || detail === undefined ? '' : ' # ' + detail));
  return !!ok;
}
/** 观测：输出 "NAME <实际值>"（原始计数/文本，供报告引用） */
function observe(name, value) {
  console.log(name + ' ' + value);
}

/**
 * 取登录 token，带重试。
 * 为什么不用 createShooter 自带的登录：它只尝试一次，遇并发抢占导致的
 * 「后端刚好重启」就直接抛 fetch failed，整个脚本白跑。
 * 这里自己轮询等待后端就绪，把 token 交给 createShooter 的 presetToken。
 */
async function getTokenWithRetry(maxWaitMs = 180000) {
  const API = process.env.SHOT_API || 'http://127.0.0.1:8080/api';
  const t0 = Date.now();
  let tries = 0;
  while (Date.now() - t0 < maxWaitMs) {
    tries++;
    try {
      const res = await fetch(API + '/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: 'admin', password: 'admin123' }),
      });
      const j = await res.json().catch(() => ({}));
      if (j && j.data && j.data.token) {
        console.log('TOKEN_OK len=' + String(j.data.token).length + ' tries=' + tries);
        return j.data.token;
      }
    } catch (e) { /* 服务未就绪，继续等 */ }
    if (tries % 10 === 0) console.log('WAIT_API tries=' + tries);
    await sleep(3000);
  }
  throw new Error('后端 API 在 ' + Math.round(maxWaitMs / 1000) + 's 内未就绪');
}

/**
 * 等前端静态服务器就绪（8090）。并发跑录屏时后端/前端会被抢占重启，
 * 这里轮询到能拿到 HTML 才开录，避免白跑一轮。
 */
async function waitFrontend(maxWaitMs = 120000) {
  const BASE = process.env.SHOT_BASE || 'http://127.0.0.1:8090/frontend/index.html';
  const t0 = Date.now();
  let tries = 0;
  while (Date.now() - t0 < maxWaitMs) {
    tries++;
    try {
      const res = await fetch(BASE, { method: 'GET' });
      const txt = await res.text();
      if (res.ok && txt.includes('<html')) {
        console.log('FRONTEND_OK tries=' + tries);
        return true;
      }
    } catch (e) { /* 未就绪，继续等 */ }
    if (tries % 10 === 0) console.log('WAIT_FRONTEND tries=' + tries);
    await sleep(3000);
  }
  throw new Error('前端静态服务器在 ' + Math.round(maxWaitMs / 1000) + 's 内未就绪');
}

/**
 * 人类化地操作原生 select：先把鼠标滑过去（让录屏里能看到指针移动到控件），
 * 再用 selectOption 赋值。headless 下 click 会弹原生下拉且不渲染，选不中。
 */
async function pickSelect(S, locator, value) {
  const box = await locator.boundingBox().catch(() => null);
  if (box) {
    await S.glide(box.x + box.width / 2, box.y + box.height / 2);
    await sleep(320);
  }
  await locator.selectOption(value);
  await sleep(700);
}

/**
 * 在表格里找到包含指定文本、且状态列匹配的那一行。
 * ⚠️ 必须带状态过滤：同一病案号可能有多条历史借阅记录（多轮演示会累积），
 * 只按文本找会命中第一条**旧的**已归还记录，后续「通过/续借/归还」按钮全找不到。
 * @param {string} text      行内应包含的文本（病案号）
 * @param {string} [status]  状态列（td[7]）应包含的状态，如「待审批」
 */
async function rowByText(page, text, status) {
  const rows = page.locator('table.data tbody tr');
  const n = await rows.count();
  for (let i = 0; i < n; i++) {
    const row = rows.nth(i);
    const t = await row.innerText().catch(() => '');
    if (!String(t).includes(text)) continue;
    if (status) {
      const st = await row.locator('td').nth(7).innerText().catch(() => '');
      if (!new RegExp(status).test(st)) continue;
    }
    return row;
  }
  return null;
}

/** 关闭病案详情抽屉（.drawer，不是 mr-modal） */
async function closeDrawer(S) {
  const btn = S.page.locator('.drawer-head button', { hasText: '关闭' }).first();
  if (await btn.count()) {
    await S.click(btn);
    await sleep(900);
    return true;
  }
  return false;
}

(async () => {
  // SHOT_SEG=archive|borrow 可单独跑某一段（避免一段失败拖垮另一段的视频）
  const SEG = process.env.SHOT_SEG || 'both';
  const TOKEN = await getTokenWithRetry();
  await waitFrontend();

  // ════════════════════════════════════════════════════════════════════
  // 第一段：归档域
  // ════════════════════════════════════════════════════════════════════
  if (SEG === 'both' || SEG === 'archive') {
  const A = await createShooter({
    outDir: 'docs/screenshots/archive',
    presetToken: TOKEN,
    stepPlan: [
      '打开系统并登录',
      '进入「病案归档」',
      '查看归档列表与状态分布',
      '查看病案详情',
      '填写病案登记表单',
      '批量归档操作入口',
      '导出归档清单',
    ],
    video: process.env.SHOT_VIDEO === '1',
    recordName: 'demo',
  });

  await A.open({ loginShot: '01-archive-login' });

  // ── 步骤 2：进入归档页 ───────────────────────────────────────────────
  await A.step(2);
  await A.click(A.page.locator('text=病案归档').first());
  await sleep(3000);

  // ── 步骤 3：查看归档列表与状态分布 ───────────────────────────────────
  await A.step(3);
  await A.shot('02-archive-list');

  observe('ARCHIVE_ROWS', await A.page.locator('table.data tbody tr').count());   // 每页行数
  observe('ARCHIVE_TOTAL_HIT', (await textOf(A.page)).match(/命中\s*(\d+)\s*条/)?.[1] ?? 'NA');

  // 状态列（第 9 列）的状态种类数 —— 证明列表里有多种归档状态
  const kinds = await A.page.evaluate(() => {
    const set = new Set();
    document.querySelectorAll('table.data tbody tr').forEach((tr) => {
      const td = tr.children[8];
      if (td) set.add(td.innerText.trim());
    });
    return set.size;
  });
  observe('ARCHIVE_STATUS_KINDS', kinds);

  // ── 步骤 4：查看详情抽屉 ─────────────────────────────────────────────
  await A.step(4);
  const detailBtn = A.page.locator('table.data tbody tr button.link-btn', { hasText: '详情' }).first();
  if (await detailBtn.count()) {
    await A.click(detailBtn);
    await sleep(2200);
    await A.shot('03-archive-detail');
    const drawer = A.page.locator('.drawer').first();
    const drawerOpen = (await drawer.count()) > 0 && (await drawer.isVisible().catch(() => false));
    const dTitle = drawerOpen ? await drawer.locator('.drawer-head h3').innerText().catch(() => '') : '';
    assert('ARCHIVE_DETAIL_OPENED', drawerOpen && /病案详情/.test(dTitle), 'title=' + dTitle);
    observe('ARCHIVE_DETAIL_FIELDS', drawerOpen ? await drawer.locator('.desc-list dt').count() : 0);
    const dText = drawerOpen ? await drawer.innerText().catch(() => '') : '';
    assert('ARCHIVE_DETAIL_HAS_FEE', /住院费用/.test(dText) && /诊断/.test(dText), '');
    await closeDrawer(A);
    observe('ARCHIVE_DETAIL_CLOSED', (await A.page.locator('.drawer').count()) === 0 ? 1 : 0);
  } else {
    assert('ARCHIVE_DETAIL_OPENED', false, '详情按钮未找到');
  }

  // ── 步骤 5：病案登记表单（填字段，不提交）─────────────────────────────
  await A.step(5);
  const addBtn = A.page.locator('button', { hasText: '+ 病案登记' }).first();
  if (await addBtn.count()) {
    await A.click(addBtn);
    await sleep(2000);
    await A.shot('04-archive-register-empty');

    const modal = A.page.locator('.mask .modal').last();
    const mTitle = await modal.locator('.modal-head h3').innerText().catch(() => '');
    assert('ARCHIVE_REGISTER_MODAL_OPENED', /病案登记/.test(mTitle), 'title=' + mTitle);
    observe('ARCHIVE_REGISTER_FORM_FIELDS', await modal.locator('.field').count());

    // 姓名
    const nameInput = modal.locator('.field', { hasText: '患者姓名' }).locator('input').first();
    if (await nameInput.count()) {
      await A.type(nameInput, '演示患者甲');
      await sleep(500);
    }
    // 年龄
    const ageInput = modal.locator('.field', { hasText: '性别 / 年龄' }).locator('input[type="number"]').first();
    if (await ageInput.count()) {
      await A.type(ageInput, '58');
      await sleep(400);
    }
    // 主要诊断（ICD-10）：modal 内第 4 个 select 才是「主要诊断」
    // （顺序：性别 / 科室 / 医师 / 主要诊断 / 手术操作 / 质量等级）
    const mSelects = modal.locator('select');
    observe('ARCHIVE_REGISTER_SELECTS', await mSelects.count());
    if ((await mSelects.count()) >= 4) {
      await pickSelect(A, mSelects.nth(3), 'H25.9');
      const icdVal = await mSelects.nth(3).inputValue().catch(() => '');
      assert('ARCHIVE_REGISTER_ICD_SET', !!icdVal, 'icd=' + icdVal);
    }
    // 住院费用
    const feeInput = modal.locator('.field', { hasText: '住院费用' }).locator('input').first();
    if (await feeInput.count()) {
      await A.type(feeInput, '12800');
      await sleep(400);
    }
    await A.shot('05-archive-register-filled');

    const filled = await nameInput.inputValue().catch(() => '');
    assert('ARCHIVE_REGISTER_NAME_FILLED', filled.length > 0, 'name=' + filled);

    // **不提交**：点「取消」关闭（任务要求「填几个字段但不要真提交」）
    const cancelBtn = modal.locator('.modal-foot button', { hasText: '取消' }).first();
    if (await cancelBtn.count()) {
      await A.click(cancelBtn);
      await sleep(1200);
    }
    const stillOpen = (await A.page.locator('.mask .modal').count()) > 0;
    assert('ARCHIVE_REGISTER_NOT_SUBMITTED', !stillOpen, '弹窗仍打开，可能已写入');
    const after = await A.api('/records/page?size=100');
    observe('ARCHIVE_RECORDS_TOTAL', after.json?.data?.total ?? 'NA');
  } else {
    assert('ARCHIVE_REGISTER_MODAL_OPENED', false, '登记按钮未找到');
  }

  // ── 步骤 6：批量归档入口（只展示，不真归档）────────────────────────────
  await A.step(6);
  // 工具栏第 2 个 select 是「归档状态」，第 1 个是科室
  const toolbarSelects = A.page.locator('.toolbar select');
  observe('ARCHIVE_TOOLBAR_SELECTS', await toolbarSelects.count());
  if ((await toolbarSelects.count()) >= 2) {
    await pickSelect(A, toolbarSelects.nth(1), '未归档');
    await sleep(1400);
    await A.shot('06-archive-filter-unarchived');
    observe('ARCHIVE_UNARCHIVED_ROWS', await A.page.locator('table.data tbody tr').count());

    // 勾选第一行 → 「批量归档（N）」按钮变可点（展示入口）
    const cb = A.page.locator('table.data tbody tr input[type="checkbox"]').first();
    if (await cb.count()) {
      await cb.check({ force: true, timeout: 8000 }).catch(() => {});
      await sleep(1000);
      await A.shot('07-archive-batch-entry');
      observe('ARCHIVE_CHECKED_COUNT', await A.page.locator('table.data tbody tr input[type="checkbox"]:checked').count());
      const batchBtn = A.page.locator('button', { hasText: '批量归档' }).first();
      const bTxt = await batchBtn.innerText().catch(() => '');
      const bDisabled = await batchBtn.isDisabled().catch(() => true);
      assert('ARCHIVE_BATCH_BTN_ENABLED', !bDisabled, 'btn=' + bTxt.trim() + ' disabled=' + bDisabled);
      observe('ARCHIVE_BATCH_BTN_TEXT', bTxt.replace(/\s+/g, ' ').trim());
      // **不点批量归档**（避免把种子数据全部归档掉），取消勾选还原
      await cb.uncheck({ force: true, timeout: 8000 }).catch(() => {});
      await sleep(700);
    }
  }

  // ── 步骤 7：导出清单 ────────────────────────────────────────────────
  await A.step(7);
  const expBtn = A.page.locator('button', { hasText: '导出清单' }).first();
  if (await expBtn.count()) {
    await A.click(expBtn);
    await sleep(1500);
    const t = await textOf(A.page);
    await A.shot('08-archive-export');
    assert('ARCHIVE_EXPORT_TOAST', /已导出/.test(t), t.replace(/\s+/g, ' ').slice(0, 160));
  } else {
    assert('ARCHIVE_EXPORT_TOAST', false, '导出按钮未找到');
  }

  await A.finish();
  }

  // ════════════════════════════════════════════════════════════════════
  // 第二段：借阅域
  // ════════════════════════════════════════════════════════════════════
  if (SEG === 'both' || SEG === 'borrow') {
  const B = await createShooter({
    outDir: 'docs/screenshots/borrow',
    presetToken: TOKEN,
    stepPlan: [
      '打开系统并登录',
      '进入「病案借阅」',
      '查看待审批与台账概览',
      '提交借阅申请',
      '病案室审批通过',
      '办理借出并续借',
      '登记归还核销',
      '查看台账与逾期视角',
    ],
    video: process.env.SHOT_VIDEO === '1',
    recordName: 'demo',
  });

  await B.open({ loginShot: '01-borrow-login' });

  // ── 步骤 2：进入借阅页 ───────────────────────────────────────────────
  await B.step(2);
  await B.click(B.page.locator('text=病案借阅').first());
  await sleep(3000);

  // ── 步骤 3：查看待审批与台账概览 ──────────────────────────────────────
  await B.step(3);
  await B.shot('02-borrow-overview');

  observe('BORROW_KPI_CARDS', await B.page.locator('.card.kpi').count());
  observe('BORROW_TOTAL_HIT', (await textOf(B.page)).match(/命中\s*(\d+)\s*条/)?.[1] ?? 'NA');
  observe('BORROW_TABLE_ROWS', await B.page.locator('table.data tbody tr').count());
  observe('BORROW_STATUS_FILTER_OPTIONS', await B.page.locator('.toolbar select option').count());
  observe('BORROW_HAS_LEDGER_BTN', await B.page.locator('button', { hasText: '导出台账' }).count());

  // ── 步骤 4：提交借阅申请（真写库）─────────────────────────────────────
  await B.step(4);
  // 选一个「已归档(在架)」且**台账里从未出现过**的病案。
  // ⚠️ 排除条件不能只看「进行中」状态：同一病案号可能有多条**已归还**的历史记录，
  // 多轮演示会累积，关键词过滤后仍会命中多行 → 行内按钮定位到旧行 → 断言全灭。
  // 所以这里排除**所有**出现在台账里的病案号，保证新申请是唯一一行。
  const led = await B.api('/borrow-ledger/page?size=100');
  const ledList = (led.json && led.json.data && led.json.data.list) || [];
  const everUsed = new Set(ledList.map((b) => b.recordNo));
  const busy = new Set(
    ledList
      .filter((b) => ['APPLYING', 'APPROVED', 'BORROWED', 'OVERDUE'].includes(b.status))
      .map((b) => b.recordNo)
  );
  const recs = await B.api('/records/page?size=100');
  const archList = ((recs.json && recs.json.data && recs.json.data.list) || []).filter(
    (r) => r.status === 'ARCHIVED'
  );
  const free = archList.filter((r) => !everUsed.has(r.recordNo));
  observe('BORROW_ARCHIVED_POOL', archList.length);
  observe('BORROW_BUSY_RECORDS', busy.size);
  observe('BORROW_FREE_POOL', free.length);

  let target = null;
  const applyBtn = B.page.locator('button', { hasText: '+ 借阅申请' }).first();
  if ((await applyBtn.count()) && free.length) {
    await B.click(applyBtn);
    await sleep(2000);
    await B.shot('03-borrow-apply-form');

    const modal = B.page.locator('.mask .modal').last();
    const mTitle = await modal.locator('.modal-head h3').innerText().catch(() => '');
    assert('BORROW_APPLY_MODAL_OPENED', /病案借阅申请/.test(mTitle), 'title=' + mTitle);
    const mSelects = modal.locator('select');
    observe('BORROW_FORM_SELECTS', await mSelects.count());
    observe('BORROW_FORM_FIELDS', await modal.locator('.field').count());

    // 借阅病案：只列「已归档」病案，直接按病案号选中目标
    const mrSelect = mSelects.nth(0);
    const opts = await mrSelect.locator('option').allTextContents().catch(() => []);
    target = free.find((r) => opts.some((o) => o.includes(r.recordNo))) || null;
    observe('BORROW_MR_OPTION_COUNT', opts.length);
    observe('BORROW_MR_TARGET', target ? target.recordNo : 'NONE');
    if (!target) {
      console.log('BORROW_APPLY_SUBMITTED 0 # 没有可用病案');
    } else {
      await pickSelect(B, mrSelect, target.recordNo);
      // 借阅人：默认是当前用户姓名，可能不在医生字典里（初始显示空白）→ 显式选一个真实医生
      if ((await mSelects.count()) >= 2) {
        await pickSelect(B, mSelects.nth(1), '张建国');
        const borrowerVal = await mSelects.nth(1).inputValue().catch(() => '');
        observe('BORROW_BORROWER_SELECTED', borrowerVal || '(空)');
      }
      // 借阅事由
      if ((await mSelects.count()) >= 4) await pickSelect(B, mSelects.nth(3), '临床病例讨论');
      // 借阅期限：填 30 天（观察实际应还日期是否真按 30 天）
      const daysInput = modal.locator('.field', { hasText: '借阅期限' }).locator('input').first();
      if (await daysInput.count()) {
        await B.type(daysInput, '30');
        await sleep(400);
      }
      // 补充说明
      const ta = modal.locator('textarea').first();
      if (await ta.count()) {
        await B.type(ta, '教学演示：病案借阅全流程');
        await sleep(500);
      }
      await B.shot('04-borrow-apply-filled');

      const submit = modal.locator('.modal-foot button', { hasText: '提交申请' }).first();
      if (await submit.count()) {
        await B.click(submit);
        await sleep(1600);
        const t = await textOf(B.page);   // toast 2.6s 自动消失，必须立刻读
        await B.shot('05-borrow-applied');
        assert('BORROW_APPLY_SUBMITTED', /借阅申请已提交/.test(t), t.replace(/\s+/g, ' ').slice(0, 180));
      } else {
        assert('BORROW_APPLY_SUBMITTED', false, '提交按钮未找到');
      }
    }
  } else {
    assert('BORROW_APPLY_SUBMITTED', false, '申请按钮未找到或无可用病案');
  }

  // ── 步骤 5：定位刚提交的那一行并审批通过 ─────────────────────────────
  await B.step(5);
  if (target) {
    // 用关键词搜索框把列表过滤到只剩目标行，避免点到别人的待审批单
    const kw = B.page.locator('input[placeholder*="搜索单号"]').first();
    if (await kw.count()) {
      await B.type(kw, target.recordNo);
      await sleep(1600);
      await B.shot('06-borrow-target-row');
      observe('BORROW_TARGET_ROWS', await B.page.locator('table.data tbody tr').count());
    }
    const row = await rowByText(B.page, target.recordNo, '待审批');
    if (row) {
      const stCell = await row.locator('td').nth(7).innerText().catch(() => '');
      assert('BORROW_TARGET_IS_PENDING', /待审批/.test(stCell), 'status=' + stCell.trim());

      const approveBtn = row.locator('button.link-btn', { hasText: /^通过$/ }).first();
      if (await approveBtn.count()) {
        await B.click(approveBtn);
        await sleep(2000);
        const t = await textOf(B.page);
        await B.shot('07-borrow-approved');
        assert('BORROW_APPROVED', /已审批通过并借出|已批准借阅/.test(t), t.replace(/\s+/g, ' ').slice(0, 180));
        // 审批链里 approve+lend 一起做 ⇒ 状态直接变「已借出」
        const row2 = await rowByText(B.page, target.recordNo, '已借出');
        const st2 = row2 ? await row2.locator('td').nth(7).innerText().catch(() => '') : '';
        assert('BORROW_STATUS_LENT', /已借出/.test(st2), 'status=' + st2.trim());
        observe('BORROW_LEDGER_ID_COLUMN', row2 ? (await row2.locator('td').nth(0).innerText().catch(() => '')).trim() : 'NA');
      } else {
        assert('BORROW_APPROVED', false, '通过按钮未找到');
      }
    } else {
      assert('BORROW_APPROVED', false, '未找到目标行');
    }
  }

  // ── 步骤 6：续借（只在「已借出」行有，必须在归还之前）─────────────────
  await B.step(6);
  if (target) {
    const row = await rowByText(B.page, target.recordNo, '已借出');
    // 页面上没有独立「借出」按钮（审批通过即内部链式完成借出）—— 记录这一事实
    const lendBtns = await B.page.locator('button.link-btn', { hasText: /^借出$/ }).count();
    observe('BORROW_LEND_BTN_ABSENT', lendBtns);
    assert('BORROW_LEND_BTN_ABSENT', lendBtns === 0, '存在独立借出按钮 ' + lendBtns);

    if (row) {
      const dueBefore = await row.locator('td').nth(6).innerText().catch(() => '');
      const renewBtn = row.locator('button.link-btn', { hasText: /^续借$/ }).first();
      observe('BORROW_RENEW_BTN_COUNT', await row.locator('button.link-btn', { hasText: /^续借$/ }).count());
      if (await renewBtn.count()) {
        await B.click(renewBtn);
        await sleep(2000);
        const t = await textOf(B.page);
        await B.shot('08-borrow-renewed');
        assert('BORROW_RENEWED', /已续借\s*7\s*天/.test(t), t.replace(/\s+/g, ' ').slice(0, 180));
        const row2 = await rowByText(B.page, target.recordNo, '已借出');
        const dueAfter = row2 ? await row2.locator('td').nth(6).innerText().catch(() => '') : '';
        observe('BORROW_DUE_BEFORE', dueBefore.replace(/\s+/g, ' ').trim());
        observe('BORROW_DUE_AFTER_RENEW', dueAfter.replace(/\s+/g, ' ').trim());
      } else {
        assert('BORROW_RENEWED', false, '续借按钮未找到');
      }
    }
  }

  // ── 步骤 7：归还核销 ────────────────────────────────────────────────
  await B.step(7);
  if (target) {
    const row = await rowByText(B.page, target.recordNo, '已借出');
    const retBtn = row ? row.locator('button.link-btn', { hasText: /^归还$/ }).first() : null;
    if (retBtn && (await retBtn.count())) {
      await B.click(retBtn);
      await sleep(2200);
      const t = await textOf(B.page);
      await B.shot('09-borrow-returned');
      assert('BORROW_RETURNED', /已登记归还|归还核销完成/.test(t), t.replace(/\s+/g, ' ').slice(0, 180));
      const row2 = await rowByText(B.page, target.recordNo, '已归还');
      const st = row2 ? await row2.locator('td').nth(7).innerText().catch(() => '') : '';
      assert('BORROW_STATUS_RETURNED', /已归还/.test(st), 'status=' + st.trim());
      const due = row2 ? (await row2.locator('td').nth(6).innerText().catch(() => '')).replace(/\s+/g, ' ').trim() : '';
      observe('BORROW_DUE_DATE_FINAL', due);
      observe('BORROW_FORM_DAYS_REQUESTED', 30);
    } else {
      assert('BORROW_RETURNED', false, '归还按钮未找到');
    }
  }

  // ── 步骤 8：台账与逾期视角 ──────────────────────────────────────────
  await B.step(8);
  // 清掉关键词过滤，回到全量台账
  const kw = B.page.locator('input[placeholder*="搜索单号"]').first();
  if (await kw.count()) {
    await kw.fill('');
    await sleep(1200);
  }
  // 切到「已逾期」筛选
  const bSelects = B.page.locator('.toolbar select');
  if ((await bSelects.count()) >= 1) {
    await pickSelect(B, bSelects.nth(0), '已逾期');
    await sleep(1400);
    await B.shot('10-borrow-overdue');
    observe('BORROW_OVERDUE_ROWS', await B.page.locator('table.data tbody tr').count());
    const odText = await B.page.locator('table.data').innerText().catch(() => '');
    assert('BORROW_OVERDUE_MARKED', /逾期\s*\d+\s*天/.test(odText), '');
    await pickSelect(B, bSelects.nth(0), 'all');
    await sleep(1200);
  }
  // 导出台账
  const bExpBtn = B.page.locator('button', { hasText: '导出台账' }).first();
  if (await bExpBtn.count()) {
    await B.click(bExpBtn);
    await sleep(1600);
    const t = await textOf(B.page);
    await B.shot('11-borrow-ledger-export');
    assert('BORROW_LEDGER_EXPORT_TOAST', /已导出/.test(t), t.replace(/\s+/g, ' ').slice(0, 160));
  } else {
    assert('BORROW_LEDGER_EXPORT_TOAST', false, '导出台账按钮未找到');
  }
  observe('BORROW_LEDGER_TOTAL_FINAL', (await textOf(B.page)).match(/命中\s*(\d+)\s*条/)?.[1] ?? 'NA');

  await B.finish();
  }
  console.log('ALL_DONE');
})().catch((e) => {
  console.error('FATAL ' + (e && e.stack ? e.stack.split('\n').slice(0, 4).join(' | ') : e));
  process.exit(1);
});
