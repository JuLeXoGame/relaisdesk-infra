package main

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// Version des CGV vente, identique au web (TERMS_VERSION). À aligner si le
// serveur la fait évoluer (mailer.CurrentTermsVersion).
const customerTermsVersion = "2026-09-27"

// asyncFetchPanel charge des données puis construit le contenu, avec gestion
// homogène de l'expiration de session et du réessai.
func asyncFetchPanel(panelID int, relaunch func(int), fetch func(token string) (fyne.CanvasObject, error)) fyne.CanvasObject {
	box := container.NewMax()
	var load func()
	load = func() {
		box.Objects = []fyne.CanvasObject{container.NewCenter(
			widget.NewLabelWithStyle(T("loading_dashboard"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
		)}
		box.Refresh()
		go func() {
			token := getCustomerSessionToken()
			content, err := fetch(token)
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						box.Objects = []fyne.CanvasObject{customerLoginPanel(panelID, relaunch, T("customer_session_expired"))}
						box.Refresh()
						return
					}
					retry := widget.NewButton(T("refresh_btn"), func() { load() })
					box.Objects = []fyne.CanvasObject{container.NewCenter(container.NewVBox(
						widget.NewLabelWithStyle(err.Error(), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
						retry,
					))}
					box.Refresh()
					return
				}
				box.Objects = []fyne.CanvasObject{container.NewVScroll(content)}
				box.Refresh()
			})
		}()
	}
	load()
	return box
}

func saveBytesDialog(data []byte, defaultName string) {
	saveDialog := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		defer w.Close()
		if _, err := w.Write(data); err != nil {
			dialog.ShowError(err, mainWindow)
			return
		}
		dialog.ShowInformation(T("download_saved_title"), TF("download_saved_msg", w.URI().Name()), mainWindow)
	}, mainWindow)
	saveDialog.SetFileName(defaultName)
	saveDialog.Show()
}

func folderNameMap(folders []customerFolder) map[string]string {
	names := map[string]string{}
	for _, f := range folders {
		names[f.FolderID] = f.Name
	}
	return names
}

