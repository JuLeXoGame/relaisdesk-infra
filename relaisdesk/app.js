/**
 * RelaisDesk - Script officiel du site web relaisdesk.fr
 * Gestion des tarifs, commandes, vérifications de licence et téléchargements.
 */

// =========================================================================
// 1. CONFIGURATION GLOBALE DE L'API
// =========================================================================
const API_BASE_URL = "https://api.relaisdesk.fr";
const TERMS_VERSION = "2026-09-27";
const PRO_MONTHLY_PRICE = 110.0;
const PRO_ANNUAL_PRICE = PRO_MONTHLY_PRICE * 10;
const ULTRA_BASE_MONTHLY_PRICE = 199.0;
const ORDER_BUTTON_LABEL = "Commande avec obligation de paiement";

// =========================================================================
// 2. ÉTAT DE L'APPLICATION
// =========================================================================
let currentBillingCycle = "monthly"; // "monthly" | "annual"

let currentOrder = {
  plan: "pro",
  technicians: 5,
  billingCycle: "monthly",
  price: PRO_MONTHLY_PRICE,
  paymentMethod: "stripe"
};

// Last crypto order shown in the recap box, for quote refresh.
let lastCryptoOrder = null;

// =========================================================================
// 3. CALCUL DU PRIX ULTRA / PERSONNALISÉ (10 à 500 techniciens)
// =========================================================================
function calculateManagedDeviceLimit(plan, technicians) {
  if (plan === 'starter') return 500;
  if (plan === 'pro') return 1000;
  const techs = Math.min(500, Math.max(10, Math.trunc(Number(technicians)) || 10));
  return techs === 500 ? 4500 : 2000 + (techs - 10) * 5;
}

function calculateUltraPrice(technicians, cycle) {
  let techs = parseInt(technicians, 10) || 10;
  if (techs < 10) techs = 10;
  if (techs > 500) techs = 500;

  let monthly = ULTRA_BASE_MONTHLY_PRICE;
  if (techs <= 10) {
    monthly = ULTRA_BASE_MONTHLY_PRICE;
  } else if (techs <= 50) {
    monthly = ULTRA_BASE_MONTHLY_PRICE + (techs - 10) * 14.0;
  } else if (techs <= 100) {
    const base50 = ULTRA_BASE_MONTHLY_PRICE + 40 * 14.0; // 759.00
    monthly = base50 + (techs - 50) * 10.0;
  } else {
    const base100 = ULTRA_BASE_MONTHLY_PRICE + 40 * 14.0 + 50 * 10.0; // 1259.00
    monthly = base100 + (techs - 100) * 7.0;
  }

  monthly = Math.round(monthly * 100) / 100;
  if (cycle === "annual") {
    return Math.round(monthly * 10 * 100) / 100;
  }
  return monthly;
}

// =========================================================================
// 4. INITIALISATION AU CHARGEMENT DU DOM
// =========================================================================
document.addEventListener("DOMContentLoaded", () => {
  initPublicActions();
  initBillingCycleToggle();
  initUltraSlider();
  initFaqAccordion();
  initNotificationBanner();
  initMobileMenu();
  initOrderForm();
  initWithdrawalForm();
  initContactForm();
  initOsHighlight();
  initMacBetaGate();
  updatePricingDisplays();
  window.addEventListener("relaisdesk:langchange", () => {
    updatePricingDisplays();
    const modal = document.getElementById("orderModal");
    if (modal && modal.classList.contains("active")) {
      updateModalPaymentUI();
      const planEl = document.getElementById("modalPlanName");
      const techEl = document.getElementById("modalTechs");
      const cycleEl = document.getElementById("modalBillingCycle");
      const ui = getUiText();
      const isAnnual = currentBillingCycle === "annual";
      const techs = currentOrder.technicians || 1;
      if (planEl) {
        if (currentOrder.plan === "starter") planEl.textContent = "Starter";
        else if (currentOrder.plan === "ultra") planEl.textContent = ui.planCustomSeats(techs);
        else planEl.textContent = ui.planProSeats;
      }
      if (techEl) techEl.textContent = ui.concurrentSessions(techs);
      if (cycleEl) cycleEl.textContent = isAnnual ? ui.cycleAnnualModal : ui.cycleMonthlyModal;
    }
  });
});

function initOsHighlight() {
  const ua = (navigator.userAgent || navigator.platform || "").toLowerCase();
  let detected = "windows";
  if (ua.includes("mac") || ua.includes("darwin")) {
    detected = "mac";
  } else if (ua.includes("linux") && !ua.includes("android")) {
    detected = "linux";
  }

  // Si Linux détecté, mettre en valeur le bouton Linux .deb
  if (detected === "linux") {
    document.querySelectorAll(".download-actions").forEach(container => {
      const linuxBtn = container.querySelector('a[href*=".deb"]');
      if (linuxBtn) {
        container.querySelectorAll("a").forEach(a => a.classList.replace("btn-primary", "btn-secondary"));
        linuxBtn.classList.replace("btn-secondary", "btn-primary");
      }
    });
  }
  // Note: macOS est temporairement réservé aux testeurs avec mot de passe,
  // donc nous ne le promouvons pas en bouton principal actif.
}

function initPublicActions() {
  document.addEventListener("click", (event) => {
    const trigger = event.target.closest("[data-action]");
    if (!trigger) return;

    const action = trigger.dataset.action;
    if (action === "open-order") openOrderModal(trigger.dataset.plan);
    if (action === "close-order") closeOrderModal();
    if (action === "select-payment") selectPaymentMethod(trigger.dataset.paymentMethod);
    if (action === "copy-text") copyText(trigger.dataset.target, trigger);
  });
}

// =========================================================================
// 5. BASCULE FACTURATION MENSUELLE / ANNUELLE (2 MOIS OFFERTS)
// =========================================================================
function initBillingCycleToggle() {
  const monthlyBtn = document.getElementById("cycleMonthlyBtn");
  const annualBtn = document.getElementById("cycleAnnualBtn");
  if (!monthlyBtn || !annualBtn) return;

  function setBillingCycle(cycle) {
    currentBillingCycle = cycle;
    if (cycle === "annual") {
      annualBtn.classList.add("active");
      monthlyBtn.classList.remove("active");
    } else {
      monthlyBtn.classList.add("active");
      annualBtn.classList.remove("active");
    }
    updatePricingDisplays();
  }

  monthlyBtn.addEventListener("click", () => setBillingCycle("monthly"));
  annualBtn.addEventListener("click", () => setBillingCycle("annual"));
}

