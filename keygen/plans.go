package main

import (
	dbpkg "database"
	"fmt"
	"math"
	"strings"
)

// PlanInfo holds details about a subscription plan.
type PlanInfo struct {
	Name           string
	Technicians    int
	MaxConnections int
	MonthlyPrice   float64
	Description    string
}

// CalculatePlan determines plan details, max connections, and monthly price in EUR.
func CalculatePlan(plan string, technicians int) (*PlanInfo, error) {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "starter":
		return &PlanInfo{
			Name:           "Starter",
			Technicians:    1,
			MaxConnections: 1,
			MonthlyPrice:   24.90,
			Description:    "Starter : 24,90€/mois (ou 239€/an), 1 technicien, viewers illimités",
		}, nil

	case "pro":
		techs := technicians
		if techs < 1 || techs > 5 {
			techs = 5
		}
		return &PlanInfo{
			Name:           "Pro",
			Technicians:    techs,
			MaxConnections: 5,
			MonthlyPrice:   dbpkg.ProMonthlyPrice,
			Description:    "Pro : 110€/mois (ou 1 100€/an), 1 à 5 techniciens, viewers illimités",
		}, nil

	case "ultra", "custom", "personnalise", "personnalisé":
		techs := technicians
		if techs < 10 {
			techs = 10
		}
		if techs > 500 {
			return nil, fmt.Errorf("nombre de techniciens limité à 500")
		}

		price := CalculateUltraPrice(techs)
		return &PlanInfo{
			Name:           "Personnalisé",
			Technicians:    techs,
			MaxConnections: techs,
			MonthlyPrice:   price,
			Description:    fmt.Sprintf("Personnalisé : %.2f€/mois, %d techniciens, viewers illimités", price, techs),
		}, nil

	default:
		return nil, fmt.Errorf("plan inconnu %q (choix possibles: starter, pro, ultra)", plan)
	}
}

// CalculateUltraPrice calculates the monthly price for the Ultra plan based on technician count.
// - Base 10 techs : 199.00€
// - 10 à 50 techs : +14.00€ / tech (+140€ / tranche de 10)
// - 50 à 100 techs : +10.00€ / tech (+100€ / tranche de 10)
// - 100 à 500 techs : +7.00€ / tech (+70€ / tranche de 10)
func CalculateUltraPrice(technicians int) float64 {
	if technicians <= 10 {
		return dbpkg.UltraBaseMonthlyPrice
	}

	var rawPrice float64
	if technicians <= 50 {
		extra := technicians - 10
		rawPrice = dbpkg.UltraBaseMonthlyPrice + float64(extra)*14.00
	} else if technicians <= 100 {
		base50 := dbpkg.UltraBaseMonthlyPrice + 40.0*14.00 // 759.00€
		extra := technicians - 50
		rawPrice = base50 + float64(extra)*10.00
	} else {
		base100 := dbpkg.UltraBaseMonthlyPrice + 40.0*14.00 + 50.0*10.00 // 1259.00€
		extra := technicians - 100
		if extra > 400 {
			extra = 400
		}
		rawPrice = base100 + float64(extra)*7.00
	}

	return math.Round(rawPrice*100) / 100
}

// PrintPricingGrid displays the official RelaisDesk pricing table.
func PrintPricingGrid() {
	fmt.Println("=========================================================================")
	fmt.Println("                       GRILLE TARIFAIRE RELAISDESK                      ")
	fmt.Println("=========================================================================")
	fmt.Println("  1. PLAN STARTER")
	fmt.Println("     - Tarif Mensuel : 24,90 € / mois")
	fmt.Println("     - Tarif Annuel  : 239,00 € / an (~19,90 € / mois - 2 mois offerts)")
	fmt.Println("     - Capacité      : 1 technicien (1 connexion simultanée)")
	fmt.Println("     - Viewers       : Illimités (0 € / machine)")
	fmt.Println("")
	fmt.Println("  2. PLAN PRO")
	fmt.Println("     - Tarif Mensuel : 110,00 € / mois")
	fmt.Println("     - Tarif Annuel  : 1 100,00 € / an (~91,67 € / mois - 2 mois offerts)")
	fmt.Println("     - Capacité      : 1 à 5 techniciens (5 connexions simultanées)")
	fmt.Println("     - Viewers       : Illimités (0 € / machine)")
	fmt.Println("")
	fmt.Println("  3. PLAN PERSONNALISÉ (Sur-mesure 10 à 500 techniciens)")
	fmt.Println("     - Base 10 techs : 199,00 € / mois (1 990,00 € / an)")
	fmt.Println("     - 10 à 50 techs : +14,00 € / technicien supplémentaire")
	fmt.Println("     - 50 à 100 techs: +10,00 € / technicien supplémentaire")
	fmt.Println("     - 100 à 500 tech: +7,00 € / technicien supplémentaire")
	fmt.Println("     - Tarif Annuel  : 10 × tarif mensuel (2 mois offerts)")
	fmt.Println("     - Viewers       : Illimités (0 € / machine)")
	fmt.Println("")
	fmt.Println("  Exemples de tarifs Personnalisé (Mensuel / Annuel) :")
	fmt.Printf("     * 10 techniciens  : %7.2f € / mois | %8.2f € / an\n", CalculateUltraPrice(10), CalculateUltraPrice(10)*10)
	fmt.Printf("     * 20 techniciens  : %7.2f € / mois | %8.2f € / an\n", CalculateUltraPrice(20), CalculateUltraPrice(20)*10)
	fmt.Printf("     * 50 techniciens  : %7.2f € / mois | %8.2f € / an\n", CalculateUltraPrice(50), CalculateUltraPrice(50)*10)
	fmt.Printf("     * 100 techniciens : %7.2f € / mois | %8.2f € / an\n", CalculateUltraPrice(100), CalculateUltraPrice(100)*10)
	fmt.Printf("     * 200 techniciens : %7.2f € / mois | %8.2f € / an\n", CalculateUltraPrice(200), CalculateUltraPrice(200)*10)
	fmt.Printf("     * 500 techniciens : %7.2f € / mois | %8.2f € / an\n", CalculateUltraPrice(500), CalculateUltraPrice(500)*10)
	fmt.Println("=========================================================================")
}
