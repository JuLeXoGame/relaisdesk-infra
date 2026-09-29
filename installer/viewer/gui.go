//go:build !linux

package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

//go:embed icon.png
var appIconBytes []byte

var (
	viewerApp    fyne.App
	viewerWindow fyne.Window
)

// permWaitPollGen invalidates stale permanent-password polling loops whenever the
// UI navigates to another screen; permEnrollClaimed guarantees a single
// enrollment when the manual save and the background detector race.
var permWaitPollGen atomic.Uint64
var permEnrollClaimed atomic.Bool

// RunGUI creates and displays the cross-platform activation GUI using Fyne for the Viewer.
func RunGUI() error {
	return RunGUIWithPreset("", false)
}

// RunGUIWithPreset starts the GUI with an optional preset code and optional automatic connection.
func RunGUIWithPreset(presetCode string, autoStart bool) error {
	viewerApp = app.NewWithID("com.relaisdesk.viewer")
	if len(appIconBytes) > 0 {
		appIcon := fyne.NewStaticResource("icon.png", appIconBytes)
		viewerApp.SetIcon(appIcon)
	}

	viewerWindow = viewerApp.NewWindow(PRODUCT_NAME)
	viewerWindow.Resize(fyne.NewSize(480, 460))
	viewerWindow.CenterOnScreen()
	viewerWindow.SetFixedSize(true)

	if len(appIconBytes) > 0 {
		appIcon := fyne.NewStaticResource("icon.png", appIconBytes)
		viewerWindow.SetIcon(appIcon)
	}

	viewerWindow.SetCloseIntercept(func() {
		if fleetServiceExists() {
			viewerWindow.Close()
			return
		}
		stopViewerNetworkRefresh()
		cleanupRustDesk2Toml()
		terminateRustDesk()
		viewerWindow.Close()
	})

	renderViewerScreen(presetCode, autoStart)
	viewerWindow.ShowAndRun()

	return nil
}

func renderViewerScreen(presetCode string, autoStart bool) {
	permWaitPollGen.Add(1)
	viewerWindow.SetTitle(PRODUCT_NAME)

	title := widget.NewLabelWithStyle(TF("welcome_title", PRODUCT_NAME), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	desc := widget.NewLabelWithStyle(T("enter_code_desc"), fyne.TextAlignCenter, fyne.TextStyle{})

	codeEntry := widget.NewEntry()
	codeEntry.PlaceHolder = T("code_placeholder")
	if presetCode != "" {
		codeEntry.SetText(presetCode)
	}

	statusLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	var activateButton *widget.Button
	activateButton = widget.NewButton(T("connect_btn"), func() {
		key := strings.TrimSpace(codeEntry.Text)
		if key == "" {
			dialog.ShowInformation(T("error"), T("err_empty_code"), viewerWindow)
			return
		}

		activateButton.Disable()
		codeEntry.Disable()

		setStatus := func(msg string) {
			fyne.Do(func() {
				statusLabel.SetText(msg)
			})
		}

		go func() {
			err := handleActivationFyne(key, setStatus)

			if err != nil {
				setStatus(TF("err_prefix", err))
				time.Sleep(3 * time.Second)
				fyne.Do(func() {
					activateButton.Enable()
					codeEntry.Enable()
					statusLabel.SetText("")
				})
			}
		}()
	})
	activateButton.Importance = widget.HighImportance

	if autoStart && presetCode != "" {
		go func() {
			time.Sleep(300 * time.Millisecond)
			fyne.Do(func() {
				if activateButton != nil && !activateButton.Disabled() {
					activateButton.OnTapped()
				}
			})
		}()
	}

	langBtn := widget.NewButton(T("lang_btn"), func() {
		ToggleLang()
		renderViewerScreen(codeEntry.Text, false)
	})

	topBar := container.NewHBox(layout.NewSpacer(), langBtn)

	form := container.NewVBox(
		widget.NewLabel(T("access_code_label")),
		codeEntry,
	)

	items := []fyne.CanvasObject{topBar}
	if len(appIconBytes) > 0 {
		logoImg := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIconBytes))
		logoImg.FillMode = canvas.ImageFillContain
		logoImg.SetMinSize(fyne.NewSize(72, 72))
		items = append(items, container.NewCenter(logoImg))
	}
	items = append(items,
		title,
		desc,
		layout.NewSpacer(),
		form,
		layout.NewSpacer(),
		container.NewPadded(activateButton),
		layout.NewSpacer(),
		statusLabel,
	)

	content := container.NewVBox(items...)
	viewerWindow.SetContent(container.NewPadded(content))
}

