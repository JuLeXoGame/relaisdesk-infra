/**
 * RelaisDesk — Espace Technicien Web Application
 * Application de gestion des sessions et codes viewers RelaisDesk
 */

// Configuration de l'API
const API_BASE_URL = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1'
  ? 'http://localhost:8443'
  : 'https://api.relaisdesk.fr';

// État de l'application
const state = {
  token: null,
  licenseId: null,
  email: null,
  expiresAt: null,
  relaisConfig: {
	serverIp: 'api.relaisdesk.fr',
	rendezvousPort: 21116,
	relayPort: 21117,
	publicKey: ''
  },
  codes: [],
	codeToDelete: null,
	interventions: [],
	interventionToComplete: null,
	parkTokens: []
};

// =============================================================================
// Initialisation & Cycle de Vie
// =============================================================================
document.addEventListener('DOMContentLoaded', () => {
  initServiceWorker();
  initEventListeners();
  checkAutoLogin();
});

function initServiceWorker() {
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('sw.js?v=1')
      .catch((err) => console.log('SW non enregistré:', err));
  }
}

function initEventListeners() {
  document.addEventListener('click', (event) => {
    const trigger = event.target.closest('[data-action]');
    if (!trigger) return;

    const action = trigger.dataset.action;
    const code = trigger.dataset.code || '';
    if (action === 'switch-tab') switchTab(trigger.dataset.tab || 'overview');
    if (action === 'copy-code') copyToClipboard(code, `Code ${code} copié !`);
    if (action === 'message-code') openClientMsgModal(code);
    if (action === 'delete-code') promptDeleteCode(code);
	if (action === 'start-intervention') updateIntervention(trigger.dataset.intervention, 'start');
	if (action === 'complete-intervention') openCompleteIntervention(trigger.dataset.intervention);
	if (action === 'cancel-intervention') updateIntervention(trigger.dataset.intervention, 'cancel');
	if (action === 'revoke-park') revokeParkToken(trigger.dataset.park);
  });

  // Formulaire de connexion
  const loginForm = document.getElementById('techLoginForm');
  if (loginForm) {
    loginForm.addEventListener('submit', handleLogin);
  }

  // Navigation par onglets
  const navItems = document.querySelectorAll('.nav-item');
  navItems.forEach((item) => {
    item.addEventListener('click', (e) => {
      e.preventDefault();
      const tab = item.getAttribute('data-tab');
      switchTab(tab);
      // Fermer menu mobile si ouvert
      document.getElementById('sidebar')?.classList.remove('open');
    });
  });

  // Toggle menu mobile
  const mobileMenuBtn = document.getElementById('mobileMenuBtn');
  if (mobileMenuBtn) {
    mobileMenuBtn.addEventListener('click', () => {
      document.getElementById('sidebar')?.classList.toggle('open');
    });
  }

  // Bouton Actualiser
  const refreshBtn = document.getElementById('refreshBtn');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => {
      fetchDashboard(true);
    });
  }

  // Bouton Déconnexion
  const logoutBtn = document.getElementById('sidebarLogoutBtn');
  if (logoutBtn) {
    logoutBtn.addEventListener('click', handleLogout);
  }

  // Formulaire Génération de Code
  const genForm = document.getElementById('generateCodeForm');
  if (genForm) {
    genForm.addEventListener('submit', handleGenerateCode);
  }

  // Formulaire Token de Parc
  const parkForm = document.getElementById('parkTokenForm');
  if (parkForm) {
    parkForm.addEventListener('submit', handleCreateParkToken);
  }
  const copyParkTokenBtn = document.getElementById('copyParkTokenBtn');
  if (copyParkTokenBtn) {
    copyParkTokenBtn.addEventListener('click', () => {
      const token = document.getElementById('parkTokenDisplay').textContent;
      if (token && token !== '----') {
        copyToClipboard(token, 'Token de parc copié dans le presse-papiers !');
      }
    });
  }
  const copyParkCmdBtn = document.getElementById('copyParkCmdBtn');
  if (copyParkCmdBtn) {
    copyParkCmdBtn.addEventListener('click', () => {
      const token = document.getElementById('parkTokenDisplay').textContent;
      if (token && token !== '----') {
        copyToClipboard(`RelaisDesk_Setup.exe /S /ENROLLCODE=${token} /PASSWORD=<mdp>`, 'Commande d\u2019installation copiée !');
      }
    });
  }

	const completeForm = document.getElementById('completeInterventionForm');
	if (completeForm) completeForm.addEventListener('submit', completeIntervention);
	document.getElementById('cancelCompleteInterventionBtn')?.addEventListener('click', closeCompleteIntervention);

  // Boutons copie du code généré
  const copyGenCodeBtn = document.getElementById('copyGenCodeBtn');
  if (copyGenCodeBtn) {
    copyGenCodeBtn.addEventListener('click', () => {
      const code = document.getElementById('generatedCodeDisplay').textContent;
      if (code && code !== '----') {
        copyToClipboard(code, `Code ${code} copié dans le presse-papiers !`);
      }
    });
  }

  const copyGenMessageBtn = document.getElementById('copyGenMessageBtn');
  if (copyGenMessageBtn) {
    copyGenMessageBtn.addEventListener('click', () => {
      const code = document.getElementById('generatedCodeDisplay').textContent;
      if (code && code !== '----') {
        openClientMsgModal(code);
      }
    });
  }

  // Filtres de la table des codes
  const searchInput = document.getElementById('codesSearchInput');
  if (searchInput) {
    searchInput.addEventListener('input', filterAndRenderCodes);
  }

  const statusFilter = document.getElementById('codesFilterStatus');
  if (statusFilter) {
    statusFilter.addEventListener('change', filterAndRenderCodes);
  }

  // Modale de confirmation suppression
  const cancelDeleteBtn = document.getElementById('cancelDeleteBtn');
  if (cancelDeleteBtn) {
    cancelDeleteBtn.addEventListener('click', closeDeleteModal);
  }

  const confirmDeleteBtn = document.getElementById('confirmDeleteBtn');
  if (confirmDeleteBtn) {
    confirmDeleteBtn.addEventListener('click', executeDeleteCode);
  }

  // Modale message client
  const closeClientMsgBtn = document.getElementById('closeClientMsgBtn');
  if (closeClientMsgBtn) {
    closeClientMsgBtn.addEventListener('click', closeClientMsgModal);
  }

  const copyClientMsgModalBtn = document.getElementById('copyClientMsgModalBtn');
  if (copyClientMsgModalBtn) {
    copyClientMsgModalBtn.addEventListener('click', () => {
      const text = document.getElementById('clientMsgTextarea').value;
      copyToClipboard(text, 'Instructions client copiées dans le presse-papiers !');
      closeClientMsgModal();
    });
  }

  // Copie chaîne de configuration complète
  const copyFullConfigBtn = document.getElementById('copyFullConfigBtn');
  if (copyFullConfigBtn) {
    copyFullConfigBtn.addEventListener('click', () => {
      const host = state.relaisConfig.serverIp;
      const key = state.relaisConfig.publicKey || '(Non requise)';
	  const configStr = `rendezvous_server = '${host}:21116'\nnat_type = 1\nserial = 0\n\n[options]\ncustom-rendezvous-server = '${host}'\nrelay-server = '${host}'\nkey = '${key}'`;
      copyToClipboard(configStr, 'Paramètres serveur copiés dans le presse-papiers !');
    });
  }
}