func sortedFolderOptions(folders []customerFolder) ([]string, map[string]string) {
	byID := map[string]string{}
	labels := []string{}
	for _, f := range folders {
		label := f.Name + "  (" + f.FolderID + ")"
		byID[label] = f.FolderID
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels, byID
}

// teamPanel reproduit la gestion d'équipe web : invitations, membres,
// dossiers autorisés et équipes rejointes.
func teamPanel(panelID int, relaunch func(int)) fyne.CanvasObject {
	return asyncFetchPanel(panelID, relaunch, func(token string) (fyne.CanvasObject, error) {
		team, err := getCustomerTeam(token)
		if err != nil {
			return nil, err
		}
		folders, err := listCustomerFolders(token)
		if err != nil {
			return nil, err
		}
		names := folderNameMap(folders)

		licBox := container.NewVBox()
		licOptions := []string{}
		licByLabel := map[string]string{}
		for _, lic := range team.Licenses {
			licBox.Add(widget.NewLabel(TF("team_license_line", lic.LicenseID, lic.Plan, lic.Used, lic.Capacity)))
			if lic.Enabled && lic.Used < lic.Capacity {
				label := fmt.Sprintf("%s (%s)", lic.LicenseID, lic.Plan)
				licOptions = append(licOptions, label)
				licByLabel[label] = lic.LicenseID
			}
		}
		if len(team.Licenses) == 0 {
			licBox.Add(widget.NewLabel(T("team_no_licenses")))
		}
		licCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("team_licenses_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			licBox,
		), customerCardBorder, customerCardFill)

		membersBox := container.NewVBox()
		if len(team.Members) == 0 {
			membersBox.Add(widget.NewLabel(T("team_no_members")))
		}
		for _, m := range team.Members {
			member := m
			folderNames := []string{}
			for _, id := range member.FolderIDs {
				if n, ok := names[id]; ok {
					folderNames = append(folderNames, n)
				} else {
					folderNames = append(folderNames, id)
				}
			}
			foldersLabel := strings.Join(folderNames, ", ")
			if foldersLabel == "" {
				foldersLabel = T("team_all_folders")
			}
			row := container.NewVBox(
				container.NewHBox(
					widget.NewLabelWithStyle(member.Email, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabel(member.Status),
				),
				widget.NewLabel(TF("team_member_line", member.LicenseID, foldersLabel)),
			)
			actions := []fyne.CanvasObject{}
			if member.Status != "active" {
				resendBtn := widget.NewButton(T("team_resend_btn"), func() {
					go func() {
						err := resendTeamInvitation(getCustomerSessionToken(), member.MemberID)
						fyne.Do(func() {
							if err != nil {
								if isCustomerSessionExpired(err) {
									clearCustomerSession()
									relaunch(panelID)
									return
								}
								dialog.ShowError(err, mainWindow)
								return
							}
							dialog.ShowInformation(T("team_resend_btn"), T("team_resend_ok"), mainWindow)
						})
					}()
				})
				actions = append(actions, resendBtn)
			}
			foldersBtn := widget.NewButton(T("team_folders_btn"), func() {
				openTeamFoldersDialog(member, folders, panelID, relaunch)
			})
			revokeBtn := widget.NewButton(T("team_revoke_btn"), func() {
				dialog.ShowConfirm(T("team_revoke_btn"), TF("team_revoke_confirm", member.Email), func(ok bool) {
					if !ok {
						return
					}
					go func() {
						err := revokeTeamMember(getCustomerSessionToken(), member.MemberID)
						fyne.Do(func() {
							if err != nil {
								if isCustomerSessionExpired(err) {
									clearCustomerSession()
								}
								dialog.ShowError(err, mainWindow)
								return
							}
							relaunch(panelID)
						})
					}()
				}, mainWindow)
			})
			actions = append(actions, foldersBtn, revokeBtn)
			row.Add(container.NewHBox(actions...))
			row.Add(widget.NewSeparator())
			membersBox.Add(row)
		}
		membersCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("team_members_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			membersBox,
		), customerCardBorder, customerCardFill)

		inviteCard := createCardBox(teamInviteForm(licOptions, licByLabel, folders, panelID, relaunch), customerCardBorder, customerCardFill)

		joinedBox := container.NewVBox()
		if len(team.Memberships) == 0 {
			joinedBox.Add(widget.NewLabel(T("team_no_memberships")))
		}
		for _, ms := range team.Memberships {
			joinedBox.Add(widget.NewLabel(TF("team_membership_line", ms.LicenseID, ms.Status)))
			joinedBox.Add(widget.NewSeparator())
		}
		joinedCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("team_joined_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			joinedBox,
		), customerCardBorder, customerCardFill)

		return container.NewVBox(licCard, membersCard, inviteCard, joinedCard), nil
	})
}

func teamInviteForm(licOptions []string, licByLabel map[string]string, folders []customerFolder, panelID int, relaunch func(int)) fyne.CanvasObject {
	form := container.NewVBox(
		widget.NewLabelWithStyle(T("team_invite_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	if len(licOptions) == 0 {
		form.Add(widget.NewLabel(T("team_invite_no_capacity")))
		return form
	}
	licSelect := widget.NewSelect(licOptions, nil)
	licSelect.SetSelected(licOptions[0])
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder(T("email_placeholder"))
	labels, byID := sortedFolderOptions(folders)
	folderCheck := widget.NewCheckGroup(labels, nil)
	msg := widget.NewLabel("")
	msg.Wrapping = fyne.TextWrapWord
	submit := widget.NewButton(T("team_invite_btn"), func() {
		licenseID := licByLabel[licSelect.Selected]
		email := strings.ToLower(strings.TrimSpace(emailEntry.Text))
		if licenseID == "" || email == "" {
			msg.SetText(T("login_err_empty"))
			return
		}
		selected := []string{}
		for _, label := range folderCheck.Selected {
			selected = append(selected, byID[label])
		}
		msg.SetText(T("login_checking"))
		go func() {
			text, err := inviteTeamMember(getCustomerSessionToken(), licenseID, email, selected)
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					msg.SetText(err.Error())
					return
				}
				msg.SetText(text)
				emailEntry.SetText("")
				relaunch(panelID)
			})
		}()
	})
	submit.Importance = widget.HighImportance
	form.Add(widget.NewLabel(T("team_license_label")))
	form.Add(licSelect)
	form.Add(widget.NewLabel(T("email_label")))
	form.Add(emailEntry)
	form.Add(widget.NewLabel(T("team_folders_label")))
	form.Add(folderCheck)
	form.Add(msg)
	form.Add(submit)
	return form
}

