//go:build !linux

package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"strings"
	"time"
)

var sessionServicesEnabled bool
var serviceWindows = map[fyne.Window]bool{}

func closeServiceBillingUI() {
	if fyneApp == nil {
		return
	}
	fyne.Do(func() {
		for win := range serviceWindows {
			win.Close()
		}
	})
}

func openServiceDialog(kind, target string) {
	token := sessionToken
	auth := sessionNetworkAuthorization
	go func() {
		catalog, err := serviceCatalog(token)
		binary := ""
		if err == nil {
			binary, err = ensureRustDesk()
		}
		if err == nil {
			err = checkServiceEngine(binary)
		}
		fyne.Do(func() {
			if token != sessionToken || auth != sessionNetworkAuthorization {
				return
			}
			if err != nil {
				dialog.ShowError(err, mainWindow)
				return
			}
			if auth == nil {
				dialog.ShowInformation("Prestations", "Reconnectez-vous avant de démarrer une prestation.", mainWindow)
				return
			}
			win := fyneApp.NewWindow("Connexion & Prestation")
			serviceWindows[win] = true
			win.Resize(fyne.NewSize(620, 510))
			var work *ServiceWork
			var bridge *serviceBridge
			busy := false
			closed := false
			summary := widget.NewLabel("Choisissez le tarif convenu avec le client. Aucune saisie bancaire dans RelaisDesk.")
			summary.Wrapping = fyne.TextWrapWord
			message := widget.NewLabel("")
			message.Wrapping = fyne.TextWrapWord
			labels := []string{}
			for _, r := range catalog.Rates {
				unit := "€ — forfait prépayé"
				if r.Mode == "hourly" {
					unit = "€/h — paiement après connexion"
				}
				labels = append(labels, fmt.Sprintf("%s — %.2f %s", r.Label, float64(r.Cents)/100, unit))
			}
			rateSelect := widget.NewSelect(labels, nil)
			agreed := widget.NewCheck("Le client a accepté ce tarif et son mode de calcul", nil)
			password := widget.NewPasswordEntry()
			password.SetPlaceHolder("Mot de passe du poste (facultatif)")
			link := widget.NewEntry()
			link.Disable()
			var controls []*widget.Button
			refresh := func() {
				if work != nil {
					summary.SetText(fmt.Sprintf("%s — %s\nTemps confirmé : %.2f min • Montant : %.2f €\nÉtat : %s • Payé : %t", work.Label, work.Mode, float64(work.ConnectedMS)/60000, float64(work.AmountCents)/100, work.State, work.Paid))
					link.SetText(work.CheckoutURL)
				}
				for _, b := range controls {
					if busy {
						b.Disable()
					} else {
						b.Enable()
					}
				}
			}
			run := func(task func() (*ServiceWork, error)) {
				if token != sessionToken || auth != sessionNetworkAuthorization {
					message.SetText("Session terminée : fermez cette fenêtre et reconnectez-vous.")
					return
				}
				if busy {
					return
				}
				busy = true
				refresh()
				message.SetText("Traitement en cours…")
				go func() {
					next, e := task()
					fyne.Do(func() {
						if closed || token != sessionToken || auth != sessionNetworkAuthorization {
							return
						}
						busy = false
						if e != nil {
							message.SetText(e.Error())
						} else {
							if next != nil {
								work = next
							}
							message.SetText("")
						}
						refresh()
					})
				}()
			}
			prepare := widget.NewButton("Préparer la prestation", func() {
				if !catalog.Enabled {
					message.SetText("Nouvelles prestations désactivées.")
					return
				}
				if work != nil {
					message.SetText("Terminez cette prestation avant d'en créer une autre.")
					return
				}
				index := rateSelect.SelectedIndex()
				if index < 0 || !agreed.Checked {
					message.SetText("Sélectionnez un tarif et confirmez l'accord du client.")
					return
				}
				rate := catalog.Rates[index]
				run(func() (*ServiceWork, error) {
					return serviceWorkCall(token, "", "", map[string]any{"rate_id": rate.ID, "target_kind": kind, "target_id": target, "client_agreed": true})
				})
			})
			reload := widget.NewButton("Actualiser le paiement / la durée", func() {
				if work != nil {
					id := work.ID
					run(func() (*ServiceWork, error) { return serviceWorkCall(token, id, "", nil) })
				}
			})
			pay := widget.NewButton("Créer le lien Stripe", func() {
				if work != nil {
					id := work.ID
					run(func() (*ServiceWork, error) { return serviceWorkCall(token, id, "checkout", map[string]any{}) })
				}
			})
			copy := widget.NewButton("Copier le lien pour le client", func() {
				if work != nil && validServiceCheckout(work.CheckoutURL) {
					win.Clipboard().SetContent(work.CheckoutURL)
					message.SetText("Lien copié. Le client doit l'ouvrir sur son appareil, sans partage d'écran actif.")
				}
			})
			connect := widget.NewButton("Démarrer / reprendre la connexion", func() {
				if work == nil {
					return
				}
				if bridge != nil {
					message.SetText("La connexion est déjà préparée ; utilisez RustDesk pour vous reconnecter.")
					return
				}
				id := work.ID
				pass := strings.TrimSpace(password.Text)
				run(func() (*ServiceWork, error) {
					fresh, e := serviceWorkCall(token, id, "", nil)
					if e != nil {
						return nil, e
					}
					b, e := startServiceBridge(token, auth.TokenFile, fresh)
					if e != nil {
						return nil, e
					}
					e = launchRustDeskSession(binary, fresh.PeerID, pass)
					if e != nil {
						b.stop()
						return nil, e
					}
					fyne.Do(func() {
						if closed || token != sessionToken || auth != sessionNetworkAuthorization {
							go b.stop()
						} else {
							bridge = b
						}
					})
					return fresh, nil
				})
			})
			finish := widget.NewButton("Fermer la prise en main et terminer", func() {
				if work == nil {
					return
				}
				id := work.ID
				b := bridge
				bridge = nil
				run(func() (*ServiceWork, error) {
					if b != nil {
						b.stop()
						time.Sleep(11 * time.Second)
					}
					return serviceWorkCall(token, id, "finish", map[string]any{})
				})
			})
			cancel := widget.NewButton("Annuler la prestation sans paiement", func() {
				if work == nil || bridge != nil {
					return
				}
				id := work.ID
				run(func() (*ServiceWork, error) { return serviceWorkCall(token, id, "cancel", map[string]any{}) })
			})
			controls = []*widget.Button{prepare, reload, pay, copy, connect, finish, cancel}
			for _, old := range catalog.Work {
				if old.TargetKind == kind && old.TargetID == target && (old.State == "prepared" || old.State == "open" || (old.State == "finished" && !old.Paid && old.AmountCents >= 50)) {
					v := old
					work = &v
					break
				}
			}
			if !catalog.Enabled && work == nil {
				prepare.Disable()
				message.SetText("L'administrateur de votre entreprise doit activer les prestations dans l'espace client.")
			}
			notice := widget.NewLabel("Prépayé : le lien doit être payé avant la connexion.\nHoraire : seule la connexion confirmée compte, au prorata ; fermez toutes les prises en main avant d'envoyer le lien.\nLa facturation et les remboursements de la prestation relèvent de votre entreprise, pas de RelaisDesk.")
			notice.Wrapping = fyne.TextWrapWord
			win.SetContent(container.NewVScroll(container.NewVBox(summary, rateSelect, agreed, prepare, password, connect, reload, finish, cancel, pay, link, copy, message, notice)))
			win.SetOnClosed(func() {
				delete(serviceWindows, win)
				closed = true
				if bridge != nil {
					go bridge.stop()
				}
			})
			refresh()
			win.Show()
		})
	}()
}