// =============================================================================
// Authentification
// =============================================================================
async function handleLogin(e) {
  e.preventDefault();
  const identifier = document.getElementById('techLicenseId').value.trim();
  const secret = document.getElementById('techLicenseKey').value.trim();
  const remember = document.getElementById('rememberMe').checked;

  if (!identifier || !secret) {
    showToast('Veuillez renseigner vos identifiants de connexion.', 'error');
    return;
  }

  try {
    loginBtn.disabled = true;
    loginBtn.textContent = '⏳ Authentification en cours...';

    const payload = identifier.includes('@')
      ? { email: identifier.toLowerCase(), password: secret }
      : { license_id: identifier, license_key: secret };

    const resp = await fetch(`${API_BASE_URL}/api/v1/technician/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      body: JSON.stringify(payload)
    });

    let data = await resp.json();

    if (!resp.ok) {
      throw new Error(data.error || 'Identifiants invalides ou licence expirée.');
    }

    if (data.requires_2fa) {
      let code = null;
      const wantsEmail = window.confirm("Double authentification requise.\n\nSouhaitez-vous recevoir votre code de sécurité par e-mail ?\n(Cliquez sur 'OK' pour recevoir le code par e-mail, ou 'Annuler' pour saisir directement votre code Authenticator)");
      if (wantsEmail) {
        const mailResp = await fetch(`${API_BASE_URL}/api/v1/technician/login/email-code`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
          body: JSON.stringify({ challenge_token: data.challenge_token })
        });
        const mailData = await mailResp.json();
        if (!mailResp.ok || !mailData.success) {
          throw new Error(mailData.error || "Impossible d'envoyer le code par e-mail.");
        }
        showToast(mailData.message || "Code envoyé par e-mail.", 'info');
        code = window.prompt(`Code envoyé à ${mailData.email_masked || 'votre adresse e-mail'}.\nVeuillez saisir le code à 6 chiffres reçu par e-mail :`);
      } else {
        code = window.prompt("Veuillez saisir le code à 6 chiffres de votre application Authenticator :");
      }
      if (!code) {
        throw new Error("Authentification 2FA requise pour accéder au compte.");
      }
      const resp2fa = await fetch(`${API_BASE_URL}/api/v1/technician/login/2fa`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
        body: JSON.stringify({ challenge_token: data.challenge_token, code: code.trim(), remember_device: remember })
      });
      const data2fa = await resp2fa.json();
      if (!resp2fa.ok || !data2fa.valid) {
        throw new Error(data2fa.error || 'Code 2FA invalide.');
      }
      data = data2fa;
    }

    if (!data.valid) {
      throw new Error(data.error || 'Identifiants invalides ou licence expirée.');
    }

    const effectiveLicId = data.license_id || identifier;

    // Sauvegarder dans l'état
    state.token = data.token;
    state.licenseId = effectiveLicId;
    state.email = data.email;
    state.expiresAt = data.expires_at;
    state.relaisConfig = {
	  serverIp: data.server_ip || 'api.relaisdesk.fr',
	  rendezvousPort: data.rendezvous_port || 21116,
	  relayPort: data.relay_port || 21117,
	  publicKey: data.public_key || ''
    };

    // Stockage persistant
    const storage = remember ? localStorage : sessionStorage;
    storage.setItem('rd_tech_token', data.token);
    storage.setItem('rd_tech_lic_id', effectiveLicId);
    storage.setItem('rd_tech_email', data.email || '');
    storage.setItem('rd_tech_expires_at', data.expires_at || '');
    storage.setItem('rd_tech_config', JSON.stringify(state.relaisConfig));

    showToast('Connexion réussie !', 'success');
    showAppScreen();
    await fetchDashboard();
  } catch (err) {
    showToast(err.message, 'error');
  } finally {
    loginBtn.disabled = false;
    loginBtn.textContent = '🚀 Accéder à mon Espace Technicien';
  }
}

async function checkAutoLogin() {
  // Nettoie les clés de licence que les versions antérieures pouvaient avoir
  // conservées côté navigateur. La persistance utilise maintenant uniquement
  // le jeton de session révocable.
  localStorage.removeItem('rd_tech_lic_key');
  sessionStorage.removeItem('rd_tech_lic_key');

  const savedToken = localStorage.getItem('rd_tech_token') || sessionStorage.getItem('rd_tech_token');
  const savedLicId = localStorage.getItem('rd_tech_lic_id') || sessionStorage.getItem('rd_tech_lic_id');
  const savedEmail = localStorage.getItem('rd_tech_email') || sessionStorage.getItem('rd_tech_email');
  const savedExpiresAt = localStorage.getItem('rd_tech_expires_at') || sessionStorage.getItem('rd_tech_expires_at');
  const savedConfig = localStorage.getItem('rd_tech_config') || sessionStorage.getItem('rd_tech_config');

  if (savedToken && savedLicId) {
    document.getElementById('techLicenseId').value = savedLicId;
    state.token = savedToken;
    state.licenseId = savedLicId;
    state.email = savedEmail || 'Technicien';
    state.expiresAt = savedExpiresAt || '';
    try {
      state.relaisConfig = savedConfig ? JSON.parse(savedConfig) : state.relaisConfig;
    } catch (_) {
      // Keep safe defaults if local metadata was corrupted.
    }
    showAppScreen();
    await fetchDashboard();
  }
}

async function handleLogout() {
  if (state.token) {
    try {
      await fetch(`${API_BASE_URL}/api/v1/technician/logout`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${state.token}` }
      });
    } catch (e) {
      // Ignorer
    }
  }

  localStorage.removeItem('rd_tech_token');
  localStorage.removeItem('rd_tech_lic_id');
  localStorage.removeItem('rd_tech_lic_key');
  localStorage.removeItem('rd_tech_email');
  localStorage.removeItem('rd_tech_expires_at');
  localStorage.removeItem('rd_tech_config');
  sessionStorage.removeItem('rd_tech_token');
  sessionStorage.removeItem('rd_tech_lic_id');
  sessionStorage.removeItem('rd_tech_lic_key');
  sessionStorage.removeItem('rd_tech_email');
  sessionStorage.removeItem('rd_tech_expires_at');
  sessionStorage.removeItem('rd_tech_config');

  state.token = null;
  state.licenseId = null;
  state.email = null;
  state.codes = [];

  document.getElementById('appScreen').style.display = 'none';
  document.getElementById('authScreen').style.display = 'flex';
  showToast('Déconnexion effectuée.', 'info');
}

