/**
 * 端到端截图脚本（ADR-0002 批次 C/D 演示动线）
 *
 * 动线：登录 → 病案编目 → 选病案 → 质控工作台 → CC/MCC 严重度预览
 *       → 填报其他诊断 → 保存并重新判读 → 跑质控 → 回看质控项（低靠/高靠 WARN）
 *
 * 依赖：playwright-core（装在 ~/.workbuddy/binaries/node/workspace，不下载浏览器内核，
 *      直接驱动本机已安装的 Chrome）。
 *
 * 运行（仓库根目录）：
 *   NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules" \
 *     node tools/shot-ccmcc.js
 *   SHOT_VIDEO=1 ... node tools/shot-ccmcc.js    # 需先装 ffmpeg（playwright-core cli install ffmpeg）
 *
 * 前置：后端 :8080、前端 :8090 已启动。
 */
const { chromium } = require('playwright-core');
const fs = require('fs');
const path = require('path');

const OUT = path.join(__dirname, '..', 'docs', 'screenshots');
const BASE = 'http://127.0.0.1:8090/frontend/index.html';
const API = 'http://127.0.0.1:8080/api';
const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

(async () => {
  fs.mkdirSync(OUT, { recursive: true });

  // 0) 先拿 token（Node 侧要用它直接调 QC 接口）
  const loginRes = await fetch(API + '/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: 'admin123' }),
  });
  const loginJson = await loginRes.json();
  const token = loginJson && loginJson.data && loginJson.data.token;
  if (!token) throw new Error('登录失败：' + JSON.stringify(loginJson));
  console.log('TOKEN_OK len=' + token.length);

  const browser = await chromium.launch({
    executablePath: CHROME,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });

  const wantVideo = process.env.SHOT_VIDEO === '1';
  const ctxOpts = wantVideo
    ? { viewport: { width: 1280, height: 800 }, recordVideo: { dir: path.join(OUT, 'video'), size: { width: 1280, height: 800 } } }
    : { viewport: { width: 1680, height: 1050 } };
  const ctx = await browser.newContext(ctxOpts);
  const page = await ctx.newPage();

  const errs = [];
  const notFound = [];
  page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
  page.on('console', (m) => { if (m.type() === 'error') errs.push('console: ' + m.text()); });
  page.on('response', (r) => { if (r.status() === 404) notFound.push(r.url()); });

  // 从网络请求里抓当前病案的 recordId（前端内部状态不暴露，抓请求最稳）。
  // 路径形态是 /front-pages/<id>/qc-workbench（api-store.js:1011），**不是** query 串，
  // 早先写成 qc-workbench?recordId= 永远匹配不上 → 第 8 步直接 NO_RID_CAPTURED 降级。
  let rid = null;
  const ridPatterns = [
    /\/front-pages\/(\d+)\/qc-workbench/, // 主来源：进质控工作台必发
    /\/front-pages\/(\d+)(?:[/?#]|$)/,    // 兜底：读首页 / 保存 / 跑质控都算
  ];
  page.on('request', (r) => {
    if (rid) return;
    const u = r.url();
    for (const re of ridPatterns) {
      const m = u.match(re);
      if (m) { rid = m[1]; return; }
    }
  });

  // 录屏一律用 1280×800（playwright 视频尺寸上限），故录屏那一轮的静态图另存子目录，
  // 不覆盖 1680×1050 的高清版截图。
  const stillDir = wantVideo ? path.join(OUT, 'video-stills') : OUT;
  fs.mkdirSync(stillDir, { recursive: true });
  const shot = async (name) => {
    await page.screenshot({ path: path.join(stillDir, name + '.png') });
    console.log('SHOT ' + name);
  };

  // 1. 登录页
  await page.goto(BASE, { waitUntil: 'networkidle', timeout: 60000 });
  await sleep(800);
  await shot('01-login');

  // 2. 注入 token 登录
  await page.evaluate((t) => sessionStorage.setItem('mrms_token', t), token);
  await page.reload({ waitUntil: 'networkidle' });
  await sleep(1800);
  await shot('02-dashboard');

  // 3. 病案编目
  await page.click('text=病案编目');
  await sleep(2200);
  await shot('03-catalog');

  // 4. 选中队列第一份病案
  const items = page.locator('div[style*="cursor"]');
  const n = await items.count();
  console.log('QUEUE_ITEMS ' + n);
  await items.first().click();
  await sleep(1600);
  await shot('04-record-selected');

  // 5. 质控工作台（此时多半还没有质控结果）
  await page.click('text=质控工作台');
  await sleep(2600);
  await shot('05-qc-workbench');
  console.log('CAPTURED_RID ' + rid);

  // 6. CC/MCC 严重度预览（与工作台互斥，打开时会收起工作台）
  await page.click('text=CC/MCC 严重度预览');
  await page.waitForTimeout(2600);
  await shot('06-ccmcc-preview');
  // 判读结果是否真的渲染出来了 —— 「暂无判读结果」是最容易被截图放过的失败态，
  // 这里用可见文本做断言，红就是红，不许靠肉眼看图判绿。
  const ccmccRendered = await page.locator('text=本例总分层').count();
  const ccmccEmpty = await page.locator('text=暂无判读结果').count();
  console.log('CCMCC_RENDERED ' + ccmccRendered + ' EMPTY ' + ccmccEmpty);

  // 7. 填报其他诊断 → 保存并重新判读（低靠形态：I50.900 标 0；同时 K80.201 标 C 触发高靠）
  try {
    /* 演示数据取值不是随手写的（2026-10-27 踩坑：flags 与 codes **按位一一对应**，
       填反了会静默"全部命中 but 全部不提示"，症状与"功能坏了"一模一样）：
         codes = I50.900,K80.201  flags = 0,C
         → I50.900 判读 MCC 却标 0                 ⇒ CC_MCC_LOW  （疑似漏编）
         → K80.201 演示表无条目却标 C              ⇒ CC_MCC_HIGH （疑似套高）
       早先错填成 flags="C,0"，两条都因"判读与标记一致"被正确沉默，
       反让我误判成"规则没落库"——真实原因恰恰是规则工作得太好。 */
    await page.waitForSelector('input[placeholder="I50.900,K80.201"]', { timeout: 15000 });
    await page.fill('input[placeholder="I50.900,K80.201"]', 'I50.900,K80.201');

    /* 两轮保存是刻意的负控制：第一轮落**干扰值** M,M，第二轮才落目标值 0,C。
       若"保存后刷新快照"没修好，第二轮之后编辑框会回填打开工作台时的旧快照（≠0,C），
       断言立刻变红；只有真刷新了才显示 0,C。只跑一轮时库值与快照恰好相同，
       该缺陷会被完全掩盖 —— 2026-10-27 就是这么漏过去一次的。 */
    await page.fill('input[placeholder="M,0"]', 'M,M');
    await page.waitForTimeout(200);
    await page.click('text=保存并重新判读');
    await page.waitForTimeout(2600);
    console.log('ROUND1_FLAGS ' + (await page.inputValue('input[placeholder="M,0"]')));

    await page.fill('input[placeholder="M,0"]', '0,C');
    await page.waitForTimeout(200);
    await page.click('text=保存并重新判读');
    await page.waitForTimeout(3400);
    const flagsShown = await page.inputValue('input[placeholder="M,0"]');
    console.log('ROUND2_FLAGS ' + flagsShown + ' | SNAPSHOT_REFRESHED=' + (flagsShown === '0,C'));
    await shot('07-ccmcc-after-save');
    const again = await page.locator('text=本例总分层').count();
    console.log('CCMCC_AFTER_SAVE_RENDERED ' + again);
  } catch (e) {
    console.log('SAVE_STEP_FAILED ' + e.message.split('\n')[0]);
  }

  // 8. 直连接口跑一次质控（让低靠/高靠 WARN 真正落库），再回看质控项
  if (!rid) {
    // 兜底：直接问接口要一份已有首页的病案 ID，避免第 8 步整体被跳过。
    try {
      const lp = await fetch(API + '/front-pages/page?page=1&size=100', {
        headers: { Authorization: 'Bearer ' + token },
      });
      const lj = await lp.json().catch(() => ({}));
      const list = (lj && lj.data && (lj.data.list || lj.data.items)) || [];
      rid = list.length ? list[0].recordId : null;
      console.log('RID_FALLBACK ' + rid);
    } catch (e) {
      console.log('RID_FALLBACK_FAILED ' + e.message);
    }
  }
  if (rid) {
    const qcRes = await fetch(`${API}/front-pages/${rid}/qc`, {
      method: 'POST',
      headers: { Authorization: 'Bearer ' + token },
    });
    const qcJson = await qcRes.json().catch(() => ({}));
    const results = (qcJson.data && (qcJson.data.qcResults || qcJson.data.results)) || [];
    console.log('QC_STATUS ' + qcRes.status + ' items=' + results.length);
    /* 命中统计按**文案特征**匹配，不认 r.ruleType —— 该字段在响应里根本不存在
       （只有 ruleId/fieldName/errorLevel/errorMsg）。早先按 ruleType 过滤，低靠那条
       因为文案里没有 "CC/MCC" 字样被漏数，日志显示 1 条，实际库里是 2 条。 */
    const hit = results.filter((r) => /演示判定表/.test(r.errorMsg || ''));
    console.log('QC_CCMCC_HITS ' + hit.length);
    hit.forEach((h) => console.log('  ' + h.errorLevel + ' [' + h.fieldName + '] ' + (h.errorMsg || '').slice(0, 70)));

    /* 关弹窗：common.js 的 mr-modal 是 <div class="mask" @click.self="$emit('close')"> 包
       一个 dialog，同名层级下后渲染的 mask 会盖住前一个 modal 的按钮。直接点按钮会被
       mask 拦下（locator 报 intercepts pointer events），故统一从最外层 mask 的空白处
       逐层点掉（点 mask 自身才是产品自带的关闭途径）。 */
    for (let i = 0; i < 4; i++) {
      const masks = page.locator('.mask');
      const cnt = await masks.count();
      if (!cnt) break;
      console.log('CLOSE_MASK_LAYER ' + i + ' remain=' + cnt);
      await masks.last().click({ position: { x: 6, y: 6 }, timeout: 8000 }).catch((e) => {
        console.log('MASK_CLICK_FAILED ' + e.message.split('\n')[0]);
      });
      await page.waitForTimeout(500);
    }
    await page.locator('button:has-text("质控工作台")').first().click({ timeout: 15000 });
    await page.waitForTimeout(3000);
    await shot('08-qc-with-ccmcc');
    const warnText = await page.locator('text=CC/MCC').count();
    console.log('QC_PAGE_CCMCC_TEXT_NODES ' + warnText);
  } else {
    console.log('NO_RID_CAPTURED');
  }

  console.log('PAGE_ERRORS ' + JSON.stringify(errs.slice(0, 12), null, 1));
  console.log('HTTP_404 ' + JSON.stringify([...new Set(notFound)].slice(0, 10), null, 1));
  try { await ctx.close(); } catch (e) { console.log('CTX_CLOSE ' + e.message); }
  await browser.close();
  console.log('DONE');
})().catch((e) => {
  console.error('FATAL ' + e.message);
  process.exit(1);
});
