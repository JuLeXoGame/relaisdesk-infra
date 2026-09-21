'use strict';
(() => {
  const base = ['localhost', '127.0.0.1'].includes(location.hostname) ? 'http://localhost:8443' : 'https://api.relaisdesk.fr';
  const el = id => document.getElementById(id);
  let token = new URLSearchParams(location.hash.slice(1)).get('token') || '';
  if (location.hash) history.replaceState(null, '', location.pathname + location.search);
  const result = new URLSearchParams(location.search).get('result');
  let pricing;
  const message = text => { el('message').textContent = text; };
  const money = value => new Intl.NumberFormat('fr-FR', { style: 'currency', currency: 'EUR' }).format(value);
  async function request(path, body) {
    const response = await fetch(base + path, { method: body ? 'POST' : 'GET', headers: body ? { 'Content-Type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined, cache: 'no-store', referrerPolicy: 'no-referrer' });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || 'Service temporairement indisponible.');
    return data;
  }
  function selection() {
    const plan = el('plan').value;
    const annual = el('billing_cycle').value === 'annual';
    const technicians = plan === 'starter' ? 1 : plan === 'pro' ? 5 : Number(el('technicians').value);
    let price;
    if (plan === 'ultra') {
      const extra = Math.max(0, Math.min(technicians, 50) - 10) * 14 + Math.max(0, Math.min(technicians, 100) - 50) * 10 + Math.max(0, technicians - 100) * 7;
      price = (pricing.ultra.base_monthly + extra) * (annual ? 10 : 1);
    } else price = pricing[plan][annual ? 'price_annual' : 'price_monthly'];
    return { plan, technicians, price, interval: annual ? 'an' : 'mois' };
  }
  function render() {
    const s = selection();
    el('capacityField').hidden = s.plan !== 'ultra';
    el('technicians').disabled = s.plan !== 'ultra';
    el('recurring').checked = false;
    const techs = Math.min(pricing.ultra.max_techs ?? 500, Math.max(pricing.ultra.base_techs ?? 10, s.technicians));
    const devices = s.plan === 'ultra'
      ? (techs === (pricing.ultra.max_techs ?? 500)
        ? (pricing.ultra.max_managed_devices ?? 4500)
        : (pricing.ultra.managed_devices ?? 2000) + (techs - (pricing.ultra.base_techs ?? 10)) * (pricing.ultra.managed_devices_per_extra_technician ?? 5))
      : (pricing[s.plan].managed_devices ?? (s.plan === 'starter' ? 500 : 1000));
    el('summary').textContent = `0 € pendant 30 jours, puis ${money(s.price)}/${s.interval} pour ${s.technicians} technicien(s) simultané(s). Jusqu’à ${devices.toLocaleString('fr-FR')} postes enregistrés dans la console, pendant l’essai et l’abonnement. ${s.interval === 'an' ? 'Le prix annuel est prélevé en une seule fois.' : 'Prélèvement chaque mois.'}`;
    el('recurringText').textContent = `J'autorise le prélèvement de ${money(s.price)} après les 30 jours gratuits, puis chaque ${s.interval}, sauf annulation préalable dans l'espace client.`;
    const consumer = el('customer_type').value === 'consumer';
    el('immediateLabel').hidden = !consumer; el('immediate').required = consumer;
  }
  for (const id of ['plan', 'technicians', 'billing_cycle', 'customer_type']) el(id).addEventListener('change', render);
  el('trialForm').addEventListener('submit', async event => {
    event.preventDefault();
    if (!el('trialForm').reportValidity()) return;
    const s = selection();
    if (!Number.isInteger(s.technicians) || s.technicians < 1 || s.technicians > 500) return;
    const data = Object.fromEntries(new FormData(el('trialForm')));
    Object.assign(data, { technicians: s.technicians, expected_price_cents: Math.round(s.price * 100), terms_accepted: el('terms').checked, terms_version: '2026-09-21', recurring_accepted: el('recurring').checked, trial_terms_version: '2026-09-21-fleet-v2', immediate_performance_requested: el('immediate').checked });
    el('submitButton').disabled = true;
    try { const reply = await request('/api/v1/public/trials/request', data); message(reply.message); el('trialForm').hidden = true; }
    catch (error) { message(error.message); }
    finally { el('submitButton').disabled = false; }
  });
  el('verifyButton').addEventListener('click', async () => {
    el('verifyButton').disabled = true;
    try {
      const data = await request('/api/v1/public/trials/verify', { token });
      const url = new URL(data.checkout_url);
      if (url.protocol !== 'https:' || url.host !== 'checkout.stripe.com' || url.username || url.password) throw new Error('Adresse Stripe invalide.');
      location.assign(url.href);
    } catch (error) { message(error.message); el('verifyButton').disabled = false; }
  });
  async function init() {
    if (result === 'setup') { message("Vérification de carte reçue. Ce retour ne confirme pas encore l'activation : vous recevrez le résultat par e-mail. Aucun achat supplémentaire n'est nécessaire. Vous pouvez accéder à l'espace client via « Mot de passe oublié »."); return; }
    if (result === 'cancel') { message("Vérification de carte interrompue. Si vous avez déjà reçu une confirmation d'activation, annulez le renouvellement dans l'espace client ; fermer une page Stripe n'annule pas un abonnement existant."); return; }
    try {
      const availability = await request('/api/v1/public/trials');
      if (!availability.enabled) { message('Les essais ne sont pas encore ouverts.'); return; }
      if (token) { el('verifyPanel').hidden = false; message('Cliquez ci-dessous pour confirmer votre demande.'); return; }
      pricing = await request('/api/v1/public/pricing');
      if (availability.consumer_enabled) { el('consumerOption').disabled = false; el('consumerOption').textContent = 'Particulier'; }
      el('mediationNotice').hidden = !availability.mediation_pending;
      const params = new URLSearchParams(location.search);
      const planParam = params.get('plan');
      if (planParam && ['starter', 'pro', 'ultra'].includes(planParam)) {
        el('plan').value = planParam;
      }
      const cycleParam = params.get('cycle');
      if (cycleParam && ['monthly', 'annual'].includes(cycleParam)) {
        el('billing_cycle').value = cycleParam;
      }
      const techsParam = parseInt(params.get('techs'), 10);
      if (techsParam && techsParam >= 10 && techsParam <= 500) {
        el('technicians').value = techsParam;
      }
      render(); el('trialForm').hidden = false; message('Choisissez votre offre et vérifiez le tarif après l’essai.');
    } catch (_) { message('Impossible de vérifier la disponibilité des essais. Réessayez ultérieurement.'); }
  }
  window.addEventListener('hashchange', () => {
    token = new URLSearchParams(location.hash.slice(1)).get('token') || '';
    history.replaceState(null, '', location.pathname + location.search);
    el('trialForm').hidden = true; el('verifyPanel').hidden = true;
    init();
  });
  init();
})();
