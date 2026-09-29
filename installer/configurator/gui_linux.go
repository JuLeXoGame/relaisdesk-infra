//go:build linux

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func RunGUI() error {
	fmt.Printf("=== %s - Espace Technicien (v%s) ===\n", PRODUCT_NAME, APP_VERSION)

	// 1. Authentification / Chargement de la licence
	loginResp, licenseID, licenseKey, err := authenticateTechnicianLinux()
	if err != nil {
		showErrorLinux("RelaisDesk Technicien", fmt.Sprintf("Authentification impossible : %v", err))
		return err
	}

	activation := &ActivationResponse{
		LicenseID:      licenseID,
		ServerIP:       loginResp.ServerIP,
		RendezvousPort: loginResp.RendezvousPort,
		RelayPort:      loginResp.RelayPort,
		PublicKey:      loginResp.PublicKey,
	}

	// 2. Recherche / Installation de RustDesk
	fmt.Println("Vérification de RustDesk...")
	rustdeskPath, err := ensureRustDesk()
	if err != nil {
		msg := fmt.Sprintf("RustDesk est introuvable ou son empreinte ne correspond pas : %v", err)
		showErrorLinux("RelaisDesk Technicien", msg)
		return err
	}

	// 3. Autorisation réseau sécurisée
	fmt.Println("Initialisation de l'autorisation réseau...")
	networkAuthorization, err := prepareTechnicianNetworkAuthorization(loginResp.Token)
	if err != nil {
		msg := fmt.Sprintf("Erreur d'autorisation réseau : %v", err)
		showErrorLinux("RelaisDesk Technicien", msg)
		return fmt.Errorf("autorisation réseau: %w", err)
	}
	defer removeNetworkToken(networkAuthorization)
	defer cleanupRustDesk2Toml()

	activation.NetworkTokenFile = networkAuthorization.TokenFile
	activation.NetworkProofKeyFile = networkAuthorization.ProofKeyFile
	linuxServiceTokenFile = networkAuthorization.TokenFile

	// 4. Configuration de RustDesk
	if err := configureRustDesk(activation, licenseKey); err != nil {
		showErrorLinux("RelaisDesk Technicien", fmt.Sprintf("Erreur de configuration RustDesk : %v", err))
		return err
	}

	// 4b. Redémarrage du moteur pour charger la configuration fraîche.
	fmt.Println("Redémarrage de RustDesk...")
	if err := restartRustDeskForConfig(rustdeskPath); err != nil {
		showErrorLinux("RelaisDesk Technicien", fmt.Sprintf("Erreur de redémarrage RustDesk : %v", err))
		return err
	}

	// 5. Contexte et boucle de renouvellement de jeton en tâche de fond
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Until(networkAuthorization.ExpiresAt) <= 2*time.Minute {
					if err := refreshTechnicianNetworkAuthorization(loginResp.Token, networkAuthorization); err != nil && time.Now().After(networkAuthorization.ExpiresAt) {
						removeNetworkToken(networkAuthorization)
						fmt.Printf("Avertissement: autorisation réseau expirée : %v\n", err)
					}
				}
			}
		}
	}()

	// 6. Prise en charge d'un URI de connexion directe (ex: relaisdesk://connect/DEV-XXXX-XXXX)
	if pendingFleetError != "" {
		fmt.Fprintf(os.Stderr, "Lien de parc invalide : %s\n", pendingFleetError)
		showErrorLinux("RelaisDesk Technicien", pendingFleetError)
		pendingFleetError = ""
	}
	if pendingFleetDevice != "" {
		targetID := pendingFleetDevice
		pendingFleetDevice = ""
		handleFleetDirectConnect(loginResp.Token, rustdeskPath, targetID)
	}

	// Notification de bienvenue
	notifyDesktopLinux("RelaisDesk Technicien", fmt.Sprintf("Connecté en tant que %s.\nLicence valide jusqu'au %s.", loginResp.Email, loginResp.ExpiresAt))

	// 7. Boucle du menu interactif
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nFermeture de RelaisDesk Technicien...")
			return nil
		default:
		}

		choice, ok := promptSelectLinux(
			fmt.Sprintf("RelaisDesk Technicien v%s", APP_VERSION),
			fmt.Sprintf("Connecté : %s (Licence : %s)\nChoisissez une action :", loginResp.Email, licenseID),
			[][2]string{
				{"rustdesk", "🚀 Lancer RustDesk"},
				{"temp_code", "🎟️ Générer un code d'assistance (12h)"},
				{"assistance", "Connexions aux codes d'assistance"},
				{"fleet", "🖥️ Postes permanents (Prendre la main)"},
				{"enroll", "➕ Enrôler un nouveau poste permanent"},
				{"web", "🌐 Ouvrir l'Espace Web Technicien"},
				{"update", "🔄 Vérifier les mises à jour"},
				{"diag", "🩺 Diagnostic réseau"},
				{"logout", "🚪 Se déconnecter (changer de compte)"},
				{"exit", "❌ Quitter"},
			},
		)

		if !ok || choice == "exit" {
			fmt.Println("Fermeture de RelaisDesk Technicien.")
			break
		}

		switch choice {
		case "rustdesk":
			fmt.Println("Lancement de RustDesk...")
			if err := launchRustDesk(rustdeskPath); err != nil {
				showErrorLinux("RelaisDesk", fmt.Sprintf("Erreur lors du lancement de RustDesk : %v", err))
			} else {
				showInfoLinux("RelaisDesk", "RustDesk a été démarré avec la configuration sécurisée RelaisDesk.")
			}

		case "temp_code":
			handleGenerateTempCodeLinux(loginResp.Token)
		case "assistance":
			handleServiceCodesLinux(loginResp.Token, networkAuthorization.TokenFile, rustdeskPath)

		case "fleet":
			handleFleetManagementLinux(loginResp.Token, rustdeskPath)

		case "enroll":
			handleEnrollFleetLinux(loginResp.Token)

		case "web":
			openBrowserLinux("https://relaisdesk.fr/technicien/")

		case "update":
			handleCheckUpdateLinux()

		case "diag":
			diagCtx, diagCancel := context.WithTimeout(context.Background(), 12*time.Second)
			report := runConnectivityDiagnostics(diagCtx, APIURL, activation)
			diagCancel()
			showInfoLinux("Diagnostic Réseau", report.String())

		case "logout":
			if confirmDialogLinux("Déconnexion", "Voulez-vous vraiment vous déconnecter et réinitialiser les identifiants enregistrés ?") {
				_ = ClearLicense()
				_ = logoutTechnician(loginResp.Token)
				showInfoLinux("RelaisDesk", "Vous avez été déconnecté avec succès.")
				return nil
			}
		}
	}

	return nil
}

