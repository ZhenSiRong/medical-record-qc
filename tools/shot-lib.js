/**
 * 端到端录屏 / 截图公共库 —— 「人类化操作层」
 *
 * 为什么要有这个库（2026-11-06 建）：
 *   录屏若只拍页面，���众看不出「点了哪、点的顺序」。要做到「像人在用」，
 *   必须补齐四件东西，缺一件视频就退化成幻灯片：
 *     1. **虚拟光标** —— Playwright 录屏**不录 OS 指针**（headless 下也没有系统光标），
 *        所以光标必须注入页面里：一个跟随真实 mousemove 的 div。
 *     2. **慢速鼠标轨迹** —— `mouse.move(x,y,{steps:n})` 走完只要几毫秒，
 *        25fps 的录屏只会拍到「瞬移」，轨迹等于没有 ⇒ 必须自己分步插值 + 每步延时。
 *     3. **点击涟漪 + 被点元素高亮** —— 让「这一下点在哪儿」可见。
 *     4. **顶部步骤条** —— 「步骤 N/M · 当前动作」，操作顺序一目了然。
 *
 *   另有两条踩过的坑写在这里：
 *     - 标注层必须用 **context.addInitScript** 注入，而不是 goto 之后 addStyleTag：
 *       后者会让录屏开头有约 2 秒「页面已显示、标注还没挂上」的裸画面，
 *       观众第一眼看到的是没有步骤条的登录页（首帧抽检才发现）。
 *     - `fill()` 是瞬时赋值，视频里字符凭空出现，不像人在打字 ⇒ 用 pressSequentially。
 *
 * 用法：
 *   const S = await createShooter({ outDir: 'docs/screenshots/feat-archive', stepPlan: [...] });
 *   await S.step(1);
 *   await S.click(S.page.locator('text=病案归档').first());
 *   await S.shot('01-archive');
 *   await S.finish();
 *
 * 依赖：playwright-core（装在 ~/.workbuddy/binaries/node/workspace，不下载浏览器内核，
 *       直接驱动本机已安装的 Chrome）。
 * 运行（仓库根目录）：
 *   NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules" \
 *     node tools/shot-<name>.js
 *   SHOT_VIDEO=1 ... node tools/shot-<name>.js    # 录制视频
 *
 * 前置：后端 :8080、前端 :8090 已启动（PG 在 5432）。
 */
const { chromium } = require('playwright-core');
const fs = require('fs');
const path = require('path');

