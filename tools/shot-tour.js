/**
 * 全系统总览演示（master reel）—— 一条视频走完 8 个菜单页
 *
 * 与分段演示的关系：
 *   - 分段片（tools/shot-archive-borrow.js 等）= 单域**深度**操作闭环
 *   - 本片 = 全局**广度**巡览，每个菜单页停留展示核心内容，并打开该域的关键弹窗
 *   两类互补：观众先看总览知道系统长什么样，再看分段片学具体怎么操作。
 *
 * 为什么自己录而不把分段片拼起来：Playwright 自带的 ffmpeg 是**精简版**——
 *   有 libvpx-vp9 编码器 + webm 复用器 + png 编码器 + image2 复用器 + scale/fps 滤镜，
 *   但**没有 concat 滤镜、没有 concat demuxer、没有 lavfi、没有 png 解码器**。
 *   拼接 webm 的常规路径（concat filter / concat demuxer）在这里都不存在，
 *   硬拼只能走「抽帧 → 重新编码」把画质打回损失一遍 —— 不如直接录一条干净的。
 *
 * 运行（仓库根目录）：
 *   NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules" \
 *     "C:/Users/12743/.workbuddy/binaries/node/versions/22.22.2-6/node.exe" tools/shot-tour.js
 *   SHOT_VIDEO=1 ... # 录制视频
 */
const { createShooter } = require('./shot-lib');

const OUT = 'docs/screenshots/tour';

/** 总览片的 12 步动线（步骤条文案 = 操作顺序） */
const STEPS = [
  '打开系统并登录',
  '工作台总览：指标卡与业务动态',
  '病案归档：状态分布与批量操作',
  '归档详情：病案主档一览',
  '病案编目：待编目队列与选码',
  '质控工作台：评分与等级',
  '病案借阅：申请与审批',
  '数据统计：疾病谱与科室排名',
  '首页数据上报：批次与一致性核对',
  '质量看板：归档时效与超期催缴',
  '我的待整改：质控整改闭环',
  'CC/MCC 严重度预警：低靠与高靠',
];

(async () => {
  const S = await createShooter({
    outDir: OUT,
    stepPlan: STEPS,
    video: process.env.SHOT_VIDEO === '1',
    recordName: 'demo',
  });

  await S.open({ loginShot: '01-login' });

  // 1. 工作台总览
  await S.step(2);
  await S.sleep(2200);
  await S.shot('02-dashboard');

  /* 逐域巡览。每个域的动作是「点菜单 → 等数据加载 → 截图 → 打开关键弹窗 → 关掉」。
     找不到元素时不抛错、只记日志 —— 巡览片的职责是把系统全貌拍出来，
     某一个域的数据为空不该让整条片子作废（具体断言由分段片负责）。 */
  const visit = async (menuText, shotName, opts = {}) => {
    try {
      const link = S.page.locator(`text=${menuText}`).first();
      if (!(await link.count())) { console.log('MENU_MISS ' + menuText); return false; }
      await S.click(link);
      await S.sleep(opts.wait || 2600);
      await S.shot(shotName);
      return true;
    } catch (e) {
      console.log('VISIT_FAILED ' + menuText + ' ' + e.message.split('\n')[0]);
      return false;
    }
  };

  /** 在当前页面上点一个文本按钮，失败只记日志 */
  const tryClick = async (text, wait = 2200, shotName = null) => {
    try {
      const loc = S.page.locator(`text=${text}`).first();
      if (!(await loc.count())) { console.log('BTN_MISS ' + text); return false; }
      await S.click(loc);
      await S.sleep(wait);
      if (shotName) await S.shot(shotName);
      return true;
    } catch (e) {
      console.log('BTN_CLICK_FAILED ' + text + ' ' + e.message.split('\n')[0]);
      return false;
    }
  };

  // 2. 病案归档
  await S.step(3);
  await visit('病案归档', '03-archive');

  // 3. 归档详情
  await S.step(4);
  await tryClick('详情', 2000, '04-archive-detail');
  await S.closeModals();

  // 4. 病案编目
  await S.step(5);
  await visit('病案编目', '05-catalog');

  // 5. 质控工作台
  await S.step(6);
  await tryClick('质控工作台', 2800, '06-qc-workbench');
  await S.closeModals();

  // 6. 病案借阅
  await S.step(7);
  await visit('病案借阅', '07-borrow');
  await tryClick('+ 借阅申请', 1800, '08-borrow-apply');
  await S.closeModals();

  // 7. 数据统计
  await S.step(8);
  await visit('数据统计', '09-statistics', { wait: 3200 });

  // 8. 首页数据上报
  await S.step(9);
  await visit('首页数据上报', '10-report', { wait: 3000 });

  // 9. 质量看板
  await S.step(10);
  await visit('质量看板', '11-quality', { wait: 3200 });

  // 10. 我的待整改
  await S.step(11);
  await visit('我的待整改', '12-rectify', { wait: 3000 });

  // 11. CC/MCC 严重度预警（回到编目页演示）
  await S.step(12);
  await visit('病案编目', '13-catalog-again', { wait: 2400 });
  try {
    // 选一份病案，才能打开质控工作台与 CC/MCC 预览
    const items = S.page.locator('div[style*="cursor"]');
    const n = await items.count();
    console.log('CATALOG_QUEUE_ITEMS ' + n);
    if (n) {
      await S.click(items.first());
      await S.sleep(1800);
      await tryClick('质控工作台', 2800);
      await tryClick('CC/MCC 严重度预览', 3000, '14-ccmcc-preview');
    }
  } catch (e) {
    console.log('CCMCC_STEP_FAILED ' + e.message.split('\n')[0]);
  }

  // 收尾停留
  await S.sleep(2500);
  await S.finish();
})().catch((e) => {
  console.error('FATAL ' + e.message);
  process.exit(1);
});
