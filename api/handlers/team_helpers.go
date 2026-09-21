package handlers

import (
	dbpkg "database"
	"database/sql"
)

func technicianIsTeamMember(db *sql.DB, token string) bool {
	m, err := dbpkg.TeamMemberForSession(db, token)
	return err == nil && m != nil
}
