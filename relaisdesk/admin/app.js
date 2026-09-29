// =============================================================================
// RelaisDesk — Dashboard Administrateur (JavaScript)
// =============================================================================

// Configuration API
const API_BASE_URL = "https://api.relaisdesk.fr";

// State
let currentToken = "";
let currentAdminEmail = "";
let currentTab = "overview";
let unmaskKeys = false;
let licensesData = [];
let invoicesData = [];
let ledgerCache = { orders: [], invoices: [] };
let currentLedgerRows = [];
let unpaidCache = [];

// Pagination state (server-side)
const PAGE_SIZE = 100;
let ordersPage = 1, ordersTotal = 0;
let invoicesPage = 1, invoicesTotal = 0;
let licensesPage = 1, licensesTotal = 0;
let selectedPlan = "starter";
let targetLicenseId = "";

// Financial State
let fiscalMode = "micro";
let urssafRate = 21.2;
let vatRate = 20.0;
let feesRate = 1.5;
let financialsData = {
  total_revenue: 0,
  paid_orders_count: 0,
  monthly_recurring_revenue: 0,
  active_subscribers_count: 0,
  monthly_history: []
};

// PWA Service Worker Registration
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("sw.js").catch(() => {});
  });
}

// Initialization on DOM Ready
document.addEventListener("DOMContentLoaded", () => {
  initAuth();
  initNavigation();
  initPagination();
  initActionButtons();
  initFinancials();
  initPlanSelector();
  initModals();
  initForms();

  // Check existing session
  const savedToken = localStorage.getItem("rd_admin_token") || sessionStorage.getItem("rd_admin_token");
  const savedEmail = localStorage.getItem("rd_admin_email") || sessionStorage.getItem("rd_admin_email");
  if (savedToken) {
    currentToken = savedToken;
    currentAdminEmail = savedEmail || "Administrateur";
    showApp();
  }
});

function initActionButtons() {
  document.addEventListener("click", (event) => {
    const trigger = event.target.closest("[data-action]");
    if (!trigger) return;

    const action = trigger.dataset.action;
    if (action === "set-fiscal-mode") setFiscalMode(trigger.dataset.mode);
    if (action === "switch-tab") switchTab(trigger.dataset.tab);
    if (action === "open-test-email") openTestEmailModal();
    if (action === "open-modal") openModal(trigger.dataset.modal);
    if (action === "close-modal") closeModal(trigger.dataset.modal);
    if (action === "close-and-switch") {
      closeModal(trigger.dataset.modal);
      switchTab(trigger.dataset.tab);
    }
    if (action === "copy-license") copyLicKey(trigger.dataset.licenseKey || "");
    if (action === "extend-license") openExtendModal(trigger.dataset.licenseId || "");
    if (action === "revoke-license") openRevokeModal(trigger.dataset.licenseId || "");
    if (action === "mark-order-paid") markOrderPaid(trigger.dataset.orderId || "");
    if (action === "retry-order-fulfillment") retryOrderFulfillment(trigger.dataset.orderId || "");
    if (action === "delete-order") deleteOrder(trigger.dataset.orderId || "");
    if (action === "download-invoice") downloadInvoice(trigger.dataset.invoiceNumber || "");
    if (action === "download-invoice-cii") downloadInvoiceCII(trigger.dataset.invoiceNumber || "");
    if (action === "export-ledger-csv") exportLedgerCSV();
    if (action === "send-invoice") sendInvoiceEmail(trigger.dataset.invoiceNumber || "");
    if (action === "credit-note") createCreditNote(trigger.dataset.invoiceNumber || "");
  });
}

// =============================================================================
// 1. AUTHENTIFICATION
// =============================================================================
function initAuth() {
  const loginForm = document.getElementById("adminLoginForm");
  const logoutBtn = document.getElementById("logoutBtn");
  const tabEmailMode = document.getElementById("tabEmailMode");
  const tabLicenseMode = document.getElementById("tabLicenseMode");
  const emailAuthFields = document.getElementById("emailAuthFields");
  const licenseAuthFields = document.getElementById("licenseAuthFields");
  const admin2FAFields = document.getElementById("admin2FAFields");
  const authModeTabs = document.getElementById("authModeTabs");
  const adminEmail = document.getElementById("adminEmail");
  const adminPassword = document.getElementById("adminPassword");
  const adminLicenseId = document.getElementById("adminLicenseId");
  const adminLicenseKey = document.getElementById("adminLicenseKey");
  const admin2FACode = document.getElementById("admin2FACode");
  const admin2FAChallengeToken = document.getElementById("admin2FAChallengeToken");
  const adminCancel2FABtn = document.getElementById("adminCancel2FABtn");
  const loginBtn = document.getElementById("loginBtn");

  let adminLoginMode = "email"; // 'email' | 'license'
  let pending2FAChallenge = "";

  function set2FAMode(active, challengeToken = "") {
    pending2FAChallenge = active ? challengeToken : "";
    if (admin2FAChallengeToken) admin2FAChallengeToken.value = pending2FAChallenge;

    if (active) {
      if (authModeTabs) authModeTabs.style.display = "none";
      if (emailAuthFields) emailAuthFields.style.display = "none";
      if (licenseAuthFields) licenseAuthFields.style.display = "none";
      if (admin2FAFields) admin2FAFields.style.display = "block";
      if (admin2FACode) {
        admin2FACode.value = "";
        admin2FACode.required = true;
        setTimeout(() => admin2FACode.focus(), 100);
      }
      if (loginBtn) loginBtn.textContent = "🔓 Valider le code 2FA";
    } else {
      if (authModeTabs) authModeTabs.style.display = "flex";
      if (admin2FAFields) admin2FAFields.style.display = "none";
      if (admin2FACode) {
        admin2FACode.value = "";
        admin2FACode.required = false;
      }
      if (loginBtn) loginBtn.textContent = "🔐 Accéder au Dashboard";
      applyLoginMode(adminLoginMode);
    }
  }

  function applyLoginMode(mode) {
    adminLoginMode = mode;
    if (mode === "email") {
      tabEmailMode?.classList.add("active");
      tabEmailMode?.setAttribute("aria-selected", "true");
      tabLicenseMode?.classList.remove("active");
      tabLicenseMode?.setAttribute("aria-selected", "false");
      if (emailAuthFields) emailAuthFields.style.display = "block";
      if (licenseAuthFields) licenseAuthFields.style.display = "none";
      if (adminEmail) adminEmail.required = true;
      if (adminPassword) adminPassword.required = true;
      if (adminLicenseId) adminLicenseId.required = false;
      if (adminLicenseKey) adminLicenseKey.required = false;
    } else {
      tabLicenseMode?.classList.add("active");
      tabLicenseMode?.setAttribute("aria-selected", "true");
      tabEmailMode?.classList.remove("active");
      tabEmailMode?.setAttribute("aria-selected", "false");
      if (emailAuthFields) emailAuthFields.style.display = "none";
      if (licenseAuthFields) licenseAuthFields.style.display = "block";
      if (adminEmail) adminEmail.required = false;
      if (adminPassword) adminPassword.required = false;
      if (adminLicenseId) adminLicenseId.required = true;
      if (adminLicenseKey) adminLicenseKey.required = true;
    }
  }

  tabEmailMode?.addEventListener("click", () => {
    set2FAMode(false);
    applyLoginMode("email");
  });

  tabLicenseMode?.addEventListener("click", () => {
    set2FAMode(false);
    applyLoginMode("license");
  });

  adminCancel2FABtn?.addEventListener("click", () => {
    set2FAMode(false);
  });

  // Handle Login submit
  if (loginForm) {
    loginForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (!loginBtn) return;
      loginBtn.disabled = true;
      loginBtn.textContent = "⏳ Vérification en cours...";

      try {
        let payload = {};
        if (pending2FAChallenge) {
          const codeVal = admin2FACode ? admin2FACode.value.trim() : "";
          if (codeVal.length !== 6) {
            throw new Error("Veuillez saisir le code à 6 chiffres généré par votre application.");
          }
          payload = {
            challenge_token: pending2FAChallenge,
            code: codeVal
          };
        } else if (adminLoginMode === "email") {
          const emailVal = adminEmail ? adminEmail.value.trim() : "";
          const passVal = adminPassword ? adminPassword.value : "";
          if (!emailVal || !passVal) {
            throw new Error("Veuillez renseigner votre adresse e-mail et votre mot de passe.");
          }
          payload = {
            email: emailVal,
            password: passVal
          };
        } else {
          const secretToken = document.getElementById("adminSecretToken")?.value.trim() || "";
          const licId = adminLicenseId ? adminLicenseId.value.trim() : "";
          const licKey = adminLicenseKey ? adminLicenseKey.value.trim() : "";
          if (secretToken) {
            payload.admin_token = secretToken;
          } else {
            if (!licId || !licKey) {
              throw new Error("Veuillez renseigner votre identifiant et votre clé de licence.");
            }
            payload.license_id = licId;
            payload.license_key = licKey;
          }
        }

        const resp = await fetch(`${API_BASE_URL}/api/v1/admin/login`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });

        const data = await resp.json();

        // Cas 2FA Requis
        if (resp.ok && data.requires_2fa) {
          set2FAMode(true, data.challenge_token);
          showToast(data.message || "Code de validation 2FA requis.", "warning");
          return;
        }

        if (!resp.ok || !data.valid) {
          throw new Error(data.error || "Identifiants administrateur invalides.");
        }

        currentToken = data.token;
        currentAdminEmail = data.email || (adminLoginMode === "email" ? (adminEmail?.value.trim() || "Master Admin") : "Master Admin");

        const rememberMe = document.getElementById("rememberMe")?.checked;
        if (rememberMe) {
          localStorage.setItem("rd_admin_token", currentToken);
          localStorage.setItem("rd_admin_email", currentAdminEmail);
        } else {
          sessionStorage.setItem("rd_admin_token", currentToken);
          sessionStorage.setItem("rd_admin_email", currentAdminEmail);
        }

        set2FAMode(false);
        showToast("Connexion administrateur réussie !", "success");
        showApp();
      } catch (err) {
        showToast(err.message, "error");
      } finally {
        loginBtn.disabled = false;
        loginBtn.textContent = pending2FAChallenge ? "🔓 Valider le code 2FA" : "🔐 Accéder au Dashboard";
      }
    });
  }

  // Handle Password Setup submit
  const setupForm = document.getElementById("adminPasswordSetupForm");
  if (setupForm) {
    setupForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const submitBtn = document.getElementById("submitSetupPasswordBtn");
      const errBox = document.getElementById("setupPasswordErrorBox");
      if (errBox) {
        errBox.style.display = "none";
        errBox.textContent = "";
      }

      const licId = document.getElementById("setupAdminLicenseId")?.value.trim() || "";
      const licKey = document.getElementById("setupAdminLicenseKey")?.value.trim() || "";
      const newPass = document.getElementById("setupAdminNewPassword")?.value || "";
      const confirmPass = document.getElementById("setupAdminConfirmPassword")?.value || "";

      if (newPass.length < 8) {
        if (errBox) {
          errBox.textContent = "Le mot de passe doit contenir au moins 8 caractères.";
          errBox.style.display = "block";
        }
        return;
      }
      if (newPass !== confirmPass) {
        if (errBox) {
          errBox.textContent = "Les deux mots de passe ne correspondent pas.";
          errBox.style.display = "block";
        }
        return;
      }

      if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.textContent = "⏳ Enregistrement...";
      }

      try {
        const resp = await fetch(`${API_BASE_URL}/api/v1/admin/password/setup`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            license_id: licId,
            license_key: licKey,
            password: newPass
          })
        });

        const data = await resp.json();
        if (!resp.ok || !data.success) {
          throw new Error(data.error || "Impossible de configurer le mot de passe.");
        }

        showToast(data.message || "Mot de passe administrateur défini avec succès !", "success");
        closeModal("setupPasswordModal");

        // Pré-remplir l'adresse e-mail et se positionner en mode e-mail
        applyLoginMode("email");
        if (data.email && adminEmail) {
          adminEmail.value = data.email;
        }
        if (adminPassword) {
          adminPassword.value = "";
          adminPassword.focus();
        }
        setupForm.reset();
      } catch (err) {
        if (errBox) {
          errBox.textContent = err.message;
          errBox.style.display = "block";
        }
        showToast(err.message, "error");
      } finally {
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.textContent = "💾 Enregistrer le mot de passe";
        }
      }
    });
  }

  // Handle Logout
  if (logoutBtn) {
    logoutBtn.addEventListener("click", async () => {
      const tokenToRevoke = currentToken;
      if (tokenToRevoke) {
        try {
          await fetch(`${API_BASE_URL}/api/v1/admin/logout`, {
            method: "POST",
            headers: { "Authorization": `Bearer ${tokenToRevoke}` }
          });
        } catch (_) {
          // Local logout must still succeed if the network is unavailable.
        }
      }
      stopAutoRefresh();
      sessionStorage.removeItem("rd_admin_token");
      sessionStorage.removeItem("rd_admin_email");
      localStorage.removeItem("rd_admin_token");
      localStorage.removeItem("rd_admin_email");
      currentToken = "";
      document.getElementById("appScreen").style.display = "none";
      document.getElementById("authScreen").style.display = "flex";
      showToast("Déconnexion effectuée.", "success");
    });
  }
}

