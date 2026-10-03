'use strict';

const API_BASE_URL = location.hostname === 'localhost' || location.hostname === '127.0.0.1'
  ? 'http://localhost:8443'
  : 'https://api.relaisdesk.fr';
const TERMS_VERSION = '2026-09-27';
const state = {
  token: sessionStorage.getItem('rd_customer_token') || '',
  data: null,
  renewalLicense: '',
  lastGeneratedCode: '',
  lastGeneratedEnrollCode: '',
  interventions: [],
  interventionToComplete: '',
  currentPanel: 'overview',
  deviceNextCursor: 0,
  devicesLoading: false,
  devices: [],
  deviceQuotas: [],
  folders: [],
  activeFolderId: '',
  viewAllDevices: false
};

const byId = (id) => document.getElementById(id);
const cancellationReview = window.RdSubscriptionCancellation.create(byId('cancelSubscriptionDialog'));
const formatDate = (value, withTime = false) => {
  if (!value) return '—';
  if (new Date(value).getUTCFullYear() === 9999) return 'Sans date de fin';
  const lang = (window.RdI18n && window.RdI18n.getLang() === 'en') ? 'en-US' : 'fr-FR';
  return new Intl.DateTimeFormat(lang, withTime ? { dateStyle: 'short', timeStyle: 'short' } : { dateStyle: 'medium' }).format(new Date(value));
};
const money = (value) => new Intl.NumberFormat('fr-FR', { style: 'currency', currency: 'EUR' }).format(Number(value || 0));

function setMessage(target, text, error = false) {
  if (!target) return;
  target.replaceChildren();
  for (const part of String(text || '').split(/(https?:\/\/[^\s<>"']+)/g)) {
    if (!part) continue;
    if (/^https?:\/\//.test(part)) {
      const href = part.replace(/[.,;:!?)\]]+$/, '');
      const link = document.createElement('a');
      link.href = href;
      link.textContent = href;
      link.target = '_blank';
      link.rel = 'noopener noreferrer';
      target.append(link);
      if (href.length < part.length) target.append(part.slice(href.length));
    } else {
      target.append(part);
    }
  }
  target.classList.toggle('error', error);
}

function copyTextToClipboard(text, successMsg) {
  if (!text || text === '----') return;
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => {
      setMessage(byId('appMessage'), successMsg);
    }).catch(() => {
      fallbackCopy(text, successMsg);
    });
  } else {
    fallbackCopy(text, successMsg);
  }
}

function fallbackCopy(text, successMsg) {
  const ta = document.createElement('textarea');
  ta.value = text;
  document.body.appendChild(ta);
  ta.select();
  document.execCommand('copy');
  document.body.removeChild(ta);
  setMessage(byId('appMessage'), successMsg);
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set('Accept', 'application/json');
  if (options.body && !(options.body instanceof FormData)) headers.set('Content-Type', 'application/json');
  if (state.token) headers.set('Authorization', `Bearer ${state.token}`);
  const response = await fetch(`${API_BASE_URL}${path}`, { ...options, headers, cache: 'no-store' });
  if (response.status === 401 && !path.includes('/login') && !path.includes('/password/')) void logout(false, true).catch(() => {});
  if (!response.ok) {
    let message = 'Une erreur est survenue.';
    let payload = null;
    try {
      payload = await response.json();
      message = payload.error || message;
    } catch (_) { /* réponse non JSON */ }
    const err = new Error(message);
    if (payload) err.payload = payload;
    throw err;
  }
  return response;
}

function showApp() {
  byId('authScreen').hidden = true;
  byId('appScreen').hidden = false;
}

function showAuth() {
  byId('authScreen').hidden = false;
  byId('appScreen').hidden = true;
}

function showPasswordLoginView() {
  byId('passwordLoginView').hidden = false;
  byId('forgotPasswordView').hidden = true;
  byId('resetPasswordView').hidden = true;
  if (byId('totpChallengeView')) byId('totpChallengeView').hidden = true;
  setMessage(byId('authMessage'), '');
  if (window.initGoogleSignIn) window.initGoogleSignIn();
}

function showForgotPasswordView() {
  byId('passwordLoginView').hidden = true;
  byId('forgotPasswordView').hidden = false;
  byId('resetPasswordView').hidden = true;
  if (byId('totpChallengeView')) byId('totpChallengeView').hidden = true;
  const currentEmail = byId('loginEmail').value.trim();
  if (currentEmail) byId('forgotEmail').value = currentEmail;
  setMessage(byId('authMessage'), '');
}

function showResetPasswordView(token) {
  byId('passwordLoginView').hidden = true;
  byId('forgotPasswordView').hidden = true;
  byId('resetPasswordView').hidden = false;
  if (byId('totpChallengeView')) byId('totpChallengeView').hidden = true;
  byId('resetToken').value = token || '';
  if (byId('enable2FAAtReset')) byId('enable2FAAtReset').checked = false;
  if (byId('reset2FAContainer')) byId('reset2FAContainer').style.display = 'none';
  reset2FASetup = null;
  setMessage(byId('authMessage'), '');
}

let emailCodeTimerInterval = null;

function stopEmailCodeTimer() {
  if (emailCodeTimerInterval) {
    clearInterval(emailCodeTimerInterval);
    emailCodeTimerInterval = null;
  }
}

function showTOTPChallengeView(challengeToken) {
  byId('passwordLoginView').hidden = true;
  byId('forgotPasswordView').hidden = true;
  byId('resetPasswordView').hidden = true;
  stopEmailCodeTimer();
  if (byId('totpChallengeView')) {
    byId('totpChallengeView').hidden = false;
    byId('totpChallengeToken').value = challengeToken;
    byId('totpChallengeCode').value = '';
    byId('totpChallengeCode').focus();
    if (byId('emailCodeNotice')) {
      byId('emailCodeNotice').textContent = '';
      byId('emailCodeNotice').style.display = 'none';
      byId('emailCodeNotice').classList.remove('error');
    }
    const sendBtn = byId('btnSend2FAEmailCode');
    if (sendBtn) {
      sendBtn.disabled = false;
      const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
      sendBtn.textContent = t('client_2fa_send_email_btn') || '✉️ M\'envoyer un code par e-mail';
    }
    if (byId('rememberDeviceCheckbox')) {
      byId('rememberDeviceCheckbox').checked = true;
    }
  }
  setMessage(byId('authMessage'), '');
}

window.handleGoogleLoginResponse = async function(googleResponse) {
  if (!googleResponse || !googleResponse.credential) {
    setMessage(byId('authMessage'), 'Impossible de récupérer les informations Google.', true);
    return;
  }

  setMessage(byId('authMessage'), 'Connexion avec Google en cours…');

  try {
    const response = await api('/api/v1/customer/login/google', {
      method: 'POST',
      body: JSON.stringify({ credential: googleResponse.credential })
    });
    const data = await response.json();
    if (data.requires_2fa) {
      showTOTPChallengeView(data.challenge_token);
      const sendBtn = byId('btnSend2FAEmailCode');
      if (sendBtn) sendBtn.disabled = data.email_code_allowed === false;
      setMessage(byId('authMessage'), 'Utilisez votre application d’authentification ou un code de secours.');
      return;
    }
    state.token = data.token;
    sessionStorage.setItem('rd_customer_token', state.token);
    await loadDashboard();
  } catch (error) {
    setMessage(byId('authMessage'), error.message || 'Aucun compte client associé à cette adresse Google.', true);
  }
};

window.initGoogleSignIn = function() {
  const container = document.getElementById('googleSignInBtn');
  if (!container) return;
  if (typeof google === 'undefined' || !google.accounts || !google.accounts.id) {
    setTimeout(window.initGoogleSignIn, 150);
    return;
  }
  if (container.querySelector('iframe')) return;

  try {
    google.accounts.id.initialize({
      client_id: '226991768980-c2rcfkicoft0hl9m346n45ad088utri9.apps.googleusercontent.com',
      callback: window.handleGoogleLoginResponse,
      auto_select: false,
      cancel_on_tap_outside: true
    });
    google.accounts.id.renderButton(container, {
      type: 'standard',
      theme: 'filled_black',
      size: 'large',
      text: 'signin_with',
      shape: 'rectangular',
      logo_alignment: 'left',
      width: 320
    });
  } catch (err) {
    console.error('Initialisation Google Sign-In:', err);
  }
};

async function handlePasswordLogin(event) {
  event.preventDefault();
  const button = byId('passwordLoginButton');
  button.disabled = true;
  setMessage(byId('authMessage'), 'Connexion en cours…');
  const email = byId('loginEmail').value.trim().toLowerCase();
  const password = byId('loginPassword').value;
  const deviceToken = localStorage.getItem('rd_device_token') || '';

  try {
    const response = await api('/api/v1/customer/login', {
      method: 'POST',
      body: JSON.stringify({ email, password, device_token: deviceToken })
    });
    const data = await response.json();
    if (data.requires_2fa) {
      showTOTPChallengeView(data.challenge_token);
      return;
    }
    state.token = data.token;
    sessionStorage.setItem('rd_customer_token', state.token);
    await loadDashboard();
  } catch (error) {
    if (error.payload && error.payload.need_password_setup) {
      showForgotPasswordView();
      byId('forgotEmail').value = email;
      setMessage(byId('authMessage'), 'Aucun mot de passe n’est configuré pour ce compte. Cliquez ci-dessous pour recevoir un lien par e-mail afin de le définir.', true);
    } else {
      setMessage(byId('authMessage'), error.message, true);
    }
  } finally {
    button.disabled = false;
  }
}

async function handleTOTPChallenge(event) {
  event.preventDefault();
  const button = byId('totpChallengeSubmitButton');
  button.disabled = true;
  setMessage(byId('authMessage'), 'Vérification du code…');
  const challengeToken = byId('totpChallengeToken').value;
  const code = byId('totpChallengeCode').value.trim();
  const rememberDevice = byId('rememberDeviceCheckbox') ? byId('rememberDeviceCheckbox').checked : false;
  const deviceName = (navigator.userAgent || '').slice(0, 100);

  try {
    const response = await api('/api/v1/customer/login/2fa', {
      method: 'POST',
      body: JSON.stringify({
        challenge_token: challengeToken,
        code,
        remember_device: rememberDevice,
        device_name: deviceName
      })
    });
    const data = await response.json();
    state.token = data.token;
    sessionStorage.setItem('rd_customer_token', state.token);
    if (data.device_token) {
      localStorage.setItem('rd_device_token', data.device_token);
    } else if (!rememberDevice) {
      localStorage.removeItem('rd_device_token');
    }
    stopEmailCodeTimer();
    await loadDashboard();
  } catch (error) {
    setMessage(byId('authMessage'), error.message, true);
  } finally {
    button.disabled = false;
  }
}

async function handleSend2FAEmailCode() {
  const button = byId('btnSend2FAEmailCode');
  const notice = byId('emailCodeNotice');
  const challengeToken = byId('totpChallengeToken').value;
  if (!challengeToken) return;

  button.disabled = true;
  if (notice) {
    notice.style.display = 'block';
    notice.classList.remove('error');
    notice.textContent = 'Envoi du code en cours…';
  }

  try {
    const response = await api('/api/v1/customer/login/2fa/send-email-code', {
      method: 'POST',
      body: JSON.stringify({ challenge_token: challengeToken })
    });
    const data = await response.json();
    const masked = data.email_masked || '';
    if (notice) {
      notice.style.display = 'block';
      notice.classList.remove('error');
      const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
      const baseNotice = t('client_2fa_email_sent') || 'Code envoyé par e-mail ! Valable 15 minutes.';
      notice.textContent = masked ? `${baseNotice} (${masked})` : baseNotice;
    }
    byId('totpChallengeCode').focus();

    // 30s countdown
    let remaining = 30;
    stopEmailCodeTimer();
    button.disabled = true;
    const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
    const baseText = t('client_2fa_send_email_btn') || '✉️ M\'envoyer un code par e-mail';
    button.textContent = `Renvoyer (${remaining}s)`;
    emailCodeTimerInterval = setInterval(() => {
      remaining--;
      if (remaining <= 0) {
        stopEmailCodeTimer();
        button.disabled = false;
        button.textContent = baseText;
      } else {
        button.textContent = `Renvoyer (${remaining}s)`;
      }
    }, 1000);
  } catch (error) {
    button.disabled = false;
    if (notice) {
      notice.style.display = 'block';
      notice.classList.add('error');
      notice.textContent = error.message;
    }
  }
}

let reset2FASetup = null;

