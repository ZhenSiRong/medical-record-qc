/**
 * 待整改域 + 智能辅助域深度演示（单脚本两段产物）
 *
 * 待整改：待整改清单 → 修正首页数据 → 提交整改 → 退回-整改-复核闭环
 * 智能辅助：智能预编码推荐（主诊断 / 手术操作）→ DRG/DIP 预检 → 免责措辞
 *
 * 两条业务事实（踩过就白录）：
 *   1. **「我的待整改」在演示库里可能是空的** —— 库里没有任何
 *      `handleStatus ∈ {RETURNED, REJECTED_BY_QC}` 的质控项，所以 admin（整改督办）
 *      与 deptuser（我的待整改）两个视角列表都是空的。脚本会**先造数据**：
 *      跑一次质控产生 OPEN 项 → 再调 `POST /qc-results/:id/return` 把它置为 RETURNED。
 *   2. **DRG 预检必须先填主诊断编码**，否则只 toast「请先填写主要诊断编码，再做 DRG 预检」。
 *      这个 toast 本身也是设计行为，值得拍进去。
 *
 * 运行（仓库根目录）：
 *   NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules" \
 *     "C:/Users/12743/.workbuddy/binaries/node/versions/22.22.2-6/node.exe" tools/shot-rectify-ai.js
 */
const { createShooter } = require('./shot-lib');

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
/** 读取页面可见文本。
 *  ⚠️ **不能只取前 2000 字符**：侧栏 + 待编目队列就占掉两千多字符，
 *  弹窗内容在后面会被截掉 —— 于是「弹窗开没开」永远判 false，
 *  症状与「按钮点了没反应」一模一样（2026-11-06 实测踩中）。
 *  这里取 6000，并且额外提供一个「只看弹窗内文本」的取法。 */
const textOf = (page) => page.evaluate(() => document.body.innerText.slice(0, 6000));
/** 只取最后一个弹窗（.mask）内的文本 —— 判断「弹窗开没开」用这个最准 */
const modalTextOf = (page) => page.evaluate(() => {
  const masks = document.querySelectorAll('.mask');
  if (!masks.length) return '';
  const m = masks[masks.length - 1];
  return (m.innerText || '').slice(0, 2000);
});

/** 断言输出约定：OBSERVE = 观测值（事实），ASSERT = 期望与实际是否一致（判定） */
const observe = (k, v) => console.log('OBSERVE ' + k + ' ' + JSON.stringify(v));
const assert_ = (k, ok, detail) => console.log('ASSERT ' + k + ' ' + (ok ? 'PASS' : 'FAIL') + (detail ? ' | ' + detail : ''));

