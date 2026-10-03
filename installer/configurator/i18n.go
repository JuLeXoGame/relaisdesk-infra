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
		LangFR: "Racine",
		LangEN: "Root",
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
	"nav_overview": {
		LangFR: "Vue d'ensemble",
		LangEN: "Overview",
	},
	"nav_codes": {
		LangFR: "Téléassistance & Codes",
		LangEN: "Remote support & Codes",
	},
	"nav_fleet": {
		LangFR: "Parc & Postes",
		LangEN: "Fleet & Devices",
	},
	"nav_team": {
		LangFR: "Équipe & droits",
		LangEN: "Team & access",
	},
	"nav_licenses": {
		LangFR: "Licences",
		LangEN: "Licenses",
	},
	"nav_billing": {
		LangFR: "Commandes & factures",
		LangEN: "Orders & invoices",
	},
	"nav_history": {
		LangFR: "Historique",
		LangEN: "History",
	},
	"nav_services": {
		LangFR: "Prestations & paiements",
		LangEN: "Services & payments",
	},
	"nav_settings": {
		LangFR: "Préférences",
		LangEN: "Preferences",
	},
	"coming_soon": {
		LangFR: "Rubrique en cours d'intégration…",
		LangEN: "Section being integrated…",
	},
	"customer_login_title": {
		LangFR: "Connexion — Espace client",
		LangEN: "Sign in — Customer portal",
	},
	"customer_session_expired": {
		LangFR: "Votre session a expiré, reconnectez-vous.",
		LangEN: "Your session has expired, please sign in again.",
	},
	"totp_subtitle_customer": {
		LangFR: "Saisissez le code à 6 chiffres pour %s",
		LangEN: "Enter the 6-digit code for %s",
	},
	"overview_account_title": {
		LangFR: "Mon compte",
		LangEN: "My account",
	},
	"overview_account_line": {
		LangFR: "Client %s — %s",
		LangEN: "Customer %s — %s",
	},
	"overview_metric_licenses": {
		LangFR: "Licences",
		LangEN: "Licenses",
	},
	"overview_metric_invoices": {
		LangFR: "Factures",
		LangEN: "Invoices",
	},
	"overview_metric_interventions": {
		LangFR: "Interventions",
		LangEN: "Interventions",
	},
	"licenses_title": {
		LangFR: "Mes licences",
		LangEN: "My licenses",
	},
	"licenses_empty": {
		LangFR: "Aucune licence pour le moment.",
		LangEN: "No licenses yet.",
	},
	"license_expiry_line": {
		LangFR: "Expire le %s — %d connexion(s) simultanée(s)",
		LangEN: "Expires %s — %d simultaneous connection(s)",
	},
	"license_no_end_date": {
		LangFR: "Sans date de fin",
		LangEN: "No end date",
	},
	"subscriptions_title": {
		LangFR: "Abonnements",
		LangEN: "Subscriptions",
	},
	"subscriptions_empty": {
		LangFR: "Aucun abonnement pour le moment.",
		LangEN: "No subscriptions yet.",
	},
	"subscription_active": {
		LangFR: "Actif",
		LangEN: "Active",
	},
	"subscription_cancelled": {
		LangFR: "Résilié (fin de période)",
		LangEN: "Cancelled (end of period)",
	},
	"subscription_cancel_pending": {
		LangFR: "Résiliation demandée",
		LangEN: "Cancellation requested",
	},
	"subscription_withdrawn": {
		LangFR: "Rétracté",
		LangEN: "Withdrawn",
	},
	"subscription_detail_line": {
		LangFR: "%s / mois — prochaine échéance : %s",
		LangEN: "%s / month — next billing: %s",
	},
	"subscription_cancel_btn": {
		LangFR: "Résilier",
		LangEN: "Cancel",
	},
	"subscription_cancel_confirm": {
		LangFR: "Résilier cet abonnement à la fin de la période en cours ?",
		LangEN: "Cancel this subscription at the end of the current period?",
	},
	"settings_account_title": {
		LangFR: "Compte client",
		LangEN: "Customer account",
	},
	"settings_prefs_title": {
		LangFR: "Notifications",
		LangEN: "Notifications",
	},
	"settings_reminders_label": {
		LangFR: "M'avertir avant l'expiration de mes licences",
		LangEN: "Warn me before my licenses expire",
	},
	"settings_password_title": {
		LangFR: "Mot de passe",
		LangEN: "Password",
	},
	"password_old_placeholder": {
		LangFR: "Mot de passe actuel",
		LangEN: "Current password",
	},
	"password_new_placeholder": {
		LangFR: "Nouveau mot de passe",
		LangEN: "New password",
	},
	"password_confirm_placeholder": {
		LangFR: "Confirmer le nouveau mot de passe",
		LangEN: "Confirm new password",
	},
	"password_change_btn": {
		LangFR: "Changer le mot de passe",
		LangEN: "Change password",
	},
	"password_mismatch": {
		LangFR: "Les deux mots de passe ne correspondent pas.",
		LangEN: "The two passwords do not match.",
	},
	"password_changed_ok": {
		LangFR: "Mot de passe modifié. Les anciennes sessions sont fermées.",
		LangEN: "Password changed. Previous sessions are closed.",
	},
	"settings_2fa_title": {
		LangFR: "Double authentification",
		LangEN: "Two-factor authentication",
	},
	"twofa_disabled": {
		LangFR: "2FA désactivée",
		LangEN: "2FA disabled",
	},
	"twofa_enabled": {
		LangFR: "2FA activée — %d code(s) de secours restant(s)",
		LangEN: "2FA enabled — %d recovery code(s) left",
	},
	"twofa_manage_btn": {
		LangFR: "Gérer sur le site",
		LangEN: "Manage on website",
	},
	"settings_session_title": {
		LangFR: "Session client",
		LangEN: "Customer session",
	},
	"logout_customer_btn": {
		LangFR: "Déconnecter le compte client",
		LangEN: "Sign out customer account",
	},
	"dialog_save_btn": {
		LangFR: "Enregistrer",
		LangEN: "Save",
	},
	"dialog_confirm_btn": {
		LangFR: "Confirmer",
		LangEN: "Confirm",
	},
	"dialog_cancel_btn": {
		LangFR: "Annuler",
		LangEN: "Cancel",
	},
	"download_saved_title": {
		LangFR: "Fichier enregistré",
		LangEN: "File saved",
	},
	"download_saved_msg": {
		LangFR: "Enregistré sous %s",
		LangEN: "Saved as %s",
	},
	"team_licenses_title": {
		LangFR: "Licences éligibles",
		LangEN: "Eligible licenses",
	},
	"team_no_licenses": {
		LangFR: "Aucune licence éligible à l'équipe.",
		LangEN: "No team-eligible licenses.",
	},
	"team_license_line": {
		LangFR: "%s (%s) — %d/%d place(s)",
		LangEN: "%s (%s) — %d/%d seat(s)",
	},
	"team_members_title": {
		LangFR: "Membres invités",
		LangEN: "Invited members",
	},
	"team_no_members": {
		LangFR: "Aucun membre pour le moment.",
		LangEN: "No members yet.",
	},
	"team_member_line": {
		LangFR: "Licence %s — dossiers : %s",
		LangEN: "License %s — folders: %s",
	},
	"team_all_folders": {
		LangFR: "tous",
		LangEN: "all",
	},
	"team_resend_btn": {
		LangFR: "Renvoyer",
		LangEN: "Resend",
	},
	"team_resend_ok": {
		LangFR: "Invitation renvoyée.",
		LangEN: "Invitation resent.",
	},
	"team_folders_btn": {
		LangFR: "Dossiers",
		LangEN: "Folders",
	},
	"team_revoke_btn": {
		LangFR: "Révoquer",
		LangEN: "Revoke",
	},
	"team_revoke_confirm": {
		LangFR: "Révoquer l'accès de %s ?",
		LangEN: "Revoke access for %s?",
	},
	"team_invite_title": {
		LangFR: "Inviter un membre",
		LangEN: "Invite a member",
	},
	"team_invite_no_capacity": {
		LangFR: "Aucune licence avec des places disponibles.",
		LangEN: "No license with available seats.",
	},
	"team_license_label": {
		LangFR: "Licence",
		LangEN: "License",
	},
	"team_folders_label": {
		LangFR: "Dossiers autorisés (vide = tout le parc)",
		LangEN: "Allowed folders (empty = whole fleet)",
	},
	"team_invite_btn": {
		LangFR: "Envoyer l'invitation",
		LangEN: "Send invitation",
	},
	"team_joined_title": {
		LangFR: "Équipes que j'ai rejointes",
		LangEN: "Teams I joined",
	},
	"team_no_memberships": {
		LangFR: "Vous n'avez rejoint aucune équipe.",
		LangEN: "You have not joined any team.",
	},
	"team_membership_line": {
		LangFR: "Licence %s — %s",
		LangEN: "License %s — %s",
	},
	"team_folders_dialog_sub": {
		LangFR: "Dossiers autorisés pour %s (vide = tout le parc) :",
		LangEN: "Allowed folders for %s (empty = whole fleet):",
	},
	"license_detail_line": {
		LangFR: "Créée le %s — expire le %s — %d connexion(s)",
		LangEN: "Created %s — expires %s — %d connection(s)",
	},
	"license_pending_renewal": {
		LangFR: "Renouvellement en attente",
		LangEN: "Renewal pending",
	},
	"license_renew_btn": {
		LangFR: "Renouveler",
		LangEN: "Renew",
	},
	"pay_method_stripe": {
		LangFR: "Carte bancaire (Stripe)",
		LangEN: "Credit card (Stripe)",
	},
	"pay_method_bank": {
		LangFR: "Virement bancaire",
		LangEN: "Bank transfer",
	},
	"pay_method_btc": {
		LangFR: "Crypto — Bitcoin",
		LangEN: "Crypto — Bitcoin",
	},
	"pay_method_xrp": {
		LangFR: "Crypto — XRP",
		LangEN: "Crypto — XRP",
	},
	"renew_dialog_sub": {
		LangFR: "Renouvellement de %s :",
		LangEN: "Renewing %s:",
	},
	"renew_terms_label": {
		LangFR: "J'accepte les CGV en vigueur (version %s)",
		LangEN: "I accept the current T&Cs (version %s)",
	},
	"renew_terms_link": {
		LangFR: "Lire les CGV",
		LangEN: "Read T&Cs",
	},
	"renew_terms_required": {
		LangFR: "Vous devez accepter les CGV en vigueur.",
		LangEN: "You must accept the current T&Cs.",
	},
	"renew_immediate_label": {
		LangFR: "Exécution immédiate demandée (consommateurs)",
		LangEN: "Immediate performance requested (consumers)",
	},
	"renew_order_created": {
		LangFR: "Commande %s créée. Finalisez le paiement dans le navigateur.",
		LangEN: "Order %s created. Complete payment in the browser.",
	},
	"renew_bank_details": {
		LangFR: "Virez %s à %s — IBAN %s — BIC %s — référence %s. Les instructions ont aussi été envoyées par e-mail.",
		LangEN: "Transfer %s to %s — IBAN %s — BIC %s — reference %s. Instructions were also emailed.",
	},
	"renew_crypto_title": {
		LangFR: "Paiement crypto",
		LangEN: "Crypto payment",
	},
	"renew_crypto_details": {
		LangFR: "%s %s à envoyer à %s (commande %s).",
		LangEN: "Send %s %s to %s (order %s).",
	},
	"orders_title": {
		LangFR: "Commandes",
		LangEN: "Orders",
	},
	"orders_empty": {
		LangFR: "Aucune commande pour le moment.",
		LangEN: "No orders yet.",
	},
	"order_detail_line": {
		LangFR: "%s — %d technicien(s) — %s (%s) — %s",
		LangEN: "%s — %d technician(s) — %s (%s) — %s",
	},
	"invoices_title": {
		LangFR: "Factures",
		LangEN: "Invoices",
	},
	"invoices_empty": {
		LangFR: "Aucune facture pour le moment.",
		LangEN: "No invoices yet.",
	},
	"invoice_detail_line": {
		LangFR: "%s TTC — %s",
		LangEN: "%s incl. tax — %s",
	},
	"invoice_pdf_btn": {
		LangFR: "PDF",
		LangEN: "PDF",
	},
	"invoice_cii_btn": {
		LangFR: "Factur-X",
		LangEN: "Factur-X",
	},
	"billing_portal_btn": {
		LangFR: "Gérer le paiement",
		LangEN: "Manage payment",
	},
	"billing_portal_invalid": {
		LangFR: "Lien du portail de paiement invalide.",
		LangEN: "Invalid billing portal link.",
	},
	"history_create_btn": {
		LangFR: "Nouvelle intervention",
		LangEN: "New intervention",
	},
	"history_export_btn": {
		LangFR: "Exporter CSV",
		LangEN: "Export CSV",
	},
	"history_empty": {
		LangFR: "Aucune intervention pour le moment.",
		LangEN: "No interventions yet.",
	},
	"history_item_line": {
		LangFR: "%s — réf. %s — %s",
		LangEN: "%s — ref. %s — %s",
	},
	"history_dates_line": {
		LangFR: "%s → %s (%d min)",
		LangEN: "%s → %s (%d min)",
	},
	"history_start_btn": {
		LangFR: "Démarrer",
		LangEN: "Start",
	},
	"history_complete_btn": {
		LangFR: "Clôturer",
		LangEN: "Complete",
	},
	"history_cancel_btn": {
		LangFR: "Annuler",
		LangEN: "Cancel",
	},
	"history_ref_label": {
		LangFR: "Référence client",
		LangEN: "Client reference",
	},
	"history_ref_placeholder": {
		LangFR: "ex. DUPONT-2026",
		LangEN: "e.g. SMITH-2026",
	},
	"history_title_label": {
		LangFR: "Titre",
		LangEN: "Title",
	},
	"history_title_placeholder": {
		LangFR: "ex. Remplacement du disque",
		LangEN: "e.g. Disk replacement",
	},
	"history_summary_placeholder": {
		LangFR: "Compte-rendu de l'intervention…",
		LangEN: "Intervention report…",
	},
	"history_complete_sub": {
		LangFR: "Clôturer « %s » :",
		LangEN: "Complete \"%s\":",
	},
	"services_unavailable": {
		LangFR: "Prestations non activées sur le serveur.",
		LangEN: "Services not enabled on the server.",
	},
	"services_merchant_title": {
		LangFR: "Compte d'encaissement",
		LangEN: "Payout account",
	},
	"services_no_account": {
		LangFR: "Aucun compte Stripe relié.",
		LangEN: "No Stripe account linked.",
	},
	"services_onboard_btn": {
		LangFR: "Relier Stripe",
		LangEN: "Connect Stripe",
	},
	"services_account_line": {
		LangFR: "Compte %s",
		LangEN: "Account %s",
	},
	"services_enabled_label": {
		LangFR: "Encaissements activés",
		LangEN: "Payments enabled",
	},
	"services_terms_title": {
		LangFR: "Conditions de prestations",
		LangEN: "Service terms",
	},
	"services_terms_accepted": {
		LangFR: "Acceptées le %s",
		LangEN: "Accepted on %s",
	},
	"services_terms_pending": {
		LangFR: "Version %s en attente d'acceptation",
		LangEN: "Version %s awaiting acceptance",
	},
	"services_terms_accept_btn": {
		LangFR: "Accepter les conditions",
		LangEN: "Accept terms",
	},
	"services_terms_read_btn": {
		LangFR: "Lire les conditions",
		LangEN: "Read terms",
	},
	"services_rates_title": {
		LangFR: "Catalogue",
		LangEN: "Catalog",
	},
	"services_no_rates": {
		LangFR: "Aucun tarif pour le moment.",
		LangEN: "No rates yet.",
	},
	"services_rate_prepaid": {
		LangFR: "forfait",
		LangEN: "flat",
	},
	"services_rate_hourly": {
		LangFR: "/heure",
		LangEN: "/hour",
	},
	"services_rate_delete_btn": {
		LangFR: "Supprimer",
		LangEN: "Delete",
	},
	"services_rate_delete_confirm": {
		LangFR: "Supprimer le tarif « %s » ?",
		LangEN: "Delete rate \"%s\"?",
	},
	"services_rate_add_btn": {
		LangFR: "Ajouter un tarif",
		LangEN: "Add a rate",
	},
	"services_rate_label_label": {
		LangFR: "Intitulé",
		LangEN: "Label",
	},
	"services_rate_label_placeholder": {
		LangFR: "ex. Dépannage à domicile",
		LangEN: "e.g. On-site repair",
	},
	"services_rate_mode_label": {
		LangFR: "Mode",
		LangEN: "Mode",
	},
	"services_rate_amount_label": {
		LangFR: "Montant (€)",
		LangEN: "Amount (€)",
	},
	"services_rate_amount_invalid": {
		LangFR: "Montant invalide.",
		LangEN: "Invalid amount.",
	},
	"services_work_title": {
		LangFR: "Prestations",
		LangEN: "Jobs",
	},
	"services_no_work": {
		LangFR: "Aucune prestation pour le moment.",
		LangEN: "No jobs yet.",
	},
	"services_paid": {
		LangFR: "payée",
		LangEN: "paid",
	},
	"services_unpaid": {
		LangFR: "impayée",
		LangEN: "unpaid",
	},
	"services_work_line": {
		LangFR: "%s — %s",
		LangEN: "%s — %s",
	},
	"services_open_link_btn": {
		LangFR: "Ouvrir le lien",
		LangEN: "Open link",
	},
	"services_copy_link_btn": {
		LangFR: "Copier",
		LangEN: "Copy",
	},
	"services_checkout_btn": {
		LangFR: "Préparer le paiement",
		LangEN: "Prepare payment",
	},
	"services_finish_btn": {
		LangFR: "Terminer",
		LangEN: "Finish",
	},
	"services_cancel_btn": {
		LangFR: "Annuler",
		LangEN: "Cancel",
	},
	"fleet_tree_title": {
		LangFR: "Dossiers",
		LangEN: "Folders",
	},
	"btn_move_folder": {
		LangFR: "⇄ Déplacer",
		LangEN: "⇄ Move",
	},
	"move_folder_title": {
		LangFR: "Déplacer le dossier",
		LangEN: "Move folder",
	},
	"move_folder_dialog_sub": {
		LangFR: "Destination de « %s » :",
		LangEN: "Destination for \"%s\":",
	},
	"move_folder_confirm": {
		LangFR: "Déplacer « %s » vers « %s » ?",
		LangEN: "Move \"%s\" to \"%s\"?",
	},
	"move_folder_select_first": {
		LangFR: "Sélectionnez d'abord un dossier dans l'arbre.",
		LangEN: "Select a folder in the tree first.",
	},
	"twofa_enable_btn": {
		LangFR: "Activer la 2FA",
		LangEN: "Enable 2FA",
	},
	"twofa_disable_btn": {
		LangFR: "Désactiver la 2FA",
		LangEN: "Disable 2FA",
	},
	"twofa_disable_sub": {
		LangFR: "Confirmez avec votre mot de passe et un code :",
		LangEN: "Confirm with your password and a code:",
	},
	"twofa_scan_label": {
		LangFR: "Scannez ce code avec votre application d'authentification :",
		LangEN: "Scan this code with your authenticator app:",
	},
	"twofa_secret_label": {
		LangFR: "Clé secrète (saisie manuelle) :",
		LangEN: "Secret key (manual entry):",
	},
	"twofa_recovery_label": {
		LangFR: "Codes de secours (à conserver précieusement) :",
		LangEN: "Recovery codes (keep them safe):",
	},
	"twofa_code_label": {
		LangFR: "Code à 6 chiffres",
		LangEN: "6-digit code",
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