async function handleToggle2FAAtReset() {
  const checkbox = byId('enable2FAAtReset');
  const container = byId('reset2FAContainer');
  if (!checkbox || !container) return;
  if (!checkbox.checked) {
    container.style.display = 'none';
    reset2FASetup = null;
    return;
  }

  const token = byId('resetToken').value.trim();
  if (!token) return;

  try {
    container.style.display = 'block';
    byId('resetSecretDisplay').textContent = 'Génération de la clé…';
    const response = await api('/api/v1/customer/2fa/setup-token', {
      method: 'POST',
      body: JSON.stringify({ token })
    });
    const data = await response.json();
    reset2FASetup = data;

    if (window.qrcode) {
      const qr = window.qrcode(0, 'M');
      qr.addData(data.otpauth_url);
      qr.make();
      byId('resetQRCodeDisplay').innerHTML = qr.createSvgTag({ scalable: true, cellSize: 4, margin: 0 });
    }
    byId('resetSecretDisplay').textContent = data.secret;
    if (data.recovery_codes && data.recovery_codes.length) {
      byId('resetRecoveryCodesBox').style.display = 'block';
      byId('resetRecoveryCodesList').textContent = data.recovery_codes.join('\n');
    }
  } catch (err) {
    checkbox.checked = false;
    container.style.display = 'none';
    setMessage(byId('authMessage'), 'Impossible de préparer la 2FA : ' + err.message, true);
  }
}

async function handleForgotPassword(event) {
  event.preventDefault();
  const button = byId('forgotSubmitButton');
  button.disabled = true;
  setMessage(byId('authMessage'), 'Envoi en cours…');
  const email = byId('forgotEmail').value.trim().toLowerCase();

  try {
    const response = await api('/api/v1/customer/login/request', {
      method: 'POST',
      body: JSON.stringify({ email })
    });
    const data = await response.json();
    setMessage(byId('authMessage'), data.message || 'Si cette adresse correspond à un compte, un lien sécurisé vous a été envoyé.');
  } catch (error) {
    setMessage(byId('authMessage'), error.message, true);
  } finally {
    button.disabled = false;
  }
}

async function handleResetPassword(event) {
  event.preventDefault();
  const button = byId('resetSubmitButton');
  const token = byId('resetToken').value.trim();
  const password = byId('newPassword').value;
  const confirm = byId('confirmPassword').value;

  if (password !== confirm) {
    setMessage(byId('authMessage'), 'Les deux mots de passe ne correspondent pas.', true);
    return;
  }
  if (password.length < 8) {
    setMessage(byId('authMessage'), 'Le mot de passe doit comporter au moins 8 caractères.', true);
    return;
  }

  const enable2FA = byId('enable2FAAtReset')?.checked;
  const totpCode = byId('resetTotpCode')?.value.trim();

  if (enable2FA) {
    if (!totpCode || totpCode.length !== 6) {
      setMessage(byId('authMessage'), 'Veuillez saisir le code à 6 chiffres de votre application Authenticator pour confirmer la 2FA.', true);
      return;
    }
  }

  button.disabled = true;
  setMessage(byId('authMessage'), 'Enregistrement en cours…');

  try {
    const payload = { token, password, current_2fa_code: byId('resetCurrent2FACode').value.trim() };
    if (enable2FA && reset2FASetup) {
      payload.totp_secret = reset2FASetup.secret;
      payload.totp_code = totpCode;
      payload.recovery_codes = reset2FASetup.recovery_codes;
    }

    const response = await api('/api/v1/customer/password/reset', {
      method: 'POST',
      body: JSON.stringify(payload)
    });
    const data = await response.json();
    state.token = data.token;
    sessionStorage.setItem('rd_customer_token', state.token);
    if (window.history && window.history.replaceState) {
      window.history.replaceState({}, document.title, location.pathname);
    }
    await loadDashboard();
  } catch (error) {
    setMessage(byId('authMessage'), error.message, true);
  } finally {
    button.disabled = false;
  }
}

async function handleChangePassword(event) {
  event.preventDefault();
  const button = byId('changePasswordButton');
  const msg = byId('changePasswordMessage');
  const oldPassword = byId('currentPassword').value;
  const newPassword = byId('changeNewPassword').value;
  const confirmPassword = byId('changeConfirmPassword').value;

  if (newPassword.length < 8) {
    setMessage(msg, 'Le nouveau mot de passe doit comporter au moins 8 caractères.', true);
    return;
  }
  if (newPassword !== confirmPassword) {
    setMessage(msg, 'Les deux nouveaux mots de passe ne correspondent pas.', true);
    return;
  }

  button.disabled = true;
  setMessage(msg, 'Modification en cours…');

  try {
    const response = await api('/api/v1/customer/preferences/password', {
      method: 'POST',
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword })
    });
    const data = await response.json();
    setMessage(msg, data.message || 'Mot de passe modifié avec succès.');
    byId('changePasswordForm').reset();
    await logout(false);
    showPasswordLoginView();
    setMessage(byId('authMessage'), 'Mot de passe modifié. Les anciennes sessions sont fermées ; reconnectez-vous.');
  } catch (error) {
    setMessage(msg, error.message, true);
  } finally {
    button.disabled = false;
  }
}

let current2FASetup = null;

async function fetch2FAStatus() {
  if (!state.token) return;
  try {
    const resp = await api('/api/v1/customer/2fa/status');
    const data = await resp.json();
    const enabled = Boolean(data.enabled);
    const badge = byId('totpStatusBadge');
    const desc = byId('totpStatusDescription');
    const btnStart = byId('btnStart2FASetup');
    const btnRegen = byId('btnRegenRecoveryCodes');
    const btnDisable = byId('btnDisable2FA');
    const recoveryInfo = byId('totpRecoveryRemaining');

    if (badge) {
      badge.textContent = enabled ? 'Activé' : 'Désactivé';
      badge.style.background = enabled ? 'rgba(16, 185, 129, 0.15)' : 'rgba(239, 68, 68, 0.15)';
      badge.style.color = enabled ? '#10b981' : '#ef4444';
      badge.style.borderColor = enabled ? 'rgba(16, 185, 129, 0.3)' : 'rgba(239, 68, 68, 0.3)';
    }
    if (desc) {
      desc.textContent = enabled
        ? 'Votre compte est protégé par la validation en deux étapes (TOTP).'
        : 'Protégez votre compte avec une application d’authentification (Google Authenticator, Microsoft Authenticator, Aegis, etc.).';
    }
    if (btnStart) btnStart.style.display = enabled ? 'none' : 'inline-block';
    if (btnRegen) btnRegen.style.display = enabled ? 'inline-block' : 'none';
    if (btnDisable) btnDisable.style.display = enabled ? 'inline-block' : 'none';
    if (recoveryInfo) {
      recoveryInfo.style.display = enabled ? 'block' : 'none';
      if (enabled) {
        const count = data.remaining_recovery_codes ?? 0;
        recoveryInfo.textContent = `Codes de secours restants : ${count}`;
      }
    }
  } catch (err) {
    console.warn('Erreur statut 2FA:', err);
  }
}

async function start2FASetup() {
  const msg = byId('setup2FAMessage');
  setMessage(msg, '');
  byId('setup2FAInputCode').value = '';
  byId('setup2FAPassword').value = '';
  byId('btnConfirm2FAActivation').disabled = false;

  try {
    const resp = await api('/api/v1/customer/2fa/setup', { method: 'POST' });
    const data = await resp.json();
    current2FASetup = data;

    byId('setup2FASecretKey').textContent = data.secret;
    if (window.qrcode) {
      const qr = window.qrcode(0, 'M');
      qr.addData(data.otpauth_url);
      qr.make();
      byId('setup2FAQRElement').innerHTML = qr.createSvgTag({ scalable: true, cellSize: 4, margin: 0 });
    }

    if (data.recovery_codes && data.recovery_codes.length) {
      byId('setup2FARecoveryNotice').style.display = 'block';
      byId('setup2FARecoveryList').textContent = data.recovery_codes.join('\n');
    } else {
      byId('setup2FARecoveryNotice').style.display = 'none';
    }

    byId('setup2FADialog').showModal();
  } catch (err) {
    setMessage(byId('appMessage'), 'Impossible de démarrer la configuration 2FA : ' + err.message, true);
  }
}

async function handleConfirm2FAActivation(e) {
  e.preventDefault();
  if (!current2FASetup) return;
  const code = byId('setup2FAInputCode').value.trim();
  const msg = byId('setup2FAMessage');
  const btn = byId('btnConfirm2FAActivation');

  if (code.length !== 6) {
    setMessage(msg, 'Le code doit comporter exactement 6 chiffres.', true);
    return;
  }

  btn.disabled = true;
  setMessage(msg, 'Vérification du code…');

  try {
    await api('/api/v1/customer/2fa/enable', {
      method: 'POST',
      body: JSON.stringify({
        password: byId('setup2FAPassword').value,
        secret: current2FASetup.secret,
        code: code,
        recovery_codes: current2FASetup.recovery_codes
      })
    });
    byId('setup2FADialog').close();
    current2FASetup = null;
    byId('setup2FAPassword').value = '';
    byId('setup2FASecretKey').textContent = '';
    byId('setup2FARecoveryList').textContent = '';
    byId('setup2FAQRElement').replaceChildren();
    await logout(false);
    showPasswordLoginView();
    setMessage(byId('authMessage'), '2FA activée. Reconnectez-vous avec votre mot de passe et un code 2FA.');
  } catch (err) {
    setMessage(msg, err.message, true);
    btn.disabled = false;
  }
}

function openDisable2FADialog() {
  byId('disable2FAPassword').value = '';
  byId('disable2FACode').value = '';
  setMessage(byId('disable2FAMessage'), '');
  byId('btnConfirmDisable2FA').disabled = false;
  byId('disable2FADialog').showModal();
}

async function handleConfirmDisable2FA(e) {
  e.preventDefault();
  const password = byId('disable2FAPassword').value;
  const msg = byId('disable2FAMessage');
  const btn = byId('btnConfirmDisable2FA');

  if (!password) {
    setMessage(msg, 'Veuillez saisir votre mot de passe.', true);
    return;
  }

  btn.disabled = true;
  setMessage(msg, 'Désactivation en cours…');

  try {
    await api('/api/v1/customer/2fa/disable', {
      method: 'POST',
      body: JSON.stringify({ password, code: byId('disable2FACode').value.trim() })
    });
    byId('disable2FADialog').close();
    byId('disable2FAPassword').value = '';
  byId('disable2FACode').value = '';
    await logout(false);
    showPasswordLoginView();
    setMessage(byId('authMessage'), '2FA désactivée. Les anciennes sessions sont fermées ; reconnectez-vous.');
  } catch (err) {
    setMessage(msg, err.message, true);
    btn.disabled = false;
  }
}

async function handleRegenRecoveryCodes() {
  const pwd = window.prompt('Pour renouveler vos codes de secours, veuillez confirmer votre mot de passe :');
  if (!pwd) return;
  const code = window.prompt('Saisissez un nouveau code 2FA ou un code de secours non utilisé :');
  if (!code) return;

  try {
    const resp = await api('/api/v1/customer/2fa/recovery-codes', {
      method: 'POST',
      body: JSON.stringify({ password: pwd, code: code.trim() })
    });
    const data = await resp.json();
    byId('regenRecoveryCodesList').textContent = (data.recovery_codes || []).join('\n');
    byId('recoveryCodesDialog').showModal();
    await fetch2FAStatus();
  } catch (err) {
    setMessage(byId('appMessage'), 'Erreur régénération des codes : ' + err.message, true);
  }
}

async function loadDashboard() {
  if (!state.token) return showAuth();
  document.body.classList.add('loading');
  try {
    const response = await api('/api/v1/customer/dashboard');
    state.data = await response.json();
    renderDashboard();
    setMessage(byId('appMessage'), '');
    showApp();
  } catch (error) {
    setMessage(byId('appMessage'), error.message, true);
    if (!state.token) showAuth();
  } finally { document.body.classList.remove('loading'); }
}

function renderDashboard() {
  const data = state.data;
  byId('accountCustomerID').textContent = data.customer_id ? `Compte ${data.customer_id}` : '';
  byId('accountEmail').textContent = data.email;
  byId('metricLicenses').textContent = data.licenses.length;
  byId('metricInvoices').textContent = data.invoices.length;
  byId('metricInterventions').textContent = data.interventions.length;
  byId('remindersToggle').checked = Boolean(data.renewal_reminders_enabled);
  renderExpiries(data.licenses);
  renderSubscriptions(data.subscriptions || []);
  renderLicenses(data.licenses);
  renderOrders(data.orders);
  renderInvoices(data.invoices);
  renderInterventions(data.interventions);
  fillLicenseSelect(data.licenses);
  fillCodesLicenseSelect(data.licenses);
  fillEnrollLicenseSelect(data.licenses);
  fetchViewerCodes();
  fetchDevices();
  fetch2FAStatus();

  if (window.rdTeams) window.rdTeams.load();
  if (window.rdServices) window.rdServices.navigation();

  const savedPanel = state.currentPanel || (location.hash ? location.hash.replace('#', '') : null);
  if (savedPanel) {
    switchPanel(savedPanel);
  }
}

