import assert from 'node:assert/strict';
import { createHash, createPublicKey, verify } from 'node:crypto';
import { createReadStream } from 'node:fs';
import fs from 'node:fs/promises';
import path from 'node:path';

const [directory, encodedPublicKey, expectedVersion, mode] = process.argv.slice(2);
assert(directory && encodedPublicKey && expectedVersion, 'directory, public key and version are required');
assert(mode === undefined || mode === '--without-nsis', 'unknown verification mode');
const files = [
  'RelaisDesk_Portable.exe', 'RelaisDesk_Setup.exe', 'RelaisDesk_Technicien.deb',
  'RelaisDesk_Technicien_Portable.exe', 'RelaisDesk_Technicien_Setup_1.0.0.exe',
  'RelaisDesk_viewer.deb', 'SHA256SUMS.txt',
].filter(name => mode !== '--without-nsis' || !name.includes('Setup')).sort();
assert.deepEqual((await fs.readdir(directory)).sort(), [...files, 'release-manifest.json'].sort(), 'unexpected or missing release files');
const manifest = JSON.parse(await fs.readFile(path.join(directory, 'release-manifest.json'), 'utf8'));
assert.equal(manifest.version, expectedVersion);
assert.deepEqual(manifest.artifacts.map(a => a.name), files);
const rawKey = Buffer.from(encodedPublicKey, 'base64url');
assert.equal(rawKey.length, 32);
const key = createPublicKey({
  key: Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), rawKey]),
  format: 'der', type: 'spki',
});
const payload = {
  version: manifest.version, published_at: manifest.published_at,
  key_id: manifest.key_id,
  artifacts: manifest.artifacts.map(({ name, url, sha256, size }) => ({ name, url, sha256, size })),
};
assert(verify(null, Buffer.from(JSON.stringify(payload)), key, Buffer.from(manifest.signature, 'base64url')), 'invalid Ed25519 signature');
const sums = [];
for (const artifact of manifest.artifacts) {
  const file = path.join(directory, artifact.name);
  const info = await fs.lstat(file);
  assert(info.isFile(), `${artifact.name}: regular file required`);
  assert.equal(info.size, artifact.size);
  assert.equal(artifact.url, `https://api.relaisdesk.fr/api/v1/downloads/${artifact.name}`);
  const hash = createHash('sha256');
  for await (const chunk of createReadStream(file)) hash.update(chunk);
  assert.equal(hash.digest('hex'), artifact.sha256, `${artifact.name}: SHA-256 mismatch`);
  if (artifact.name.endsWith('.txt')) continue;
  assert(info.size > 1024 * 1024, `${artifact.name}: unexpectedly small package`);
  const handle = await fs.open(file, 'r');
  try {
    const magic = artifact.name.endsWith('.exe') ? Buffer.from('MZ') : Buffer.from('!<arch>\n');
    const header = Buffer.alloc(magic.length);
    await handle.read(header, 0, header.length, 0);
    assert(header.equals(magic), `${artifact.name}: invalid package format`);
  } finally { await handle.close(); }
  sums.push(`${artifact.sha256}  ${artifact.name}`);
}
const recordedSums = (await fs.readFile(path.join(directory, 'SHA256SUMS.txt'), 'utf8')).trim().split(/\r?\n/);
assert.deepEqual(recordedSums.sort(), sums.sort());
console.log(`Verified ${manifest.version}: Ed25519, ${files.length} hashes, package headers and SHA256SUMS.`);