// authenticateTechnicianLinux gère la connexion avec identifiants enregistrés ou saisie
func authenticateTechnicianLinux() (*TechnicianLoginResponse, string, string, error) {
	// Tentative avec identifiants sauvegardés
	if saved, err := LoadCredentials(); err == nil && saved != nil {
		var ident, secret string
		if saved.Email != "" && saved.Password != "" {
			ident = saved.Email
			secret = saved.Password
		} else if saved.LicenseID != "" && saved.LicenseKey != "" {
			ident = saved.LicenseID
			secret = saved.LicenseKey
		}

		if ident != "" && secret != "" {
			fmt.Printf("Authentification avec les identifiants mémorisés (%s)...\n", ident)
			resp, err := loginTechnician(ident, secret, "", saved.DeviceToken)
			if err == nil {
				if resp.Requires2FA {
					twoFAResp, twoFAErr := handle2FALinux(resp.ChallengeToken)
					if twoFAErr == nil {
						effectiveLicID := twoFAResp.LicenseID
						if effectiveLicID == "" {
							effectiveLicID = saved.LicenseID
						}
						if twoFAResp.DeviceToken != "" {
							if saved.Email != "" {
								_ = SaveCredentials(saved.Email, saved.Password, twoFAResp.DeviceToken)
							} else {
								_ = SaveLicense(saved.LicenseID, saved.LicenseKey, twoFAResp.DeviceToken)
							}
						}
						return twoFAResp, effectiveLicID, secret, nil
					}
					fmt.Printf("Échec de la validation 2FA : %v\n", twoFAErr)
				} else {
					effectiveLicID := resp.LicenseID
					if effectiveLicID == "" {
						effectiveLicID = saved.LicenseID
					}
					return resp, effectiveLicID, secret, nil
				}
			} else {
				fmt.Printf("Identifiants enregistrés invalides ou expirés (%v). Nouvelle saisie requise.\n", err)
				_ = ClearLicense()
			}
		}
	}

	// Saisie utilisateur
	for {
		choice, ok := promptSelectLinux(
			"RelaisDesk - Connexion Technicien",
			"Choisissez un mode de connexion :",
			[][2]string{
				{"credentials", "🔑 Adresse e-mail / Mot de passe (ou Clé de Licence)"},
				{"google", "🌐 Se connecter avec Google"},
			},
		)
		if !ok {
			return nil, "", "", fmt.Errorf("connexion annulée")
		}

		if choice == "google" {
			fmt.Println("Lancement de la connexion avec Google...")
			fmt.Println("Une page va s'ouvrir dans votre navigateur pour vous authentifier.")
			savedCreds, _ := LoadCredentials()
			deviceToken := ""
			if savedCreds != nil {
				deviceToken = savedCreds.DeviceToken
			}
			resp, err := performGoogleOAuthFlow(context.Background(), deviceToken)
			if err != nil {
				showErrorLinux("Erreur de connexion Google", fmt.Sprintf("Connexion Google échouée : %v", err))
				continue
			}

			if resp.Requires2FA {
				twoFAResp, twoFAErr := handle2FALinux(resp.ChallengeToken)
				if twoFAErr != nil {
					showErrorLinux("Erreur 2FA", fmt.Sprintf("Double authentification échouée : %v", twoFAErr))
					continue
				}
				resp = twoFAResp
			}

			if resp.Email != "" && resp.DeviceToken != "" {
				_ = SaveCredentials(resp.Email, "", resp.DeviceToken)
			}

			effectiveLicID := resp.LicenseID
			if effectiveLicID == "" {
				effectiveLicID = resp.Email
			}
			return resp, effectiveLicID, "", nil
		}

		ident, ok := promptEntryLinux("RelaisDesk - Connexion Technicien", "Adresse e-mail (ou ID Licence) :", "", false)
		if !ok || strings.TrimSpace(ident) == "" {
			return nil, "", "", fmt.Errorf("connexion annulée")
		}
		ident = strings.TrimSpace(ident)

		pwdPrompt := "Mot de passe :"
		if !strings.Contains(ident, "@") {
			pwdPrompt = "Clé secrète de Licence :"
		}
		pwd, ok := promptEntryLinux("RelaisDesk - Connexion Technicien", pwdPrompt, "", true)
		if !ok || strings.TrimSpace(pwd) == "" {
			return nil, "", "", fmt.Errorf("connexion annulée")
		}
		pwd = strings.TrimSpace(pwd)

		fmt.Println("Vérification auprès du serveur...")
		resp, err := loginTechnician(ident, pwd)
		if err != nil {
			showErrorLinux("Erreur de connexion", fmt.Sprintf("Connexion refusée : %v", err))
			continue
		}

		if resp.Requires2FA {
			twoFAResp, twoFAErr := handle2FALinux(resp.ChallengeToken)
			if twoFAErr != nil {
				showErrorLinux("Erreur 2FA", fmt.Sprintf("Double authentification échouée : %v", twoFAErr))
				continue
			}
			resp = twoFAResp
		}

		if strings.Contains(ident, "@") {
			_ = SaveCredentials(ident, pwd, resp.DeviceToken)
		} else {
			_ = SaveLicense(ident, pwd, resp.DeviceToken)
		}

		effectiveLicID := resp.LicenseID
		if effectiveLicID == "" {
			effectiveLicID = ident
		}
		return resp, effectiveLicID, pwd, nil
	}
}