function clearAndEmpty(container, emptyText) {
  container.replaceChildren();
  if (!container.children.length && emptyText) {
    const empty = document.createElement('p'); empty.className = 'empty'; empty.textContent = emptyText; container.append(empty);
  }
}

function renderSubscriptions(items) {
  const isEn = window.RdI18n && window.RdI18n.getLang() === 'en';
  const container = byId('subscriptionsList');
  if (!container) return;
  container.replaceChildren(); byId('subscriptionsCard').hidden = items.length === 0;
  for (const item of items) {
    const card = document.createElement('article'); card.className = 'license-card';
    const title = document.createElement('h3'); title.textContent = `${item.plan} — ${item.technicians} ${isEn ? 'concurrent technician(s)' : 'technicien(s) simultané(s)'}`;
    const end = Math.max(item.trial_end, item.paid_through);
    const period = item.billing_cycle === 'annual' ? (isEn ? 'year' : 'an') : (isEn ? 'month' : 'mois');
    const price = `${money(item.price_cents / 100)}/${period}`;
    const expiry = end ? formatDate(new Date(end * 1000), true) : (isEn ? 'activation pending' : 'activation en cours');
    const detail = document.createElement('p'); detail.textContent = isEn ? `Price after trial: ${price}. Free or paid access ends: ${expiry}.` : `Tarif après l'essai : ${price}. Fin de l'accès gratuit ou payé : ${expiry}.`;
    if (item.withdrawal_immediate) detail.textContent = isEn ? 'Access stopped following withdrawal.' : 'Accès arrêté à la suite de la rétractation.';
    const status = document.createElement('p');
    const cancelled = item.cancel_at_period_end || item.subscription_status === 'canceled';
    status.textContent = cancelled ? (isEn ? 'Automatic renewal cancelled.' : 'Renouvellement automatique annulé.') : item.cancel_requested_at ? (isEn ? 'Cancellation requested — awaiting Stripe confirmation.' : 'Annulation demandée — confirmation Stripe en attente.') : (isEn ? 'Automatic renewal and billing enabled after the trial.' : 'Renouvellement et prélèvement automatiques actifs après l’essai.');
    const button = document.createElement('button'); button.className = 'button secondary'; button.type = 'button';
    button.textContent = isEn ? 'Cancel automatic renewal' : 'Annuler le renouvellement automatique'; button.disabled = cancelled;
    button.addEventListener('click', async () => {
      if (button.disabled) return;
      button.disabled = true;
      const reviewToken = state.token;
      try {
        const accepted = await cancellationReview.confirm({
          language: isEn ? 'en' : 'fr', contract: item.id, account: state.data?.email,
          offer: title.textContent, price, end: expiry
        });
        if (!accepted || !reviewToken || state.token !== reviewToken) return;
        const response = await api('/api/v1/customer/subscriptions/cancel', { method: 'POST', body: JSON.stringify({ id: item.id }) });
        const data = await response.json(); await loadDashboard(); setMessage(byId('appMessage'), data.message, response.status === 202);
      } catch (error) { setMessage(byId('appMessage'), error.message, true); }
      finally { button.disabled = cancelled; }
    });
    const portalBtn = document.createElement('button'); portalBtn.className = 'button secondary'; portalBtn.type = 'button';
    portalBtn.textContent = isEn ? 'Manage payment method' : 'Gérer mon moyen de paiement';
    portalBtn.addEventListener('click', async () => {
      if (portalBtn.disabled) return;
      portalBtn.disabled = true;
      try {
        const response = await api('/api/v1/customer/billing-portal', { method: 'POST', body: JSON.stringify({ id: item.id }) });
        const data = await response.json();
        if (!response.ok || !data.url) throw new Error(data.error || (isEn ? 'Payment portal unavailable' : 'Portail de paiement indisponible'));
        window.location.href = data.url;
      } catch (error) { setMessage(byId('appMessage'), error.message, true); }
      finally { portalBtn.disabled = false; }
    });
    const reference = document.createElement('p'); reference.textContent = `${isEn ? 'Contract' : 'Contrat'} : ${item.id}`; reference.style.overflowWrap = 'anywhere';
    const withdrawal = document.createElement('a'); withdrawal.href = '../formulaire-retractation.html#trial=' + encodeURIComponent(item.id);
    withdrawal.textContent = item.withdrawal_requested_at ? (isEn ? 'Withdrawal already notified — view form' : 'Rétractation déjà notifiée — formulaire') : (isEn ? 'Withdraw from this contract' : 'Se rétracter de ce contrat');
    card.append(title, reference, detail, status, button, portalBtn, withdrawal); container.append(card);
  }
}

function badge(status) {
  const node = document.createElement('span'); node.className = 'badge';
  const labels = {
    fr: { active: 'Active', expired: 'Expirée', revoked: 'Révoquée', pending: 'En attente', paid: 'Payée', processing: 'Traitement', cancelled: 'Annulée', planned: 'Planifiée', client_ready: 'Client prêt', in_progress: 'En cours', completed: 'Terminée' },
    en: { active: 'Active', expired: 'Expired', revoked: 'Revoked', pending: 'Pending', paid: 'Paid', processing: 'Processing', cancelled: 'Cancelled', planned: 'Planned', client_ready: 'Client ready', in_progress: 'In progress', completed: 'Completed' },
    de: { active: 'Aktiv', expired: 'Abgelaufen', revoked: 'Widerrufen', pending: 'Ausstehend', paid: 'Bezahlt', processing: 'In Bearbeitung', cancelled: 'Storniert', planned: 'Geplant', client_ready: 'Kunde bereit', in_progress: 'In Bearbeitung', completed: 'Abgeschlossen' },
    es: { active: 'Activa', expired: 'Expirada', revoked: 'Revocada', pending: 'Pendiente', paid: 'Pagada', processing: 'Procesando', cancelled: 'Cancelada', planned: 'Planificada', client_ready: 'Cliente listo', in_progress: 'En curso', completed: 'Completada' },
    it: { active: 'Attiva', expired: 'Scaduta', revoked: 'Revocata', pending: 'In attesa', paid: 'Pagata', processing: 'In elaborazione', cancelled: 'Annullata', planned: 'Pianificata', client_ready: 'Cliente pronto', in_progress: 'In corso', completed: 'Completata' },
    ru: { active: 'Активна', expired: 'Истекла', revoked: 'Отозвана', pending: 'В ожидании', paid: 'Оплачена', processing: 'Обработка', cancelled: 'Отменена', planned: 'Запланирована', client_ready: 'Клиент готов', in_progress: 'В процессе', completed: 'Завершена' },
    pl: { active: 'Aktywna', expired: 'Wygasła', revoked: 'Odwołana', pending: 'Oczekująca', paid: 'Opłacona', processing: 'Przetwarzanie', cancelled: 'Anulowana', planned: 'Zaplanowana', client_ready: 'Klient gotowy', in_progress: 'W toku', completed: 'Ukończona' }
  };
  const lang = (window.RdI18n && window.RdI18n.getLang()) || 'fr';
  const dict = labels[lang] || labels.fr;
  node.textContent = dict[status] || status;
  if (['expired', 'pending', 'processing', 'client_ready'].includes(status)) node.classList.add('warn');
  if (['revoked', 'cancelled'].includes(status)) node.classList.add('danger');
  return node;
}

function renderExpiries(licenses) {
  const container = byId('expirySummary'); container.replaceChildren();
  if (!licenses.length) return clearAndEmpty(container, 'Aucun forfait associé à ce compte.');
  licenses.slice(0, 5).forEach((license) => {
    const row = document.createElement('div'); row.className = 'list-item';
    const planName = license.plan ? ('Plan ' + license.plan.charAt(0).toUpperCase() + license.plan.slice(1)) : (license.notes || 'Forfait');
    const label = document.createElement('span'); label.textContent = planName;
    const date = document.createElement('strong'); date.textContent = `Échéance : ${formatDate(license.expires_at)}`;
    row.append(label, date); container.append(row);
  });
}

function renderLicenses(licenses) {
  const container = byId('licensesList'); container.replaceChildren();
  if (!licenses.length) return clearAndEmpty(container, 'Aucun forfait disponible.');
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  licenses.forEach((license) => {
    const card = document.createElement('article'); card.className = 'license-card';
    const planTitle = license.plan ? ('Plan ' + license.plan.charAt(0).toUpperCase() + license.plan.slice(1)) : 'Forfait RelaisDesk';
    const header = document.createElement('header'); const title = document.createElement('h3'); title.textContent = planTitle; header.append(title, badge(license.status));
    const meta = document.createElement('div'); meta.className = 'license-meta';
    const capacity = document.createElement('span'); capacity.textContent = t('capacity_label'); const capacityValue = document.createElement('strong'); capacityValue.textContent = `${license.max_connections} ${t('capacity_unit')}`; capacity.append(capacityValue);
    const expiry = document.createElement('span'); expiry.textContent = t('expiry_label'); const expiryValue = document.createElement('strong'); expiryValue.textContent = formatDate(license.expires_at); expiry.append(expiryValue); meta.append(capacity, expiry);
    const button = document.createElement('button'); button.className = 'button primary'; button.type = 'button';
    button.textContent = license.pending_renewal ? t('renew_pending') : t('btn_renew_month'); button.disabled = license.pending_renewal || !license.renewable;
    button.addEventListener('click', () => openRenewal(license));
    card.append(header, meta, button); container.append(card);
  });
}

function renderOrders(orders) {
  const body = byId('ordersBody'); body.replaceChildren();
  orders.forEach((order) => {
    const row = document.createElement('tr');
    [order.order_id, order.order_kind === 'renewal' ? 'Renouvellement' : 'Souscription', `${order.plan} — ${order.technicians}`, money(order.price)].forEach((value) => { const cell = document.createElement('td'); cell.textContent = value; row.append(cell); });
    const status = document.createElement('td'); status.append(badge(order.status)); row.append(status);
    const date = document.createElement('td'); date.textContent = formatDate(order.created_at); row.append(date); body.append(row);
  });
  if (!orders.length) { const row = body.insertRow(); const cell = row.insertCell(); cell.colSpan = 6; cell.className = 'empty'; cell.textContent = 'Aucune commande.'; }
}

function renderInvoices(invoices) {
  const body = byId('invoicesBody'); body.replaceChildren();
  const isEn = window.RdI18n && window.RdI18n.getLang() === 'en';
  invoices.forEach((item) => {
    const row = document.createElement('tr');
    [item.invoice_number, item.plan, money(item.amount_ttc), formatDate(item.created_at)].forEach((value) => { const cell = document.createElement('td'); cell.textContent = value; row.append(cell); });
    const action = document.createElement('td'); const button = document.createElement('button'); button.className = 'button secondary'; button.type = 'button'; button.textContent = isEn ? 'Download' : 'Télécharger'; button.addEventListener('click', () => downloadInvoice(item.invoice_number)); action.append(button); const xmlButton = document.createElement('button'); xmlButton.className = 'button secondary'; xmlButton.type = 'button'; xmlButton.textContent = 'XML'; xmlButton.title = isEn ? 'Download e-invoice (Factur-X)' : 'Télécharger la facture électronique (Factur-X)'; xmlButton.addEventListener('click', () => downloadInvoiceCII(item.invoice_number)); action.append(xmlButton); row.append(action); body.append(row);
  });
  if (!invoices.length) { const row = body.insertRow(); const cell = row.insertCell(); cell.colSpan = 5; cell.className = 'empty'; cell.textContent = 'Aucune facture.'; }
}

function renderInterventions(items) {
  const body = byId('interventionsBody'); body.replaceChildren();
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  items.forEach((item) => {
    const row = document.createElement('tr');
    [item.intervention_id, item.client_reference || '—', item.title, item.summary || '—'].forEach((value) => { const cell = document.createElement('td'); cell.textContent = value; row.append(cell); });
    const status = document.createElement('td'); status.append(badge(item.status)); row.append(status);
    const started = document.createElement('td'); started.textContent = formatDate(item.started_at || item.created_at, true); row.append(started);
    const duration = document.createElement('td'); duration.textContent = item.duration_minutes == null ? '—' : `${item.duration_minutes} min`; row.append(duration);
    const actions = document.createElement('td');
    const actionsWrap = document.createElement('div'); actionsWrap.className = 'device-actions';
    const addButton = (label, className, onClick) => { const button = document.createElement('button'); button.className = className; button.type = 'button'; button.textContent = label; button.addEventListener('click', onClick); actionsWrap.append(button); };
    if (item.status === 'planned' || item.status === 'client_ready') addButton(t('btn_start_intervention'), 'button secondary', () => updateIntervention(item.intervention_id, 'start'));
    if (item.status === 'in_progress') addButton(t('btn_complete_intervention'), 'button primary', () => openCompleteIntervention(item.intervention_id));
    if (item.status !== 'completed' && item.status !== 'cancelled') addButton(t('btn_cancel_intervention'), 'button btn-danger', () => updateIntervention(item.intervention_id, 'cancel'));
    if (actionsWrap.hasChildNodes()) actions.append(actionsWrap); else actions.textContent = '—';
    row.append(actions); body.append(row);
  });
  if (!items.length) { const row = body.insertRow(); const cell = row.insertCell(); cell.colSpan = 8; cell.className = 'empty'; cell.textContent = 'Aucune intervention enregistrée.'; }
}

