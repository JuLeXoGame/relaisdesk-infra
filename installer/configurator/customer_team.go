package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Équipe du compte client (miroir du panneau web) : licences éligibles,
// membres invités et dossiers autorisés.

type customerTeamLicense struct {
	LicenseID string `json:"license_id"`
	Plan      string `json:"plan"`
	Capacity  int    `json:"capacity"`
	Used      int    `json:"used"`
	Enabled   bool   `json:"enabled"`
}

type customerTeamMember struct {
	MemberID        string   `json:"member_id"`
	LicenseID       string   `json:"license_id"`
	Email           string   `json:"email"`
	Status          string   `json:"status"`
	InviteExpiresAt string   `json:"invite_expires_at,omitempty"`
	FolderIDs       []string `json:"folder_ids"`
}

type customerTeam struct {
	Licenses    []customerTeamLicense `json:"licenses"`
	Members     []customerTeamMember  `json:"members"`
	Memberships []customerTeamMember  `json:"memberships"`
}

func getCustomerTeam(token string) (*customerTeam, error) {
	var res customerTeam
	if err := doTechnicianReq(http.MethodGet, "/api/v1/customer/team", token, nil, &res); err != nil {
		return nil, err
	}
	if res.Licenses == nil {
		res.Licenses = []customerTeamLicense{}
	}
	if res.Members == nil {
		res.Members = []customerTeamMember{}
	}
	if res.Memberships == nil {
		res.Memberships = []customerTeamMember{}
	}
	return &res, nil
}

func inviteTeamMember(token, licenseID, email string, folderIDs []string) (string, error) {
	var res struct {
		Message string `json:"message"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/team/invitations", token, map[string]any{
		"license_id": licenseID, "email": email, "folder_ids": folderIDs,
	}, &res); err != nil {
		return "", err
	}
	return res.Message, nil
}

func resendTeamInvitation(token, memberID string) error {
	var res map[string]any
	return doTechnicianReq(http.MethodPost, "/api/v1/customer/team/members/"+url.PathEscape(memberID)+"/resend", token, map[string]string{}, &res)
}

func updateTeamMemberFolders(token, memberID string, folderIDs []string) error {
	var res map[string]any
	return doTechnicianReq(http.MethodPut, "/api/v1/customer/team/members/"+url.PathEscape(memberID), token, map[string]any{
		"folder_ids": folderIDs,
	}, &res)
}

func revokeTeamMember(token, memberID string) error {
	var res map[string]any
	return doTechnicianReq(http.MethodDelete, "/api/v1/customer/team/members/"+url.PathEscape(memberID), token, nil, &res)
}

type customerFolder struct {
	FolderID       string `json:"folder_id"`
	Name           string `json:"name"`
	ParentFolderID string `json:"parent_folder_id"`
	LicenseID      string `json:"license_id"`
}

func listCustomerFolders(token string) ([]customerFolder, error) {
	var res struct {
		Folders []customerFolder `json:"folders"`
	}
	if err := doTechnicianReq(http.MethodGet, "/api/v1/customer/device-folders", token, nil, &res); err != nil {
		return nil, err
	}
	if res.Folders == nil {
		res.Folders = []customerFolder{}
	}
	return res.Folders, nil
}

// downloadCustomerFile récupère un binaire (PDF, CII, CSV) avec la session
// client et retourne son contenu brut.
func downloadCustomerFile(token, apiPath string) ([]byte, error) {
	if !strings.HasPrefix(apiPath, "/api/v1/customer/") {
		return nil, fmt.Errorf("chemin de téléchargement refusé")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, APIURL+apiPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 25<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("téléchargement impossible (%d)", resp.StatusCode)
	}
	return body, nil
}