let autoRefreshTimer = null;

function startAutoRefresh() {
  stopAutoRefresh();
  // Rafraîchissement automatique toutes les 60 secondes pour maintenir l'état des services à jour et la session active
  autoRefreshTimer = setInterval(() => {
    if (!currentToken || document.hidden) return;
    if (currentTab === "overview") {
      fetchStats();
      fetchSystemStatus();
    }
  }, 60000);
}

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
  }
}

function handleSessionExpired(reason = "Votre session a expiré après une période d'inactivité. Veuillez vous reconnecter.") {
  stopAutoRefresh();
  sessionStorage.removeItem("rd_admin_token");
  sessionStorage.removeItem("rd_admin_email");
  localStorage.removeItem("rd_admin_token");
  localStorage.removeItem("rd_admin_email");
  currentToken = "";

  const appScreen = document.getElementById("appScreen");
  const authScreen = document.getElementById("authScreen");
  if (appScreen) appScreen.style.display = "none";
  if (authScreen) authScreen.style.display = "flex";

  showToast(reason, "warning");
}

document.addEventListener("visibilitychange", () => {
  if (!document.hidden && currentToken && currentTab === "overview") {
    fetchStats();
    fetchSystemStatus();
  }
});

function showApp() {
  const authScreen = document.getElementById("authScreen");
  const appScreen = document.getElementById("appScreen");
  if (authScreen) authScreen.style.display = "none";
  if (appScreen) appScreen.style.display = "flex";
  
  const emailDisplay = document.getElementById("adminEmailDisplay");
  if (emailDisplay) emailDisplay.textContent = currentAdminEmail;

  startAutoRefresh();
  switchTab("overview");
}

// =============================================================================
// 2. NAVIGATION & TABS
// =============================================================================
function initNavigation() {
  document.querySelectorAll(".nav-item").forEach(item => {
    item.addEventListener("click", () => {
      const tab = item.getAttribute("data-tab");
      switchTab(tab);
    });
  });

  const refreshBtn = document.getElementById("refreshBtn");
  if (refreshBtn) {
    refreshBtn.addEventListener("click", () => {
      refreshAll();
      showToast("Données actualisées", "success");
    });
  }
}

function debounce(fn, ms) {
  let timer = null;
  return (...args) => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => { timer = null; fn(...args); }, ms);
  };
}

function updatePager(pagerId, labelId, prevId, nextId, page, total, perPage, noun) {
  const pager = document.getElementById(pagerId);
  if (!pager) return;
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  const safePage = Math.min(Math.max(page, 1), totalPages);
  pager.style.display = totalPages > 1 ? "flex" : "none";
  const label = document.getElementById(labelId);
  if (label) label.textContent = `Page ${safePage} / ${totalPages} — ${total} ${noun}`;
  const prev = document.getElementById(prevId);
  const next = document.getElementById(nextId);
  if (prev) prev.disabled = safePage <= 1;
  if (next) next.disabled = safePage >= totalPages;
}

function initPagination() {
  const wire = (prevId, nextId, getPage, setPage, fetch) => {
    document.getElementById(prevId)?.addEventListener("click", () => {
      if (getPage() > 1) { setPage(getPage() - 1); fetch(); }
    });
    document.getElementById(nextId)?.addEventListener("click", () => { setPage(getPage() + 1); fetch(); });
  };
  wire("ordersPrevBtn", "ordersNextBtn", () => ordersPage, v => ordersPage = v, fetchOrders);
  wire("invoicesPrevBtn", "invoicesNextBtn", () => invoicesPage, v => invoicesPage = v, fetchInvoices);
  wire("licensesPrevBtn", "licensesNextBtn", () => licensesPage, v => licensesPage = v, fetchLicenses);
}

function switchTab(tabName) {
  currentTab = tabName;

  // Update nav items
  document.querySelectorAll(".nav-item").forEach(el => {
    el.classList.toggle("active", el.getAttribute("data-tab") === tabName);
  });

  // Update panes
  document.querySelectorAll(".tab-pane").forEach(pane => {
    pane.style.display = "none";
  });

  const titles = {
    overview: { title: "Vue d'ensemble", sub: "Supervision globale de l'infrastructure et des licences RelaisDesk" },
    licenses: { title: "Licences Techniciens", sub: "Liste complète, révocation, prolongation et consultation des clés" },
    create: { title: "Créer une Licence", sub: "Génération immédiate selon les plans commerciaux ou sur-mesure" },
    orders: { title: "Commandes & Paiements", sub: "Suivi des commandes, activation en 1 clic des virements et rapprochement crypto" },
    invoices: { title: "Gestion des Factures", sub: "Factures conformes micro-entreprise (art. 293 B du CGI) archivées sur le serveur" },
    alerts: { title: "Alertes de Sécurité", sub: "Journal d'audit anti-partage de codes et tentatives d'intrusion" },
    ledger: { title: "Livre des comptes", sub: "Registre chronologique des encaissements avec export CSV comptable" },
    unpaid: { title: "Impayés", sub: "Commandes en attente de paiement, triées par ancienneté" }
  };

  const currentInfo = titles[tabName] || { title: "Dashboard", sub: "" };
  const titleEl = document.getElementById("tabTitle");
  const subEl = document.getElementById("tabSubtitle");
  if (titleEl) titleEl.textContent = currentInfo.title;
  if (subEl) subEl.textContent = currentInfo.sub;

  const targetPane = document.getElementById(`tab${capitalize(tabName)}`);
  if (targetPane) {
    targetPane.style.display = "block";
  }

  // Fetch relevant tab data
  if (tabName === "overview") {
    fetchStats();
    fetchSystemStatus();
    fetchFinancials();
  }
  if (tabName === "licenses") fetchLicenses();
  if (tabName === "orders") fetchOrders();
  if (tabName === "invoices") fetchInvoices();
  if (tabName === "alerts") fetchAlerts();
  if (tabName === "ledger") fetchLedger();
  if (tabName === "unpaid") fetchUnpaid();
}

function capitalize(s) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function refreshAll() {
  fetchStats();
  fetchSystemStatus();
  fetchFinancials();
  if (currentTab === "licenses") fetchLicenses();
  if (currentTab === "orders") fetchOrders();
  if (currentTab === "invoices") fetchInvoices();
  if (currentTab === "alerts") fetchAlerts();
  if (currentTab === "ledger") fetchLedger();
  if (currentTab === "unpaid") fetchUnpaid();
}