const UI_TEXTS = {
  fr: {
    perYear: "€ / an",
    perMonth: "€ / mois",
    starterSavingsAnnual: "30 jours offerts • Économisez 59,80 € / an",
    starterSavingsMonthly: "30 jours d'essai gratuit • Sans engagement",
    proSavingsAnnual: (p) => `30 jours offerts • ~${p.replace(".", ",")} € / mois (remise annuelle ✨)`,
    savingsMonthly: "30 jours d'essai gratuit • Sans engagement",
    ultraSavingsAnnual: (p) => `30 jours offerts • ~${p.replace(".", ",")} € / mois (remise annuelle ✨)`,
    concurrentConn: (n) => `${n} connexion(s) simultanée(s)`,
    concurrentSessions: (n) => `${n} connexion(s) simultanée(s)`,
    planProSeats: "Pro (1-5 accès)",
    planCustomSeats: (n) => `Personnalisé (${n} accès)`,
    cycleAnnualModal: "Annuelle (365 jours - tarif réduit)",
    cycleMonthlyModal: "Mensuelle (30 jours)",
    dueToday: "Total aujourd'hui :",
    freeTrialPill: "(30 j gratuits)",
    stripeNote: (price, isAnnual) => `0 € prélevé aujourd'hui. Vos coordonnées de carte sont vérifiées par Stripe. L'abonnement payant ne débutera qu'après les 30 jours d'essai gratuit (${price} € / ${isAnnual ? 'an' : 'mois'}), résiliable à tout moment en 1 clic dans l'espace client.`,
    bankNoteAnnual: "Période de 1 an (365 jours), sans renouvellement automatique. TVA non applicable, art. 293 B du CGI. L'accès est envoyé après confirmation du paiement.",
    bankNoteMonthly: "Période de 30 jours, sans renouvellement automatique. TVA non applicable, art. 293 B du CGI. L'accès est envoyé après confirmation du paiement.",
    recurringConsent: (price) => `J'accepte le prélèvement automatique de ${price} € après les 30 jours gratuits, résiliable à tout moment dans l'espace client. *`,
    submitStripe: "Démarrer mon essai gratuit de 30 jours (0 €)",
    submitBank: "Valider la commande par virement",
    submitCrypto: (label) => `Valider la commande en ${label}`,
    totalBankAnnual: "Total pour 1 an (365 jours) :",
    totalBankMonthly: "Total pour 30 jours :",
    contactingServer: "Vérification auprès du serveur...",
    commError: "Erreur de communication avec le serveur d'authentification.",
    licNotFound: (status) => `❌ <strong>Licence introuvable ou inactive</strong><br>Statut : ${status}. Contactez le support si vous venez d'effectuer un virement.`,
    licActive: (id, email, max, date) => `✅ <strong>Licence Active</strong><br>• Identifiant : <code>${id}</code><br>• Client : ${email}<br>• Connexions simultanées : ${max} technicien(s)<br>• Expire le : ${date}`,
    dateLocale: "fr-FR"
  },
  en: {
    perYear: "€ / year",
    perMonth: "€ / month",
    starterSavingsAnnual: "30 days free • Save €59.80/yr",
    starterSavingsMonthly: "30 days free trial • Cancel anytime",
    proSavingsAnnual: (p) => `30 days free • ~€${p}/mo (annual discount ✨)`,
    savingsMonthly: "30 days free trial • Cancel anytime",
    ultraSavingsAnnual: (p) => `30 days free • ~€${p}/mo (annual discount ✨)`,
    concurrentConn: (n) => `${n} concurrent connection(s)`,
    concurrentSessions: (n) => `${n} concurrent session(s)`,
    planProSeats: "Pro (1-5 seats)",
    planCustomSeats: (n) => `Custom (${n} seats)`,
    cycleAnnualModal: "Annual (365 days - discounted price)",
    cycleMonthlyModal: "Monthly (30 days)",
    dueToday: "Due today :",
    freeTrialPill: "(30 days free)",
    stripeNote: (price, isAnnual) => `€0 charged today. Card verified securely via Stripe. Paid subscription begins automatically after the 30-day free trial (€${price} / ${isAnnual ? 'year' : 'month'}), cancelable anytime in 1 click in your client portal.`,
    bankNoteAnnual: "1-year period (365 days), prepaid without automatic renewal. Access sent upon bank transfer receipt.",
    bankNoteMonthly: "30-day period, prepaid without automatic renewal. Access sent upon bank transfer receipt.",
    recurringConsent: (price) => `I accept automatic billing of €${price} after the 30-day free trial, cancelable anytime in the client portal. *`,
    submitStripe: "Start 30-day free trial (0 €)",
    submitBank: "Confirm order by bank transfer",
    submitCrypto: (label) => `Confirm order in ${label}`,
    totalBankAnnual: "Total for 1 year (365 days) :",
    totalBankMonthly: "Total for 30 days :",
    contactingServer: "Contacting server…",
    commError: "Communication error with license server.",
    licNotFound: (status) => `❌ <strong>License not found or inactive</strong><br>Status: ${status}. Contact support if you recently sent a bank transfer.`,
    licActive: (id, email, max, date) => `✅ <strong>Active License</strong><br>• License ID: <code>${id}</code><br>• Customer: ${email}<br>• Concurrent connections: ${max} technician(s)<br>• Expires on: ${date}`,
    dateLocale: "en-US"
  },
  de: {
    perYear: "€ / Jahr",
    perMonth: "€ / Monat",
    starterSavingsAnnual: "30 Tage kostenlos • Sparen Sie 59,80 € / Jahr",
    starterSavingsMonthly: "30 Tage kostenlos testen • Ohne Mindestlaufzeit",
    proSavingsAnnual: (p) => `30 Tage kostenlos • ~${p.replace(".", ",")} € / Monat (Jahresrabatt ✨)`,
    savingsMonthly: "30 Tage kostenlos testen • Ohne Mindestlaufzeit",
    ultraSavingsAnnual: (p) => `30 Tage kostenlos • ~${p.replace(".", ",")} € / Monat (Jahresrabatt ✨)`,
    concurrentConn: (n) => `${n} gleichzeitige Verbindung(en)`,
    concurrentSessions: (n) => `${n} gleichzeitige Sitzung(en)`,
    planProSeats: "Pro (1-5 Zugänge)",
    planCustomSeats: (n) => `Individuell (${n} Zugänge)`,
    cycleAnnualModal: "Jährlich (365 Tage - reduzierter Tarif)",
    cycleMonthlyModal: "Monatlich (30 Tage)",
    dueToday: "Heute fällig:",
    freeTrialPill: "(30 Tage gratis)",
    stripeNote: (price, isAnnual) => `0 € heute abgebucht. Ihre Kartendaten werden sicher über Stripe verifiziert. Das kostenpflichtige Abonnement beginnt erst nach den 30 Tagen kostenloser Testphase (${price} € / ${isAnnual ? 'Jahr' : 'Monat'}), jederzeit mit 1 Klick im Kundenbereich kündbar.`,
    bankNoteAnnual: "Zeitraum von 1 Jahr (365 Tage), im Voraus bezahlt ohne automatische Verlängerung. Der Zugang wird nach Zahlungseingang freigeschaltet.",
    bankNoteMonthly: "Zeitraum von 30 Tagen, im Voraus bezahlt ohne automatische Verlängerung. Der Zugang wird nach Zahlungseingang freigeschaltet.",
    recurringConsent: (price) => `Ich akzeptiere die automatische Abbuchung von ${price} € nach den 30 kostenlosen Tagen, jederzeit kündbar im Kundenbereich. *`,
    submitStripe: "30 Tage kostenlose Testphase starten (0 €)",
    submitBank: "Bestellung per Überweisung bestätigen",
    submitCrypto: (label) => `Bestellung in ${label} bestätigen`,
    totalBankAnnual: "Gesamtbetrag für 1 Jahr (365 Tage):",
    totalBankMonthly: "Gesamtbetrag für 30 Tage:",
    contactingServer: "Serverabfrage läuft…",
    commError: "Kommunikationsfehler mit dem Lizenzserver.",
    licNotFound: (status) => `❌ <strong>Lizenz nicht gefunden oder inaktiv</strong><br>Status: ${status}. Kontaktieren Sie den Support, falls Sie kürzlich eine Überweisung getätigt haben.`,
    licActive: (id, email, max, date) => `✅ <strong>Aktive Lizenz</strong><br>• Lizenz-ID: <code>${id}</code><br>• Kunde: ${email}<br>• Gleichzeitige Verbindungen: ${max} Techniker<br>• Gültig bis: ${date}`,
    dateLocale: "de-DE"
  },
  es: {
    perYear: "€ / año",
    perMonth: "€ / mes",
    starterSavingsAnnual: "30 días gratis • Ahorre 59,80 € / año",
    starterSavingsMonthly: "30 días de prueba gratis • Sin permanencia",
    proSavingsAnnual: (p) => `30 días gratis • ~${p.replace(".", ",")} € / mes (descuento anual ✨)`,
    savingsMonthly: "30 días de prueba gratis • Sin permanencia",
    ultraSavingsAnnual: (p) => `30 días gratis • ~${p.replace(".", ",")} € / mes (descuento anual ✨)`,
    concurrentConn: (n) => `${n} conexión(es) simultánea(s)`,
    concurrentSessions: (n) => `${n} sesión(es) simultánea(s)`,
    planProSeats: "Pro (1-5 accesos)",
    planCustomSeats: (n) => `Personalizado (${n} accesos)`,
    cycleAnnualModal: "Anual (365 días - tarifa reducida)",
    cycleMonthlyModal: "Mensual (30 días)",
    dueToday: "Total a pagar hoy:",
    freeTrialPill: "(30 días gratis)",
    stripeNote: (price, isAnnual) => `0 € cobrados hoy. Sus datos de tarjeta son verificados por Stripe. La suscripción de pago comenzará tras los 30 días de prueba gratuita (${price} € / ${isAnnual ? 'año' : 'mes'}), cancelable en cualquier momento en 1 clic desde el área de clientes.`,
    bankNoteAnnual: "Período de 1 año (365 días), prepagado sin renovación automática. El acceso se envía tras la confirmación del pago.",
    bankNoteMonthly: "Período de 30 días, prepagado sin renovación automática. El acceso se envía tras la confirmación del pago.",
    recurringConsent: (price) => `Acepto el cobro automático de ${price} € tras los 30 días gratuitos, cancelable en cualquier momento desde el área de clientes. *`,
    submitStripe: "Comenzar mi prueba gratuita de 30 días (0 €)",
    submitBank: "Confirmar pedido por transferencia",
    submitCrypto: (label) => `Confirmar pedido en ${label}`,
    totalBankAnnual: "Total por 1 año (365 días):",
    totalBankMonthly: "Total por 30 días:",
    contactingServer: "Verificando con el servidor...",
    commError: "Error de comunicación con el servidor de licencias.",
    licNotFound: (status) => `❌ <strong>Licencia no encontrada o inactiva</strong><br>Estado: ${status}. Contacte con soporte si acaba de realizar una transferencia.`,
    licActive: (id, email, max, date) => `✅ <strong>Licencia Activa</strong><br>• ID de licencia: <code>${id}</code><br>• Cliente: ${email}<br>• Conexiones simultáneas: ${max} técnico(s)<br>• Vence el: ${date}`,
    dateLocale: "es-ES"
  },
  it: {
    perYear: "€ / anno",
    perMonth: "€ / mese",
    starterSavingsAnnual: "30 giorni gratis • Risparmia 59,80 € / anno",
    starterSavingsMonthly: "30 giorni di prova gratis • Senza impegno",
    proSavingsAnnual: (p) => `30 giorni gratis • ~${p.replace(".", ",")} € / mese (sconto annuale ✨)`,
    savingsMonthly: "30 giorni di prova gratis • Senza impegno",
    ultraSavingsAnnual: (p) => `30 giorni gratis • ~${p.replace(".", ",")} € / mese (sconto annuale ✨)`,
    concurrentConn: (n) => `${n} connessione/i simultanea/e`,
    concurrentSessions: (n) => `${n} sessione/i simultanea/e`,
    planProSeats: "Pro (1-5 accessi)",
    planCustomSeats: (n) => `Personalizzato (${n} accessi)`,
    cycleAnnualModal: "Annuale (365 giorni - tariffa scontata)",
    cycleMonthlyModal: "Mensile (30 giorni)",
    dueToday: "Totale dovuto oggi:",
    freeTrialPill: "(30 gg gratuiti)",
    stripeNote: (price, isAnnual) => `0 € addebitati oggi. I dettagli della carta sono verificati da Stripe. L'abbonamento a pagamento inizierà solo dopo i 30 giorni di prova gratuita (${price} € / ${isAnnual ? 'anno' : 'mese'}), annullabile in qualsiasi momento con 1 clic nell'area clienti.`,
    bankNoteAnnual: "Periodo di 1 anno (365 giorni), prepagato senza rinnovo automatico. L'accesso viene inviato dopo la conferma del pagamento.",
    bankNoteMonthly: "Periodo di 30 giorni, prepagato senza rinnovo automatico. L'accesso viene inviato dopo la conferma del pagamento.",
    recurringConsent: (price) => `Accetto l'addebito automatico di ${price} € dopo i 30 giorni gratuiti, annullabile in qualsiasi momento nell'area clienti. *`,
    submitStripe: "Inizia la mia prova gratuita di 30 giorni (0 €)",
    submitBank: "Conferma l'ordine tramite bonifico",
    submitCrypto: (label) => `Conferma l'ordine in ${label}`,
    totalBankAnnual: "Totale per 1 anno (365 giorni):",
    totalBankMonthly: "Totale per 30 giorni:",
    contactingServer: "Verifica con il server in corso...",
    commError: "Errore di comunicazione con il server delle licenze.",
    licNotFound: (status) => `❌ <strong>Licenza non trovata o inattiva</strong><br>Stato: ${status}. Contatta il supporto se hai appena effettuato un bonifico.`,
    licActive: (id, email, max, date) => `✅ <strong>Licenza Attiva</strong><br>• ID licenza: <code>${id}</code><br>• Cliente: ${email}<br>• Connessioni simultanee: ${max} tecnico/i<br>• Scade il: ${date}`,
    dateLocale: "it-IT"
  },
  ru: {
    perYear: "€ / год",
    perMonth: "€ / мес",
    starterSavingsAnnual: "30 дней бесплатно • Экономия 59,80 € / год",
    starterSavingsMonthly: "30 дней бесплатной пробной версии • Без обязательств",
    proSavingsAnnual: (p) => `30 дней бесплатно • ~${p.replace(".", ",")} € / мес (годовая скидка ✨)`,
    savingsMonthly: "30 дней бесплатной пробной версии • Без обязательств",
    ultraSavingsAnnual: (p) => `30 дней бесплатно • ~${p.replace(".", ",")} € / мес (годовая скидка ✨)`,
    concurrentConn: (n) => `${n} одновременных сессий`,
    concurrentSessions: (n) => `${n} одновременных сессий`,
    planProSeats: "Pro (1-5 доступов)",
    planCustomSeats: (n) => `Индивидуальный (${n} доступов)`,
    cycleAnnualModal: "Годовой (365 дней - скидка)",
    cycleMonthlyModal: "Ежемесячный (30 дней)",
    dueToday: "К оплате сегодня:",
    freeTrialPill: "(30 дней бесплатно)",
    stripeNote: (price, isAnnual) => `0 € сегодня. Данные карты проверяются через Stripe. Платная подписка начнется только после 30 дней бесплатного пробного периода (${price} € / ${isAnnual ? 'год' : 'месяц'}), отмена в любой момент в 1 клик в личном кабинете.`,
    bankNoteAnnual: "Период на 1 год (365 дней), по предоплате без автопродления. Доступ отправляется после подтверждения оплаты.",
    bankNoteMonthly: "Период на 30 дней, по предоплате без автопродления. Доступ отправляется после подтверждения оплаты.",
    recurringConsent: (price) => `Я соглашаюсь на автоматическое списание ${price} € после 30 бесплатных дней, отмена в любой момент в личном кабинете. *`,
    submitStripe: "Начать 30-дневный бесплатный период (0 €)",
    submitBank: "Подтвердить заказ банковским переводом",
    submitCrypto: (label) => `Подтвердить заказ в ${label}`,
    totalBankAnnual: "Итого за 1 год (365 дней):",
    totalBankMonthly: "Итого за 30 дней:",
    contactingServer: "Проверка на сервере...",
    commError: "Ошибка связи с сервером лицензий.",
    licNotFound: (status) => `❌ <strong>Лицензия не найдена или неактивна</strong><br>Статус: ${status}. Обратитесь в службу поддержки, если вы недавно сделали перевод.`,
    licActive: (id, email, max, date) => `✅ <strong>Активная лицензия</strong><br>• ID лицензии: <code>${id}</code><br>• Клиент: ${email}<br>• Одновременных сессий: ${max}<br>• Действует до: ${date}`,
    dateLocale: "ru-RU"
  },
  pl: {
    perYear: "€ / rok",
    perMonth: "€ / mies.",
    starterSavingsAnnual: "30 dni gratis • Oszczędź 59,80 € / rok",
    starterSavingsMonthly: "30 dni darmowego okresu próbnego • Bez zobowiązań",
    proSavingsAnnual: (p) => `30 dni gratis • ~${p.replace(".", ",")} € / mies. (rabat roczny ✨)`,
    savingsMonthly: "30 dni darmowego okresu próbnego • Bez zobowiązań",
    ultraSavingsAnnual: (p) => `30 dni gratis • ~${p.replace(".", ",")} € / mies. (rabat roczny ✨)`,
    concurrentConn: (n) => `${n} jednoczesnych połączeń`,
    concurrentSessions: (n) => `${n} jednoczesnych sesji`,
    planProSeats: "Pro (1-5 dostępów)",
    planCustomSeats: (n) => `Niestandardowy (${n} dostępów)`,
    cycleAnnualModal: "Roczny (365 dni - cena obniżona)",
    cycleMonthlyModal: "Miesięczny (30 dni)",
    dueToday: "Do zapłaty dzisiaj:",
    freeTrialPill: "(30 dni za darmo)",
    stripeNote: (price, isAnnual) => `0 € pobrane dzisiaj. Dane karty są weryfikowane przez Stripe. Płatna subskrypcja rozpocznie się po 30-dniowym bezpłatnym okresie próbnym (${price} € / ${isAnnual ? 'rok' : 'miesiąc'}), z możliwością anulowania w dowolnym momencie w panelu klienta.`,
    bankNoteAnnual: "Okres 1 roku (365 dni), przedpłacony bez automatycznego odnawiania. Dostęp jest wysyłany po potwierdzeniu płatności.",
    bankNoteMonthly: "Okres 30 dni, przedpłacony bez automatycznego odnawiania. Dostęp jest wysyłany po potwierdzeniu płatności.",
    recurringConsent: (price) => `Akceptuję automatyczne pobieranie opłaty w wysokości ${price} € po 30 bezpłatnych dniach, z możliwością anulowania w panelu klienta. *`,
    submitStripe: "Rozpocznij 30-dniowy darmowy okres próbny (0 €)",
    submitBank: "Potwierdź zamówienie przelewem bankowym",
    submitCrypto: (label) => `Potwierdź zamówienie w ${label}`,
    totalBankAnnual: "Łącznie za 1 rok (365 dni):",
    totalBankMonthly: "Łącznie za 30 dni:",
    contactingServer: "Weryfikacja na serwerze...",
    commError: "Błąd komunikacji z serwerem licencji.",
    licNotFound: (status) => `❌ <strong>Licencja nie znaleziona lub nieaktywna</strong><br>Status: ${status}. Skontaktuj się ze wsparciem, jeśli niedawno wykonałeś przelew.`,
    licActive: (id, email, max, date) => `✅ <strong>Aktywna licencja</strong><br>• Identyfikator: <code>${id}</code><br>• Klient: ${email}<br>• Jednoczesne połączenia: ${max} technik(ów)<br>• Wygasa: ${date}`,
    dateLocale: "pl-PL"
  }
};

