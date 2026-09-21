'use strict';
// Hide the offer until its API, mail delivery and legal rollout are enabled.
fetch('https://api.relaisdesk.fr/api/v1/public/trials', { cache: 'no-store' })
  .then(response => response.ok ? response.json() : null)
  .then(data => { const banner = document.getElementById('trialOffer'); if (banner && data && data.enabled) banner.hidden = false; })
  .catch(() => {});