// =============================================================================
// 3. STATISTIQUES & ANALYSE FINANCIÈRE
// =============================================================================
async function fetchStats() {
  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/stats`);
    if (!resp.ok) throw new Error();
    const data = await resp.json();

    const setTxt = (id, val) => {
      const el = document.getElementById(id);
      if (el) el.textContent = val;
    };

    setTxt("statTotal", data.total_licences || 0);
    setTxt("statActive", data.active_licences || 0);
    setTxt("statExpired", data.expired_licences || 0);
    setTxt("statRevoked", data.revoked_licences || 0);
    setTxt("statConnections", data.connection_count_available ? (data.current_connections || 0) : "hbbs");

  } catch (err) {
    const pill = document.getElementById("apiStatusPill");
    if (pill) {
      pill.className = "status-pill offline";
      pill.textContent = "🔴 API Déconnectée";
    }
  }
}

async function fetchSystemStatus() {
  const target = document.getElementById("systemStatusComponents");
  if (!target) return;
  target.innerHTML = `<span style="color: var(--text-muted);">Contrôle des services en cours...</span>`;

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/system-status`);
    if (!resp.ok) throw new Error("Supervision indisponible");
    const data = await resp.json();
    const components = Array.isArray(data.components) ? data.components : [];
    target.innerHTML = components.map(component => {
      const healthy = component.status === "healthy";
      const icon = healthy ? "✅" : "❌";
      const color = healthy ? "var(--success)" : "var(--danger)";
      return `<div style="padding: 0.7rem; border: 1px solid ${color}; border-radius: var(--radius-sm); background: rgba(15,23,42,0.45);">
        <strong style="color: ${color};">${icon} ${escapeHtml(component.name)}</strong><br>
        <small style="color: var(--text-secondary);">${escapeHtml(component.message)} — ${Number(component.latency_ms) || 0} ms</small>
      </div>`;
    }).join("");

    const pill = document.getElementById("apiStatusPill");
    if (pill) {
      const healthy = data.status === "healthy";
      pill.className = healthy ? "status-pill" : "status-pill offline";
      pill.textContent = healthy ? "🟢 Tous services opérationnels" : "🟠 Service dégradé";
    }
  } catch (err) {
    target.innerHTML = `<span style="color: var(--danger);">❌ ${escapeHtml(err.message)}</span>`;
    const pill = document.getElementById("apiStatusPill");
    if (pill) {
      pill.className = "status-pill offline";
      pill.textContent = "🔴 Supervision indisponible";
    }
  }
}

function initFinancials() {
  const urssafSlider = document.getElementById("urssafSlider");
  const vatSlider = document.getElementById("vatSlider");
  const feesSlider = document.getElementById("feesSlider");

  if (urssafSlider) {
    urssafSlider.addEventListener("input", (e) => {
      urssafRate = parseFloat(e.target.value) || 0;
      const d = document.getElementById("urssafRateDisplay");
      if (d) d.textContent = `${urssafRate.toFixed(1)} %`;
      renderFinancials();
    });
  }

  if (vatSlider) {
    vatSlider.addEventListener("input", (e) => {
      vatRate = parseFloat(e.target.value) || 0;
      const d = document.getElementById("vatRateDisplay");
      if (d) d.textContent = `${vatRate.toFixed(1)} %`;
      renderFinancials();
    });
  }

  if (feesSlider) {
    feesSlider.addEventListener("input", (e) => {
      feesRate = parseFloat(e.target.value) || 0;
      const d = document.getElementById("feesRateDisplay");
      if (d) d.textContent = `${feesRate.toFixed(1)} %`;
      renderFinancials();
    });
  }

  window.addEventListener("resize", () => {
    if (currentTab === "overview") drawRevenueChart();
  });
}

function setFiscalMode(mode) {
  fiscalMode = mode;
  document.getElementById("modeMicroBtn")?.classList.toggle("active", mode === "micro");
  document.getElementById("modeVatBtn")?.classList.toggle("active", mode === "vat");

  const vatGroup = document.getElementById("vatRateGroup");
  if (vatGroup) {
    vatGroup.style.display = mode === "vat" ? "block" : "none";
  }

  const bannerText = document.getElementById("fiscalBannerText");
  const bannerIcon = document.getElementById("fiscalBannerIcon");

  if (mode === "micro") {
    if (bannerIcon) bannerIcon.textContent = "🇫🇷";
    if (bannerText) {
      bannerText.innerHTML = `<strong>Régime Micro-Entreprise (Franchise en base de TVA - Art. 293 B du CGI)</strong> : Vous ne facturez pas de TVA (TVA 0%). Vos cotisations URSSAF sont calculées à ${urssafRate.toFixed(1)}% sur votre Chiffre d'Affaires brut encaissé.`;
    }
  } else {
    if (bannerIcon) bannerIcon.textContent = "🏢";
    if (bannerText) {
      bannerText.innerHTML = `<strong>Régime Réel (Entreprise assujettie à la TVA)</strong> : La TVA de ${vatRate.toFixed(1)}% est déduite du CA encaissé pour obtenir le CA Hors Taxes (HT). Vos bénéfices sont calculés sur la base HT.`;
    }
  }

  renderFinancials();
}

async function fetchFinancials() {
  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/financials`);
    if (!resp.ok) throw new Error();
    const data = await resp.json();
    financialsData = data;
  } catch (err) {
    console.warn("Impossible de charger les données financières:", err);
  }
  renderFinancials();
}

function renderFinancials() {
  try {
    const totalRev = financialsData?.total_revenue || 0;
    const mrr = financialsData?.monthly_recurring_revenue || 0;

    const mrrEl = document.getElementById("finMrrDisplay");
    if (mrrEl) mrrEl.textContent = `${mrr.toFixed(2)} €`;

    let caBrut = totalRev;
    let caHT = totalRev;
    let tvaAmount = 0;
    let tvaRateLabel = "0.00 % (Franchise Art. 293 B)";
    let chargesAmount = 0;
    let netProfit = 0;

    const tvaLabelEl = document.getElementById("finTvaLabel");
    const subRevEl = document.getElementById("finSubRev");

    if (fiscalMode === "micro") {
      if (tvaLabelEl) tvaLabelEl.textContent = "TVA COLLECTÉE";
      if (subRevEl) subRevEl.textContent = "Total des encaissements (TTC = HT)";
      tvaAmount = 0;
      tvaRateLabel = "0.00 % (Franchise Art. 293 B)";
      chargesAmount = caBrut * ((urssafRate + feesRate) / 100);
      netProfit = caBrut - chargesAmount;
    } else {
      if (tvaLabelEl) tvaLabelEl.textContent = `TVA COLLECTÉE (${vatRate.toFixed(1)}%)`;
      if (subRevEl) subRevEl.textContent = "Chiffre d'Affaires Brut TTC";
      caHT = caBrut / (1 + (vatRate / 100));
      tvaAmount = caBrut - caHT;
      tvaRateLabel = `${vatRate.toFixed(1)}% déduite (${tvaAmount.toFixed(2)} €)`;
      chargesAmount = caHT * ((urssafRate + feesRate) / 100);
      netProfit = caHT - chargesAmount;
    }

    const setTxt = (id, val) => {
      const el = document.getElementById(id);
      if (el) el.textContent = val;
    };

    setTxt("finTotalRevenue", `${caBrut.toFixed(2)} €`);
    setTxt("finTvaValue", `${tvaAmount.toFixed(2)} €`);
    setTxt("finSubTva", tvaRateLabel);
    setTxt("finChargesValue", `${chargesAmount.toFixed(2)} €`);
    setTxt("finSubCharges", `URSSAF (${urssafRate.toFixed(1)}%) + Frais (${feesRate.toFixed(1)}%)`);
    setTxt("finNetProfit", `${netProfit.toFixed(2)} €`);
    setTxt("finSubNet", fiscalMode === "micro" ? "Bénéfice net en poche (après URSSAF & frais)" : "Bénéfice net d'exploitation HT");

    // Defer chart drawing to ensure DOM paint
    requestAnimationFrame(() => {
      drawRevenueChart();
    });
  } catch (e) {
    console.error("renderFinancials error:", e);
  }
}

function drawRevenueChart() {
  try {
    const canvas = document.getElementById("revenueChart");
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const parentW = canvas.parentElement ? canvas.parentElement.clientWidth : 0;
    const rect = canvas.getBoundingClientRect();
    const w = Math.max(rect.width > 50 ? rect.width : (parentW > 50 ? parentW : 600), 200);
    const h = Math.max(rect.height > 50 ? rect.height : 230, 150);

    const dpr = window.devicePixelRatio || 1;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    ctx.resetTransform ? ctx.resetTransform() : ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.scale(dpr, dpr);

    ctx.clearRect(0, 0, w, h);

    let history = financialsData?.monthly_history || [];
    if (!Array.isArray(history) || history.length === 0 || (history.length === 1 && history[0].revenue === 0)) {
      const mrr = financialsData?.monthly_recurring_revenue || 50;
      const months = ["Mars", "Avr", "Mai", "Juin", "Juil", "Août"];
      history = months.map((m, idx) => ({
        label: m,
        revenue: Math.round(mrr * (0.4 + 0.15 * idx) * 100) / 100
      }));
    }

    const paddingLeft = 45;
    const paddingBottom = 25;
    const paddingTop = 20;
    const paddingRight = 20;
    const plotW = Math.max(w - paddingLeft - paddingRight, 50);
    const plotH = Math.max(h - paddingTop - paddingBottom, 50);

    let maxVal = 100;
    history.forEach(item => {
      if (item && item.revenue > maxVal) maxVal = item.revenue;
    });
    maxVal = Math.ceil(maxVal * 1.25 / 10) * 10;
    if (maxVal <= 0) maxVal = 100;

    // Grid Lines & Y Labels
    ctx.strokeStyle = "rgba(255, 255, 255, 0.08)";
    ctx.fillStyle = "#9ca3af";
    ctx.font = "11px Inter, sans-serif";
    ctx.textAlign = "right";

    const steps = 4;
    for (let i = 0; i <= steps; i++) {
      const yVal = (maxVal / steps) * i;
      const y = paddingTop + plotH - (plotH / steps) * i;
      ctx.beginPath();
      ctx.moveTo(paddingLeft, y);
      ctx.lineTo(w - paddingRight, y);
      ctx.stroke();

      ctx.fillText(`${Math.round(yVal)} €`, paddingLeft - 8, y + 4);
    }

    // Calculate coordinates
    const pointsCA = [];
    const pointsNet = [];
    const stepX = history.length > 1 ? plotW / (history.length - 1) : plotW / 2;

    history.forEach((item, idx) => {
      const x = history.length > 1 ? paddingLeft + idx * stepX : paddingLeft + plotW / 2;
      const ca = (item && typeof item.revenue === "number") ? item.revenue : 0;
      let net = 0;
      if (fiscalMode === "micro") {
        net = ca * (1 - (urssafRate + feesRate) / 100);
      } else {
        const caHT = ca / (1 + vatRate / 100);
        net = caHT * (1 - (urssafRate + feesRate) / 100);
      }

      const yCA = paddingTop + plotH - (ca / maxVal) * plotH;
      const yNet = paddingTop + plotH - (net / maxVal) * plotH;

      pointsCA.push({ x, y: yCA, val: ca, label: item?.label || item?.month || `M${idx+1}` });
      pointsNet.push({ x, y: yNet, val: net });

      ctx.textAlign = "center";
      ctx.fillStyle = "#9ca3af";
      ctx.fillText(item?.label || item?.month || `M${idx+1}`, x, h - 6);
    });

    function drawSeries(points, strokeColor, fillColor) {
      if (!points || points.length === 0) return;

      ctx.beginPath();
      ctx.moveTo(points[0].x, paddingTop + plotH);
      ctx.lineTo(points[0].x, points[0].y);
      for (let i = 1; i < points.length; i++) {
        ctx.lineTo(points[i].x, points[i].y);
      }
      ctx.lineTo(points[points.length - 1].x, paddingTop + plotH);
      ctx.closePath();
      ctx.fillStyle = fillColor;
      ctx.fill();

      ctx.beginPath();
      ctx.moveTo(points[0].x, points[0].y);
      for (let i = 1; i < points.length; i++) {
        ctx.lineTo(points[i].x, points[i].y);
      }
      ctx.strokeStyle = strokeColor;
      ctx.lineWidth = 2.5;
      ctx.stroke();

      points.forEach(p => {
        ctx.beginPath();
        ctx.arc(p.x, p.y, 4, 0, Math.PI * 2);
        ctx.fillStyle = "#0b0f19";
        ctx.fill();
        ctx.strokeStyle = strokeColor;
        ctx.lineWidth = 2;
        ctx.stroke();
      });
    }

    // Draw CA (Cyan)
    const gradCA = ctx.createLinearGradient(0, paddingTop, 0, paddingTop + plotH);
    gradCA.addColorStop(0, "rgba(56, 189, 248, 0.25)");
    gradCA.addColorStop(1, "rgba(56, 189, 248, 0.0)");
    drawSeries(pointsCA, "#38bdf8", gradCA);

    // Draw Net (Emerald)
    const gradNet = ctx.createLinearGradient(0, paddingTop, 0, paddingTop + plotH);
    gradNet.addColorStop(0, "rgba(16, 185, 129, 0.25)");
    gradNet.addColorStop(1, "rgba(16, 185, 129, 0.0)");
    drawSeries(pointsNet, "#10b981", gradNet);
  } catch (err) {
    console.error("drawRevenueChart error:", err);
  }
}

// =============================================================================
// 4. GESTION DES LICENCES
// =============================================================================
async function fetchLicenses() {
  const tbody = document.getElementById("licensesTableBody");
  tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-muted); padding: 2rem;">Chargement des licences...</td></tr>`;

  try {
    const q = document.getElementById("licenseSearchInput")?.value.trim() || "";
    const status = document.getElementById("licenseStatusFilter")?.value || "";
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/licences?unmask=${unmaskKeys}&status=${encodeURIComponent(status)}&q=${encodeURIComponent(q)}&page=${licensesPage}&limit=${PAGE_SIZE}`);
    if (!resp.ok) throw new Error("Impossible de récupérer les licences");
    const data = await resp.json();
    licensesData = data.licences || [];
    licensesTotal = data.total ?? licensesData.length;
    if (licensesData.length === 0 && licensesPage > 1) {
      licensesPage -= 1;
      return fetchLicenses();
    }
    updatePager("licensesPager", "licensesPageLabel", "licensesPrevBtn", "licensesNextBtn", licensesPage, licensesTotal, PAGE_SIZE, "licence(s)");
    renderLicensesTable();
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--danger); padding: 2rem;">❌ ${escapeHtml(err.message)}</td></tr>`;
  }
}