func handle2FALinux(challengeToken string) (*TechnicianLoginResponse, error) {
	methodChoice, ok := promptSelectLinux(
		"Double Authentification (2FA)",
		"Choisissez votre méthode d'authentification :",
		[][2]string{
			{"totp", "🔑 Code Authenticator (Google Authenticator, Authy...)"},
			{"email", "📧 Recevoir un code temporaire par e-mail"},
		},
	)
	if !ok {
		return nil, fmt.Errorf("authentification 2FA annulée")
	}

	if methodChoice == "email" {
		masked, err := requestTechnician2FAEmailCode(challengeToken)
		if err != nil {
			showErrorLinux("Erreur d'envoi", fmt.Sprintf("Impossible d'envoyer le code : %v", err))
			return nil, err
		}
		showInfoLinux("Code envoyé", fmt.Sprintf("Un code de sécurité à 6 chiffres a été envoyé à : %s\n(Valable 15 minutes)", masked))
	}

	code, ok := promptEntryLinux("Double Authentification (2FA)", "Entrez le code à 6 chiffres (reçu par e-mail ou Authenticator) :", "", false)
	if !ok || strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("authentification 2FA annulée")
	}

	rememberDevice := confirmDialogLinux("Appareil de confiance", "Faire confiance à cet appareil pendant 30 jours (ne plus redemander de code 2FA) ?")
	return loginTechnician2FA(challengeToken, strings.TrimSpace(code), rememberDevice)
}