const REPO = path.join(__dirname, '..');
const BASE = process.env.SHOT_BASE || 'http://127.0.0.1:8090/frontend/index.html';
const API = process.env.SHOT_API || 'http://127.0.0.1:8080/api';
const CHROME = process.env.SHOT_CHROME || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** 标注层样式：虚拟光标 + 步骤条 + 点击涟漪 */
const OVERLAY_CSS = `
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

/**
 * 创建一个「会做人」的截图/录屏器。
 * @param {object} opts
 * @param {string} opts.outDir        产物目录（相对仓库根）
 * @param {string[]} opts.stepPlan     步骤条文案，顺序即操作顺序
 * @param {boolean} opts.video        是否录屏（默认读环境变量 SHOT_VIDEO=1）
 * @param {string} opts.username      登录账号（默认 admin）
 * @param {string} opts.password      登录口令（默认 admin123）
 * @param {string} opts.token         预置 token（给了就跳过登录接口）
 * @param {number} opts.slowFactor     节奏放慢倍数（录屏默认 1.5）
 */
async function createShooter(opts = {}) {
  const {
    outDir = 'docs/screenshots',
    stepPlan = [],
    video = process.env.SHOT_VIDEO === '1',
    viewport = video ? { width: 1280, height: 800 } : { width: 1680, height: 1050 },
    username = 'admin',
    password = 'admin123',
    presetToken = null,
    slowFactor = video ? 1.5 : 1,
    recordName = 'demo',
  } = opts;

  const absOut = path.isAbsolute(outDir) ? outDir : path.join(REPO, outDir);
  fs.mkdirSync(absOut, { recursive: true });

  const slow = (ms) => sleep(Math.round(ms * slowFactor));

  // ── 登录拿 token（Node 侧也要用它直接调接口）─────────────────────────
  let token = presetToken;
  if (!token) {
    const loginRes = await fetch(API + '/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    const loginJson = await loginRes.json().catch(() => ({}));
    token = loginJson && loginJson.data && loginJson.data.token;
    if (!token) throw new Error('登录失败：' + JSON.stringify(loginJson).slice(0, 200));
  }
  console.log('TOKEN_OK len=' + String(token).length + ' user=' + username);

  const browser = await chromium.launch({
    executablePath: CHROME,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });

  const videoDir = video ? path.join(absOut, '.video') : null;
  const ctxOpts = {
    viewport,
    ...(video ? { recordVideo: { dir: videoDir, size: { width: 1280, height: 800 } } } : {}),
  };
  const ctx = await browser.newContext(ctxOpts);
  const page = await ctx.newPage();

  const errs = [];
  const notFound = [];
  page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
  page.on('console', (m) => { if (m.type() === 'error') errs.push('console: ' + m.text()); });
  page.on('response', (r) => { if (r.status() === 404) notFound.push(r.url()); });

  // 记录 video 对象以便 finish 时取路径
  let videoPath = null;

  // ── 标注层：addInitScript（首帧即带标注）────────────────────────────
  await ctx.addInitScript((plan) => {
    const inject = () => {
      if (!document.body || document.getElementById('vb-cursor')) return;
      const st = document.createElement('style');
      st.textContent = window.__VB_CSS__;
      (document.head || document.documentElement).appendChild(st);
      const cur = document.createElement('div');
      cur.id = 'vb-cursor';
      const bar = document.createElement('div');
      bar.id = 'vb-steps';
      document.body.appendChild(cur);
      document.body.appendChild(bar);
      /* 初始内容就地写好 —— 若等 Node 侧 renderStep 再填，首帧仍是空壳。 */
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
  }, stepPlan);
  // 样式串通过 init script 的闭包拿不到，改走 evaluate 前的全局注入
  await ctx.addInitScript((css) => { window.__VB_CSS__ = css; }, OVERLAY_CSS);

  /** 兜底：万一某个文档没跑到 initScript，现场补一次 */
  const ensureOverlay = async () => {
    const has = await page.evaluate(() => !!document.getElementById('vb-cursor')).catch(() => false);
    if (has) return;
    await page.addStyleTag({ content: OVERLAY_CSS }).catch(() => {});
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
    }, stepPlan).catch(() => {});
  };

  /** 刷新步骤条（第 n 步 / 共 M 步 + 进度点） */
  const step = async (n) => {
    await ensureOverlay();
    await page.evaluate(({ n, plan }) => {
      const bar = document.getElementById('vb-steps');
      if (!bar) return;
      const dots = plan.map((_, i) => `<span class="vb-dot${i < n ? ' on' : ''}></span>`).join('');
      bar.innerHTML = `<div class="vb-dots">${dots}</div><div class="vb-txt"><b>步骤 ${n}/${plan.length}</b> · ${plan[n - 1]}</div>`;
    }, { n, plan: stepPlan }).catch(() => {});
  };

  // ── 人类化鼠标 ─────────────────────────────────────────────────────
  let lastPos = { x: 60, y: 60 };

  /** 带延时的鼠标插值移动 —— 必须慢到 25fps 能拍到，否则只见瞬移 */
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
  const click = async (locator, o = {}) => {
    // ⚠️ **必须先滚动到可视区**：`boundingBox()` 对视口外的元素返回的是页面坐标，
    // `page.mouse.move(x, y)` 移到的是视口坐标 —— 两者坐标系不同，
    // 结果是「光标飘到别处、点击落空、按钮毫无反应」，症状与「按钮坏了」一模一样。
    // 这一条是录制脚本最容易踩的坑（2026-11-06 由 recorder-stats 发现）。
    if (o.scroll !== false) {
      await locator.scrollIntoViewIfNeeded({ timeout: 5000 }).catch(() => {});
    }
    const box = await locator.boundingBox().catch(() => null);
    if (!box) {
      await locator.click(o).catch((e) => console.log('CLICK_FALLBACK_FAILED ' + e.message.split('\n')[0]));
      return false;
    }
    // boundingBox 已经是相对视口的坐标，鼠标 move 用它即可
    const x = box.x + box.width / 2 + (o.dx || 0);
    const y = box.y + box.height / 2 + (o.dy || 0);
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
    return true;
  };

  /** 逐字输入（fill 是瞬时赋值，视频里像凭空出现） */
  const type = async (locator, text) => {
    await click(locator);
    await locator.fill('').catch(() => {});
    await slow(140);
    if (typeof locator.pressSequentially === 'function') {
      await locator.pressSequentially(text, { delay: 55 }).catch(() => locator.type(text, { delay: 55 }));
    } else {
      await locator.type(text, { delay: 55 });
    }
    await slow(160);
  };

  /** 关掉所有叠着的弹窗：mr-modal 的 mask 是全屏层，后渲染的会拦住前一个的按钮 */
  const closeModals = async (max = 4) => {
    for (let i = 0; i < max; i++) {
      const masks = page.locator('.mask');
      const cnt = await masks.count();
      if (!cnt) break;
      const m = masks.last();
      const bx = await m.boundingBox().catch(() => null);
      if (bx) {
        // mask 全屏，点它自身左上角（.self 才触发 close）
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
  };

  /** 截图到产物目录 */
  const shot = async (name) => {
    await page.screenshot({ path: path.join(absOut, name + '.png') });
    console.log('SHOT ' + name);
  };

  /** 打开前端并注入 token（跳过登录表单的输入过程，但保留登录页一帧） */
  const open = async (opts2 = {}) => {
    await page.goto(BASE, { waitUntil: 'networkidle', timeout: 60000 });
    await sleep(900);
    await ensureOverlay();
    if (opts2.shotLogin !== false) await shot(opts2.loginShot || '01-login');
    await page.evaluate((t) => sessionStorage.setItem('mrms_token', t), token);
    await page.reload({ waitUntil: 'networkidle' });
    await sleep(1900);
    await ensureOverlay();
  };

  /** 带鉴权的接口调用 */
  const api = async (path_, init = {}) => {
    const res = await fetch(API + path_, {
      ...init,
      headers: { Authorization: 'Bearer ' + token, ...(init.headers || {}) },
    });
    const json = await res.json().catch(() => ({}));
    return { status: res.status, json };
  };

  /** 收尾：打印错误、落视频、关浏览器 */
  const finish = async () => {
    console.log('PAGE_ERRORS ' + JSON.stringify(errs.slice(0, 12), null, 1));
    console.log('HTTP_404 ' + JSON.stringify([...new Set(notFound)].slice(0, 10), null, 1));
    if (video) {
      try {
        videoPath = await page.video().path();
      } catch (e) {
        console.log('VIDEO_PATH_FAILED ' + e.message.split('\n')[0]);
      }
    }
    try { await ctx.close(); } catch (e) { console.log('CTX_CLOSE ' + e.message.split('\n')[0]); }
    await browser.close();
    if (videoPath) {
      const dest = path.join(absOut, recordName + '.webm');
      try {
        fs.copyFileSync(videoPath, dest);
        const sz = (fs.statSync(dest).size / 1048576).toFixed(1);
        console.log('VIDEO ' + dest + ' ' + sz + 'MB');
      } catch (e) {
        console.log('VIDEO_COPY_FAILED ' + e.message.split('\n')[0]);
      }
      try { fs.rmSync(videoDir, { recursive: true, force: true }); } catch (e) { /* ignore */ }
    }
    console.log('DONE');
  };

  return {
    page, ctx, browser, token, api, open, step, click, type, glide, shot,
    closeModals, finish, sleep, slow, errs, notFound,
    outDir: absOut,
  };
}

module.exports = { createShooter, sleep, OVERLAY_CSS, BASE, API, CHROME };
