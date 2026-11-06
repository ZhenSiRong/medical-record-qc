/** 冒烟测试：验证 shot-lib.js 能开页、注入标注、截图、收尾 */
const { createShooter } = require('./shot-lib');

(async () => {
  const S = await createShooter({
    outDir: 'docs/screenshots/.smoke',
    stepPlan: ['冒烟 A', '冒烟 B', '冒烟 C'],
  });
  await S.open({ loginShot: 'smoke-login' });
  await S.step(1);
  await S.shot('smoke-dashboard');
  await S.step(2);
  await S.click(S.page.locator('text=病案归档').first());
  await S.sleep(2000);
  await S.shot('smoke-archive');
  await S.step(3);
  const rows = await S.page.locator('table tbody tr').count();
  console.log('ARCHIVE_ROWS ' + rows);
  await S.finish();
})().catch((e) => { console.error('FATAL ' + e.message); process.exit(1); });
