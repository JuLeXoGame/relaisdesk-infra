//go:build linux

package main

import (
	"fmt"
	"strings"
	"time"
)

var linuxServiceTokenFile string

func handleServiceCodesLinux(token, tokenFile, binary string) {
	dash, err := getDashboard(token)
	if err != nil {
		showErrorLinux("Assistance", err.Error())
		return
	}
	choices := [][2]string{}
	for _, c := range dash.Codes {
		if c.IsActive && numericFleetTarget(c.ClientRustDeskID) {
			choices = append(choices, [2]string{c.Code, c.Code + " — " + c.ClientEmail})
		}
	}
	if len(choices) == 0 {
		showInfoLinux("Assistance", "Aucun client prêt pour la connexion.")
		return
	}
	code, ok := promptSelectLinux("Assistance", "Sélectionnez le client", choices)
	if !ok {
		return
	}
	actions := [][2]string{{"direct", "Connexion directe"}}
	if catalog, e := serviceCatalog(token); e == nil && (catalog.Enabled || len(catalog.Work) > 0) {
		actions = append(actions, [2]string{"service", "Connexion & Prestation"})
	}
	action, ok := promptSelectLinux("Assistance", "Choisissez le parcours", actions)
	if !ok {
		return
	}
	if action == "service" {
		openServiceLinux(token, tokenFile, binary, "code", code)
		return
	}
	for _, c := range dash.Codes {
		if c.Code == code {
			if err = serviceDirectAllowed(c.ClientRustDeskID); err == nil {
				var interventionID string
				interventionID, err = technicianConnectViewerCode(token, c.Code)
				if err == nil {
					err = launchTrackedIntervention(token, tokenFile, binary, c.ClientRustDeskID, interventionID)
				}
			}
			if err != nil {
				showErrorLinux("Connexion", err.Error())
			}
			return
		}
	}
}

func openServiceLinux(token, tokenFile, binary, kind, target string) {
	if err := checkServiceEngine(binary); err != nil {
		showErrorLinux("Prestation", err.Error())
		return
	}
	catalog, err := serviceCatalog(token)
	if err != nil {
		showErrorLinux("Prestation", err.Error())
		return
	}
	var work *ServiceWork
	for _, old := range catalog.Work {
		if old.TargetKind == kind && old.TargetID == target && (old.State == "open" || old.State == "prepared" || (old.State == "finished" && !old.Paid && old.AmountCents >= 50)) {
			v := old
			work = &v
			break
		}
	}
	if work == nil {
		if !catalog.Enabled {
			showErrorLinux("Prestation", "Option désactivée par l'entreprise ou le serveur.")
			return
		}
		options := [][2]string{}
		for _, r := range catalog.Rates {
			options = append(options, [2]string{r.ID, fmt.Sprintf("%s — %.2f € (%s)", r.Label, float64(r.Cents)/100, r.Mode)})
		}
		rate, ok := promptSelectLinux("Connexion & Prestation", "Sélectionnez le tarif accepté par le client (horaire : temps connecté au prorata).", options)
		if !ok {
			return
		}
		if !confirmDialogLinux("Accord du client", "Le client a-t-il accepté le tarif et le mode de calcul ?") {
			return
		}
		work, err = serviceWorkCall(token, "", "", map[string]any{"rate_id": rate, "target_kind": kind, "target_id": target, "client_agreed": true})
		if err != nil {
			showErrorLinux("Prestation", err.Error())
			return
		}
	}
	var bridge *serviceBridge
	defer func() {
		if bridge != nil {
			bridge.stop()
		}
	}()
	for {
		action, ok := promptSelectLinux("Connexion & Prestation", fmt.Sprintf("%s\nTemps confirmé : %.2f min • Montant : %.2f €\nÉtat : %s • Payé : %t", work.Label, float64(work.ConnectedMS)/60000, float64(work.AmountCents)/100, work.State, work.Paid), [][2]string{{"connect", "Démarrer / reprendre la connexion"}, {"refresh", "Actualiser"}, {"finish", "Fermer la prise en main et terminer"}, {"checkout", "Créer / copier le lien Stripe"}, {"exit", "Fermer"}})
		if !ok || action == "exit" {
			return
		}
		switch action {
		case "connect":
			if bridge != nil {
				showInfoLinux("Prestation", "Utilisez la fenêtre RustDesk existante pour vous reconnecter.")
				continue
			}
			var fresh *ServiceWork
			fresh, err = serviceWorkCall(token, work.ID, "", nil)
			if err == nil {
				bridge, err = startServiceBridge(token, tokenFile, fresh)
			}
			if err == nil {
				pwd, _ := promptEntryLinux("Connexion", "Mot de passe du poste (facultatif)", "", true)
				err = launchRustDeskSession(binary, fresh.PeerID, strings.TrimSpace(pwd))
				if err != nil {
					bridge.stop()
					bridge = nil
				}
			}
		case "finish":
			if bridge != nil {
				bridge.stop()
				bridge = nil
				time.Sleep(11 * time.Second)
			}
			work, err = serviceWorkCall(token, work.ID, "finish", map[string]any{})
		case "checkout":
			var next *ServiceWork
			next, err = serviceWorkCall(token, work.ID, "checkout", map[string]any{})
			if err == nil {
				work = next
				if validServiceCheckout(work.CheckoutURL) {
					copyToClipboardLinux(work.CheckoutURL)
					showInfoLinux("Paiement externe", "Lien Stripe copié. Transmettez-le au client après fermeture de toutes les prises en main, ou pour paiement sur son téléphone.")
				}
			}
		default:
			var next *ServiceWork
			next, err = serviceWorkCall(token, work.ID, "", nil)
			if err == nil {
				work = next
			}
		}
		if err != nil {
			showErrorLinux("Prestation", err.Error())
			return
		}
	}
}
