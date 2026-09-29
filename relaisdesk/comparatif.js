'use strict';

/* Comparateur RelaisDesk : logique pure (testable) + câblage DOM. */

const RD_PLANS = [
  { id: 'starter', name: 'Starter', monthly: 24.90, maxTechs: 1 },
  { id: 'pro', name: 'Pro', monthly: 110.00, maxTechs: 5 },
  { id: 'ultra', name: 'Personnalisé', monthly: 199.00, maxTechs: 500 }
];

function recommendPlan(techs) {
  const n = Math.max(1, Math.min(500, Math.ceil(Number(techs) || 1)));
  for (const plan of RD_PLANS) {
    if (n <= plan.maxTechs) return plan;
  }
  return RD_PLANS[RD_PLANS.length - 1];
}

function computeSavings(currentMonthly, techs) {
  const bill = Math.max(0, Number(currentMonthly) || 0);
  const plan = recommendPlan(techs);
  const currentAnnual = bill * 12;
  const rdAnnual = plan.monthly * 12;
  const savingsAnnual = currentAnnual - rdAnnual;
  const savingsPct = currentAnnual > 0 ? Math.max(0, Math.round((savingsAnnual / currentAnnual) * 100)) : 0;
  return { planId: plan.id, planName: plan.name, rdMonthly: plan.monthly, rdAnnual, currentAnnual, savingsAnnual, savingsPct };
}

function formatEUR(amount) {
  return Number(amount).toLocaleString('fr-FR', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' €';
}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', () => {
    const billInput = document.getElementById('calc-bill');
    const techsInput = document.getElementById('calc-techs');
    const out = document.getElementById('calc-result');
    if (!billInput || !techsInput || !out) return;
    const update = () => {
      const r = computeSavings(billInput.value, techsInput.value);
      const verdict = r.savingsAnnual > 0
        ? `Économie estimée : <strong>${formatEUR(r.savingsAnnual)} par an</strong> (${r.savingsPct} %).`
        : `Coût comparable : RelaisDesk ${r.planName} reviendrait à <strong>${formatEUR(r.rdAnnual)} par an</strong>.`;
      out.innerHTML =
        `<p>Offre recommandée : <strong>RelaisDesk ${r.planName}</strong> — ${formatEUR(r.rdMonthly)} / mois, soit ${formatEUR(r.rdAnnual)} / an.</p>` +
        `<p>Votre facture actuelle projetée : ${formatEUR(r.currentAnnual)} / an.</p>` +
        `<p>${verdict}</p>` +
        `<p class="calc-note">Estimation indicative sur la base de vos saisies et de nos tarifs publics mensuels hors offre annuelle.</p>`;
    };
    billInput.addEventListener('input', update);
    techsInput.addEventListener('input', update);
    update();
  });
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { RD_PLANS, recommendPlan, computeSavings, formatEUR };
}
