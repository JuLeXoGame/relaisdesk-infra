package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"image/color"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed icon.png
var appIconBytes []byte

var (
	fyneApp                     fyne.App
	mainWindow                  fyne.Window
	sessionToken                string
	sessionLicenseID            string
	sessionLicenseKey           string
	sessionActivation           *ActivationResponse
	sessionNetworkAuthorization *NetworkAuthorization
	pendingFleetFolder          string
)

// relaisDeskTheme provides a sober dark-navy and light-blue modern aesthetic
type relaisDeskTheme struct{}

func (t *relaisDeskTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		// Deep midnight navy blue
		return color.NRGBA{R: 11, G: 17, B: 30, A: 255}
	case theme.ColorNameInputBackground:
		// Dark blue input background
		return color.NRGBA{R: 16, G: 25, B: 43, A: 255}
	case theme.ColorNameInputBorder:
		// Subtle light blue input border
		return color.NRGBA{R: 45, G: 80, B: 135, A: 180}
	case theme.ColorNameButton:
		// Slate navy blue button
		return color.NRGBA{R: 24, G: 38, B: 64, A: 255}
	case theme.ColorNamePrimary:
		// RelaisDesk vibrant royal blue
		return color.NRGBA{R: 37, G: 99, B: 235, A: 255}
	case theme.ColorNameHover:
		// Hover effect with light blue glow
		return color.NRGBA{R: 34, G: 54, B: 88, A: 255}
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 45, G: 70, B: 110, A: 160}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 28, G: 44, B: 72, A: 140}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 14, G: 22, B: 38, A: 255}
	case theme.ColorNameForeground:
		// Crisp bright white text
		return color.NRGBA{R: 241, G: 245, B: 249, A: 255}
	case theme.ColorNameSuccess:
		// Vibrant emerald green for online status and success
		return color.NRGBA{R: 34, G: 197, B: 94, A: 255}
	default:
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (t *relaisDeskTheme) Font(s fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(s)
}

func (t *relaisDeskTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (t *relaisDeskTheme) Size(s fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(s)
}

// createCardBox wraps canvas content into a rounded card container with dark blue bg & light blue stroke
func createCardBox(content fyne.CanvasObject, strokeColor color.Color, fillColor ...color.Color) fyne.CanvasObject {
	fill := color.Color(color.NRGBA{R: 16, G: 24, B: 42, A: 255})
	if len(fillColor) > 0 && fillColor[0] != nil {
		fill = fillColor[0]
	}
	rect := canvas.NewRectangle(fill)
	rect.CornerRadius = 7
	rect.StrokeWidth = 1
	rect.StrokeColor = strokeColor

	return container.NewStack(rect, container.NewPadded(content))
}

const (
	svgWindows    = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path fill="#38bdf8" d="M0 2.277L6.685 1.36v6.331H0V2.277zm7.531-1.464L16 0v7.691H7.531V.813zM0 8.441h6.685v6.33L0 13.856V8.441zm7.531 0H16V16l-8.469-.813V8.441z"/></svg>`
	svgApple      = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 170 170"><path fill="#f8fafc" d="M150.37 130.25c-2.45 5.66-5.35 10.87-8.71 15.66-4.58 6.53-8.33 11.05-11.22 13.56-4.48 4.12-9.28 6.23-14.42 6.35-3.69 0-8.14-1.05-13.32-3.18-5.19-2.12-9.97-3.17-14.34-3.17-4.58 0-9.49 1.05-14.75 3.17-5.26 2.13-9.5 3.24-12.74 3.35-4.35.13-9.16-1.9-14.42-6.08-3.7-3.04-7.6-7.79-11.7-14.25-5.78-9.13-10.27-19.14-13.48-30.04-3.21-10.9-4.82-21.37-4.82-31.4 0-12.18 2.87-22.68 8.62-31.5 5.75-8.83 13.3-13.36 22.65-13.6 4.69 0 10.02 1.25 16 3.75 5.98 2.5 10.08 3.81 12.3 3.93 1.98-.12 6.24-1.49 12.77-4.12 6.54-2.62 12.12-3.81 16.75-3.56 12.78.62 22.84 5.25 30.18 13.9-11.23 6.88-16.66 16.27-16.28 28.18.38 9.5 4.13 17.43 11.24 23.8 7.12 6.37 15.54 10.08 25.26 11.13-2.12 6.62-4.63 13.12-7.53 19.5zM119.22 31.84c0-7.38 2.63-14.38 7.88-21 5.26-6.63 11.76-10.63 19.51-12 0 .99.04 1.88.13 2.68 0 7.12-2.73 14.12-8.2 21-5.46 6.87-12.01 10.63-19.64 11.25-.13-.63-.26-1.25-.38-1.93z"/></svg>`
	svgLinux      = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="6" r="4" fill="#fbbf24"/><path fill="#fbbf24" d="M6 14c0 3 2 6 6 6s6-3 6-6c0-2-1-4-3-5H9c-2 1-3 3-3 5z"/><ellipse cx="12" cy="15" rx="3.5" ry="4" fill="#f8fafc"/><circle cx="10.5" cy="5.5" r="1" fill="#0f172a"/><circle cx="13.5" cy="5.5" r="1" fill="#0f172a"/><polygon points="11,7 13,7 12,9" fill="#f97316"/><ellipse cx="9" cy="21" rx="2" ry="1" fill="#f97316"/><ellipse cx="15" cy="21" rx="2" ry="1" fill="#f97316"/></svg>`
	svgGeneric    = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#94a3b8" d="M4 3h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zm0 2v10h16V5H4zm4 14h8v2H8v-2z"/></svg>`
	svgOnlineDot  = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5" fill="#22c55e"/></svg>`
	svgOfflineDot = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5" fill="#64748b"/></svg>`
)

var (
	resWindows    = fyne.NewStaticResource("os_win.svg", []byte(svgWindows))
	resApple      = fyne.NewStaticResource("os_apple.svg", []byte(svgApple))
	resLinux      = fyne.NewStaticResource("os_linux.svg", []byte(svgLinux))
	resGeneric    = fyne.NewStaticResource("os_generic.svg", []byte(svgGeneric))
	resOnlineDot  = fyne.NewStaticResource("dot_online.svg", []byte(svgOnlineDot))
	resOfflineDot = fyne.NewStaticResource("dot_offline.svg", []byte(svgOfflineDot))
)

func createOSBadge(osName string) fyne.CanvasObject {
	osLower := strings.ToLower(osName)
	var res fyne.Resource
	labelStr := "Poste"
	switch {
	case strings.Contains(osLower, "win"):
		res = resWindows
		labelStr = "Windows"
	case strings.Contains(osLower, "darwin") || strings.Contains(osLower, "mac") || strings.Contains(osLower, "apple") || strings.Contains(osLower, "osx"):
		res = resApple
		labelStr = "macOS"
	case strings.Contains(osLower, "linux"):
		res = resLinux
		labelStr = "Linux"
	default:
		res = resGeneric
		if osName != "" {
			labelStr = osName
		}
	}

	icon := canvas.NewImageFromResource(res)
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(15, 15))

	lbl := widget.NewLabel(labelStr)

	return container.NewHBox(icon, lbl)
}

func createStatusDot(online bool) fyne.CanvasObject {
	res := resOfflineDot
	if online {
		res = resOnlineDot
	}
	dot := canvas.NewImageFromResource(res)
	dot.FillMode = canvas.ImageFillContain
	dot.SetMinSize(fyne.NewSize(10, 10))
	return dot
}

func RunGUI() error {
	fyneApp = app.NewWithID("com.relaisdesk.technician")
	fyneApp.Settings().SetTheme(&relaisDeskTheme{})

	// Set application & taskbar icon
	if len(appIconBytes) > 0 {
		appIcon := fyne.NewStaticResource("icon.png", appIconBytes)
		fyneApp.SetIcon(appIcon)
	}

	mainWindow = fyneApp.NewWindow(T("app_title"))
	mainWindow.Resize(fyne.NewSize(1020, 640))
	mainWindow.CenterOnScreen()

	if len(appIconBytes) > 0 {
		appIcon := fyne.NewStaticResource("icon.png", appIconBytes)
		mainWindow.SetIcon(appIcon)
	}

	mainWindow.SetCloseIntercept(func() {
		if dashboardCancel != nil {
			close(dashboardCancel)
			dashboardCancel = nil
		}
		cleanupRustDesk2Toml()
		removeNetworkToken(sessionNetworkAuthorization)
		terminateRustDesk()
		mainWindow.Close()
	})

	// Try to auto-login
	if saved, err := LoadCredentials(); err == nil && saved != nil {
		var ident, secret string
		if saved.Email != "" && saved.Password != "" {
			ident = saved.Email
			secret = saved.Password
		} else if saved.LicenseID != "" && saved.LicenseKey != "" {
			ident = saved.LicenseID
			secret = saved.LicenseKey
		}

		if ident != "" && secret != "" {
			sessionLicenseID = saved.LicenseID
			sessionLicenseKey = saved.LicenseKey

			// Background validation to show loading
			showLoadingScreen(T("connecting"))
			go func() {
				loginResp, err := loginTechnician(ident, secret, "", saved.DeviceToken)
				if err != nil {
					// Failed login, maybe expired. Clear license and show login screen.
					ClearLicense()
					fyne.Do(func() { showLoginScreen("") })
					return
				}

				if loginResp.LicenseID != "" {
					sessionLicenseID = loginResp.LicenseID
				}

				if loginResp.Requires2FA {
					fyne.Do(func() {
						show2FAScreen(loginResp.ChallengeToken, ident, secret)
					})
					return
				}

				sessionToken = loginResp.Token
				if strings.Contains(ident, "@") {
					silentEmail, silentSecret, silentDev := ident, secret, saved.DeviceToken
					go openCustomerSessionSilent(silentEmail, silentSecret, silentDev)
				}
				fyne.Do(func() {
					showSetupScreen(loginResp, func(setupErr error) {
						fyne.Do(func() {
							if setupErr != nil {
								dialog.ShowError(setupErr, mainWindow)
								showLoginScreen("")
								return
							}
							showDashboardScreen(loginResp.Email, loginResp.ExpiresAt)
						})
					})
				})
			}()
		} else {
			showLoginScreen("")
		}
	} else {
		showLoginScreen("")
	}

	if pendingFleetError != "" {
		msg := pendingFleetError
		pendingFleetError = ""
		dialog.ShowError(errors.New(msg), mainWindow)
	}

	startSelfUpdateAtStartup(mainWindow)
	mainWindow.ShowAndRun()
	return nil
}

