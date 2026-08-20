#!/usr/bin/env node
// Renders the next unused entry from ../posts.json into a branded PNG via a
// headless Chromium screenshot (no AI/LLM calls — pure template fill) and
// writes a PR body describing it. Advances ../state.json's index on success.
//
// Usage: node build-post.mjs
// Writes GITHUB_OUTPUT keys (if GITHUB_OUTPUT env var is set): skip, index
import { chromium } from "playwright";
import { readFileSync, writeFileSync, mkdirSync, appendFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const dir = path.dirname(fileURLToPath(import.meta.url));
const socialDir = path.resolve(dir, "..");

const posts = JSON.parse(readFileSync(path.join(socialDir, "posts.json"), "utf8"));
const state = JSON.parse(readFileSync(path.join(socialDir, "state.json"), "utf8"));

function setOutput(key, value) {
  if (process.env.GITHUB_OUTPUT) {
    appendFileSync(process.env.GITHUB_OUTPUT, `${key}=${value}\n`);
  }
}

if (state.next >= posts.length) {
  console.log(`No unused posts left (index ${state.next} >= ${posts.length}). Skipping.`);
  setOutput("skip", "true");
  process.exit(0);
}

const post = posts[state.next];
const dayLabel = String(post.id).padStart(2, "0");

function escapeHtml(s) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

const pointsHtml = post.points
  .map((p) => `<div class="point"><span class="dot"></span>${escapeHtml(p)}</div>`)
  .join("\n");

let html = readFileSync(path.join(socialDir, "template.html"), "utf8");
html = html
  .replace("{{EYEBROW}}", escapeHtml(post.eyebrow))
  .replace("{{HEADLINE}}", post.headlineHtml) // pre-authored, trusted content — contains the <span class="hl"> markup
  .replace("{{SUBTEXT}}", escapeHtml(post.subtext))
  .replace("{{POINTS}}", pointsHtml);

const outDir = path.join(socialDir, "output");
mkdirSync(outDir, { recursive: true });

const htmlPath = path.join(outDir, `day-${dayLabel}.html`);
const pngPath = path.join(outDir, `day-${dayLabel}.png`);
writeFileSync(htmlPath, html);

const browser = await chromium.launch();
try {
  const page = await browser.newPage({ viewport: { width: 1080, height: 1350 } });
  await page.goto("file://" + path.resolve(htmlPath), { waitUntil: "networkidle", timeout: 30000 });
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: pngPath });
} finally {
  await browser.close();
}

const branch = process.env.SOCIAL_BRANCH || "main";
const repo = process.env.GITHUB_REPOSITORY || "";
const imageUrl = repo
  ? `https://raw.githubusercontent.com/${repo}/${branch}/social/output/day-${dayLabel}.png`
  : `social/output/day-${dayLabel}.png`;

const prBody = `**DRAFT — for review, not posted.** This PR was opened automatically from a pre-written content calendar (\`social/posts.json\`, entry ${post.id}/${posts.length}) — no AI generation ran, it's a template fill + screenshot. Review the image and captions below, post manually on each platform, then merge or close this PR.

![draft image](${imageUrl})

### Instagram
${post.captions.instagram}

### Facebook
${post.captions.facebook}

### X/Twitter
${post.captions.twitter}

### LinkedIn
${post.captions.linkedin}
`;
writeFileSync(path.join(outDir, "pr-body.md"), prBody);

state.next += 1;
writeFileSync(path.join(socialDir, "state.json"), JSON.stringify(state, null, 2) + "\n");

console.log(`Built post ${post.id} -> ${pngPath}`);
setOutput("skip", "false");
setOutput("index", String(post.id));
setOutput("day_label", dayLabel);
