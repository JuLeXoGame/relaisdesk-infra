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
	"welcome_title": {
		LangFR: "Bienvenue dans %s",
		LangEN: "Welcome to %s",
	},
	"enter_code_desc": {
		LangFR: "Entrez le code fourni par votre technicien :",
		LangEN: "Enter the code provided by your technician:",
	},
	"access_code_label": {
		LangFR: "Code d'accès :",
		LangEN: "Access code:",
	},
	"code_placeholder": {
		LangFR: "XXXX-XXXX",
		LangEN: "XXXX-XXXX",
	},
	"connect_btn": {
		LangFR: "🔌 Se connecter",
		LangEN: "🔌 Connect",
	},
	"error": {
		LangFR: "Erreur",
		LangEN: "Error",
	},
	"su_title": {
		LangFR: "Mise à jour",
		LangEN: "Update",
	},
	"su_available": {
		LangFR: "La version %s est disponible. L'installer et redémarrer ?",
		LangEN: "Version %s is available. Install and restart?",
	},
	"su_install_restart": {
		LangFR: "Installer et redémarrer",
		LangEN: "Install and restart",
	},
	"su_later": {
		LangFR: "Plus tard",
		LangEN: "Later",
	},
	"su_working": {
		LangFR: "Téléchargement et installation en cours…",
		LangEN: "Downloading and installing…",
	},
	"su_failed": {
		LangFR: "Échec de la mise à jour : %v",
		LangEN: "Update failed: %v",
	},
	"err_empty_code": {
		LangFR: "Veuillez entrer un code.",
		LangEN: "Please enter a code.",
	},
	"err_prefix": {
		LangFR: "❌ Erreur : %v",
		LangEN: "❌ Error: %v",
	},
	"active_desc": {
		LangFR: "✅ RelaisDesk est actif. Gardez cette fenêtre ouverte pendant l’assistance.",
		LangEN: "✅ RelaisDesk is active. Keep this window open during remote support.",
	},
	"step_check_code": {
		LangFR: "🔄 Vérification du code...",
		LangEN: "🔄 Verifying access code...",
	},
	"step_valid_code": {
		LangFR: "✅ Code valide ! Connexion au serveur...",
		LangEN: "✅ Valid code! Connecting to server...",
	},
	"step_prep_rustdesk": {
		LangFR: "🔍 Préparation de RustDesk...",
		LangEN: "🔍 Preparing RustDesk...",
	},
	"err_prep_rustdesk": {
		LangFR: "Erreur lors de la préparation de RustDesk",
		LangEN: "Error preparing RustDesk",
	},
	"step_securing_auth": {
		LangFR: "🔐 Autorisation sécurisée de l’appareil...",
		LangEN: "🔐 Securing device authorization...",
	},
	"err_net_auth": {
		LangFR: "Erreur d'autorisation réseau",
		LangEN: "Network authorization error",
	},
	"step_config_rustdesk": {
		LangFR: "⚙️ Configuration de RustDesk...",
		LangEN: "⚙️ Configuring RustDesk...",
	},
	"step_config_applied": {
		LangFR: "✅ Configuration appliquée !",
		LangEN: "✅ Configuration applied!",
	},
	"step_shortcut": {
		LangFR: "🔗 Création du raccourci bureau...",
		LangEN: "🔗 Creating desktop shortcut...",
	},
	"warn_shortcut": {
		LangFR: "⚠️ Raccourci non créé : %s",
		LangEN: "⚠️ Shortcut not created: %s",
	},
	"step_launch": {
		LangFR: "🚀 Lancement de RustDesk...",
		LangEN: "🚀 Launching RustDesk...",
	},
	"step_success_expire": {
		LangFR: "🎉 Configuration réussie !\nVotre code expire le %s.",
		LangEN: "🎉 Setup successful!\nYour code expires on %s.",
	},
	"date_format": {
		LangFR: "02/01/2006 à 15:04",
		LangEN: "01/02/2006 at 15:04",
	},
	"lang_btn": {
		LangFR: "🌐 English",
		LangEN: "🌐 Français",
	},
	"session_ready": {
		LangFR: "🟢 Téléassistance Prête",
		LangEN: "🟢 Remote Assistance Ready",
	},
	"tech_can_connect": {
		LangFR: "Le code est validé.\nLe technicien peut à présent se connecter à votre ordinateur.",
		LangEN: "The code is verified.\nThe technician can now connect to your computer.",
	},
	"keep_window_open": {
		LangFR: "Veuillez laisser cette fenêtre ouverte pendant toute la durée de l'intervention.",
		LangEN: "Please keep this window open throughout the assistance session.",
	},
	"disconnect_btn": {
		LangFR: "🛑 Terminer l'assistance",
		LangEN: "🛑 End assistance",
	},
	"session_active_code": {
		LangFR: "Code d'accès actif : %s",
		LangEN: "Active access code: %s",
	},
	"session_valid_until": {
		LangFR: "Valide jusqu'au : %s",
		LangEN: "Valid until: %s",
	},
	"perm_enrolled_title": {
		LangFR: "Service d'autorisation installé",
		LangEN: "Authorization Service Installed",
	},
	"perm_enrolled_desc": {
		LangFR: "Ce poste est enregistré de manière permanente dans la console de gestion.",
		LangEN: "This machine is permanently enrolled in the management console.",
	},
	"perm_enrolled_notice": {
		LangFR: "Le service reste actif après fermeture et redémarre avec Windows. Validez une connexion réelle avant de quitter le poste.",
		LangEN: "The service remains active after closing and restarts with Windows. Test an actual connection before leaving the device.",
	},
	"close_btn": {
		LangFR: "Fermer",
		LangEN: "Close",
	},
	"perm_elevate_notice": {
		LangFR: "Demande d'autorisation administrateur (UAC)...",
		LangEN: "Requesting administrator privileges (UAC)...",
	},
	"perm_step_install_service": {
		LangFR: "📦 Installation du service système RustDesk...",
		LangEN: "📦 Installing RustDesk system service...",
	},
	"perm_step_wait_password": {
		LangFR: "Configuration du mot de passe permanent",
		LangEN: "Permanent Password Configuration",
	},
	"perm_open_rustdesk_btn": {
		LangFR: "⚙️ Ouvrir RustDesk pour configurer le mot de passe",
		LangEN: "⚙️ Open RustDesk to configure password",
	},
	"perm_password_instruction": {
		LangFR: "Une fenêtre RustDesk est ouverte.\nAllez dans Paramètres ⚙️ > Sécurité, activez le mot de passe permanent et définissez un mot de passe fort.\nLa détection est automatique dès sa configuration.",
		LangEN: "A RustDesk window is open.\nGo to Settings ⚙️ > Security, enable permanent password and set a strong password.\nDetection is automatic once configured.",
	},
	"perm_password_detected": {
		LangFR: "✅ Mot de passe permanent configuré !",
		LangEN: "✅ Permanent password configured!",
	},
	"perm_step_enrolling": {
		LangFR: "🌐 Enrôlement du poste auprès de l'API RelaisDesk...",
		LangEN: "🌐 Enrolling machine with RelaisDesk API...",
	},
	"perm_step_replacing": {
		LangFR: "♻️ Remplacement de l'accès permanent existant...",
		LangEN: "♻️ Replacing the existing permanent access...",
	},
	"cancel_btn": {
		LangFR: "Annuler",
		LangEN: "Cancel",
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
