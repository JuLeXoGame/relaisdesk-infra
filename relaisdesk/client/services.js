'use strict';
(() => {
  let current = null, generation = 0;
  const termsHash = '9f6136a3785dcd03cfc330d28eea3eee2f1c39f71eaf74cd4036f71cf5c296bf';
  const node = (tag, value) => { const n = document.createElement(tag); n.textContent = value; return n; };
  const request = async (suffix = '', method = 'GET', body) => (await api(`/api/v1/customer/service-billing${suffix}`, { method, ...(body !== undefined ? { body: JSON.stringify(body) } : {}) })).json();
  const report = error => setMessage(byId('serviceStatus'), error.message, true);
  function navigation() { byId('servicesNav').hidden = state.data?.customer_role !== 'owner' || !state.data?.licenses?.length; }
  async function load() {
    const version = ++generation, token = state.token;
    try {
      const next = await request();
      if (version !== generation || !token || state.token !== token) return;
      current = next;
      const accepted = next.terms?.accepted_at > 0;
      const configurable = next.available && next.active_license && accepted;
      byId('serviceTermsForm').hidden = accepted;
      byId('serviceTermsAccept').checked = false;
      byId('serviceTermsStatus').textContent = accepted ? `Conditions ${next.terms.version} acceptées le ${new Date(next.terms.accepted_at * 1000).toLocaleString('fr-FR')}.` : 'Acceptez les conditions de cette option avant de configurer Stripe ou de l’activer.';
      setMessage(byId('serviceStatus'), !next.available ? 'Option non activée sur le serveur. Le catalogue est consultable ; aucun nouvel encaissement ne peut être lancé.' : next.merchant.enabled ? 'Prestations activées pour votre entreprise.' : 'Prestations désactivées. Terminez la configuration Stripe avant activation.');
      byId('serviceToggle').textContent = next.merchant.enabled ? 'Désactiver les nouvelles prestations' : 'Activer les prestations';
      byId('serviceToggle').disabled = !next.merchant.enabled && !configurable;
      byId('serviceConnect').disabled = !configurable;
      byId('serviceRateForm').querySelector('button').disabled = !configurable;
      byId('serviceRates').replaceChildren();
      for (const rate of next.rates) {
        const row = node('article', ''); row.className = 'license-card';
        row.append(node('h3', rate.label), node('p', `${money(rate.cents / 100)} ${rate.mode === 'hourly' ? '/ heure connectée' : '— forfait prépayé'}${rate.active ? '' : ' (archivé)'}`));
        if (rate.active && next.available) {
          const archive = node('button', 'Archiver'); archive.type = 'button'; archive.className = 'button secondary';
          archive.addEventListener('click', async () => { archive.disabled = true; try { await request(`/rates/${encodeURIComponent(rate.id)}`, 'DELETE'); await load(); } catch (e) { report(e); archive.disabled = false; } });
          row.append(archive);
        }
        byId('serviceRates').append(row);
      }
      byId('serviceWork').replaceChildren();
      for (const work of next.work) {
        const row = node('article', ''); row.className = 'license-card';
        row.append(node('h3', work.label), node('p', `${work.id} — ${work.target_id}`), node('p', `${(work.connected_ms / 60000).toFixed(2)} min confirmées — ${money(work.amount_cents / 100)} — ${work.state} — ${work.paid ? 'Payé' : 'Non payé'}`));
        const action = (label, suffix) => {
          const button = node('button', label); button.type = 'button'; button.className = 'button secondary';
          button.addEventListener('click', async () => {
            if (suffix === 'finish' && !confirm('Toutes les prises en main de cette prestation sont-elles fermées ? Le montant sera figé.')) return;
            button.disabled = true;
            try { await request(`/work/${encodeURIComponent(work.id)}/${suffix}`, 'POST', {}); await load(); } catch (e) { report(e); button.disabled = false; }
          }); row.append(button);
        };
        if ((work.state === 'open' || work.state === 'prepared') && (work.mode === 'hourly' || work.paid)) action('Clôturer après déconnexion', 'finish');
        if (work.state === 'prepared' && !work.paid && !work.checkout_url) action('Annuler la prestation', 'cancel');
        if (!work.paid && next.available && ((work.mode === 'prepaid' && work.state === 'prepared') || (work.mode === 'hourly' && work.state === 'finished'))) action('Préparer le lien de paiement', 'checkout');
        if (work.checkout_url) {
          const u = new URL(work.checkout_url);
          if (u.protocol === 'https:' && u.host === 'checkout.stripe.com' && !u.username && !u.password) {
            const copy = node('button', 'Copier le lien pour le client'); copy.type = 'button'; copy.className = 'button secondary';
            copy.addEventListener('click', () => copyTextToClipboard(u.href, 'Lien copié. Le client doit payer sans partage d’écran actif, ou sur son téléphone.')); row.append(copy);
          }
        }
        byId('serviceWork').append(row);
      }
      if (!next.work.length) byId('serviceWork').append(node('p', 'Aucune prestation. Sélectionnez « Connexion & Prestation » dans l’application technicien.'));
    } catch (e) { if (version === generation && state.token === token) report(e); }
  }
  byId('serviceRefresh').addEventListener('click', load);
  byId('serviceTermsForm').addEventListener('submit', async event => {
    event.preventDefault();
    if (!current?.terms || !byId('serviceTermsAccept').checked) return;
    const button = event.currentTarget.querySelector('button'); button.disabled = true;
    try {
      if (current.terms.version !== '2026-09-19-prestations-v1' || current.terms.sha256 !== termsHash) throw new Error('Actualisez le portail pour lire les nouvelles conditions avant acceptation.');
      await request('/terms/accept', 'POST', { accepted: true, version: current.terms.version, sha256: current.terms.sha256 });
      await load();
    } catch (e) { report(e); } finally { button.disabled = false; }
  });
  byId('serviceConnect').addEventListener('click', async event => {
    const button = event.currentTarget, token = state.token; button.disabled = true;
    try {
      const result = await request('/onboarding', 'POST', {});
      if (!token || token !== state.token) return;
      const u = new URL(result.url);
      if (u.protocol !== 'https:' || u.host !== 'connect.stripe.com' || u.username || u.password) throw new Error('Lien Stripe refusé.');
      location.assign(u.href);
    } catch (e) { report(e); } finally { button.disabled = false; }
  });
  byId('serviceToggle').addEventListener('click', async event => {
    if (!current) return;
    const button = event.currentTarget; button.disabled = true;
    try { await request('', 'PUT', { enabled: !current.merchant.enabled }); await load(); } catch (e) { report(e); } finally { button.disabled = !current?.merchant.enabled && !(current?.available && current?.active_license && current?.terms?.accepted_at); }
  });
  byId('serviceRateForm').addEventListener('submit', async event => {
    event.preventDefault(); const button = event.currentTarget.querySelector('button'); button.disabled = true;
    try {
      const value = byId('serviceRateAmount').value.trim().replace(',', '.');
      if (!/^\d{1,5}(\.\d{1,2})?$/.test(value)) throw new Error('Montant invalide.');
      const [whole, decimal = ''] = value.split('.'); const cents = Number(whole) * 100 + Number(decimal.padEnd(2, '0'));
      await request('/rates', 'POST', { label: byId('serviceRateLabel').value.trim(), mode: byId('serviceRateMode').value, cents });
      byId('serviceRateForm').reset(); await load();
    } catch (e) { report(e); } finally { button.disabled = !current?.available; }
  });
  window.rdServices = { load, navigation, clear() { generation++; current = null; byId('serviceRates').replaceChildren(); byId('serviceWork').replaceChildren(); byId('serviceStatus').textContent = ''; byId('serviceTermsStatus').textContent = ''; byId('serviceTermsForm').reset(); byId('serviceTermsForm').hidden = true; byId('serviceRateForm').reset(); byId('servicesNav').hidden = true; } };
})();