func openTeamFoldersDialog(member customerTeamMember, folders []customerFolder, panelID int, relaunch func(int)) {
	labels, byID := sortedFolderOptions(folders)
	check := widget.NewCheckGroup(labels, nil)
	initial := []string{}
	for label, id := range byID {
		for _, have := range member.FolderIDs {
			if have == id {
				initial = append(initial, label)
			}
		}
	}
	check.SetSelected(initial)
	content := container.NewVBox(
		widget.NewLabel(TF("team_folders_dialog_sub", member.Email)),
		container.NewVScroll(check),
	)
	dialog.ShowCustomConfirm(T("team_folders_btn"), T("dialog_save_btn"), T("dialog_cancel_btn"), content, func(ok bool) {
		if !ok {
			return
		}
		selected := []string{}
		for _, label := range check.Selected {
			selected = append(selected, byID[label])
		}
		go func() {
			err := updateTeamMemberFolders(getCustomerSessionToken(), member.MemberID, selected)
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					dialog.ShowError(err, mainWindow)
					return
				}
				relaunch(panelID)
			})
		}()
	}, mainWindow)
}

// licensesPanel liste les licences et leur renouvellement.
func licensesPanel(panelID int, relaunch func(int)) fyne.CanvasObject {
	return asyncCustomerPanel(panelID, relaunch, func(dash *customerDashboard) fyne.CanvasObject {
		box := container.NewVBox()
		if len(dash.Licenses) == 0 {
			box.Add(widget.NewLabel(T("licenses_empty")))
			return box
		}
		for _, lic := range dash.Licenses {
			l := lic
			card := container.NewVBox(
				container.NewHBox(
					widget.NewLabelWithStyle(l.LicenseID, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabel(licenseStatusLabel(l.Status)),
				),
				widget.NewLabel(TF("license_detail_line", formatCustomerDateRFC3339(l.CreatedAt), formatCustomerDateRFC3339(l.ExpiresAt), l.MaxConnections)),
			)
			if l.PendingRenewal {
				pending := widget.NewLabel(T("license_pending_renewal"))
				pending.Importance = widget.WarningImportance
				card.Add(pending)
			} else if l.Renewable {
				renewBtn := widget.NewButton(T("license_renew_btn"), func() {
					openRenewDialog(l.LicenseID, panelID, relaunch)
				})
				card.Add(renewBtn)
			}
			box.Add(createCardBox(card, customerCardBorder, customerCardFill))
		}
		return box
	})
}

var renewMethodOptions = []struct {
	value string
	key   string
}{
	{"stripe", "pay_method_stripe"},
	{"bank_transfer", "pay_method_bank"},
	{"crypto_btc", "pay_method_btc"},
	{"crypto_xrp", "pay_method_xrp"},
}

func openRenewDialog(licenseID string, panelID int, relaunch func(int)) {
	labels := []string{}
	byLabel := map[string]string{}
	for _, opt := range renewMethodOptions {
		labels = append(labels, T(opt.key))
		byLabel[T(opt.key)] = opt.value
	}
	methodSelect := widget.NewSelect(labels, nil)
	methodSelect.SetSelected(labels[0])
	termsCheck := widget.NewCheck(TF("renew_terms_label", customerTermsVersion), nil)
	termsLink := widget.NewButton(T("renew_terms_link"), func() {
		_ = openBrowserCrossPlatform("https://relaisdesk.fr/cgv.html")
	})
	immediateCheck := widget.NewCheck(T("renew_immediate_label"), nil)
	msg := widget.NewLabel("")
	msg.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		widget.NewLabel(TF("renew_dialog_sub", licenseID)),
		methodSelect, termsCheck, termsLink, immediateCheck, msg,
	)
	dialog.ShowCustomConfirm(T("license_renew_btn"), T("dialog_confirm_btn"), T("dialog_cancel_btn"), content, func(ok bool) {
		if !ok {
			return
		}
		if !termsCheck.Checked {
			dialog.ShowError(fmt.Errorf("%s", T("renew_terms_required")), mainWindow)
			return
		}
		method := byLabel[methodSelect.Selected]
		go func() {
			res, err := renewLicense(getCustomerSessionToken(), licenseID, method, customerTermsVersion, immediateCheck.Checked)
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					dialog.ShowError(err, mainWindow)
					return
				}
				switch {
				case res.CheckoutURL != "":
					_ = openBrowserCrossPlatform(res.CheckoutURL)
					dialog.ShowInformation(T("license_renew_btn"), TF("renew_order_created", res.OrderID), mainWindow)
					relaunch(panelID)
				case res.IBAN != "":
					dialog.ShowInformation(T("pay_method_bank"), TF("renew_bank_details",
						res.Beneficiary, res.IBAN, res.BIC, formatEUR(res.Amount), res.Reference), mainWindow)
					relaunch(panelID)
				case res.CryptoAddress != "":
					dialog.ShowInformation(T("renew_crypto_title"), TF("renew_crypto_details",
						res.CryptoAsset, res.CryptoAmount, res.CryptoAddress, res.OrderID), mainWindow)
					relaunch(panelID)
				default:
					dialog.ShowInformation(T("license_renew_btn"), TF("renew_order_created", res.OrderID), mainWindow)
					relaunch(panelID)
				}
			})
		}()
	}, mainWindow)
}

