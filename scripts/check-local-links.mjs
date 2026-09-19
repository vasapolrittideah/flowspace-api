#!/usr/bin/env node

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, extname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const linkPattern = /!?\[[^\]]*\]\((<[^>]+>|[^)\s]+)(?:\s+(?:"[^"]*"|'[^']*'|\([^)]*\)))?\)/g;

export function brokenLocalLinks(markdown, sourceFile) {
  const findings = [];
  let fence = '';

  for (const [index, line] of markdown.split('\n').entries()) {
    const marker = line.match(/^\s*(`{3,}|~{3,})/)?.[1]?.[0] ?? '';
    if (marker) {
      if (!fence) fence = marker;
      else if (fence === marker) fence = '';
      continue;
    }
    if (fence) continue;

    for (const match of line.matchAll(linkPattern)) {
      const rawTarget = match[1].replace(/^<|>$/g, '');
      const [path] = rawTarget.split('#', 1);
      if (!path || /^[a-z][a-z0-9+.-]*:/i.test(path)) continue;

      let decodedPath;
      try {
        decodedPath = decodeURIComponent(path);
      } catch {
        decodedPath = path;
      }
      const target = decodedPath.startsWith('/') ? resolve(decodedPath.slice(1)) : resolve(dirname(sourceFile), decodedPath);
      if (!existsSync(target)) findings.push({ line: index + 1, target: rawTarget });
    }
  }

  return findings;
}

function markdownFiles(paths) {
  const files = [];
  for (const path of paths) {
    const absolutePath = resolve(path);
    if (!existsSync(absolutePath)) throw new Error(`Path does not exist: ${path}`);
    if (statSync(absolutePath).isDirectory()) {
      files.push(...markdownFiles(readdirSync(absolutePath).map((entry) => resolve(absolutePath, entry))));
    } else if (extname(absolutePath) === '.md') {
      files.push(absolutePath);
    }
  }
  return files;
}

function main() {
  const files = markdownFiles(process.argv.slice(2).length ? process.argv.slice(2) : ['docs']);
  if (files.length === 0) throw new Error('No Markdown files were found.');

  const findings = files.flatMap((file) => brokenLocalLinks(readFileSync(file, 'utf8'), file).map((finding) => ({ file, ...finding })));
  if (findings.length > 0) {
    for (const { file, line, target } of findings) console.error(`${file}:${line}: Local link target does not exist: ${target}`);
    process.exitCode = 1;
    return;
  }

  console.log(`Checked local links in ${files.length} Markdown file(s).`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
