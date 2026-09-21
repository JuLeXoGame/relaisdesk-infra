const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

const source = fs.readFileSync(path.join(__dirname, '../relaisdesk/client/app.js'), 'utf8');
const start = source.indexOf('window.handleGoogleLoginResponse =');
const end = source.indexOf('window.initGoogleSignIn =', start);
assert(start >= 0 && end > start);

async function check(payload, expectChallenge) {
  const button = { disabled: false };
  let challenge = '', loads = 0;
  const stored = new Map();
  const context = {
    window: {}, state: {},
    byId: id => id === 'btnSend2FAEmailCode' ? button : {},
    setMessage: () => {},
    showTOTPChallengeView: token => { challenge = token; },
    sessionStorage: { setItem: (key, value) => stored.set(key, value) },
    loadDashboard: async () => { loads++; },
    api: async () => ({ json: async () => payload }),
  };
  vm.runInNewContext(source.slice(start, end), context);
  await context.window.handleGoogleLoginResponse({ credential: 'test-google-token' });
  if (expectChallenge) {
    assert.equal(challenge, 'challenge');
    assert.equal(button.disabled, true);
    assert.equal(stored.size, 0, 'No session may be stored before MFA');
    assert.equal(loads, 0);
  } else {
    assert.equal(stored.get('rd_customer_token'), 'session');
    assert.equal(loads, 1);
  }
}
(async () => {
  await check({ requires_2fa: true, challenge_token: 'challenge', email_code_allowed: false }, true);
  await check({ token: 'session' }, false);
  console.log('Google login security: OK (MFA challenge and normal session)');
})().catch(err => { console.error(err); process.exitCode = 1; });