// billingPanel liste commandes, factures (PDF/CII) et abonnements.
func billingPanel(panelID int, relaunch func(int)) fyne.CanvasObject {
	return asyncCustomerPanel(panelID, relaunch, func(dash *customerDashboard) fyne.CanvasObject {
		ordersBox := container.NewVBox()
		if len(dash.Orders) == 0 {
			ordersBox.Add(widget.NewLabel(T("orders_empty")))
		}
		for _, o := range dash.Orders {
			order := o
			ordersBox.Add(container.NewVBox(
				container.NewHBox(
					widget.NewLabelWithStyle(order.OrderID, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabel(order.Status),
				),
				widget.NewLabel(TF("order_detail_line", order.Plan, order.Technicians, formatEUR(order.Price), order.PaymentMethod, formatCustomerDateRFC3339(order.CreatedAt))),
				widget.NewSeparator(),
			))
		}
		ordersCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("orders_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			ordersBox,
		), customerCardBorder, customerCardFill)

		invoicesBox := container.NewVBox()
		if len(dash.Invoices) == 0 {
			invoicesBox.Add(widget.NewLabel(T("invoices_empty")))
		}
		for _, inv := range dash.Invoices {
			invoice := inv
			pdfBtn := widget.NewButton(T("invoice_pdf_btn"), func() {
				go func() {
					data, err := downloadInvoicePDF(getCustomerSessionToken(), invoice.InvoiceNumber)
					fyne.Do(func() {
						if err != nil {
							dialog.ShowError(err, mainWindow)
							return
						}
						saveBytesDialog(data, "facture-"+invoice.InvoiceNumber+".pdf")
					})
				}()
			})
			ciiBtn := widget.NewButton(T("invoice_cii_btn"), func() {
				go func() {
					data, err := downloadInvoiceCII(getCustomerSessionToken(), invoice.InvoiceNumber)
					fyne.Do(func() {
						if err != nil {
							dialog.ShowError(err, mainWindow)
							return
						}
						saveBytesDialog(data, "facture-"+invoice.InvoiceNumber+".xml")
					})
				}()
			})
			invoicesBox.Add(container.NewVBox(
				container.NewHBox(
					widget.NewLabelWithStyle(invoice.InvoiceNumber, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabel(invoice.Status),
				),
				widget.NewLabel(TF("invoice_detail_line", formatEUR(invoice.AmountTTC), formatCustomerDateRFC3339(invoice.CreatedAt))),
				container.NewHBox(pdfBtn, ciiBtn),
				widget.NewSeparator(),
			))
		}
		invoicesCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("invoices_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			invoicesBox,
		), customerCardBorder, customerCardFill)

		subsBox := container.NewVBox()
		if len(dash.Subscriptions) == 0 {
			subsBox.Add(widget.NewLabel(T("subscriptions_empty")))
		}
		for _, sub := range dash.Subscriptions {
			s := sub
			portalBtn := widget.NewButton(T("billing_portal_btn"), func() {
				go func() {
					portalURL, err := openBillingPortal(getCustomerSessionToken(), s.ID)
					fyne.Do(func() {
						if err != nil {
							if isCustomerSessionExpired(err) {
								clearCustomerSession()
								relaunch(panelID)
								return
							}
							dialog.ShowError(err, mainWindow)
							return
						}
						if _, err := url.ParseRequestURI(portalURL); err != nil || !strings.HasPrefix(portalURL, "https://") {
							dialog.ShowError(fmt.Errorf("%s", T("billing_portal_invalid")), mainWindow)
							return
						}
						_ = openBrowserCrossPlatform(portalURL)
					})
				}()
			})
			subsBox.Add(container.NewVBox(
				widget.NewLabelWithStyle(fmt.Sprintf("%s — %d technicien(s)", s.Plan, s.Technicians), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				portalBtn,
				widget.NewSeparator(),
			))
		}
		subsCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("subscriptions_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			subsBox,
		), customerCardBorder, customerCardFill)

		return container.NewVBox(ordersCard, invoicesCard, subsCard)
	})
}