async function fetchInterventions() {
  const tbody = byId('interventionsBody');
  if (!tbody) return;
  try {
    const response = await api('/api/v1/customer/interventions');
    const data = await response.json();
    const items = Array.isArray(data.interventions) ? data.interventions : [];
    state.interventions = items;
    if (state.data) state.data.interventions = items;
    if (byId('metricInterventions')) byId('metricInterventions').textContent = items.length;
    renderInterventions(items);
  } catch (err) {
    clearAndEmpty(tbody, err.message);
  }
}

async function updateIntervention(interventionId, action, payload = null, messageTarget = null) {
  if (!interventionId) return false;
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  const target = messageTarget || byId('appMessage');
  try {
    await api(`/api/v1/customer/interventions/${encodeURIComponent(interventionId)}/${action}`, payload ? { method: 'POST', body: JSON.stringify(payload) } : { method: 'POST' });
    await fetchInterventions();
    setMessage(target, action === 'start' ? t('msg_intervention_started') : action === 'complete' ? t('msg_intervention_completed') : t('msg_intervention_cancelled'));
    return true;
  } catch (err) { setMessage(target, err.message, true); return false; }
}

function openCompleteIntervention(interventionId) {
  const item = (state.interventions || []).find((entry) => entry.intervention_id === interventionId);
  if (!item) return;
  state.interventionToComplete = interventionId;
  byId('completeClientReference').value = item.client_reference || '';
  byId('completeTitle').value = item.title || 'Assistance à distance';
  byId('completeSummary').value = item.summary || '';
  setMessage(byId('completeInterventionMessage'), '');
  byId('completeInterventionDialog').showModal();
}

async function submitCompleteIntervention(event) {
  event.preventDefault(); const submit = event.submitter; submit.disabled = true;
  try {
    const done = await updateIntervention(state.interventionToComplete, 'complete', {
      client_reference: byId('completeClientReference').value.trim(),
      title: byId('completeTitle').value.trim(),
      summary: byId('completeSummary').value.trim()
    }, byId('completeInterventionMessage'));
    if (done) { state.interventionToComplete = ''; byId('completeInterventionDialog').close(); }
  } finally { submit.disabled = false; }
}

// =============================================================================
// VIEWER CODES & REMOTE SUPPORT (Fusion Espace Technicien)
// =============================================================================

function fillCodesLicenseSelect(licenses) {
  const select = byId('codeLicenseSelect');
  if (!select) return;
  select.replaceChildren();
  const activeLics = licenses.filter((item) => item.status === 'active');
  activeLics.forEach((item) => {
    const opt = document.createElement('option');
    opt.value = item.license_id;
    const planName = item.plan ? ('Plan ' + item.plan.charAt(0).toUpperCase() + item.plan.slice(1)) : 'Forfait';
    opt.textContent = `${planName} (${item.max_connections} simultanées)`;
    select.append(opt);
  });
  byId('generateCodeSubmit').disabled = !activeLics.length;
}

async function fetchViewerCodes() {
  const tbody = byId('viewerCodesBody');
  if (!tbody) return;
  try {
    const response = await api('/api/v1/customer/viewer-codes');
    const data = await response.json();
    renderViewerCodes(data.codes || []);
  } catch (err) {
    clearAndEmpty(tbody, err.message);
  }
}

function renderViewerCodes(codes) {
  const tbody = byId('viewerCodesBody');
  if (!tbody) return;
  tbody.replaceChildren();
  const isEn = window.RdI18n && window.RdI18n.getLang() === 'en';

  if (!codes.length) {
    const row = tbody.insertRow();
    const cell = row.insertCell();
    cell.colSpan = 8;
    cell.className = 'empty';
    cell.textContent = isEn ? 'No access codes generated yet.' : 'Aucun code d\'accès généré pour l\'instant.';
    return;
  }

  codes.forEach((item) => {
    const row = document.createElement('tr');

    // Code
    const codeCell = document.createElement('td');
    const codeStrong = document.createElement('strong');
    codeStrong.style.fontFamily = 'monospace';
    codeStrong.style.fontSize = '15px';
    codeStrong.style.color = '#38bdf8';
    codeStrong.textContent = item.code;
    codeCell.append(codeStrong);
    row.append(codeCell);

    // Client
    const clientCell = document.createElement('td');
    clientCell.textContent = item.client_email || '—';
    row.append(clientCell);

    // License ID
    const licCell = document.createElement('td');
    licCell.textContent = item.technician_license_id;
    row.append(licCell);

    // Status badge
    const statusCell = document.createElement('td');
    statusCell.append(badge(item.status));
    row.append(statusCell);

    // Created At
    const createdCell = document.createElement('td');
    createdCell.textContent = formatDate(item.created_at, true);
    row.append(createdCell);

    // Expires At
    const expiresCell = document.createElement('td');
    expiresCell.textContent = formatDate(item.expires_at, true);
    row.append(expiresCell);

    // RustDesk ID
    const rdCell = document.createElement('td');
    rdCell.textContent = item.client_rustdesk_id || '—';
    row.append(rdCell);

    // Action (Revoke)
    const actionCell = document.createElement('td');
    actionCell.style.textAlign = 'right';
    if (item.status === 'active') {
      const btn = document.createElement('button');
      btn.className = 'button secondary';
      btn.style.padding = '5px 10px';
      btn.style.fontSize = '12px';
      btn.textContent = isEn ? 'Revoke' : 'Révoquer';
      btn.addEventListener('click', () => handleRevokeViewerCode(item.code));
      actionCell.append(btn);
    } else {
      actionCell.textContent = '—';
    }
    row.append(actionCell);

    tbody.append(row);
  });
}

async function handleGenerateCode(event) {
  event.preventDefault();
  const btn = byId('generateCodeSubmit');
  const msg = byId('generateCodeMessage');
  const isEn = window.RdI18n && window.RdI18n.getLang() === 'en';
  btn.disabled = true;
  setMessage(msg, isEn ? 'Generating code…' : 'Génération en cours…');
  const licenseID = byId('codeLicenseSelect').value;
  const clientEmail = byId('codeClientEmail').value.trim();

  try {
    const response = await api('/api/v1/customer/viewer-codes/generate', {
      method: 'POST',
      body: JSON.stringify({ license_id: licenseID, client_email: clientEmail })
    });
    const data = await response.json();
    setMessage(msg, '');

    byId('displayGeneratedCode').textContent = data.code;
    const expText = isEn
      ? `Valid until ${formatDate(data.expires_at, true)} (12 hours)`
      : `Valable jusqu'au ${formatDate(data.expires_at, true)} (12 heures)`;
    byId('displayGeneratedExpiry').textContent = expText;
    byId('codeResultBox').style.display = 'block';

    state.lastGeneratedCode = data.code;
    await fetchViewerCodes();
  } catch (err) {
    setMessage(msg, err.message, true);
  } finally {
    btn.disabled = false;
  }
}

async function handleRevokeViewerCode(code) {
  const isEn = window.RdI18n && window.RdI18n.getLang() === 'en';
  const confirmMsg = isEn
    ? `Revoke code ${code}? The client will no longer be able to connect.`
    : `Confirmez-vous la révocation du code ${code} ? Le client ne pourra plus se connecter.`;
  if (!confirm(confirmMsg)) return;

  try {
    await api(`/api/v1/customer/viewer-codes/${encodeURIComponent(code)}/revoke`, { method: 'PUT' });
    setMessage(byId('appMessage'), isEn ? `Code ${code} successfully revoked.` : `Code ${code} révoqué avec succès.`);
    await fetchViewerCodes();
  } catch (err) {
    setMessage(byId('appMessage'), err.message, true);
  }
}

// =============================================================================
// FLEET MANAGEMENT & UNATTENDED ACCESS (Console de gestion de parc)
// =============================================================================

function fillEnrollLicenseSelect(licenses) {
  const select = byId('enrollLicenseSelect');
  if (!select) return;
  const selectedLicense = select.value;
  select.replaceChildren();
  const activeLics = (licenses || []).filter((item) => item.status === 'active');
  activeLics.forEach((item) => {
    const opt = document.createElement('option');
    opt.value = item.license_id;
    const planName = item.plan ? ('Plan ' + item.plan.charAt(0).toUpperCase() + item.plan.slice(1)) : 'Forfait';
    opt.textContent = `${planName} (${item.max_connections} simultanées)`;
    select.append(opt);
  });
  if (activeLics.some(item => item.license_id === selectedLicense)) select.value = selectedLicense;
  select.onchange = updateEnrollAvailability;
  if (byId('enrollSubmitBtn')) {
    byId('enrollSubmitBtn').disabled = !activeLics.length;
  }
  updateEnrollAvailability();
}

function updateEnrollAvailability() {
  const select = byId('enrollLicenseSelect');
  const button = byId('enrollSubmitBtn');
  if (!select || !button) return;
  const quota = (state.deviceQuotas || []).find(item => item.license_id === select.value);
  button.disabled = !select.value || (quota && !quota.can_enroll);
}

function renderFleetQuotas() {
  const container = byId('fleetQuotaSummary');
  if (!container) return;
  container.replaceChildren();
  const quotas = state.deviceQuotas || [];
  container.hidden = !quotas.length;
  const lang = window.RdI18n?.getLang() || 'fr';
  const labels = {
    fr: ['Quota du parc par forfait', 'places utilisées', 'réservées par des codes en attente', 'restantes'],
    en: ['Fleet quota per plan', 'slots used', 'reserved by pending codes', 'remaining'],
    de: ['Gerätekontingent pro Tarif', 'Plätze belegt', 'durch ausstehende Codes reserviert', 'verbleibend'],
    es: ['Cuota de dispositivos por plan', 'plazas utilizadas', 'reservadas por códigos pendientes', 'restantes'],
    it: ['Quota dispositivi per piano', 'posti utilizzati', 'riservati da codici in attesa', 'rimanenti'],
    ru: ['Лимит устройств на тариф', 'мест занято', 'зарезервировано кодами', 'осталось'],
    pl: ['Limit urządzeń na pakiet', 'wykorzystanych miejsc', 'zarezerwowanych kodami', 'pozostało']
  }[lang] || ['Fleet quota per plan', 'slots used', 'reserved by pending codes', 'remaining'];
  const title = document.createElement('h3');
  title.textContent = labels[0];
  container.append(title);
  quotas.forEach(item => {
    const line = document.createElement('p');
    const planName = item.plan ? ('Plan ' + item.plan.charAt(0).toUpperCase() + item.plan.slice(1)) : 'Forfait';
    line.textContent = `${planName} : ${item.used.toLocaleString(lang)} / ${item.limit.toLocaleString(lang)} ${labels[1]} — ${item.reserved} ${labels[2]} — ${item.remaining.toLocaleString(lang)} ${labels[3]}.`;
    container.append(line);
  });
  updateEnrollAvailability();
}

// --- Folder and Navigation Helpers ---
function getFolderById(id) {
  if (!id) return null;
  return (state.folders || []).find((f) => f.folder_id === id) || null;
}