function showAppScreen() {
  document.getElementById('authScreen').style.display = 'none';
  document.getElementById('appScreen').style.display = 'flex';

  // Mise à jour de la sidebar et de l'onglet config
  document.getElementById('sidebarTechEmail').textContent = state.email || 'Technicien';
  document.getElementById('sidebarTechLicense').textContent = state.licenseId || '';
  document.getElementById('infoTechEmail').textContent = state.email || '-';
  document.getElementById('infoTechLicenseId').textContent = state.licenseId || '-';
  document.getElementById('infoTechExpiresAt').textContent = state.expiresAt || '-';

  document.getElementById('cfgServerIP').textContent = state.relaisConfig.serverIp;
  document.getElementById('cfgRendezvousPort').textContent = state.relaisConfig.rendezvousPort;
  document.getElementById('cfgRelayPort').textContent = state.relaisConfig.relayPort;
  document.getElementById('cfgPublicKey').textContent = state.relaisConfig.publicKey || '(Non requise / Automatique)';
}

// =============================================================================
// Dashboard & Gestion des Données
// =============================================================================
async function fetchDashboard(showSuccessToast = false) {
  if (!state.token) return;

  try {
    const resp = await fetch(`${API_BASE_URL}/api/v1/technician/dashboard`, {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${state.token}`,
        'Accept': 'application/json'
      }
    });

    if (resp.status === 401) {
      handleLogout();
      throw new Error('Session expirée, veuillez vous reconnecter.');
    }

    const data = await resp.json();
    if (!resp.ok) {
      throw new Error(data.error || 'Erreur lors du chargement du tableau de bord.');
    }

    if (data.restricted_to_folders) {
      // Folder-based accounts use the shared customer application's restricted view.
      window.location.replace('../client/#team-devices');
      return;
    }
    // Mise à jour des statistiques
    document.getElementById('statTotalCodes').textContent = data.total_codes || 0;
    document.getElementById('statActiveCodes').textContent = data.active_codes || 0;
    document.getElementById('statExpiredCodes').textContent = data.expired_codes || 0;

    state.codes = data.codes || [];

    // Rendu des tableaux
    renderOverviewCodes();
    filterAndRenderCodes();
	await fetchInterventions();
	await fetchParkTokens();

    if (showSuccessToast) {
      showToast('Données actualisées avec succès.', 'success');
    }
  } catch (err) {
    showToast(err.message, 'error');
  }
}

async function fetchInterventions() {
	const resp = await fetch(`${API_BASE_URL}/api/v1/technician/interventions`, {
	  headers: { 'Authorization': `Bearer ${state.token}`, 'Accept': 'application/json' }
	});
	const data = await resp.json();
	if (!resp.ok) throw new Error(data.error || 'Erreur lors du chargement des interventions.');
	state.interventions = data.interventions || [];
	renderInterventions();
}

function interventionStatus(item) {
	const labels = { planned: 'Planifiée', client_ready: 'Client prêt', in_progress: 'En cours', completed: 'Terminée', cancelled: 'Annulée' };
	const klass = item.status === 'completed' ? 'active' : (item.status === 'cancelled' ? 'revoked' : 'expired');
	return `<span class="badge-status ${klass}">${escapeHtml(labels[item.status] || item.status)}</span>`;
}

function renderInterventions() {
	const tbody = document.getElementById('interventionsTbody');
	if (!tbody) return;
	if (!state.interventions.length) {
	  tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:2rem;">Aucune intervention enregistrée.</td></tr>';
	  return;
	}
	tbody.innerHTML = state.interventions.map((item) => {
	  let actions = '';
	  if (item.status === 'planned' || item.status === 'client_ready') actions = `<button class="btn btn-primary btn-sm" data-action="start-intervention" data-intervention="${escapeHtml(item.intervention_id)}">Démarrer</button>`;
	  if (item.status === 'in_progress') actions = `<button class="btn btn-primary btn-sm" data-action="complete-intervention" data-intervention="${escapeHtml(item.intervention_id)}">Clôturer</button>`;
	  if (item.status !== 'completed' && item.status !== 'cancelled') actions += ` <button class="btn btn-danger btn-sm" data-action="cancel-intervention" data-intervention="${escapeHtml(item.intervention_id)}">Annuler</button>`;
	  const duration = item.duration_minutes == null ? '-' : `${Number(item.duration_minutes)} min`;
	  return `<tr><td><span class="code-pill">${escapeHtml(item.intervention_id)}</span></td><td>${escapeHtml(item.client_reference || '-')}</td><td>${escapeHtml(item.title)}</td><td>${escapeHtml(item.summary || '-')}</td><td>${interventionStatus(item)}</td><td>${escapeHtml(formatDateTime(item.started_at || item.created_at))}</td><td>${escapeHtml(duration)}</td><td style="text-align:right;"><div class="table-actions" style="justify-content:flex-end;">${actions}</div></td></tr>`;
	}).join('');
}

async function updateIntervention(interventionId, action, payload = null) {
	if (!interventionId) return;
	try {
	  const options = { method: 'POST', headers: { 'Authorization': `Bearer ${state.token}`, 'Accept': 'application/json' } };
	  if (payload) { options.headers['Content-Type'] = 'application/json'; options.body = JSON.stringify(payload); }
	  const resp = await fetch(`${API_BASE_URL}/api/v1/technician/interventions/${encodeURIComponent(interventionId)}/${action}`, options);
	  const data = await resp.json();
	  if (!resp.ok) throw new Error(data.error || 'Action impossible.');
	  await fetchInterventions();
	  showToast(action === 'start' ? 'Intervention démarrée.' : action === 'complete' ? 'Intervention clôturée.' : 'Intervention annulée.', 'success');
	  return true;
	} catch (err) { showToast(err.message, 'error'); return false; }
}

function openCompleteIntervention(interventionId) {
	const item = state.interventions.find((entry) => entry.intervention_id === interventionId);
	if (!item) return;
	state.interventionToComplete = interventionId;
	document.getElementById('completeClientReference').value = item.client_reference || '';
	document.getElementById('completeTitle').value = item.title || 'Assistance à distance';
	document.getElementById('completeSummary').value = item.summary || '';
	document.getElementById('completeInterventionModal').classList.add('active');
}

function closeCompleteIntervention() {
	state.interventionToComplete = null;
	document.getElementById('completeInterventionModal').classList.remove('active');
}

async function completeIntervention(event) {
	event.preventDefault();
	const interventionId = state.interventionToComplete;
	if (!interventionId) return;
	const completed = await updateIntervention(interventionId, 'complete', {
	  client_reference: document.getElementById('completeClientReference').value.trim(),
	  title: document.getElementById('completeTitle').value.trim(),
	  summary: document.getElementById('completeSummary').value.trim()
	});
	if (completed) closeCompleteIntervention();
}

// Génération d'un code client
async function handleGenerateCode(e) {
  e.preventDefault();
  const btn = document.getElementById('generateBtn');
  const emailInput = document.getElementById('clientEmailInput');
  const clientEmail = emailInput.value.trim();

  try {
    btn.disabled = true;
    btn.textContent = '⚡ Génération...';

    const resp = await fetch(`${API_BASE_URL}/api/v1/technician/viewer-codes/generate`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${state.token}`,
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      body: JSON.stringify({ client_email: clientEmail })
    });

    const data = await resp.json();

    if (!resp.ok) {
      throw new Error(data.error || 'Erreur lors de la génération du code.');
    }

    const newCode = data.code;

    // Afficher la box résultat
    const resultBox = document.getElementById('genResultBox');
    const codeDisplay = document.getElementById('generatedCodeDisplay');
    codeDisplay.textContent = newCode;
    resultBox.style.display = 'flex';

    // Copie automatique
    await copyToClipboard(newCode, `✨ Code ${newCode} généré et copié dans le presse-papiers !`);

    emailInput.value = '';
    await fetchDashboard();
  } catch (err) {
    showToast(err.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = '⚡ Générer le Code d\'Accès';
  }
}

// Rendu des codes récents (Vue d'ensemble)
function renderOverviewCodes() {
  const tbody = document.getElementById('overviewCodesTbody');
  if (!tbody) return;

  if (state.codes.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="5" style="text-align: center; color: var(--text-muted); padding: 2rem;">
          Aucun code d'accès généré pour le moment.
        </td>
      </tr>
    `;
    return;
  }

  // 5 plus récents
  const recent = state.codes.slice(0, 5);
  tbody.innerHTML = recent.map((c) => createCodeTableRow(c, false)).join('');
}

// Rendu et filtrage de la liste complète des codes
function filterAndRenderCodes() {
  const tbody = document.getElementById('allCodesTbody');
  if (!tbody) return;

  const query = (document.getElementById('codesSearchInput')?.value || '').toLowerCase().trim();
  const statusFilter = document.getElementById('codesFilterStatus')?.value || 'all';

  let filtered = state.codes.filter((c) => {
    // Filtre texte
    const matchText = (c.code || '').toLowerCase().includes(query) ||
                      (c.client_email || '').toLowerCase().includes(query);

    // Filtre statut
    let matchStatus = true;
    const computedStatus = getComputedStatus(c);
    if (statusFilter !== 'all') {
      matchStatus = computedStatus === statusFilter;
    }

    return matchText && matchStatus;
  });

  if (filtered.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="6" style="text-align: center; color: var(--text-muted); padding: 2rem;">
          Aucune session ne correspond à vos critères.
        </td>
      </tr>
    `;
    return;
  }

  tbody.innerHTML = filtered.map((c) => createCodeTableRow(c, true)).join('');
}

function getComputedStatus(c) {
  if (c.status === 'revoked' || (!c.is_active && c.status !== 'expired')) {
    return 'revoked';
  }
  if (c.status === 'expired') {
    return 'expired';
  }
  return 'active';
}

function createCodeTableRow(c, showCreatedDate = true) {
  const computedStatus = getComputedStatus(c);
  let statusBadge = '<span class="badge-status active">🟢 Actif (12h)</span>';
  if (computedStatus === 'expired') {
    statusBadge = '<span class="badge-status expired">⏳ Expiré</span>';
  } else if (computedStatus === 'revoked') {
    statusBadge = '<span class="badge-status revoked">🚫 Révoqué</span>';
  }

  const clientName = c.client_email ? escapeHtml(c.client_email) : '<em style="color: var(--text-muted);">Anonyme</em>';
  const createdDate = c.created_at ? formatDateTime(c.created_at) : '-';
  const expiresDate = c.expires_at ? formatDateTime(c.expires_at) : '-';

  return `
    <tr>
      <td><span class="code-pill">${escapeHtml(c.code)}</span></td>
      <td><strong>${clientName}</strong></td>
      <td>${statusBadge}</td>
      ${showCreatedDate ? `<td><span style="color: var(--text-secondary); font-size: 0.85rem;">${escapeHtml(createdDate)}</span></td>` : ''}
      <td><span style="color: var(--text-secondary); font-size: 0.85rem;">${escapeHtml(expiresDate)}</span></td>
      <td style="text-align: right;">
        <div class="table-actions" style="justify-content: flex-end;">
          <button class="btn btn-secondary btn-sm" data-action="copy-code" data-code="${escapeHtml(c.code)}" title="Copier le code">
            📋 Copier
          </button>
          <button class="btn btn-secondary btn-sm" data-action="message-code" data-code="${escapeHtml(c.code)}" title="Message client">
            📩 Message
          </button>
          <button class="btn btn-danger btn-sm" data-action="delete-code" data-code="${escapeHtml(c.code)}" title="Supprimer / Révoquer">
            ❌
          </button>
        </div>
      </td>
    </tr>
  `;
}

// =============================================================================
// Suppression et Révocation
// =============================================================================
window.promptDeleteCode = function(code) {
  const item = state.codes.find((c) => c.code === code);
  if (!item) return;

  state.codeToDelete = item;
  const status = getComputedStatus(item);

  if (status === 'active') {
    const clientStr = item.client_email ? `(${escapeHtml(item.client_email)})` : '';
    document.getElementById('deleteModalTitle').textContent = '⚠️ Confirmer la révocation et suppression';
    document.getElementById('deleteModalBody').innerHTML = `
      Le code d'accès <strong>${escapeHtml(item.code)}</strong> ${clientStr} est actuellement <strong>ACTIF</strong>.<br><br>
      Voulez-vous vraiment <strong>RÉVOQUER</strong> immédiatement l'accès du client et <strong>SUPPRIMER</strong> cette entrée de votre liste ?
    `;
    document.getElementById('confirmDeleteBtn').textContent = '🚫 Révoquer et Supprimer';
    document.getElementById('deleteConfirmModal').classList.add('active');
  } else {
    // Code déjà expiré ou révoqué : suppression directe
    executeDeleteCode();
  }
};

function closeDeleteModal() {
  document.getElementById('deleteConfirmModal').classList.remove('active');
  state.codeToDelete = null;
}

async function executeDeleteCode() {
  if (!state.codeToDelete || !state.token) return;
  const { code } = state.codeToDelete;
  const status = getComputedStatus(state.codeToDelete);

  try {
    // Si actif, révoquer d'abord (coupure d'accès immédiate : un échec bloque tout)
    if (status === 'active') {
      const revokeResp = await fetch(`${API_BASE_URL}/api/v1/technician/viewer-codes/${encodeURIComponent(code)}/revoke`, {
        method: 'PUT',
        headers: { 'Authorization': `Bearer ${state.token}` }
      });
      if (!revokeResp.ok) {
        let msg = 'Erreur lors de la révocation du code.';
        try {
          const revokeData = await revokeResp.json();
          if (revokeData && revokeData.error) msg = revokeData.error;
        } catch (_) { /* corps vide ou non-JSON : message générique */ }
        throw new Error(msg);
      }
    }

    // Supprimer définitivement
    const delResp = await fetch(`${API_BASE_URL}/api/v1/technician/viewer-codes/${encodeURIComponent(code)}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${state.token}` }
    });

    const delData = await delResp.json();
    if (!delResp.ok) {
      throw new Error(delData.error || 'Erreur lors de la suppression du code.');
    }

    closeDeleteModal();
    showToast(`Code ${code} supprimé avec succès.`, 'success');
    await fetchDashboard();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// =============================================================================
// Message Client Modal
// =============================================================================
window.openClientMsgModal = function(code) {
  const msg = `Bonjour,\n\nPour démarrer la session d'assistance à distance avec votre technicien :\n\n1. Rendez-vous sur https://relaisdesk.fr et téléchargez le Viewer gratuit (ou lancez RelaisDesk_Portable.exe)\n2. Saisissez votre code d'accès : ${code}\n\nCe code est sécurisé et valable pendant 12 heures.\n\nCordialement,\nVotre support technique`;
  document.getElementById('clientMsgTextarea').value = msg;
  document.getElementById('clientMsgModal').classList.add('active');
};

function closeClientMsgModal() {
  document.getElementById('clientMsgModal').classList.remove('active');
}

// =============================================================================
// Onglets
// =============================================================================
window.switchTab = function(tabName) {
  document.querySelectorAll('.tab-pane').forEach((pane) => pane.classList.remove('active'));
  document.querySelectorAll('.nav-item').forEach((item) => item.classList.remove('active'));

  const targetPane = document.getElementById(`tab-${tabName}`);
  const targetNav = document.querySelector(`.nav-item[data-tab="${tabName}"]`);

  if (targetPane) targetPane.classList.add('active');
  if (targetNav) targetNav.classList.add('active');

  const titles = {
    overview: 'Tableau de Bord Technicien',
    codes: 'Gestion des Sessions & Codes',
	interventions: 'Historique des Interventions',
    config: 'Paramètres Serveur Relais',
    downloads: 'Espace Téléchargements'
  };

  const titleEl = document.getElementById('pageTitle');
  if (titleEl && titles[tabName]) {
    titleEl.textContent = titles[tabName];
  }
};

// =============================================================================
// Utilitaires
// =============================================================================
window.copyToClipboard = async function(text, successMsg = 'Copié !') {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
    } else {
      const textArea = document.createElement('textarea');
      textArea.value = text;
      textArea.style.position = 'fixed';
      textArea.style.opacity = '0';
      document.body.appendChild(textArea);
      textArea.select();
      document.execCommand('copy');
      document.body.removeChild(textArea);
    }
    showToast(successMsg, 'success');
  } catch (e) {
    showToast('Erreur lors de la copie dans le presse-papiers.', 'error');
  }
};

