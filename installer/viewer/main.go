package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// readEnrollPasswordFile reads a one-shot password handoff file created by the
// unelevated GUI and deletes it immediately after reading.
func readEnrollPasswordFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	os.Remove(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 || len(data) > 4096 {
		return "", errors.New("fichier de mot de passe invalide")
	}
	return string(data), nil
}

func main() {
	if len(os.Args) >= 2 {
		arg := strings.ToLower(os.Args[1])
		if arg == "--fleet-check" {
			if err := fleetCheckAuthorization(); err != nil {
				log.Fatal(err)
			}
			return
		}
		if arg == "--fleet-service" {
			if err := runFleetService(); err != nil {
				log.Fatal(err)
			}
			return
		}
		if arg == "--set-permanent-password" {
			if len(os.Args) < 3 || strings.TrimSpace(os.Args[2]) == "" {
				log.Fatal("Mot de passe requis : viewer.exe --set-permanent-password <mot_de_passe>")
			}
			pwd := strings.TrimSpace(os.Args[2])
			if !isElevated() {
				fmt.Println("Droits administrateur requis. Demande d'élévation...")
				if err := relaunchElevated(os.Args[1:]); err != nil {
					log.Fatalf("Échec de l'élévation: %v", err)
				}
				return
			}
			if err := setFleetPermanentPassword(pwd); err != nil {
				log.Fatalf("Échec de la configuration du mot de passe permanent: %v", err)
			}
			fmt.Println("Mot de passe permanent configuré avec succès dans le service RustDesk.")
			return
		}
		if arg == "--unenroll" {
			if err := uninstallFleet(); err != nil {
				log.Fatal(err)
			}
			fmt.Println("Autorisation permanente locale et services retirés. Supprimez également la fiche dans votre console.")
			return
		}
		if arg == "--gui-enroll" {
			preset := ""
			if len(os.Args) >= 3 {
				preset = os.Args[2]
			}
			if err := RunGUIWithPreset(preset, true); err != nil {
				log.Fatalf("Erreur fatale: %v", err)
			}
			return
		}
		if strings.HasPrefix(strings.ToUpper(os.Args[1]), "PERM-") {
			if err := RunGUIWithPreset(strings.ToUpper(os.Args[1]), true); err != nil {
				log.Fatalf("Erreur fatale: %v", err)
			}
			return
		}
		if arg == "--enroll" || arg == "-enroll" || arg == "/enroll" {
			code := ""
			password := ""
			if len(os.Args) >= 3 {
				code = os.Args[2]
			} else {
				fmt.Print("Entrez le code d'enrôlement permanent (ex: PERM-XXXX-XXXX) : ")
				fmt.Scanln(&code)
			}
			if len(os.Args) >= 4 {
				password = strings.TrimSpace(os.Args[3])
			}
			if password == "" {
				password = strings.TrimSpace(os.Getenv("RELAISDESK_ENROLL_PASSWORD"))
			}
			if strings.HasPrefix(password, "@") {
				filePassword, err := readEnrollPasswordFile(strings.TrimPrefix(password, "@"))
				if err != nil {
					log.Fatalf("Fichier de mot de passe illisible: %v", err)
				}
				password = strings.TrimSpace(filePassword)
			}
			code = strings.ToUpper(strings.TrimSpace(code))
			if code == "" {
				log.Fatal("Code d'enrôlement permanent requis.")
			}
			if !isElevated() {
				fmt.Println("Droits administrateur requis pour l'accès permanent. Demande d'élévation...")
				if err := relaunchElevated(os.Args[1:]); err != nil {
					log.Fatalf("Échec de l'élévation: %v", err)
				}
				return
			}
			fmt.Println("Vérification et installation du service système RustDesk...")
			if _, err := ensureRustDeskServiceInstalled(); err != nil {
				log.Fatalf("Échec de l'installation du service: %v", err)
			}
			if password != "" {
				fmt.Println("Configuration du mot de passe permanent...")
				if err := setFleetPermanentPassword(password); err != nil {
					log.Fatalf("Échec de la configuration du mot de passe permanent: %v", err)
				}
				fmt.Println("Mot de passe permanent configuré avec succès !")
			} else {
				hasPwd, _ := hasFleetPermanentPassword()
				if !hasPwd {
					fmt.Println("Veuillez configurer un mot de passe permanent dans RustDesk (⚙️ Paramètres > Sécurité).")
					_ = openRustDeskSettings()
					fmt.Print("En attente de la configuration du mot de passe...")
					for {
						time.Sleep(2 * time.Second)
						hasPwd, _ = hasFleetPermanentPassword()
						if hasPwd {
							fmt.Println(" Mot de passe configuré !")
							break
						}
						fmt.Print(".")
					}
				}
			}
			fmt.Println("Installation de l'autorisation permanente...")
			resp, err := installFleet(code)
			if isFleetReplaceableError(err) {
				fmt.Println("Remplacement de l'accès permanent existant...")
				_ = uninstallFleet()
				resp, err = installFleet(code)
			}
			if err != nil {
				log.Fatalf("Échec de l'enrôlement: %v", err)
			}
			fmt.Printf("Service d'autorisation installé pour le poste %s (Alias: %s). Vérifiez sa connexion dans la console.\n", resp.DeviceID, resp.Alias)
			fmt.Printf("Serveur relais: %s, Ports: %d/%d\n", resp.ServerIP, resp.RendezvousPort, resp.RelayPort)
			return
		}
	}

	if err := RunGUI(); err != nil {
		log.Fatalf("Erreur fatale: %v", err)
	}
}
