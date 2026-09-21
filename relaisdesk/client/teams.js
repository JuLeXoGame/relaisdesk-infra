'use strict';

// Same customer application; team sessions never replace the personal session.
(() => {
  let data = { licenses: [], members: [], memberships: [] };
  let ownerFolders = [], techToken = '', cursor = 0, generation = 0, memberOnly = false, deviceRequest = 0;
  const text = (tag, value, className) => {
    const node = document.createElement(tag); node.textContent = value;
    if (className) node.className = className;
    return node;
  };
  const button = (label, action) => {
    const node = text('button', label, 'button secondary'); node.type = 'button';
    node.addEventListener('click', async () => {
      node.disabled = true;
      try { await action(); } catch (err) { setMessage(byId(byId('appScreen').hidden ? 'authMessage' : 'appMessage'), err.message, true); }
      finally { node.disabled = false; }
    });
    return node;
  };
  const json = async (path, method = 'GET', body) => (await api(path, { method, ...(body ? { body: JSON.stringify(body) } : {}) })).json();
  const commercialPanels = ['overview', 'codes', 'devices', 'licenses', 'billing', 'interventions', 'team', 'services'];
  function applyNavigation() {
    memberOnly = data.memberships.length > 0 && !(state.data?.licenses?.length || state.data?.orders?.length);
    document.querySelectorAll('.nav-item').forEach(node => {
      if (commercialPanels.includes(node.dataset.panel)) node.hidden = memberOnly;
    });
    byId('teamNav').hidden = !data.licenses.some(item => item.enabled);
    byId('teamDevicesNav').hidden = !data.memberships.length;
    if (window.rdServices) window.rdServices.navigation();
    if (memberOnly && commercialPanels.includes(state.currentPanel)) switchPanel('team-devices');
  }
  function renderMembers() {
    const id = byId('teamLicense').value;
    const license = data.licenses.find(item => item.license_id === id);
    byId('teamCapacity').textContent = license ? `${license.used} / ${license.capacity} places occupées (utilisateurs et invitations en cours)` : 'Disponible avec un plan Pro ou supérieur actif.';
    byId('teamInvite').disabled = !license || !license.enabled || license.used >= license.capacity;
    const list = byId('teamMembers'); list.replaceChildren();
    for (const member of data.members.filter(item => item.license_id === id)) {
      const row = text('article', '', 'license-card');
      row.append(text('h3', member.email));
      const expired = member.status === 'invited' && new Date(member.invite_expires_at) <= new Date();
      row.append(text('p', expired ? 'Invitation expirée — vous pouvez inviter à nouveau cette adresse.' : ({ active: 'Actif', invited: 'Invitation en attente', revoked: 'Révoqué' }[member.status] || member.status)));
      const names = member.folder_ids.map(id => ownerFolders.find(f => f.folder_id === id)?.name || id);
      row.append(text('p', names.length ? `Dossiers et sous-dossiers : ${names.join(', ')}` : 'Aucun dossier autorisé'));
      if (member.status !== 'revoked') {
        row.append(button('Modifier les dossiers', () => editMember(member)));
        if (member.status === 'invited' && !expired) row.append(button('Renvoyer', async () => {
          await json(`/api/v1/customer/team/members/${encodeURIComponent(member.member_id)}/resend`, 'POST');
          setMessage(byId('appMessage'), 'Invitation mise en file d’envoi.');
        }));
        row.append(button('Révoquer', async () => {
          if (!confirm(`Retirer l’accès de ${member.email} à cette équipe ? Son compte personnel sera conservé.`)) return;
          await json(`/api/v1/customer/team/members/${encodeURIComponent(member.member_id)}`, 'DELETE'); await load();
        }));
      }
      list.append(row);
    }
    if (!list.children.length) list.append(text('p', 'Aucun utilisateur invité.'));
  }
  function editMember(member = null) {
    const licenseID = byId('teamLicense').value;
    const dialog = document.createElement('dialog'); dialog.id = 'teamInviteDialog';
    const form = document.createElement('form'); form.className = 'dialog-card stack';
    form.append(text('h2', member ? `Droits de ${member.email}` : 'Inviter un technicien'));
    const email = document.createElement('input'); email.type = 'email'; email.required = true; email.maxLength = 254;
    email.id = 'teamInviteEmail'; email.autocomplete = 'email'; email.value = member?.email || ''; email.disabled = !!member;
    const label = text('label', 'Adresse e-mail personnelle'); label.htmlFor = email.id; form.append(label, email);
    const fieldset = document.createElement('fieldset'); fieldset.append(text('legend', 'Dossiers autorisés, sous-dossiers inclus'));
    const choices = ownerFolders.filter(f => !f.license_id || f.license_id === licenseID);
    for (const folder of choices) {
      const line = document.createElement('label'); const check = document.createElement('input');
      check.type = 'checkbox'; check.value = folder.folder_id; check.checked = !!member?.folder_ids.includes(folder.folder_id);
      line.append(check, document.createTextNode(` ${folder.name}`)); fieldset.append(line, document.createElement('br'));
    }
    if (!choices.length) fieldset.append(text('p', 'Créez d’abord les dossiers dans Parc & Postes permanents.'));
    const message = text('p', '', 'message'); message.setAttribute('role', 'status');
    const save = text('button', member ? 'Enregistrer les droits' : 'Envoyer l’invitation', 'button primary'); save.type = 'submit';
    form.append(fieldset, text('p', 'Sans sélection : aucun poste accessible. Maximum 100 dossiers racines.'), message, save, button('Annuler', () => dialog.close()));
    form.addEventListener('submit', async event => {
      event.preventDefault(); save.disabled = true;
      const folder_ids = [...fieldset.querySelectorAll('input:checked')].map(node => node.value);
      try {
        if (member) await json(`/api/v1/customer/team/members/${encodeURIComponent(member.member_id)}`, 'PUT', { folder_ids });
        else await json('/api/v1/customer/team/invitations', 'POST', { license_id: licenseID, email: email.value.trim(), folder_ids });
        dialog.close(); await load();
      } catch (err) { setMessage(message, err.message, true); } finally { save.disabled = false; }
    });
    dialog.append(form); document.body.append(dialog); dialog.addEventListener('close', () => dialog.remove(), { once: true }); dialog.showModal();
  }
  async function load() {
    const requestGeneration = ++generation, personalToken = state.token;
    try {
      const next = await json('/api/v1/customer/team');
      const folders = await json('/api/v1/customer/device-folders');
      if (generation !== requestGeneration || !personalToken || state.token !== personalToken) return;
      data = next; ownerFolders = folders.folders || [];
      const enabledLicenses = data.licenses.filter(l => l.enabled);
      const teamLicenseSelect = byId('teamLicense');
      const teamLicenseLabel = byId('teamLicenseLabel');
      if (teamLicenseSelect) {
        const previous = teamLicenseSelect.value;
        teamLicenseSelect.replaceChildren();
        for (const item of enabledLicenses) {
          const planTitle = item.plan ? ('Plan ' + item.plan.charAt(0).toUpperCase() + item.plan.slice(1) + ' (' + item.capacity + ' techniciens)') : 'Votre forfait';
          teamLicenseSelect.append(new Option(planTitle, item.license_id));
        }
        if (enabledLicenses.some(item => item.license_id === previous)) teamLicenseSelect.value = previous;
        const hideSelector = enabledLicenses.length <= 1;
        teamLicenseSelect.hidden = hideSelector;
        if (teamLicenseLabel) teamLicenseLabel.hidden = hideSelector;
      }
      const teamContextSelect = byId('teamContext');
      if (teamContextSelect) {
        const previous = teamContextSelect.value;
        teamContextSelect.replaceChildren();
        for (const item of data.memberships) {
          const contextTitle = item.owner_email || item.email ? ('Équipe de ' + (item.owner_email || item.email)) : (item.plan ? ('Plan ' + item.plan) : 'Organisation');
          teamContextSelect.append(new Option(contextTitle, item.license_id));
        }
        if (data.memberships.some(item => item.license_id === previous)) teamContextSelect.value = previous;
      }
      applyNavigation(); renderMembers();
      if (data.memberships.length) await loadDevices(false);
    } catch (err) { if (state.token === personalToken) setMessage(byId('appMessage'), `Équipes : ${err.message}`, true); }
  }
  async function loadDevices(append = false) {
    const request = ++deviceRequest;
    const selected = byId('teamContext').value;
    if (!selected) return;
    const currentGeneration = generation;
    if (!append) {
      techToken = ''; cursor = 0; byId('teamDeviceList').replaceChildren(); byId('teamDevicesMore').hidden = true;
      const session = await json('/api/v1/customer/team/technician-session', 'POST', { license_id: selected });
      if (request !== deviceRequest || currentGeneration !== generation || selected !== byId('teamContext').value) return;
      techToken = session.token;
    }
    const response = await fetch(`${API_BASE_URL}/api/v1/technician/devices?after=${cursor}`, { headers: { Authorization: `Bearer ${techToken}` }, cache: 'no-store' });
    if (!response.ok) throw new Error('Accès refusé ou session expirée. Actualisez vos accès.');
    const page = await response.json();
    if (request !== deviceRequest || currentGeneration !== generation || selected !== byId('teamContext').value) return;
    const folders = page.folders || page.device_folders || [];
    for (const device of page.devices || []) {
      const row = text('article', '', 'license-card');
      row.append(text('h3', device.alias || device.hostname || device.device_id));
      row.append(text('p', `${folders.find(f => f.folder_id === device.folder_id)?.name || 'Dossier autorisé'} — ${device.status === 'online' ? 'En ligne' : 'Hors ligne'}`));
      const connect = text('a', 'Ouvrir dans RelaisDesk Technicien', 'button secondary');
      connect.href = `relaisdesk://connect/${encodeURIComponent(device.device_id)}`; row.append(connect); byId('teamDeviceList').append(row);
    }
    cursor = Number(page.next_cursor || 0); byId('teamDevicesMore').hidden = !cursor;
    if (!byId('teamDeviceList').children.length) byId('teamDeviceList').append(text('p', 'Aucun poste accessible. Demandez au propriétaire de vérifier vos dossiers autorisés.'));
  }
  window.rdTeams = {
    load,
    canOpen: name => !(memberOnly && commercialPanels.includes(name)),
    clear: () => { generation++; techToken = ''; data = { licenses: [], members: [], memberships: [] }; memberOnly = false; ownerFolders = []; byId('teamMembers').replaceChildren(); byId('teamDeviceList').replaceChildren(); }
  };
  byId('teamLicense').addEventListener('change', renderMembers);
  byId('teamInvite').addEventListener('click', () => editMember());
  const refreshDevices = append => loadDevices(append).catch(err => setMessage(byId('appMessage'), err.message, true));
  byId('teamContext').addEventListener('change', () => { generation++; refreshDevices(false); });
  byId('teamDevicesRefresh').addEventListener('click', () => refreshDevices(false));
  byId('teamDevicesMore').addEventListener('click', () => refreshDevices(true));

  const invitation = new URLSearchParams(location.hash.slice(1)).get('invite');
  if (invitation) {
    history.replaceState(null, '', location.pathname);
    showAuth(); byId('passwordLoginView').hidden = true;
    const accept = button('Accepter l’invitation à cette équipe', async () => {
      const response = await fetch(`${API_BASE_URL}/api/v1/team/invitations/accept`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token: invitation }), cache: 'no-store' });
      const result = await response.json();
      if (!response.ok) { setMessage(byId('authMessage'), result.error || 'Invitation invalide', true); return; }
      state.token = ''; sessionStorage.removeItem('rd_customer_token');
      accept.remove(); showResetPasswordView(result.token);
      setMessage(byId('authMessage'), `Invitation acceptée pour ${result.email}. Définissez votre mot de passe personnel. Si votre compte est protégé par une double authentification, elle reste requise.`);
    });
    byId('authMessage').before(accept);
    setMessage(byId('authMessage'), 'Ce lien accorde des accès techniques uniquement. Acceptez-le seulement si vous connaissez l’équipe qui vous invite.');
  }
})();
