// Génère les images de l'icône Windows (desktop/winres) à partir des SVG de ce dossier :
//   node branding/render.mjs
// Playwright (Chromium) est nécessaire. Grandes tailles : logo.svg ; 32 px et moins : logo-small.svg.
import { chromium } from "playwright";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "..", "desktop", "winres");
const sizes = [
  [256, "logo.svg", "icon.png"],
  [48, "logo.svg", "icon48.png"],
  [32, "logo-small.svg", "icon32.png"],
  [24, "logo-small.svg", "icon24.png"],
  [16, "logo-small.svg", "icon16.png"],
];

const browser = await chromium.launch();
const page = await browser.newPage();
for (const [size, source, name] of sizes) {
  const svg = readFileSync(join(here, source), "utf8").replace(/width="120" height="120"/, `width="${size}" height="${size}"`);
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(`<html><body style="margin:0;background:transparent">${svg}</body></html>`);
  await page.locator("svg").screenshot({ path: join(out, name), omitBackground: true });
  console.log(`${name} (${size} px)`);
}
await browser.close();
