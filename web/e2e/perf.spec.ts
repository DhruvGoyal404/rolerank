import { expect, test } from "@playwright/test";

/**
 * Largest Contentful Paint under 2.5 s on throttled 4G (ROADMAP.md, P6). Measured with the
 * throttling Lighthouse uses for slow 4G (150 ms round trip, 1.6 Mbps down, 750 kbps up, CPU
 * slowed 4x) through Chrome's DevTools protocol, so no Lighthouse dependency is needed; PHASE6.md
 * records that as a deviation. One warm-up load first (a cold preview server isn't what users
 * meet), then three loads with the cache off; the median counts.
 */
for (const path of ["/", "/signin"]) {
  test(`LCP of ${path} is under 2.5 s on slow 4G`, async ({ page, context }) => {
    test.setTimeout(120_000);
    await page.goto(path);
    const cdp = await context.newCDPSession(page);
    await cdp.send("Network.enable");
    await cdp.send("Network.setCacheDisabled", { cacheDisabled: true });
    await cdp.send("Network.emulateNetworkConditions", {
      offline: false,
      latency: 150,
      downloadThroughput: (1.6 * 1024 * 1024) / 8,
      uploadThroughput: (750 * 1024) / 8,
    });
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
    const runs: number[] = [];
    for (let i = 0; i < 3; i++) {
      await page.goto(path, { waitUntil: "load" });
      runs.push(
        await page.evaluate(
          () =>
            new Promise<number>((resolve) => {
              new PerformanceObserver((list) => {
                const entries = list.getEntries();
                resolve(entries[entries.length - 1]?.startTime ?? 0);
              }).observe({ type: "largest-contentful-paint", buffered: true });
            }),
        ),
      );
    }
    runs.sort((a, b) => a - b);
    const median = runs[1] ?? 0;
    console.log(`LCP ${path}: median ${String(Math.round(median))} ms of ${runs.map((r) => String(Math.round(r))).join(", ")}`);
    expect(median).toBeGreaterThan(0);
    expect(median).toBeLessThan(2500);
  });
}
