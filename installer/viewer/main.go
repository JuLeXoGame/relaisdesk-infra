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
// unelevated parent and deletes it immediately after reading.
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

// writeEnrollPasswordFile stores a password in a 0600 temp file for a single
// handoff to the elevated child, which deletes it right after reading.
// (If elevation is cancelled the file lingers in the per-user temp dir until
// cleaned; strictly better than argv, which is visible to all users via ps
// and persisted in shell history and event logs.)
func writeEnrollPasswordFile(password string) (string, error) {
	f, err := os.CreateTemp("", "relaisdesk-enroll-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(password); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
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
			src := ""
			rawArg := false
			if len(os.Args) >= 3 {
				src = strings.TrimSpace(os.Args[2])
				rawArg = src != "" && !strings.HasPrefix(src, "@")
			}
			if src == "" {
				src = strings.TrimSpace(os.Getenv("RELAISDESK_ENROLL_PASSWORD"))
			}
			if rawArg {
				fmt.Fprintln(os.Stderr, "avertissement : mot de passe en ligne de commande (visible via ps) ; préférez @fichier ou RELAISDESK_ENROLL_PASSWORD")
			}
			if !isElevated() {
				// Never forward the raw secret on argv: materialize it into
				// a one-shot file. An @file source is forwarded unread: the
				// elevated child consumes and deletes it.
				fwd := src
				if !strings.HasPrefix(fwd, "@") {
					if fwd == "" {
						log.Fatal("Mot de passe requis : viewer.exe --set-permanent-password @fichier (ou RELAISDESK_ENROLL_PASSWORD)")
					}
					path, err := writeEnrollPasswordFile(fwd)
					if err != nil {
						log.Fatalf("Préparation du mot de passe impossible: %v", err)
					}
					fwd = "@" + path
				}
				fmt.Println("Droits administrateur requis. Demande d'élévation...")
				if err := relaunchElevated([]string{os.Args[1], fwd}); err != nil {
					log.Fatalf("Échec de l'élévation: %v", err)
				}
				return
			}
			pwd := src
			if strings.HasPrefix(pwd, "@") {
				filePassword, err := readEnrollPasswordFile(strings.TrimPrefix(pwd, "@"))
				if err != nil {
					log.Fatalf("Fichier de mot de passe illisible: %v", err)
				}
				pwd = strings.TrimSpace(filePassword)
			}
			if pwd == "" {
				log.Fatal("Mot de passe requis : viewer.exe --set-permanent-password @fichier (ou RELAISDESK_ENROLL_PASSWORD)")
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
		if arg == "--set-hwcodec" {
			if len(os.Args) < 3 {
				log.Fatal("Usage : viewer --set-hwcodec on|off")
			}
			var enable bool
			switch strings.ToLower(strings.TrimSpace(os.Args[2])) {
			case "on", "1", "true", "yes", "y", "enable", "enabled", "auto":
				enable = true
			case "off", "0", "false", "no", "n", "disable", "disabled", "software", "vp9":
				enable = false
			default:
				log.Fatal("Valeur invalide (on|off) : " + os.Args[2])
			}
			n, err := setHwCodecEverywhere(enable)
			if err != nil {
				log.Fatal(err)
			}
			mode := "auto (matériel si disponible)"
			if !enable {
				mode = "logiciel VP9 forcé"
			}
			fmt.Printf("Codec vidéo : %s (%d fichier(s) mis à jour). Redémarrez RustDesk.\n", mode, n)
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
			if len(os.Args) >= 3 {
				code = os.Args[2]
			} else {
				fmt.Print("Entrez le code d'enrôlement permanent (ex: PERM-XXXX-XXXX) : ")
				fmt.Scanln(&code)
			}
			code = strings.ToUpper(strings.TrimSpace(code))
			if code == "" {
				log.Fatal("Code d'enrôlement permanent requis.")
			}
			src := ""
			if len(os.Args) >= 4 {
				src = strings.TrimSpace(os.Args[3])
				if src != "" && !strings.HasPrefix(src, "@") {
					fmt.Fprintln(os.Stderr, "avertissement : mot de passe en ligne de commande (visible via ps) ; préférez @fichier ou RELAISDESK_ENROLL_PASSWORD")
				}
			}
			if src == "" {
				src = strings.TrimSpace(os.Getenv("RELAISDESK_ENROLL_PASSWORD"))
			}
			if !isElevated() {
				fmt.Println("Droits administrateur requis pour l'accès permanent. Demande d'élévation...")
				fwd := src
				if fwd != "" && !strings.HasPrefix(fwd, "@") {
					// Never forward the raw secret on argv: materialize it.
					// @file sources are forwarded unread (one-shot).
					path, err := writeEnrollPasswordFile(fwd)
					if err != nil {
						log.Fatalf("Préparation du mot de passe impossible: %v", err)
					}
					fwd = "@" + path
				}
				args := []string{os.Args[1], code}
				if fwd != "" {
					args = append(args, fwd)
				}
				if err := relaunchElevated(args); err != nil {
					log.Fatalf("Échec de l'élévation: %v", err)
				}
				return
			}
			password := src
			if strings.HasPrefix(password, "@") {
				filePassword, err := readEnrollPasswordFile(strings.TrimPrefix(password, "@"))
				if err != nil {
					log.Fatalf("Fichier de mot de passe illisible: %v", err)
				}
				password = strings.TrimSpace(filePassword)
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