func handleActivationFyne(code string, setStatus func(string)) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if strings.HasPrefix(code, "PERM-") {
		return handlePermanentEnrollmentFyne(code, setStatus)
	}
	if fleetServiceExists() {
		return fmt.Errorf("un accès permanent est installé ; retirez-le explicitement avant une session temporaire")
	}

	setStatus(T("step_check_code"))
	activation, err := activateViewerCode(code)
	if err != nil {
		return err
	}
	setStatus(T("step_valid_code"))
	time.Sleep(500 * time.Millisecond)

	setStatus(T("step_prep_rustdesk"))
	rustdeskPath, err := ensureRustDesk()
	if err != nil {
		return fmt.Errorf("%s: %w", T("err_prep_rustdesk"), err)
	}
	time.Sleep(300 * time.Millisecond)

	setStatus(T("step_securing_auth"))
	networkAuthorization, err := prepareViewerNetworkAuthorization(code)
	if err != nil {
		return fmt.Errorf("%s: %w", T("err_net_auth"), err)
	}
	keepAuthorization := false
	defer func() {
		if !keepAuthorization {
			_ = os.Remove(networkAuthorization.TokenFile)
			cleanupRustDesk2Toml()
		}
	}()
	activation.NetworkTokenFile = networkAuthorization.TokenFile
	activation.NetworkProofKeyFile = networkAuthorization.ProofKeyFile

	setStatus(T("step_config_rustdesk"))
	if err := configureRustDesk(activation, code); err != nil {
		return err
	}
	setStatus(T("step_config_applied"))
	time.Sleep(500 * time.Millisecond)

	setStatus(T("step_shortcut"))
	if err := createDesktopShortcut(rustdeskPath); err != nil {
		setStatus(TF("warn_shortcut", err.Error()))
	}
	time.Sleep(500 * time.Millisecond)

	setStatus(T("step_launch"))
	if err := launchRustDesk(rustdeskPath); err != nil {
		return err
	}
	startViewerNetworkRefresh(code, networkAuthorization)
	keepAuthorization = true

	// Attendre l'annonce avant de fermer le Viewer : les goroutines seraient
	// sinon interrompues par la fin du processus et l'ID pourrait ne jamais
	// parvenir au technicien.
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		id := getRustDeskID(rustdeskPath)
		if id != "" {
			if isRustDeskKeyConfirmed(RUSTDESK_CONFIG_DIR) || i >= 3 {
				_ = announceViewerID(code, id, networkAuthorization.DevicePublicKey)
				break
			}
		}
	}

	expDate, _ := time.Parse(time.RFC3339, activation.ExpiresAt)
	dateFormat := T("date_format")
	expStr := expDate.Format(dateFormat)
	setStatus(TF("step_success_expire", expStr))

	fyne.Do(func() {
		renderViewerActiveScreen(code, expStr)
	})
	return nil
}

