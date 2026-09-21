//go:build !linux

package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// requestFleetConnection initiates remote control on a permanent device.
// If the device is currently offline / turned off, a Wake-on-LAN signal
// (both local broadcast and cloud relay via online peers) is automatically
// and silently dispatched.
func requestFleetConnection(id string, devOpt ...DeviceItem) {
	token := sessionToken
	authorization := sessionNetworkAuthorization
	go func() {
		target, err := getFleetConnectionTarget(token, id)
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, mainWindow)
				return
			}

			// Détection du statut éteint / hors ligne
			isOffline := target.Status != "online"
			if len(devOpt) > 0 && devOpt[0].Status != "online" {
				isOffline = true
			}

			var wolNotice fyne.CanvasObject
			if isOffline {
				targetMAC := target.MACAddress
				if targetMAC == "" && len(devOpt) > 0 {
					targetMAC = devOpt[0].MACAddress
				}

				// Déclenchement automatique et transparent du réveil WoL (Local + Cloud Relay)
				go func(targetID, mac string) {
					if mac != "" {
						_, _ = SendWakeOnLAN(mac)
					}
					if token != "" {
						_, _ = wakeTechnicianDevice(token, targetID)
					}
				}(target.DeviceID, targetMAC)

				msgLabel := widget.NewLabel("⚡ Poste éteint / hors ligne : signal de réveil (Wake-on-LAN) envoyé automatiquement (en local et par les postes relais en ligne du site).\nLe démarrage de la machine peut prendre 1 à 2 minutes.")
				msgLabel.Wrapping = fyne.TextWrapWord
				msgLabel.TextStyle = fyne.TextStyle{Italic: true}
				wolNotice = container.NewVBox(
					msgLabel,
					widget.NewSeparator(),
				)
			}

			pwdEntry := widget.NewPasswordEntry()
			pwdEntry.SetPlaceHolder("Optionnel si mémorisé par RustDesk")

			promptLabel := widget.NewLabel(fmt.Sprintf("Ouvrir une connexion vers %s (%s) ?", target.Alias, target.RustDeskID))
			noteLabel := widget.NewLabel("Pour un accès direct sans confirmation du client, saisissez le mot de passe permanent du poste.")
			noteLabel.Wrapping = fyne.TextWrapWord

			formElements := []fyne.CanvasObject{
				promptLabel,
			}
			if wolNotice != nil {
				formElements = append(formElements, wolNotice)
			}
			formElements = append(formElements,
				widget.NewLabel("Mot de passe permanent :"),
				pwdEntry,
				noteLabel,
			)

			formContent := container.NewVBox(formElements...)

			d := dialog.NewCustomConfirm(
				"Connexion au poste permanent",
				"Se connecter",
				"Annuler",
				formContent,
				func(ok bool) {
					if !ok {
						return
					}
					pass := strings.TrimSpace(pwdEntry.Text)
					go func() {
						fresh, err := getFleetConnectionTarget(token, id)
						if err == nil {
							err = serviceDirectAllowed(fresh.RustDeskID)
						}
						if err == nil && authorization == nil {
							err = fmt.Errorf("autorisation réseau absente ; reconnectez-vous")
						}
						if err == nil {
							var binary string
							binary, err = ensureRustDesk()
							if err == nil {
								var interventionID string
								interventionID, err = technicianConnectDevice(token, fresh.DeviceID)
								if err == nil {
									err = launchTrackedIntervention(token, authorization.TokenFile, binary, fresh.RustDeskID, interventionID, pass)
								}
							}
						}
						if err != nil {
							fyne.Do(func() { dialog.ShowError(err, mainWindow) })
						}
					}()
				},
				mainWindow,
			)
			d.Resize(fyne.NewSize(450, 260))
			d.Show()
		})
	}()
}