// historyPanel reproduit l'historique web : création, démarrage, clôture,
// annulation et export CSV.
func historyPanel(panelID int, relaunch func(int)) fyne.CanvasObject {
	return asyncFetchPanel(panelID, relaunch, func(token string) (fyne.CanvasObject, error) {
		dash, err := getCustomerDashboard(token)
		if err != nil {
			return nil, err
		}
		items, err := listCustomerInterventions(token)
		if err != nil {
			return nil, err
		}
		toolbar := container.NewHBox()
		if len(dash.Licenses) > 0 {
			createBtn := widget.NewButton(T("history_create_btn"), func() {
				openInterventionCreateDialog(dash.Licenses, panelID, relaunch)
			})
			toolbar.Add(createBtn)
		}
		exportBtn := widget.NewButton(T("history_export_btn"), func() {
			go func() {
				data, err := exportCustomerInterventionsCSV(getCustomerSessionToken())
				fyne.Do(func() {
					if err != nil {
						if isCustomerSessionExpired(err) {
							clearCustomerSession()
							relaunch(panelID)
							return
						}
						dialog.ShowError(err, mainWindow)
						return
					}
					saveBytesDialog(data, "interventions-relaisdesk.csv")
				})
			}()
		})
		toolbar.Add(exportBtn)

		list := container.NewVBox()
		if len(items) == 0 {
			list.Add(widget.NewLabel(T("history_empty")))
		}
		for _, it := range items {
			item := it
			dates := formatInterventionDates(item)
			row := container.NewVBox(
				container.NewHBox(
					widget.NewLabelWithStyle(item.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabel(item.Status),
				),
				widget.NewLabel(TF("history_item_line", item.InterventionID, item.ClientReference, dates)),
			)
			if item.Summary != "" {
				summary := widget.NewLabel(item.Summary)
				summary.Wrapping = fyne.TextWrapWord
				row.Add(summary)
			}
			actions := []fyne.CanvasObject{}
			switch item.Status {
			case "open":
				actions = append(actions, historyActionButton(T("history_start_btn"), item, "start", panelID, relaunch))
				actions = append(actions, historyActionButton(T("history_cancel_btn"), item, "cancel", panelID, relaunch))
			case "in_progress":
				completeBtn := widget.NewButton(T("history_complete_btn"), func() {
					openInterventionCompleteDialog(item, panelID, relaunch)
				})
				actions = append(actions, completeBtn)
				actions = append(actions, historyActionButton(T("history_cancel_btn"), item, "cancel", panelID, relaunch))
			}
			if len(actions) > 0 {
				row.Add(container.NewHBox(actions...))
			}
			row.Add(widget.NewSeparator())
			list.Add(row)
		}
		return container.NewVBox(toolbar, list), nil
	})
}

func formatInterventionDates(item customerInterventionView) string {
	start := "—"
	if item.StartedAt != nil && *item.StartedAt != "" {
		start = formatCustomerDateRFC3339(*item.StartedAt)
	}
	end := "—"
	if item.EndedAt != nil && *item.EndedAt != "" {
		end = formatCustomerDateRFC3339(*item.EndedAt)
	}
	if item.DurationMinutes != nil {
		return TF("history_dates_line", start, end, *item.DurationMinutes)
	}
	return TF("history_dates_line", start, end, 0)
}

func historyActionButton(label string, item customerInterventionView, action string, panelID int, relaunch func(int)) *widget.Button {
	return widget.NewButton(label, func() {
		go func() {
			_, err := runCustomerInterventionAction(getCustomerSessionToken(), item.InterventionID, action, "", "", "")
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					dialog.ShowError(err, mainWindow)
					return
				}
				relaunch(panelID)
			})
		}()
	})
}

