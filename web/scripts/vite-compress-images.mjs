import { readdir, readFile, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';

const SUPPORTED_EXTS = new Set(['.png', '.jpg', '.jpeg', '.webp']);

async function collectImages(dir, out = []) {
  const entries = await readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      await collectImages(fullPath, out);
    } else if (entry.isFile() && SUPPORTED_EXTS.has(path.extname(entry.name).toLowerCase())) {
      out.push(fullPath);
    }
  }
  return out;
}

async function compressBuffer(input, ext) {
  const pipeline = sharp(input);
  if (ext === '.png') {
    // libimagequant 调色板量化支持完整 Alpha 通道，图标观感几乎无损且体积大幅下降
    return pipeline.png({ palette: true, quality: 90, compressionLevel: 9, effort: 8 }).toBuffer();
  }
  if (ext === '.jpg' || ext === '.jpeg') {
    return pipeline.jpeg({ quality: 82, mozjpeg: true }).toBuffer();
  }
  if (ext === '.webp') {
    return pipeline.webp({ quality: 82 }).toBuffer();
  }
  return input;
}

function formatBytes(bytes) {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  return `${(bytes / 1024).toFixed(1)} KB`;
}

/**
 * 构建期图片压缩插件：仅在 vite build 写入 dist 后压缩产物中的图片，
 * 不改动源码或 public/ 下的原始素材。
 */
export function compressDistImages() {
  let outDir = '';
  return {
    name: 'dst-compress-dist-images',
    apply: 'build',
    configResolved(config) {
      outDir = path.resolve(config.root, config.build.outDir);
    },
    async closeBundle() {
      if (!outDir) return;
      const files = await collectImages(outDir);
      if (files.length === 0) return;

      const concurrency = Math.min(8, Math.max(2, os.availableParallelism ? os.availableParallelism() : os.cpus().length));
      let cursor = 0;
      let totalBefore = 0;
      let totalAfter = 0;
      let compressedCount = 0;

      const worker = async () => {
        while (cursor < files.length) {
          const filePath = files[cursor++];
          const ext = path.extname(filePath).toLowerCase();
          const input = await readFile(filePath);
          totalBefore += input.length;
          const output = await compressBuffer(input, ext);
          if (output.length < input.length) {
            await writeFile(filePath, output);
            totalAfter += output.length;
            compressedCount += 1;
          } else {
            totalAfter += input.length;
          }
        }
      };

      await Promise.all(Array.from({ length: Math.min(concurrency, files.length) }, worker));
      const saved = totalBefore - totalAfter;
      const pct = totalBefore > 0 ? ((saved / totalBefore) * 100).toFixed(1) : '0.0';
      console.log(
        `[compress-images] 已压缩 ${compressedCount}/${files.length} 张图片：`
        + `${formatBytes(totalBefore)} → ${formatBytes(totalAfter)}（节省 ${formatBytes(saved)} / ${pct}%）`,
      );
    },
  };
}
