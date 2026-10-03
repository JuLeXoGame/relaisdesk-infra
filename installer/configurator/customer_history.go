package main

import (
	"net/http"
	"net/url"
)

// Historique des interventions du compte client : liste, création,
// démarrage, clôture, annulation et export CSV.

type customerInterventionsList struct {
	Interventions []customerInterventionView `json:"interventions"`
}

func listCustomerInterventions(token string) ([]customerInterventionView, error) {
	var res customerInterventionsList
	if err := doTechnicianReq(http.MethodGet, "/api/v1/customer/interventions", token, nil, &res); err != nil {
		return nil, err
	}
	if res.Interventions == nil {
		res.Interventions = []customerInterventionView{}
	}
	return res.Interventions, nil
}

func createCustomerIntervention(token, licenseID, clientReference, title string) (*customerInterventionView, error) {
	var res customerInterventionView
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/interventions", token, map[string]string{
		"license_id": licenseID, "client_reference": clientReference, "title": title,
	}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func runCustomerInterventionAction(token, interventionID, action, clientReference, title, summary string) (*customerInterventionView, error) {
	var res customerInterventionView
	body := map[string]string{}
	if action == "complete" {
		body["client_reference"] = clientReference
		body["title"] = title
		body["summary"] = summary
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/interventions/"+url.PathEscape(interventionID)+"/"+action, token, body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func exportCustomerInterventionsCSV(token string) ([]byte, error) {
	return downloadCustomerFile(token, "/api/v1/customer/interventions/export")
}
