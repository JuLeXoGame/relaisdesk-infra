package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

type Lang string

const (
	LangFR Lang = "fr"
	LangEN Lang = "en"
)

var (
	currentLang   = detectDefaultLang()
	currentLangMu sync.RWMutex
)

func detectDefaultLang() Lang {
	langEnv := strings.ToLower(os.Getenv("LANG"))
	if strings.HasPrefix(langEnv, "en") {
		return LangEN
	}
	return LangFR
}

func GetLang() Lang {
	currentLangMu.RLock()
	defer currentLangMu.RUnlock()
	return currentLang
}

func SetLang(l Lang) {
	currentLangMu.Lock()
	defer currentLangMu.Unlock()
	if l == LangEN {
		currentLang = LangEN
	} else {
		currentLang = LangFR
	}
}

func ToggleLang() Lang {
	currentLangMu.Lock()
	defer currentLangMu.Unlock()
	if currentLang == LangFR {
		currentLang = LangEN
	} else {
		currentLang = LangFR
	}
	return currentLang
}

var messages = map[string]map[Lang]string{
	"app_title": {
		LangFR: PRODUCT_NAME + " — Espace Technicien",
		LangEN: PRODUCT_NAME + " — Technician Workspace",
	},
	"login_subtitle": {
		LangFR: "Connectez-vous avec vos identifiants pour gérer vos accès d'assistance",
		LangEN: "Sign in with your credentials to manage remote support access",
	},
	"email_label": {
		LangFR: "Adresse e-mail :",
		LangEN: "Email Address:",
	},
	"email_placeholder": {
		LangFR: "votre.email@domaine.fr",
		LangEN: "your.email@domain.com",
	},
	"password_label": {
		LangFR: "Mot de passe :",
		LangEN: "Password:",
	},
	"password_placeholder": {
		LangFR: "Mot de passe de votre compte",
		LangEN: "Account password",
	},
	"forgot_password_btn": {
		LangFR: "Mot de passe oublié ?",
		LangEN: "Forgot password?",
	},
	"license_id_placeholder": {
		LangFR: "ID Licence (ex: MP-XXXX-XXXX-XXXX)",
		LangEN: "License ID (e.g.: MP-XXXX-XXXX-XXXX)",
	},
	"license_key_placeholder": {
		LangFR: "Clé secrète de Licence",
		LangEN: "Secret License Key",
	},
	"license_id_label": {
		LangFR: "Identifiant Licence :",
		LangEN: "License ID:",
	},
	"license_key_label": {
		LangFR: "Clé de Licence :",
		LangEN: "License Key:",
	},
	"login_btn": {
		LangFR: "🔐 Se Connecter",
		LangEN: "🔐 Sign In",
	},
	"google_login_btn": {
		LangFR: "🌐 Se connecter avec Google",
		LangEN: "🌐 Sign in with Google",
	},
	"or_separator": {
		LangFR: "— OU —",
		LangEN: "— OR —",
	},
	"google_waiting": {
		LangFR: "Connexion Google en cours dans le navigateur...",
		LangEN: "Completing Google sign-in in browser...",
	},
	"login_err_empty": {
		LangFR: "Veuillez renseigner votre adresse e-mail et votre mot de passe.",
		LangEN: "Please enter your email address and password.",
	},
	"login_checking": {
		LangFR: "Vérification auprès du serveur...",
		LangEN: "Verifying with server...",
	},
	"connecting": {
		LangFR: "Connexion en cours...",
		LangEN: "Connecting...",
	},
	"loading_dashboard": {
		LangFR: "Chargement du tableau de bord...",
		LangEN: "Loading dashboard...",
	},
	"setup_rustdesk": {
		LangFR: "Configuration de RustDesk...",
		LangEN: "Configuring RustDesk...",
	},
	"setup_init": {
		LangFR: "Initialisation...",
		LangEN: "Initializing...",
	},
	"setup_prep": {
		LangFR: "Préparation de RustDesk...",
		LangEN: "Preparing RustDesk...",
	},
	"setup_auth": {
		LangFR: "Autorisation sécurisée de l’appareil...",
		LangEN: "Securing device authorization...",
	},
	"setup_servers": {
		LangFR: "Configuration des serveurs RustDesk...",
		LangEN: "Configuring RustDesk servers...",
	},
	"setup_shortcut": {
		LangFR: "Création du raccourci bureau...",
		LangEN: "Creating desktop shortcut...",
	},
	"setup_launch": {
		LangFR: "Lancement de RustDesk...",
		LangEN: "Launching RustDesk...",
	},
	"tech_header": {
		LangFR: "Technicien : %s   •   Licence valide jusqu'au : %s",
		LangEN: "Technician: %s   •   License valid until: %s",
	},
	"stats_summary": {
		LangFR: "📊 Total codes : %d   |   🟢 Actifs : %d   |   ⏳ Expirés/Révoqués : %d",
		LangEN: "📊 Total codes: %d   |   🟢 Active: %d   |   ⏳ Expired/Revoked: %d",
	},
	"client_email_label": {
		LangFR: "E-mail ou Nom du client assisté (Recommandé) :",
		LangEN: "Client name or email (Recommended):",
	},
	"client_email_placeholder": {
		LangFR: "Ex: client@entreprise.fr ou M. Dupont (laisser vide si anonyme)",
		LangEN: "e.g.: client@company.com or John Doe (leave empty if anonymous)",
	},
	"generate_code_btn": {
		LangFR: "✨ Générer un nouveau code d'accès client",
		LangEN: "✨ Generate new client access code",
	},
	"gen_card_title": {
		LangFR: "Générateur de Session Client",
		LangEN: "Client Session Generator",
	},
	"recent_codes_label": {
		LangFR: "💻 Postes Clients & Ordinateurs distants :",
		LangEN: "💻 Client Computers & Remote PCs:",
	},
	"no_codes_yet": {
		LangFR: "Aucun code d'accès généré pour le moment.",
		LangEN: "No access codes generated yet.",
	},
	"launch_rustdesk_btn": {
		LangFR: "🚀 Lancer RustDesk",
		LangEN: "🚀 Launch RustDesk",
	},
	"refresh_btn": {
		LangFR: "🔄 Actualiser",
		LangEN: "🔄 Refresh",
	},
	"diagnostic_btn": {
		LangFR: "🩺 Diagnostic de connexion",
		LangEN: "🩺 Connection Diagnostics",
	},
	"updates_btn": {
		LangFR: "🔐 Mises à jour",
		LangEN: "🔐 Check Updates",
	},
	"logout_btn": {
		LangFR: "🚪 Se déconnecter",
		LangEN: "🚪 Sign Out",
	},
	"status_active": {
		LangFR: "Actif",
		LangEN: "Active",
	},
	"status_revoked": {
		LangFR: "Révoqué",
		LangEN: "Revoked",
	},
	"status_expired": {
		LangFR: "Expiré",
		LangEN: "Expired",
	},
	"anonymous_client": {
		LangFR: "Anonyme / Sans email",
		LangEN: "Anonymous / No email",
	},
	"client_ready": {
		LangFR: "🟢 Client prêt — ID : %s",
		LangEN: "🟢 Client ready — ID: %s",
	},
	"client_waiting": {
		LangFR: "⏳ En attente de connexion du client...",
		LangEN: "⏳ Waiting for client connection...",
	},
	"take_control_btn": {
		LangFR: "🚀 Prendre la main",
		LangEN: "🚀 Take Control",
	},
	"session_launching": {
		LangFR: "Connexion au poste distant %s en cours...",
		LangEN: "Connecting to remote PC %s...",
	},
	"connect_btn": {
		LangFR: "📞 Connecter",
		LangEN: "📞 Connect",
	},
	"copy_btn": {
		LangFR: "📋 Copier code",
		LangEN: "📋 Copy code",
	},
	"copy_id_btn": {
		LangFR: "📋 Copier l'ID",
		LangEN: "📋 Copy ID",
	},
	"id_copied": {
		LangFR: "ID client %s copié dans le presse-papiers !",
		LangEN: "Client ID %s copied to clipboard!",
	},
	"rustdesk_already_running": {
		LangFR: "RustDesk est déjà ouvert et opérationnel sur votre poste.",
		LangEN: "RustDesk is already open and running on your computer.",
	},
	"diag_title": {
		LangFR: "Diagnostic de connexion RelaisDesk",
		LangEN: "RelaisDesk Connection Diagnostics",
	},
	"diag_analyzing": {
		LangFR: "Analyse de la connectivité réseau en cours...",
		LangEN: "Analyzing network connectivity...",
	},
	"diag_analyzing_sub": {
		LangFR: "Vérification de l'API, de la résolution DNS et des ports de téléassistance...",
		LangEN: "Checking API, DNS resolution, and remote support ports...",
	},
	"diag_generated": {
		LangFR: "Rapport généré le %s",
		LangEN: "Report generated on %s",
	},
	"diag_all_ok": {
		LangFR: "✅ Tous les contrôles sont validés — Connexion opérationnelle",
		LangEN: "✅ All checks passed — Connection operational",
	},
	"diag_issues": {
		LangFR: "⚠️ Anomalie détectée — Un ou plusieurs services ne répondent pas",
		LangEN: "⚠️ Issue detected — One or more services are not responding",
	},
	"diag_privacy": {
		LangFR: "🔒 Ce rapport ne contient aucune clé secrète, aucun jeton de session, ni aucune donnée privée.",
		LangEN: "🔒 This report contains no secret keys, session tokens, or private data.",
	},
	"diag_copy": {
		LangFR: "📋 Copier le rapport",
		LangEN: "📋 Copy Report",
	},
	"close_btn": {
		LangFR: "Fermer",
		LangEN: "Close",
	},
	"success": {
		LangFR: "Succès",
		LangEN: "Success",
	},
	"error": {
		LangFR: "Erreur",
		LangEN: "Error",
	},
	"code_copied": {
		LangFR: "Code d'accès %s copié dans le presse-papiers !",
		LangEN: "Access code %s copied to clipboard!",
	},
	"code_generated_popup": {
		LangFR: "Code généré et copié dans le presse-papiers !\n\n%s",
		LangEN: "Code generated and copied to clipboard!\n\n%s",
	},
	"confirm_delete_title": {
		LangFR: "Confirmer la suppression",
		LangEN: "Confirm Deletion",
	},
	"confirm_delete_active": {
		LangFR: "Le client %s (Code : %s) est actuellement ACTIF.\n\nVoulez-vous vraiment RÉVOQUER cet accès et SUPPRIMER cette entrée ?",
		LangEN: "The client %s (Code: %s) is currently ACTIVE.\n\nDo you really want to REVOKE this access and DELETE this entry?",
	},
	"delete_success": {
		LangFR: "L'accès du client a été révoqué et l'entrée supprimée.",
		LangEN: "Client access has been revoked and entry deleted.",
	},
	"logout_success": {
		LangFR: "Déconnexion réussie.",
		LangEN: "Signed out successfully.",
	},
	"lang_btn": {
		LangFR: "🌐 English",
		LangEN: "🌐 Français",
	},
	"tab_temporary_codes": {
		LangFR: "🔑 Assistance ponctuelle (12h)",
		LangEN: "🔑 Temporary Support (12h)",
	},
	"tab_permanent_devices": {
		LangFR: "🖥️ Parc & Postes permanents",
		LangEN: "🖥️ Fleet & Permanent Devices",
	},
	"perm_gen_card_title": {
		LangFR: "Enrôler un poste permanent (Accès sans surveillance)",
		LangEN: "Enroll Permanent Machine (Unattended Access)",
	},
	"device_alias_label": {
		LangFR: "Nom du poste distant (Alias) :",
		LangEN: "Remote Device Name (Alias):",
	},
	"device_alias_placeholder": {
		LangFR: "Ex : Serveur Compta, PC Accueil, Caisse #1...",
		LangEN: "E.g.: Accounting Server, Reception PC, POS #1...",
	},
	"device_notes_label": {
		LangFR: "Emplacement / Notes (optionnel) :",
		LangEN: "Location / Notes (optional):",
	},
	"device_notes_placeholder": {
		LangFR: "Ex : Bâtiment A, Bureau 204...",
		LangEN: "E.g.: Building A, Office 204...",
	},
	"generate_perm_code_btn": {
		LangFR: "⚡ Créer le code d'installation (15 min)",
		LangEN: "⚡ Create installation code (15 min)",
	},
	"perm_code_generated_popup": {
		LangFR: "Code d'installation : %s\n\nCopié dans le presse-papiers. Valable 15 minutes pour un seul poste.\nWindows ou Linux avec systemd : service du fork et mot de passe permanent requis. Windows : Viewer en administrateur. Linux : nouvelles versions du Viewer et du fork, puis sudo relaisdesk-viewer --enroll (saisir le code).",
		LangEN: "Installation code: %s\n\nCopied to clipboard. Valid 15 minutes for one device.\nWindows or Linux with systemd: fork service and permanent password required. Windows: run Viewer as administrator. Linux: updated Viewer and client fork, then sudo relaisdesk-viewer --enroll (enter the code).",
	},
	"devices_summary": {
		LangFR: "🖥️ %d postes  •  🟢 %d en ligne  •  ⚪ %d hors ligne",
		LangEN: "🖥️ %d devices  •  🟢 %d online  •  ⚪ %d offline",
	},
	"no_devices_yet": {
		LangFR: "Aucun poste permanent n'est enregistré dans votre parc pour le moment.\nGénérez un code ci-dessus pour enrôler vos serveurs et ordinateurs distants.",
		LangEN: "No permanent devices registered in your fleet yet.\nGenerate a code above to enroll your remote servers and computers.",
	},
	"device_status_online": {
		LangFR: "Agent et service actifs",
		LangEN: "Agent and service active",
	},
	"device_status_offline": {
		LangFR: "Hors ligne",
		LangEN: "Offline",
	},
	"last_seen_prefix": {
		LangFR: "Activité : %s",
		LangEN: "Last seen: %s",
	},
	"confirm_delete_device_title": {
		LangFR: "Supprimer le poste permanent",
		LangEN: "Delete Permanent Device",
	},
	"confirm_delete_device_msg": {
		LangFR: "Voulez-vous supprimer définitivement le poste '%s' (ID : %s) de votre console de gestion ?",
		LangEN: "Do you want to permanently remove device '%s' (ID: %s) from your management console?",
	},
	"delete_device_success": {
		LangFR: "Poste supprimé de votre parc avec succès.",
		LangEN: "Device successfully deleted from your fleet.",
	},
	"copy_cli_cmd": {
		LangFR: "💻 Commande CLI",
		LangEN: "💻 CLI Command",
	},
	"cli_cmd_copied": {
		LangFR: "Commande d'enrôlement copiée dans le presse-papiers !",
		LangEN: "Enrollment command copied to clipboard!",
	},
	"totp_title": {
		LangFR: "Double authentification (2FA)",
		LangEN: "Two-Factor Authentication (2FA)",
	},
	"totp_subtitle": {
		LangFR: "Saisissez votre code Authenticator ou demandez un code par e-mail :",
		LangEN: "Enter your Authenticator code or request a code by email:",
	},
	"totp_code_placeholder": {
		LangFR: "Code à 6 chiffres (e-mail ou Authenticator)",
		LangEN: "6-digit code (email or Authenticator)",
	},
	"totp_send_email_btn": {
		LangFR: "📧 M'envoyer un code par e-mail",
		LangEN: "📧 Send me a code by email",
	},
	"totp_email_sending": {
		LangFR: "Envoi du code par e-mail...",
		LangEN: "Sending code by email...",
	},
	"totp_email_sent_msg": {
		LangFR: "Code envoyé à %s ! (valide 15 minutes)",
		LangEN: "Code sent to %s! (valid for 15 minutes)",
	},
	"totp_email_wait": {
		LangFR: "Renvoyer dans %ds",
		LangEN: "Resend in %ds",
	},
	"totp_remember_device": {
		LangFR: "Faire confiance à cet appareil pendant 30 jours",
		LangEN: "Trust this device for 30 days",
	},
	"totp_or_divider": {
		LangFR: "— OU —",
		LangEN: "— OR —",
	},
	"totp_verify_btn": {
		LangFR: "Valider la connexion",
		LangEN: "Verify & Login",
	},
	"totp_back_btn": {
		LangFR: "← Retour",
		LangEN: "← Back",
	},
	"totp_err_empty": {
		LangFR: "Veuillez saisir votre code d'authentification",
		LangEN: "Please enter your authentication code",
	},
	"totp_checking": {
		LangFR: "Vérification du code...",
		LangEN: "Verifying code...",
	},
	"search_devices_placeholder": {
		LangFR: "Rechercher par nom, hôte, dossier...",
		LangEN: "Search by name, host, folder...",
	},
	"btn_new_folder": {
		LangFR: "➕ Nouveau dossier",
		LangEN: "➕ New folder",
	},
	"btn_new_subfolder": {
		LangFR: "➕ Sous-dossier",
		LangEN: "➕ Subfolder",
	},
	"btn_rename_folder": {
		LangFR: "✏️ Renommer",
		LangEN: "✏️ Rename",
	},
	"btn_delete_folder": {
		LangFR: "🗑️ Supprimer",
		LangEN: "🗑️ Delete",
	},
	"btn_move_device": {
		LangFR: "📁 Déplacer",
		LangEN: "📁 Move",
	},
	"folder_filter_label": {
		LangFR: "Dossier :",
		LangEN: "Folder:",
	},
	"folder_all": {
		LangFR: "📁 Tous les postes",
		LangEN: "📁 All devices",
	},
	"folder_root": {
		LangFR: "📁 Racine",
		LangEN: "📁 Root",
	},
	"breadcrumb_root": {
		LangFR: "🏠 Racine",
		LangEN: "🏠 Root",
	},
	"breadcrumb_all_devices": {
		LangFR: "👁️ Tous les postes",
		LangEN: "👁️ All devices",
	},
	"subfolders_label": {
		LangFR: "Sous-dossiers :",
		LangEN: "Subfolders:",
	},
	"level3_subfolder_hint": {
		LangFR: "Sous-sous-dossier (Niveau 3)",
		LangEN: "Sub-subfolder (Level 3)",
	},
	"parent_folder_label": {
		LangFR: "Emplacement (dossier parent) :",
		LangEN: "Location (parent folder):",
	},
	"folder_level_1": {
		LangFR: "Niveau 1 (Dossier racine)",
		LangEN: "Level 1 (Root folder)",
	},
	"folder_level_2": {
		LangFR: "Niveau 2 (Sous-dossier)",
		LangEN: "Level 2 (Subfolder)",
	},
	"folder_level_3": {
		LangFR: "Niveau 3 (Sous-sous-dossier)",
		LangEN: "Level 3 (Sub-subfolder)",
	},
	"folder_name_prompt": {
		LangFR: "Nom du dossier :",
		LangEN: "Folder name:",
	},
	"rename_folder_title": {
		LangFR: "Renommer le dossier",
		LangEN: "Rename folder",
	},
	"delete_folder_title": {
		LangFR: "Supprimer le dossier",
		LangEN: "Delete folder",
	},
	"delete_folder_confirm": {
		LangFR: "Voulez-vous supprimer ce dossier ? Ses sous-dossiers seront également supprimés et les postes rattachés replacés à la racine.",
		LangEN: "Do you want to delete this folder? Subfolders will also be deleted and devices moved to root.",
	},
	"move_device_title": {
		LangFR: "Déplacer / Renommer le poste",
		LangEN: "Move / Rename device",
	},
	"move_device_prompt": {
		LangFR: "Sélectionnez le dossier de destination :",
		LangEN: "Select destination folder:",
	},
	"device_folder_label": {
		LangFR: "Dossier de destination :",
		LangEN: "Destination folder:",
	},
	"folder_success": {
		LangFR: "Opération réussie",
		LangEN: "Operation succeeded",
	},
	"save_btn": {
		LangFR: "Enregistrer",
		LangEN: "Save",
	},
	"cancel_btn": {
		LangFR: "Annuler",
		LangEN: "Cancel",
	},
	"wake_btn": {
		LangFR: "⚡ Réveiller (WoL)",
		LangEN: "⚡ Wake (WoL)",
	},
	"wake_dialog_title": {
		LangFR: "Réveiller le poste (Wake-on-LAN)",
		LangEN: "Wake Device (Wake-on-LAN)",
	},
	"wake_mac_label": {
		LangFR: "Adresse MAC :",
		LangEN: "MAC Address:",
	},
	"wake_target_label": {
		LangFR: "Poste cible :",
		LangEN: "Target device:",
	},
	"wake_local_chk": {
		LangFR: "Diffuser sur le réseau local (WoL Direct)",
		LangEN: "Broadcast on local network (Direct WoL)",
	},
	"wake_relay_chk": {
		LangFR: "Relayer via les postes actifs du parc (WoL Cloud)",
		LangEN: "Relay via active fleet peers (Cloud WoL)",
	},
	"wake_send_btn": {
		LangFR: "Envoyer le signal de réveil",
		LangEN: "Send wake signal",
	},
	"wake_success_title": {
		LangFR: "Signal WoL envoyé",
		LangEN: "WoL Signal Sent",
	},
	"wake_success_msg": {
		LangFR: "Signal de réveil transmis avec succès.\n• Paquets locaux émis : %d\n• Postes relais notifiés : %d\n\nSi le poste a le WoL activé dans son BIOS, il démarrera d'ici 1 à 2 minutes.",
		LangEN: "Wake signal transmitted successfully.\n• Local packets sent: %d\n• Relay peers notified: %d\n\nIf the machine has WoL enabled in BIOS, it will boot up within 1 to 2 minutes.",
	},
	"wake_tool_title": {
		LangFR: "Outil Wake-on-LAN (WoL)",
		LangEN: "Wake-on-LAN Tool (WoL)",
	},
	"wake_manual_btn": {
		LangFR: "⚡ Outil Wake-on-LAN",
		LangEN: "⚡ Wake-on-LAN Tool",
	},
	"wake_manual_prompt": {
		LangFR: "Saisissez l'adresse MAC de l'ordinateur à réveiller sur le réseau local :",
		LangEN: "Enter the MAC address of the computer to wake on the local network:",
	},
	"wake_manual_sent": {
		LangFR: "%d paquets magiques WoL émis avec succès sur le réseau local vers %s.",
		LangEN: "%d WoL magic packets successfully broadcast on local network to %s.",
	},
	"wake_mac_missing": {
		LangFR: "Adresse MAC non renseignée pour ce poste. Saisissez-la manuellement ci-dessous pour le réveiller.",
		LangEN: "MAC address not found for this device. Enter it manually below to wake it up.",
	},
	"btn_update_device": {
		LangFR: "Mise à jour disponible",
		LangEN: "Update available",
	},
	"confirm_update_device_msg": {
		LangFR: "Programmer la mise à jour automatique à distance pour « %s » ?\nLe poste appliquera la mise à jour et redémarrera son service automatiquement lors de son prochain battement de cœur.",
		LangEN: "Schedule remote automatic update for \"%s\"?\nThe device will apply the update and restart its service automatically at its next heartbeat.",
	},
	"update_scheduled_success": {
		LangFR: "Mise à jour programmée avec succès. Le poste se mettra à jour en tâche de fond.",
		LangEN: "Update successfully scheduled. The device will update silently in the background.",
	},
}

func T(key string) string {
	l := GetLang()
	if m, ok := messages[key]; ok {
		if val, ok := m[l]; ok {
			return val
		}
		if val, ok := m[LangFR]; ok {
			return val
		}
	}
	return key
}

func TF(key string, args ...any) string {
	format := T(key)
	return fmt.Sprintf(format, args...)
}