func handleGenerateTempCodeLinux(token string) {
	email, _ := promptEntryLinux("Assistance temporaire", "Nom ou adresse e-mail du client (optionnel) :", "", false)
	res, err := generateViewerCode(token, strings.TrimSpace(email))
	if err != nil {
		showErrorLinux("Erreur", fmt.Sprintf("Impossible de générer le code : %v", err))
		return
	}

	copyToClipboardLinux(res.Code)

	expStr := res.ExpiresAt
	if t, parseErr := time.Parse(time.RFC3339, res.ExpiresAt); parseErr == nil {
		expStr = t.Format("02/01/2006 à 15:04")
	}

	msg := fmt.Sprintf("Code d'accès généré avec succès :\n\n  %s\n\nValable jusqu'au : %s\n\nLe code a été copié dans le presse-papiers.\nTransmettez ce code à votre client.", res.Code, expStr)
	showInfoLinux("Code d'assistance", msg)
}

func handleFleetManagementLinux(token, rustdeskPath string) {
	currentFolderID := "ALL"

	for {
		devices, err := listTechnicianDevices(token)
		if err != nil {
			showErrorLinux("Parc permanent", fmt.Sprintf("Erreur lors de la récupération des postes : %v", err))
			return
		}
		if len(devices) == 0 {
			showInfoLinux("Parc permanent", "Aucun poste permanent n'est actuellement enregistré.\nUtilisez 'Enrôler un nouveau poste' pour en ajouter un.")
			return
		}
		folders, _ := listTechnicianFolders(token)

		// Filtrage hiérarchique récursif selon le dossier sélectionné
		var filtered []DeviceItem
		var allowedFolderIDs map[string]bool
		if currentFolderID != "ALL" && currentFolderID != "" {
			allowedFolderIDs = getFolderAndDescendantIDs(currentFolderID, folders)
		}

		for _, d := range devices {
			if currentFolderID == "" {
				if d.FolderID != "" {
					continue
				}
			} else if currentFolderID != "ALL" {
				if !allowedFolderIDs[d.FolderID] {
					continue
				}
			}
			filtered = append(filtered, d)
		}

		// Calcul du fil d'Ariane pour le dossier actif
		currentPath := "👁️ Tous les postes"
		if currentFolderID == "" {
			currentPath = "🏠 Racine"
		} else if currentFolderID != "ALL" {
			currentPath = "🏠 Racine > " + getFolderBreadcrumbPath(currentFolderID, folders)
		}

		items := make([][2]string, 0, len(filtered)+4)
		items = append(items, [2]string{"folder_nav", fmt.Sprintf("📁 Dossier : %s (%d postes)", currentPath, len(filtered))})
		items = append(items, [2]string{"new_folder", "➕ Créer un nouveau dossier / sous-dossier"})

		for _, dev := range filtered {
			statusLabel := "⚪ Hors ligne"
			if dev.EnrollmentState == "enrolled" && dev.RustDeskID != "" {
				statusLabel = "🟢 En ligne"
			}
			osIcon := "💻"
			osLower := strings.ToLower(dev.OS)
			switch {
			case strings.Contains(osLower, "win"):
				osIcon = "🪟"
			case strings.Contains(osLower, "darwin") || strings.Contains(osLower, "mac") || strings.Contains(osLower, "apple") || strings.Contains(osLower, "osx"):
				osIcon = "🍎"
			case strings.Contains(osLower, "linux"):
				osIcon = "🐧"
			}
			folderBadge := ""
			if dev.FolderID != "" {
				if bPath := getFolderBreadcrumbPath(dev.FolderID, folders); bPath != "" {
					folderBadge = fmt.Sprintf(" [📁 %s]", bPath)
				}
			}
			label := fmt.Sprintf("%s %s (%s)%s - %s", osIcon, dev.Alias, dev.DeviceID, folderBadge, statusLabel)
			if dev.Hostname != "" {
				label = fmt.Sprintf("%s %s [%s]%s - %s", osIcon, dev.Alias, dev.Hostname, folderBadge, statusLabel)
			}
			items = append(items, [2]string{dev.DeviceID, label})
		}
		items = append(items, [2]string{"back", "← Retour au menu principal"})

		choicePrompt := fmt.Sprintf("Fil d'Ariane : %s\nSélectionnez un poste pour agir ou changez de dossier :", currentPath)
		devChoice, ok := promptSelectLinux("Postes Permanents", choicePrompt, items)
		if !ok || devChoice == "back" {
			return
		}

		if devChoice == "folder_nav" {
			folderOptions := buildFolderOptions(folders, true)
			navItems := make([][2]string, 0, len(folderOptions))
			for _, fo := range folderOptions {
				count := 0
				if fo.id == "ALL" {
					count = len(devices)
				} else if fo.id == "" {
					for _, d := range devices {
						if d.FolderID == "" {
							count++
						}
					}
				} else {
					descIDs := getFolderAndDescendantIDs(fo.id, folders)
					for _, d := range devices {
						if descIDs[d.FolderID] {
							count++
						}
					}
				}
				navItems = append(navItems, [2]string{fo.id, fmt.Sprintf("%s (%d)", fo.label, count)})
			}
			navChoice, navOk := promptSelectLinux("Fil d'Ariane & Dossiers", "Sélectionnez un niveau hiérarchique :", navItems)
			if navOk {
				currentFolderID = navChoice
			}
			continue
		}

		if devChoice == "new_folder" {
			name, nameOk := promptEntryLinux("Nouveau dossier", "Nom du dossier ou sous-dossier :", "", false)
			if nameOk && strings.TrimSpace(name) != "" {
				parentOptions := buildFolderOptions(folders, false)
				pItems := make([][2]string, 0, len(parentOptions))
				for _, po := range parentOptions {
					pItems = append(pItems, [2]string{po.id, po.label})
				}
				pChoice, pOk := promptSelectLinux("Emplacement parent", "Sélectionnez le dossier parent (pour Niveau 1, 2 ou 3) :", pItems)
				if pOk {
					_, err := createTechnicianFolder(token, name, pChoice)
					if err != nil {
						showErrorLinux("Erreur création", fmt.Sprintf("Impossible de créer le dossier : %v", err))
					} else {
						showInfoLinux("Dossier créé", fmt.Sprintf("Le dossier '%s' a été créé avec succès.", name))
					}
				}
			}
			continue
		}

		// Trouver le device sélectionné
		var selectedDev *DeviceItem
		for i := range filtered {
			if filtered[i].DeviceID == devChoice {
				selectedDev = &filtered[i]
				break
			}
		}
		if selectedDev == nil {
			continue
		}

		devFolderPath := "📁 Racine"
		if selectedDev.FolderID != "" {
			if bp := getFolderBreadcrumbPath(selectedDev.FolderID, folders); bp != "" {
				devFolderPath = "📁 " + bp
			}
		}

		deviceActions := [][2]string{
			{"connect", "Connexion directe"},
		}
		if catalog, e := serviceCatalog(token); e == nil && (catalog.Enabled || len(catalog.Work) > 0) {
			deviceActions = append(deviceActions, [2]string{"service", "Connexion & Prestation"})
		}
		if selectedDev.EnrollmentState == "enrolled" && IsDeviceUpdateAvailable(selectedDev.AgentVersion) {
			deviceActions = append(deviceActions, [2]string{"update_device", "⬆️ Mise à jour disponible"})
		}
		deviceActions = append(deviceActions,
			[2]string{"edit", "📁 Déplacer / Renommer le poste"},
			[2]string{"delete", "🗑️ Supprimer ce poste du parc"},
			[2]string{"back", "← Retour"},
		)

		devOS := selectedDev.OS
		if devOS == "" {
			devOS = "Inconnu"
		}
		actionChoice, ok := promptSelectLinux(
			selectedDev.Alias,
			fmt.Sprintf("Poste : %s (%s)\nOS : %s   •   Dossier : %s\nID RustDesk : %s\nQue souhaitez-vous faire ?", selectedDev.Alias, selectedDev.DeviceID, devOS, devFolderPath, selectedDev.RustDeskID),
			deviceActions,
		)
		if !ok || actionChoice == "back" {
			continue
		}

		switch actionChoice {
		case "service":
			openServiceLinux(token, linuxServiceTokenFile, rustdeskPath, "device", selectedDev.DeviceID)
		case "connect":
			if selectedDev.RustDeskID == "" {
				showErrorLinux("Connexion impossible", "Ce poste n'a pas encore transmis d'identifiant RustDesk valide.")
				continue
			}

			// Si le poste est éteint / hors ligne, réveil automatique (local + relais cloud)
			if selectedDev.Status != "online" {
				go func(targetID, mac string) {
					if mac != "" {
						_, _ = SendWakeOnLAN(mac)
					}
					if token != "" {
						_, _ = wakeTechnicianDevice(token, targetID)
					}
				}(selectedDev.DeviceID, selectedDev.MACAddress)

				showInfoLinux("Réveil automatique", fmt.Sprintf("⚡ Le poste '%s' est actuellement éteint / hors ligne.\n\nUn signal de réveil (Wake-on-LAN) a été transmis automatiquement (en local et via les postes relais du réseau).\n\nLe démarrage de la machine peut prendre 1 à 2 minutes.", selectedDev.Alias))
			}

			pwd, _ := promptEntryLinux(
				"Mot de passe permanent",
				fmt.Sprintf("Entrez le mot de passe permanent pour %s :\n(Laisser vide si déjà mémorisé dans RustDesk)", selectedDev.Alias),
				"",
				true,
			)
			pwd = strings.TrimSpace(pwd)
			fmt.Printf("Ouverture de session vers %s (%s)...\n", selectedDev.Alias, selectedDev.RustDeskID)
			if err := serviceDirectAllowed(selectedDev.RustDeskID); err != nil {
				showErrorLinux("Connexion directe", err.Error())
				continue
			}
			interventionID, err := technicianConnectDevice(token, selectedDev.DeviceID)
			if err == nil {
				err = launchTrackedIntervention(token, linuxServiceTokenFile, rustdeskPath, selectedDev.RustDeskID, interventionID, pwd)
			}
			if err != nil {
				showErrorLinux("Connexion directe", err.Error())
			}

		case "update_device":
			if confirmDialogLinux("Mise à jour disponible", fmt.Sprintf("Programmer la mise à jour automatique à distance pour « %s » (%s) ?", selectedDev.Alias, selectedDev.DeviceID)) {
				resp, err := triggerTechnicianDeviceUpdate(token, selectedDev.DeviceID, APP_VERSION)
				if err != nil {
					showErrorLinux("Erreur mise à jour", fmt.Sprintf("Impossible de programmer la mise à jour : %v", err))
				} else {
					msg := resp.Message
					if msg == "" {
						msg = "Mise à jour programmée avec succès. Le poste distant appliquera la mise à jour en tâche de fond."
					}
					showInfoLinux("Mise à jour disponible", msg)
				}
			}

		case "edit":
			newAlias, ok := promptEntryLinux("Déplacer / Renommer le poste", "Nom du poste (alias) :", selectedDev.Alias, false)
			if !ok {
				continue
			}
			fOpts := buildFolderOptions(folders, false)
			folderItems := make([][2]string, 0, len(fOpts))
			for _, fo := range fOpts {
				folderItems = append(folderItems, [2]string{fo.id, fo.label})
			}
			targetFolder := selectedDev.FolderID
			if len(folders) > 0 {
				curBreadcrumb := getFolderBreadcrumbPath(selectedDev.FolderID, folders)
				if curBreadcrumb == "" {
					curBreadcrumb = "Racine"
				}
				promptMsg := fmt.Sprintf("Dossier actuel : %s\nSélectionnez le dossier de destination :", curBreadcrumb)
				fChoice, fOk := promptSelectLinux("Dossier de destination", promptMsg, folderItems)
				if fOk {
					targetFolder = fChoice
				}
			}
			if err := updateTechnicianDevice(token, selectedDev.DeviceID, newAlias, selectedDev.Notes, targetFolder); err != nil {
				showErrorLinux("Erreur", fmt.Sprintf("Mise à jour impossible : %v", err))
			} else {
				showInfoLinux("Succès", "Poste mis à jour avec succès.")
			}

		case "delete":
			if confirmDialogLinux("Suppression de poste", fmt.Sprintf("Voulez-vous vraiment supprimer le poste '%s' (%s) ?", selectedDev.Alias, selectedDev.DeviceID)) {
				if err := deleteTechnicianDevice(token, selectedDev.DeviceID); err != nil {
					showErrorLinux("Erreur", fmt.Sprintf("Suppression impossible : %v", err))
				} else {
					showInfoLinux("Succès", "Poste supprimé de votre parc.")
				}
			}
		}
	}
}

