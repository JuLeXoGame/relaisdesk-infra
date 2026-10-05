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

// parseEnrollCLI splits `viewer --enroll` trailing arguments into the
// enrollment code, the optional password source and the silent flag. The
// flag is accepted anywhere among the arguments. A code starting with @ is a
// one-shot handoff file left by the unelevated parent (consumed and deleted
// here); an unreadable handoff yields an empty code.
func parseEnrollCLI(args []string) (code, src string, silent bool) {
	var positional []string
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "--silent", "-silent", "/silent", "--batch", "-batch", "/batch":
			silent = true
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) >= 2 {
		src = strings.TrimSpace(positional[1])
	}
	if len(positional) >= 1 {
		code = strings.TrimSpace(positional[0])
		if strings.HasPrefix(code, "@") {
			fileCode, err := readEnrollPasswordFile(strings.TrimPrefix(code, "@"))
			if err != nil {
				return "", src, silent
			}
			code = strings.TrimSpace(fileCode)
		}
		code = strings.ToUpper(code)
	}
	return code, src, silent
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
			code, src, silent := parseEnrollCLI(os.Args[2:])
			codeFromEnv := false
			if code == "" {
				// Silent deployments (GPO/Intune/SCCM) pass the code via the
				// environment so it never appears on any command line.
				code = strings.ToUpper(strings.TrimSpace(os.Getenv("RELAISDESK_ENROLL_CODE")))
				codeFromEnv = code != ""
			}
			if code == "" {
				if silent {
					log.Fatal("Code d'enrôlement requis en mode silencieux : passez-le en argument ou via RELAISDESK_ENROLL_CODE.")
				}
				fmt.Print("Entrez le code d'enrôlement permanent (ex: PERM-XXXX-XXXX) : ")
				fmt.Scanln(&code)
				code = strings.ToUpper(strings.TrimSpace(code))
			}
			if code == "" {
				log.Fatal("Code d'enrôlement permanent requis.")
			}
			if src != "" && !strings.HasPrefix(src, "@") {
				fmt.Fprintln(os.Stderr, "avertissement : mot de passe en ligne de commande (visible via ps) ; préférez @fichier ou RELAISDESK_ENROLL_PASSWORD")
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
				// Same rule for a code received via the environment: keep it
				// off the elevated child's command line.
				fwdCode := code
				if codeFromEnv {
					path, err := writeEnrollPasswordFile(code)
					if err != nil {
						log.Fatalf("Préparation du code impossible: %v", err)
					}
					fwdCode = "@" + path
				}
				args := []string{os.Args[1]}
				if silent {
					args = append(args, "--silent")
				}
				args = append(args, fwdCode)
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
					if silent {
						log.Fatal("Mot de passe permanent requis en mode silencieux : /PASSWORD= ou RELAISDESK_ENROLL_PASSWORD (ou pré-configurez le mot de passe RustDesk avant le déploiement).")
					}
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