function getUiText() {
  const lang = (window.RdI18n && window.RdI18n.getLang()) || 'fr';
  return UI_TEXTS[lang] || UI_TEXTS.fr;
}

function updatePricingDisplays() {
  const isAnnual = currentBillingCycle === "annual";
  const ui = getUiText();

  // Starter
  const starterVal = document.getElementById("starterPriceVal");
  const starterPeriod = document.getElementById("starterPricePeriod");
  const starterSavings = document.getElementById("starterSavings");
  if (starterVal && starterPeriod && starterSavings) {
    if (isAnnual) {
      starterVal.textContent = "239,00";
      starterPeriod.textContent = ui.perYear;
      starterSavings.textContent = ui.starterSavingsAnnual;
      starterSavings.classList.add("highlight");
    } else {
      starterVal.textContent = "24,90";
      starterPeriod.textContent = ui.perMonth;
      starterSavings.textContent = ui.starterSavingsMonthly;
      starterSavings.classList.remove("highlight");
    }
  }

  // Pro
  const proVal = document.getElementById("proPriceVal");
  const proPeriod = document.getElementById("proPricePeriod");
  const proSavings = document.getElementById("proSavings");
  if (proVal && proPeriod && proSavings) {
    if (isAnnual) {
      proVal.textContent = PRO_ANNUAL_PRICE.toLocaleString("fr-FR", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
      proPeriod.textContent = ui.perYear;
      const perMonth = (PRO_ANNUAL_PRICE / 12).toFixed(2);
      proSavings.textContent = ui.proSavingsAnnual(perMonth);
      proSavings.classList.add("highlight");
    } else {
      proVal.textContent = PRO_MONTHLY_PRICE.toLocaleString("fr-FR", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
      proPeriod.textContent = ui.perMonth;
      proSavings.textContent = ui.savingsMonthly;
      proSavings.classList.remove("highlight");
    }
  }

  // Ultra / Personnalisé
  const slider = document.getElementById("ultraTechSlider");
  const ultraCount = document.getElementById("ultraTechCount");
  const ultraVal = document.getElementById("ultraPriceVal");
  const ultraPeriod = document.getElementById("ultraPricePeriod");
  const ultraSavings = document.getElementById("ultraSavings");
  const ultraFeature = document.getElementById("ultraTechFeature");

  const techs = slider ? parseInt(slider.value, 10) : 10;
  if (ultraCount) ultraCount.textContent = techs;
  if (ultraFeature) ultraFeature.textContent = ui.concurrentConn(techs);
  const ultraDevices = document.getElementById('ultraDeviceCount');
  if (ultraDevices) ultraDevices.textContent = calculateManagedDeviceLimit('ultra', techs).toLocaleString('fr-FR');

  const price = calculateUltraPrice(techs, currentBillingCycle);
  if (ultraVal) {
    ultraVal.textContent = price.toLocaleString("fr-FR", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }
  if (ultraPeriod) {
    ultraPeriod.textContent = isAnnual ? ui.perYear : ui.perMonth;
  }
  if (ultraSavings) {
    if (isAnnual) {
      const perMonth = (price / 12).toFixed(2);
      ultraSavings.textContent = ui.ultraSavingsAnnual(perMonth);
      ultraSavings.classList.add("highlight");
    } else {
      ultraSavings.textContent = ui.savingsMonthly;
      ultraSavings.classList.remove("highlight");
    }
  }
}

// Exposer globalement pour mise à jour lors d'un switch de langue
window.RdUpdatePricing = updatePricingDisplays;

// =========================================================================
// 6. GESTION DU SLIDER ULTRA (10 à 500 techniciens)
// =========================================================================
function initUltraSlider() {
  const slider = document.getElementById("ultraTechSlider");
  if (!slider) return;

  slider.addEventListener("input", updatePricingDisplays);
}

// =========================================================================
// 7. GESTION DU MODAL DE COMMANDE
// =========================================================================
function openOrderModal(planType) {
  const modal = document.getElementById("orderModal");
  if (!modal) return;

  const isAnnual = currentBillingCycle === "annual";
  const ui = getUiText();
  let planName = "Pro";
  let techs = 5;
  let price = isAnnual ? PRO_ANNUAL_PRICE : PRO_MONTHLY_PRICE;

  if (planType === "starter") {
    planName = "Starter";
    techs = 1;
    price = isAnnual ? 239.0 : 24.90;
  } else if (planType === "ultra") {
    const slider = document.getElementById("ultraTechSlider");
    techs = slider ? parseInt(slider.value, 10) : 10;
    price = calculateUltraPrice(techs, currentBillingCycle);
    planName = ui.planCustomSeats(techs);
  } else {
    planName = ui.planProSeats;
    techs = 5;
    price = isAnnual ? PRO_ANNUAL_PRICE : PRO_MONTHLY_PRICE;
  }

  currentOrder.plan = planType;
  currentOrder.technicians = techs;
  currentOrder.billingCycle = currentBillingCycle;
  currentOrder.price = price;
  currentOrder.paymentMethod = "stripe";
  
  // Mise à jour de l'affichage du modal
  const planEl = document.getElementById("modalPlanName");
  const techEl = document.getElementById("modalTechs");
  const cycleEl = document.getElementById("modalBillingCycle");

  if (planEl) planEl.textContent = planName;
  if (techEl) techEl.textContent = ui.concurrentSessions(techs);
  if (cycleEl) {
    cycleEl.textContent = isAnnual ? ui.cycleAnnualModal : ui.cycleMonthlyModal;
  }

  // Réinitialisation du formulaire
  document.getElementById("orderForm").reset();
  document.getElementById("orderCustomerType").value = "business";
  updateCustomerTypeFields();
  document.getElementById("orderErrorMessage").style.display = "none";
  document.getElementById("bankTransferResult").style.display = "none";
  const trialResult = document.getElementById("trialResult");
  if (trialResult) trialResult.style.display = "none";
  document.getElementById("cryptoResult").style.display = "none";
  lastCryptoOrder = null;
  document.getElementById("orderFormFields").style.display = "block";
  document.getElementById("submitOrderBtn").disabled = false;

  // Sélectionner Stripe (Essai 30 jours) par défaut
  selectPaymentMethod("stripe");
  refreshPaymentMethods();

  modal.classList.add("active");
}

function closeOrderModal() {
  const modal = document.getElementById("orderModal");
  if (modal) {
    modal.classList.remove("active");
  }
}

function updateModalPaymentUI() {
  const isAnnual = currentBillingCycle === "annual";
  const ui = getUiText();
  const price = currentOrder.price || 0;
  const isStripe = currentOrder.paymentMethod === "stripe";
  const priceFormatted = price.toLocaleString("fr-FR", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

  const totalLabelEl = document.getElementById("modalTotalLabel");
  const totalValEl = document.getElementById("modalTotalPrice");
  const periodNoteEl = document.getElementById("orderTermsPeriodNote");
  const fleetQuotaEl = document.getElementById("orderFleetQuota");
  if (fleetQuotaEl) {
    const quotaLabel = window.RdI18n && window.RdI18n.t
      ? window.RdI18n.t('plan_feat_managed_devices') : 'postes enregistrables dans la console';
    fleetQuotaEl.textContent = `${calculateManagedDeviceLimit(currentOrder.plan, currentOrder.technicians).toLocaleString('fr-FR')} ${quotaLabel}`;
  }
  const recurringLabel = document.getElementById("trialRecurringLabel");
  const recurringInput = document.getElementById("orderRecurringAccepted");
  const recurringText = document.getElementById("orderRecurringText");
  const submitBtn = document.getElementById("submitOrderBtn");

  if (isStripe) {
    if (totalLabelEl) totalLabelEl.textContent = ui.dueToday;
    if (totalValEl) totalValEl.innerHTML = `<span style="color: #34d399;">0,00 €</span> <small style="font-size: 0.75rem; color: var(--text-secondary); font-weight: normal;">${ui.freeTrialPill}</small>`;
    if (periodNoteEl) {
      periodNoteEl.textContent = ui.stripeNote(priceFormatted, isAnnual);
    }
    if (recurringLabel) recurringLabel.style.display = "flex";
    if (recurringInput) recurringInput.required = true;
    if (recurringText) {
      recurringText.textContent = ui.recurringConsent(priceFormatted);
    }
    if (submitBtn) submitBtn.textContent = ui.submitStripe;
  } else {
    if (totalLabelEl) totalLabelEl.textContent = isAnnual ? ui.totalBankAnnual : ui.totalBankMonthly;
    if (totalValEl) totalValEl.textContent = `${priceFormatted} €`;
    if (periodNoteEl) {
      periodNoteEl.textContent = isAnnual ? ui.bankNoteAnnual : ui.bankNoteMonthly;
    }
    if (recurringLabel) recurringLabel.style.display = "none";
    if (recurringInput) {
      recurringInput.required = false;
      recurringInput.checked = false;
    }
    if (submitBtn) {
      const cryptoLabel = cryptoAssetLabel(currentOrder.paymentMethod);
      submitBtn.textContent = cryptoLabel ? ui.submitCrypto(cryptoLabel) : ui.submitBank;
    }
  }
}

// cryptoAssetLabel returns the display name of a crypto payment method, or
// null for non-crypto methods.
function cryptoAssetLabel(method) {
  if (method === "crypto_btc") return "Bitcoin";
  if (method === "crypto_xrp") return "XRP";
  return null;
}

// refreshPaymentMethods shows the crypto options only when the API offers
// them (CGV: "lorsqu'ils sont proposés au récapitulatif"). On any failure
// crypto stays hidden and the order falls back to card or bank transfer.
async function refreshPaymentMethods() {
  try {
    const response = await fetch(`${API_BASE_URL}/api/v1/public/payment-methods`, { cache: "no-store" });
    if (!response.ok) return;
    const methods = await response.json();
    const btc = document.getElementById("payOpt_crypto_btc");
    const xrp = document.getElementById("payOpt_crypto_xrp");
    if (btc) btc.style.display = methods.crypto_btc ? "flex" : "none";
    if (xrp) xrp.style.display = methods.crypto_xrp ? "flex" : "none";
  } catch (_) { /* crypto stays hidden when unreachable */ }
}

function selectPaymentMethod(method) {
  if (method === "paypal") return; // Phase 2

  currentOrder.paymentMethod = method;

  document.querySelectorAll(".payment-option").forEach(el => {
    el.classList.remove("selected");
  });

  const selectedEl = document.getElementById(`payOpt_${method}`);
  if (selectedEl) {
    selectedEl.classList.add("selected");
  }

  updateModalPaymentUI();
}

function initOrderForm() {
  const form = document.getElementById("orderForm");
  if (!form) return;

  const customerType = document.getElementById("orderCustomerType");
  customerType.addEventListener("change", updateCustomerTypeFields);
  updateCustomerTypeFields();

  form.addEventListener("submit", async (e) => {
    e.preventDefault();

    const emailInput = document.getElementById("orderEmail");
    const email = emailInput.value.trim().toLowerCase();
    const errorMsg = document.getElementById("orderErrorMessage");
    const submitBtn = document.getElementById("submitOrderBtn");

    errorMsg.style.display = "none";

    if (!email || !email.includes("@") || !email.includes(".")) {
      errorMsg.textContent = "Veuillez saisir une adresse email valide.";
      errorMsg.style.display = "block";
      return;
    }

    const nameInput = document.getElementById("orderName");
    const addrInput = document.getElementById("orderAddress");
    const zipInput = document.getElementById("orderPostalCode");
    const cityInput = document.getElementById("orderCity");
    const siretInput = document.getElementById("orderSiret");
    const customerTypeInput = document.getElementById("orderCustomerType");
    const termsAcceptedInput = document.getElementById("orderTermsAccepted");
    const immediatePerformanceInput = document.getElementById("orderImmediatePerformance");
    const customerTypeValue = customerTypeInput.value;

    if (!termsAcceptedInput.checked) {
      errorMsg.textContent = "Vous devez accepter les CGV/CGU avant de continuer.";
      errorMsg.style.display = "block";
      return;
    }
    if (customerTypeValue === "consumer" && !immediatePerformanceInput.checked) {
      errorMsg.textContent = "Pour une activation immédiate, confirmez votre demande expresse de commencement du service.";
      errorMsg.style.display = "block";
      return;
    }

    // Traitement Carte Bancaire (Essai 30 jours gratuit)
    if (currentOrder.paymentMethod === "stripe") {
      const recurringInput = document.getElementById("orderRecurringAccepted");
      if (recurringInput && !recurringInput.checked) {
        errorMsg.textContent = "Vous devez accepter l'autorisation de prélèvement automatique après les 30 jours gratuits.";
        errorMsg.style.display = "block";
        return;
      }

      submitBtn.disabled = true;
      submitBtn.textContent = "Préparation de l'essai en cours...";

      try {
        const response = await fetch(`${API_BASE_URL}/api/v1/public/trials/request`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            email: email,
            name: nameInput ? nameInput.value.trim() : "",
            address: addrInput ? addrInput.value.trim() : "",
            postal_code: zipInput ? zipInput.value.trim() : "",
            city: cityInput ? cityInput.value.trim() : "",
            country: "France",
            siret: siretInput ? siretInput.value.trim() : "",
            customer_type: customerTypeValue,
            terms_version: "2026-09-24",
            terms_accepted: true,
            recurring_accepted: true,
            trial_terms_version: "2026-09-24-fleet-v3",
            immediate_performance_requested: customerTypeValue === "consumer" && immediatePerformanceInput.checked,
            plan: currentOrder.plan,
            technicians: currentOrder.technicians,
            billing_cycle: currentOrder.billingCycle || currentBillingCycle,
            expected_price_cents: Math.round(currentOrder.price * 100)
          })
        });

        const data = await response.json();
        if (!response.ok) {
          throw new Error(data.error || "Impossible de préparer votre demande d'essai.");
        }

        document.getElementById("orderFormFields").style.display = "none";
        const trialRes = document.getElementById("trialResult");
        if (trialRes) trialRes.style.display = "block";
        const sentEl = document.getElementById("trialEmailSent");
        if (sentEl) sentEl.textContent = email;
      } catch (err) {
        errorMsg.textContent = err.message || "Erreur de connexion avec le serveur.";
        errorMsg.style.display = "block";
        submitBtn.disabled = false;
        updateModalPaymentUI();
      }
      return;
    }

    // Traitement Crypto (Bitcoin/XRP, devis au cours OKX)
    if (cryptoAssetLabel(currentOrder.paymentMethod)) {
      submitBtn.disabled = true;
      submitBtn.textContent = "Préparation du devis en cours...";

      try {
        const response = await fetch(`${API_BASE_URL}/api/v1/public/order`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            email: email,
            name: nameInput ? nameInput.value.trim() : "",
            address: addrInput ? addrInput.value.trim() : "",
            postal_code: zipInput ? zipInput.value.trim() : "",
            city: cityInput ? cityInput.value.trim() : "",
            country: "France",
            siret: siretInput ? siretInput.value.trim() : "",
            customer_type: customerTypeValue,
            terms_version: TERMS_VERSION,
            terms_accepted: termsAcceptedInput.checked,
            immediate_performance_requested: customerTypeValue === "consumer" && immediatePerformanceInput.checked,
            plan: currentOrder.plan,
            technicians: currentOrder.technicians,
            billing_cycle: currentOrder.billingCycle || currentBillingCycle,
            payment_method: currentOrder.paymentMethod
          })
        });

        const data = await response.json();
        if (!response.ok) {
          throw new Error(data.error || "Devis crypto indisponible pour le moment");
        }

        lastCryptoOrder = { order_id: data.order_id, email: email };
        showCryptoRecap(data);
      } catch (err) {
        errorMsg.textContent = err.message || "Erreur de connexion avec le serveur.";
        errorMsg.style.display = "block";
        submitBtn.disabled = false;
        updateModalPaymentUI();
      }
      return;
    }

    // Traitement Virement bancaire (Paiement direct sans récurrence)
    submitBtn.disabled = true;
    submitBtn.textContent = "Traitement en cours...";

    try {
      const response = await fetch(`${API_BASE_URL}/api/v1/public/order`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          email: email,
          name: nameInput ? nameInput.value.trim() : "",
          address: addrInput ? addrInput.value.trim() : "",
          postal_code: zipInput ? zipInput.value.trim() : "",
          city: cityInput ? cityInput.value.trim() : "",
          country: "France",
          siret: siretInput ? siretInput.value.trim() : "",
          customer_type: customerTypeValue,
          terms_version: TERMS_VERSION,
          terms_accepted: termsAcceptedInput.checked,
          immediate_performance_requested: customerTypeValue === "consumer" && immediatePerformanceInput.checked,
          plan: currentOrder.plan,
          technicians: currentOrder.technicians,
          billing_cycle: currentOrder.billingCycle || currentBillingCycle,
          payment_method: "bank_transfer"
        })
      });

      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || "Erreur lors de l'enregistrement de la commande");
      }

      if (data.instructions) {
        showBankTransferInstructions(data);
      }
    } catch (err) {
      errorMsg.textContent = err.message || "Erreur de connexion avec le serveur.";
      errorMsg.style.display = "block";
      submitBtn.disabled = false;
      updateModalPaymentUI();
    }
  });

  const cryptoRefreshBtn = document.getElementById("cryptoRefreshBtn");
  if (cryptoRefreshBtn) cryptoRefreshBtn.addEventListener("click", refreshCryptoQuote);
}