func handleEnrollFleetLinux(token string) {
	alias, ok := promptEntryLinux("Enrôlement poste permanent", "Nom du poste distant (ex : PC Accueil, Serveur Compta) :", "", false)
	if !ok || strings.TrimSpace(alias) == "" {
		return
	}
	notes, _ := promptEntryLinux("Enrôlement poste permanent", "Emplacement / Notes (optionnel) :", "", false)

	resp, err := generateTechnicianPermanentCode(token, alias, notes)
	if err != nil {
		showErrorLinux("Erreur", fmt.Sprintf("Impossible de générer le code d'enrôlement : %v", err))
		return
	}

	copyToClipboardLinux(resp.PermanentCode)

	msg := fmt.Sprintf("Code d'enrôlement permanent créé :\n\n  %s\n\nValable 15 minutes pour un poste unique.\n(Copié dans le presse-papiers)\n\nSur le poste client Linux :\nsudo relaisdesk-viewer --enroll %s\n\nSur le poste client Windows :\nLancer RelaisDesk Viewer en tant qu'administrateur et saisir ce code.", resp.PermanentCode, resp.PermanentCode)
	showInfoLinux("Enrôlement permanent", msg)
}

func handleFleetDirectConnect(token, rustdeskPath, targetID string) {
	target, err := getFleetConnectionTarget(token, targetID)
	if err != nil {
		showErrorLinux("Connexion directe", fmt.Sprintf("Cible de parc introuvable : %v", err))
		return
	}
	pwd, _ := promptEntryLinux(
		"Connexion au poste permanent",
		fmt.Sprintf("Ouvrir une connexion vers %s (%s) ?\nMot de passe permanent (optionnel) :", target.Alias, target.RustDeskID),
		"",
		true,
	)
	pwd = strings.TrimSpace(pwd)
	if err := serviceDirectAllowed(target.RustDeskID); err != nil {
		showErrorLinux("Connexion directe", err.Error())
		return
	}
	interventionID, err := technicianConnectDevice(token, target.DeviceID)
	if err == nil {
		err = launchTrackedIntervention(token, linuxServiceTokenFile, rustdeskPath, target.RustDeskID, interventionID, pwd)
	}
	if err != nil {
		showErrorLinux("Connexion directe", err.Error())
	}
}