func openInterventionCreateDialog(licenses []customerLicenseView, panelID int, relaunch func(int)) {
	options := []string{}
	for _, lic := range licenses {
		options = append(options, lic.LicenseID)
	}
	licSelect := widget.NewSelect(options, nil)
	licSelect.SetSelected(options[0])
	refEntry := widget.NewEntry()
	refEntry.SetPlaceHolder(T("history_ref_placeholder"))
	titleEntry := widget.NewEntry()
	titleEntry.SetPlaceHolder(T("history_title_placeholder"))
	content := container.NewVBox(
		widget.NewLabel(T("team_license_label")),
		licSelect,
		widget.NewLabel(T("history_ref_label")),
		refEntry,
		widget.NewLabel(T("history_title_label")),
		titleEntry,
	)
	dialog.ShowCustomConfirm(T("history_create_btn"), T("dialog_confirm_btn"), T("dialog_cancel_btn"), content, func(ok bool) {
		if !ok {
			return
		}
		if strings.TrimSpace(titleEntry.Text) == "" {
			return
		}
		go func() {
			_, err := createCustomerIntervention(getCustomerSessionToken(), licSelect.Selected, strings.TrimSpace(refEntry.Text), strings.TrimSpace(titleEntry.Text))
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					dialog.ShowError(err, mainWindow)
					return
				}
				relaunch(panelID)
			})
		}()
	}, mainWindow)
}

func openInterventionCompleteDialog(item customerInterventionView, panelID int, relaunch func(int)) {
	summaryEntry := widget.NewMultiLineEntry()
	summaryEntry.SetPlaceHolder(T("history_summary_placeholder"))
	content := container.NewVBox(
		widget.NewLabel(TF("history_complete_sub", item.Title)),
		summaryEntry,
	)
	dialog.ShowCustomConfirm(T("history_complete_btn"), T("dialog_confirm_btn"), T("dialog_cancel_btn"), content, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			_, err := runCustomerInterventionAction(getCustomerSessionToken(), item.InterventionID, "complete", item.ClientReference, item.Title, strings.TrimSpace(summaryEntry.Text))
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					dialog.ShowError(err, mainWindow)
					return
				}
				relaunch(panelID)
			})
		}()
	}, mainWindow)
}