function renderLicensesTable() {
  const tbody = document.getElementById("licensesTableBody");
  // Filtrage effectué côté serveur (paramètres q et status) : on affiche la page reçue.
  const filtered = licensesData;

  if (filtered.length === 0) {
    tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-muted); padding: 2rem;">Aucune licence trouvée.</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.map(lic => {
    const badgeClass = lic.status === "active" ? "badge-active" : (lic.status === "expired" ? "badge-expired" : "badge-revoked");
    const statusLabel = lic.status === "active" ? "Active" : (lic.status === "expired" ? "Expirée" : "Révoquée");

    const expDate = lic.expires_at ? lic.expires_at.split(" ")[0] : "—";
    const notesTxt = lic.notes || "—";

    return `
      <tr>
        <td><strong style="color: var(--text-primary);">${escapeHtml(lic.license_id)}</strong></td>
        <td>${escapeHtml(lic.email)}</td>
        <td>
          <span class="key-code">${escapeHtml(lic.license_key)}</span>
          <button class="btn btn-secondary btn-sm" style="padding: 0.2rem 0.4rem; margin-left: 0.3rem;" data-action="copy-license" data-license-key="${escapeHtml(lic.license_key)}" title="Copier">📋</button>
        </td>
        <td>${Number(lic.max_connections) || 0} connexion(s) simultanée(s)</td>
        <td>${escapeHtml(expDate)}</td>
        <td><span class="badge ${badgeClass}">${statusLabel}</span></td>
        <td><span style="font-size: 0.85rem; color: var(--text-secondary);">${escapeHtml(notesTxt)}</span></td>
        <td>
          <div style="display: flex; gap: 0.4rem;">
            <button class="btn btn-secondary btn-sm" data-action="extend-license" data-license-id="${escapeHtml(lic.license_id)}" title="Prolonger (+30j)">🔄</button>
            ${lic.status !== "revoked" ? `<button class="btn btn-danger btn-sm" data-action="revoke-license" data-license-id="${escapeHtml(lic.license_id)}" title="Révoquer">🛑</button>` : ""}
          </div>
        </td>
      </tr>
    `;
  }).join("");
}

// Search & Filter Listeners
document.getElementById("licenseSearchInput")?.addEventListener("input", debounce(() => { licensesPage = 1; fetchLicenses(); }, 300));
document.getElementById("licenseStatusFilter")?.addEventListener("change", () => { licensesPage = 1; fetchLicenses(); });

// Toggle unmasked keys
document.getElementById("toggleUnmaskBtn")?.addEventListener("click", () => {
  unmaskKeys = !unmaskKeys;
  const btn = document.getElementById("toggleUnmaskBtn");
  btn.textContent = unmaskKeys ? "🔒 Masquer les clés" : "👁️ Afficher clés en clair";
  fetchLicenses();
});

// =============================================================================
// 5. CRÉATEUR DE LICENCES COMMERCIALES
// =============================================================================
function initPlanSelector() {
  const planCards = document.querySelectorAll(".plan-choice-card");
  const ultraOptions = document.getElementById("createUltraOptions");
  const customOptions = document.getElementById("createCustomOptions");
  const techSlider = document.getElementById("createTechSlider");
  const techCount = document.getElementById("createTechCount");

  planCards.forEach(card => {
    card.addEventListener("click", () => {
      planCards.forEach(c => c.classList.remove("selected"));
      card.classList.add("selected");
      selectedPlan = card.getAttribute("data-plan");

      ultraOptions.style.display = selectedPlan === "ultra" ? "block" : "none";
      customOptions.style.display = selectedPlan === "custom" ? "block" : "none";

      updateUltraPricing();
    });
  });

  if (techSlider) {
    techSlider.addEventListener("input", (e) => {
      techCount.textContent = e.target.value;
      updateUltraPricing();
    });
  }
}

function updateUltraPricing() {
  const techSlider = document.getElementById("createTechSlider");
  const priceDisplay = document.getElementById("createUltraPriceDisplay");
  if (!techSlider || !priceDisplay) return;

  const techs = parseInt(techSlider.value, 10) || 10;
  let price = 199;

  if (techs <= 10) {
    price = 199;
  } else if (techs <= 50) {
    price = 199 + (techs - 10) * 14;
  } else if (techs <= 100) {
    price = 759 + (techs - 50) * 10;
  } else {
    const extra = Math.min(techs - 100, 400);
    price = 1259 + extra * 7;
  }

  priceDisplay.innerHTML = `${price} € <span style="font-size: 0.8rem; color: var(--text-secondary);">/mois (${price * 10} €/an)</span>`;
}