func handleCheckUpdateLinux() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := checkForUpdate(ctx, APIURL)
	if err != nil {
		showErrorLinux("Mises à jour", fmt.Sprintf("Vérification impossible : %v", err))
		return
	}

	if info.Available {
		msg := fmt.Sprintf("Une nouvelle version est disponible !\n\nVersion actuelle : %s\nDernière version : %s\n\nTéléchargement vérifié :\n%s", info.CurrentVersion, info.LatestVersion, info.DownloadURL)
		showInfoLinux("Mise à jour disponible", msg)
	} else {
		msg := fmt.Sprintf("Vous êtes à jour (version %s)", info.CurrentVersion)
		showInfoLinux("Mises à jour", msg)
	}
}

// ---------------------------------------------------------------------------
// Helpers UI (Zenity / Kdialog / Console fallback)
// ---------------------------------------------------------------------------

func isDisplayAvailable() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func showInfoLinux(title, message string) {
	if !isDisplayAvailable() {
		fmt.Printf("\n[%s]\n%s\n", title, message)
		return
	}
	if zenityPath, err := exec.LookPath("zenity"); err == nil {
		_ = exec.Command(zenityPath, "--info", "--title="+title, "--text="+message, "--width=450").Run()
		return
	}
	if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
		_ = exec.Command(kdialogPath, "--msgbox", message, "--title", title).Run()
		return
	}
	fmt.Printf("\n[%s]\n%s\n", title, message)
}