// servicesPanel reproduit les prestations web : marchand Stripe, conditions,
// catalogue et encaissements.
func servicesPanel(panelID int, relaunch func(int)) fyne.CanvasObject {
	return asyncFetchPanel(panelID, relaunch, func(token string) (fyne.CanvasObject, error) {
		nav, err := getCustomerServiceBilling(token)
		if err != nil {
			return nil, err
		}
		if !nav.Available {
			return container.NewCenter(widget.NewLabel(T("services_unavailable"))), nil
		}
		merchantRows := container.NewVBox()
		if nav.Merchant.AccountID == "" {
			merchantRows.Add(widget.NewLabel(T("services_no_account")))
			onboardBtn := widget.NewButton(T("services_onboard_btn"), func() {
				go func() {
					linkURL, err := startCustomerServiceOnboarding(getCustomerSessionToken())
					fyne.Do(func() {
						if err != nil {
							if isCustomerSessionExpired(err) {
								clearCustomerSession()
								relaunch(panelID)
								return
							}
							showErrorWithLinks(err, mainWindow)
							return
						}
						if linkURL != "" {
							_ = openBrowserCrossPlatform(linkURL)
						}
						relaunch(panelID)
					})
				}()
			})
			merchantRows.Add(onboardBtn)
		} else {
			merchantRows.Add(widget.NewLabel(TF("services_account_line", nav.Merchant.AccountID)))
			enabledCheck := widget.NewCheck(T("services_enabled_label"), nil)
			enabledCheck.Checked = nav.Merchant.Enabled
			enabledCheck.OnChanged = func(on bool) {
				go func() {
					err := setCustomerServiceEnabled(getCustomerSessionToken(), on)
					fyne.Do(func() {
						if err != nil {
							enabledCheck.SetChecked(!on)
							showErrorWithLinks(err, mainWindow)
						}
					})
				}()
			}
			merchantRows.Add(enabledCheck)
		}
		merchantCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("services_merchant_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			merchantRows,
		), customerCardBorder, customerCardFill)

		termsRows := container.NewVBox()
		if nav.Terms.AcceptedAt > 0 {
			termsRows.Add(widget.NewLabel(TF("services_terms_accepted", formatCustomerDateUnix(nav.Terms.AcceptedAt))))
		} else {
			termsRows.Add(widget.NewLabel(TF("services_terms_pending", nav.Terms.Version)))
			acceptBtn := widget.NewButton(T("services_terms_accept_btn"), func() {
				go func() {
					err := acceptCustomerServiceTerms(getCustomerSessionToken(), nav.Terms.Version, nav.Terms.SHA256)
					fyne.Do(func() {
						if err != nil {
							if isCustomerSessionExpired(err) {
								clearCustomerSession()
								relaunch(panelID)
								return
							}
							dialog.ShowError(err, mainWindow)
							return
						}
						relaunch(panelID)
					})
				}()
			})
			termsRows.Add(acceptBtn)
		}
		if nav.Terms.URL != "" {
			termsURL := nav.Terms.URL
			readBtn := widget.NewButton(T("services_terms_read_btn"), func() {
				_ = openBrowserCrossPlatform(termsURL)
			})
			termsRows.Add(readBtn)
		}
		termsCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("services_terms_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			termsRows,
		), customerCardBorder, customerCardFill)

		ratesBox := container.NewVBox()
		if len(nav.Rates) == 0 {
			ratesBox.Add(widget.NewLabel(T("services_no_rates")))
		}
		for _, r := range nav.Rates {
			rate := r
			unit := T("services_rate_prepaid")
			if rate.Mode == "hourly" {
				unit = T("services_rate_hourly")
			}
			delBtn := widget.NewButton(T("services_rate_delete_btn"), func() {
				dialog.ShowConfirm(T("services_rate_delete_btn"), TF("services_rate_delete_confirm", rate.Label), func(ok bool) {
					if !ok {
						return
					}
					go func() {
						err := deleteCustomerServiceRate(getCustomerSessionToken(), rate.ID)
						fyne.Do(func() {
							if err != nil {
								if isCustomerSessionExpired(err) {
									clearCustomerSession()
									relaunch(panelID)
									return
								}
								dialog.ShowError(err, mainWindow)
								return
							}
							relaunch(panelID)
						})
					}()
				}, mainWindow)
			})
			ratesBox.Add(container.NewVBox(
				container.NewHBox(
					widget.NewLabelWithStyle(rate.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabel(formatEUR(float64(rate.Cents)/100)+" "+unit),
				),
				delBtn,
				widget.NewSeparator(),
			))
		}
		addRateBtn := widget.NewButton(T("services_rate_add_btn"), func() {
			openServiceRateDialog(panelID, relaunch)
		})
		ratesBox.Add(addRateBtn)
		ratesCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("services_rates_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			ratesBox,
		), customerCardBorder, customerCardFill)

		workBox := container.NewVBox()
		if len(nav.Work) == 0 {
			workBox.Add(widget.NewLabel(T("services_no_work")))
		}
		for _, w := range nav.Work {
			work := w
			paidLabel := T("services_unpaid")
			if work.Paid {
				paidLabel = T("services_paid")
			}
			row := container.NewVBox(
				container.NewHBox(
					widget.NewLabelWithStyle(work.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabel(work.State+" — "+paidLabel),
				),
				widget.NewLabel(TF("services_work_line", formatEUR(float64(work.AmountCents)/100), formatCustomerDateUnix(work.CreatedAt))),
			)
			actions := []fyne.CanvasObject{}
			canCheckout := !work.Paid && nav.Available && ((work.Mode == "prepaid" && work.State == "prepared") || (work.Mode == "hourly" && work.State == "finished"))
			if work.CheckoutURL != "" {
				link := work.CheckoutURL
				openBtn := widget.NewButton(T("services_open_link_btn"), func() {
					_ = openBrowserCrossPlatform(link)
				})
				copyBtn := widget.NewButton(T("services_copy_link_btn"), func() {
					fyneApp.Clipboard().SetContent(link)
				})
				actions = append(actions, openBtn, copyBtn)
			} else if canCheckout {
				checkoutBtn := widget.NewButton(T("services_checkout_btn"), func() {
					serviceWorkAction(work.ID, "checkout", panelID, relaunch)
				})
				actions = append(actions, checkoutBtn)
			}
			if !work.Paid && work.State != "finished" && work.State != "cancelled" && work.CheckoutURL == "" {
				finishBtn := widget.NewButton(T("services_finish_btn"), func() {
					serviceWorkAction(work.ID, "finish", panelID, relaunch)
				})
				cancelBtn := widget.NewButton(T("services_cancel_btn"), func() {
					serviceWorkAction(work.ID, "cancel", panelID, relaunch)
				})
				actions = append(actions, finishBtn, cancelBtn)
			} else if !work.Paid && work.CheckoutURL == "" && work.State == "prepared" {
				cancelBtn := widget.NewButton(T("services_cancel_btn"), func() {
					serviceWorkAction(work.ID, "cancel", panelID, relaunch)
				})
				actions = append(actions, cancelBtn)
			}
			if len(actions) > 0 {
				row.Add(container.NewHBox(actions...))
			}
			row.Add(widget.NewSeparator())
			workBox.Add(row)
		}
		workCard := createCardBox(container.NewVBox(
			widget.NewLabelWithStyle(T("services_work_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			workBox,
		), customerCardBorder, customerCardFill)

		return container.NewVBox(merchantCard, termsCard, ratesCard, workCard), nil
	})
}

func serviceWorkAction(workID, action string, panelID int, relaunch func(int)) {
	go func() {
		_, err := runCustomerServiceWork(getCustomerSessionToken(), workID, action)
		fyne.Do(func() {
			if err != nil {
				if isCustomerSessionExpired(err) {
					clearCustomerSession()
					relaunch(panelID)
					return
				}
				dialog.ShowError(err, mainWindow)
				return
			}
			relaunch(panelID)
		})
	}()
}

func openServiceRateDialog(panelID int, relaunch func(int)) {
	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder(T("services_rate_label_placeholder"))
	modeSelect := widget.NewSelect([]string{T("services_rate_prepaid"), T("services_rate_hourly")}, nil)
	modeSelect.SetSelected(T("services_rate_prepaid"))
	amountEntry := widget.NewEntry()
	amountEntry.SetPlaceHolder("25.00")
	content := container.NewVBox(
		widget.NewLabel(T("services_rate_label_label")),
		labelEntry,
		widget.NewLabel(T("services_rate_mode_label")),
		modeSelect,
		widget.NewLabel(T("services_rate_amount_label")),
		amountEntry,
	)
	dialog.ShowCustomConfirm(T("services_rate_add_btn"), T("dialog_confirm_btn"), T("dialog_cancel_btn"), content, func(ok bool) {
		if !ok {
			return
		}
		label := strings.TrimSpace(labelEntry.Text)
		if label == "" {
			return
		}
		mode := "prepaid"
		if modeSelect.Selected == T("services_rate_hourly") {
			mode = "hourly"
		}
		var euros float64
		if _, err := fmt.Sscanf(strings.ReplaceAll(strings.TrimSpace(amountEntry.Text), ",", "."), "%f", &euros); err != nil || euros <= 0 {
			dialog.ShowError(fmt.Errorf("%s", T("services_rate_amount_invalid")), mainWindow)
			return
		}
		go func() {
			_, err := createCustomerServiceRate(getCustomerSessionToken(), label, mode, int64(euros*100+0.5))
			fyne.Do(func() {
				if err != nil {
					if isCustomerSessionExpired(err) {
						clearCustomerSession()
						relaunch(panelID)
						return
					}
					dialog.ShowError(err, mainWindow)
					return
				}
				relaunch(panelID)
			})
		}()
	}, mainWindow)
}