function getFolderName(id) {
  if (!id) return (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t('root_folder_name') : 'Racine';
  const f = getFolderById(id);
  return f ? f.name : id;
}

function getFolderAncestors(id) {
  const ancestors = [];
  let currentId = id;
  const visited = new Set();
  while (currentId && !visited.has(currentId)) {
    visited.add(currentId);
    const f = getFolderById(currentId);
    if (!f) break;
    ancestors.unshift(f);
    currentId = f.parent_folder_id;
  }
  return ancestors;
}

function getFolderChildren(parentId) {
  const pid = parentId || '';
  return (state.folders || []).filter((f) => (f.parent_folder_id || '') === pid);
}

function countFolderDevices(folderId) {
  const fid = folderId || '';
  return (state.devices || []).filter((d) => (d.folder_id || '') === fid).length;
}

function getFolderTreeOptions(currentDeviceFolderId = '', excludeDescendantsOf = '') {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  const options = [{ id: '', label: t('no_folder_root'), level: 0 }];

  const excluded = new Set();
  if (excludeDescendantsOf) {
    excluded.add(excludeDescendantsOf);
    let added = true;
    while (added) {
      added = false;
      (state.folders || []).forEach((f) => {
        if (f.parent_folder_id && excluded.has(f.parent_folder_id) && !excluded.has(f.folder_id)) {
          excluded.add(f.folder_id);
          added = true;
        }
      });
    }
  }

  function addChildren(parentId, level) {
    const children = (state.folders || []).filter((f) => (f.parent_folder_id || '') === parentId && !excluded.has(f.folder_id));
    children.sort((a, b) => a.name.localeCompare(b.name));
    for (const child of children) {
      const prefix = level > 0 ? '  '.repeat(level) + '↳ 📁 ' : '📁 ';
      options.push({ id: child.folder_id, label: prefix + child.name, level });
      addChildren(child.folder_id, level + 1);
    }
  }

  addChildren('', 0);
  return options;
}

function renderFolderBreadcrumbs() {
  const container = byId('deviceFolderBreadcrumbs');
  if (!container) return;
  container.replaceChildren();
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;

  const rootItem = document.createElement('span');
  rootItem.className = `breadcrumb-item ${state.activeFolderId === '' && !state.viewAllDevices ? 'active' : ''}`;
  rootItem.textContent = '🏠 ' + t('root_folder_name');
  rootItem.addEventListener('click', () => {
    state.activeFolderId = '';
    state.viewAllDevices = false;
    renderFolderBreadcrumbs();
    renderFolderPills();
    updateFolderActionButtons();
    renderDevices(filterDevicesList(state.devices));
  });
  container.append(rootItem);

  if (state.viewAllDevices) {
    const sep = document.createElement('span');
    sep.className = 'breadcrumb-sep';
    sep.textContent = '>';
    const allItem = document.createElement('span');
    allItem.className = 'breadcrumb-item active';
    allItem.textContent = '👁️ ' + t('all_devices_view');
    container.append(sep, allItem);
    return;
  }

  if (state.activeFolderId) {
    const ancestors = getFolderAncestors(state.activeFolderId);
    for (let i = 0; i < ancestors.length; i++) {
      const f = ancestors[i];
      const isLast = i === ancestors.length - 1;
      const sep = document.createElement('span');
      sep.className = 'breadcrumb-sep';
      sep.textContent = '>';
      const crumb = document.createElement('span');
      crumb.className = `breadcrumb-item ${isLast ? 'active' : ''}`;
      crumb.textContent = '📁 ' + f.name;
      if (!isLast) {
        crumb.addEventListener('click', () => {
          state.activeFolderId = f.folder_id;
          state.viewAllDevices = false;
          renderFolderBreadcrumbs();
          renderFolderPills();
          updateFolderActionButtons();
          renderDevices(filterDevicesList(state.devices));
        });
      }
      container.append(sep, crumb);
    }
  }
}

function renderFolderPills() {
  const container = byId('deviceFoldersList');
  if (!container) return;
  container.replaceChildren();
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;

  const viewAllPill = document.createElement('button');
  viewAllPill.type = 'button';
  viewAllPill.className = `folder-pill ${state.viewAllDevices ? 'active' : ''}`;
  const viewAllIcon = document.createTextNode('👁️ ' + t('all_devices_view') + ' ');
  const viewAllCount = document.createElement('span');
  viewAllCount.className = 'folder-pill-count';
  viewAllCount.textContent = state.devices.length;
  viewAllPill.append(viewAllIcon, viewAllCount);
  viewAllPill.addEventListener('click', () => {
    state.viewAllDevices = !state.viewAllDevices;
    renderFolderBreadcrumbs();
    renderFolderPills();
    updateFolderActionButtons();
    renderDevices(filterDevicesList(state.devices));
  });
  container.append(viewAllPill);

  if (state.viewAllDevices) {
    return;
  }

  const children = getFolderChildren(state.activeFolderId);
  children.sort((a, b) => a.name.localeCompare(b.name));

  children.forEach((child) => {
    const pill = document.createElement('button');
    pill.type = 'button';
    pill.className = 'folder-pill';
    const pillText = document.createTextNode('📁 ' + child.name + ' ');
    const countSpan = document.createElement('span');
    countSpan.className = 'folder-pill-count';
    countSpan.textContent = countFolderDevices(child.folder_id);
    pill.append(pillText, countSpan);
    pill.addEventListener('click', () => {
      state.activeFolderId = child.folder_id;
      state.viewAllDevices = false;
      renderFolderBreadcrumbs();
      renderFolderPills();
      updateFolderActionButtons();
      renderDevices(filterDevicesList(state.devices));
    });
    container.append(pill);
  });
}

function updateFolderActionButtons() {
  const btnNewFolder = byId('btnNewFolder');
  const btnNewSubFolder = byId('btnNewSubFolder');
  const btnRenameFolder = byId('btnRenameFolder');
  const btnDeleteFolder = byId('btnDeleteFolder');

  const inFolder = Boolean(state.activeFolderId) && !state.viewAllDevices;

  if (btnNewFolder) btnNewFolder.style.display = 'inline-block';
  if (btnNewSubFolder) btnNewSubFolder.style.display = inFolder ? 'inline-block' : 'none';
  if (btnRenameFolder) btnRenameFolder.style.display = inFolder ? 'inline-block' : 'none';
  if (btnDeleteFolder) btnDeleteFolder.style.display = inFolder ? 'inline-block' : 'none';
}

async function fetchDevices(append = false) {
  const tbody = byId('devicesBody');
  if (!tbody || state.devicesLoading) return;
  append = append === true;
  state.devicesLoading = true;
  const session = state.token;
  try {
    const cursor = append ? state.deviceNextCursor : 0;
    const response = await api(`/api/v1/customer/devices?after=${encodeURIComponent(cursor)}`);
    const data = await response.json();
    if (session !== state.token) return;
    state.devices = append ? state.devices.concat(data.devices || []) : (data.devices || []);
    state.deviceNextCursor = Number(data.next_cursor || 0);
    state.deviceQuotas = Array.isArray(data.quotas) ? data.quotas : [];
    if (Array.isArray(data.folders)) {
      state.folders = data.folders;
    } else if (Array.isArray(data.device_folders)) {
      state.folders = data.device_folders;
    } else if (!append) {
      await fetchFolders();
    }
    renderFleetQuotas();
    byId('moreDevicesButton').hidden = !state.deviceNextCursor;
    updateDeviceStats(state.devices);
    renderFolderBreadcrumbs();
    renderFolderPills();
    updateFolderActionButtons();
    renderDevices(filterDevicesList(state.devices));
  } catch (err) {
    setMessage(byId('appMessage'), err.message, true);
  } finally {
    state.devicesLoading = false;
  }
}

async function fetchFolders() {
  try {
    const response = await api('/api/v1/customer/device-folders');
    const data = await response.json();
    const list = Array.isArray(data.folders) ? data.folders : (Array.isArray(data.device_folders) ? data.device_folders : null);
    if (list) {
      state.folders = list;
    }
  } catch (_) {
    /* ignore background error */
  }
}

function updateDeviceStats(devices) {
  const total = devices.length;
  const online = devices.filter((d) => d.status === 'online').length;
  const offline = total - online;

  if (byId('devicesTotalCount')) byId('devicesTotalCount').textContent = total;
  if (byId('devicesOnlineCount')) byId('devicesOnlineCount').textContent = online;
  if (byId('devicesOfflineCount')) byId('devicesOfflineCount').textContent = offline;
}

function filterDevicesList(devices) {
  const searchInput = byId('deviceSearchInput');
  const searchCountBadge = byId('deviceSearchCount');
  const query = (searchInput ? searchInput.value : '').toLowerCase().trim();

  let filtered = devices;

  if (query) {
    filtered = devices.filter((d) => {
      const folderName = getFolderName(d.folder_id).toLowerCase();
      return (d.alias && d.alias.toLowerCase().includes(query)) ||
        (d.hostname && d.hostname.toLowerCase().includes(query)) ||
        (d.device_id && d.device_id.toLowerCase().includes(query)) ||
        (d.rustdesk_id && d.rustdesk_id.toLowerCase().includes(query)) ||
        (d.permanent_code && d.permanent_code.toLowerCase().includes(query)) ||
        (d.os && d.os.toLowerCase().includes(query)) ||
        folderName.includes(query);
    });

    if (searchCountBadge) {
      searchCountBadge.style.display = 'inline-block';
      const isPlural = filtered.length > 1;
      searchCountBadge.textContent = `${filtered.length} ${isPlural ? 'résultats' : 'résultat'}`;
    }
    return filtered;
  }

  if (searchCountBadge) {
    searchCountBadge.style.display = 'none';
  }

  if (state.viewAllDevices) {
    return devices;
  }

  const currentFid = state.activeFolderId || '';
  return devices.filter((d) => (d.folder_id || '') === currentFid);
}

function renderOSBadge(osRaw) {
  const badge = document.createElement('span');
  const os = (osRaw || '').toLowerCase();
  let label = osRaw || '—';
  let osClass = 'os-generic';
  let svgHTML = '';

  if (os.includes('win')) {
    label = 'Windows';
    osClass = 'os-windows';
    // Microsoft Windows 4-quadrant official perspective vector
    svgHTML = '<svg class="os-icon" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M0 2.277L6.685 1.36v6.331H0V2.277zm7.531-1.464L16 0v7.691H7.531V.813zM0 8.441h6.685v6.33L0 13.856V8.441zm7.531 0H16V16l-8.469-.813V8.441z"/></svg>';
  } else if (os.includes('darwin') || os.includes('mac') || os.includes('apple') || os.includes('osx') || os.includes('ios')) {
    label = 'macOS';
    osClass = 'os-apple';
    // Apple monochrome silhouette vector
    svgHTML = '<svg class="os-icon" viewBox="0 0 170 170" fill="currentColor" aria-hidden="true"><path d="M150.37 130.25c-2.45 5.66-5.35 10.87-8.71 15.66-4.58 6.53-8.33 11.05-11.22 13.56-4.48 4.12-9.28 6.23-14.42 6.35-3.69 0-8.14-1.05-13.32-3.18-5.19-2.12-9.97-3.17-14.34-3.17-4.58 0-9.49 1.05-14.75 3.17-5.26 2.13-9.5 3.24-12.74 3.35-4.35.13-9.16-1.9-14.42-6.08-3.7-3.04-7.6-7.79-11.7-14.25-5.78-9.13-10.27-19.14-13.48-30.04-3.21-10.9-4.82-21.37-4.82-31.4 0-12.18 2.87-22.68 8.62-31.5 5.75-8.83 13.3-13.36 22.65-13.6 4.69 0 10.02 1.25 16 3.75 5.98 2.5 10.08 3.81 12.3 3.93 1.98-.12 6.24-1.49 12.77-4.12 6.54-2.62 12.12-3.81 16.75-3.56 12.78.62 22.84 5.25 30.18 13.9-11.23 6.88-16.66 16.27-16.28 28.18.38 9.5 4.13 17.43 11.24 23.8 7.12 6.37 15.54 10.08 25.26 11.13-2.12 6.62-4.63 13.12-7.53 19.5zM119.22 31.84c0-7.38 2.63-14.38 7.88-21 5.26-6.63 11.76-10.63 19.51-12 0 .99.04 1.88.13 2.68 0 7.12-2.73 14.12-8.2 21-5.46 6.87-12.01 10.63-19.64 11.25-.13-.63-.26-1.25-.38-1.93z"/></svg>';
  } else if (os.includes('linux')) {
    label = 'Linux';
    osClass = 'os-linux';
    // Linux Tux silhouette vector
    svgHTML = '<svg class="os-icon" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12.003 2c-2.31 0-4.14 1.83-4.14 4.14 0 .49.09.96.25 1.4-.41.22-.8.51-1.14.86-.77.77-1.25 1.83-1.25 3 0 .76.2 1.47.56 2.08-.88.66-1.46 1.7-1.46 2.89 0 1.99 1.62 3.63 3.63 3.63h.28c.45.62 1.04 1.13 1.74 1.48-.38.25-.8.44-1.25.56-.47.12-.76.6-.64 1.07.1.39.46.66.86.66.07 0 .14-.01.21-.03.74-.2 1.43-.52 2.03-.96.6.44 1.29.76 2.03.96.07.02.14.03.21.03.4 0 .76-.27.86-.66.12-.47-.17-.95-.64-1.07-.45-.12-.87-.31-1.25-.56.7-.35 1.29-.86 1.74-1.48h.28c2.01 0 3.63-1.64 3.63-3.63 0-1.19-.58-2.23-1.46-2.89.36-.61.56-1.32.56-2.08 0-1.17-.48-2.23-1.25-3-.34-.35-.73-.64-1.14-.86.16-.44.25-.91.25-1.4 0-2.31-1.83-4.14-4.14-4.14z"/></svg>';
  } else {
    // Monitor / workstation fallback
    svgHTML = '<svg class="os-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>';
  }

  badge.className = `os-badge ${osClass}`;
  badge.innerHTML = svgHTML;
  const labelSpan = document.createElement('span');
  labelSpan.textContent = label;
  badge.appendChild(labelSpan);
  return badge;
}

function renderDevices(devices) {
  const tbody = byId('devicesBody');
  if (!tbody) return;
  tbody.replaceChildren();
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;

  if (!devices.length) {
    const row = tbody.insertRow();
    const cell = row.insertCell();
    cell.colSpan = 8;
    cell.className = 'empty';
    cell.textContent = t('no_devices_yet');
    return;
  }

  devices.forEach((item) => {
    const row = document.createElement('tr');

    // 1. Status (online / offline)
    const statusCell = document.createElement('td');
    const indicator = document.createElement('span');
    indicator.className = 'status-indicator';
    const dot = document.createElement('span');
    dot.className = `status-dot ${item.status === 'online' ? 'online' : 'offline'}`;
    const statusText = document.createTextNode(item.status === 'online' ? t('devices_stat_online') : t('devices_stat_offline'));
    indicator.append(dot, statusText);
    statusCell.append(indicator);
    row.append(statusCell);

    // 2. Name / Alias & Hostname
    const nameCell = document.createElement('td');
    const aliasTitle = document.createElement('span');
    aliasTitle.className = 'device-alias-title';
    aliasTitle.textContent = item.alias || item.hostname || item.device_id;
    const hostSub = document.createElement('span');
    hostSub.className = 'device-hostname-sub';
    hostSub.textContent = item.hostname ? `${item.hostname} (${item.device_id})` : item.device_id;
    nameCell.append(aliasTitle, hostSub);
    if (item.mac_address) {
      const macSub = document.createElement('span');
      macSub.className = 'device-hostname-sub';
      macSub.style.fontFamily = 'monospace';
      macSub.style.fontSize = '11px';
      macSub.style.opacity = '0.75';
      macSub.textContent = `MAC: ${item.mac_address}`;
      nameCell.append(macSub);
    }
    row.append(nameCell);

    // 3. OS
    const osCell = document.createElement('td');
    osCell.append(renderOSBadge(item.os));
    row.append(osCell);

    // 4. Folder
    const folderCell = document.createElement('td');
    if (item.folder_id) {
      const fTag = document.createElement('span');
      fTag.className = 'device-folder-tag';
      fTag.textContent = '📁 ' + getFolderName(item.folder_id);
      fTag.title = 'Naviguer dans ce dossier';
      fTag.style.cursor = 'pointer';
      fTag.addEventListener('click', () => {
        state.activeFolderId = item.folder_id;
        state.viewAllDevices = false;
        renderFolderBreadcrumbs();
        renderFolderPills();
        updateFolderActionButtons();
        renderDevices(filterDevicesList(state.devices));
      });
      folderCell.append(fTag);
    } else {
      const noFolder = document.createElement('span');
      noFolder.style.color = 'var(--muted)';
      noFolder.style.fontSize = '12px';
      noFolder.textContent = '—';
      folderCell.append(noFolder);
    }
    row.append(folderCell);

    // 5. RustDesk ID
    const rdCell = document.createElement('td');
    const rdCode = document.createElement('strong');
    rdCode.style.fontFamily = 'monospace';
    rdCode.style.fontSize = '14px';
    rdCode.style.color = '#38bdf8';
    rdCode.textContent = item.rustdesk_id || '—';
    rdCell.append(rdCode);
    row.append(rdCell);

    // 6. Permanent Code
    const permCell = document.createElement('td');
    const permCode = document.createElement('span');
    permCode.style.fontFamily = 'monospace';
    permCode.style.fontSize = '13px';
    permCode.style.color = '#a7f3d0';
    const enrollmentLabels = { enrolled: 'fleet_enrolled', pending: 'fleet_pending', expired: 'fleet_expired', reenrollment_required: 'fleet_reenroll' };
    permCode.textContent = t(enrollmentLabels[item.enrollment_state] || 'fleet_reenroll');
    permCell.append(permCode);
    row.append(permCell);

    // 7. Last Activity
    const lastSeenCell = document.createElement('td');
    lastSeenCell.textContent = formatDate(item.last_seen_at || item.created_at, true);
    row.append(lastSeenCell);

    // 8. Actions
    const actionCell = document.createElement('td');

    // 1-Click Connect button (if RustDesk ID is known)
    if (/^\d{6,16}$/.test(item.rustdesk_id || '') && item.enrollment_state === 'enrolled') {
      const connectBtn = document.createElement('button');
      connectBtn.className = 'button btn-connect';
      connectBtn.type = 'button';
      connectBtn.textContent = t('btn_connect');
      connectBtn.title = `ID: ${item.rustdesk_id}`;
      connectBtn.addEventListener('click', async () => {
        copyTextToClipboard(item.rustdesk_id, `ID ${item.rustdesk_id} copié ! Ouverture de l'accès distant...`);
        if (!/^DEV-[A-Z0-9]{4}(?:-[A-Z0-9]{4}){1,3}$/.test(item.device_id)) return;
        try {
          await api(`/api/v1/customer/devices/${encodeURIComponent(item.device_id)}/connect`, { method: 'POST' });
        } catch (_) {}
        if (item.status !== 'online') {
          setMessage(byId('appMessage'), '⚡ Poste éteint / hors ligne : signal de réveil (Wake-on-LAN) envoyé automatiquement. Démarrage en cours (1 à 2 min)…');
          try {
            await api(`/api/v1/customer/devices/${encodeURIComponent(item.device_id)}/wake`, { method: 'POST' });
          } catch (_) {}
        } else {
          setMessage(byId('appMessage'), t('fleet_connect_help'));
        }
        window.location.href = `relaisdesk://connect/${encodeURIComponent(item.device_id)}`;
      });
      actionCell.append(connectBtn);
    }

    // Remote OTA update button (for enrolled devices when update is available)
    if (item.enrollment_state === 'enrolled' && isDeviceUpdateAvailable(item.agent_version)) {
      const updateBtn = document.createElement('button');
      updateBtn.className = 'button secondary';
      updateBtn.style.padding = '6px 10px';
      updateBtn.style.fontSize = '12px';
      updateBtn.textContent = t('btn_update_device');
      updateBtn.title = t('update_device_tooltip');
      updateBtn.addEventListener('click', () => handleUpdateDevice(item));
      actionCell.append(updateBtn);
    }

    // Move button
    const moveBtn = document.createElement('button');
    moveBtn.className = 'button secondary';
    moveBtn.style.padding = '6px 10px';
    moveBtn.style.fontSize = '12px';
    moveBtn.textContent = t('btn_move_device');
    moveBtn.addEventListener('click', () => openMoveDeviceModal(item));
    actionCell.append(moveBtn);

    // Edit button
    const editBtn = document.createElement('button');
    editBtn.className = 'button secondary';
    editBtn.style.padding = '6px 10px';
    editBtn.style.fontSize = '12px';
    editBtn.textContent = t('btn_edit_alias');
    editBtn.addEventListener('click', () => handleEditDeviceAlias(item));
    actionCell.append(editBtn);

    // Delete button
    const delBtn = document.createElement('button');
    delBtn.className = 'button btn-danger';
    delBtn.style.padding = '6px 10px';
    delBtn.style.fontSize = '12px';
    delBtn.textContent = t('btn_delete_device');
    delBtn.addEventListener('click', () => handleDeleteDevice(item.device_id, item.alias || item.hostname));
    actionCell.append(delBtn);

    const actionsWrap = document.createElement('div'); actionsWrap.className = 'device-actions';
    while (actionCell.firstChild) actionsWrap.append(actionCell.firstChild);
    actionCell.append(actionsWrap);
    row.append(actionCell);
    tbody.append(row);
  });
}

function openCreateFolderModal(isSubfolder = false) {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  byId('folderModalMode').value = 'create';
  byId('folderModalId').value = '';
  byId('folderModalParentId').value = isSubfolder ? (state.activeFolderId || '') : '';
  byId('folderModalTitle').textContent = isSubfolder ? t('btn_new_subfolder') : t('btn_new_folder');
  byId('folderModalName').value = '';
  setMessage(byId('folderModalMessage'), '');
  byId('folderModalSubmitBtn').disabled = false;

  const parentGroup = byId('folderModalParentGroup');
  if (parentGroup) {
    if (isSubfolder) {
      parentGroup.style.display = 'block';
      const select = byId('folderModalParentSelect');
      select.replaceChildren();
      const options = getFolderTreeOptions();
      options.forEach((opt) => {
        const o = document.createElement('option');
        o.value = opt.id;
        o.textContent = opt.label;
        if (opt.id === state.activeFolderId) o.selected = true;
        select.append(o);
      });
    } else {
      parentGroup.style.display = 'none';
    }
  }

  byId('folderModal').showModal();
}

function openRenameFolderModal() {
  if (!state.activeFolderId) return;
  const current = getFolderById(state.activeFolderId);
  if (!current) return;
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;

  byId('folderModalMode').value = 'rename';
  byId('folderModalId').value = current.folder_id;
  byId('folderModalParentId').value = current.parent_folder_id || '';
  byId('folderModalTitle').textContent = t('btn_rename_folder');
  byId('folderModalName').value = current.name;
  setMessage(byId('folderModalMessage'), '');
  byId('folderModalSubmitBtn').disabled = false;
  if (byId('folderModalParentGroup')) byId('folderModalParentGroup').style.display = 'none';

  byId('folderModal').showModal();
}

async function handleFolderSubmit(event) {
  event.preventDefault();
  const mode = byId('folderModalMode').value;
  const name = byId('folderModalName').value.trim();
  const msg = byId('folderModalMessage');
  const btn = byId('folderModalSubmitBtn');
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;

  if (!name) {
    setMessage(msg, 'Veuillez saisir un nom de dossier.', true);
    return;
  }

  btn.disabled = true;
  setMessage(msg, 'Enregistrement en cours…');

  try {
    if (mode === 'create') {
      let parentId = byId('folderModalParentId').value;
      if (byId('folderModalParentGroup') && byId('folderModalParentGroup').style.display !== 'none') {
        parentId = byId('folderModalParentSelect').value;
      }
      await api('/api/v1/customer/device-folders', {
        method: 'POST',
        body: JSON.stringify({ name, parent_folder_id: parentId })
      });
      setMessage(byId('appMessage'), t('folder_created_success'));
    } else if (mode === 'rename') {
      const folderId = byId('folderModalId').value;
      await api(`/api/v1/customer/device-folders/${encodeURIComponent(folderId)}`, {
        method: 'PUT',
        body: JSON.stringify({ name })
      });
      setMessage(byId('appMessage'), t('folder_updated_success'));
    }

    byId('folderModal').close();
    await fetchDevices();
  } catch (err) {
    setMessage(msg, err.message, true);
    btn.disabled = false;
  }
}

async function handleDeleteActiveFolder() {
  if (!state.activeFolderId) return;
  const current = getFolderById(state.activeFolderId);
  if (!current) return;
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;

  if (!confirm(t('confirm_delete_folder'))) return;

  try {
    await api(`/api/v1/customer/device-folders/${encodeURIComponent(current.folder_id)}`, {
      method: 'DELETE'
    });
    setMessage(byId('appMessage'), t('folder_deleted_success'));
    state.activeFolderId = current.parent_folder_id || '';
    await fetchDevices();
  } catch (err) {
    setMessage(byId('appMessage'), err.message, true);
  }
}

function openMoveDeviceModal(device) {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  byId('moveDeviceModalId').value = device.device_id;
  const name = device.alias || device.hostname || device.device_id;
  byId('moveDeviceModalDesc').textContent = `${t('select_target_folder')} "${name}" :`;
  setMessage(byId('moveDeviceMessage'), '');
  byId('moveDeviceSubmitBtn').disabled = false;

  const select = byId('moveDeviceSelect');
  select.replaceChildren();
  const options = getFolderTreeOptions(device.folder_id);
  options.forEach((opt) => {
    const o = document.createElement('option');
    o.value = opt.id;
    o.textContent = opt.label;
    if (opt.id === (device.folder_id || '')) o.selected = true;
    select.append(o);
  });

  byId('moveDeviceModal').showModal();
}

async function handleMoveDeviceSubmit(event) {
  event.preventDefault();
  const deviceId = byId('moveDeviceModalId').value;
  const targetFolderId = byId('moveDeviceSelect').value;
  const msg = byId('moveDeviceMessage');
  const btn = byId('moveDeviceSubmitBtn');
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;

  btn.disabled = true;
  setMessage(msg, 'Déplacement en cours…');

  try {
    const device = (state.devices || []).find((d) => d.device_id === deviceId);
    await api(`/api/v1/customer/devices/${encodeURIComponent(deviceId)}`, {
      method: 'PUT',
      body: JSON.stringify({
        alias: device ? (device.alias || '') : '',
        notes: device ? (device.notes || '') : '',
        folder_id: targetFolderId
      })
    });
    byId('moveDeviceModal').close();
    setMessage(byId('appMessage'), t('device_moved_success'));
    await fetchDevices();
  } catch (err) {
    setMessage(msg, err.message, true);
    btn.disabled = false;
  }
}

async function handleEnrollDevice(event) {
  event.preventDefault();
  const btn = byId('enrollSubmitBtn');
  const msg = byId('enrollDialogMessage');
  btn.disabled = true;
  setMessage(msg, 'Génération du code permanent en cours…');

  const licenseID = byId('enrollLicenseSelect').value;
  const alias = byId('enrollAliasInput').value.trim();
  const notes = byId('enrollNotesInput').value.trim();

  try {
    const response = await api('/api/v1/customer/devices/enrollment-code', {
      method: 'POST',
      body: JSON.stringify({ license_id: licenseID, alias, notes })
    });
    const data = await response.json();
    setMessage(msg, '');

    const code = data.device.permanent_code;
    state.lastGeneratedEnrollCode = code;
    byId('displayEnrollCode').textContent = code;
    byId('enrollResultBox').style.display = 'block';

    await fetchDevices();
  } catch (err) {
    setMessage(msg, err.message, true);
  } finally {
    updateEnrollAvailability();
  }
}

async function handleEditDeviceAlias(device) {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  const current = device.alias || device.hostname || '';
  const newAlias = prompt(t('prompt_edit_alias'), current);
  if (newAlias === null || newAlias.trim() === current) return;

  try {
    await api(`/api/v1/customer/devices/${encodeURIComponent(device.device_id)}`, {
      method: 'PUT',
      body: JSON.stringify({ alias: newAlias.trim(), notes: device.notes || '' })
    });
    setMessage(byId('appMessage'), 'Poste mis à jour avec succès.');
    await fetchDevices();
  } catch (err) {
    setMessage(byId('appMessage'), err.message, true);
  }
}

async function handleWakeDevice(device) {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  const name = device.alias || device.hostname || device.device_id;
  if (!device.mac_address) {
    const manualMac = prompt(t('prompt_enter_mac_to_wake'), '');
    if (!manualMac || !manualMac.trim()) return;
    try {
      await api(`/api/v1/customer/devices/${encodeURIComponent(device.device_id)}`, {
        method: 'PUT',
        body: JSON.stringify({ alias: device.alias, notes: device.notes, folder_id: device.folder_id, mac_address: manualMac.trim() })
      });
      device.mac_address = manualMac.trim();
    } catch (err) {
      setMessage(byId('appMessage'), err.message, true);
      return;
    }
  }

  try {
    setMessage(byId('appMessage'), t('waking_device_progress'));
    const response = await api(`/api/v1/customer/devices/${encodeURIComponent(device.device_id)}/wake`, {
      method: 'POST'
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || t('wake_device_failed'));
    }
    const peers = data.online_relay_peers || 0;
    const msg = peers > 0
      ? `${t('wake_success_relayed_prefix')} ${peers} ${t('wake_success_relayed_suffix')}`
      : t('wake_success_no_peers');
    setMessage(byId('appMessage'), `⚡ ${name} : ${msg}`);
  } catch (err) {
    setMessage(byId('appMessage'), err.message, true);
  }
}

function isDeviceUpdateAvailable(currentVersion, targetVersion) {
  const target = targetVersion || (typeof window !== 'undefined' && window.__RELAISDESK_LATEST_VERSION) || '1.0.0';
  if (!currentVersion || typeof currentVersion !== 'string' || !currentVersion.trim()) {
    return false;
  }
  const parse = (v) => {
    const parts = v.trim().replace(/^v/, '').split('.');
    return parts.map(p => parseInt(p, 10) || 0);
  };
  const cur = parse(currentVersion);
  const tgt = parse(target);
  for (let i = 0; i < Math.max(cur.length, tgt.length, 3); i++) {
    const c = cur[i] || 0;
    const t = tgt[i] || 0;
    if (t > c) return true;
    if (t < c) return false;
  }
  return false;
}

async function handleUpdateDevice(device) {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  const name = device.alias || device.hostname || device.device_id;
  const confirmMsg = t('confirm_update_device') || `Programmer la mise à jour automatique à distance pour "${name}" ?`;
  if (!confirm(confirmMsg)) return;

  try {
    setMessage(byId('appMessage'), t('updating_device_progress') || 'Programmation de la mise à jour à distance…');
    const response = await api(`/api/v1/customer/devices/${encodeURIComponent(device.device_id)}/update`, {
      method: 'POST',
      body: JSON.stringify({ target_version: '1.0.0' })
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || 'Impossible de programmer la mise à jour.');
    }
    setMessage(byId('appMessage'), `⬆️ ${name} : ${data.message || 'Mise à jour programmée.'}`);
  } catch (err) {
    setMessage(byId('appMessage'), err.message, true);
  }
}

async function handleDeleteDevice(deviceID, alias) {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  if (!confirm(t('confirm_delete_device'))) return;

  try {
    await api(`/api/v1/customer/devices/${encodeURIComponent(deviceID)}`, { method: 'DELETE' });
    setMessage(byId('appMessage'), `Poste ${alias || deviceID} supprimé avec succès.`);
    await fetchDevices();
  } catch (err) {
    setMessage(byId('appMessage'), err.message, true);
  }
}

function fillLicenseSelect(licenses) {
  const select = byId('interventionLicense'); select.replaceChildren();
  licenses.filter((item) => item.status !== 'revoked').forEach((item) => {
    const option = document.createElement('option');
    option.value = item.license_id;
    const planName = item.plan ? ('Plan ' + item.plan.charAt(0).toUpperCase() + item.plan.slice(1)) : 'Forfait';
    option.textContent = planName;
    select.append(option);
  });
  byId('newInterventionButton').disabled = !select.options.length;
}

function openRenewal(license) {
  state.renewalLicense = license.license_id;
  state.renewalCryptoOrder = null;
  const planName = license.plan ? ('Plan ' + license.plan.charAt(0).toUpperCase() + license.plan.slice(1)) : 'Forfait';
  byId('renewLicenseLabel').textContent = `${planName} — échéance actuelle ${formatDate(license.expires_at)}`;
  byId('renewTerms').checked = false; byId('renewImmediate').checked = false; setMessage(byId('renewMessage'), '');
  byId('renewCryptoRecap').style.display = 'none';
  refreshRenewPaymentMethods();
  byId('renewDialog').showModal();
}

// refreshRenewPaymentMethods shows the crypto options only when the API
// offers them. On any failure crypto stays hidden.
async function refreshRenewPaymentMethods() {
  try {
    const response = await fetch(`${API_BASE_URL}/api/v1/public/payment-methods`, { cache: 'no-store' });
    if (!response.ok) return;
    const methods = await response.json();
    const select = byId('paymentMethod');
    select.querySelectorAll('option').forEach((opt) => {
      if (opt.value === 'crypto_btc') opt.hidden = !methods.crypto_btc;
      if (opt.value === 'crypto_xrp') opt.hidden = !methods.crypto_xrp;
    });
    if (select.selectedOptions.length && select.selectedOptions[0].hidden) select.value = 'stripe';
  } catch (_) { /* crypto stays hidden when unreachable */ }
}

function showRenewCryptoRecap(data) {
  byId('renewCryptoAmount').textContent = data.amount_crypto;
  byId('renewCryptoAsset').textContent = data.asset;
  byId('renewCryptoAddress').textContent = data.pay_address;
  const tagRow = byId('renewCryptoTagRow');
  if (data.dest_tag !== undefined && data.dest_tag !== null) {
    byId('renewCryptoTag').textContent = data.dest_tag;
    tagRow.style.display = 'block';
  } else {
    tagRow.style.display = 'none';
  }
  byId('renewCryptoRate').textContent = `${data.rate_eur} EUR/${data.asset} (OKX)`;
  byId('renewCryptoExpiry').textContent = formatDate(data.expires_at, true);
  byId('renewCryptoRef').textContent = data.order_id;
  byId('renewCryptoNotice').textContent = data.notice || '';
  byId('renewCryptoRecap').style.display = 'grid';
}

async function refreshRenewCryptoQuote() {
  if (!state.renewalCryptoOrder || !state.data || !state.data.email) return;
  const button = byId('renewCryptoRefresh');
  button.disabled = true;
  try {
    const response = await api('/api/v1/public/crypto/quote', { method: 'POST', body: JSON.stringify({ order_id: state.renewalCryptoOrder, email: state.data.email }) });
    showRenewCryptoRecap(await response.json());
    setMessage(byId('renewMessage'), 'Nouveau devis affiché ci-dessus.');
  } catch (error) { setMessage(byId('renewMessage'), error.message, true); } finally { button.disabled = false; }
}

async function submitRenewal(event) {
  event.preventDefault();
  if (!byId('renewTerms').checked) return setMessage(byId('renewMessage'), 'Vous devez accepter les CGV/CGU.', true);
  const submit = event.submitter; submit.disabled = true;
  try {
    const response = await api(`/api/v1/customer/licenses/${encodeURIComponent(state.renewalLicense)}/renew`, { method: 'POST', body: JSON.stringify({ payment_method: byId('paymentMethod').value, terms_version: TERMS_VERSION, terms_accepted: true, immediate_performance_requested: byId('renewImmediate').checked }) });
    const data = await response.json();
    if (data.checkout_url) return location.assign(data.checkout_url);
    if (data.amount_crypto) {
      state.renewalCryptoOrder = data.order_id;
      showRenewCryptoRecap(data);
      setMessage(byId('renewMessage'), 'Un e-mail récapitulatif vient de vous être envoyé. La licence sera prolongée automatiquement à réception du dépôt.');
      await loadDashboard();
      return;
    }
    const instructions = data.instructions;
    setMessage(byId('renewMessage'), `Virement à préparer : ${money(instructions.amount)} — référence obligatoire ${instructions.reference}. Les instructions complètes vous ont été envoyées par e-mail.`);
    await loadDashboard();
  } catch (error) { setMessage(byId('renewMessage'), error.message, true); } finally { submit.disabled = false; }
}

async function createIntervention(event) {
  event.preventDefault(); const submit = event.submitter; submit.disabled = true;
  try {
    await api('/api/v1/customer/interventions', { method: 'POST', body: JSON.stringify({ license_id: byId('interventionLicense').value, client_reference: byId('interventionClient').value.trim(), title: byId('interventionTitle').value.trim() }) });
    byId('interventionDialog').close(); byId('interventionClient').value = ''; await loadDashboard(); switchPanel('interventions');
  } catch (error) { setMessage(byId('interventionMessage'), error.message, true); } finally { submit.disabled = false; }
}

async function downloadProtected(path, filename) {
  try {
    const response = await api(path); const blob = await response.blob(); const href = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = href; link.download = filename; document.body.append(link); link.click(); link.remove(); setTimeout(() => URL.revokeObjectURL(href), 1000);
  } catch (error) { setMessage(byId('appMessage'), error.message, true); }
}

const downloadInvoice = (number) => downloadProtected(`/api/v1/customer/invoices/${encodeURIComponent(number)}/download`, `${number}.pdf`);
const downloadInvoiceCII = (number) => downloadProtected(`/api/v1/customer/invoices/${encodeURIComponent(number)}/cii`, `${number}.xml`);

async function updateReminders() {
  const enabled = byId('remindersToggle').checked;
  try { await api('/api/v1/customer/preferences', { method: 'PUT', body: JSON.stringify({ renewal_reminders_enabled: enabled }) }); state.data.renewal_reminders_enabled = enabled; setMessage(byId('appMessage'), 'Préférence enregistrée.'); }
  catch (error) { byId('remindersToggle').checked = !enabled; setMessage(byId('appMessage'), error.message, true); }
}

function safeCloseDialog(id) {
  // dialog.close() throws InvalidStateError when the dialog is not open:
  // never let that interrupt a logout.
  try {
    const dialog = byId(id);
    if (dialog && dialog.open) dialog.close();
  } catch (_) { /* already closed or missing */ }
}

async function logout(callAPI = true, sessionExpired = false) {
  // Never throws: cleanup must always reach showAuth(), even if a widget
  // or dialog call fails.
  try {
    if (window.rdServices) window.rdServices.clear();
    if (window.rdTeams) window.rdTeams.clear();
    try { cancellationReview.dismiss(); } catch (_) { /* no review pending */ }
    if (callAPI && state.token) { try { await api('/api/v1/customer/logout', { method: 'POST' }); } catch (_) { /* session déjà expirée */ } }
    state.token = ''; state.data = null; state.devices = []; state.deviceNextCursor = 0;
    state.deviceQuotas = []; renderFleetQuotas();
    state.folders = []; state.activeFolderId = ''; state.viewAllDevices = false;
    state.lastGeneratedEnrollCode = ''; state.lastGeneratedCode = '';
    byId('displayEnrollCode').textContent = '----';
    byId('enrollResultBox').style.display = 'none';
    safeCloseDialog('enrollDeviceDialog');
    safeCloseDialog('setup2FADialog');
    safeCloseDialog('disable2FADialog');
    safeCloseDialog('recoveryCodesDialog');
    safeCloseDialog('folderModal');
    safeCloseDialog('moveDeviceModal');
    byId('devicesBody').replaceChildren();
  } finally {
    try { sessionStorage.removeItem('rd_customer_token'); } catch (_) { /* stockage indisponible */ }
    if (sessionExpired) setMessage(byId('authMessage'), 'Votre session a expiré ou a été fermée. Veuillez vous reconnecter.', true);
    showAuth();
  }
}

function getPanelTitles() {
  const t = (window.RdI18n && window.RdI18n.t) ? window.RdI18n.t : (k) => k;
  return {
    overview: t('nav_overview'),
    codes: t('nav_codes'),
    devices: t('nav_devices'),
    team: t('nav_team') || 'Équipe & techniciens',
    'team-devices': t('nav_team_devices') || 'Mes postes autorisés',
    licenses: t('nav_licenses'),
    billing: t('nav_billing'),
    interventions: t('nav_interventions'),
    services: 'Prestations & paiements clients',
    settings: t('nav_settings')
  };
}

function switchPanel(name) {
  if (name === 'services' && window.rdServices) window.rdServices.load();
  if (window.rdTeams && !window.rdTeams.canOpen(name)) return;
  state.currentPanel = name;
  try {
    if (history.replaceState) {
      history.replaceState(null, '', '#' + name);
    }
  } catch (_) {}
  document.querySelectorAll('.panel').forEach((panel) => panel.classList.toggle('active', panel.id === `panel-${name}`));
  document.querySelectorAll('.nav-item').forEach((item) => item.classList.toggle('active', item.dataset.panel === name));
  const titles = getPanelTitles();
  byId('pageTitle').textContent = titles[name] || name;

  const isCompact = (name === 'codes' || name === 'devices');
  byId('pageTitle').style.display = isCompact ? 'none' : '';
  const topbar = document.querySelector('.topbar');
  if (topbar) topbar.classList.toggle('compact', isCompact);

  if (name === 'codes') {
    fetchViewerCodes();
  } else if (name === 'devices') {
    fetchDevices();
  } else if (name === 'interventions') {
    fetchInterventions();
  } else if (name === 'settings') {
    fetch2FAStatus();
  }
}

byId('passwordLoginForm').addEventListener('submit', handlePasswordLogin);
byId('showForgotButton').addEventListener('click', showForgotPasswordView);
byId('forgotPasswordForm').addEventListener('submit', handleForgotPassword);
byId('backToLoginButton').addEventListener('click', showPasswordLoginView);
byId('resetPasswordForm').addEventListener('submit', handleResetPassword);
byId('changePasswordForm').addEventListener('submit', handleChangePassword);

// 2FA Auth & Setup listeners
if (byId('totpChallengeForm')) byId('totpChallengeForm').addEventListener('submit', handleTOTPChallenge);
if (byId('btnSend2FAEmailCode')) byId('btnSend2FAEmailCode').addEventListener('click', handleSend2FAEmailCode);
if (byId('backToPasswordLoginButton')) byId('backToPasswordLoginButton').addEventListener('click', showPasswordLoginView);
if (byId('enable2FAAtReset')) byId('enable2FAAtReset').addEventListener('change', handleToggle2FAAtReset);

if (byId('btnStart2FASetup')) byId('btnStart2FASetup').addEventListener('click', start2FASetup);
if (byId('closeSetup2FADialog')) byId('closeSetup2FADialog').addEventListener('click', () => byId('setup2FADialog').close());
if (byId('setup2FAForm')) byId('setup2FAForm').addEventListener('submit', handleConfirm2FAActivation);

if (byId('btnDisable2FA')) byId('btnDisable2FA').addEventListener('click', openDisable2FADialog);
if (byId('closeDisable2FADialog')) byId('closeDisable2FADialog').addEventListener('click', () => byId('disable2FADialog').close());
if (byId('disable2FAForm')) byId('disable2FAForm').addEventListener('submit', handleConfirmDisable2FA);

if (byId('btnRegenRecoveryCodes')) byId('btnRegenRecoveryCodes').addEventListener('click', handleRegenRecoveryCodes);
if (byId('closeRecoveryCodesDialog')) byId('closeRecoveryCodesDialog').addEventListener('click', () => byId('recoveryCodesDialog').close());
if (byId('btnCopyRegenCodes')) {
  byId('btnCopyRegenCodes').addEventListener('click', () => {
    const codes = byId('regenRecoveryCodesList').textContent;
    copyTextToClipboard(codes, 'Codes de secours copiés dans le presse-papiers !');
  });
}

byId('logoutButton').addEventListener('click', () => { logout(true).catch(() => {}); });
byId('refreshButton').addEventListener('click', loadDashboard);
byId('renewForm').addEventListener('submit', submitRenewal);
byId('renewCryptoRefresh').addEventListener('click', refreshRenewCryptoQuote);
byId('copyRenewCryptoAmount').addEventListener('click', () => copyTextToClipboard(byId('renewCryptoAmount').textContent, 'Montant copié.'));
byId('copyRenewCryptoAddress').addEventListener('click', () => copyTextToClipboard(byId('renewCryptoAddress').textContent, 'Adresse copiée.'));
byId('copyRenewCryptoTag').addEventListener('click', () => copyTextToClipboard(byId('renewCryptoTag').textContent, 'Destination Tag copié.'));
byId('interventionForm').addEventListener('submit', createIntervention);
byId('closeRenewDialog').addEventListener('click', () => byId('renewDialog').close());
byId('closeInterventionDialog').addEventListener('click', () => byId('interventionDialog').close());
byId('newInterventionButton').addEventListener('click', () => { setMessage(byId('interventionMessage'), ''); byId('interventionDialog').showModal(); });
byId('completeInterventionForm').addEventListener('submit', submitCompleteIntervention);
byId('closeCompleteInterventionDialog').addEventListener('click', () => byId('completeInterventionDialog').close());
byId('exportButton').addEventListener('click', () => downloadProtected('/api/v1/customer/interventions/export', 'interventions-relaisdesk.csv'));
if (byId('refreshInterventionsButton')) byId('refreshInterventionsButton').addEventListener('click', fetchInterventions);
byId('remindersToggle').addEventListener('change', updateReminders);
document.querySelectorAll('.nav-item').forEach((item) => item.addEventListener('click', () => switchPanel(item.dataset.panel)));

// Codes generator & action listeners
if (byId('generateCodeForm')) byId('generateCodeForm').addEventListener('submit', handleGenerateCode);
if (byId('refreshCodesButton')) byId('refreshCodesButton').addEventListener('click', fetchViewerCodes);
if (byId('copyCodeOnlyBtn')) {
  byId('copyCodeOnlyBtn').addEventListener('click', () => {
    const code = state.lastGeneratedCode || byId('displayGeneratedCode').textContent;
    copyTextToClipboard(code, 'Code copié dans le presse-papiers !');
  });
}
if (byId('copyClientMessageBtn')) {
  byId('copyClientMessageBtn').addEventListener('click', () => {
    const code = state.lastGeneratedCode || byId('displayGeneratedCode').textContent;
    const lang = (window.RdI18n && window.RdI18n.getLang()) || 'fr';
    const messages = {
      fr: `Bonjour, pour démarrer la session de téléassistance, téléchargez l'application RelaisDesk sur https://relaisdesk.fr et saisissez le code : ${code}`,
      en: `Hello, to start your remote support session, please download the RelaisDesk application from https://relaisdesk.fr and enter this access code: ${code}`,
      de: `Hallo, um Ihre Fernunterstützungssitzung zu starten, laden Sie bitte die RelaisDesk-Anwendung von https://relaisdesk.fr herunter und geben Sie diesen Code ein: ${code}`,
      es: `Hola, para iniciar la sesión de teleasistencia, descargue la aplicación RelaisDesk desde https://relaisdesk.fr e introduzca este código: ${code}`,
      it: `Salve, per avviare la sessione di teleassistenza, scarica l'applicazione RelaisDesk su https://relaisdesk.fr e inserisci questo codice: ${code}`,
      ru: `Здравствуйте! Чтобы начать сеанс удаленной поддержки, скачайте приложение RelaisDesk на https://relaisdesk.fr и введите код: ${code}`,
      pl: `Dzień dobry, aby rozpocząć sesję zdalnego wsparcia, pobierz aplikację RelaisDesk z https://relaisdesk.fr i wprowadź ten kod: ${code}`
    };
    const copiedAlerts = {
      fr: 'Message client copié dans le presse-papiers !',
      en: 'Client instructions copied to clipboard!',
      de: 'Kundenanweisung in die Zwischenablage kopiert!',
      es: '¡Instrucciones para el cliente copiadas al portapapeles!',
      it: 'Istruzioni per il cliente copiate negli appunti!',
      ru: 'Инструкция для клиента скопирована в буфер обмена!',
      pl: 'Instrukcje dla klienta zostały skopiowane do schowka!'
    };
    const message = messages[lang] || messages.fr;
    copyTextToClipboard(message, copiedAlerts[lang] || copiedAlerts.fr);
  });
}

// Fleet management listeners
if (byId('refreshDevicesButton')) byId('refreshDevicesButton').addEventListener('click', fetchDevices);
if (byId('openEnrollDialogButton')) {
  byId('openEnrollDialogButton').addEventListener('click', () => {
    byId('enrollAliasInput').value = '';
    byId('enrollNotesInput').value = '';
    setMessage(byId('enrollDialogMessage'), '');
    byId('enrollResultBox').style.display = 'none';
    byId('enrollDeviceDialog').showModal();
  });
}
if (byId('closeEnrollDeviceDialog')) {
  byId('closeEnrollDeviceDialog').addEventListener('click', () => byId('enrollDeviceDialog').close());
}
if (byId('enrollDeviceForm')) {
  byId('enrollDeviceForm').addEventListener('submit', handleEnrollDevice);
}
if (byId('copyEnrollCodeBtn')) {
  byId('copyEnrollCodeBtn').addEventListener('click', () => {
    const code = state.lastGeneratedEnrollCode || byId('displayEnrollCode').textContent;
    copyTextToClipboard(code, 'Code permanent copié dans le presse-papiers !');
  });
}
if (byId('copyEnrollCmdBtn')) {
  byId('copyEnrollCmdBtn').addEventListener('click', () => {
    const code = state.lastGeneratedEnrollCode || byId('displayEnrollCode').textContent;
    const cmd = `viewer.exe --enroll ${code}`;
    copyTextToClipboard(cmd, 'Commande CLI copiée dans le presse-papiers !');
  });
}
if (byId('deviceSearchInput')) {
  byId('deviceSearchInput').addEventListener('input', () => {
    renderDevices(filterDevicesList(state.devices));
  });
}

// Folder buttons & modal listeners
if (byId('btnNewFolder')) {
  byId('btnNewFolder').addEventListener('click', () => openCreateFolderModal(false));
}
if (byId('btnNewSubFolder')) {
  byId('btnNewSubFolder').addEventListener('click', () => openCreateFolderModal(true));
}
if (byId('btnRenameFolder')) {
  byId('btnRenameFolder').addEventListener('click', openRenameFolderModal);
}
if (byId('btnDeleteFolder')) {
  byId('btnDeleteFolder').addEventListener('click', handleDeleteActiveFolder);
}
if (byId('closeFolderModal')) {
  byId('closeFolderModal').addEventListener('click', () => byId('folderModal').close());
}
if (byId('folderForm')) {
  byId('folderForm').addEventListener('submit', handleFolderSubmit);
}
if (byId('closeMoveDeviceModal')) {
  byId('closeMoveDeviceModal').addEventListener('click', () => byId('moveDeviceModal').close());
}
if (byId('moveDeviceForm')) {
  byId('moveDeviceForm').addEventListener('submit', handleMoveDeviceSubmit);
}
byId('moreDevicesButton').addEventListener('click', () => fetchDevices(true));

// React to language switch
window.addEventListener('relaisdesk:langchange', () => {
  renderFleetQuotas();
  const titles = getPanelTitles();
  byId('pageTitle').textContent = titles[state.currentPanel] || state.currentPanel;
  const isCompact = (state.currentPanel === 'codes' || state.currentPanel === 'devices');
  byId('pageTitle').style.display = isCompact ? 'none' : '';
  const topbar = document.querySelector('.topbar');
  if (topbar) topbar.classList.toggle('compact', isCompact);
  if (state.data) {
    renderSubscriptions(state.data.subscriptions || []);
    renderLicenses(state.data.licenses);
    renderInvoices(state.data.invoices);
    fetchViewerCodes();
    fetchDevices();
  }
});

const queryParams = new URLSearchParams(location.search);
const hashParams = new URLSearchParams(location.hash.slice(1));
const magicToken = queryParams.get('token') || queryParams.get('reset') || hashParams.get('token');

if (hashParams.get('invite')) {
  showAuth();
} else if (magicToken) {
  showAuth();
  showResetPasswordView(magicToken);
} else if (state.token) {
  loadDashboard();
} else {
  showAuth();
  showPasswordLoginView();
}
