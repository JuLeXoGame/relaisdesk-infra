package main

import (
	"context"
	"fmt"
	"image/color"
	"net/url"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/skip2/go-qrcode"
)

var (
	customerCardBorder = color.NRGBA{R: 35, G: 55, B: 85, A: 140}
	customerCardFill   = color.NRGBA{R: 14, G: 20, B: 35, A: 255}
)

// Menu latéral miroir de l'espace client web : mêmes rubriques, même ordre.
// Les rubriques technicien (codes, parc) réutilisent la session licence ;
// les autres demandent la session compte client, ouverte à la volée.
const (
	PanelOverview = iota
	PanelCodes
	PanelFleet
	PanelTeam
	PanelLicenses
	PanelBilling
	PanelHistory
	PanelServices
	PanelSettings
)

type sidebarEntry struct {
	id            int
	labelKey      string
	svgPaths      string
	needsCustomer bool
}

func sidebarEntries(restricted bool) []sidebarEntry {
	entries := []sidebarEntry{
		{PanelOverview, "nav_overview", `<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>`, true},
		{PanelCodes, "nav_codes", `<circle cx="8" cy="15" r="4"/><path d="m11 12 9-9"/><path d="m17 5 2 2"/><path d="m14 8 2 2"/>`, false},
		{PanelFleet, "nav_fleet", `<rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8"/><path d="M12 17v4"/>`, false},
		{PanelTeam, "nav_team", `<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>`, true},
		{PanelLicenses, "nav_licenses", `<circle cx="12" cy="8" r="6"/><path d="M15.5 13 17 22l-5-3-5 3 1.5-9"/>`, true},
		{PanelBilling, "nav_billing", `<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M16 13H8"/><path d="M16 17H8"/>`, true},
		{PanelHistory, "nav_history", `<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>`, true},
		{PanelServices, "nav_services", `<rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"/>`, true},
		{PanelSettings, "nav_settings", `<path d="M4 21v-7"/><path d="M4 10V3"/><path d="M12 21v-9"/><path d="M12 8V3"/><path d="M20 21v-5"/><path d="M20 12V3"/><path d="M1 14h6"/><path d="M9 8h6"/><path d="M17 16h6"/>`, true},
	}
	if restricted {
		kept := entries[:0]
		for _, e := range entries {
			if e.id != PanelCodes {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	return entries
}

var sidebarIconCache = map[int]fyne.Resource{}

func sidebarIcon(entry sidebarEntry) fyne.Resource {
	if res, ok := sidebarIconCache[entry.id]; ok {
		return res
	}
	doc := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#bfdbfe" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + entry.svgPaths + `</svg>`
	res := fyne.NewStaticResource(fmt.Sprintf("nav-%d.svg", entry.id), []byte(doc))
	sidebarIconCache[entry.id] = res
	return res
}

func collapseIconResource() fyne.Resource {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#bfdbfe" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m11 17-5-5 5-5"/><path d="m18 17-5-5 5-5"/></svg>`
	return fyne.NewStaticResource("nav-collapse.svg", []byte(doc))
}

func expandIconResource() fyne.Resource {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#bfdbfe" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m13 17 5-5-5-5"/><path d="m6 17 5-5-5-5"/></svg>`
	return fyne.NewStaticResource("nav-expand.svg", []byte(doc))
}

var fleetTreeIconCache = map[string]fyne.Resource{}

// fleetTreeIcon fournit les icônes de l'explorateur de parc (comme le web).
func fleetTreeIcon(kind string) fyne.Resource {
	if res, ok := fleetTreeIconCache[kind]; ok {
		return res
	}
	var paths string
	switch kind {
	case "root":
		paths = `<path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9 22V12h6v10"/>`
	default:
		paths = `<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>`
	}
	doc := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#7cb3f5" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`
	res := fyne.NewStaticResource("tree-"+kind+".svg", []byte(doc))
	fleetTreeIconCache[kind] = res
	return res
}

// buildSidebarNav construit la colonne de navigation. onSelect reçoit
// l'identifiant du panneau ; footer (compte + déconnexion) est ancré en bas.
func buildSidebarNav(selected int, collapsed bool, restricted bool, onSelect func(int), footer fyne.CanvasObject) fyne.CanvasObject {
	items := []fyne.CanvasObject{}
	for _, entry := range sidebarEntries(restricted) {
		e := entry
		label := T(e.labelKey)
		if collapsed {
			label = ""
		}
		btn := widget.NewButtonWithIcon(label, sidebarIcon(e), func() {
			if e.id != selected {
				onSelect(e.id)
			}
		})
		btn.Importance = widget.MediumImportance
		if e.id == selected {
			btn.Importance = widget.HighImportance
		}
		btn.Alignment = widget.ButtonAlignLeading
		items = append(items, btn)
	}
	nav := container.NewVBox(items...)
	if footer != nil {
		return container.NewBorder(nil, footer, nil, nil, nav)
	}
	return nav
}

// sidebarSplitOffset donne le ratio du séparateur latéral : colonne
// étroite (icônes seules) quand replié, confortable sinon. totalWidth et
// navMinWidth sont les largeurs rendues (0 si inconnues -> repli par défaut).
func sidebarSplitOffset(collapsed bool, totalWidth, navMinWidth float32) float64 {
	const expandedOffset = 0.24
	if !collapsed {
		return expandedOffset
	}
	if totalWidth > 200 && navMinWidth > 0 {
		ratio := float64((navMinWidth + 8) / totalWidth)
		if ratio < 0.02 {
			return 0.02
		}
		if ratio > 0.12 {
			return 0.12
		}
		return ratio
	}
	return 0.05
}

// toggleSidebarCollapsed inverse l'état du panneau, le persiste et
// retourne le nouvel état (le réaffichage reste à l'appelant, sans
// reconstruction de l'écran).
func toggleSidebarCollapsed(collapsed bool) bool {
	collapsed = !collapsed
	if app := fyne.CurrentApp(); app != nil {
		app.Preferences().SetBool("sidebar_collapsed", collapsed)
	}
	return collapsed
}

func isCustomerSessionExpired(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Session client invalide")
}

// asyncCustomerPanel charge le tableau de bord puis construit le contenu.
// Session expirée -> écran de connexion ; autre erreur -> message + réessai.
func asyncCustomerPanel(panelID int, relaunch func(int), build func(*customerDashboard) fyne.CanvasObject) fyne.CanvasObject {
	box := container.NewMax()
	showLoading := func() {
		box.Objects = []fyne.CanvasObject{container.NewCenter(
			widget.NewLabelWithStyle(T("loading_dashboard"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
		)}
		box.Refresh()
	}
	var load func()
	load = func() {
		showLoading()
		go func() {
			token := getCustomerSessionToken()
			dash, err := getCustomerDashboard(token)
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						box.Objects = []fyne.CanvasObject{customerLoginPanel(panelID, relaunch, T("customer_session_expired"))}
						box.Refresh()
						return
					}
					retry := widget.NewButton(T("refresh_btn"), func() { load() })
					box.Objects = []fyne.CanvasObject{container.NewCenter(container.NewVBox(
						widget.NewLabelWithStyle(err.Error(), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
						retry,
					))}
					box.Refresh()
					return
				}
				box.Objects = []fyne.CanvasObject{container.NewVScroll(build(dash))}
				box.Refresh()
			})
		}()
	}
	load()
	return box
}

func formatCustomerDateRFC3339(raw string) string {
	if raw == "" {
		return "—"
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		if parsed.UTC().Year() == 9999 {
			return T("license_no_end_date")
		}
		return parsed.Local().Format("02/01/2006")
	}
	return raw
}

func formatCustomerDateUnix(sec int64) string {
	if sec <= 0 {
		return "—"
	}
	return time.Unix(sec, 0).Local().Format("02/01/2006 15:04")
}

func formatEUR(amount float64) string {
	return fmt.Sprintf("%.2f €", amount)
}

// customerCardWidth est la largeur confortable des cartes compte client.
const customerCardWidth = 440

// centeredCardScroll centre une carte horizontalement tout en lui donnant
// toute la hauteur disponible (avec défilement si la fenêtre est petite).
// Ne jamais envelopper un VScroll dans container.NewCenter : la hauteur
// minimale d'un scroll vertical vaut 32px (Fyne), Center réduirait donc
// la carte à une vignette inutilisable.
func centeredCardScroll(card fyne.CanvasObject) fyne.CanvasObject {
	scroll := container.NewVScroll(container.NewPadded(card))
	scroll.SetMinSize(fyne.NewSize(customerCardWidth, 0))
	return container.NewHBox(layout.NewSpacer(), scroll, layout.NewSpacer())
}

var errorURLPattern = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

// errorURLs extrait les liens http(s) d'un message d'erreur (dédupliqués,
// ponctuation finale retirée, hôte requis).
func errorURLs(msg string) []string {
	raw := errorURLPattern.FindAllString(msg, -1)
	seen := map[string]bool{}
	out := []string{}
	for _, u := range raw {
		u = strings.TrimRight(u, ".,;:!?")
		if u == "" || seen[u] {
			continue
		}
		if parsed, err := url.Parse(u); err != nil || parsed.Host == "" {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// errorWithLinksContent rend un message d'erreur suivi d'un lien cliquable
// par URL détectée (ouverture dans le navigateur).
func errorWithLinksContent(msg string) fyne.CanvasObject {
	label := widget.NewLabel(msg)
	label.Wrapping = fyne.TextWrapWord
	rows := []fyne.CanvasObject{label}
	for _, u := range errorURLs(msg) {
		link := u
		parsed, _ := url.Parse(link)
		hl := widget.NewHyperlink(link, parsed)
		hl.OnTapped = func() { _ = openBrowserCrossPlatform(link) }
		rows = append(rows, hl)
	}
	return container.NewVBox(rows...)
}

// showErrorWithLinks affiche une erreur comme dialog.ShowError, sauf que
// les URL du message deviennent des liens cliquables sous le texte.
func showErrorWithLinks(err error, parent fyne.Window) {
	if err == nil {
		return
	}
	if len(errorURLs(err.Error())) == 0 {
		dialog.ShowError(err, parent)
		return
	}
	scroll := container.NewVScroll(errorWithLinksContent(err.Error()))
	scroll.SetMinSize(fyne.NewSize(520, 160))
	dialog.ShowCustom(T("error"), "OK", scroll, parent)
}

// customerGatePanel affiche le contenu client si la session est ouverte,
// sinon le formulaire de code 2FA quand l'ouverture silencieuse a obtenu
// un challenge, sinon le formulaire de connexion complet.
func customerGatePanel(panelID int, relaunch func(int), builder func(int, func(int)) fyne.CanvasObject) fyne.CanvasObject {
	if !customerLoggedIn() {
		if chg, eml, allowed := getCustomerPending(); chg != "" {
			return customer2FAPanel(chg, eml, allowed, panelID, relaunch)
		}
		return customerLoginPanel(panelID, relaunch, "")
	}
	return builder(panelID, relaunch)
}

// customerLoginPanel affiche la connexion compte client dans le panneau.
func customerLoginPanel(panelID int, relaunch func(int), notice string) fyne.CanvasObject {
	box := container.NewMax()
	form := container.NewVBox()
	title := widget.NewLabelWithStyle(T("customer_login_title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	form.Add(title)
	if notice != "" {
		noticeLabel := widget.NewLabelWithStyle(notice, fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
		form.Add(noticeLabel)
	}
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder(T("email_placeholder"))
	emailEntry.Text = getCustomerSessionEmail()
	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder(T("password_placeholder"))
	errLabel := widget.NewLabel("")
	errLabel.Importance = widget.DangerImportance
	errLabel.Wrapping = fyne.TextWrapWord
	submit := func() {}
	loginBtn := widget.NewButton(T("login_btn"), func() { submit() })
	loginBtn.Importance = widget.HighImportance
	submit = func() {
		email := strings.ToLower(strings.TrimSpace(emailEntry.Text))
		password := strings.TrimSpace(passEntry.Text)
		if email == "" || password == "" {
			errLabel.SetText(T("login_err_empty"))
			return
		}
		loginBtn.Disable()
		errLabel.SetText(T("login_checking"))
		go func() {
			res, err := customerLoginPassword(email, password, getCustomerDeviceToken())
			fyne.Do(func() {
				loginBtn.Enable()
				if err != nil {
					errLabel.SetText(err.Error())
					return
				}
				if res.Requires2FA {
					allowEmail := true
					if res.EmailCodeAllowed != nil {
						allowEmail = *res.EmailCodeAllowed
					}
					box.Objects = []fyne.CanvasObject{customer2FAPanel(res.ChallengeToken, res.Email, allowEmail, panelID, relaunch)}
					box.Refresh()
					return
				}
				storeCustomerSession(res)
				relaunch(panelID)
			})
		}()
	}
	googleBtn := widget.NewButton(T("google_login_btn"), func() {
		errLabel.SetText(T("google_waiting"))
		go func() {
			res, err := performGoogleOAuthFlowCustomer(context.Background())
			fyne.Do(func() {
				if err != nil {
					errLabel.SetText(err.Error())
					return
				}
				if res.Requires2FA {
					allowEmail := true
					if res.EmailCodeAllowed != nil {
						allowEmail = *res.EmailCodeAllowed
					}
					box.Objects = []fyne.CanvasObject{customer2FAPanel(res.ChallengeToken, res.Email, allowEmail, panelID, relaunch)}
					box.Refresh()
					return
				}
				storeCustomerSession(res)
				relaunch(panelID)
			})
		}()
	})
	forgotBtn := widget.NewButton(T("forgot_password_btn"), func() {
		_ = openBrowserCrossPlatform("https://relaisdesk.fr/client/")
	})
	form.Add(widget.NewLabel(T("email_label")))
	form.Add(emailEntry)
	form.Add(widget.NewLabel(T("password_label")))
	form.Add(passEntry)
	form.Add(errLabel)
	form.Add(loginBtn)
	form.Add(widget.NewLabelWithStyle(T("or_separator"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
	form.Add(googleBtn)
	form.Add(forgotBtn)
	card := createCardBox(container.NewPadded(form), customerCardBorder, customerCardFill)
	box.Objects = []fyne.CanvasObject{centeredCardScroll(card)}
	return box
}

// customer2FAPanel vérifie le second facteur du compte client.
func customer2FAPanel(challengeToken, email string, emailCodeAllowed bool, panelID int, relaunch func(int)) fyne.CanvasObject {
	form := container.NewVBox()
	form.Add(widget.NewLabelWithStyle(T("totp_title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	form.Add(widget.NewLabelWithStyle(TF("totp_subtitle_customer", email), fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
	codeEntry := widget.NewEntry()
	codeEntry.SetPlaceHolder(T("totp_code_placeholder"))
	rememberCheck := widget.NewCheck(T("totp_remember_device"), nil)
	errLabel := widget.NewLabel("")
	errLabel.Importance = widget.DangerImportance
	errLabel.Wrapping = fyne.TextWrapWord
	verifyBtn := widget.NewButton(T("totp_verify_btn"), func() {
		code := strings.TrimSpace(codeEntry.Text)
		if code == "" {
			errLabel.SetText(T("totp_err_empty"))
			return
		}
		errLabel.SetText(T("totp_checking"))
		go func() {
			res, err := customerLogin2FA(challengeToken, code, rememberCheck.Checked, "RelaisDesk Technicien")
			fyne.Do(func() {
				if err != nil {
					errLabel.SetText(err.Error())
					return
				}
				if res.DeviceToken != "" {
					setCustomerDeviceToken(res.DeviceToken)
				} else if !rememberCheck.Checked {
					setCustomerDeviceToken("")
				}
				storeCustomerSession(res)
				relaunch(panelID)
			})
		}()
	})
	verifyBtn.Importance = widget.HighImportance
	form.Add(codeEntry)
	form.Add(rememberCheck)
	form.Add(errLabel)
	form.Add(verifyBtn)
	if emailCodeAllowed {
		mailBtn := widget.NewButton(T("totp_send_email_btn"), func() {
			errLabel.SetText(T("totp_email_sending"))
			go func() {
				res, err := sendCustomer2FAEmailCode(challengeToken)
				fyne.Do(func() {
					if err != nil {
						errLabel.SetText(err.Error())
						return
					}
					if res.EmailMasked != "" {
						errLabel.SetText(TF("totp_email_sent_msg", res.EmailMasked))
					} else {
						errLabel.SetText(res.Message)
					}
				})
			}()
		})
		form.Add(mailBtn)
	}
	backBtn := widget.NewButton(T("totp_back_btn"), func() { relaunch(panelID) })
	form.Add(backBtn)
	card := createCardBox(container.NewPadded(form), customerCardBorder, customerCardFill)
	return centeredCardScroll(card)
}

// performGoogleOAuthFlowCustomer réutilise la boucle OAuth du navigateur
// puis ouvre une session compte client (et non technicien).
func performGoogleOAuthFlowCustomer(ctx context.Context) (*customerLoginResult, error) {
	flowCtx, flowCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer flowCancel()

	authURL, resChan, cleanup, err := startGoogleOAuthLoopback(flowCtx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := openBrowserCrossPlatform(authURL); err != nil {
		return nil, fmt.Errorf("impossible d'ouvrir le navigateur : %w", err)
	}

	select {
	case res := <-resChan:
		if res.Error != nil {
			return nil, res.Error
		}
		return customerLoginGoogle(res.Credential)
	case <-flowCtx.Done():
		return nil, fmt.Errorf("connexion Google annulée ou expirée")
	}
}

// overviewPanel reproduit la vue d'ensemble web : compte, chiffres clés,
// licences, abonnements.
func overviewPanel(panelID int, relaunch func(int)) fyne.CanvasObject {
	return asyncCustomerPanel(panelID, relaunch, func(dash *customerDashboard) fyne.CanvasObject {
		accountCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("overview_account_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabel(TF("overview_account_line", dash.CustomerID, dash.Email)),
		), customerCardBorder, customerCardFill)

		metrics := container.NewGridWithColumns(3,
			metricCard(T("overview_metric_licenses"), fmt.Sprint(len(dash.Licenses))),
			metricCard(T("overview_metric_invoices"), fmt.Sprint(len(dash.Invoices))),
			metricCard(T("overview_metric_interventions"), fmt.Sprint(len(dash.Interventions))),
		)

		licenseRows := container.NewVBox()
		if len(dash.Licenses) == 0 {
			licenseRows.Add(widget.NewLabel(T("licenses_empty")))
		}
		for _, lic := range dash.Licenses {
			l := lic
			statusLabel := widget.NewLabelWithStyle(licenseStatusLabel(l.Status), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			if l.Status == "active" {
				statusLabel.Importance = widget.SuccessImportance
			} else if l.Status == "expired" {
				statusLabel.Importance = widget.WarningImportance
			} else {
				statusLabel.Importance = widget.DangerImportance
			}
			licenseRows.Add(container.NewVBox(
				container.NewHBox(widget.NewLabelWithStyle(l.LicenseID, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), statusLabel),
				widget.NewLabel(TF("license_expiry_line", formatCustomerDateRFC3339(l.ExpiresAt), l.MaxConnections)),
				widget.NewSeparator(),
			))
		}
		licensesCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("licenses_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			licenseRows,
		), customerCardBorder, customerCardFill)

		subsBox := container.NewVBox()
		if len(dash.Subscriptions) == 0 {
			subsBox.Add(widget.NewLabel(T("subscriptions_empty")))
		}
		for _, sub := range dash.Subscriptions {
			s := sub
			end := s.TrialEnd
			if s.PaidThrough > end {
				end = s.PaidThrough
			}
			cancelled := s.CancelAtPeriodEnd
			statusText := T("subscription_active")
			if s.WithdrawalImmediate {
				statusText = T("subscription_withdrawn")
			} else if cancelled {
				statusText = T("subscription_cancelled")
			} else if s.CancelRequestedAt > 0 {
				statusText = T("subscription_cancel_pending")
			}
			row := container.NewVBox(
				widget.NewLabelWithStyle(fmt.Sprintf("%s — %d technicien(s)", s.Plan, s.Technicians), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewLabel(TF("subscription_detail_line", formatEUR(float64(s.PriceCents)/100), formatCustomerDateUnix(end))),
				widget.NewLabel(statusText),
			)
			if !cancelled && !s.WithdrawalImmediate {
				cancelBtn := widget.NewButton(T("subscription_cancel_btn"), func() {
					dialog.ShowConfirm(T("subscription_cancel_btn"), T("subscription_cancel_confirm"), func(ok bool) {
						if !ok {
							return
						}
						go func() {
							msg, err := cancelCustomerSubscription(getCustomerSessionToken(), s.ID)
							fyne.Do(func() {
								if err != nil {
									dialog.ShowError(err, mainWindow)
									return
								}
								if msg != "" {
									dialog.ShowInformation(T("subscription_cancel_btn"), msg, mainWindow)
								}
								relaunch(panelID)
							})
						}()
					}, mainWindow)
				})
				row.Add(cancelBtn)
			}
			row.Add(widget.NewSeparator())
			subsBox.Add(row)
		}
		subsCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("subscriptions_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			subsBox,
		), customerCardBorder, customerCardFill)

		return container.NewVBox(accountCard, metrics, licensesCard, subsCard)
	})
}

func open2FAEnableDialog(panelID int, relaunch func(int)) {
	loading := dialog.NewCustomWithoutButtons(T("twofa_enable_btn"),
		container.NewCenter(widget.NewLabel(T("login_checking"))), mainWindow)
	loading.Show()
	go func() {
		setup, err := setupCustomer2FA(getCustomerSessionToken())
		fyne.Do(func() {
			loading.Hide()
			if err != nil {
				if isCustomerSessionExpired(err) {
					clearCustomerSession()
					relaunch(panelID)
					return
				}
				dialog.ShowError(err, mainWindow)
				return
			}
			form := container.NewVBox()
			form.Add(widget.NewLabelWithStyle(T("twofa_scan_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
			if png, err := qrcode.Encode(setup.OTPAuthURL, qrcode.Medium, 220); err == nil {
				qr := canvas.NewImageFromResource(fyne.NewStaticResource("2fa-qr.png", png))
				qr.FillMode = canvas.ImageFillContain
				qr.SetMinSize(fyne.NewSize(220, 220))
				form.Add(container.NewCenter(qr))
			}
			secretEntry := widget.NewEntry()
			secretEntry.SetText(setup.Secret)
			secretEntry.Disable()
			form.Add(widget.NewLabel(T("twofa_secret_label")))
			form.Add(secretEntry)
			recoveryEntry := widget.NewMultiLineEntry()
			recoveryEntry.SetText(strings.Join(setup.RecoveryCodes, "\n"))
			recoveryEntry.Disable()
			form.Add(widget.NewLabel(T("twofa_recovery_label")))
			form.Add(recoveryEntry)
			passwordEntry := widget.NewPasswordEntry()
			passwordEntry.SetPlaceHolder(T("password_placeholder"))
			codeEntry := widget.NewEntry()
			codeEntry.SetPlaceHolder(T("totp_code_placeholder"))
			form.Add(widget.NewLabel(T("password_label")))
			form.Add(passwordEntry)
			form.Add(widget.NewLabel(T("twofa_code_label")))
			form.Add(codeEntry)
			dialog.ShowCustomConfirm(T("twofa_enable_btn"), T("dialog_confirm_btn"), T("dialog_cancel_btn"),
				container.NewVScroll(form), func(ok bool) {
					if !ok {
						return
					}
					password := strings.TrimSpace(passwordEntry.Text)
					code := strings.TrimSpace(codeEntry.Text)
					if password == "" || code == "" {
						return
					}
					go func() {
						msg, err := enableCustomer2FA(getCustomerSessionToken(), password, setup.Secret, code, setup.RecoveryCodes)
						fyne.Do(func() {
							if err != nil {
								if isCustomerSessionExpired(err) {
									clearCustomerSession()
									relaunch(panelID)
									return
								}
								dialog.ShowError(err, mainWindow)
								return
							}
							clearCustomerSession()
							dialog.ShowInformation(T("twofa_enable_btn"), msg, mainWindow)
							relaunch(panelID)
						})
					}()
				}, mainWindow)
		})
	}()
}

func open2FADisableDialog(panelID int, relaunch func(int)) {
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder(T("password_placeholder"))
	codeEntry := widget.NewEntry()
	codeEntry.SetPlaceHolder(T("totp_code_placeholder"))
	content := container.NewVBox(
		widget.NewLabel(T("twofa_disable_sub")),
		widget.NewLabel(T("password_label")),
		passwordEntry,
		widget.NewLabel(T("twofa_code_label")),
		codeEntry,
	)
	dialog.ShowCustomConfirm(T("twofa_disable_btn"), T("dialog_confirm_btn"), T("dialog_cancel_btn"), content, func(ok bool) {
		if !ok {
			return
		}
		password := strings.TrimSpace(passwordEntry.Text)
		if password == "" {
			return
		}
		go func() {
			msg, err := disableCustomer2FA(getCustomerSessionToken(), password, strings.TrimSpace(codeEntry.Text))
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					dialog.ShowError(err, mainWindow)
					return
				}
				clearCustomerSession()
				dialog.ShowInformation(T("twofa_disable_btn"), msg, mainWindow)
				relaunch(panelID)
			})
		}()
	}, mainWindow)
}

func metricCard(label, value string) fyne.CanvasObject {
	return createCardBox(container.NewVBox(
		widget.NewLabelWithStyle(value, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle(label, fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
	), customerCardBorder, customerCardFill)
}

func licenseStatusLabel(status string) string {
	switch status {
	case "active":
		return T("status_active")
	case "expired":
		return T("status_expired")
	case "revoked":
		return T("status_revoked")
	default:
		return status
	}
}

// settingsPanel reproduit les préférences web : rappels, mot de passe,
// état 2FA, déconnexion du compte client.
func settingsPanel(panelID int, relaunch func(int)) fyne.CanvasObject {
	return asyncCustomerPanel(panelID, relaunch, func(dash *customerDashboard) fyne.CanvasObject {
		accountCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("settings_account_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabel(TF("overview_account_line", dash.CustomerID, dash.Email)),
		), customerCardBorder, customerCardFill)

		remindersCheck := widget.NewCheck(T("settings_reminders_label"), nil)
		remindersCheck.Checked = dash.RenewalRemindersEnabled
		remindersCheck.OnChanged = func(on bool) {
			go func() {
				err := setRenewalReminders(getCustomerSessionToken(), on)
				fyne.Do(func() {
					if err != nil {
						remindersCheck.SetChecked(!on)
						dialog.ShowError(err, mainWindow)
					}
				})
			}()
		}
		prefsCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("settings_prefs_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			remindersCheck,
		), customerCardBorder, customerCardFill)

		oldEntry := widget.NewPasswordEntry()
		oldEntry.SetPlaceHolder(T("password_old_placeholder"))
		newEntry := widget.NewPasswordEntry()
		newEntry.SetPlaceHolder(T("password_new_placeholder"))
		confirmEntry := widget.NewPasswordEntry()
		confirmEntry.SetPlaceHolder(T("password_confirm_placeholder"))
		pwdMsg := widget.NewLabel("")
		pwdMsg.Wrapping = fyne.TextWrapWord
		pwdBtn := widget.NewButton(T("password_change_btn"), func() {
			old := strings.TrimSpace(oldEntry.Text)
			newPass := strings.TrimSpace(newEntry.Text)
			if old == "" || newPass == "" {
				pwdMsg.SetText(T("login_err_empty"))
				return
			}
			if newPass != strings.TrimSpace(confirmEntry.Text) {
				pwdMsg.SetText(T("password_mismatch"))
				return
			}
			pwdMsg.SetText(T("login_checking"))
			go func() {
				err := changeCustomerPassword(getCustomerSessionToken(), old, newPass)
				fyne.Do(func() {
					if err != nil {
						pwdMsg.SetText(err.Error())
						return
					}
					oldEntry.SetText("")
					newEntry.SetText("")
					confirmEntry.SetText("")
					pwdMsg.SetText(T("password_changed_ok"))
				})
			}()
		})
		pwdBtn.Importance = widget.HighImportance
		pwdCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("settings_password_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			oldEntry, newEntry, confirmEntry, pwdMsg, pwdBtn,
		), customerCardBorder, customerCardFill)

		twofaBox := container.NewVBox(widget.NewLabel(T("login_checking")))
		go func() {
			status, err := getCustomer2FAStatus(getCustomerSessionToken())
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					twofaBox.Objects = []fyne.CanvasObject{widget.NewLabel(err.Error())}
					twofaBox.Refresh()
					return
				}
				stateLabel := widget.NewLabelWithStyle(T("twofa_disabled"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
				objects := []fyne.CanvasObject{stateLabel}
				if status.Enabled {
					stateLabel = widget.NewLabelWithStyle(TF("twofa_enabled", status.RemainingRecoveryCodes), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
					stateLabel.Importance = widget.SuccessImportance
					disableBtn := widget.NewButton(T("twofa_disable_btn"), func() {
						open2FADisableDialog(panelID, relaunch)
					})
					objects = []fyne.CanvasObject{stateLabel, disableBtn}
				} else {
					enableBtn := widget.NewButton(T("twofa_enable_btn"), func() {
						open2FAEnableDialog(panelID, relaunch)
					})
					enableBtn.Importance = widget.HighImportance
					objects = append(objects, enableBtn)
				}
				twofaBox.Objects = objects
				twofaBox.Refresh()
			})
		}()
		twofaCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("settings_2fa_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			twofaBox,
		), customerCardBorder, customerCardFill)

		logoutCustomerBtn := widget.NewButton(T("logout_customer_btn"), func() {
			token := getCustomerSessionToken()
			if token != "" {
				go func() { _ = customerLogout(token) }()
			}
			clearCustomerSession()
			relaunch(panelID)
		})
		sessionCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("settings_session_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			logoutCustomerBtn,
		), customerCardBorder, customerCardFill)

		return container.NewVBox(accountCard, prefsCard, pwdCard, twofaCard, sessionCard)
	})
}