func showErrorLinux(title, message string) {
	if !isDisplayAvailable() {
		fmt.Fprintf(os.Stderr, "\n[%s - ERREUR]\n%s\n", title, message)
		return
	}
	if zenityPath, err := exec.LookPath("zenity"); err == nil {
		_ = exec.Command(zenityPath, "--error", "--title="+title, "--text="+message, "--width=450").Run()
		return
	}
	if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
		_ = exec.Command(kdialogPath, "--error", message, "--title", title).Run()
		return
	}
	fmt.Fprintf(os.Stderr, "\n[%s - ERREUR]\n%s\n", title, message)
}

func confirmDialogLinux(title, message string) bool {
	if !isDisplayAvailable() {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("\n%s (o/N) : ", message)
		ans, _ := reader.ReadString('\n')
		ans = strings.TrimSpace(strings.ToLower(ans))
		return ans == "o" || ans == "oui" || ans == "y" || ans == "yes"
	}
	if zenityPath, err := exec.LookPath("zenity"); err == nil {
		err := exec.Command(zenityPath, "--question", "--title="+title, "--text="+message, "--width=400").Run()
		return err == nil
	}
	if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
		err := exec.Command(kdialogPath, "--yesno", message, "--title", title).Run()
		return err == nil
	}
	return false
}