function initForms() {
  const createForm = document.getElementById("createLicenseForm");
  if (createForm) {
    createForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const btn = document.getElementById("submitCreateBtn");
      const email = document.getElementById("createEmail").value.trim();
      const notes = document.getElementById("createNotes").value.trim();

      btn.disabled = true;
      btn.textContent = "⏳ Génération en cours...";

      try {
        let payload = {
          email: email,
          notes: notes,
          days: 30
        };

        if (selectedPlan === "starter") {
          payload.plan = "Starter";
          payload.max_connections = 1;
        } else if (selectedPlan === "pro") {
          payload.plan = "Pro";
          payload.max_connections = 5;
        } else if (selectedPlan === "ultra") {
          payload.plan = "Ultra";
          payload.technicians = parseInt(document.getElementById("createTechSlider").value, 10) || 10;
          payload.max_connections = payload.technicians;
        } else if (selectedPlan === "custom") {
          payload.max_connections = parseInt(document.getElementById("customConnections").value, 10) || 1;
          payload.days = parseInt(document.getElementById("customDays").value, 10) || 30;
        }

        const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/licences`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });

        const data = await resp.json();
        if (!resp.ok) {
          throw new Error(data.error || "Échec de création de la licence");
        }

        // Show Created Modal
        document.getElementById("resLicId").textContent = data.license_id;
        document.getElementById("resLicKey").textContent = data.license_key;
        document.getElementById("resLicEmail").textContent = data.email;
        document.getElementById("resLicMax").textContent = data.max_connections;
        document.getElementById("resLicExp").textContent = data.expires_at;

        // Copy button setup
        const copyBtn = document.getElementById("copyCreatedLicBtn");
        copyBtn.onclick = () => {
          const text = `Bonjour,\n\nVoici vos accès pour le service RelaisDesk :\n----------------------------------------\nIdentifiant Licence : ${data.license_id}\nClé de Licence      : ${data.license_key}\nNombre de techniciens : ${data.max_connections}\nDate d'expiration   : ${data.expires_at}\n----------------------------------------\nTéléchargement du Logiciel Technicien : https://relaisdesk.fr/downloads/RelaisDesk_Technicien_Setup_1.0.0.exe\n(Version portable : https://relaisdesk.fr/downloads/RelaisDesk_Technicien_Portable.exe)\n\nCordialement,\nL'équipe RelaisDesk\nhttps://relaisdesk.fr`;
          navigator.clipboard.writeText(text);
          showToast("Message client copié dans le presse-papiers !", "success");
        };

        openModal("createdModal");
        createForm.reset();
        showToast("Licence créée avec succès !", "success");
      } catch (err) {
        showToast(err.message, "error");
      } finally {
        btn.disabled = false;
        btn.textContent = "✨ Générer la Licence Immédiatement";
      }
    });
  }
  initInvoiceUploadForm();
  initTestEmailForm();
}

// =============================================================================
// 6. COMMANDES & VIREMENTS
// =============================================================================
async function fetchOrders() {
  const tbody = document.getElementById("ordersTableBody");
  tbody.innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--text-muted); padding: 2rem;">Chargement des commandes...</td></tr>`;

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/orders?page=${ordersPage}&limit=${PAGE_SIZE}`);
    if (!resp.ok) throw new Error("Impossible de récupérer les commandes");
    const data = await resp.json();
    const orders = data.orders || [];
    ordersTotal = data.total ?? orders.length;
    if (orders.length === 0 && ordersPage > 1) {
      ordersPage -= 1;
      return fetchOrders();
    }
    updatePager("ordersPager", "ordersPageLabel", "ordersPrevBtn", "ordersNextBtn", ordersPage, ordersTotal, PAGE_SIZE, "commande(s)");

    if (orders.length === 0) {
      tbody.innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--text-muted); padding: 2rem;">Aucune commande enregistrée.</td></tr>`;
      return;
    }

    const cryptoStatusFr = (status) => status === "paid" ? "réglé" : (status === "expired" ? "expiré" : (status === "cancelled" ? "annulé" : "en attente"));

    tbody.innerHTML = orders.map(ord => {
      const isPendingVirement = ord.status === "pending" && (ord.payment_method === "virement" || ord.payment_method === "bank_transfer");
      const isPendingCrypto = ord.status === "pending" && (ord.payment_method === "crypto_btc" || ord.payment_method === "crypto_xrp");
      const methodLabel = paymentMethodLabel(ord.payment_method);
      let cryptoDetail = "";
      if (ord.crypto) {
        cryptoDetail = `<br><small style="color: var(--text-secondary);">${escapeHtml(ord.crypto.amount_crypto || "—")} ${escapeHtml(ord.crypto.asset || "")} — devis ${cryptoStatusFr(ord.crypto.status)}</small>`;
        if (ord.crypto.txid) {
          cryptoDetail += `<br><small style="color: var(--text-secondary);" title="${escapeHtml(ord.crypto.txid)}">TX ${escapeHtml(ord.crypto.txid.slice(0, 18))}…</small>`;
        }
      }
      const statusBadge = ord.status === "paid" ? "badge-active" : (ord.status === "pending" ? "badge-expired" : "badge-revoked");
      const statusTxt = ord.status === "paid" ? "Payée" : (ord.status === "pending" ? "En attente" : "Annulée/Expirée");
      const dateStr = ord.created_at ? ord.created_at.split("T")[0] : "—";
      const billingDisplay = ord.billing_name ? `<br><small style="color: var(--text-secondary);">${escapeHtml(ord.billing_name)}</small>` : "";
      const stage = (done, label) => `<span title="${escapeHtml(label)}" style="white-space: nowrap;">${done ? "✅" : "❌"} ${escapeHtml(label)}</span>`;
      const fulfillment = [
        stage(ord.payment_complete, "Paiement"),
        stage(ord.license_created, "Licence"),
        stage(ord.invoice_created, "Facture"),
        stage(ord.license_email_sent, "Email licence"),
        stage(ord.invoice_email_sent, "Email facture")
      ].join("<br>");
      const needsRetry = ord.status === "paid" && !ord.fulfillment_done;

      return `
        <tr>
          <td><strong>${escapeHtml(ord.order_id)}</strong></td>
          <td>${escapeHtml(ord.email)}${billingDisplay}</td>
          <td><span class="badge badge-plan">${escapeHtml(ord.plan)}</span><br><small>${Number(ord.technicians) || 0} connexion(s) simultanée(s)</small></td>
          <td><strong style="color: var(--border-focus);">${(Number(ord.price) || 0).toFixed(2)} €</strong></td>
          <td>${methodLabel}${cryptoDetail}</td>
          <td><span class="badge ${statusBadge}">${statusTxt}</span></td>
          <td>${escapeHtml(dateStr)}</td>
          <td style="font-size: 0.76rem; line-height: 1.55;">${fulfillment}</td>
          <td>
            <div style="display: flex; gap: 0.35rem; align-items: center;">
              ${isPendingVirement ? `
                <button class="btn btn-success btn-sm" data-action="mark-order-paid" data-order-id="${escapeHtml(ord.order_id)}">
                  ✅ Valider Virement
                </button>
              ` : (isPendingCrypto ? `<span style="font-size: 0.8rem; color: var(--text-secondary);">⏳ Dépôt attendu (auto)</span>` : (needsRetry ? `
                <button class="btn btn-secondary btn-sm" data-action="retry-order-fulfillment" data-order-id="${escapeHtml(ord.order_id)}">
                  🔄 Relancer
                </button>
              ` : (ord.license_id ? `<span style="font-size: 0.8rem; color: var(--text-secondary);">${escapeHtml(ord.license_id)}</span>` : "—")))}
              ${ord.invoice_number ? `
                <button class="btn btn-secondary btn-sm" data-action="download-invoice" data-invoice-number="${escapeHtml(ord.invoice_number)}" title="Facture ${escapeHtml(ord.invoice_number)}">🧾</button>
              ` : ""}
              ${ord.status !== "paid" ? `<button class="btn btn-danger btn-sm" data-action="delete-order" data-order-id="${escapeHtml(ord.order_id)}" title="Supprimer la commande ${escapeHtml(ord.order_id)}">🗑️</button>` : ""}
            </div>
          </td>
        </tr>
      `;
    }).join("");
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--danger); padding: 2rem;">❌ ${escapeHtml(err.message)}</td></tr>`;
  }
}

async function deleteOrder(orderId) {
  if (!confirm(`⚠️ Confirmez-vous la suppression définitive de la commande ${orderId} ?\n\nCette action est irréversible.`)) {
    return;
  }

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/orders/${encodeURIComponent(orderId)}`, {
      method: "DELETE"
    });
    const data = await resp.json();
    if (!resp.ok) {
      throw new Error(data.error || "Échec de la suppression de la commande");
    }

    showToast(`Commande ${orderId} supprimée avec succès.`, "success");
    fetchOrders();
    fetchStats();
    fetchFinancials();
    fetchUnpaid();
  } catch (err) {
    showToast(err.message, "error");
  }
}

async function retryOrderFulfillment(orderId) {
  if (!confirm(`Relancer les étapes manquantes de la commande ${orderId} ?\n\nLes emails déjà envoyés ne seront pas renvoyés.`)) {
    return;
  }
  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/orders/${encodeURIComponent(orderId)}/retry-fulfillment`, { method: "POST" });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || "Échec de la relance");
    showToast(`Traitement de ${orderId} terminé.`, "success");
    fetchOrders();
  } catch (err) {
    showToast(err.message, "error");
  }
}

async function markOrderPaid(orderId) {
  if (!confirm(`Confirmez-vous la réception du virement pour la commande ${orderId} ?\n\nCela va créer immédiatement la licence client, générer la facture et expédier l'email.`)) {
    return;
  }

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/orders/${encodeURIComponent(orderId)}/mark-paid`, {
      method: "POST"
    });
    const data = await resp.json();
    if (!resp.ok) {
      throw new Error(data.error || "Échec de validation de la commande");
    }

    showToast(`Commande ${orderId} validée ! Licence créée et facture générée.`, "success");
    fetchOrders();
    fetchStats();
    fetchFinancials();
    fetchUnpaid();
  } catch (err) {
    showToast(err.message, "error");
  }
}

// =============================================================================
// 7. FACTURES & DOCUMENTS
// =============================================================================
async function fetchInvoices() {
  const tbody = document.getElementById("invoicesTableBody");
  if (!tbody) return;
  tbody.innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--text-muted); padding: 2rem;">Chargement des factures...</td></tr>`;

  try {
    const q = document.getElementById("invoiceSearchInput")?.value.trim() || "";
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/invoices?q=${encodeURIComponent(q)}&page=${invoicesPage}&limit=${PAGE_SIZE}`);
    if (!resp.ok) throw new Error("Impossible de récupérer les factures");
    const data = await resp.json();
    invoicesData = data.invoices || [];
    invoicesTotal = data.total ?? invoicesData.length;
    if (invoicesData.length === 0 && invoicesPage > 1) {
      invoicesPage -= 1;
      return fetchInvoices();
    }
    updatePager("invoicesPager", "invoicesPageLabel", "invoicesPrevBtn", "invoicesNextBtn", invoicesPage, invoicesTotal, PAGE_SIZE, "facture(s)");
    renderInvoicesTable();
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--danger); padding: 2rem;">❌ ${escapeHtml(err.message)}</td></tr>`;
  }
}

