// 部署后目视核对：连真实容器，只读，不发保存请求。
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const OUT = new URL('../../.pi/after-deploy/', import.meta.url);
mkdirSync(OUT, { recursive: true });

test('截图海难与火山世界配置页', async () => {
  const browser = await chromium.launch();
  for (const [name, width] of [['desktop', 1440], ['mobile', 420]]) {
    const page = await browser.newPage({ viewport: { width, height: 1100 } });
    const blockedWrites = [];
    await page.route('**/api/**', route => {
      if (route.request().method() !== 'GET') {
        blockedWrites.push(route.request().url());
        return route.abort();
      }
      return route.continue();
    });
    await page.addInitScript(() => {
      localStorage.setItem('dst-tab', 'world');
      localStorage.setItem('dst-room', '2');
    });
    await page.goto('http://127.0.0.1:16800/');
    await page.locator('.world-options-card .seg-count').nth(1).waitFor({ timeout: 20000 });
    await page.waitForFunction(() => !document.querySelector('.world-category-skeleton'), null, { timeout: 20000 });

    assert.deepEqual(await page.locator('.shard-tab').allInnerTexts(), ['海难', '洞穴', '火山', '森林']);
    for (const [shard, label, settingsCount, worldgenCount] of [
      ['Master', '海难', 109, 64], ['Caves', '火山', 26, 9],
    ]) {
      await page.getByRole('button', { name: label, exact: true }).click();
      await page.waitForFunction(n => !document.querySelector('.world-category-skeleton')
        && document.querySelectorAll('.opt-row').length === n, settingsCount, { timeout: 20000 });

      const info = await page.evaluate(() => ({
        header: document.querySelector('[data-slot=card-description]')?.textContent?.trim(),
        segments: [...document.querySelectorAll('.seg-btn')].map(b => b.textContent.replace(/\s+/g, ' ').trim()),
        notice: document.querySelector('.notice')?.textContent?.replace(/\s+/g, ' ').trim().slice(0, 160),
        rows: document.querySelectorAll('.opt-row').length,
        changed: document.querySelector('.world-count')?.textContent?.trim(),
        images: [...document.querySelectorAll('.opt-row img')].map(i => i.getAttribute('src')).slice(0, 6),
        brokenImages: [...document.querySelectorAll('.opt-row img')]
          .filter(i => i.complete && i.naturalWidth === 0).map(i => i.getAttribute('src')),
      }));
      console.log(`[${name}] ${shard}:`, JSON.stringify(info));
      await page.screenshot({ path: new URL(`${name}-${shard}.png`, OUT).pathname.slice(1), fullPage: false });

      // 世界生成标签
      await page.locator('.seg-btn', { hasText: '世界生成' }).click();
      await page.waitForFunction(() => !document.querySelector('.world-category-skeleton'), null, { timeout: 20000 });
      await page.waitForFunction(n => document.querySelectorAll('.opt-row').length === n,
        worldgenCount, { timeout: 20000 });
      const worldgen = await page.evaluate(() => ({
        rows: document.querySelectorAll('.opt-row').length,
        disabled: [...document.querySelectorAll('.opt-row button, .opt-row input')]
          .every(el => el.disabled),
        notice: document.querySelector('.notice')?.textContent?.replace(/\s+/g, ' ').trim().slice(0, 120),
      }));
      assert.equal(worldgen.rows, worldgenCount);
      assert.equal(worldgen.disabled, true, '已有存档生成配置必须只读');
      console.log(`[${name}] ${shard} worldgen:`, JSON.stringify(worldgen));
      await page.screenshot({ path: new URL(`${name}-${shard}-worldgen.png`, OUT).pathname.slice(1), fullPage: false });
      await page.locator('.seg-btn', { hasText: '世界规则' }).click();
      await page.waitForFunction(() => !document.querySelector('.world-category-skeleton'), null, { timeout: 20000 });
    }
    assert.deepEqual(blockedWrites, [], '部署验收只能读接口');
    await page.close();
  }
  await browser.close();
});
