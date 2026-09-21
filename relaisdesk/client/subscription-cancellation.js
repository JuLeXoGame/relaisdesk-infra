/* Presentation only: the existing API remains responsible for cancellation. */
'use strict';
window.RdSubscriptionCancellation = {
  create(dialog) {
    let pending = null;
    const field = name => dialog.querySelector('[data-cancel="' + name + '"]');
    const finish = accepted => {
      if (!pending) return;
      const resolve = pending;
      pending = null;
      dialog.close();
      resolve(accepted);
    };
    dialog.querySelector('form').addEventListener('submit', event => {
      event.preventDefault();
      finish(true);
    });
    field('back').addEventListener('click', () => finish(false));
    dialog.addEventListener('cancel', event => { event.preventDefault(); finish(false); });
    // A queued close event from a previous review must not dismiss a new one.
    dialog.addEventListener('close', () => { if (!dialog.open) finish(false); });
    return {
      dismiss() { finish(false); },
      confirm(details) {
        if (pending || dialog.open) return Promise.resolve(false);
        const en = details.language === 'en';
        const labels = en ? {
          title: 'Review your cancellation', contractLabel: 'Contract reference',
          accountLabel: 'Account email', offerLabel: 'Subscription', priceLabel: 'Recurring price',
          endLabel: 'End of free or paid access', back: 'Back to subscriptions',
          submit: 'Confirm contract cancellation',
          explanation: 'This cancels future renewals. Access remains available until the date above. Any refunds are handled separately. To correct the selected contract, go back without confirming.',
          support: 'For a different effective date or an exceptional termination request, contact us. Your statutory rights remain unaffected.'
        } : {
          title: 'Vérifier votre résiliation', contractLabel: 'Référence du contrat',
          accountLabel: 'E-mail du compte', offerLabel: 'Abonnement', priceLabel: 'Prix récurrent',
          endLabel: "Fin de l’accès gratuit ou payé", back: 'Retour aux abonnements',
          submit: 'Confirmer la résiliation du contrat',
          explanation: "Cette demande arrête les prochains renouvellements. L’accès reste disponible jusqu’à la date ci-dessus. Les remboursements éventuels sont traités séparément. Pour corriger le contrat sélectionné, revenez en arrière sans confirmer.",
          support: 'Pour une autre date de fin ou un motif de résiliation exceptionnelle, contactez-nous. Vos droits légaux restent applicables.'
        };
        for (const [name, value] of Object.entries(labels)) field(name).textContent = value;
        for (const name of ['contract', 'account', 'offer', 'price', 'end']) {
          field(name).textContent = String(details[name] || '—');
        }
        return new Promise(resolve => {
          pending = resolve;
          try {
            dialog.showModal();
            // Start at the recap, not at the buttons below the fold on mobile.
            field('title').focus();
            dialog.querySelector('form').scrollTop = 0;
            dialog.scrollTop = 0;
          }
          catch (_) { pending = null; resolve(false); }
        });
      }
    };
  }
};