function renderInvoicesTable() {
  const tbody = document.getElementById("invoicesTableBody");
  if (!tbody) return;

  // Filtrage effectué côté serveur (paramètre q) : on affiche la page reçue.
  const filtered = invoicesData;

  if (filtered.length === 0) {
    tbody.innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--text-muted); padding: 2rem;">Aucune facture trouvée.</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.map(inv => {
    const dateStr = inv.created_at ? inv.created_at.split("T")[0] : "—";
    const originBadge = inv.is_manual ? `<span class="badge badge-plan">Manuelle</span>` : `<span class="badge badge-active">Auto</span>`;
    const siretStr = inv.customer_siret ? `<br><small style="color: var(--text-muted);">SIRET: ${escapeHtml(inv.customer_siret)}</small>` : "";
    const isCredit = inv.status === "credit_note" || (inv.invoice_number && inv.invoice_number.startsWith("AV-"));
    const statusBadge = isCredit
      ? `<span class="badge" style="background: rgba(239, 68, 68, 0.2); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.4);">Avoir</span>`
      : `<span class="badge badge-active">Payée</span>`;
    const amountColor = isCredit ? "#f87171" : "#34d399";
    const amountPrefix = isCredit ? "-" : "";

    return `
      <tr>
        <td><strong>${escapeHtml(inv.invoice_number)}</strong></td>
        <td>${escapeHtml(dateStr)}</td>
        <td><strong>${escapeHtml(inv.customer_name || "—")}</strong>${siretStr}</td>
        <td>${escapeHtml(inv.customer_email)}</td>
        <td><span class="badge badge-plan">${escapeHtml(inv.plan)}</span> (${Number(inv.technicians) || 0} tech)</td>
        <td><strong style="color: ${amountColor};">${amountPrefix}${(Number(inv.amount_ttc || inv.amount_ht) || 0).toFixed(2)} €</strong></td>
        <td>${statusBadge}</td>
        <td>${originBadge}</td>
        <td>
          <div style="display: flex; gap: 0.35rem; align-items: center;">
            <button class="btn btn-secondary btn-sm" data-action="download-invoice" data-invoice-number="${escapeHtml(inv.invoice_number)}" title="Visualiser / Imprimer la facture">👁️ Voir</button>
            <button class="btn btn-secondary btn-sm" data-action="download-invoice-cii" data-invoice-number="${escapeHtml(inv.invoice_number)}" title="Télécharger la facture électronique (Factur-X)">📄 XML</button>
            <button class="btn btn-secondary btn-sm" data-action="send-invoice" data-invoice-number="${escapeHtml(inv.invoice_number)}" title="Renvoyer au client par email">✉️</button>
            ${!isCredit ? `<button class="btn btn-secondary btn-sm" data-action="credit-note" data-invoice-number="${escapeHtml(inv.invoice_number)}" title="Émettre un avoir officiel" style="color: #f59e0b;">↩️ Avoir</button>` : ""}
          </div>
        </td>
      </tr>
    `;
  }).join("");
}

// Search listener
document.getElementById("invoiceSearchInput")?.addEventListener("input", debounce(() => { invoicesPage = 1; fetchInvoices(); }, 300));

// Ledger & unpaid filter listeners
document.getElementById("ledgerSearchInput")?.addEventListener("input", renderLedgerTable);
document.getElementById("ledgerMonthInput")?.addEventListener("change", renderLedgerTable);
document.getElementById("ledgerMethodFilter")?.addEventListener("change", renderLedgerTable);
document.getElementById("ledgerTypeFilter")?.addEventListener("change", renderLedgerTable);
document.getElementById("unpaidSearchInput")?.addEventListener("input", renderUnpaidTable);

async function downloadInvoice(invoiceNumber) {
  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/invoices/${encodeURIComponent(invoiceNumber)}/download`);
    if (!resp.ok) throw new Error("Erreur téléchargement facture");
    const blob = await resp.blob();
    const url = window.URL.createObjectURL(blob);
    window.open(url, "_blank");
  } catch (err) {
    showToast(err.message, "error");
  }
}

async function downloadInvoiceCII(invoiceNumber) {
  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/invoices/${encodeURIComponent(invoiceNumber)}/cii`);
    if (!resp.ok) throw new Error("Erreur téléchargement facture électronique");
    const blob = await resp.blob();
    const url = window.URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${invoiceNumber}.xml`;
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => window.URL.revokeObjectURL(url), 1000);
  } catch (err) {
    showToast(err.message, "error");
  }
}

async function sendInvoiceEmail(invoiceNumber) {
  if (!confirm(`Envoyer la facture ${invoiceNumber} par email au client ?`)) return;

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/invoices/${encodeURIComponent(invoiceNumber)}/send-email`, {
      method: "POST"
    });
    if (!resp.ok) throw new Error("Échec d'envoi de la facture");
    showToast(`Facture ${invoiceNumber} expédiée par email !`, "success");
  } catch (err) {
    showToast(err.message, "error");
  }
}

async function createCreditNote(invoiceNumber) {
  const reason = prompt(`Confirmez-vous la création d'un AVOIR officiel pour la facture ${invoiceNumber} ?\n\nMotif de l'avoir (ex: Annulation, Erreur de facturation) :`, "Remboursement / Annulation");
  if (reason === null) return;

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/invoices/${encodeURIComponent(invoiceNumber)}/credit-note`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ reason: reason.trim() })
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || "Échec création de l'avoir");
    showToast(`Avoir ${data.invoice_number} créé avec succès !`, "success");
    await fetchInvoices();
  } catch (err) {
    showToast(err.message, "error");
  }
}

function initInvoiceUploadForm() {
  const form = document.getElementById("uploadInvoiceForm");
  if (!form) return;

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = document.getElementById("submitUploadInvoiceBtn");
    const fileInput = document.getElementById("uploadInvoiceFile");
    if (!fileInput.files || fileInput.files.length === 0) {
      showToast("Veuillez sélectionner un fichier (PDF ou HTML)", "error");
      return;
    }

    btn.disabled = true;
    btn.textContent = "⏳ Upload en cours...";

    try {
      const formData = new FormData();
      formData.append("file", fileInput.files[0]);
      formData.append("customer_name", document.getElementById("uploadCustomerName").value.trim());
      formData.append("customer_email", document.getElementById("uploadCustomerEmail").value.trim());
      formData.append("invoice_number", document.getElementById("uploadInvoiceNumber").value.trim());
      formData.append("amount", document.getElementById("uploadAmount").value.trim());
      formData.append("plan", document.getElementById("uploadPlan").value);
      formData.append("order_id", document.getElementById("uploadOrderId").value.trim());
      formData.append("customer_address", document.getElementById("uploadCustomerAddress").value.trim());
      formData.append("customer_city", document.getElementById("uploadCustomerCity").value.trim());
      formData.append("customer_siret", document.getElementById("uploadCustomerSiret").value.trim());
      formData.append("notes", document.getElementById("uploadNotes").value.trim());

      const resp = await fetch(`${API_BASE_URL}/api/v1/admin/invoices/upload`, {
        method: "POST",
        headers: { "Authorization": `Bearer ${currentToken}` },
        body: formData
      });

      const data = await resp.json();
      if (!resp.ok) {
        throw new Error(data.error || "Échec de l'upload de la facture");
      }

      showToast(`Facture ${data.invoice_number} enregistrée avec succès !`, "success");
      closeModal("uploadInvoiceModal");
      form.reset();
      fetchInvoices();
    } catch (err) {
      showToast(err.message, "error");
    } finally {
      btn.disabled = false;
      btn.textContent = "📤 Enregistrer & Archiver";
    }
  });
}

function openTestEmailModal() {
  const box = document.getElementById("testEmailResultBox");
  if (box) box.style.display = "none";
  openModal("testEmailModal");
}

function initTestEmailForm() {
  const form = document.getElementById("testEmailForm");
  if (!form) return;

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = document.getElementById("submitTestEmailBtn");
    const targetInput = document.getElementById("testEmailTarget");
    const resultBox = document.getElementById("testEmailResultBox");

    const to = targetInput.value.trim();
    if (!to) {
      showToast("Veuillez saisir une adresse email de test", "error");
      return;
    }

    btn.disabled = true;
    btn.textContent = "⏳ Envoi du test en cours...";
    if (resultBox) resultBox.style.display = "none";

    try {
      const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/test-email`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ to: to })
      });

      const data = await resp.json();

      if (!resp.ok || !data.valid) {
        throw new Error(data.error || "Échec de connexion au serveur SMTP");
      }

      if (resultBox) {
        resultBox.style.display = "block";
        resultBox.style.background = "#ecfdf5";
        resultBox.style.border = "1px solid #a7f3d0";
        resultBox.style.color = "#065f46";
        resultBox.innerHTML = `✅ <strong>Succès !</strong><br>${escapeHtml(data.message)}<br><small style="color: #047857;">Serveur : ${escapeHtml(data.host || "SMTP")} (Port ${escapeHtml(data.port || "587")})</small>`;
      }
      showToast(`Email de test envoyé à ${to} !`, "success");
    } catch (err) {
      if (resultBox) {
        resultBox.style.display = "block";
        resultBox.style.background = "#fef2f2";
        resultBox.style.border = "1px solid #fecaca";
        resultBox.style.color = "#991b1b";
        resultBox.innerHTML = `❌ <strong>Échec SMTP :</strong><br>${escapeHtml(err.message)}`;
      }
      showToast("Échec SMTP : " + err.message, "error");
    } finally {
      btn.disabled = false;
      btn.textContent = "🚀 Envoyer l'Email de Test";
    }
  });
}

