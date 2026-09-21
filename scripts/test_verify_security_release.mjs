// Offline tests: generated fixture keys are never used for real releases.
import assert from 'node:assert/strict';
import { createHash, generateKeyPairSync, sign } from 'node:crypto';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const parent = await fs.mkdtemp(path.join(os.tmpdir(), 'relaisdesk-verify-test-'));
const verifier = fileURLToPath(new URL('./verify-security-release.mjs', import.meta.url));
const { publicKey, privateKey } = generateKeyPairSync('ed25519');
const pub = publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64url');
const hash = b => createHash('sha256').update(b).digest('hex');
let count = 0;
async function fixture(withoutNSIS = false) {
  const dir = path.join(parent, `case-${count++}`);
  await fs.mkdir(dir);
  let names = ['RelaisDesk_Portable.exe', 'RelaisDesk_Setup.exe', 'RelaisDesk_Technicien.deb',
    'RelaisDesk_Technicien_Portable.exe', 'RelaisDesk_Technicien_Setup_1.0.0.exe', 'RelaisDesk_viewer.deb'];
  if (withoutNSIS) names = names.filter(n => !n.includes('Setup'));
  const artifacts = [], sums = [];
  for (const name of names.sort()) {
    const body = Buffer.alloc(1024 * 1024 + 16);
    body.write(name.endsWith('.exe') ? 'MZ' : '!<arch>\n');
    await fs.writeFile(path.join(dir, name), body);
    artifacts.push({ name, url: `https://api.relaisdesk.fr/api/v1/downloads/${name}`, sha256: hash(body), size: body.length });
    sums.push(`${hash(body)}  ${name}`);
  }
  const body = Buffer.from(sums.join('\n') + '\n');
  await fs.writeFile(path.join(dir, 'SHA256SUMS.txt'), body);
  artifacts.push({ name: 'SHA256SUMS.txt', url: 'https://api.relaisdesk.fr/api/v1/downloads/SHA256SUMS.txt', sha256: hash(body), size: body.length });
  const payload = { version: '9.8.7', published_at: '2026-09-19T12:00:00Z', key_id: 'fixture', artifacts };
  await fs.writeFile(path.join(dir, 'release-manifest.json'), JSON.stringify({ ...payload, signature: sign(null, Buffer.from(JSON.stringify(payload)), privateKey).toString('base64url') }));
  return dir;
}
function verify(dir, extra = [], key = pub) {
  const result = spawnSync(process.execPath, [verifier, dir, key, '9.8.7', ...extra], { encoding: 'utf8', timeout: 15000 });
  assert(!result.error, String(result.error));
  return result.status;
}
try {
  let dir = await fixture();
  assert.equal(verify(dir), 0);
  assert.notEqual(verify(dir, [], Buffer.alloc(32).toString('base64url')), 0);
  dir = await fixture(true);
  assert.equal(verify(dir, ['--without-nsis']), 0);
  assert.notEqual(verify(dir), 0);
  dir = await fixture();
  await fs.appendFile(path.join(dir, 'RelaisDesk_Portable.exe'), 'tampered');
  assert.notEqual(verify(dir), 0);
  dir = await fixture();
  await fs.writeFile(path.join(dir, 'unexpected.env'), 'fixture');
  assert.notEqual(verify(dir), 0);
  dir = await fixture();
  await fs.unlink(path.join(dir, 'RelaisDesk_Setup.exe'));
  assert.notEqual(verify(dir), 0);
  console.log('7 signed-release checks passed (offline).');
} finally {
  // Only the unique temporary directory created above, with a verified parent.
  assert(path.dirname(parent) === path.resolve(os.tmpdir()));
  assert(path.basename(parent).startsWith('relaisdesk-verify-test-'));
  await fs.rm(parent, { recursive: true });
}