func renderViewerActiveScreen(code, expiresAt string) {
	permWaitPollGen.Add(1)
	viewerWindow.SetTitle(PRODUCT_NAME + " - " + T("session_ready"))

	title := widget.NewLabelWithStyle(T("session_ready"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	desc := widget.NewLabelWithStyle(T("tech_can_connect"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	codeInfo := widget.NewLabelWithStyle(TF("session_active_code", code), fyne.TextAlignCenter, fyne.TextStyle{Bold: true, Monospace: true})
	validUntil := widget.NewLabelWithStyle(TF("session_valid_until", expiresAt), fyne.TextAlignCenter, fyne.TextStyle{})
	warning := widget.NewLabelWithStyle(T("keep_window_open"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	disconnectBtn := widget.NewButton(T("disconnect_btn"), func() {
		stopViewerNetworkRefresh()
		cleanupRustDesk2Toml()
		terminateRustDesk()
		viewerWindow.Close()
	})
	disconnectBtn.Importance = widget.DangerImportance

	langBtn := widget.NewButton(T("lang_btn"), func() {
		ToggleLang()
		renderViewerActiveScreen(code, expiresAt)
	})

	topBar := container.NewHBox(layout.NewSpacer(), langBtn)

	items := []fyne.CanvasObject{topBar}
	if len(appIconBytes) > 0 {
		logoImg := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIconBytes))
		logoImg.FillMode = canvas.ImageFillContain
		logoImg.SetMinSize(fyne.NewSize(72, 72))
		items = append(items, container.NewCenter(logoImg))
	}

	items = append(items,
		title,
		layout.NewSpacer(),
		desc,
		layout.NewSpacer(),
		codeInfo,
		validUntil,
		layout.NewSpacer(),
		warning,
		layout.NewSpacer(),
		container.NewPadded(disconnectBtn),
	)

	content := container.NewVBox(items...)
	viewerWindow.SetContent(container.NewPadded(content))
}

func handlePermanentEnrollmentFyne(code string, setStatus func(string)) error {
	if !isElevated() {
		setStatus(T("perm_elevate_notice"))
		time.Sleep(600 * time.Millisecond)
		err := relaunchElevated([]string{"--gui-enroll", code})
		if err != nil {
			return fmt.Errorf("élévation administrateur requise : %w", err)
		}
		os.Exit(0)
		return nil
	}

	setStatus(T("perm_step_install_service"))
	if _, err := ensureRustDeskServiceInstalled(); err != nil {
		return fmt.Errorf("échec de l'installation du service : %w", err)
	}

	hasPwd, _ := hasFleetPermanentPassword()
	if !hasPwd {
		_ = openRustDeskSettings()
		fyne.Do(func() {
			renderPermanentPasswordWaitScreen(code)
		})
		return nil
	}

	setStatus(T("perm_step_enrolling"))
	resp, err := installFleet(code)
	if isFleetReplaceableError(err) {
		setStatus(T("perm_step_replacing"))
		_ = uninstallFleet()
		resp, err = installFleet(code)
	}
	if err != nil {
		return err
	}
	setStatus("Service installé ; vérifier la connexion dans la console.")
	fyne.Do(func() { renderPermanentEnrolledScreen(resp.DeviceID, resp.Alias, "Code d'installation consommé") })
	return nil
}

func renderPermanentPasswordWaitScreen(code string) {
	myPollGen := permWaitPollGen.Add(1)
	permEnrollClaimed.Store(false)
	viewerWindow.SetTitle(PRODUCT_NAME + " - " + T("perm_step_wait_password"))

	title := widget.NewLabelWithStyle("🔑 "+T("perm_step_wait_password"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	desc := widget.NewLabelWithStyle("Définissez le mot de passe permanent pour permettre l'accès à ce poste sans confirmation manuelle :", fyne.TextAlignCenter, fyne.TextStyle{})

	pwdEntry := widget.NewPasswordEntry()
	pwdEntry.SetPlaceHolder("Mot de passe permanent (min. 6 caractères)")

	confirmEntry := widget.NewPasswordEntry()
	confirmEntry.SetPlaceHolder("Confirmer le mot de passe")

	statusLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	var saveBtn *widget.Button
	saveBtn = widget.NewButton("🔒 Enregistrer et activer l'accès permanent", func() {
		p1 := strings.TrimSpace(pwdEntry.Text)
		p2 := strings.TrimSpace(confirmEntry.Text)
		if len(p1) < 6 {
			statusLabel.SetText("⚠️ Le mot de passe doit comporter au moins 6 caractères")
			return
		}
		if p1 != p2 {
			statusLabel.SetText("⚠️ Les deux mots de passe ne correspondent pas")
			return
		}
		if !permEnrollClaimed.CompareAndSwap(false, true) {
			statusLabel.SetText("Activation déjà en cours, veuillez patienter...")
			return
		}
		saveBtn.Disable()
		statusLabel.SetText("Configuration du mot de passe permanent...")
		go func() {
			if err := setFleetPermanentPassword(p1); err != nil {
				permEnrollClaimed.Store(false)
				fyne.Do(func() {
					saveBtn.Enable()
					statusLabel.SetText(fmt.Sprintf("❌ Erreur : %v", err))
				})
				return
			}
			fyne.Do(func() {
				statusLabel.SetText(T("perm_password_detected"))
			})
			time.Sleep(500 * time.Millisecond)
			setStatus := func(msg string) {
				fyne.Do(func() {
					statusLabel.SetText(msg)
				})
			}
			if err := handlePermanentEnrollmentFyne(code, setStatus); err != nil {
				permEnrollClaimed.Store(false)
				fyne.Do(func() {
					saveBtn.Enable()
					statusLabel.SetText(fmt.Sprintf("❌ Erreur : %v", err))
				})
			}
		}()
	})
	saveBtn.Importance = widget.HighImportance

	manualBtn := widget.NewButton(T("perm_open_rustdesk_btn"), func() {
		_ = openRustDeskSettings()
	})

	cancelBtn := widget.NewButton(T("cancel_btn"), func() {
		renderViewerScreen(code, false)
	})

	var items []fyne.CanvasObject
	if len(appIconBytes) > 0 {
		logoImg := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIconBytes))
		logoImg.FillMode = canvas.ImageFillContain
		logoImg.SetMinSize(fyne.NewSize(64, 64))
		items = append(items, container.NewCenter(logoImg))
	}

	formContainer := container.NewVBox(
		widget.NewLabel("Mot de passe :"),
		pwdEntry,
		widget.NewLabel("Confirmation :"),
		confirmEntry,
		container.NewPadded(saveBtn),
	)

	items = append(items,
		title,
		desc,
		formContainer,
		statusLabel,
		widget.NewSeparator(),
		container.NewPadded(manualBtn),
		container.NewPadded(cancelBtn),
	)

	viewerWindow.SetContent(container.NewPadded(container.NewVBox(items...)))

	go func() {
		for {
			time.Sleep(1500 * time.Millisecond)
			if permWaitPollGen.Load() != myPollGen {
				return
			}
			hasPwd, _ := hasFleetPermanentPassword()
			if hasPwd {
				if !permEnrollClaimed.CompareAndSwap(false, true) {
					return
				}
				if permWaitPollGen.Load() != myPollGen {
					permEnrollClaimed.Store(false)
					return
				}
				fyne.Do(func() {
					statusLabel.SetText(T("perm_password_detected"))
				})
				time.Sleep(1 * time.Second)
				setStatus := func(msg string) {
					fyne.Do(func() {
						statusLabel.SetText(msg)
					})
				}
				if err := handlePermanentEnrollmentFyne(code, setStatus); err != nil {
					permEnrollClaimed.Store(false)
					fyne.Do(func() {
						statusLabel.SetText(fmt.Sprintf("❌ Erreur : %v", err))
					})
				}
				return
			}
		}
	}()
}

func renderPermanentEnrolledScreen(deviceID, alias, code string) {
	permWaitPollGen.Add(1)
	viewerWindow.SetTitle(PRODUCT_NAME + " - " + T("perm_enrolled_title"))

	title := widget.NewLabelWithStyle("🖥️ "+T("perm_enrolled_title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	desc := widget.NewLabelWithStyle(T("perm_enrolled_desc"), fyne.TextAlignCenter, fyne.TextStyle{})

	aliasLbl := widget.NewLabelWithStyle("Poste : "+alias, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	idLbl := widget.NewLabelWithStyle("ID : "+deviceID, fyne.TextAlignCenter, fyne.TextStyle{Monospace: true})
	codeLbl := widget.NewLabelWithStyle("Code : "+code, fyne.TextAlignCenter, fyne.TextStyle{Monospace: true})

	notice := widget.NewLabelWithStyle(T("perm_enrolled_notice"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	closeBtn := widget.NewButton(T("close_btn"), func() {
		viewerWindow.Close()
	})
	closeBtn.Importance = widget.HighImportance

	var items []fyne.CanvasObject
	if len(appIconBytes) > 0 {
		logoImg := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIconBytes))
		logoImg.FillMode = canvas.ImageFillContain
		logoImg.SetMinSize(fyne.NewSize(64, 64))
		items = append(items, container.NewCenter(logoImg))
	}
	items = append(items,
		title,
		desc,
		widget.NewSeparator(),
		aliasLbl,
		idLbl,
		codeLbl,
		widget.NewSeparator(),
		notice,
		layout.NewSpacer(),
		container.NewPadded(closeBtn),
	)

	viewerWindow.SetContent(container.NewPadded(container.NewVBox(items...)))
}