// =============================================================================
// 7. ALERTES DE SÉCURITÉ
// =============================================================================
async function fetchAlerts() {
  const tbody = document.getElementById("alertsTableBody");
  tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: var(--text-muted); padding: 2rem;">Chargement des alertes...</td></tr>`;

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/alerts`);
    if (!resp.ok) throw new Error("Impossible de récupérer les alertes");
    const data = await resp.json();
    const alerts = data.alerts || [];

    if (alerts.length === 0) {
      tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: var(--success); padding: 2rem;">🛡️ Aucune alerte de sécurité. Tous les flux sont conformes.</td></tr>`;
      return;
    }

    tbody.innerHTML = alerts.map(alt => {
      const dateStr = alt.created_at ? alt.created_at.split("T")[0] : "—";
      return `
        <tr>
          <td>${escapeHtml(dateStr)}</td>
          <td><span class="badge badge-revoked">${escapeHtml(alt.type)}</span></td>
          <td><code>${escapeHtml(alt.code || "—")}</code></td>
          <td>${escapeHtml(alt.first_ip || "—")}</td>
          <td><strong style="color: #f87171;">${escapeHtml(alt.second_ip || "—")}</strong></td>
          <td>${escapeHtml(alt.message)}</td>
        </tr>
      `;
    }).join("");
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: var(--danger); padding: 2rem;">❌ ${escapeHtml(err.message)}</td></tr>`;
  }
}

// =============================================================================
// 8. LIVRE DES COMPTES (RECETTES)
// =============================================================================
function paymentMethodLabel(method) {
  const labels = {
    stripe: "💳 Carte Bancaire",
    bank_transfer: "🏦 Virement",
    virement: "🏦 Virement",
    crypto_btc: "₿ Bitcoin (BTC)",
    crypto_xrp: "◈ XRP"
  };
  return labels[method] || escapeHtml(method || "—");
}

function paymentMethodText(method) {
  const labels = {
    stripe: "Carte bancaire",
    bank_transfer: "Virement",
    virement: "Virement",
    crypto_btc: "Bitcoin (BTC)",
    crypto_xrp: "XRP"
  };
  return labels[method] || method || "";
}

function methodGroup(method) {
  if (method === "virement" || method === "bank_transfer") return "bank_transfer";
  return method || "";
}

function formatDateFR(iso) {
  if (!iso) return "—";
  const parts = String(iso).split("T")[0].split("-");
  if (parts.length !== 3) return String(iso);
  return `${parts[2]}/${parts[1]}/${parts[0]}`;
}

function isCreditNote(inv) {
  return !!inv && (inv.status === "credit_note" || (inv.invoice_number && inv.invoice_number.startsWith("AV-")));
}

// Construit les lignes du livre : commandes payées + factures manuelles
// orphelines (sans commande associée) + avoirs en négatif.
// Les factures automatiques ne sont pas ajoutées séparément : elles sont déjà
// comptées via leurs commandes (pas de double comptage).
// Tri chronologique croissant (exigence du livre des recettes).
function buildLedgerRows(orders, invoices) {
  const rows = [];
  const paidOrderIds = new Set();
  const paidInvoiceNumbers = new Set();

  (orders || []).forEach(ord => {
    if (!ord || ord.status !== "paid") return;
    if (ord.order_id) paidOrderIds.add(ord.order_id);
    if (ord.invoice_number) paidInvoiceNumbers.add(ord.invoice_number);
    rows.push({
      dateISO: ord.paid_at || ord.created_at || "",
      type: "recette",
      orderId: ord.order_id || "",
      invoiceNumber: ord.invoice_number || "",
      client: ord.billing_name || "",
      email: ord.email || "",
      plan: ord.plan || "",
      technicians: Number(ord.technicians) || 0,
      orderKind: ord.order_kind || "initial",
      amount: Number(ord.price) || 0,
      method: ord.payment_method || ""
    });
  });

  const methodByOrder = {};
  (orders || []).forEach(ord => {
    if (ord && ord.order_id) methodByOrder[ord.order_id] = ord.payment_method || "";
  });

  (invoices || []).forEach(inv => {
    if (!inv) return;
    if (isCreditNote(inv)) {
      rows.push({
        dateISO: inv.created_at || "",
        type: "avoir",
        orderId: inv.order_id || "",
        invoiceNumber: inv.invoice_number || "",
        client: inv.customer_name || "",
        email: inv.customer_email || "",
        plan: inv.plan || "",
        technicians: Number(inv.technicians) || 0,
        orderKind: "",
        amount: -(Number(inv.amount_ttc || inv.amount_ht) || 0),
        method: (inv.order_id && methodByOrder[inv.order_id]) || ""
      });
      return;
    }
    if (!inv.is_manual) return;
    if (inv.order_id && paidOrderIds.has(inv.order_id)) return;
    if (inv.invoice_number && paidInvoiceNumbers.has(inv.invoice_number)) return;
    rows.push({
      dateISO: inv.created_at || "",
      type: "recette",
      orderId: inv.order_id || "",
      invoiceNumber: inv.invoice_number || "",
      client: inv.customer_name || "",
      email: inv.customer_email || "",
      plan: inv.plan || "",
      technicians: Number(inv.technicians) || 0,
      orderKind: "",
      amount: Number(inv.amount_ttc || inv.amount_ht) || 0,
      method: (inv.order_id && methodByOrder[inv.order_id]) || ""
    });
  });

  rows.sort((a, b) => (Date.parse(a.dateISO) || 0) - (Date.parse(b.dateISO) || 0));
  return rows;
}

function filterLedgerRows(rows, filters) {
  const f = filters || {};
  const q = (f.search || "").trim().toLowerCase();
  return (rows || []).filter(r => {
    if (f.type && r.type !== f.type) return false;
    if (f.method && methodGroup(r.method) !== f.method) return false;
    if (f.month && (r.dateISO || "").slice(0, 7) !== f.month) return false;
    if (!q) return true;
    return [r.orderId, r.invoiceNumber, r.client, r.email, r.plan]
      .some(v => (v || "").toLowerCase().includes(q));
  });
}

function csvCell(value) {
  const s = value === null || value === undefined ? "" : String(value);
  return /[;"\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

function formatAmountCSV(amount) {
  return (Number(amount) || 0).toFixed(2).replace(".", ",");
}

// Export CSV français : séparateur point-virgule, décimales à virgule, BOM UTF-8.
function ledgerToCSV(rows) {
  const header = ["Date", "Type", "N° Commande", "N° Facture", "Client", "Email", "Prestation", "Montant (EUR)", "Moyen de paiement"];
  const lines = [header.map(csvCell).join(";")];
  (rows || []).forEach(r => {
    const prestation = r.orderKind === "renewal" && r.plan
      ? `${r.plan} (renouvellement)`
      : (r.plan || "");
    lines.push([
      formatDateFR(r.dateISO),
      r.type === "avoir" ? "Avoir" : "Recette",
      r.orderId,
      r.invoiceNumber,
      r.client,
      r.email,
      prestation,
      formatAmountCSV(r.amount),
      paymentMethodText(r.method)
    ].map(csvCell).join(";"));
  });
  return "\ufeff" + lines.join("\r\n") + "\r\n";
}

function downloadCSV(filename, content) {
  const blob = new Blob([content], { type: "text/csv;charset=utf-8" });
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => window.URL.revokeObjectURL(url), 1000);
}

async function fetchLedger() {
  const tbody = document.getElementById("ledgerTableBody");
  if (tbody) tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-muted); padding: 2rem;">Chargement du livre des comptes...</td></tr>`;

  try {
    const [ordersResp, invoicesResp] = await Promise.all([
      authFetch(`${API_BASE_URL}/api/v1/admin/orders`),
      authFetch(`${API_BASE_URL}/api/v1/admin/invoices`)
    ]);
    if (!ordersResp.ok) throw new Error("Impossible de récupérer les commandes");
    if (!invoicesResp.ok) throw new Error("Impossible de récupérer les factures");
    const ordersData = await ordersResp.json();
    const invoicesDataResp = await invoicesResp.json();
    ledgerCache.orders = ordersData.orders || [];
    ledgerCache.invoices = invoicesDataResp.invoices || [];
    renderLedgerTable();
  } catch (err) {
    if (tbody) tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--danger); padding: 2rem;">❌ ${escapeHtml(err.message)}</td></tr>`;
  }
}

function renderLedgerTable() {
  const tbody = document.getElementById("ledgerTableBody");
  if (!tbody) return;

  const rows = buildLedgerRows(ledgerCache.orders, ledgerCache.invoices);
  const filtered = filterLedgerRows(rows, {
    search: document.getElementById("ledgerSearchInput")?.value || "",
    month: document.getElementById("ledgerMonthInput")?.value || "",
    method: document.getElementById("ledgerMethodFilter")?.value || "",
    type: document.getElementById("ledgerTypeFilter")?.value || ""
  });
  currentLedgerRows = filtered;

  const setTxt = (id, val) => {
    const el = document.getElementById(id);
    if (el) el.textContent = val;
  };
  const total = rows.reduce((s, r) => s + r.amount, 0);
  const receiptCount = rows.filter(r => r.type === "recette").length;
  const creditTotal = rows.filter(r => r.type === "avoir").reduce((s, r) => s + r.amount, 0);
  setTxt("ledgerTotal", `${total.toFixed(2)} €`);
  setTxt("ledgerCount", receiptCount);
  setTxt("ledgerCredits", `${creditTotal.toFixed(2)} €`);

  if (filtered.length === 0) {
    tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-muted); padding: 2rem;">Aucune ligne dans le livre des comptes.</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.map(r => {
    const isCredit = r.type === "avoir";
    const typeBadge = isCredit
      ? `<span class="badge badge-revoked">Avoir</span>`
      : `<span class="badge badge-active">Recette</span>`;
    const kindLabel = r.orderKind === "renewal" ? `<br><small style="color: var(--text-secondary);">renouvellement</small>` : "";
    const techLabel = r.technicians > 0 ? ` <small style="color: var(--text-secondary);">(${r.technicians} tech)</small>` : "";
    const color = isCredit ? "#f87171" : "#34d399";
    return `
      <tr>
        <td>${escapeHtml(formatDateFR(r.dateISO))}</td>
        <td>${typeBadge}</td>
        <td>${r.orderId ? `<strong>${escapeHtml(r.orderId)}</strong>` : "—"}</td>
        <td>${r.invoiceNumber ? `<button class="btn btn-secondary btn-sm" data-action="download-invoice" data-invoice-number="${escapeHtml(r.invoiceNumber)}" title="Voir ${escapeHtml(r.invoiceNumber)}">🧾 ${escapeHtml(r.invoiceNumber)}</button>` : "—"}</td>
        <td><strong>${escapeHtml(r.client || "—")}</strong>${r.email ? `<br><small style="color: var(--text-secondary);">${escapeHtml(r.email)}</small>` : ""}</td>
        <td><span class="badge badge-plan">${escapeHtml(r.plan || "—")}</span>${techLabel}${kindLabel}</td>
        <td><strong style="color: ${color};">${r.amount.toFixed(2)} €</strong></td>
        <td>${paymentMethodLabel(r.method)}</td>
      </tr>
    `;
  }).join("");
}