// =============================================================================
// Tokens de Parc (enrôlement de masse)
// =============================================================================
async function fetchParkTokens() {
  const tbody = document.getElementById('parkTokensTbody');
  try {
    const resp = await fetch(`${API_BASE_URL}/api/v1/technician/device-park-tokens`, {
      headers: { 'Authorization': `Bearer ${state.token}`, 'Accept': 'application/json' }
    });
    if (resp.status === 401) {
      handleLogout();
      throw new Error('Session expirée, veuillez vous reconnecter.');
    }
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || 'Erreur lors du chargement des tokens.');
    state.parkTokens = data.park_tokens || [];
    renderParkTokens();
  } catch (err) {
    if (tbody) {
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:2rem;">${escapeHtml(err.message)}</td></tr>`;
    }
  }
}

function parkTokenStatus(tok) {
  const now = Date.now();
  const expired = tok.expires_at && new Date(tok.expires_at).getTime() < now;
  const exhausted = tok.use_count >= tok.max_uses;
  if (!tok.is_active) return '<span class="badge-status revoked">Révoqué</span>';
  if (expired) return '<span class="badge-status expired">Expiré</span>';
  if (exhausted) return '<span class="badge-status expired">Épuisé</span>';
  return '<span class="badge-status active">Actif</span>';
}

