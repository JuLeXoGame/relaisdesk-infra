package main

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

//go:embed icon.png
var appIconBytes []byte

const (
	PRODUCT_NAME = "RelaisDesk"
)

var (
	fyneApp           fyne.App
	mainWindow        fyne.Window
	sessionToken      string
	sessionAdminEmail string
	unmaskKeys        bool = false
)

func RunAdminGUI() error {
	fyneApp = app.NewWithID("com.relaisdesk.admindashboard")
	if len(appIconBytes) > 0 {
		appIcon := fyne.NewStaticResource("icon.png", appIconBytes)
		fyneApp.SetIcon(appIcon)
	}

	mainWindow = fyneApp.NewWindow(PRODUCT_NAME + " — Dashboard Administrateur")
	mainWindow.Resize(fyne.NewSize(950, 680))
	mainWindow.CenterOnScreen()

	if len(appIconBytes) > 0 {
		appIcon := fyne.NewStaticResource("icon.png", appIconBytes)
		mainWindow.SetIcon(appIcon)
	}

	// Try auto-login with saved session
	saved, err := LoadAdminSession()
	if err == nil && saved != nil && saved.Token != "" {
		sessionToken = saved.Token
		sessionAdminEmail = saved.Email
		if sessionAdminEmail == "" {
			sessionAdminEmail = "Master Admin"
		}
		showDashboardScreen()
	} else {
		showLoginScreen("")
	}

	mainWindow.ShowAndRun()
	return nil
}

