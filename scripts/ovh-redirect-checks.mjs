// Static/regression checks for the www redirect. This is not an Apache server
// test: the actual OVH redirect must also be checked after upload.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const config = readFileSync(new URL('../relaisdesk/.htaccess', import.meta.url), 'utf8');
assert.ok(config.includes('RewriteCond %{HTTP_HOST} ^www\\.relaisdesk\\.fr$ [NC]\n  RewriteCond %{THE_REQUEST} \\s/+([^?\\s]*)\n  RewriteRule ^ https://relaisdesk.fr/%1 [R=301,L,NE]'));
assert.ok(!config.includes('RewriteRule ^(.*)$ https://relaisdesk.fr/$1'));

for (const [uri, expected] of [
  ['/', 'https://relaisdesk.fr/'],
  ['/essai/', 'https://relaisdesk.fr/essai/'],
  ['/cgv.html', 'https://relaisdesk.fr/cgv.html'],
  ['/essai/?token=fixture&next=%2Fclient%2F', 'https://relaisdesk.fr/essai/?token=fixture&next=%2Fclient%2F'],
  ['/fichier%20avec%20espace%23.txt', 'https://relaisdesk.fr/fichier%20avec%20espace%23.txt'],
  ['//outside.example/path', 'https://relaisdesk.fr/outside.example/path'],
  ['/%0D%0AInjected%3Avalue', 'https://relaisdesk.fr/%0D%0AInjected%3Avalue'],
]) {
  const request = `GET ${uri} HTTP/1.1`;
  const path = /\s\/+([^?\s]*)/.exec(request)?.[1];
  assert.notEqual(path, undefined);
  const query = uri.includes('?') ? uri.slice(uri.indexOf('?')) : '';
  const target = `https://relaisdesk.fr/${path}${query}`;
  assert.equal(target, expected);
  assert.equal(new URL(target).hostname, 'relaisdesk.fr');
  assert.doesNotMatch(target, /[\r\n]/);
}
console.log('OK: www redirect uses the original request path, preserves query/encoding and pins the target host. Live Apache verification still required.');