function renderParkTokens() {
  const tbody = document.getElementById('parkTokensTbody');
  if (!tbody) return;
  if (!state.parkTokens.length) {
    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:2rem;">Aucun token de parc. Créez-en un pour une vague de déploiement.</td></tr>';
    return;
  }
  tbody.innerHTML = state.parkTokens.map((tok) => {
    const actions = tok.is_active
      ? `<button class="btn btn-danger btn-sm" data-action="revoke-park" data-park="${tok.id}">Révoquer</button>`
      : '';
    return `<tr><td><span class="code-pill">${escapeHtml(tok.prefix)}…</span></td>` +
      `<td>${escapeHtml(tok.label || '-')}</td>` +
      `<td>${tok.use_count}/${tok.max_uses}</td>` +
      `<td>${escapeHtml(formatDateTime(tok.expires_at))}</td>` +
      `<td>${parkTokenStatus(tok)}</td>` +
      `<td style="text-align:right;"><div class="table-actions" style="justify-content:flex-end;">${actions}</div></td></tr>`;
  }).join('');
}

async function handleCreateParkToken(e) {
  e.preventDefault();
  const btn = document.getElementById('parkCreateBtn');
  const label = document.getElementById('parkLabelInput').value.trim();
  const maxUses = parseInt(document.getElementById('parkMaxUsesInput').value, 10) || 100;
  const ttlDays = parseInt(document.getElementById('parkTtlInput').value, 10) || 30;
  try {
    btn.disabled = true;
    btn.textContent = '🎫 Création...';
    const resp = await fetch(`${API_BASE_URL}/api/v1/technician/device-park-tokens`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${state.token}`,
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      body: JSON.stringify({ label, max_uses: maxUses, ttl_days: ttlDays })
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || 'Erreur lors de la création du token.');
    document.getElementById('parkTokenDisplay').textContent = data.token;
    document.getElementById('parkResultBox').style.display = 'flex';
    await copyToClipboard(data.token, 'Token de parc créé et copié ! Notez-le, il ne sera plus affiché.');
    document.getElementById('parkLabelInput').value = '';
    await fetchParkTokens();
  } catch (err) {
    showToast(err.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = '🎫 Créer le Token';
  }
}

async function revokeParkToken(id) {
  if (!window.confirm('Révoquer ce token ? Les postes déjà enrôlés restent connectés.')) return;
  try {
    const resp = await fetch(`${API_BASE_URL}/api/v1/technician/device-park-tokens/${id}/revoke`, {
      method: 'PUT',
      headers: { 'Authorization': `Bearer ${state.token}`, 'Accept': 'application/json' }
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) throw new Error(data.error || 'Erreur lors de la révocation.');
    showToast('Token révoqué.', 'success');
    await fetchParkTokens();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  if (!container) return;

  const icons = {
    success: '✅',
    error: '❌',
    info: 'ℹ️'
  };

  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.innerHTML = `<span>${icons[type] || 'ℹ️'}</span> <span>${escapeHtml(message)}</span>`;

  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateX(40px)';
    toast.style.transition = 'all 0.3s ease';
    setTimeout(() => toast.remove(), 300);
  }, 4000);
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function formatDateTime(isoString) {
  if (!isoString) return '-';
  try {
    const d = new Date(isoString);
    if (isNaN(d.getTime())) return isoString;
    return d.toLocaleString('fr-FR', {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  } catch (e) {
    return isoString;
  }
}