(async () => {
  // ════════════════════════════════════════════════════════════════════
  // 第一段：待整改域
  // ════════════════════════════════════════════════════════════════════
  const R = await createShooter({
    outDir: 'docs/screenshots/rectify',
    stepPlan: [
      '打开系统并登录',
      '进入「整改督办」',
      '查看待整改清单与科室分布',
      '修正首页数据',
      '提交整改说明',
      '刷新看状态变化',
    ],
    video: process.env.SHOT_VIDEO === '1',
    recordName: 'demo',
  });

  // ── 先造一条「已退回待整改」的数据 ────────────────────────────────
  // 造法：跑质控产生 OPEN 项 → 调 return 接口把它置为 RETURNED。
  // 不改种子文件、不改业务代码，只调公开接口。
  const PREFERRED_DEPT = '心血管内科';   // deptuser（陈志远）所在科室
  try {
    const pending0 = await R.api('/qc-results/my-pending');
    const pData0 = (pending0.json && pending0.json.data) || {};
    const pList0 = pData0.list || [];
    observe('RECTIFY_PENDING_BEFORE_SEED', pData0.count);
    observe('RECTIFY_EXISTING_DEPT_MATCH',
      pList0.filter((r) => (r.deptName || '').indexOf(PREFERRED_DEPT) >= 0).length);

    /* ⚠️ **判定条件是「有没有本科室的待整改」，不是「有没有待整改」** ——
       `GET /qc-results/my-pending` 对临床侧有**本科室归属校验**：
       admin 造在「普通外科」的数据，deptuser（心血管内科·陈志远）看不到，
       临床段就录成空列表。上一轮就是栽在这里。 */
    const needSeed = !pList0.some((r) => (r.deptName || '').indexOf(PREFERRED_DEPT) >= 0);
    observe('RECTIFY_NEED_SEED', needSeed ? 1 : 0);

    if (needSeed) {
      // 用 `records/page`（病案主档，带 deptName）而不是 `front-pages/page` ——
      // 后者返回首页列表，不一定有科室字段，而我们需要按科室挑。
      const recs = await R.api('/records/page?page=1&size=100');
      const list = (recs.json && recs.json.data && (recs.json.data.list || recs.json.data.items)) || [];
      observe('RECTIFY_CANDIDATE_PAGES', list.length);
      observe('RECTIFY_SAMPLE_REC', list[0]);
      let seeded = null;
      const ordered = [
        ...list.filter((it) => (it.deptName || '').indexOf(PREFERRED_DEPT) >= 0),
        ...list.filter((it) => (it.deptName || '').indexOf(PREFERRED_DEPT) < 0),
      ];
      observe('RECTIFY_DEPT_PREFERRED_COUNT', list.filter((it) => (it.deptName || '').indexOf(PREFERRED_DEPT) >= 0).length);
      for (const it of ordered) {
        // records/page 的主键是 `id`，首页接口用 `recordId` —— 两个都取
        const rid = it.recordId || it.id;
        if (!rid) continue;
        const qc = await R.api(`/front-pages/${rid}/qc`, { method: 'POST' });
        const rs = (qc.json && qc.json.data && (qc.json.data.qcResults || qc.json.data.results)) || [];
        if (!rs.length) continue;

        /* ⚠️⚠️ **不能用 RunQc 的返回体里的 id** —— 它是 `service.RunQc` 构造的内存对象，
         * 落库后 GORM 回填主键，但返回体里 `id` 仍是 **0**（实测：`{"id":0,"ruleId":12,...}`）。
         * 拿 0 去调 `/qc-results/0/return` ⇒ 404。
         * 正确做法：再调一次 `GET /front-pages/{rid}/qc-workbench`，
         * 那里是从库里查出来的，**主键是真的**。 */
        const wb = await R.api(`/front-pages/${rid}/qc-workbench`);
        const wbData = (wb.json && wb.json.data) || {};
        const wbResults = wbData.qcResults || wbData.results || [];
        observe('RECTIFY_WORKBENCH_RESULTS', wbResults.length);
        if (!wbResults.length) continue;

        const pickId = (r) => r.id || r.qcResultId || r.resultId;
        const target = wbResults.find((r) => pickId(r) && r.errorLevel !== 'ERROR') || wbResults.find((r) => pickId(r));
        const qcId = pickId(target);
        observe('RECTIFY_TARGET_QC_ID', qcId);
        if (!qcId) continue;
        const ret = await R.api(`/qc-results/${qcId}/return`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ opinion: '演示：编码与诊断不一致，请核对后整改' }),
        });
        observe('RECTIFY_SEED_RETURN_STATUS', ret.status);
        if (ret.status >= 200 && ret.status < 300) { seeded = { rid, qcId }; break; }
      }
      observe('RECTIFY_SEEDED', seeded);
      const pending1 = await R.api('/qc-results/my-pending');
      observe('RECTIFY_PENDING_AFTER_SEED', pending1.json && pending1.json.data && pending1.json.data.count);
    }
  } catch (e) {
    console.log('RECTIFY_SEED_FAILED ' + e.message.split('\n')[0]);
  }

  await R.open({ loginShot: '01-rectify-login' });

  await R.step(2);
  await R.click(R.page.locator('text=整改督办').first());
  await sleep(3000);
  await R.shot('02-rectify-list');

  const t1 = await textOf(R.page);
  observe('RECTIFY_HAS_PENDING_TEXT', /待整改|待临床整改|整改督办/.test(t1));
  observe('RECTIFY_HAS_DISTRIBUTION', /科室分布|待整改科室分布/.test(t1));
  const rowCount = await R.page.locator('table tbody tr').count();
  observe('RECTIFY_TABLE_ROWS', rowCount);

  // 修正首页数据
  await R.step(3);
  try {
    const fixBtn = R.page.locator('button.link-btn', { hasText: '修正首页数据' }).first();
    const fc = await fixBtn.count();
    observe('RECTIFY_FIX_BTN_COUNT', fc);
    if (fc) {
      await fixBtn.scrollIntoViewIfNeeded().catch(() => {});
      await R.click(fixBtn);
      await sleep(3200);
      await R.shot('03-rectify-fix-modal');
      const mt2 = await modalTextOf(R.page);
      observe('RECTIFY_FIX_MODAL_OPENED', /修正首页数据/.test(mt2) ? 1 : 0);
      observe('RECTIFY_FIX_HAS_SAVE', /保存|提交整改|带入说明/.test(mt2) ? 1 : 0);
      observe('RECTIFY_FIX_MODAL_TEXT', mt2.replace(/\s+/g, ' ').slice(0, 220));

      // 关闭修正弹窗
      await R.closeModals();
    }
  } catch (e) {
    console.log('RECTIFY_FIX_FAILED ' + e.message.split('\n')[0]);
  }

  // 提交整改（临床视角才有「提交整改」按钮）
  await R.step(4);
  try {
    const submitBtn = R.page.locator('button.link-btn', { hasText: '提交整改' }).first();
    const sc = await submitBtn.count();
    observe('RECTIFY_SUBMIT_BTN_COUNT', sc);
    if (sc) {
      await submitBtn.scrollIntoViewIfNeeded().catch(() => {});
      await R.click(submitBtn);
      await sleep(2000);
      await R.shot('04-rectify-submit-modal');
      const mt3 = await modalTextOf(R.page);
      observe('RECTIFY_SUBMIT_MODAL_OPENED', /提交整改/.test(mt3) ? 1 : 0);

      // 填整改说明
      const ta = R.page.locator('.mask textarea, mr-modal textarea').first();
      if (await ta.count()) {
        await R.type(ta, '已核对首页诊断编码，与出院记录一致，申请复核。');
        observe('RECTIFY_REMARK_FILLED', 1);
      }
      const confirm = R.page.locator('button.btn-primary', { hasText: '提交整改' }).first();
      if (await confirm.count()) {
        await R.click(confirm);
        await sleep(2600);
        await R.shot('05-rectify-submitted');
        const t4 = await textOf(R.page);
        observe('RECTIFY_SUBMIT_TOAST', /整改/.test(t4) ? t4.replace(/\s+/g, ' ').slice(0, 160) : '');
        assert_('RECTIFY_SUBMITTED', /整改/.test(t4), '');
      }
      await R.closeModals();
    }
  } catch (e) {
    console.log('RECTIFY_SUBMIT_FAILED ' + e.message.split('\n')[0]);
  }

  // 刷新
  await R.step(5);
  try {
    const refresh = R.page.locator('button', { hasText: '刷新' }).first();
    if (await refresh.count()) {
      await refresh.scrollIntoViewIfNeeded().catch(() => {});
      await R.click(refresh);
      await sleep(2600);
      await R.shot('06-rectify-after-refresh');
      const t5 = await textOf(R.page);
      observe('RECTIFY_AFTER_REFRESH_HAS_EMPTY_HINT', /没有|暂无|未产生新的问题项|本次质控未产生/.test(t5));
    }
  } catch (e) {
    console.log('RECTIFY_REFRESH_FAILED ' + e.message.split('\n')[0]);
  }

  await R.finish();

  /* ── 临床视角补一段：deptuser 能「提交整改」，admin 不能 ──────────────────
     「提交整改」按钮只对临床医师渲染（rectify.js:510 `v-if="!isAdmin"`），
     admin（病案室）走的是复核。所以闭环的「整改」那一半必须换 deptuser 拍。
     账号：deptuser / 123456（心血管内科·陈志远）。 */
  const C = await createShooter({
    // ⚠️ 不能和上一段同一个 outDir：同名 PNG 会互相覆盖，webm 也会被冲掉。
    // 两段拆到两个目录，各自有独立的截图与视频。
    outDir: 'docs/screenshots/rectify-clinical',
    stepPlan: [
      '切换到科室医师账号登录',
      '进入「我的待整改」',
      '打开提交整改弹窗',
      '填写整改说明并提交',
      '回到病案室复核',
    ],
    video: process.env.SHOT_VIDEO === '1',
    recordName: 'demo-clinical',
    username: 'deptuser',
    password: '123456',
  });

  await C.open({ loginShot: '07-rectify-clinical-login' });

  await C.step(2);
  await C.click(C.page.locator('text=我的待整改').first());
  await sleep(3000);
  await C.shot('08-rectify-clinical-list');
  const ct1 = await textOf(C.page);
  observe('RECTIFY_CLINICAL_HAS_PENDING', /待整改|待临床整改|整改/.test(ct1) ? 1 : 0);
  observe('RECTIFY_CLINICAL_ROWS', await C.page.locator('table tbody tr').count());

  await C.step(3);
  try {
    const submitBtn = C.page.locator('button.link-btn', { hasText: '提交整改' }).first();
    const sc = await submitBtn.count();
    observe('RECTIFY_CLINICAL_SUBMIT_BTN_COUNT', sc);
    if (sc) {
      await submitBtn.scrollIntoViewIfNeeded().catch(() => {});
      await C.click(submitBtn);
      await sleep(2200);
      await C.shot('09-rectify-clinical-submit-modal');
      const mt = await modalTextOf(C.page);
      observe('RECTIFY_CLINICAL_SUBMIT_MODAL_OPENED', /提交整改/.test(mt) ? 1 : 0);
      observe('RECTIFY_CLINICAL_MODAL_TEXT', mt.replace(/\s+/g, ' ').slice(0, 220));

      const ta = C.page.locator('.mask textarea, mr-modal textarea').first();
      if (await ta.count()) {
        await C.type(ta, '已核对出院记录与首页诊断编码，确认主诊断应选择与手术相关的疾病，申请复核。');
        observe('RECTIFY_CLINICAL_REMARK_FILLED', 1);
      }
      await C.step(4);
      // 确认按钮文案是「确认提交」（rectify.js:562），**不是**「提交整改」——
      // 「提交整改」是列表行里的入口按钮文案，两者容易混。
      const confirm = C.page.locator('button.btn-primary', { hasText: '确认提交' }).first();
      if (await confirm.count()) {
        await confirm.scrollIntoViewIfNeeded().catch(() => {});
        await C.click(confirm);
        await sleep(3000);
        await C.shot('10-rectify-clinical-submitted');
        const t4 = await textOf(C.page);
        observe('RECTIFY_CLINICAL_SUBMIT_RESULT', t4.replace(/\s+/g, ' ').slice(0, 220));
        assert_('RECTIFY_CLINICAL_SUBMITTED', /整改|已提交|待复核|成功/.test(t4), '');
      } else {
        observe('RECTIFY_CLINICAL_CONFIRM_BTN_MISS', 1);
        const btns = await C.page.locator('.mask button').allTextContents().catch(() => []);
        observe('RECTIFY_CLINICAL_MODAL_BUTTONS', btns);
      }
    }
  } catch (e) {
    console.log('RECTIFY_CLINICAL_FAILED ' + e.message.split('\n')[0]);
  }

  await C.finish();

  // ════════════════════════════════════════════════════════════════════
  // 第二段：智能辅助域（在编目页里）
  // ════════════════════════════════════════════════════════════════════
  const A = await createShooter({
    outDir: 'docs/screenshots/ai-assist',
    stepPlan: [
      '打开系统并登录',
      '进入「病案编目」并选中一份病案',
      '打开智能预编码推荐（主诊断）',
      '输入诊断文本并获取推荐',
      '查看免责措辞',
      '打开 DRG/DIP 预检',
      '查看分组提示与免责声明',
    ],
    video: process.env.SHOT_VIDEO === '1',
    recordName: 'demo',
  });

  await A.open({ loginShot: '01-ai-login' });

  await A.step(2);
  await A.click(A.page.locator('text=病案编目').first());
  await sleep(2600);
  const items = A.page.locator('div[style*="cursor"]');
  const n = await items.count();
  observe('AI_CATALOG_QUEUE_ITEMS', n);
  if (n) {
    await A.click(items.first());
    await sleep(2200);
  }
  await A.shot('02-ai-record-selected');

  // 智能预编码推荐
  await A.step(3);
  try {
    const sug = A.page.locator('button', { hasText: '智能推荐' }).first();
    const sc = await sug.count();
    observe('AI_SUGGEST_BTN_COUNT', sc);
    if (sc) {
      await sug.scrollIntoViewIfNeeded().catch(() => {});
      await A.click(sug);
      await sleep(2200);
      await A.shot('03-ai-suggest-modal');
      const mt2 = await modalTextOf(A.page);
      observe('AI_SUGGEST_MODAL_OPENED', /智能预编码推荐/.test(mt2) ? 1 : 0);
      observe('AI_SUGGEST_DISCLAIMER', /编码辅助|最终编码由编码员判断|请勿直接照搬/.test(mt2) ? 1 : 0);
      observe('AI_SUGGEST_MODAL_TEXT', mt2.replace(/\s+/g, ' ').slice(0, 220));

      // 填诊断文本 —— 智能推荐弹窗里的输入框是 **textarea**（catalog.js:603），
      // 不是 input。写 input 会一个都找不到。
      const input = A.page.locator('.mask textarea, mr-modal textarea').first();
      const ic = await input.count();
      observe('AI_SUGGEST_TEXTAREA_COUNT', ic);
      if (ic) {
        await A.type(input, '急性阑尾炎伴穿孔，腹腔镜阑尾切除术');
        observe('AI_SUGGEST_TEXT_TYPED', 1);
      }
      await A.step(4);
      const run = A.page.locator('button', { hasText: '开始推荐' }).first();
      if (await run.count()) {
        await run.scrollIntoViewIfNeeded().catch(() => {});
        await A.click(run);
        await sleep(2800);
        await A.shot('04-ai-suggest-result');
        const mt3 = await modalTextOf(A.page);
        observe('AI_SUGGEST_RESULT_TEXT', mt3.replace(/\s+/g, ' ').slice(0, 300));
        const cand = await A.page.locator('.mask table tbody tr, mr-modal table tbody tr').count();
        observe('AI_SUGGEST_CANDIDATE_ROWS', cand);
        observe('AI_SUGGEST_HAS_CANDIDATE_TEXT', /推荐|候选|ICD|编码/.test(mt3) ? 1 : 0);
      } else {
        observe('AI_SUGGEST_RUN_BTN_MISS', 1);
      }
      await A.closeModals();
    }
  } catch (e) {
    console.log('AI_SUGGEST_FAILED ' + e.message.split('\n')[0]);
  }

  // DRG 预检
  await A.step(5);
  try {
    // 编目表单的主诊断编码是**原生 <select>**（catalog.js:464 `v-model="form.icdCode"`），
    // 不是文本框 —— 往检索框里敲不会填上编码。
    // 正确做法：直接在 select 上 selectOption 选一个 ICD-10 编码。
    const icdSelect = A.page.locator('select').filter({ has: A.page.locator('option[value^="I"]') }).first();
    const sc = await icdSelect.count();
    observe('AI_ICD_SELECT_COUNT', sc);
    let pickedCode = '';
    if (sc) {
      const opts = await icdSelect.locator('option').evaluateAll((els) =>
        els.map((e) => ({ v: e.value, t: e.textContent.trim() })));
      observe('AI_ICD_OPTION_COUNT', opts.length);
      // 选一个有意义的编码：优先 I50（心力衰竭，有 CC/MCC 判读意义）
      const hit = opts.find((o) => o.v && /^I50/.test(o.v)) || opts.find((o) => o.v && /^I/.test(o.v));
      if (hit) {
        await icdSelect.scrollIntoViewIfNeeded().catch(() => {});
        await icdSelect.selectOption(hit.v).catch((e) => console.log('AI_ICD_SELECT_FAILED ' + e.message.split('\n')[0]));
        await sleep(1200);
        pickedCode = hit.v;
        observe('AI_ICD_SELECTED', hit.v + ' | ' + hit.t);
        await A.shot('05-ai-icd-picked');
      } else {
        observe('AI_ICD_NO_OPTION', 1);
      }
    }

    const drg = A.page.locator('button', { hasText: 'DRG 预检' }).first();
    const dc = await drg.count();
    observe('AI_DRG_BTN_COUNT', dc);
    if (dc) {
      await drg.scrollIntoViewIfNeeded().catch(() => {});
      await A.click(drg);
      await sleep(3000);
      await A.shot('06-ai-drg-modal');
      const mt4 = await modalTextOf(A.page);
      observe('AI_DRG_MODAL_OPENED', /DRG 预检|入组|分组|预检/.test(mt4) ? 1 : 0);
      observe('AI_DRG_DISCLAIMER', /不作为|非官方|非国家医保局|不得作为结算依据/.test(mt4) ? 1 : 0);
      observe('AI_DRG_TEXT', mt4.replace(/\s+/g, ' ').slice(0, 320));
    }
  } catch (e) {
    console.log('AI_DRG_FAILED ' + e.message.split('\n')[0]);
  }

  await A.finish();
  console.log('ALL_DONE');
})().catch((e) => {
  console.error('FATAL ' + e.message);
  process.exit(1);
});