function updateCustomerTypeFields() {
  const customerType = document.getElementById("orderCustomerType");
  const immediateLabel = document.getElementById("immediatePerformanceLabel");
  const immediateInput = document.getElementById("orderImmediatePerformance");
  const siretGroup = document.getElementById("orderSiretGroup");
  if (!customerType || !immediateLabel || !immediateInput || !siretGroup) return;

  const isConsumer = customerType.value === "consumer";
  immediateLabel.style.display = isConsumer ? "flex" : "none";
  immediateInput.required = isConsumer;
  if (!isConsumer) immediateInput.checked = false;
  siretGroup.style.display = isConsumer ? "none" : "block";
  if (isConsumer) document.getElementById("orderSiret").value = "";
}

function initWithdrawalForm() {
  const form = document.getElementById("withdrawalForm");
  if (!form) return;
  const kind = document.getElementById('withdrawalKind');
  const reference = document.getElementById('withdrawalOrderID');
  const review = document.getElementById('withdrawalReview');
  let reviewed = '';
  const fragment = new URLSearchParams(location.hash.slice(1));
  if (fragment.has('trial')) { kind.value = 'trial'; reference.value = fragment.get('trial'); }
  if (location.hash) history.replaceState(null, '', location.pathname + location.search);
  form.addEventListener('input', () => { reviewed = ''; review.hidden = true; document.getElementById('withdrawalSubmitBtn').textContent = 'Se rétracter du contrat'; });
  form.addEventListener('change', () => { reviewed = ''; review.hidden = true; document.getElementById('withdrawalSubmitBtn').textContent = 'Se rétracter du contrat'; });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const submitBtn = document.getElementById("withdrawalSubmitBtn");
    const errorBox = document.getElementById("withdrawalErrorMessage");
    const resultBox = document.getElementById("withdrawalResult");
    errorBox.style.display = "none";
    resultBox.style.display = "none";
    const request = { email: document.getElementById('withdrawalEmail').value.trim().toLowerCase(), name: document.getElementById('withdrawalName').value.trim() };
    request[kind.value === 'trial' ? 'id' : 'order_id'] = reference.value.trim();
    const snapshot = JSON.stringify({ kind: kind.value, request });
    if (reviewed !== snapshot) {
      reviewed = snapshot;
      review.textContent = `Vérifiez votre notification de rétractation :\n${kind.value === 'trial' ? 'Essai / abonnement' : 'Commande prépayée'} : ${reference.value.trim()}\nNom : ${request.name || 'non renseigné'}\nAccusé de réception à : ${request.email}\n\nJe confirme vouloir me rétracter de ce contrat.`;
      review.hidden = false; submitBtn.textContent = 'Confirmer la rétractation'; return;
    }
    submitBtn.disabled = true;
    submitBtn.textContent = "Envoi en cours...";

    try {
      const response = await fetch(`${API_BASE_URL}${kind.value === 'trial' ? '/api/v1/public/trials/withdraw' : '/api/v1/public/withdrawal'}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request), referrerPolicy: 'no-referrer'
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "La demande n'a pas pu être enregistrée.");

      resultBox.innerHTML = `Votre notification a été reçue.<br>Référence : <code>${escapeHtml(data.request_id)}</code><br>Date de réception : ${escapeHtml(new Date(data.requested_at).toLocaleString("fr-FR"))}`;
      resultBox.style.display = "block";
      form.reset();
      review.hidden = true; reviewed = '';
    } catch (error) {
      errorBox.textContent = error.message || "Erreur de communication avec le serveur.";
      errorBox.style.display = "block";
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = reviewed ? 'Confirmer la rétractation' : 'Se rétracter du contrat';
    }
  });
}

function initContactForm() {
  const form = document.getElementById("contactForm");
  if (!form) return;

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const submitBtn = document.getElementById("contactSubmitBtn");
    const errorBox = document.getElementById("contactErrorMessage");
    const successBox = document.getElementById("contactSuccessMessage");

    if (errorBox) errorBox.style.display = "none";
    if (successBox) successBox.style.display = "none";

    const name = document.getElementById("contactName")?.value.trim() || "";
    const email = document.getElementById("contactEmail")?.value.trim() || "";
    const subject = document.getElementById("contactSubject")?.value.trim() || "";
    const message = document.getElementById("contactMessage")?.value.trim() || "";
    const honeypot = document.getElementById("contactWebsite")?.value || "";

    if (!name || !email || !message) {
      if (errorBox) {
        errorBox.textContent = "Veuillez renseigner votre nom, votre adresse email et votre message.";
        errorBox.style.display = "block";
      }
      return;
    }

    if (submitBtn) {
      submitBtn.disabled = true;
      submitBtn.textContent = "Envoi en cours...";
    }

    try {
      const response = await fetch(`${API_BASE_URL}/api/v1/public/contact`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name,
          email: email,
          subject: subject,
          message: message,
          website: honeypot
        })
      });

      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || "Impossible d'envoyer votre message pour le moment.");
      }

      if (successBox) {
        successBox.innerHTML = `✅ <strong>Message envoyé avec succès !</strong><br>${escapeHtml(data.message || "Nous vous répondrons dans les plus brefs délais.")}`;
        successBox.style.display = "block";
      }
      form.reset();
    } catch (err) {
      if (errorBox) {
        errorBox.textContent = err.message || "Erreur de communication avec le serveur.";
        errorBox.style.display = "block";
      }
    } finally {
      if (submitBtn) {
        submitBtn.disabled = false;
        submitBtn.textContent = "✉️ Envoyer mon message";
      }
    }
  });
}

function showBankTransferInstructions(data) {
  document.getElementById("orderFormFields").style.display = "none";
  const resultBox = document.getElementById("bankTransferResult");

  document.getElementById("bankIbanVal").textContent = data.instructions.iban;
  document.getElementById("bankBicVal").textContent = data.instructions.bic;
  document.getElementById("bankHolderVal").textContent = data.instructions.beneficiary;
  document.getElementById("bankRefVal").textContent = data.instructions.reference;
  document.getElementById("bankAmountVal").textContent = `${data.instructions.amount.toFixed(2).replace(".", ",")} €`;

  resultBox.style.display = "block";
}

function formatCryptoDate(iso) {
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return iso;
  const ui = getUiText();
  return parsed.toLocaleString(ui.dateLocale || "fr-FR", {
    day: "2-digit", month: "2-digit", year: "numeric",
    hour: "2-digit", minute: "2-digit"
  });
}

function showCryptoRecap(data) {
  document.getElementById("orderFormFields").style.display = "none";

  document.getElementById("cryptoAmountVal").textContent = data.amount_crypto;
  document.getElementById("cryptoAssetVal").textContent = data.asset;
  document.getElementById("cryptoAddressVal").textContent = data.pay_address;
  const tagRow = document.getElementById("cryptoTagRow");
  if (data.dest_tag !== undefined && data.dest_tag !== null) {
    document.getElementById("cryptoTagVal").textContent = data.dest_tag;
    tagRow.style.display = "flex";
  } else {
    tagRow.style.display = "none";
  }
  document.getElementById("cryptoRateVal").textContent = `${data.rate_eur} EUR/${data.asset} (OKX)`;
  document.getElementById("cryptoExpiryVal").textContent = formatCryptoDate(data.expires_at);
  document.getElementById("cryptoRefVal").textContent = data.order_id;
  document.getElementById("cryptoNotice").textContent = data.notice || "";
  document.getElementById("cryptoRefreshMessage").style.display = "none";

  document.getElementById("cryptoResult").style.display = "block";
}

async function refreshCryptoQuote() {
  const msg = document.getElementById("cryptoRefreshMessage");
  const btn = document.getElementById("cryptoRefreshBtn");
  if (!lastCryptoOrder) return;
  btn.disabled = true;
  msg.style.display = "none";
  try {
    const response = await fetch(`${API_BASE_URL}/api/v1/public/crypto/quote`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ order_id: lastCryptoOrder.order_id, email: lastCryptoOrder.email })
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || "Renouvellement du devis impossible pour le moment.");
    }
    showCryptoRecap(data);
    msg.style.color = "var(--text-secondary)";
    msg.textContent = "Nouveau devis affiché ci-dessus.";
    msg.style.display = "block";
  } catch (err) {
    msg.style.color = "var(--danger)";
    msg.textContent = err.message || "Erreur de connexion avec le serveur.";
    msg.style.display = "block";
  } finally {
    btn.disabled = false;
  }
}

// =========================================================================
// 7. ACCORDÉON FAQ
// =========================================================================
function initFaqAccordion() {
  document.querySelectorAll(".faq-question").forEach(q => {
    q.addEventListener("click", () => {
      const item = q.parentElement;
      const isOpen = item.classList.contains("open");

      document.querySelectorAll(".faq-item").forEach(el => {
        el.classList.remove("open");
        el.querySelector(".faq-question")?.setAttribute("aria-expanded", "false");
      });

      if (!isOpen) {
        item.classList.add("open");
        q.setAttribute("aria-expanded", "true");
      }
    });
  });
}

// =========================================================================
// 9. BANNIÈRE DE RETOUR DE PAIEMENT
// =========================================================================
function initNotificationBanner() {
  const urlParams = new URLSearchParams(window.location.search);
  const banner = document.getElementById("notificationBanner");

  if (!banner) return;

  if (urlParams.get("order") === "success" || urlParams.get("order") === "stripe_success") {
    banner.className = "notification-banner success";
    banner.textContent = "🎉 Paiement validé avec succès ! Votre clé de licence et vos accès vous ont été envoyés par email.";
    banner.style.display = "block";
  } else if (urlParams.get("order") === "cancel") {
    banner.className = "notification-banner cancel";
    banner.textContent = "ℹ️ La transaction a été annulée. Vous pouvez reprendre votre commande à tout moment.";
    banner.style.display = "block";
  }
}

// =========================================================================
// 10. UTILITAIRES COPIE PRESSE-PAPIER & MENU MOBILE
// =========================================================================
function copyText(elementId, btnElement) {
  const el = document.getElementById(elementId);
  if (!el) return;

  navigator.clipboard.writeText(el.textContent.trim()).then(() => {
    const originalText = btnElement.textContent;
    btnElement.textContent = "Copié !";
    setTimeout(() => {
      btnElement.textContent = originalText;
    }, 2000);
  });
}

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function initMobileMenu() {
  const toggle = document.querySelector(".mobile-toggle");
  const menu = document.querySelector(".nav-menu");

  if (!toggle || !menu) return;

  toggle.addEventListener("click", (e) => {
    e.stopPropagation();
    const isOpen = menu.classList.toggle("open");
    toggle.textContent = isOpen ? "✕" : "☰";
    toggle.setAttribute("aria-expanded", isOpen);
    toggle.setAttribute("aria-label", isOpen ? "Fermer le menu" : "Ouvrir le menu");
  });

  // Fermer le menu lors du clic sur un lien
  document.querySelectorAll(".nav-link").forEach(link => {
    link.addEventListener("click", () => {
      menu.classList.remove("open");
      toggle.textContent = "☰";
      toggle.setAttribute("aria-expanded", "false");
      toggle.setAttribute("aria-label", "Ouvrir le menu");
    });
  });

  // Fermer le menu lors d'un clic à l'extérieur
  document.addEventListener("click", (e) => {
    if (!menu.contains(e.target) && !toggle.contains(e.target)) {
      menu.classList.remove("open");
      toggle.textContent = "☰";
      toggle.setAttribute("aria-expanded", "false");
      toggle.setAttribute("aria-label", "Ouvrir le menu");
    }
  });
}

// =========================================================================
// GESTION DU TÉLÉCHARGEMENT BÊTA MACOS AVEC MOT DE PASSE
// =========================================================================
const MAC_TEST_PASSWORD = "SupermegaCidhom03";
let pendingMacDownloadUrl = null;

function initMacBetaGate() {
  const modal = document.getElementById("macPasswordModal");
  const form = document.getElementById("macPasswordForm");
  const input = document.getElementById("macPasswordInput");
  const errorMsg = document.getElementById("macPasswordError");
  const closeBtn = document.getElementById("closeMacPasswordModal");

  function openMacModal(url) {
    pendingMacDownloadUrl = url;
    if (modal) {
      modal.classList.add("active");
      if (errorMsg) errorMsg.style.display = "none";
      if (input) {
        input.value = "";
        setTimeout(() => input.focus(), 150);
      }
    } else {
      const pass = window.prompt("Accès réservé aux testeurs macOS. Veuillez entrer votre mot de passe :");
      if (pass === MAC_TEST_PASSWORD) {
        sessionStorage.setItem("mac_beta_unlocked", "1");
        window.location.href = url;
      } else if (pass !== null) {
        alert("Mot de passe incorrect.");
      }
    }
  }

  function closeMacModal() {
    if (modal) modal.classList.remove("active");
    if (errorMsg) errorMsg.style.display = "none";
    if (input) input.value = "";
  }

  // Intercepter les clics sur les boutons de téléchargement macOS
  document.querySelectorAll('a[href*="_Mac.dmg"], a[href*="RelaisDesk_Mac.dmg"], .btn-mac-locked').forEach(btn => {
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      const targetUrl = btn.getAttribute("href");

      // Si déjà débloqué pendant cette session de navigation
      if (sessionStorage.getItem("mac_beta_unlocked") === "1") {
        window.location.href = targetUrl;
        return;
      }

      openMacModal(targetUrl);
    });
  });

  if (!modal) return;

  if (closeBtn) closeBtn.addEventListener("click", closeMacModal);
  modal.addEventListener("click", (e) => {
    if (e.target === modal) closeMacModal();
  });

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && modal.classList.contains("active")) {
      closeMacModal();
    }
  });

  if (form) {
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      const entered = (input ? input.value : "").trim();
      if (entered === MAC_TEST_PASSWORD) {
        sessionStorage.setItem("mac_beta_unlocked", "1");
        closeMacModal();
        if (pendingMacDownloadUrl) {
          window.location.href = pendingMacDownloadUrl;
        }
      } else {
        if (errorMsg) errorMsg.style.display = "block";
        if (input) {
          input.focus();
          input.select();
        }
      }
    });
  }
}