function exportLedgerCSV() {
  if (!currentLedgerRows || currentLedgerRows.length === 0) {
    showToast("Aucune ligne à exporter avec les filtres actuels.", "error");
    return;
  }
  const stamp = new Date().toISOString().split("T")[0];
  downloadCSV(`livre-comptes-${stamp}.csv`, ledgerToCSV(currentLedgerRows));
  const total = currentLedgerRows.reduce((s, r) => s + r.amount, 0);
  showToast(`${currentLedgerRows.length} ligne(s) exportée(s) — total ${total.toFixed(2)} €.`, "success");
}

// =============================================================================
// 9. IMPAYÉS
// =============================================================================
function unpaidAgeDays(iso, nowMs) {
  const t = iso ? Date.parse(iso) : NaN;
  if (Number.isNaN(t)) return 0;
  const now = typeof nowMs === "number" ? nowMs : Date.now();
  return Math.max(0, Math.floor((now - t) / 86400000));
}

async function fetchUnpaid() {
  const tbody = document.getElementById("unpaidTableBody");
  if (tbody) tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-muted); padding: 2rem;">Chargement des impayés...</td></tr>`;

  try {
    const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/orders`);
    if (!resp.ok) throw new Error("Impossible de récupérer les commandes");
    const data = await resp.json();
    unpaidCache = (data.orders || []).filter(o => o && (o.status === "pending" || o.status === "processing"));
    unpaidCache.sort((a, b) => (Date.parse(a.created_at) || 0) - (Date.parse(b.created_at) || 0));
    renderUnpaidTable();
  } catch (err) {
    if (tbody) tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--danger); padding: 2rem;">❌ ${escapeHtml(err.message)}</td></tr>`;
  }
}

function renderUnpaidTable() {
  const tbody = document.getElementById("unpaidTableBody");
  if (!tbody) return;

  const q = (document.getElementById("unpaidSearchInput")?.value || "").trim().toLowerCase();
  const filtered = unpaidCache.filter(o => {
    return !q || [o.order_id, o.billing_name, o.email].some(v => (v || "").toLowerCase().includes(q));
  });

  const setTxt = (id, val) => {
    const el = document.getElementById(id);
    if (el) el.textContent = val;
  };
  const total = unpaidCache.reduce((s, o) => s + (Number(o.price) || 0), 0);
  setTxt("unpaidCount", unpaidCache.length);
  setTxt("unpaidTotal", `${total.toFixed(2)} €`);

  if (filtered.length === 0) {
    const msg = unpaidCache.length === 0
      ? "🎉 Aucun impayé. Toutes les commandes sont à jour."
      : "Aucune commande ne correspond à la recherche.";
    const color = unpaidCache.length === 0 ? "var(--success)" : "var(--text-muted)";
    tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: ${color}; padding: 2rem;">${msg}</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.map(o => {
    const isVirement = o.payment_method === "virement" || o.payment_method === "bank_transfer";
    const isCrypto = o.payment_method === "crypto_btc" || o.payment_method === "crypto_xrp";
    const age = unpaidAgeDays(o.created_at);
    const ageColor = age > 30 ? "#f87171" : (age > 7 ? "var(--warning)" : "var(--text-primary)");
    const statusHint = o.status === "processing" ? "Paiement en cours" : "En attente";
    const expiresStr = o.expires_at ? String(o.expires_at).split("T")[0] : "—";
    let payDetail = "";
    if (isCrypto && o.crypto) {
      payDetail = `<br><small style="color: var(--text-secondary);">${escapeHtml(o.crypto.amount_crypto || "—")} ${escapeHtml(o.crypto.asset || "")} — dépôt attendu</small>`;
    } else if (isCrypto) {
      payDetail = `<br><small style="color: var(--text-secondary);">dépôt attendu (auto)</small>`;
    }
    return `
      <tr>
        <td><strong>${escapeHtml(o.order_id)}</strong><br><small style="color: var(--text-secondary);">${escapeHtml(statusHint)}</small></td>
        <td>${escapeHtml(o.email || "—")}${o.billing_name ? `<br><small style="color: var(--text-secondary);">${escapeHtml(o.billing_name)}</small>` : ""}</td>
        <td><span class="badge badge-plan">${escapeHtml(o.plan || "—")}</span><br><small>${Number(o.technicians) || 0} connexion(s)</small></td>
        <td><strong style="color: var(--border-focus);">${(Number(o.price) || 0).toFixed(2)} €</strong></td>
        <td>${paymentMethodLabel(o.payment_method)}${payDetail}</td>
        <td><strong style="color: ${ageColor};">${age} j</strong></td>
        <td>${escapeHtml(expiresStr)}</td>
        <td>
          <div style="display: flex; gap: 0.35rem; align-items: center;">
            ${o.status === "pending" && isVirement ? `
              <button class="btn btn-success btn-sm" data-action="mark-order-paid" data-order-id="${escapeHtml(o.order_id)}">
                ✅ Valider Virement
              </button>
            ` : ""}
            ${o.invoice_number ? `
              <button class="btn btn-secondary btn-sm" data-action="download-invoice" data-invoice-number="${escapeHtml(o.invoice_number)}" title="Facture ${escapeHtml(o.invoice_number)}">🧾</button>
            ` : ""}
            <button class="btn btn-danger btn-sm" data-action="delete-order" data-order-id="${escapeHtml(o.order_id)}" title="Supprimer la commande ${escapeHtml(o.order_id)}">🗑️</button>
          </div>
        </td>
      </tr>
    `;
  }).join("");
}

// =============================================================================
// 10. MODALS & ACTIONS
// =============================================================================
function initModals() {
  // Confirm Revoke
  document.getElementById("confirmRevokeBtn")?.addEventListener("click", async () => {
    const reason = document.getElementById("revokeReasonInput").value.trim() || "Révocation manuelle admin";
    try {
      const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/licences/${encodeURIComponent(targetLicenseId)}/revoke`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: reason })
      });
      if (!resp.ok) throw new Error("Échec de la révocation");
      showToast(`Licence ${targetLicenseId} révoquée.`, "success");
      closeModal("revokeModal");
      fetchLicenses();
      fetchStats();
    } catch (err) {
      showToast(err.message, "error");
    }
  });

  // Confirm Extend
  document.getElementById("confirmExtendBtn")?.addEventListener("click", async () => {
    const days = parseInt(document.getElementById("extendDaysSelect").value, 10) || 30;
    try {
      const resp = await authFetch(`${API_BASE_URL}/api/v1/admin/licences/${encodeURIComponent(targetLicenseId)}/extend`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ days: days })
      });
      if (!resp.ok) throw new Error("Échec de la prolongation");
      showToast(`Licence ${targetLicenseId} prolongée de ${days} jours.`, "success");
      closeModal("extendModal");
      fetchLicenses();
      fetchStats();
    } catch (err) {
      showToast(err.message, "error");
    }
  });
}

function openRevokeModal(licenseId) {
  targetLicenseId = licenseId;
  document.getElementById("revokeLicenseIdDisplay").textContent = licenseId;
  document.getElementById("revokeReasonInput").value = "";
  openModal("revokeModal");
}

function openExtendModal(licenseId) {
  targetLicenseId = licenseId;
  document.getElementById("extendLicenseIdDisplay").textContent = licenseId;
  openModal("extendModal");
}

function openModal(id) {
  document.getElementById(id)?.classList.add("open");
}

function closeModal(id) {
  document.getElementById(id)?.classList.remove("open");
}

// =============================================================================
let isHandling401 = false;

async function authFetch(url, options = {}) {
  options.headers = options.headers || {};
  if (currentToken) {
    options.headers["Authorization"] = `Bearer ${currentToken}`;
  }
  const resp = await fetch(url, options);
  if (resp.status === 401 && currentToken) {
    if (!isHandling401) {
      isHandling401 = true;
      handleSessionExpired("Votre session a expiré après une période d'inactivité. Veuillez vous reconnecter.");
      setTimeout(() => { isHandling401 = false; }, 2500);
    }
  }
  return resp;
}

function copyLicKey(key) {
  navigator.clipboard.writeText(key).then(() => {
    showToast("Clé copiée dans le presse-papiers !", "success");
  });
}

function showToast(message, type = "success") {
  const container = document.getElementById("toastContainer");
  if (!container) return;

  const toast = document.createElement("div");
  toast.className = `toast ${type}`;
  toast.innerHTML = `<span>${type === "success" ? "✅" : "⚠️"}</span> <span>${escapeHtml(message)}</span>`;
  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = "0";
    toast.style.transform = "translateX(100%)";
    toast.style.transition = "all 0.3s ease";
    setTimeout(() => toast.remove(), 300);
  }, 3500);
}

function escapeHtml(text) {
  if (!text) return "";
  return String(text)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