func promptEntryLinux(title, prompt, defaultValue string, isPassword bool) (string, bool) {
	if isDisplayAvailable() {
		if zenityPath, err := exec.LookPath("zenity"); err == nil {
			args := []string{"--entry", "--title=" + title, "--text=" + prompt, "--width=400"}
			if isPassword {
				args = []string{"--password", "--title=" + title}
			}
			if defaultValue != "" && !isPassword {
				args = append(args, "--entry-text="+defaultValue)
			}
			cmd := exec.Command(zenityPath, args...)
			out, err := cmd.Output()
			if err != nil {
				return "", false
			}
			return strings.TrimSpace(string(out)), true
		}
		if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
			flag := "--inputbox"
			if isPassword {
				flag = "--password"
			}
			args := []string{flag, prompt}
			if defaultValue != "" && !isPassword {
				args = append(args, defaultValue)
			}
			args = append(args, "--title", title)
			cmd := exec.Command(kdialogPath, args...)
			out, err := cmd.Output()
			if err != nil {
				return "", false
			}
			return strings.TrimSpace(string(out)), true
		}
	}

	// Console fallback
	reader := bufio.NewReader(os.Stdin)
	if defaultValue != "" {
		fmt.Printf("%s [%s] : ", prompt, defaultValue)
	} else {
		fmt.Printf("%s : ", prompt)
	}
	val, err := reader.ReadString('\n')
	if err != nil {
		return "", false
	}
	val = strings.TrimSpace(val)
	if val == "" && defaultValue != "" {
		val = defaultValue
	}
	return val, true
}

func promptSelectLinux(title, prompt string, items [][2]string) (string, bool) {
	if len(items) == 0 {
		return "", false
	}

	if isDisplayAvailable() {
		if zenityPath, err := exec.LookPath("zenity"); err == nil {
			args := []string{
				"--list",
				"--title=" + title,
				"--text=" + prompt,
				"--column=Code",
				"--column=Action",
				"--hide-column=1",
				"--width=520",
				"--height=420",
			}
			for _, item := range items {
				args = append(args, item[0], item[1])
			}
			cmd := exec.Command(zenityPath, args...)
			out, err := cmd.Output()
			if err != nil {
				return "", false
			}
			return strings.TrimSpace(string(out)), true
		}

		if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
			args := []string{"--menu", prompt}
			for _, item := range items {
				args = append(args, item[0], item[1])
			}
			args = append(args, "--title", title)
			cmd := exec.Command(kdialogPath, args...)
			out, err := cmd.Output()
			if err != nil {
				return "", false
			}
			return strings.TrimSpace(string(out)), true
		}
	}

	// Console fallback
	fmt.Printf("\n--- %s ---\n%s\n", title, prompt)
	for i, item := range items {
		fmt.Printf("  %d) %s\n", i+1, item[1])
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Votre choix [1-%d] (ou 'q' pour quitter) : ", len(items))
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", false
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "q" || input == "quit" || input == "exit" {
			return "", false
		}
		num, err := strconv.Atoi(input)
		if err == nil && num >= 1 && num <= len(items) {
			return items[num-1][0], true
		}
		fmt.Println("Choix invalide. Réessayez.")
	}
}

func copyToClipboardLinux(text string) {
	if wlCopy, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.Command(wlCopy)
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
		return
	}
	if xclip, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(xclip, "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
		return
	}
	if xsel, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command(xsel, "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
		return
	}
}

func notifyDesktopLinux(title, message string) {
	if !isDisplayAvailable() {
		return
	}
	if notifyPath, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command(notifyPath, "-a", "RelaisDesk", title, message).Run()
	}
}

func openBrowserLinux(urlStr string) {
	if xdgOpen, err := exec.LookPath("xdg-open"); err == nil {
		_ = exec.Command(xdgOpen, urlStr).Start()
		return
	}
	fmt.Printf("Ouvrez le lien suivant dans votre navigateur : %s\n", urlStr)
}