func showLoadingScreen(message string) {
	label := widget.NewLabelWithStyle(message, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	content := container.NewVBox(
		layout.NewSpacer(),
		label,
		widget.NewProgressBarInfinite(),
		layout.NewSpacer(),
	)
	mainWindow.SetContent(container.NewPadded(content))
}

func showLoginScreen(errorMsg string) {
	mainWindow.SetTitle(T("app_title"))
	title := widget.NewLabelWithStyle(T("app_title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	subTitle := widget.NewLabelWithStyle(T("login_subtitle"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder(T("email_placeholder"))

	pwdEntry := widget.NewPasswordEntry()
	pwdEntry.SetPlaceHolder(T("password_placeholder"))

	statusLabel := widget.NewLabelWithStyle(errorMsg, fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	if errorMsg != "" {
		statusLabel.Importance = widget.DangerImportance
	}

	var loginBtn *widget.Button
	loginBtn = widget.NewButton(T("login_btn"), func() {
		ident := strings.TrimSpace(emailEntry.Text)
		pwd := strings.TrimSpace(pwdEntry.Text)

		if ident == "" || pwd == "" {
			statusLabel.SetText(T("login_err_empty"))
			statusLabel.Importance = widget.DangerImportance
			return
		}

		loginBtn.Disable()
		statusLabel.SetText(T("login_checking"))
		statusLabel.Importance = widget.MediumImportance

		go func() {
			resp, err := loginTechnician(ident, pwd)
			if err != nil {
				fyne.Do(func() {
					statusLabel.SetText(fmt.Sprintf("❌ %s: %v", T("error"), err))
					statusLabel.Importance = widget.DangerImportance
					loginBtn.Enable()
				})
				return
			}

			if resp.Requires2FA {
				fyne.Do(func() {
					show2FAScreen(resp.ChallengeToken, ident, pwd)
				})
				return
			}

			if strings.Contains(ident, "@") {
				silentEmail, silentPwd := ident, pwd
				go openCustomerSessionSilent(silentEmail, silentPwd, savedTechnicianDeviceToken())
			}

			if strings.Contains(ident, "@") {
				_ = SaveCredentials(ident, pwd)
			} else {
				_ = SaveLicense(ident, pwd)
			}

			// Call setup RustDesk on first login
			fyne.Do(func() {
				if resp.LicenseID != "" {
					sessionLicenseID = resp.LicenseID
				} else {
					sessionLicenseID = ident
				}
				sessionLicenseKey = pwd
				sessionToken = resp.Token
				showSetupScreen(resp, func(setupErr error) {
					fyne.Do(func() {
						if setupErr != nil {
							dialog.ShowError(fmt.Errorf("%s:\n%v", T("error"), setupErr), mainWindow)
							showLoginScreen("")
							return
						}
						showDashboardScreen(resp.Email, resp.ExpiresAt)
					})
				})
			})
		}()
	})
	loginBtn.Importance = widget.HighImportance

	pwdEntry.OnSubmitted = func(_ string) {
		loginBtn.OnTapped()
	}
	emailEntry.OnSubmitted = func(_ string) {
		if strings.TrimSpace(pwdEntry.Text) != "" {
			loginBtn.OnTapped()
		}
	}

	forgotPwdBtn := widget.NewButton(T("forgot_password_btn"), func() {
		u, err := url.Parse("https://relaisdesk.fr/client/")
		if err == nil {
			_ = fyne.CurrentApp().OpenURL(u)
		}
	})

	langBtn := widget.NewButton(T("lang_btn"), func() {
		ToggleLang()
		mainWindow.SetTitle(T("app_title"))
		showLoginScreen(errorMsg)
	})

	topBar := container.NewHBox(layout.NewSpacer(), langBtn)

	form := container.NewVBox(
		widget.NewLabelWithStyle(T("email_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		emailEntry,
		widget.NewLabelWithStyle(T("password_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pwdEntry,
		container.NewHBox(layout.NewSpacer(), forgotPwdBtn),
	)

	card := widget.NewCard("", "", form)
	items := []fyne.CanvasObject{topBar}
	if len(appIconBytes) > 0 {
		logoImg := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIconBytes))
		logoImg.FillMode = canvas.ImageFillContain
		logoImg.SetMinSize(fyne.NewSize(72, 72))
		items = append(items, container.NewCenter(logoImg))
	}

	var googleLoginBtn *widget.Button
	googleLoginBtn = widget.NewButton(T("google_login_btn"), func() {
		googleLoginBtn.Disable()
		loginBtn.Disable()
		statusLabel.SetText(T("google_waiting"))
		statusLabel.Importance = widget.MediumImportance

		go func() {
			savedCreds, _ := LoadCredentials()
			deviceToken := ""
			if savedCreds != nil {
				deviceToken = savedCreds.DeviceToken
			}

			resp, credential, err := performGoogleOAuthFlowCredential(context.Background(), deviceToken)
			if err != nil {
				fyne.Do(func() {
					statusLabel.SetText(fmt.Sprintf("❌ %s: %v", T("error"), err))
					statusLabel.Importance = widget.DangerImportance
					googleLoginBtn.Enable()
					loginBtn.Enable()
				})
				return
			}

			if resp.Requires2FA {
				fyne.Do(func() {
					show2FAScreen(resp.ChallengeToken, resp.Email, "")
				})
				return
			}

			if credential != "" {
				go openCustomerSessionSilentGoogle(credential)
			}

			if resp.Email != "" && resp.DeviceToken != "" {
				_ = SaveCredentials(resp.Email, "", resp.DeviceToken)
			}

			fyne.Do(func() {
				if resp.LicenseID != "" {
					sessionLicenseID = resp.LicenseID
				}
				sessionToken = resp.Token
				showSetupScreen(resp, func(setupErr error) {
					fyne.Do(func() {
						if setupErr != nil {
							dialog.ShowError(fmt.Errorf("%s:\n%v", T("error"), setupErr), mainWindow)
							showLoginScreen("")
							return
						}
						showDashboardScreen(resp.Email, resp.ExpiresAt)
					})
				})
			})
		}()
	})

	orLabel := widget.NewLabelWithStyle(T("or_separator"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	items = append(items,
		title,
		subTitle,
		container.NewPadded(card),
		container.NewPadded(loginBtn),
		container.NewCenter(orLabel),
		container.NewPadded(googleLoginBtn),
		statusLabel,
	)

	content := container.NewCenter(container.NewVBox(items...))

	mainWindow.SetContent(container.NewPadded(content))
}

func show2FAScreen(challengeToken, ident, secret string) {
	mainWindow.SetTitle(T("totp_title") + " — " + PRODUCT_NAME)
	title := widget.NewLabelWithStyle(T("totp_title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	subTitle := widget.NewLabelWithStyle(T("totp_subtitle"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	codeEntry := widget.NewEntry()
	codeEntry.SetPlaceHolder(T("totp_code_placeholder"))

	statusLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	emailStatusLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	rememberCheck := widget.NewCheck(T("totp_remember_device"), nil)
	rememberCheck.SetChecked(true)

	var sendEmailBtn *widget.Button
	sendEmailBtn = widget.NewButton(T("totp_send_email_btn"), func() {
		sendEmailBtn.Disable()
		emailStatusLabel.SetText(T("totp_email_sending"))
		emailStatusLabel.Importance = widget.MediumImportance

		go func() {
			masked, err := requestTechnician2FAEmailCode(challengeToken)
			if err != nil {
				fyne.Do(func() {
					emailStatusLabel.SetText(fmt.Sprintf("❌ %v", err))
					emailStatusLabel.Importance = widget.DangerImportance
					sendEmailBtn.Enable()
				})
				return
			}

			fyne.Do(func() {
				emailStatusLabel.SetText(TF("totp_email_sent_msg", masked))
				emailStatusLabel.Importance = widget.SuccessImportance
			})

			// 30 seconds cooldown countdown
			for i := 30; i > 0; i-- {
				time.Sleep(1 * time.Second)
				rem := i
				fyne.Do(func() {
					sendEmailBtn.SetText(TF("totp_email_wait", rem))
				})
			}
			fyne.Do(func() {
				sendEmailBtn.SetText(T("totp_send_email_btn"))
				sendEmailBtn.Enable()
			})
		}()
	})
	sendEmailBtn.Importance = widget.MediumImportance

	var verifyBtn *widget.Button
	verifyBtn = widget.NewButton(T("totp_verify_btn"), func() {
		code := strings.TrimSpace(codeEntry.Text)
		if code == "" {
			statusLabel.SetText(T("totp_err_empty"))
			statusLabel.Importance = widget.DangerImportance
			return
		}

		verifyBtn.Disable()
		statusLabel.SetText(T("totp_checking"))
		statusLabel.Importance = widget.MediumImportance

		go func() {
			rememberDevice := rememberCheck.Checked
			resp, err := loginTechnician2FA(challengeToken, code, rememberDevice)
			if err != nil {
				fyne.Do(func() {
					statusLabel.SetText(fmt.Sprintf("❌ %v", err))
					statusLabel.Importance = widget.DangerImportance
					verifyBtn.Enable()
				})
				return
			}

			devToken := resp.DeviceToken
			if strings.Contains(ident, "@") {
				_ = SaveCredentials(ident, secret, devToken)
			} else {
				_ = SaveLicense(ident, secret, devToken)
			}
			sessionToken = resp.Token
			if resp.LicenseID != "" {
				sessionLicenseID = resp.LicenseID
			} else {
				sessionLicenseID = ident
			}
			sessionLicenseKey = secret
			if strings.Contains(ident, "@") && secret != "" {
				silentEmail, silentSecret, silentDev := ident, secret, devToken
				go openCustomerSessionSilent(silentEmail, silentSecret, silentDev)
			}

			fyne.Do(func() {
				showSetupScreen(resp, func(setupErr error) {
					fyne.Do(func() {
						if setupErr != nil {
							dialog.ShowError(setupErr, mainWindow)
							showLoginScreen("")
							return
						}
						showDashboardScreen(resp.Email, resp.ExpiresAt)
					})
				})
			})
		}()
	})
	verifyBtn.Importance = widget.HighImportance

	codeEntry.OnSubmitted = func(_ string) {
		verifyBtn.OnTapped()
	}

	backBtn := widget.NewButton(T("totp_back_btn"), func() {
		showLoginScreen("")
	})

	langBtn := widget.NewButton(T("lang_btn"), func() {
		ToggleLang()
		show2FAScreen(challengeToken, ident, secret)
	})
	topBar := container.NewHBox(layout.NewSpacer(), langBtn)

	orDivider := widget.NewLabelWithStyle(T("totp_or_divider"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	form := container.NewVBox(
		container.NewPadded(sendEmailBtn),
		emailStatusLabel,
		orDivider,
		widget.NewLabelWithStyle(T("totp_code_placeholder"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		codeEntry,
		container.NewPadded(rememberCheck),
	)
	card := widget.NewCard("", "", form)

	items := []fyne.CanvasObject{topBar}
	if len(appIconBytes) > 0 {
		logoImg := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIconBytes))
		logoImg.FillMode = canvas.ImageFillContain
		logoImg.SetMinSize(fyne.NewSize(72, 72))
		items = append(items, container.NewCenter(logoImg))
	}
	items = append(items,
		title,
		subTitle,
		container.NewPadded(card),
		container.NewPadded(verifyBtn),
		container.NewPadded(backBtn),
		statusLabel,
	)

	content := container.NewCenter(container.NewVBox(items...))
	mainWindow.SetContent(container.NewPadded(content))
}

// showSetupScreen runs the RustDesk setup
func showSetupScreen(loginResp *TechnicianLoginResponse, onComplete func(error)) {
	label := widget.NewLabelWithStyle(T("setup_rustdesk"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	statusLabel := widget.NewLabel(T("setup_init"))

	content := container.NewVBox(
		layout.NewSpacer(),
		label,
		widget.NewProgressBarInfinite(),
		statusLabel,
		layout.NewSpacer(),
	)
	mainWindow.SetContent(container.NewPadded(content))

	go func() {
		activation := &ActivationResponse{
			LicenseID:      sessionLicenseID,
			ServerIP:       loginResp.ServerIP,
			RendezvousPort: loginResp.RendezvousPort,
			RelayPort:      loginResp.RelayPort,
			PublicKey:      loginResp.PublicKey,
		}
		sessionActivation = activation

		fyne.Do(func() { statusLabel.SetText(T("setup_prep")) })
		rustdeskPath, err := ensureRustDesk()
		if err != nil {
			onComplete(fmt.Errorf("erreur preparation RustDesk: %w", err))
			return
		}

		fyne.Do(func() { statusLabel.SetText(T("setup_auth")) })
		networkAuthorization, err := prepareTechnicianNetworkAuthorization(sessionToken)
		if err != nil {
			onComplete(fmt.Errorf("autorisation reseau: %w", err))
			return
		}
		setupFailed := func(err error) {
			removeNetworkToken(networkAuthorization)
			cleanupRustDesk2Toml()
			sessionNetworkAuthorization = nil
			onComplete(err)
		}
		activation.NetworkTokenFile = networkAuthorization.TokenFile
		activation.NetworkProofKeyFile = networkAuthorization.ProofKeyFile
		sessionNetworkAuthorization = networkAuthorization

		fyne.Do(func() { statusLabel.SetText(T("setup_servers")) })
		if err := configureRustDesk(activation, sessionLicenseKey); err != nil {
			setupFailed(err)
			return
		}

		fyne.Do(func() { statusLabel.SetText(T("setup_shortcut")) })
		createDesktopShortcut(rustdeskPath)

		fyne.Do(func() { statusLabel.SetText(T("setup_launch")) })
		if err := restartRustDeskForConfig(rustdeskPath); err != nil {
			setupFailed(fmt.Errorf("lancement echoue: %w", err))
			return
		}

		onComplete(nil)
	}()
}

var (
	dashboardCancel chan struct{}
)

func showDashboardScreen(email, expiresAt string, targetTab ...int) {
	showDashboardScreenInner(email, expiresAt, false, targetTab...)
}

// showDashboardScreenSilent reconstruit le tableau de bord sans écran de
// chargement (l'ancien contenu reste affiché pendant le rafraîchissement).
func showDashboardScreenSilent(email, expiresAt string, targetTab ...int) {
	showDashboardScreenInner(email, expiresAt, true, targetTab...)
}

func showDashboardScreenInner(email, expiresAt string, silent bool, targetTab ...int) {
	if dashboardCancel != nil {
		close(dashboardCancel)
		dashboardCancel = nil
	}

	if !silent {
		showLoadingScreen(T("loading_dashboard"))
	}

	go func() {
		token := sessionToken
		dashResp, err := getDashboard(token)
		if err != nil {
			cleanupRustDesk2Toml()
			fyne.Do(func() {
				dialog.ShowError(err, mainWindow)
				showLoginScreen("")
			})
			return
		}
		devicesResp, deviceErr := listTechnicianDevices(token)
		foldersResp, _ := listTechnicianFolders(token)
		services, _ := serviceCatalog(token)

		fyne.Do(func() {
			mainWindow.SetTitle(T("app_title"))
			sessionServicesEnabled = services != nil && (services.Enabled || len(services.Work) > 0)
			if deviceErr != nil {
				dialog.ShowError(fmt.Errorf("Chargement du parc : %w", deviceErr), mainWindow)
			}
			if pendingFleetDevice != "" {
				id := pendingFleetDevice
				pendingFleetDevice = ""
				requestFleetConnection(id)
			}

			selectedPanel := PanelCodes
			if len(targetTab) > 0 && targetTab[0] >= PanelOverview && targetTab[0] <= PanelSettings {
				selectedPanel = targetTab[0]
			}
			restricted := dashResp.RestrictedToFolders
			if restricted && selectedPanel == PanelCodes {
				selectedPanel = PanelFleet
			}
			relaunch := func(panelID int) {
				showDashboardScreen(email, expiresAt, panelID)
			}
			relaunchSilent := func(panelID int) {
				showDashboardScreenSilent(email, expiresAt, panelID)
			}

			// Header Section
			header := widget.NewLabelWithStyle(TF("tech_header", email, expiresAt), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			stats := widget.NewLabel(TF("stats_summary", dashResp.TotalCodes, dashResp.ActiveCodes, dashResp.ExpiredCodes))

			// Language button
			langBtn := widget.NewButton(T("lang_btn"), func() {
				ToggleLang()
				relaunch(selectedPanel)
			})

			var headerLeft fyne.CanvasObject
			if len(appIconBytes) > 0 {
				logoImg := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIconBytes))
				logoImg.FillMode = canvas.ImageFillContain
				logoImg.SetMinSize(fyne.NewSize(32, 32))
				headerLeft = container.NewHBox(logoImg, header)
			} else {
				headerLeft = header
			}

			headerBox := container.NewBorder(nil, nil, headerLeft, langBtn)

			// --- TAB 1: Temporary Assistance Codes (12h) ---
			clientEmailLabel := widget.NewLabelWithStyle(T("client_email_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			clientEmailEntry := widget.NewEntry()
			clientEmailEntry.SetPlaceHolder(T("client_email_placeholder"))

			var generateBtn *widget.Button
			var resultCodeLabel *widget.Label
			resultCodeLabel = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			resultCodeLabel.Importance = widget.SuccessImportance

			generateBtn = widget.NewButton(T("generate_code_btn"), func() {
				generateBtn.Disable()
				clientEmail := strings.TrimSpace(clientEmailEntry.Text)
				curToken := sessionToken

				go func() {
					res, err := generateViewerCode(curToken, clientEmail)
					if err != nil {
						fyne.Do(func() {
							dialog.ShowError(err, mainWindow)
							generateBtn.Enable()
						})
					} else {
						fyne.Do(func() {
							resultCodeLabel.SetText("Code : " + res.Code)
							mainWindow.Clipboard().SetContent(res.Code)
							dialog.ShowInformation(T("success"), TF("code_generated_popup", res.Code), mainWindow)
							showDashboardScreen(email, expiresAt)
						})
						return
					}
				}()
			})
			genCard := createCardBox(
				container.NewVBox(
					widget.NewLabelWithStyle("⚡ "+T("gen_card_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					clientEmailLabel,
					clientEmailEntry,
					container.NewHBox(generateBtn, resultCodeLabel),
				),
				color.NRGBA{R: 45, G: 95, B: 170, A: 160},
				color.NRGBA{R: 14, G: 21, B: 36, A: 255},
			)

			listContainer := NewSmoothVScroll(buildCodesList(dashResp.Codes, email, expiresAt))
			listContainer.SetMinSize(fyne.NewSize(400, 150))

			tab1Content := container.NewBorder(
				container.NewVBox(
					stats,
					widget.NewSeparator(),
					genCard,
					widget.NewSeparator(),
					widget.NewLabelWithStyle(T("recent_codes_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				),
				nil, nil, nil,
				listContainer,
			)

			// --- TAB 2: Permanent Fleet Devices (Unattended Access) ---
			permOnlineCount := 0
			for _, d := range devicesResp {
				if d.Status == "online" {
					permOnlineCount++
				}
			}
			totalPostsLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			onlineLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			offlineLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{})

			greenDot := createStatusDot(true)
			greyDot := createStatusDot(false)

			updateFleetStatsLabels := func(total, online, offline int) {
				if GetLang() == LangEN {
					pluralDev := "devices"
					if total <= 1 {
						pluralDev = "device"
					}
					totalPostsLabel.SetText(fmt.Sprintf("🖥️ %d %s", total, pluralDev))
					onlineLabel.SetText(fmt.Sprintf("%d online", online))
					offlineLabel.SetText(fmt.Sprintf("%d offline", offline))
				} else {
					pluralPost := "postes"
					if total <= 1 {
						pluralPost = "poste"
					}
					totalPostsLabel.SetText(fmt.Sprintf("🖥️ %d %s", total, pluralPost))
					onlineLabel.SetText(fmt.Sprintf("%d en ligne", online))
					offlineLabel.SetText(fmt.Sprintf("%d hors ligne", offline))
				}
			}
			updateFleetStatsLabels(len(devicesResp), permOnlineCount, len(devicesResp)-permOnlineCount)

			permStatsBox := container.NewHBox(
				totalPostsLabel,
				widget.NewLabel("   "),
				greenDot,
				onlineLabel,
				widget.NewLabel("   "),
				greyDot,
				offlineLabel,
			)
			permStatsRow := container.NewHBox(
				layout.NewSpacer(),
				createCardBox(
					permStatsBox,
					color.NRGBA{R: 45, G: 70, B: 110, A: 140},
					color.NRGBA{R: 16, G: 24, B: 42, A: 255},
				),
			)

			aliasEntry := widget.NewEntry()
			aliasEntry.SetPlaceHolder(T("device_alias_placeholder"))

			notesEntry := widget.NewEntry()
			notesEntry.SetPlaceHolder(T("device_notes_placeholder"))

			var permGenerateBtn *widget.Button
			var permResultCodeLabel *widget.Label
			var permCopyCliBtn *widget.Button

			permResultCodeLabel = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			permResultCodeLabel.Importance = widget.SuccessImportance

			permCopyCliBtn = widget.NewButton(T("copy_cli_cmd"), func() {
				rawText := permResultCodeLabel.Text
				code := strings.TrimPrefix(rawText, "Code : ")
				if code != "" {
					cmd := fmt.Sprintf("viewer.exe --enroll %s", code)
					mainWindow.Clipboard().SetContent(cmd)
					dialog.ShowInformation(T("copy_cli_cmd"), T("cli_cmd_copied"), mainWindow)
				}
			})
			permCopyCliBtn.Hide()

			permGenerateBtn = widget.NewButton(T("generate_perm_code_btn"), func() {
				alias := strings.TrimSpace(aliasEntry.Text)
				if alias == "" {
					dialog.ShowInformation(T("error"), T("device_alias_placeholder"), mainWindow)
					return
				}
				notes := strings.TrimSpace(notesEntry.Text)
				permGenerateBtn.Disable()
				curToken := sessionToken

				go func() {
					res, err := generateTechnicianPermanentCode(curToken, alias, notes)
					if err != nil {
						fyne.Do(func() {
							dialog.ShowError(err, mainWindow)
							permGenerateBtn.Enable()
						})
					} else {
						fyne.Do(func() {
							permResultCodeLabel.SetText("Code : " + res.PermanentCode)
							mainWindow.Clipboard().SetContent(res.PermanentCode)
							permCopyCliBtn.Show()
							dialog.ShowInformation(T("success"), TF("perm_code_generated_popup", res.PermanentCode), mainWindow)
							permGenerateBtn.Enable()
						})
					}
				}()
			})
			permGenerateBtn.Importance = widget.HighImportance

			permGenForm := container.NewVBox(
				container.NewGridWithColumns(2,
					container.NewVBox(
						widget.NewLabelWithStyle(T("device_alias_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						aliasEntry,
					),
					container.NewVBox(
						widget.NewLabelWithStyle(T("device_notes_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						notesEntry,
					),
				),
				container.NewHBox(permGenerateBtn, permResultCodeLabel, permCopyCliBtn),
			)
			permGenAccordion := widget.NewAccordion(
				widget.NewAccordionItem("➕ "+T("perm_gen_card_title"), permGenForm),
			)

			parkLabelEntry := widget.NewEntry()
			parkLabelEntry.SetPlaceHolder(T("park_label_placeholder"))
			parkMaxEntry := widget.NewEntry()
			parkMaxEntry.SetPlaceHolder("100")
			parkMaxEntry.SetText("100")
			parkTTLEntry := widget.NewEntry()
			parkTTLEntry.SetPlaceHolder("30")
			parkTTLEntry.SetText("30")

			var parkCreateBtn *widget.Button
			var parkResultLabel *widget.Label
			var parkCopyCmdBtn *widget.Button
			parkListBox := container.NewVBox()
			parkResultLabel = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			parkResultLabel.Importance = widget.SuccessImportance
			parkResultLabel.Wrapping = fyne.TextWrapWord

			parkCopyCmdBtn = widget.NewButton(T("park_copy_setup_cmd"), func() {
				code := strings.TrimPrefix(parkResultLabel.Text, "Token : ")
				if code != "" {
					mainWindow.Clipboard().SetContent("$env:RELAISDESK_ENROLL_CODE='" + code + "'; $env:RELAISDESK_ENROLL_PASSWORD='<mdp>'; .\\RelaisDesk_Setup.exe /S")
					dialog.ShowInformation(T("park_copy_setup_cmd"), T("park_setup_cmd_copied"), mainWindow)
				}
			})
			parkCopyCmdBtn.Hide()

			var refreshParkList func()
			refreshParkList = func() {
				curToken := sessionToken
				go func() {
					toks, err := listTechnicianParkTokens(curToken)
					fyne.Do(func() {
						parkListBox.Objects = nil
						if err != nil {
							parkListBox.Objects = []fyne.CanvasObject{widget.NewLabelWithStyle(err.Error(), fyne.TextAlignLeading, fyne.TextStyle{Italic: true})}
						} else if len(toks) == 0 {
							parkListBox.Objects = []fyne.CanvasObject{widget.NewLabel(T("park_empty"))}
						}
						for _, tok := range toks {
							tok := tok
							state := TF("park_uses", tok.UseCount, tok.MaxUses)
							if !tok.IsActive {
								state += " (" + T("park_revoked_state") + ")"
							}
							row := container.NewHBox(
								widget.NewLabelWithStyle(tok.Prefix+"…  "+tok.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
								widget.NewLabel(state),
								layout.NewSpacer(),
							)
							if tok.IsActive {
								row.Add(widget.NewButton(T("park_revoke_btn"), func() {
									dialog.ShowConfirm(T("park_revoke_btn"), T("park_revoke_confirm"), func(ok bool) {
										if !ok {
											return
										}
										go func() {
											if err := revokeTechnicianParkToken(sessionToken, tok.ID); err != nil {
												fyne.Do(func() { dialog.ShowError(err, mainWindow) })
												return
											}
											fyne.Do(refreshParkList)
										}()
									}, mainWindow)
								}))
							}
							parkListBox.Objects = append(parkListBox.Objects, createCardBox(
								row,
								color.Color(color.NRGBA{R: 50, G: 120, B: 200, A: 220}),
								color.Color(color.NRGBA{R: 15, G: 22, B: 38, A: 255}),
							))
						}
						parkListBox.Refresh()
					})
				}()
			}

			parkCreateBtn = widget.NewButton(T("park_create_btn"), func() {
				maxUses, err := strconv.Atoi(strings.TrimSpace(parkMaxEntry.Text))
				if err != nil || maxUses <= 0 {
					dialog.ShowError(errors.New(T("park_max_uses_label")), mainWindow)
					return
				}
				ttlDays, err := strconv.Atoi(strings.TrimSpace(parkTTLEntry.Text))
				if err != nil || ttlDays <= 0 {
					dialog.ShowError(errors.New(T("park_ttl_label")), mainWindow)
					return
				}
				parkCreateBtn.Disable()
				curToken := sessionToken
				label := strings.TrimSpace(parkLabelEntry.Text)
				go func() {
					res, err := createTechnicianParkToken(curToken, label, "", maxUses, ttlDays)
					if err != nil {
						fyne.Do(func() {
							dialog.ShowError(err, mainWindow)
							parkCreateBtn.Enable()
						})
					} else {
						fyne.Do(func() {
							parkResultLabel.SetText("Token : " + res.Token)
							mainWindow.Clipboard().SetContent(res.Token)
							parkCopyCmdBtn.Show()
							dialog.ShowInformation(T("success"), TF("park_created_popup", res.Token), mainWindow)
							parkCreateBtn.Enable()
							refreshParkList()
						})
					}
				}()
			})
			parkCreateBtn.Importance = widget.HighImportance

			parkGenForm := container.NewVBox(
				container.NewGridWithColumns(3,
					container.NewVBox(
						widget.NewLabelWithStyle(T("park_label_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						parkLabelEntry,
					),
					container.NewVBox(
						widget.NewLabelWithStyle(T("park_max_uses_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						parkMaxEntry,
					),
					container.NewVBox(
						widget.NewLabelWithStyle(T("park_ttl_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						parkTTLEntry,
					),
				),
				container.NewHBox(parkCreateBtn, parkCopyCmdBtn),
				parkResultLabel,
				widget.NewSeparator(),
				container.NewHBox(
					widget.NewLabelWithStyle(T("park_list_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewButton(T("park_refresh_btn"), refreshParkList),
				),
				parkListBox,
			)
			parkGenAccordion := widget.NewAccordion(
				widget.NewAccordionItem("➕ "+T("park_gen_card_title"), parkGenForm),
			)
			refreshParkList()

			permListBox := container.NewVBox(buildDevicesObjects(devicesResp, foldersResp, email, expiresAt, dashResp.RestrictedToFolders)...)
			permListContainer := NewSmoothVScroll(permListBox)
			permListContainer.SetMinSize(fyne.NewSize(400, 150))

			// Live search & folder controls
			currentFolderID := ""
			if pendingFleetFolder != "" {
				currentFolderID = pendingFleetFolder
				pendingFleetFolder = ""
			}
			searchQuery := ""

			searchEntry := widget.NewEntry()
			searchEntry.SetPlaceHolder(T("search_devices_placeholder"))

			openCreateFolderDialog := func(defaultParentID string) {
				nameEntry := widget.NewEntry()
				nameEntry.SetPlaceHolder("Ex: Agence Lyon, Comptabilité...")

				pChoices := buildFolderOptions(foldersResp)
				pLabels := make([]string, len(pChoices))
				selectedParentID := ""
				initialIdx := 0
				for i, pc := range pChoices {
					pLabels[i] = pc.label
					if defaultParentID != "" && pc.id == defaultParentID {
						selectedParentID = pc.id
						initialIdx = i
					}
				}

				levelHintLabel := widget.NewLabel("")
				levelHintLabel.TextStyle = fyne.TextStyle{Italic: true}
				updateLevelHint := func(parentID string) {
					if parentID == "" {
						levelHintLabel.SetText(T("folder_level_1"))
					} else {
						ancestors := getFolderAncestors(parentID, foldersResp)
						level := len(ancestors) + 1
						if level == 2 {
							levelHintLabel.SetText(fmt.Sprintf("%s (%s)", T("folder_level_2"), getDeviceFolderName(parentID, foldersResp)))
						} else if level == 3 {
							levelHintLabel.SetText(fmt.Sprintf("%s (%s)", T("folder_level_3"), getFolderBreadcrumbPath(parentID, foldersResp)))
						} else {
							levelHintLabel.SetText(fmt.Sprintf("Niveau %d (%s)", level, getFolderBreadcrumbPath(parentID, foldersResp)))
						}
					}
				}
				updateLevelHint(selectedParentID)

				parentSelect := widget.NewSelect(pLabels, func(s string) {
					for _, pc := range pChoices {
						if pc.label == s {
							selectedParentID = pc.id
							updateLevelHint(selectedParentID)
							break
						}
					}
				})
				if len(pLabels) > 0 {
					parentSelect.SetSelected(pLabels[initialIdx])
				}

				dlgTitle := T("btn_new_folder")
				if defaultParentID != "" {
					parentAncestors := getFolderAncestors(defaultParentID, foldersResp)
					if len(parentAncestors) >= 2 {
						dlgTitle = T("level3_subfolder_hint")
					} else {
						dlgTitle = T("btn_new_subfolder")
					}
				}

				formContent := container.NewVBox(
					widget.NewLabelWithStyle(T("folder_name_prompt"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					nameEntry,
					widget.NewLabelWithStyle(T("parent_folder_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					parentSelect,
					levelHintLabel,
				)

				d := dialog.NewCustomConfirm(dlgTitle, "Créer", "Annuler", formContent, func(ok bool) {
					if !ok {
						return
					}
					name := strings.TrimSpace(nameEntry.Text)
					if name == "" {
						return
					}
					curToken := sessionToken
					go func() {
						_, err := createTechnicianFolder(curToken, name, selectedParentID)
						fyne.Do(func() {
							if err == nil {
								pendingFleetFolder = selectedParentID
							}
							if err != nil {
								dialog.ShowError(err, mainWindow)
							} else {
								showDashboardScreen(email, expiresAt, PanelFleet)
							}
						})
					}()
				}, mainWindow)
				d.Resize(fyne.NewSize(420, 260))
				d.Show()
			}

			newFolderBtn := widget.NewButton(T("btn_new_folder"), func() {
				openCreateFolderDialog("")
			})
			newFolderBtn.Importance = widget.HighImportance

			newSubFolderBtn := widget.NewButton(T("btn_new_subfolder"), func() {
				openCreateFolderDialog(currentFolderID)
			})

			renameFolderBtn := widget.NewButton(T("btn_rename_folder"), func() {
				if currentFolderID == "" {
					dialog.ShowInformation(T("btn_rename_folder"), "Veuillez sélectionner un dossier à renommer.", mainWindow)
					return
				}
				curName := getDeviceFolderName(currentFolderID, foldersResp)
				entryDialog := dialog.NewEntryDialog(T("rename_folder_title"), T("folder_name_prompt"), func(name string) {
					name = strings.TrimSpace(name)
					if name == "" || name == curName {
						return
					}
					curToken := sessionToken
					go func() {
						err := updateTechnicianFolder(curToken, currentFolderID, name)
						fyne.Do(func() {
							if err != nil {
								dialog.ShowError(err, mainWindow)
							} else {
								pendingFleetFolder = currentFolderID
								showDashboardScreen(email, expiresAt, PanelFleet)
							}
						})
					}()
				}, mainWindow)
				entryDialog.SetText(curName)
				entryDialog.Show()
			})

			deleteFolderBtn := widget.NewButton(T("btn_delete_folder"), func() {
				if currentFolderID == "" {
					dialog.ShowInformation(T("btn_delete_folder"), "Veuillez sélectionner un dossier à supprimer.", mainWindow)
					return
				}
				dialog.ShowConfirm(T("delete_folder_title"), T("delete_folder_confirm"), func(ok bool) {
					if !ok {
						return
					}
					curToken := sessionToken
					deletedID := currentFolderID
					go func() {
						err := deleteTechnicianFolder(curToken, deletedID)
						fyne.Do(func() {
							if err != nil {
								dialog.ShowError(err, mainWindow)
								return
							}
							for _, f := range foldersResp {
								if f.FolderID == deletedID {
									pendingFleetFolder = f.ParentFolderID
								}
							}
							showDashboardScreen(email, expiresAt, PanelFleet)
						})
					}()
				}, mainWindow)
			})
			deleteFolderBtn.Importance = widget.DangerImportance

			var openMoveFolderDialog func()
			moveFolderBtn := widget.NewButton(T("btn_move_folder"), func() {
				if currentFolderID == "" {
					dialog.ShowInformation(T("btn_move_folder"), T("move_folder_select_first"), mainWindow)
					return
				}
				openMoveFolderDialog()
			})

			openMoveFolderDialog = func() {
				srcID := currentFolderID
				srcName := getDeviceFolderName(srcID, foldersResp)
				var currentParent string
				for _, f := range foldersResp {
					if f.FolderID == srcID {
						currentParent = f.ParentFolderID
					}
				}
				options := buildFolderMoveOptions(foldersResp, srcID)
				labels := []string{}
				byLabel := map[string]string{}
				preselect := ""
				for _, opt := range options {
					labels = append(labels, opt.label)
					byLabel[opt.label] = opt.id
					if opt.id == currentParent {
						preselect = opt.label
					}
				}
				destSelect := widget.NewSelect(labels, nil)
				if preselect != "" {
					destSelect.SetSelected(preselect)
				} else if len(labels) > 0 {
					destSelect.SetSelected(labels[0])
				}
				content := container.NewVBox(
					widget.NewLabel(TF("move_folder_dialog_sub", srcName)),
					destSelect,
				)
				dialog.ShowCustomConfirm(T("move_folder_title"), T("dialog_confirm_btn"), T("dialog_cancel_btn"), content, func(ok bool) {
					if !ok {
						return
					}
					dstID := byLabel[destSelect.Selected]
					if dstID == currentParent {
						return
					}
					dstName := T("folder_root")
					if dstID != "" {
						dstName = getDeviceFolderName(dstID, foldersResp)
					}
					dialog.ShowConfirm(T("move_folder_title"), TF("move_folder_confirm", srcName, dstName), func(confirmed bool) {
						if !confirmed {
							return
						}
						curToken := sessionToken
						go func() {
							err := moveTechnicianFolder(curToken, srcID, dstID)
							fyne.Do(func() {
								if err != nil {
									dialog.ShowError(err, mainWindow)
									return
								}
								pendingFleetFolder = srcID
								showDashboardScreen(email, expiresAt, PanelFleet)
							})
						}()
					}, mainWindow)
				}, mainWindow)
			}

			filterAndRender := func() {
				filtered := []DeviceItem{}
				for _, d := range filterDevicesByFolder(devicesResp, currentFolderID, foldersResp) {
					if searchQuery != "" {
						fName := strings.ToLower(getFolderBreadcrumbPath(d.FolderID, foldersResp))
						if !strings.Contains(strings.ToLower(d.Alias), searchQuery) &&
							!strings.Contains(strings.ToLower(d.Hostname), searchQuery) &&
							!strings.Contains(strings.ToLower(d.DeviceID), searchQuery) &&
							!strings.Contains(strings.ToLower(d.RustDeskID), searchQuery) &&
							!strings.Contains(strings.ToLower(d.OS), searchQuery) &&
							!strings.Contains(fName, searchQuery) {
							continue
						}
					}
					filtered = append(filtered, d)
				}
				permListBox.Objects = buildDevicesObjects(filtered, foldersResp, email, expiresAt, dashResp.RestrictedToFolders)
				permListBox.Refresh()
				permListContainer.Refresh()
				permListContainer.ScrollToTop()
			}

			searchEntry.OnChanged = func(text string) {
				searchQuery = strings.ToLower(strings.TrimSpace(text))
				filterAndRender()
			}

			countInFolderTree := func(fID string) int {
				if fID == "" {
					return len(devicesResp)
				}
				c := 0
				descendantIDs := getFolderAndDescendantIDs(fID, foldersResp)
				for _, d := range devicesResp {
					if descendantIDs[d.FolderID] {
						c++
					}
				}
				return c
			}

			breadcrumbsBox := container.NewHBox()
			fleetTreeBox := container.NewVBox()

			fleetCollapsed := map[string]bool{}
			for _, id := range strings.Split(fyneApp.Preferences().StringWithFallback("fleet_tree_collapsed", ""), ",") {
				if id != "" {
					fleetCollapsed[id] = true
				}
			}
			saveFleetCollapsed := func() {
				ids := []string{}
				for id := range fleetCollapsed {
					ids = append(ids, id)
				}
				fyneApp.Preferences().SetString("fleet_tree_collapsed", strings.Join(ids, ","))
			}
			collapseKey := func(folderID string) string {
				if folderID == "" {
					return "ROOT"
				}
				return folderID
			}

			var selectFolder func(string)
			var renderFleetTree func()

			updateBreadcrumbs := func() {
				breadcrumbsBox.Objects = nil

				rootBtn := widget.NewButton(T("breadcrumb_root"), func() {
					selectFolder("")
				})
				if currentFolderID == "" {
					rootBtn.Importance = widget.HighImportance
				} else {
					rootBtn.Importance = widget.MediumImportance
				}
				breadcrumbsBox.Add(rootBtn)

				if currentFolderID != "" {
					ancestors := getFolderAncestors(currentFolderID, foldersResp)
					for i, a := range ancestors {
						sep := widget.NewLabelWithStyle("  ➔  ", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
						breadcrumbsBox.Add(sep)
						ancID := a.FolderID
						ancName := a.Name
						isCurrent := (i == len(ancestors)-1)
						if isCurrent {
							curBtn := widget.NewButton("📁 "+ancName, nil)
							curBtn.Importance = widget.HighImportance
							curBtn.Disable()
							breadcrumbsBox.Add(curBtn)
						} else {
							crumbBtn := widget.NewButton("📁 "+ancName, func() {
								selectFolder(ancID)
							})
							crumbBtn.Importance = widget.MediumImportance
							breadcrumbsBox.Add(crumbBtn)
						}
					}
				}
				breadcrumbsBox.Refresh()
			}

			addTreeRow := func(id, label string, icon fyne.Resource, depth int, hasChildren bool) {
				row := container.NewHBox()
				if depth > 0 {
					row.Add(widget.NewLabel(strings.Repeat("  ", depth)))
				}
				if hasChildren {
					glyph := "▾"
					if fleetCollapsed[collapseKey(id)] {
						glyph = "▸"
					}
					toggleID := id
					toggleBtn := widget.NewButton(glyph, func() {
						key := collapseKey(toggleID)
						if fleetCollapsed[key] {
							delete(fleetCollapsed, key)
						} else {
							fleetCollapsed[key] = true
						}
						saveFleetCollapsed()
						renderFleetTree()
					})
					row.Add(toggleBtn)
				} else {
					row.Add(widget.NewLabel("   "))
				}
				navID := id
				navBtn := widget.NewButtonWithIcon(label, icon, func() {
					selectFolder(navID)
				})
				navBtn.Alignment = widget.ButtonAlignLeading
				if currentFolderID == id {
					navBtn.Importance = widget.HighImportance
				} else {
					navBtn.Importance = widget.MediumImportance
				}
				row.Add(navBtn)
				fleetTreeBox.Add(row)
			}

			renderFleetTree = func() {
				fleetTreeBox.Objects = nil
				topFolders := getDirectChildFolders("", foldersResp)
				addTreeRow("", fmt.Sprintf("%s (%d)", T("folder_root"), countInFolderTree("")), fleetTreeIcon("root"), 0, len(topFolders) > 0)
				if !fleetCollapsed[collapseKey("")] {
					var addLevel func(parentID string, depth int)
					addLevel = func(parentID string, depth int) {
						for _, f := range getDirectChildFolders(parentID, foldersResp) {
							children := getDirectChildFolders(f.FolderID, foldersResp)
							addTreeRow(f.FolderID, fmt.Sprintf("%s (%d)", f.Name, countInFolderTree(f.FolderID)), fleetTreeIcon("folder"), depth, len(children) > 0)
							if len(children) > 0 && !fleetCollapsed[collapseKey(f.FolderID)] {
								addLevel(f.FolderID, depth+1)
							}
						}
					}
					addLevel("", 1)
				}
				fleetTreeBox.Refresh()
			}

			selectFolder = func(fID string) {
				currentFolderID = fID
				inFolder := currentFolderID != ""
				if inFolder {
					renameFolderBtn.Show()
					moveFolderBtn.Show()
					deleteFolderBtn.Show()
				} else {
					renameFolderBtn.Hide()
					moveFolderBtn.Hide()
					deleteFolderBtn.Hide()
				}
				updateBreadcrumbs()
				renderFleetTree()
				filterAndRender()
			}

			updateBreadcrumbs()
			renderFleetTree()

			breadcrumbsScroll := NewSmoothHScroll(breadcrumbsBox)
			breadcrumbsScroll.SetMinSize(fyne.NewSize(0, 42))

			folderActions := container.NewHBox(
				newFolderBtn,
				newSubFolderBtn,
				renameFolderBtn,
				moveFolderBtn,
				deleteFolderBtn,
			)
			if dashResp.RestrictedToFolders {
				folderActions.Hide()
				permGenAccordion.Hide()
				parkGenAccordion.Hide()
			}

			breadcrumbsRow := container.NewBorder(
				nil, nil,
				container.NewHBox(
					widget.NewLabelWithStyle("🧭 "+T("folder_filter_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					widget.NewLabel(" "),
				),
				nil,
				breadcrumbsScroll,
			)

			folderBar := container.NewVBox(
				breadcrumbsRow,
				folderActions,
			)

			folderBarCard := createCardBox(
				container.NewVBox(
					searchEntry,
					folderBar,
				),
				color.NRGBA{R: 50, G: 120, B: 200, A: 220},
				color.NRGBA{R: 14, G: 20, B: 35, A: 255},
			)

			fleetTreePane := container.NewBorder(
				container.NewVBox(
					widget.NewLabelWithStyle(T("fleet_tree_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					widget.NewSeparator(),
				),
				nil, nil, nil,
				NewSmoothVScroll(fleetTreeBox),
			)
			fleetSplit := container.NewHSplit(fleetTreePane, permListContainer)
			fleetSplit.Offset = 0.3

			tab2Content := container.NewBorder(
				container.NewVBox(
					permStatsRow,
					permGenAccordion,
					parkGenAccordion,
					folderBarCard,
				),
				nil, nil, nil,
				fleetSplit,
			)

			// Menu latéral miroir du web : les rubriques technicien réutilisent
			// les contenus existants, les autres passent par le compte client.
			var fleetTabVisible atomic.Bool
			contentBox := container.NewMax()
			builtPanels := map[int]fyne.CanvasObject{}
			customerGate := func(panelID int, builder func(int, func(int)) fyne.CanvasObject) fyne.CanvasObject {
				return customerGatePanel(panelID, relaunch, builder)
			}
			navBox := container.NewMax()
			var mainSplit *container.Split
			collapsed := fyneApp.Preferences().BoolWithFallback("sidebar_collapsed", false)
			var renderNav func()
			selectPanel := func(panelID int) {
				selectedPanel = panelID
				fleetTabVisible.Store(panelID == PanelFleet)
				if _, ok := builtPanels[panelID]; !ok {
					switch panelID {
					case PanelCodes:
						builtPanels[panelID] = tab1Content
					case PanelFleet:
						builtPanels[panelID] = tab2Content
					case PanelOverview:
						builtPanels[panelID] = customerGate(panelID, overviewPanel)
					case PanelTeam:
						builtPanels[panelID] = customerGate(panelID, teamPanel)
					case PanelLicenses:
						builtPanels[panelID] = customerGate(panelID, licensesPanel)
					case PanelBilling:
						builtPanels[panelID] = customerGate(panelID, billingPanel)
					case PanelHistory:
						builtPanels[panelID] = customerGate(panelID, historyPanel)
					case PanelServices:
						builtPanels[panelID] = customerGatePanel(panelID, relaunch, func(pid int, rl func(int)) fyne.CanvasObject {
							return servicesPanel(pid, rl, relaunchSilent)
						})
					case PanelSettings:
						builtPanels[panelID] = customerGate(panelID, settingsPanel)
					default:
						builtPanels[panelID] = customerGate(panelID, overviewPanel)
					}
				}
				contentBox.Objects = []fyne.CanvasObject{builtPanels[panelID]}
				contentBox.Refresh()
				renderNav()
			}
			accountFooter := container.NewVBox(
				widget.NewLabelWithStyle(email, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
			)
			renderNav = func() {
				icon := collapseIconResource()
				if collapsed {
					icon = expandIconResource()
				}
				collapseBtn := widget.NewButtonWithIcon("", icon, func() {
					collapsed = toggleSidebarCollapsed(collapsed)
					renderNav()
					if mainSplit != nil {
						mainSplit.Offset = sidebarSplitOffset(collapsed, mainSplit.Size().Width, navBox.MinSize().Width)
						mainSplit.Refresh()
					}
				})
				var footer fyne.CanvasObject = accountFooter
				if collapsed {
					// Le pied de page (e-mail) forcerait la colonne à rester large.
					footer = nil
				}
				nav := buildSidebarNav(selectedPanel, collapsed, restricted, selectPanel, footer)
				navBox.Objects = []fyne.CanvasObject{container.NewBorder(collapseBtn, nil, nil, nil, nav)}
				navBox.Refresh()
			}
			selectPanel(selectedPanel)
			mainSplit = container.NewHSplit(navBox, contentBox)
			mainSplit.Offset = sidebarSplitOffset(collapsed, 0, 0)

			// Polling automatique toutes les 30 secondes pour rafraîchir codes et postes
			currentCancel := make(chan struct{})
			dashboardCancel = currentCancel
			pollToken := sessionToken
			go func(cancel chan struct{}, token string) {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-cancel:
						return
					case <-ticker.C:
						if token == "" {
							return
						}
						latestDash, err1 := getDashboard(token)
						var latestDevs []DeviceItem
						var err2 error
						if fleetTabVisible.Load() {
							latestDevs, err2 = listTechnicianDevices(token)
							latestFolders, _ := listTechnicianFolders(token)
							if latestFolders != nil {
								foldersResp = latestFolders
							}
						}
						if (err1 == nil && latestDash != nil) || (err2 == nil && latestDevs != nil) {
							fyne.Do(func() {
								if latestDash != nil {
									listContainer.Content = buildCodesList(latestDash.Codes, email, expiresAt)
									listContainer.Refresh()
									stats.SetText(TF("stats_summary", latestDash.TotalCodes, latestDash.ActiveCodes, latestDash.ExpiredCodes))
								}
								if latestDevs != nil {
									devicesResp = latestDevs
									filterAndRender()
									online := 0
									for _, d := range latestDevs {
										if d.Status == "online" {
											online++
										}
									}
									updateFleetStatsLabels(len(latestDevs), online, len(latestDevs)-online)
								}
							})
						}
					}
				}
			}(currentCancel, pollToken)

			go func(cancel chan struct{}, token string, authorization *NetworkAuthorization) {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-cancel:
						return
					case <-ticker.C:
						if authorization == nil || time.Until(authorization.ExpiresAt) > 2*time.Minute {
							continue
						}
						if err := refreshTechnicianNetworkAuthorization(token, authorization); err != nil && time.Now().After(authorization.ExpiresAt) {
							removeNetworkToken(authorization)
						}
					}
				}
			}(currentCancel, pollToken, sessionNetworkAuthorization)

			// Footer buttons
			launchBtn := widget.NewButton(T("launch_rustdesk_btn"), func() {
				if isRustDeskRunning() {
					dialog.ShowInformation(T("launch_rustdesk_btn"), T("rustdesk_already_running"), mainWindow)
					return
				}
				if path, err := ensureRustDesk(); err == nil {
					_ = launchRustDesk(path)
				} else {
					dialog.ShowError(fmt.Errorf("RustDesk: %v", err), mainWindow)
				}
			})
			launchBtn.Importance = widget.HighImportance

			refreshBtn := widget.NewButton(T("refresh_btn"), func() {
				relaunch(selectedPanel)
			})

			diagnosticBtn := widget.NewButton(T("diagnostic_btn"), func() {
				activation := sessionActivation
				diagWin := fyneApp.NewWindow(T("diag_title"))
				diagWin.Resize(fyne.NewSize(740, 540))
				diagWin.CenterOnScreen()
				if len(appIconBytes) > 0 {
					diagWin.SetIcon(fyne.NewStaticResource("icon.png", appIconBytes))
				}

				loadingLabel := widget.NewLabelWithStyle(T("diag_analyzing"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
				progress := widget.NewProgressBarInfinite()
				loadingBox := container.NewVBox(
					layout.NewSpacer(),
					loadingLabel,
					container.NewPadded(progress),
					widget.NewLabelWithStyle(T("diag_analyzing_sub"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
					layout.NewSpacer(),
				)
				diagWin.SetContent(container.NewPadded(loadingBox))
				diagWin.Show()

				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
					defer cancel()
					report := runConnectivityDiagnostics(ctx, APIURL, activation)
					reportText := report.String()

					fyne.Do(func() {
						headerTitle := widget.NewLabelWithStyle(T("diag_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
						headerSub := widget.NewLabel(TF("diag_generated", report.CheckedAt.Local().Format("02/01/2006 15:04:05")))

						var statusBanner *widget.Label
						if report.OK() {
							statusBanner = widget.NewLabelWithStyle(T("diag_all_ok"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
							statusBanner.Importance = widget.SuccessImportance
						} else {
							statusBanner = widget.NewLabelWithStyle(T("diag_issues"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
							statusBanner.Importance = widget.DangerImportance
						}

						checksBox := container.NewVBox()
						for _, check := range report.Checks {
							iconStr := "❌"
							if check.OK {
								iconStr = "✅"
							}
							latencyStr := ""
							if check.Latency > 0 {
								latencyStr = fmt.Sprintf(" (%d ms)", check.Latency.Milliseconds())
							}
							titleLbl := widget.NewLabelWithStyle(fmt.Sprintf("%s  %s%s", iconStr, check.Name, latencyStr), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
							if !check.OK {
								titleLbl.Importance = widget.DangerImportance
							}
							msgLbl := widget.NewLabel(fmt.Sprintf("     %s", check.Message))
							checksBox.Add(container.NewVBox(titleLbl, msgLbl))
						}

						privacyNotice := widget.NewLabelWithStyle(T("diag_privacy"), fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

						copyBtn := widget.NewButton(T("diag_copy"), func() {
							diagWin.Clipboard().SetContent(reportText)
							dialog.ShowInformation(T("diag_copy"), T("diag_copy")+" OK", diagWin)
						})

						closeBtn := widget.NewButton(T("close_btn"), func() {
							diagWin.Close()
						})

						footer := container.NewHBox(copyBtn, layout.NewSpacer(), closeBtn)
						scrollContent := NewSmoothVScroll(container.NewVBox(
							statusBanner,
							widget.NewSeparator(),
							checksBox,
							widget.NewSeparator(),
							privacyNotice,
						))
						scrollContent.SetMinSize(fyne.NewSize(700, 360))

						content := container.NewBorder(
							container.NewVBox(headerTitle, headerSub, widget.NewSeparator()),
							container.NewVBox(widget.NewSeparator(), footer),
							nil,
							nil,
							scrollContent,
						)

						diagWin.SetContent(container.NewPadded(content))
					})
				}()
			})

			updateBtn := widget.NewButton(T("updates_btn"), func() {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					info, err := checkForUpdate(ctx, APIURL)
					fyne.Do(func() {
						if err != nil {
							dialog.ShowError(err, mainWindow)
							return
						}
						message := fmt.Sprintf("Vous êtes à jour (version %s)", info.CurrentVersion)
						if GetLang() == LangEN {
							message = fmt.Sprintf("You are up to date (version %s)", info.CurrentVersion)
						}
						if info.Available {
							if GetLang() == LangEN {
								message = fmt.Sprintf("Version %s is available.\n\nVerified download: %s", info.LatestVersion, info.DownloadURL)
							} else {
								message = fmt.Sprintf("La version %s est disponible.\n\nTéléchargement vérifié : %s", info.LatestVersion, info.DownloadURL)
							}
						}
						dialog.ShowInformation(T("updates_btn"), message, mainWindow)
					})
				}()
			})

			logoutBtn := widget.NewButton(T("logout_btn"), func() {
				if dashboardCancel != nil {
					close(dashboardCancel)
					dashboardCancel = nil
				}
				token := sessionToken
				if token != "" {
					go logoutTechnician(token)
				}
				if getCustomerSessionToken() != "" {
					ct := getCustomerSessionToken()
					go func() { _ = customerLogout(ct) }()
				}
				clearCustomerSession()
				cleanupRustDesk2Toml()
				removeNetworkToken(sessionNetworkAuthorization)
				ClearLicense()
				sessionToken = ""
				sessionLicenseID = ""
				sessionLicenseKey = ""
				sessionNetworkAuthorization = nil
				showLoginScreen(T("logout_success"))
			})

			footer := container.NewHBox(launchBtn, refreshBtn, diagnosticBtn, updateBtn, layout.NewSpacer(), logoutBtn)

			content := container.NewBorder(
				headerBox,
				container.NewVBox(widget.NewSeparator(), footer),
				nil, nil,
				mainSplit,
			)

			mainWindow.SetContent(container.NewPadded(content))
		})
	}()
}

func buildDevicesObjects(devices []DeviceItem, folders []DeviceFolderItem, email, expiresAt string, restricted ...bool) []fyne.CanvasObject {
	if len(devices) == 0 {
		return []fyne.CanvasObject{
			container.NewVBox(
				widget.NewLabel(""),
				container.NewCenter(widget.NewLabelWithStyle(T("no_devices_yet"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})),
				widget.NewLabel(""),
			),
		}
	}

	var objs []fyne.CanvasObject

	for _, device := range devices {
		d := device

		titleStr := d.Alias
		if titleStr == "" {
			titleStr = d.Hostname
		}
		if titleStr == "" {
			titleStr = d.DeviceID
		}

		lastSeenStr := "—"
		if d.LastSeenAt != nil {
			lastSeenStr = d.LastSeenAt.Local().Format("02/01/2006 15:04")
		}
		if d.LastIP != "" {
			lastSeenStr += " (" + d.LastIP + ")"
		}

		statusDot := createStatusDot(d.Status == "online")
		titleLabel := widget.NewLabelWithStyle(titleStr, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		if d.Status == "online" {
			titleLabel.Importance = widget.SuccessImportance
		}
		osBadgeWidget := createOSBadge(d.OS)
		headerLeft := container.NewHBox(statusDot, titleLabel, widget.NewLabel("•"), osBadgeWidget)

		folderPath := getFolderBreadcrumbPath(d.FolderID, folders)

		var line1Parts []string
		if d.RustDeskID != "" {
			line1Parts = append(line1Parts, "ID : "+d.RustDeskID)
		}
		if d.Hostname != "" && d.Hostname != titleStr {
			if GetLang() == LangEN {
				line1Parts = append(line1Parts, "Host: "+d.Hostname)
			} else {
				line1Parts = append(line1Parts, "Hôte : "+d.Hostname)
			}
		}
		if folderPath != "" {
			line1Parts = append(line1Parts, "📁 "+folderPath)
		}
		if d.EnrollmentState != "" && d.EnrollmentState != "enrolled" {
			if GetLang() == LangEN {
				line1Parts = append(line1Parts, "⚠️ Enrollment: "+d.EnrollmentState)
			} else {
				line1Parts = append(line1Parts, "⚠️ Enrôlement : "+d.EnrollmentState)
			}
		}
		detailLine1 := widget.NewLabel(strings.Join(line1Parts, "   •   "))
		detailLine1.Wrapping = fyne.TextWrapWord

		var line2Parts []string
		if d.Status == "online" {
			onlineTxt := "🟢 En ligne"
			if GetLang() == LangEN {
				onlineTxt = "🟢 Online"
			}
			if d.LastIP != "" {
				onlineTxt += " (" + d.LastIP + ")"
			}
			line2Parts = append(line2Parts, onlineTxt)
		} else {
			if lastSeenStr != "—" {
				line2Parts = append(line2Parts, TF("last_seen_prefix", lastSeenStr))
			} else {
				if GetLang() == LangEN {
					line2Parts = append(line2Parts, "Offline")
				} else {
					line2Parts = append(line2Parts, "Hors ligne")
				}
			}
		}
		if d.MACAddress != "" {
			line2Parts = append(line2Parts, "MAC : "+d.MACAddress)
		}
		detailLine2 := widget.NewLabel(strings.Join(line2Parts, "   •   "))
		detailLine2.Wrapping = fyne.TextWrapWord

		var takeControlBtn *widget.Button
		var copyIdBtn *widget.Button

		cleanID := strings.ReplaceAll(d.RustDeskID, " ", "")
		if numericFleetTarget(cleanID) && d.EnrollmentState == "enrolled" {
			devCopy := d
			takeControlBtn = widget.NewButton("Connexion directe", func() {
				requestFleetConnection(devCopy.DeviceID, devCopy)
			})
			takeControlBtn.Importance = widget.HighImportance

			copyIdBtn = widget.NewButton(T("copy_id_btn"), func() {
				mainWindow.Clipboard().SetContent(cleanID)
				dialog.ShowInformation(T("copy_id_btn"), TF("id_copied", cleanID), mainWindow)
			})
		}

		var updateBtn *widget.Button
		if d.EnrollmentState == "enrolled" && IsDeviceUpdateAvailable(d.AgentVersion) {
			devCopy := d
			updateBtn = widget.NewButton("⬆️ "+T("btn_update_device"), func() {
				dialog.ShowConfirm(
					T("btn_update_device"),
					TF("confirm_update_device_msg", titleStr),
					func(confirmed bool) {
						if confirmed {
							curToken := sessionToken
							go func() {
								resp, err := triggerTechnicianDeviceUpdate(curToken, devCopy.DeviceID, APP_VERSION)
								fyne.Do(func() {
									if err != nil {
										dialog.ShowError(err, mainWindow)
									} else {
										msg := resp.Message
										if msg == "" {
											msg = T("update_scheduled_success")
										}
										dialog.ShowInformation(T("btn_update_device"), msg, mainWindow)
									}
								})
							}()
						}
					},
					mainWindow,
				)
			})
		}

		moveBtn := widget.NewButton(T("btn_move_device"), func() {
			moveOpts := buildFolderOptions(folders)

			labels := make([]string, len(moveOpts))
			selectedTargetID := ""
			for i, o := range moveOpts {
				labels[i] = o.label
				if o.id == d.FolderID {
					selectedTargetID = o.id
				}
			}

			sel := widget.NewSelect(labels, nil)
			for i, o := range moveOpts {
				if o.id == d.FolderID {
					sel.SetSelected(labels[i])
					break
				}
			}
			sel.OnChanged = func(s string) {
				for _, o := range moveOpts {
					if o.label == s {
						selectedTargetID = o.id
						break
					}
				}
			}

			aliasEntry := widget.NewEntry()
			aliasEntry.SetText(d.Alias)
			if d.Alias == "" && d.Hostname != "" {
				aliasEntry.SetPlaceHolder(d.Hostname)
			}

			formContent := container.NewVBox(
				widget.NewLabelWithStyle(T("device_alias_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				aliasEntry,
				widget.NewLabelWithStyle(T("device_folder_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				sel,
			)

			customDialog := dialog.NewCustomConfirm(
				T("move_device_title"),
				T("save_btn"),
				T("cancel_btn"),
				formContent,
				func(confirm bool) {
					if confirm {
						newAlias := strings.TrimSpace(aliasEntry.Text)
						curToken := sessionToken
						go func() {
							err := updateTechnicianDevice(curToken, d.DeviceID, newAlias, d.Notes, selectedTargetID)
							fyne.Do(func() {
								if err != nil {
									dialog.ShowError(err, mainWindow)
								} else {
									showDashboardScreen(email, expiresAt, PanelFleet)
								}
							})
						}()
					}
				},
				mainWindow,
			)
			customDialog.Resize(fyne.NewSize(420, 240))
			customDialog.Show()
		})

		deleteBtn := widget.NewButton("❌", func() {
			dialog.ShowConfirm(
				T("confirm_delete_device_title"),
				TF("confirm_delete_device_msg", titleStr, d.DeviceID),
				func(confirm bool) {
					if confirm {
						curToken := sessionToken
						go func() {
							err := deleteTechnicianDevice(curToken, d.DeviceID)
							if err != nil {
								fyne.Do(func() { dialog.ShowError(err, mainWindow) })
							} else {
								fyne.Do(func() {
									dialog.ShowInformation(T("success"), T("delete_device_success"), mainWindow)
									showDashboardScreen(email, expiresAt, PanelFleet)
								})
							}
						}()
					}
				},
				mainWindow,
			)
		})
		deleteBtn.Importance = widget.DangerImportance

		actionButtons := []fyne.CanvasObject{}
		if takeControlBtn != nil {
			actionButtons = append(actionButtons, takeControlBtn)
			if sessionServicesEnabled {
				actionButtons = append(actionButtons, widget.NewButton("Connexion & Prestation", func() { openServiceDialog("device", d.DeviceID) }))
			}
		}
		if copyIdBtn != nil {
			actionButtons = append(actionButtons, copyIdBtn)
		}
		if updateBtn != nil {
			actionButtons = append(actionButtons, updateBtn)
		}
		if len(restricted) == 0 || !restricted[0] {
			actionButtons = append(actionButtons, moveBtn, deleteBtn)
		}
		actions := container.NewHBox(actionButtons...)

		headerRow := container.NewBorder(nil, nil, nil, actions, headerLeft)
		cardBox := container.NewVBox(headerRow, detailLine1)
		if len(line2Parts) > 0 {
			cardBox.Add(detailLine2)
		}

		stroke := color.Color(color.NRGBA{R: 50, G: 120, B: 200, A: 220}) // crisp bright blue, same outline for every card
		fill := color.Color(color.NRGBA{R: 15, G: 22, B: 38, A: 255})
		if d.Status == "online" {
			fill = color.Color(color.NRGBA{R: 17, G: 27, B: 46, A: 255})
		}
		card := createCardBox(cardBox, stroke, fill)
		objs = append(objs, card)
	}

	return objs
}

func buildDevicesList(devices []DeviceItem, folders []DeviceFolderItem, email, expiresAt string) fyne.CanvasObject {
	return container.NewVBox(buildDevicesObjects(devices, folders, email, expiresAt)...)
}

func buildCodesList(codes []CodeStat, email, expiresAt string) fyne.CanvasObject {
	if len(codes) == 0 {
		return widget.NewLabel(T("no_codes_yet"))
	}

	rows := container.NewVBox()

	for _, code := range codes {
		c := code

		statusTxt := T("status_active")
		isRevokedOrExpired := false

		if c.Status == "revoked" || (!c.IsActive && c.Status != "expired") {
			statusTxt = T("status_revoked")
			isRevokedOrExpired = true
		} else if c.Status == "expired" {
			statusTxt = T("status_expired")
			isRevokedOrExpired = true
		}

		clientDisplay := c.ClientEmail
		if clientDisplay == "" {
			clientDisplay = T("anonymous_client")
		}

		infoStr := fmt.Sprintf(
			"Code : %s   •   Client : %s\nStatus : %s   •   Expires : %s",
			c.Code, clientDisplay, statusTxt, c.ExpiresAt,
		)
		if GetLang() == LangFR {
			infoStr = fmt.Sprintf(
				"Code : %s   •   Client : %s\nStatut : %s   •   Expire : %s",
				c.Code, clientDisplay, statusTxt, c.ExpiresAt,
			)
		}
		infoLabel := widget.NewLabel(infoStr)
		infoLabel.Wrapping = fyne.TextWrapWord

		var clientStatusLabel *widget.Label
		var takeControlBtn *widget.Button
		var copyIdBtn *widget.Button

		if c.ClientRustDeskID != "" && !isRevokedOrExpired {
			clientStatusLabel = widget.NewLabelWithStyle(TF("client_ready", c.ClientRustDeskID), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			clientStatusLabel.Importance = widget.SuccessImportance

			cleanID := strings.ReplaceAll(c.ClientRustDeskID, " ", "")
			takeControlBtn = widget.NewButton("Connexion directe", func() {
				if err := serviceDirectAllowed(cleanID); err != nil {
					dialog.ShowError(err, mainWindow)
					return
				}
				token, authorization, code := sessionToken, sessionNetworkAuthorization, c.Code
				go func() {
					if authorization == nil {
						fyne.Do(func() { dialog.ShowError(fmt.Errorf("autorisation réseau absente ; reconnectez-vous"), mainWindow) })
						return
					}
					path, err := ensureRustDesk()
					if err == nil {
						var interventionID string
						interventionID, err = technicianConnectViewerCode(token, code)
						if err == nil {
							err = launchTrackedIntervention(token, authorization.TokenFile, path, cleanID, interventionID)
						}
					}
					if err != nil {
						fyne.Do(func() { dialog.ShowError(fmt.Errorf("Erreur prise en main: %v", err), mainWindow) })
					}
				}()
			})
			takeControlBtn.Importance = widget.HighImportance

			copyIdBtn = widget.NewButton(T("copy_id_btn"), func() {
				mainWindow.Clipboard().SetContent(cleanID)
				dialog.ShowInformation(T("copy_id_btn"), TF("id_copied", cleanID), mainWindow)
			})
		} else if !isRevokedOrExpired {
			clientStatusLabel = widget.NewLabelWithStyle(T("client_waiting"), fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		}

		copyBtn := widget.NewButton(T("copy_btn"), func() {
			mainWindow.Clipboard().SetContent(c.Code)
			dialog.ShowInformation(T("copy_btn"), TF("code_copied", c.Code), mainWindow)
		})

		if isRevokedOrExpired {
			copyBtn.Disable()
		}

		// Delete Cross Button (❌)
		deleteBtn := widget.NewButton("❌", func() {
			if isRevokedOrExpired {
				token := sessionToken
				go func() {
					err := deleteViewerCode(token, c.Code)
					if err != nil {
						fyne.Do(func() { dialog.ShowError(err, mainWindow) })
					} else {
						fyne.Do(func() { showDashboardScreen(email, expiresAt) })
					}
				}()
			} else {
				dialog.ShowConfirm(
					T("confirm_delete_title"),
					TF("confirm_delete_active", clientDisplay, c.Code),
					func(confirm bool) {
						if confirm {
							token := sessionToken
							go func() {
								_ = revokeViewerCode(token, c.Code)
								err := deleteViewerCode(token, c.Code)
								if err != nil {
									fyne.Do(func() { dialog.ShowError(err, mainWindow) })
								} else {
									fyne.Do(func() {
										dialog.ShowInformation(T("success"), T("delete_success"), mainWindow)
										showDashboardScreen(email, expiresAt)
									})
								}
							}()
						}
					},
					mainWindow,
				)
			}
		})
		deleteBtn.Importance = widget.DangerImportance

		actionButtons := []fyne.CanvasObject{}
		if takeControlBtn != nil {
			actionButtons = append(actionButtons, takeControlBtn)
			if sessionServicesEnabled {
				actionButtons = append(actionButtons, widget.NewButton("Connexion & Prestation", func() { openServiceDialog("code", c.Code) }))
			}
		}
		if copyIdBtn != nil {
			actionButtons = append(actionButtons, copyIdBtn)
		}
		actionButtons = append(actionButtons, copyBtn, deleteBtn)
		actions := container.NewHBox(actionButtons...)

		infoBox := container.NewVBox(infoLabel)
		if clientStatusLabel != nil {
			infoBox.Add(clientStatusLabel)
		}

		row := container.NewBorder(nil, nil, nil, actions, infoBox)
		stroke := color.Color(color.NRGBA{R: 50, G: 120, B: 200, A: 220}) // crisp bright blue, same outline for every card
		fill := color.Color(color.NRGBA{R: 16, G: 24, B: 42, A: 255})
		if isRevokedOrExpired {
			fill = color.Color(color.NRGBA{R: 13, G: 19, B: 32, A: 255})
		}
		card := createCardBox(row, stroke, fill)
		rows.Add(card)
	}

	return rows
}
