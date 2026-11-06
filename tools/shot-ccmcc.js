/**
 * 端到端截图 + 录屏脚本（ADR-0002 批次 C/D 演示动线）
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
 *   SHOT_VIDEO=1 ... node tools/shot-ccmcc.js    # 录制视频（同时把静帧另存 video-stills/）
 *
 * 前置：后端 :8080、前端 :8090 已启动（PG 在 5432）。
 *
 * ── 人类化增强（2026-11-06）────────────────────────────────────────────
 * 起因：录屏里看不出「点了哪、点的顺序」，操作顺序完全靠观众猜。
 * 四个关键事实决定了实现方式：
 *   1. **Playwright 录屏不录 OS 指针** —— headless 下没有系统光标，录下来只有
 *      会动的页面。所以光标必须**注入到页面里**：一个跟随 mousemove 的 div。
 *   2. **`mouse.move(x,y,{steps:n})` 走完只要几毫秒** —— 25fps 的录屏只会拍到
 *      「瞬移」，轨迹等于没有。所以要**自己分步插值 + 每步延时**（下面 glide）。
 *   3. **Vue 的 @click 认 mouse.down/up** —— 可以用真实鼠标事件驱动，顺带触发涟漪。
 *   4. **fill() 是瞬时赋值** —— 视频里字符凭空出现，不像人在打字。改用
 *      pressSequentially 逐字输入。
 */
const { chromium } = require('playwright-core');
const fs = require('fs');
const path = require('path');

const OUT = path.join(__dirname, '..', 'docs', 'screenshots');
const BASE = 'http://127.0.0.1:8090/frontend/index.html';
const API = 'http://127.0.0.1:8080/api';
const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';
const VIDEO = process.env.SHOT_VIDEO === '1';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
/** 录屏轮整体放慢 1.5 倍：静态图那轮不需要慢节奏 */
const slow = (ms) => sleep(VIDEO ? Math.round(ms * 1.5) : ms);