// =============================================================================
// 1. ÉCRAN DE CONNEXION ADMIN
// =============================================================================
func showLoginScreen(errorMsg string) {
	title := widget.NewLabelWithStyle(PRODUCT_NAME+" — Espace Administrateur", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	subTitle := widget.NewLabelWithStyle("Connectez-vous pour superviser les licences et valider les commandes", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	statusLabel := widget.NewLabel(errorMsg)
	if errorMsg != "" {
		statusLabel.Importance = widget.DangerImportance
	}

	// 1. Onglet Connexion E-mail & Mot de passe
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder("admin@monentreprise.fr")

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Mot de passe du compte administrateur")

	var emailLoginBtn *widget.Button
	emailLoginBtn = widget.NewButton("🔐 Connexion avec E-mail", func() {
		email := strings.TrimSpace(emailEntry.Text)
		password := strings.TrimSpace(passwordEntry.Text)

		if email == "" || password == "" {
			statusLabel.SetText("Veuillez renseigner votre e-mail et votre mot de passe.")
			statusLabel.Importance = widget.DangerImportance
			return
		}

		emailLoginBtn.Disable()
		statusLabel.SetText("Vérification auprès du serveur API...")
		statusLabel.Importance = widget.MediumImportance

		go func() {
			resp, err := AdminLoginWithEmail(email, password)
			if err != nil {
				fyne.Do(func() {
					statusLabel.SetText("Erreur : " + err.Error())
					statusLabel.Importance = widget.DangerImportance
					emailLoginBtn.Enable()
				})
				return
			}

			// Si 2FA requis, afficher le dialogue de validation TOTP
			if resp.Requires2FA {
				fyne.Do(func() {
					statusLabel.SetText("Code TOTP 2FA requis.")
					statusLabel.Importance = widget.WarningImportance

					totpEntry := widget.NewEntry()
					totpEntry.SetPlaceHolder("123456")

					dialog.ShowCustomConfirm("🔒 Double facteur requis (2FA)", "Valider", "Annuler",
						container.NewVBox(
							widget.NewLabel("Saisissez le code à 6 chiffres de votre application :"),
							totpEntry,
						),
						func(confirmed bool) {
							if !confirmed {
								emailLoginBtn.Enable()
								statusLabel.SetText("Connexion annulée.")
								return
							}

							code := strings.TrimSpace(totpEntry.Text)
							if len(code) != 6 {
								statusLabel.SetText("Code à 6 chiffres requis.")
								statusLabel.Importance = widget.DangerImportance
								emailLoginBtn.Enable()
								return
							}

							statusLabel.SetText("Vérification du code 2FA...")
							go func() {
								verifyResp, verifyErr := AdminVerify2FA(resp.ChallengeToken, code)
								if verifyErr != nil {
									fyne.Do(func() {
										statusLabel.SetText("Erreur 2FA : " + verifyErr.Error())
										statusLabel.Importance = widget.DangerImportance
										emailLoginBtn.Enable()
									})
									return
								}

								adminEmail := verifyResp.Email
								if adminEmail == "" {
									adminEmail = email
								}

								_ = SaveAdminSession(AdminStoredSession{
									Token:     verifyResp.Token,
									LicenseID: verifyResp.LicenseID,
									Email:     adminEmail,
								})

								fyne.Do(func() {
									sessionToken = verifyResp.Token
									sessionAdminEmail = adminEmail
									showDashboardScreen()
								})
							}()
						},
						mainWindow,
					)
				})
				return
			}

			// Connexion directe réussie sans 2FA
			adminEmail := resp.Email
			if adminEmail == "" {
				adminEmail = email
			}

			_ = SaveAdminSession(AdminStoredSession{
				Token:     resp.Token,
				LicenseID: resp.LicenseID,
				Email:     adminEmail,
			})

			fyne.Do(func() {
				sessionToken = resp.Token
				sessionAdminEmail = adminEmail
				showDashboardScreen()
			})
		}()
	})
	emailLoginBtn.Importance = widget.HighImportance

	emailForm := container.NewVBox(
		widget.NewLabel("Adresse e-mail administrateur :"),
		emailEntry,
		widget.NewLabel("Mot de passe :"),
		passwordEntry,
		widget.NewSeparator(),
		emailLoginBtn,
	)

	// 2. Onglet Connexion Licence / Clé
	licIDEntry := widget.NewEntry()
	licIDEntry.SetPlaceHolder("Identifiant Licence Admin (MP-XXXX-XXXX-XXXX)")

	licKeyEntry := widget.NewPasswordEntry()
	licKeyEntry.SetPlaceHolder("Clé secrète de licence Admin (ex: mpsk_...)")

	secretTokenEntry := widget.NewPasswordEntry()
	secretTokenEntry.SetPlaceHolder("Ou Token secret serveur (ADMIN_TOKEN optionnel)")

	var licenseLoginBtn *widget.Button
	licenseLoginBtn = widget.NewButton("🔑 Connexion avec Licence", func() {
		licID := strings.TrimSpace(licIDEntry.Text)
		licKey := strings.TrimSpace(licKeyEntry.Text)
		secretToken := strings.TrimSpace(secretTokenEntry.Text)

		if licID == "" && licKey == "" && secretToken == "" {
			statusLabel.SetText("Veuillez renseigner vos identifiants administrateur.")
			statusLabel.Importance = widget.DangerImportance
			return
		}

		licenseLoginBtn.Disable()
		statusLabel.SetText("Vérification auprès du serveur API...")
		statusLabel.Importance = widget.MediumImportance

		go func() {
			resp, err := AdminLogin(licID, licKey, secretToken)
			if err != nil {
				fyne.Do(func() {
					statusLabel.SetText("Erreur : " + err.Error())
					statusLabel.Importance = widget.DangerImportance
					licenseLoginBtn.Enable()
				})
				return
			}

			adminEmail := resp.Email
			if adminEmail == "" {
				adminEmail = "Master Admin"
			}

			_ = SaveAdminSession(AdminStoredSession{
				Token:     resp.Token,
				LicenseID: licID,
				Email:     adminEmail,
			})

			fyne.Do(func() {
				sessionToken = resp.Token
				sessionAdminEmail = adminEmail
				showDashboardScreen()
			})
		}()
	})
	licenseLoginBtn.Importance = widget.HighImportance

	licenseForm := container.NewVBox(
		widget.NewLabel("Identifiant Licence Administrateur :"),
		licIDEntry,
		widget.NewLabel("Clé de Licence Administrateur :"),
		licKeyEntry,
		widget.NewSeparator(),
		widget.NewLabel("Token Secret Serveur (optionnel) :"),
		secretTokenEntry,
		widget.NewSeparator(),
		licenseLoginBtn,
	)

	authTabs := container.NewAppTabs(
		container.NewTabItem("✉️ E-mail & Mot de passe", emailForm),
		container.NewTabItem("🔑 Licence / Clé", licenseForm),
	)

	cardContent := container.NewVBox(
		authTabs,
		statusLabel,
	)

	card := widget.NewCard("", "", cardContent)
	content := container.NewCenter(container.NewVBox(
		title,
		subTitle,
		container.NewPadded(card),
	))

	mainWindow.SetContent(content)
}

// =============================================================================
// 2. DASHBOARD PRINCIPAL
// =============================================================================
func showDashboardScreen() {
	header := container.NewHBox(
		widget.NewLabelWithStyle(PRODUCT_NAME+" Admin", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("• Connecté: "+sessionAdminEmail),
		layout.NewSpacer(),
		widget.NewButton("🔄 Actualiser", func() {
			showDashboardScreen()
		}),
		widget.NewButton("🚪 Déconnexion", func() {
			token := sessionToken
			if token != "" {
				go func() { _ = AdminLogout(token) }()
			}
			_ = ClearAdminSession()
			sessionToken = ""
			showLoginScreen("Déconnexion réussie.")
		}),
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("📊 Vue d'ensemble", buildOverviewTab()),
		container.NewTabItem("🔑 Licences Techniciens", buildLicensesTab()),
		container.NewTabItem("🖥️ Parc & Dossiers", buildFleetTab()),
		container.NewTabItem("➕ Créer une Licence", buildCreateLicenseTab()),
		container.NewTabItem("🛒 Commandes & Virements", buildOrdersTab()),
		container.NewTabItem("🧾 Factures & Avoirs", buildInvoicesTab()),
		container.NewTabItem("🚨 Alertes Sécurité", buildAlertsTab()),
	)

	mainWindow.SetContent(container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		nil, nil, nil,
		tabs,
	))
}

// =============================================================================
// TAB 1 : VUE D'ENSEMBLE & ANALYSE FINANCIÈRE
// =============================================================================
func buildOverviewTab() fyne.CanvasObject {
	statsLabel := widget.NewLabel("Chargement des statistiques...")
	systemLabel := widget.NewLabel("Contrôle des services RelaisDesk...")
	finLabel := widget.NewLabel("Calcul des données financières...")

	fiscalRadio := widget.NewRadioGroup([]string{
		"🇫🇷 Micro-Entreprise (0% TVA - Franchise Art. 293 B)",
		"🏢 Entreprise Assujettie à la TVA (Régime Réel 20%)",
	}, nil)
	fiscalRadio.SetSelected("🇫🇷 Micro-Entreprise (0% TVA - Franchise Art. 293 B)")

	var lastFin *AdminFinancials

	updateFinancialDisplay := func() {
		if lastFin == nil {
			return
		}
		caBrut := lastFin.TotalRevenue
		var tvaAmount float64
		var caHT float64
		var charges float64
		var netProfit float64
		var regimeInfo string

		urssafRate := 0.212 // 21.2%
		feesRate := 0.015   // 1.5%

		if fiscalRadio.Selected == "🇫🇷 Micro-Entreprise (0% TVA - Franchise Art. 293 B)" {
			caHT = caBrut
			tvaAmount = 0.0
			charges = caBrut * (urssafRate + feesRate)
			netProfit = caBrut - charges
			regimeInfo = "Régime Micro-Entreprise (Franchise en base de TVA - Art. 293 B du CGI) :\nTVA non facturée (0%). Vos cotisations URSSAF sont calculées à 21,2% sur votre CA brut encaissé."
		} else {
			caHT = caBrut / 1.20
			tvaAmount = caBrut - caHT
			charges = caHT * (urssafRate + feesRate)
			netProfit = caHT - charges
			regimeInfo = "Régime Réel (Entreprise assujettie à la TVA 20%) :\nLa TVA de 20% est déduite pour obtenir le CA Hors Taxes (HT). Vos bénéfices sont calculés sur la base HT."
		}

		txt := fmt.Sprintf(
			"💰 ANALYSE FINANCIÈRE & BÉNÉFICES\n\n"+
				"• Chiffre d'Affaires Encaissé : %.2f €\n"+
				"• TVA Collectée              : %.2f €\n"+
				"• CA Hors Taxes (HT)          : %.2f €\n"+
				"• Charges estimées (URSSAF)   : %.2f € (21.2%% + 1.5%% frais)\n"+
				"--------------------------------------------------\n"+
				"✨ BÉNÉFICE NET ESTIMÉ        : %.2f €\n"+
				"⚡ MRR Récurrent Mensuel      : %.2f € / mois (%d abonnés)\n\n"+
				"ℹ️ %s",
			caBrut, tvaAmount, caHT, charges, netProfit, lastFin.MonthlyRecurringRevenue, lastFin.ActiveSubscribersCount, regimeInfo,
		)
		finLabel.SetText(txt)
	}

	fiscalRadio.OnChanged = func(s string) {
		updateFinancialDisplay()
	}
	token := sessionToken

	go func() {
		stats, err := GetStats(token)
		if err == nil && stats != nil {
			connectionStatus := "Mesure temps réel non exposée (quota appliqué par hbbs)"
			if stats.ConnectionCountAvailable {
				connectionStatus = fmt.Sprintf("%d tech(s)", stats.CurrentConnections)
			}
			txt := fmt.Sprintf(
				"📊 INFRASTRUCTURE & LICENCES\n\n"+
					"• Licences Totales      : %d\n"+
					"• Licences Actives      : %d\n"+
					"• Licences Expirées     : %d\n"+
					"• Licences Révoquées    : %d\n"+
					"• Connexions en direct  : %s\n"+
					"• Serveur API           : %s",
				stats.TotalLicences,
				stats.ActiveLicences,
				stats.ExpiredLicences,
				stats.RevokedLicences,
				connectionStatus,
				APIURL,
			)
			fyne.Do(func() { statsLabel.SetText(txt) })
		}

		systemStatus, err := GetSystemStatus(token)
		if err != nil {
			fyne.Do(func() { systemLabel.SetText("⚠️ Supervision indisponible : " + err.Error()) })
		} else if systemStatus != nil {
			var lines []string
			for _, component := range systemStatus.Components {
				icon := "✅"
				if component.Status != "healthy" {
					icon = "❌"
				}
				lines = append(lines, fmt.Sprintf("%s %-15s %s (%d ms)", icon, component.Name, component.Message, component.LatencyMS))
			}
			heading := "SERVICES OPÉRATIONNELS"
			if systemStatus.Status != "healthy" {
				heading = "SERVICE DÉGRADÉ — INTERVENTION REQUISE"
			}
			fyne.Do(func() { systemLabel.SetText(heading + "\n\n" + strings.Join(lines, "\n")) })
		}

		fin, err := GetFinancials(token)
		if err == nil && fin != nil {
			fyne.Do(func() {
				lastFin = fin
				updateFinancialDisplay()
			})
		}
	}()

	return container.NewScroll(container.NewPadded(container.NewVBox(
		widget.NewLabelWithStyle("Supervision & Performance Commerciale", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		statsLabel,
		widget.NewSeparator(),
		systemLabel,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Régime Fiscal & Simulation :", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		fiscalRadio,
		widget.NewSeparator(),
		finLabel,
	)))
}

// =============================================================================
// TAB 2 : LICENCES TECHNICIENS
// =============================================================================
func buildLicensesTab() fyne.CanvasObject {
	listContainer := container.NewVBox()
	listScroll := container.NewVScroll(listContainer)
	listScroll.SetMinSize(fyne.NewSize(900, 480))

	statusFilter := widget.NewSelect([]string{"Tous les statuts", "Actives uniquement", "Expirées uniquement", "Révoquées uniquement"}, nil)
	statusFilter.SetSelected("Tous les statuts")

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("🔍 Filtrer par email ou ID de licence...")

	unmaskCheck := widget.NewCheck("Afficher clés en clair", func(checked bool) {
		unmaskKeys = checked
		reloadLicenses(listContainer, searchEntry.Text, statusFilter.Selected)
	})
	unmaskCheck.SetChecked(unmaskKeys)

	statusFilter.OnChanged = func(s string) {
		reloadLicenses(listContainer, searchEntry.Text, s)
	}
	searchEntry.OnChanged = func(s string) {
		reloadLicenses(listContainer, s, statusFilter.Selected)
	}

	reloadLicenses(listContainer, "", "Tous les statuts")

	topControls := container.NewHBox(
		widget.NewLabel("Filtre:"),
		statusFilter,
		searchEntry,
		unmaskCheck,
	)

	return container.NewBorder(
		topControls,
		nil, nil, nil,
		listScroll,
	)
}

func reloadLicenses(targetBox *fyne.Container, search, statusChoice string) {
	targetBox.Objects = []fyne.CanvasObject{widget.NewLabel("Chargement des licences...")}
	targetBox.Refresh()
	showUnmaskedKeys := unmaskKeys
	token := sessionToken

	go func() {
		statusParam := ""
		if statusChoice == "Actives uniquement" {
			statusParam = "active"
		} else if statusChoice == "Expirées uniquement" {
			statusParam = "expired"
		} else if statusChoice == "Révoquées uniquement" {
			statusParam = "revoked"
		}

		lics, err := ListLicenses(token, showUnmaskedKeys, statusParam, "")
		if err != nil {
			fyne.Do(func() {
				targetBox.Objects = []fyne.CanvasObject{widget.NewLabel("Erreur : " + err.Error())}
				targetBox.Refresh()
			})
			return
		}

		fyne.Do(func() {
			search = strings.ToLower(strings.TrimSpace(search))
			var filtered []AdminLicense
			for _, l := range lics {
				if search == "" || strings.Contains(strings.ToLower(l.LicenseID), search) || strings.Contains(strings.ToLower(l.Email), search) || strings.Contains(strings.ToLower(l.Notes), search) {
					filtered = append(filtered, l)
				}
			}

			if len(filtered) == 0 {
				targetBox.Objects = []fyne.CanvasObject{widget.NewLabel("Aucune licence trouvée.")}
				targetBox.Refresh()
				return
			}

			var rows []fyne.CanvasObject
			for _, l := range filtered {
				lic := l
				statusTxt := "Actif"
				if lic.Status == "expired" {
					statusTxt = "Expiré"
				} else if lic.Status == "revoked" {
					statusTxt = "Révoqué"
				}

				infoStr := fmt.Sprintf(
					"ID: %s | Client: %s\nClé: %s\nQuota: %d connexion(s) technicien simultanée(s) | Expire: %s | Statut: %s\nNotes: %s",
					lic.LicenseID, lic.Email, lic.LicenseKey,
					lic.MaxConnections,
					lic.ExpiresAt, statusTxt, lic.Notes,
				)
				infoLabel := widget.NewLabel(infoStr)

				copyBtn := widget.NewButton("📋 Copier", func() {
					text := fmt.Sprintf("Identifiant: %s\nClé: %s", lic.LicenseID, lic.LicenseKey)
					mainWindow.Clipboard().SetContent(text)
					dialog.ShowInformation("Copié", "Accès de la licence copiés dans le presse-papiers !", mainWindow)
				})

				extendBtn := widget.NewButton("🔄 Prolonger (+30j)", func() {
					dialog.ShowConfirm("Prolonger la licence", fmt.Sprintf("Voulez-vous prolonger la licence %s de 30 jours ?", lic.LicenseID), func(confirm bool) {
						if confirm {
							actionToken := sessionToken
							go func() {
								err := ExtendLicense(actionToken, lic.LicenseID, 30)
								fyne.Do(func() {
									if err != nil {
										dialog.ShowError(err, mainWindow)
									} else {
										dialog.ShowInformation("Succès", "Licence prolongée de 30 jours !", mainWindow)
										reloadLicenses(targetBox, search, statusChoice)
									}
								})
							}()
						}
					}, mainWindow)
				})

				var revokeBtn *widget.Button
				revokeBtn = widget.NewButton("🛑 Révoquer", func() {
					dialog.ShowConfirm("Révoquer la licence", fmt.Sprintf("Confirmez-vous la révocation de la licence %s ?", lic.LicenseID), func(confirm bool) {
						if confirm {
							actionToken := sessionToken
							go func() {
								err := RevokeLicense(actionToken, lic.LicenseID, "Révocation admin")
								fyne.Do(func() {
									if err != nil {
										dialog.ShowError(err, mainWindow)
									} else {
										dialog.ShowInformation("Succès", "Licence révoquée !", mainWindow)
										reloadLicenses(targetBox, search, statusChoice)
									}
								})
							}()
						}
					}, mainWindow)
				})

				if lic.Status == "revoked" {
					revokeBtn.Disable()
				}

				actions := container.NewVBox(copyBtn, extendBtn, revokeBtn)
				row := container.NewHBox(infoLabel, layout.NewSpacer(), actions)
				rows = append(rows, row, widget.NewSeparator())
			}

			targetBox.Objects = rows
			targetBox.Refresh()
		})
	}()
}

// =============================================================================
// TAB 3 : CRÉER UNE LICENCE (PLANS COMMERCIAUX)
// =============================================================================
func buildCreateLicenseTab() fyne.CanvasObject {
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder("Email du client / technicien (ex: pro@entreprise.fr)")

	notesEntry := widget.NewEntry()
	notesEntry.SetPlaceHolder("Notes internes optionnelles (ex: Client ABC)")

	priceLabel := widget.NewLabelWithStyle("Prix mensuel : 24.90 € (ou 239.00 €/an)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	techSlider := widget.NewSlider(10, 500)
	techSlider.Step = 10
	techSlider.Value = 10
	techSliderLabel := widget.NewLabel("Nombre de techniciens : 10 techniciens")
	techSliderBox := container.NewVBox(techSliderLabel, techSlider)
	techSliderBox.Hide()

	customConnEntry := widget.NewEntry()
	customConnEntry.SetText("5")
	customDaysEntry := widget.NewEntry()
	customDaysEntry.SetText("30")
	customBox := container.NewVBox(
		widget.NewLabel("Connexions simultanées :"),
		customConnEntry,
		widget.NewLabel("Durée en jours :"),
		customDaysEntry,
	)
	customBox.Hide()

	planRadio := widget.NewRadioGroup([]string{
		"Starter (24,90 €/mois — 1 technicien)",
		"Pro (110,00 €/mois — 1 à 5 techniciens)",
		"Personnalisé / Ultra (Dès 199 €/mois — 10 à 500 techniciens)",
		"Sur-Mesure (Durée et connexions libres)",
	}, nil)
	planRadio.SetSelected("Starter (24,90 €/mois — 1 technicien)")

	updatePrice := func() {
		switch planRadio.Selected {
		case "Starter (24,90 €/mois — 1 technicien)":
			priceLabel.SetText("Prix mensuel : 24.90 € (ou 239.00 €/an)")
			techSliderBox.Hide()
			customBox.Hide()
		case "Pro (110,00 €/mois — 1 à 5 techniciens)":
			priceLabel.SetText("Prix mensuel : 110.00 € (ou 1 100.00 €/an)")
			techSliderBox.Hide()
			customBox.Hide()
		case "Personnalisé / Ultra (Dès 199 €/mois — 10 à 500 techniciens)":
			techs := int(techSlider.Value)
			var price float64
			if techs <= 10 {
				price = 199.00
			} else if techs <= 50 {
				price = 199.00 + float64(techs-10)*14.00
			} else if techs <= 100 {
				price = 759.00 + float64(techs-50)*10.00
			} else {
				extra := techs - 100
				if extra > 400 {
					extra = 400
				}
				price = 1259.00 + float64(extra)*7.00
			}
			priceLabel.SetText(fmt.Sprintf("Prix mensuel : %.2f € (Annuel : %.2f €)", price, price*10))
			techSliderLabel.SetText(fmt.Sprintf("Nombre de techniciens : %d techniciens", techs))
			techSliderBox.Show()
			customBox.Hide()
		case "Sur-Mesure (Durée et connexions libres)":
			priceLabel.SetText("Tarif sur-mesure / personnalisé")
			techSliderBox.Hide()
			customBox.Show()
		}
	}

	planRadio.OnChanged = func(s string) { updatePrice() }
	techSlider.OnChanged = func(v float64) { updatePrice() }

	var createBtn *widget.Button
	createBtn = widget.NewButton("✨ Générer la Licence Immédiatement", func() {
		email := strings.TrimSpace(emailEntry.Text)
		if email == "" {
			dialog.ShowError(fmt.Errorf("l'adresse e-mail est obligatoire"), mainWindow)
			return
		}

		notes := strings.TrimSpace(notesEntry.Text)
		selectedPlan := planRadio.Selected
		selectedTechnicians := int(techSlider.Value)
		customConnections := customConnEntry.Text
		customDays := customDaysEntry.Text
		token := sessionToken
		createBtn.Disable()
		go func() {
			var req CreateLicenseReq
			req.Email = email
			req.Notes = notes
			req.Days = 30

			switch selectedPlan {
			case "Starter (24,90 €/mois — 1 technicien)":
				req.Plan = "Starter"
				req.MaxConnections = 1
			case "Pro (110,00 €/mois — 1 à 5 techniciens)":
				req.Plan = "Pro"
				req.MaxConnections = 5
			case "Personnalisé / Ultra (Dès 199 €/mois — 10 à 500 techniciens)":
				req.Plan = "Ultra"
				req.Technicians = selectedTechnicians
				req.MaxConnections = req.Technicians
			case "Sur-Mesure (Durée et connexions libres)":
				c, _ := strconv.Atoi(customConnections)
				d, _ := strconv.Atoi(customDays)
				if c <= 0 {
					c = 1
				}
				if d <= 0 {
					d = 30
				}
				req.MaxConnections = c
				req.Days = d
			}

			lic, err := CreateLicense(token, req)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, mainWindow)
					createBtn.Enable()
				})
				return
			}

			welcomeMsg := fmt.Sprintf(
				"Bonjour,\n\n"+
					"Voici vos accès pour votre licence RelaisDesk :\n"+
					"--------------------------------------------------\n"+
					"Offre               : %s\n"+
					"Identifiant Licence : %s\n"+
					"Clé de Licence      : %s\n"+
					"Capacité            : %d technicien(s)\n"+
					"Date d'expiration   : %s\n"+
					"--------------------------------------------------\n"+
					"Télécharger le Configurateur Technicien :\n"+
					"https://relaisdesk.fr/downloads/configurator.exe\n\n"+
					"L'équipe RelaisDesk\n"+
					"https://relaisdesk.fr",
				lic.Notes, lic.LicenseID, lic.LicenseKey, lic.MaxConnections, lic.ExpiresAt,
			)

			fyne.Do(func() {
				dialog.ShowCustomConfirm("🎉 Licence Créée avec Succès !", "Copier le Message Client", "Fermer",
					widget.NewLabel(fmt.Sprintf("ID : %s\nClé : %s\nClient : %s\nExpire le : %s", lic.LicenseID, lic.LicenseKey, lic.Email, lic.ExpiresAt)),
					func(copyRequested bool) {
						if copyRequested {
							mainWindow.Clipboard().SetContent(welcomeMsg)
							dialog.ShowInformation("Succès", "Message d'accès copié dans le presse-papiers !", mainWindow)
						}
						emailEntry.SetText("")
						notesEntry.SetText("")
						createBtn.Enable()
					}, mainWindow,
				)
			})
		}()
	})
	createBtn.Importance = widget.HighImportance

	form := container.NewVBox(
		widget.NewLabelWithStyle("Création de Licence Commerciale", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Choisissez une formule :"),
		planRadio,
		techSliderBox,
		customBox,
		priceLabel,
		widget.NewSeparator(),
		widget.NewLabel("Email du Client / Technicien * :"),
		emailEntry,
		widget.NewLabel("Notes internes :"),
		notesEntry,
		widget.NewSeparator(),
		createBtn,
	)

	return container.NewScroll(container.NewPadded(form))
}

// =============================================================================
// TAB 4 : COMMANDES & VIREMENTS
// =============================================================================
func buildOrdersTab() fyne.CanvasObject {
	ordersContainer := container.NewVBox()
	scroll := container.NewVScroll(ordersContainer)
	scroll.SetMinSize(fyne.NewSize(900, 480))

	var reloadOrders func()
	reloadOrders = func() {
		ordersContainer.Objects = []fyne.CanvasObject{widget.NewLabel("Chargement des commandes...")}
		ordersContainer.Refresh()
		token := sessionToken

		go func() {
			orders, err := ListOrders(token)
			if err != nil {
				fyne.Do(func() {
					ordersContainer.Objects = []fyne.CanvasObject{widget.NewLabel("Erreur : " + err.Error())}
					ordersContainer.Refresh()
				})
				return
			}

			fyne.Do(func() {
				if len(orders) == 0 {
					ordersContainer.Objects = []fyne.CanvasObject{widget.NewLabel("Aucune commande trouvée.")}
					ordersContainer.Refresh()
					return
				}

				var rows []fyne.CanvasObject
				for _, ord := range orders {
					o := ord
					statusTxt := "Payée"
					if o.Status == "pending" {
						statusTxt = "En attente"
					} else if o.Status == "cancelled" {
						statusTxt = "Annulée/Expirée"
					}

					payTxt := "Carte Bancaire (Stripe)"
					isBankTransfer := o.PaymentMethod == "virement" || o.PaymentMethod == "bank_transfer"
					if isBankTransfer {
						payTxt = "Virement Bancaire"
					}

					info := widget.NewLabel(fmt.Sprintf(
						"Commande: %s | Client: %s\nOffre: %s (%d connexions simultanées) | Montant: %.2f € | Paiement: %s\nStatut: %s | Date: %s\nTraitement: %s Paiement  %s Licence  %s Facture  %s Email licence  %s Email facture",
						o.OrderID, o.Email, o.Plan, o.Technicians, o.Price, payTxt, statusTxt, o.CreatedAt,
						stageIcon(o.PaymentComplete), stageIcon(o.LicenseCreated), stageIcon(o.InvoiceCreated),
						stageIcon(o.LicenseEmailSent), stageIcon(o.InvoiceEmailSent),
					))

					var actionBtn fyne.CanvasObject
					if o.Status == "pending" && isBankTransfer {
						btn := widget.NewButton("✅ Valider Virement", func() {
							dialog.ShowConfirm("Valider le virement", fmt.Sprintf("Confirmez-vous le paiement de la commande %s ?\nCela va créer immédiatement la licence et expédier l'email au client.", o.OrderID), func(confirm bool) {
								if confirm {
									actionToken := sessionToken
									go func() {
										err := MarkOrderPaid(actionToken, o.OrderID)
										fyne.Do(func() {
											if err != nil {
												dialog.ShowError(err, mainWindow)
											} else {
												dialog.ShowInformation("Succès", "Virement validé et licence expédiée !", mainWindow)
												reloadOrders()
											}
										})
									}()
								}
							}, mainWindow)
						})
						btn.Importance = widget.SuccessImportance
						actionBtn = btn
					} else if o.Status == "paid" && !o.FulfillmentDone {
						btn := widget.NewButton("🔄 Relancer le traitement", func() {
							dialog.ShowConfirm("Relancer la commande", "Seules les étapes manquantes seront rejouées; les emails déjà envoyés ne seront pas doublés.", func(confirm bool) {
								if !confirm {
									return
								}
								actionToken := sessionToken
								go func() {
									err := RetryOrderFulfillment(actionToken, o.OrderID)
									fyne.Do(func() {
										if err != nil {
											dialog.ShowError(err, mainWindow)
										} else {
											dialog.ShowInformation("Succès", "Les étapes manquantes ont été exécutées.", mainWindow)
											reloadOrders()
										}
									})
								}()
							}, mainWindow)
						})
						btn.Importance = widget.WarningImportance
						actionBtn = btn
					} else if o.LicenseID != "" {
						actionBtn = widget.NewLabel("Licence : " + o.LicenseID)
					} else {
						actionBtn = widget.NewLabel("—")
					}

					delBtn := widget.NewButton("🗑️ Supprimer", func() {
						dialog.ShowConfirm("Supprimer la commande", fmt.Sprintf("Confirmez-vous la suppression définitive de la commande %s ?\n\nCette action est irréversible.", o.OrderID), func(confirm bool) {
							if !confirm {
								return
							}
							actionToken := sessionToken
							go func() {
								err := DeleteOrder(actionToken, o.OrderID)
								fyne.Do(func() {
									if err != nil {
										dialog.ShowError(err, mainWindow)
									} else {
										dialog.ShowInformation("Succès", "Commande supprimée avec succès.", mainWindow)
										reloadOrders()
									}
								})
							}()
						}, mainWindow)
					})
					delBtn.Importance = widget.DangerImportance

					actionsBox := container.NewHBox(actionBtn, delBtn)
					row := container.NewHBox(info, layout.NewSpacer(), actionsBox)
					rows = append(rows, row, widget.NewSeparator())
				}

				ordersContainer.Objects = rows
				ordersContainer.Refresh()
			})
		}()
	}

	reloadOrders()
	return scroll
}

func stageIcon(done bool) string {
	if done {
		return "✅"
	}
	return "❌"
}

// =============================================================================
// TAB 5 : ALERTES DE SÉCURITÉ
// =============================================================================
func buildAlertsTab() fyne.CanvasObject {
	alertsContainer := container.NewVBox()
	scroll := container.NewVScroll(alertsContainer)
	scroll.SetMinSize(fyne.NewSize(900, 480))
	token := sessionToken

	go func() {
		alerts, err := ListAlerts(token)
		if err != nil {
			fyne.Do(func() {
				alertsContainer.Objects = []fyne.CanvasObject{widget.NewLabel("Erreur : " + err.Error())}
				alertsContainer.Refresh()
			})
			return
		}

		fyne.Do(func() {
			if len(alerts) == 0 {
				alertsContainer.Objects = []fyne.CanvasObject{widget.NewLabel("🛡️ Aucune alerte de sécurité. Tous les flux sont conformes.")}
				alertsContainer.Refresh()
				return
			}

			var rows []fyne.CanvasObject
			for _, a := range alerts {
				info := widget.NewLabel(fmt.Sprintf(
					"Type: %s | Code: %s | Date: %s\nIP 1: %s -> IP 2: %s\nMessage: %s",
					a.Type, a.Code, a.CreatedAt, a.FirstIP, a.SecondIP, a.Message,
				))
				rows = append(rows, info, widget.NewSeparator())
			}

			alertsContainer.Objects = rows
			alertsContainer.Refresh()
		})
	}()

	return scroll
}

// =============================================================================
// TAB 5 : FACTURES & AVOIRS
// =============================================================================
func buildInvoicesTab() fyne.CanvasObject {
	invoicesContainer := container.NewVBox()
	scroll := container.NewVScroll(invoicesContainer)
	scroll.SetMinSize(fyne.NewSize(900, 480))

	var reloadInvoices func()
	reloadInvoices = func() {
		invoicesContainer.Objects = []fyne.CanvasObject{widget.NewLabel("Chargement des factures...")}
		invoicesContainer.Refresh()
		token := sessionToken

		go func() {
			invoices, err := ListInvoices(token)
			if err != nil {
				fyne.Do(func() {
					invoicesContainer.Objects = []fyne.CanvasObject{widget.NewLabel("Erreur : " + err.Error())}
					invoicesContainer.Refresh()
				})
				return
			}

			fyne.Do(func() {
				if len(invoices) == 0 {
					invoicesContainer.Objects = []fyne.CanvasObject{widget.NewLabel("Aucune facture trouvée.")}
					invoicesContainer.Refresh()
					return
				}

				var rows []fyne.CanvasObject
				for _, inv := range invoices {
					iv := inv
					isCredit := iv.Status == "credit_note" || strings.HasPrefix(iv.InvoiceNumber, "AV-")
					statusLabel := "Payée"
					if isCredit {
						statusLabel = "Avoir émis"
					}

					origin := "Auto"
					if iv.IsManual {
						origin = "Manuelle"
					}

					notesText := ""
					if iv.Notes != "" {
						notesText = fmt.Sprintf(" | Notes: %s", iv.Notes)
					}

					datePart := iv.CreatedAt
					if strings.Contains(datePart, "T") {
						datePart = strings.Split(datePart, "T")[0]
					}

					info := widget.NewLabel(fmt.Sprintf(
						"Facture: %s | Type: %s | Date: %s\nClient: %s (%s) | Offre: %s (%d tech)\nMontant: %.2f € | Statut: %s%s",
						iv.InvoiceNumber, origin, datePart,
						iv.CustomerName, iv.CustomerEmail, iv.Plan, iv.Technicians,
						iv.AmountTTC, statusLabel, notesText,
					))

					var actionsBox *fyne.Container
					delBtn := widget.NewButton("🗑️ Supprimer", func() {
						dialog.ShowConfirm("Supprimer la facture", fmt.Sprintf("⚠️ Confirmez-vous la suppression définitive de la facture %s ?\n\nCette action est irréversible et supprimera le PDF associé.", iv.InvoiceNumber), func(confirm bool) {
							if !confirm {
								return
							}
							actionToken := sessionToken
							go func() {
								err := DeleteInvoice(actionToken, iv.InvoiceNumber)
								fyne.Do(func() {
									if err != nil {
										dialog.ShowError(err, mainWindow)
									} else {
										dialog.ShowInformation("Succès", fmt.Sprintf("Facture %s supprimée avec succès.", iv.InvoiceNumber), mainWindow)
										reloadInvoices()
									}
								})
							}()
						}, mainWindow)
					})
					delBtn.Importance = widget.DangerImportance

					if !isCredit {
						creditBtn := widget.NewButton("↩️ Avoir", func() {
							reasonEntry := widget.NewEntry()
							reasonEntry.SetText("Remboursement / Annulation")
							dialog.ShowCustomConfirm(
								fmt.Sprintf("Émettre un Avoir pour %s", iv.InvoiceNumber),
								"Créer l'Avoir",
								"Annuler",
								container.NewVBox(
									widget.NewLabel(fmt.Sprintf("Montant crédité : %.2f €", iv.AmountTTC)),
									widget.NewLabel("Motif de l'avoir :"),
									reasonEntry,
								),
								func(confirm bool) {
									if !confirm {
										return
									}
									actionToken := sessionToken
									reason := strings.TrimSpace(reasonEntry.Text)
									go func() {
										err := CreateCreditNote(actionToken, iv.InvoiceNumber, reason)
										fyne.Do(func() {
											if err != nil {
												dialog.ShowError(err, mainWindow)
											} else {
												dialog.ShowInformation("Succès", "Facture d'avoir générée avec succès !", mainWindow)
												reloadInvoices()
											}
										})
									}()
								},
								mainWindow,
							)
						})
						creditBtn.Importance = widget.WarningImportance
						actionsBox = container.NewHBox(creditBtn, delBtn)
					} else {
						actionsBox = container.NewHBox(delBtn)
					}

					row := container.NewHBox(info, layout.NewSpacer(), actionsBox)
					rows = append(rows, row, widget.NewSeparator())
				}

				invoicesContainer.Objects = rows
				invoicesContainer.Refresh()
			})
		}()
	}

	reloadInvoices()

	topBar := container.NewHBox(
		widget.NewLabelWithStyle("Factures conformes micro-entreprise archivées sur le serveur", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		layout.NewSpacer(),
		widget.NewButton("🔄 Actualiser les factures", func() {
			reloadInvoices()
		}),
	)

	return container.NewBorder(topBar, nil, nil, nil, scroll)
}

// =============================================================================
// TAB 7 : PARC MACHINES & DOSSIERS
// =============================================================================
func buildFleetTab() fyne.CanvasObject {
	statsLabel := widget.NewLabel("Chargement du parc de machines...")
	statsLabel.TextStyle = fyne.TextStyle{Bold: true}

	listContainer := container.NewVBox()
	listScroll := container.NewVScroll(listContainer)
	listScroll.SetMinSize(fyne.NewSize(900, 360))

	var allDevices []AdminDeviceItem
	var allFolders []AdminFolderItem

	currentFolderID := "ALL"
	searchQuery := ""

	type folderOpt struct {
		id    string
		label string
	}

	buildFolderOpts := func(folders []AdminFolderItem) []folderOpt {
		opts := []folderOpt{
			{id: "ALL", label: "📁 Tous les dossiers"},
			{id: "", label: "📁 Racine (aucun dossier)"},
		}
		var addSubs func(parentID string, depth int)
		addSubs = func(parentID string, depth int) {
			for _, f := range folders {
				if f.ParentFolderID == parentID {
					prefix := "  📁 "
					if depth > 0 {
						prefix = strings.Repeat("    ", depth) + "↳ 📁 "
					}
					opts = append(opts, folderOpt{id: f.FolderID, label: prefix + f.Name})
					addSubs(f.FolderID, depth+1)
				}
			}
		}
		addSubs("", 0)
		return opts
	}

	getFolderName := func(id string) string {
		for _, f := range allFolders {
			if f.FolderID == id {
				return f.Name
			}
		}
		return "Racine"
	}

	var folderSelect *widget.Select
	var renameFolderBtn *widget.Button
	var deleteFolderBtn *widget.Button

	var reloadFleet func()

	filterAndRender := func() {
		filtered := []AdminDeviceItem{}
		for _, d := range allDevices {
			if currentFolderID != "ALL" {
				if d.FolderID != currentFolderID {
					continue
				}
			}
			if searchQuery != "" {
				fName := strings.ToLower(getFolderName(d.FolderID))
				if !strings.Contains(strings.ToLower(d.Alias), searchQuery) &&
					!strings.Contains(strings.ToLower(d.Hostname), searchQuery) &&
					!strings.Contains(strings.ToLower(d.DeviceID), searchQuery) &&
					!strings.Contains(strings.ToLower(d.RustDeskID), searchQuery) &&
					!strings.Contains(strings.ToLower(d.OS), searchQuery) &&
					!strings.Contains(fName, searchQuery) {
					continue
				}
			}
			filtered = append(filtered, d)
		}

		if len(filtered) == 0 {
			listContainer.Objects = []fyne.CanvasObject{
				container.NewCenter(widget.NewLabelWithStyle("Aucun poste trouvé.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})),
			}
			listContainer.Refresh()
			return
		}

		rows := []fyne.CanvasObject{}
		for _, dev := range filtered {
			d := dev
			statusIcon := "⚪"
			if d.Status == "online" {
				statusIcon = "🟢"
			}
			osBadge := "💻"
			if strings.Contains(strings.ToLower(d.OS), "win") {
				osBadge = "🪟 Windows"
			} else if strings.Contains(strings.ToLower(d.OS), "linux") {
				osBadge = "🐧 Linux"
			} else if strings.Contains(strings.ToLower(d.OS), "mac") || strings.Contains(strings.ToLower(d.OS), "darwin") {
				osBadge = "🍏 macOS"
			}

			title := d.Alias
			if title == "" {
				title = d.Hostname
			}
			if title == "" {
				title = d.DeviceID
			}

			folderDisplay := "📁 " + getFolderName(d.FolderID)
			details := fmt.Sprintf("%s %s • %s • RustDesk: %s • Dossier: %s", statusIcon, title, osBadge, d.RustDeskID, folderDisplay)
			infoLabel := widget.NewLabelWithStyle(details, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

			moveBtn := widget.NewButton("📁 Déplacer", func() {
				opts := buildFolderOpts(allFolders)
				labels := make([]string, 0, len(opts))
				targetID := ""
				initIdx := 0
				for _, o := range opts {
					if o.id == "ALL" {
						continue
					}
					labels = append(labels, o.label)
					if o.id == d.FolderID {
						initIdx = len(labels) - 1
						targetID = o.id
					}
				}
				sel := widget.NewSelect(labels, func(s string) {
					for _, o := range opts {
						if o.label == s {
							targetID = o.id
							break
						}
					}
				})
				if len(labels) > 0 {
					sel.SetSelected(labels[initIdx])
				}
				dConfirm := dialog.NewCustomConfirm("Déplacer l'appareil", "Déplacer", "Annuler",
					container.NewVBox(
						widget.NewLabel(fmt.Sprintf("Choisir le dossier de destination pour %s :", title)),
						sel,
					),
					func(ok bool) {
						if ok {
							tok := sessionToken
							go func() {
								err := UpdateAdminDeviceFolder(tok, d.DeviceID, targetID)
								fyne.Do(func() {
									if err != nil {
										dialog.ShowError(err, mainWindow)
									} else {
										reloadFleet()
									}
								})
							}()
						}
					}, mainWindow)
				dConfirm.Resize(fyne.NewSize(400, 200))
				dConfirm.Show()
			})

			delBtn := widget.NewButton("❌ Retirer", func() {
				dialog.ShowConfirm("Supprimer le poste", fmt.Sprintf("Voulez-vous vraiment retirer le poste %s du parc ?", title), func(ok bool) {
					if ok {
						tok := sessionToken
						go func() {
							err := DeleteAdminDevice(tok, d.DeviceID)
							fyne.Do(func() {
								if err != nil {
									dialog.ShowError(err, mainWindow)
								} else {
									reloadFleet()
								}
							})
						}()
					}
				}, mainWindow)
			})
			delBtn.Importance = widget.DangerImportance

			row := container.NewBorder(nil, nil, nil, container.NewHBox(moveBtn, delBtn), infoLabel)
			rows = append(rows, row, widget.NewSeparator())
		}
		listContainer.Objects = rows
		listContainer.Refresh()
	}

	openFolderModal := func(defaultParentID string) {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Ex: Agence Marseille, Comptabilité...")

		opts := buildFolderOpts(allFolders)
		labels := []string{}
		selectedParent := ""
		initIdx := 0
		for _, o := range opts {
			if o.id == "ALL" {
				continue
			}
			labels = append(labels, o.label)
			if defaultParentID != "" && defaultParentID != "ALL" && o.id == defaultParentID {
				selectedParent = o.id
				initIdx = len(labels) - 1
			}
		}

		parentSelect := widget.NewSelect(labels, func(s string) {
			for _, o := range opts {
				if o.label == s {
					selectedParent = o.id
					break
				}
			}
		})
		if len(labels) > 0 {
			parentSelect.SetSelected(labels[initIdx])
		}

		title := "➕ Nouveau dossier"
		if defaultParentID != "" && defaultParentID != "ALL" {
			title = "➕ Nouveau sous-dossier"
		}

		form := container.NewVBox(
			widget.NewLabelWithStyle("Nom du dossier :", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			nameEntry,
			widget.NewLabelWithStyle("Emplacement (dossier parent) :", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			parentSelect,
		)

		d := dialog.NewCustomConfirm(title, "Créer", "Annuler", form, func(ok bool) {
			if !ok {
				return
			}
			name := strings.TrimSpace(nameEntry.Text)
			if name == "" {
				return
			}
			tok := sessionToken
			go func() {
				_, err := CreateAdminFolder(tok, name, selectedParent)
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, mainWindow)
					} else {
						reloadFleet()
					}
				})
			}()
		}, mainWindow)
		d.Resize(fyne.NewSize(420, 240))
		d.Show()
	}

	newFolderBtn := widget.NewButton("➕ Nouveau dossier", func() {
		openFolderModal("")
	})
	newFolderBtn.Importance = widget.HighImportance

	newSubFolderBtn := widget.NewButton("➕ Sous-dossier", func() {
		target := currentFolderID
		if target == "ALL" {
			target = ""
		}
		openFolderModal(target)
	})

	renameFolderBtn = widget.NewButton("✏️ Renommer", func() {
		if currentFolderID == "ALL" || currentFolderID == "" {
			dialog.ShowInformation("Renommer", "Veuillez sélectionner un dossier existant.", mainWindow)
			return
		}
		curName := getFolderName(currentFolderID)
		ed := dialog.NewEntryDialog("Renommer le dossier", "Nouveau nom :", func(name string) {
			name = strings.TrimSpace(name)
			if name == "" || name == curName {
				return
			}
			tok := sessionToken
			go func() {
				err := UpdateAdminFolder(tok, currentFolderID, name)
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, mainWindow)
					} else {
						reloadFleet()
					}
				})
			}()
		}, mainWindow)
		ed.SetText(curName)
		ed.Show()
	})

	deleteFolderBtn = widget.NewButton("🗑️ Supprimer", func() {
		if currentFolderID == "ALL" || currentFolderID == "" {
			dialog.ShowInformation("Supprimer", "Veuillez sélectionner un dossier à supprimer.", mainWindow)
			return
		}
		curName := getFolderName(currentFolderID)
		dialog.ShowConfirm("Supprimer le dossier", fmt.Sprintf("Supprimer le dossier « %s » ? Ses sous-dossiers et appareils seront déplacés à la racine.", curName), func(ok bool) {
			if ok {
				tok := sessionToken
				go func() {
					err := DeleteAdminFolder(tok, currentFolderID)
					fyne.Do(func() {
						if err != nil {
							dialog.ShowError(err, mainWindow)
						} else {
							currentFolderID = "ALL"
							reloadFleet()
						}
					})
				}()
			}
		}, mainWindow)
	})
	deleteFolderBtn.Importance = widget.DangerImportance

	folderSelect = widget.NewSelect([]string{"📁 Tous les dossiers"}, func(s string) {
		opts := buildFolderOpts(allFolders)
		for _, o := range opts {
			if o.label == s {
				currentFolderID = o.id
				break
			}
		}
		inFolder := currentFolderID != "ALL" && currentFolderID != ""
		if inFolder {
			renameFolderBtn.Show()
			deleteFolderBtn.Show()
		} else {
			renameFolderBtn.Hide()
			deleteFolderBtn.Hide()
		}
		filterAndRender()
	})
	renameFolderBtn.Hide()
	deleteFolderBtn.Hide()

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("🔍 Rechercher par nom, hôte, ID RustDesk, dossier...")
	searchEntry.OnChanged = func(s string) {
		searchQuery = strings.ToLower(strings.TrimSpace(s))
		filterAndRender()
	}

	// Enrollment Card
	aliasEntry := widget.NewEntry()
	aliasEntry.SetPlaceHolder("Alias du poste (ex: Serveur Compta)")
	notesEntry := widget.NewEntry()
	notesEntry.SetPlaceHolder("Notes (ex: Salle serveur - 2ème étage)")
	enrollResult := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	enrollResult.Importance = widget.SuccessImportance

	enrollBtn := widget.NewButton("⚡ Générer code d'enrôlement permanent", func() {
		alias := strings.TrimSpace(aliasEntry.Text)
		if alias == "" {
			dialog.ShowInformation("Erreur", "L'alias du poste est requis.", mainWindow)
			return
		}
		notes := strings.TrimSpace(notesEntry.Text)
		tok := sessionToken
		go func() {
			code, err := GenerateAdminPermanentCode(tok, alias, notes)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, mainWindow)
				} else {
					enrollResult.SetText("Code : " + code)
					mainWindow.Clipboard().SetContent(code)
					dialog.ShowInformation("Succès", fmt.Sprintf("Code généré et copié dans le presse-papier :\n%s\n\nLancez 'viewer.exe --enroll %s' sur le poste distant.", code, code), mainWindow)
					reloadFleet()
				}
			})
		}()
	})
	enrollBtn.Importance = widget.HighImportance

	enrollCard := widget.NewCard("Enrôler un nouveau poste permanent", "",
		container.NewVBox(
			container.NewGridWithColumns(2,
				container.NewVBox(widget.NewLabel("Nom / Alias du poste :"), aliasEntry),
				container.NewVBox(widget.NewLabel("Notes / Emplacement :"), notesEntry),
			),
			container.NewHBox(enrollBtn, enrollResult),
		),
	)

	reloadFleet = func() {
		tok := sessionToken
		go func() {
			devs, err1 := ListAdminDevices(tok)
			folders, err2 := ListAdminFolders(tok)
			fyne.Do(func() {
				if err1 != nil {
					statsLabel.SetText("Erreur de chargement : " + err1.Error())
					return
				}
				if err2 == nil {
					allFolders = folders
				}
				allDevices = devs

				online := 0
				for _, d := range allDevices {
					if d.Status == "online" {
						online++
					}
				}
				statsLabel.SetText(fmt.Sprintf("🖥️ %d postes enregistrés (%d en ligne, %d hors ligne) • 📁 %d dossiers",
					len(allDevices), online, len(allDevices)-online, len(allFolders)))

				opts := buildFolderOpts(allFolders)
				labels := make([]string, len(opts))
				selectedLabel := opts[0].label
				for i, o := range opts {
					labels[i] = o.label
					if o.id == currentFolderID {
						selectedLabel = o.label
					}
				}
				folderSelect.Options = labels
				folderSelect.SetSelected(selectedLabel)

				filterAndRender()
			})
		}()
	}

	reloadFleet()

	folderToolbar := container.NewHBox(
		widget.NewLabelWithStyle("Dossier :", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		folderSelect,
		newFolderBtn,
		newSubFolderBtn,
		renameFolderBtn,
		deleteFolderBtn,
		layout.NewSpacer(),
		widget.NewButton("🔄 Actualiser", func() {
			reloadFleet()
		}),
	)

	topSection := container.NewVBox(
		statsLabel,
		widget.NewSeparator(),
		enrollCard,
		widget.NewSeparator(),
		searchEntry,
		folderToolbar,
		widget.NewSeparator(),
	)

	return container.NewBorder(topSection, nil, nil, nil, listScroll)
}