/** 固定的 9 步动线（提示条从第一步起就有完整进度点，不做「越走越长」的动态列表） */
const STEP_PLAN = [
  '打开系统并登录',
  '进入「病案编目」',
  '选中一份演示病案',
  '打开「质控工作台」',
  '打开「CC/MCC 严重度预览」',
  '填报其他诊断标记',
  '保存并重新判读',
  '运行质控（两条 WARN 落库）',
  '回看质控项：低靠 / 高靠',
];

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

  const ctxOpts = VIDEO
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
  const stillDir = VIDEO ? path.join(OUT, 'video-stills') : OUT;
  fs.mkdirSync(stillDir, { recursive: true });
  const shot = async (name) => {
    await page.screenshot({ path: path.join(stillDir, name + '.png') });
    console.log('SHOT ' + name);
  };

  // ── 人类化层 ────────────────────────────────────────────────────────
  let lastPos = { x: 60, y: 60 };
  let curStep = 0;

  /**
   * 用 **context.addInitScript** 注入标注层，而不是 goto 之后再 addStyleTag。
   * 差别很致命：后者会让录屏开头有约 2 秒「裸画面」—— 登录页已经显示、标注还没挂上，
   * 观众第一眼看到的是没有步骤条的登录页（2026-11-06 首帧抽检才发现）。
   * addInitScript 在每个新文档解析早期执行 ⇒ 首帧即带步骤条。
   */
  await ctx.addInitScript((plan) => {
    const CSS = `
        #vb-cursor{position:fixed;left:0;top:0;width:15px;height:15px;margin:-7px 0 0 -7px;border-radius:50%;
          background:rgba(255,92,26,.95);border:2px solid #fff;
          box-shadow:0 0 0 4px rgba(255,92,26,.20),0 3px 10px rgba(0,0,0,.45);
          pointer-events:none;z-index:2147483647;opacity:0;}
        #vb-steps{position:fixed;left:50%;top:14px;transform:translateX(-50%);display:flex;align-items:center;gap:12px;
          padding:9px 18px;border-radius:999px;background:rgba(22,24,30,.90);color:#fff;
          font:600 13px/1 "Microsoft YaHei",-apple-system,sans-serif;letter-spacing:.3px;
          box-shadow:0 6px 22px rgba(0,0,0,.38);pointer-events:none;z-index:2147483647;white-space:nowrap;}
        #vb-steps .vb-dots{display:flex;gap:6px;}
        #vb-steps .vb-dot{width:9px;height:9px;border-radius:50%;background:rgba(255,255,255,.26);
          transition:background .25s,transform .25s;}
        #vb-steps .vb-dot.on{background:#ff5c1a;transform:scale(1.3);}
        #vb-steps .vb-txt b{color:#ff7a3d;font-weight:700;}
        @keyframes vbrip{from{transform:scale(.35);opacity:.95;}to{transform:scale(3.6);opacity:0;}}
        .vb-ripple{position:fixed;border-radius:50%;pointer-events:none;z-index:2147483646;
          animation:vbrip .65s ease-out forwards;}`;
    const inject = () => {
      if (!document.body || document.getElementById('vb-cursor')) return;
      const st = document.createElement('style');
      st.textContent = CSS;
      (document.head || document.documentElement).appendChild(st);
      const cur = document.createElement('div');
      cur.id = 'vb-cursor';
      const bar = document.createElement('div');
      bar.id = 'vb-steps';
      document.body.appendChild(cur);
      document.body.appendChild(bar);
      /* 初始内容就地写好 —— 步骤文案若等 Node 侧 renderStep 再填，首帧仍是空壳
         （录屏第 0.5 秒抽检发现：只有一个小色块，没有文字）。 */
      const dots = plan.map((_, i) => `<span class="vb-dot${i < 1 ? ' on' : ''}"></span>`).join('');
      bar.innerHTML = `<div class="vb-dots">${dots}</div><div class="vb-txt"><b>步骤 1/${plan.length}</b> · ${plan[0]}</div>`;
      document.addEventListener('mousemove', (e) => {
        cur.style.left = e.clientX + 'px';
        cur.style.top = e.clientY + 'px';
        cur.style.opacity = '1';
      });
      document.addEventListener('mousedown', (e) => {
        const r = document.createElement('div');
        r.className = 'vb-ripple';
        r.style.left = e.clientX - 6 + 'px';
        r.style.top = e.clientY - 6 + 'px';
        r.style.width = '12px';
        r.style.height = '12px';
        r.style.border = '3px solid rgba(255,92,26,.95)';
        document.body.appendChild(r);
        setTimeout(() => r.remove(), 700);
      });
    };
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', inject, { once: true });
    } else {
      inject();
    }
  }, STEP_PLAN);

  /** 兜底：万一某个文档没跑到 initScript，现场补一次 */
  const ensureOverlay = async () => {
    const has = await page.evaluate(() => !!document.getElementById('vb-cursor')).catch(() => false);
    if (!has) {
      await page.addStyleTag({ content: '#vb-cursor,#vb-steps{position:fixed;z-index:2147483647}' }).catch(() => {});
      await page.evaluate((plan) => {
        if (document.getElementById('vb-cursor')) return;
        const cur = document.createElement('div');
        cur.id = 'vb-cursor';
        const bar = document.createElement('div');
        bar.id = 'vb-steps';
        document.body.appendChild(cur);
        document.body.appendChild(bar);
        const dots = plan.map((_, i) => `<span class="vb-dot${i < 1 ? ' on' : ''}"></span>`).join('');
        bar.innerHTML = `<div class="vb-dots">${dots}</div><div class="vb-txt"><b>步骤 1/${plan.length}</b> · ${plan[0]}</div>`;
      }, STEP_PLAN).catch(() => {});
    }
  };

  /** 刷新步骤条（第 n 步 / 共 9 步 + 进度点） */
  const renderStep = async (n) => {
    curStep = n;
    await page.evaluate(({ n, plan }) => {
      const bar = document.getElementById('vb-steps');
      if (!bar) return;
      const dots = plan.map((_, i) => `<span class="vb-dot${i < n ? ' on' : ''}"></span>`).join('');
      bar.innerHTML = `<div class="vb-dots">${dots}</div><div class="vb-txt"><b>步骤 ${n}/${plan.length}</b> · ${plan[n - 1]}</div>`;
    }, { n, plan: STEP_PLAN }).catch(() => {});
  };

  /**
   * 带延时的鼠标插值移动 —— 轨迹必须「慢」到能被 25fps 录到，
   * 否则 steps 参数走完只要几毫秒，画面上只剩瞬移。
   * 中途加一点弧度，避免像机器人走直线。
   */
  const glide = async (x, y) => {
    const from = { ...lastPos };
    const steps = 26;
    const bow = Math.min(70, Math.abs(y - from.y) * 0.12 + 26) * (y < from.y ? -1 : 1);
    for (let i = 1; i <= steps; i++) {
      const t = i / steps;
      const e = 1 - Math.pow(1 - t, 2.2); // easeOut：起步快、收尾慢
      const px = from.x + (x - from.x) * e;
      const py = from.y + (y - from.y) * e + Math.sin(e * Math.PI) * bow;
      await page.mouse.move(px, py);
      await sleep(16);
    }
    lastPos = { x, y };
  };

  /** 拟人点击：移动 → 停顿 → 高亮 → down/up（涟漪由页面监听 mousedown 触发） */
  const humanClick = async (locator, opts = {}) => {
    const box = await locator.boundingBox().catch(() => null);
    if (!box) {
      await locator.click(opts).catch((e) => console.log('CLICK_FALLBACK_FAILED ' + e.message.split('\n')[0]));
      return;
    }
    const x = box.x + box.width / 2 + (opts.dx || 0);
    const y = box.y + box.height / 2 + (opts.dy || 0);
    await glide(x, y);
    await slow(240);
    const handle = await locator.elementHandle().catch(() => null);
    if (handle) {
      await page.evaluate((el) => {
        el.style.outline = '3px solid rgba(255,92,26,.95)';
        el.style.outlineOffset = '2px';
        setTimeout(() => { el.style.outline = ''; el.style.outlineOffset = ''; }, 900);
      }, handle).catch(() => {});
    }
    await page.mouse.move(x, y);
    await page.mouse.down();
    await slow(85);
    await page.mouse.up();
    await slow(110);
  };

  /** 逐字输入（fill 是瞬时赋值，视频里像凭空出现） */
  const humanType = async (locator, text) => {
    await humanClick(locator);
    await locator.fill('').catch(() => {});
    await slow(140);
    if (typeof locator.pressSequentially === 'function') {
      await locator.pressSequentially(text, { delay: 55 }).catch(() => locator.type(text, { delay: 55 }));
    } else {
      await locator.type(text, { delay: 55 });
    }
    await slow(160);
  };

  // 1. 登录页（标注层由 addInitScript 挂载 ⇒ 首帧即带步骤条）
  await page.goto(BASE, { waitUntil: 'networkidle', timeout: 60000 });
  await sleep(900);
  await ensureOverlay();
  await renderStep(1);
  await shot('01-login');

  // 2. 注入 token 登录
  await page.evaluate((t) => sessionStorage.setItem('mrms_token', t), token);
  await page.reload({ waitUntil: 'networkidle' });
  await sleep(1900);
  await ensureOverlay();
  await renderStep(1);
  await shot('02-dashboard');

  // 3. 病案编目
  await renderStep(2);
  await humanClick(page.locator('text=病案编目').first());
  await sleep(2300);
  await shot('03-catalog');

  // 4. 选中队列第一份病案
  await renderStep(3);
  const items = page.locator('div[style*="cursor"]');
  const n = await items.count();
  console.log('QUEUE_ITEMS ' + n);
  await humanClick(items.first());
  await sleep(1700);
  await shot('04-record-selected');

  // 5. 质控工作台（此时多半还没有质控结果）
  await renderStep(4);
  await humanClick(page.locator('text=质控工作台').first());
  await sleep(2700);
  await shot('05-qc-workbench');
  console.log('CAPTURED_RID ' + rid);

  // 6. CC/MCC 严重度预览（与工作台互斥，打开时会收起工作台）
  await renderStep(5);
  await humanClick(page.locator('text=CC/MCC 严重度预览').first());
  await page.waitForTimeout(2700);
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
    await renderStep(6);
    await humanType(page.locator('input[placeholder="I50.900,K80.201"]'), 'I50.900,K80.201');

    /* 两轮保存是刻意的负控制：第一轮落**干扰值** M,M，第二轮才落目标值 0,C。
       若"保存后刷新快照"没修好，第二轮之后编辑框会回填打开工作台时的旧快照（≠0,C），
       断言立刻变红；只有真刷新了才显示 0,C。只跑一轮时库值与快照恰好相同，
       该缺陷会被完全掩盖 —— 2026-10-27 就是这么漏过去一次的。 */
    await humanType(page.locator('input[placeholder="M,0"]'), 'M,M');
    await renderStep(7);
    await humanClick(page.locator('text=保存并重新判读').first());
    await page.waitForTimeout(2700);
    console.log('ROUND1_FLAGS ' + (await page.inputValue('input[placeholder="M,0"]')));

    await humanType(page.locator('input[placeholder="M,0"]'), '0,C');
    await humanClick(page.locator('text=保存并重新判读').first());
    await page.waitForTimeout(3500);
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
    await renderStep(8);
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
      const m = masks.last();
      const bx = await m.boundingBox().catch(() => null);
      // mask 是全屏的，点它自身的左上角（.self 才会触发 close）
      if (bx) {
        await glide(8, 8);
        await slow(200);
        await page.mouse.move(8, 8);
        await page.mouse.down();
        await slow(80);
        await page.mouse.up();
      } else {
        await m.click({ position: { x: 6, y: 6 }, timeout: 8000 }).catch((e) => {
          console.log('MASK_CLICK_FAILED ' + e.message.split('\n')[0]);
        });
      }
      await page.waitForTimeout(600);
    }
    await renderStep(9);
    await humanClick(page.locator('button:has-text("质控工作台")').first());
    await page.waitForTimeout(3100);
    await shot('08-qc-with-ccmcc');
    const warnText = await page.locator('text=CC/MCC').count();
    console.log('QC_PAGE_CCMCC_TEXT_NODES ' + warnText);
    // 尾帧停留：让观众看清结果，不用倒带
    await sleep(4000);
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
