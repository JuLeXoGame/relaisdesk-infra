package database

import (
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"
)

const TeamInviteLifetime = 48 * time.Hour
const MaxTeamFolderGrants = 100

var ErrTeamAccess = errors.New("équipe ou utilisateur non autorisé")
var ErrTeamCapacity = errors.New("nombre maximal d'utilisateurs atteint pour cette licence")
var ErrTeamInvite = errors.New("invitation invalide, expirée ou révoquée")

type TeamMember struct {
	MemberID         string   `json:"member_id"`
	LicenseID        string   `json:"license_id"`
	OwnerCustomerID  int64    `json:"-"`
	Email            string   `json:"email"`
	MemberCustomerID int64    `json:"-"`
	Status           string   `json:"status"`
	InviteExpiresAt  string   `json:"invite_expires_at,omitempty"`
	Folders          []string `json:"folder_ids"`
}

type TeamLicense struct {
	LicenseID string `json:"license_id"`
	Plan      string `json:"plan"`
	Capacity  int    `json:"capacity"`
	Used      int    `json:"used"`
	Enabled   bool   `json:"enabled"`
}

type teamQueryer interface {
	customerQueryer
	Query(string, ...any) (*sql.Rows, error)
}

func teamLicense(q teamQueryer, licenseID string, owner int64) (*TeamLicense, error) {
	var t TeamLicense
	var customer int64
	var status string
	var expires any
	err := q.QueryRow(`SELECT l.license_id,COALESCE(l.customer_id,0),l.max_connections,l.status,l.expires_at,
		COALESCE((SELECT plan FROM orders WHERE license_id=l.license_id AND status='paid' ORDER BY id DESC LIMIT 1),
		(SELECT plan FROM trial_applications WHERE license_id=l.license_id LIMIT 1),'') FROM licences l WHERE l.license_id=?`, licenseID).
		Scan(&t.LicenseID, &customer, &t.Capacity, &status, &expires, &t.Plan)
	if err != nil || (owner > 0 && owner != customer) {
		return nil, ErrTeamAccess
	}
	end, err := ParseSQLiteTime(expires)
	if err != nil {
		return nil, err
	}
	plan := strings.ToLower(t.Plan)
	t.Enabled = status == "active" && end.After(time.Now().UTC()) && t.Capacity > 0 &&
		(plan == "pro" || plan == "ultra" || plan == "custom" || plan == "personnalise" || plan == "personnalisé" || (plan == "" && t.Capacity >= 5))
	if plan == "" {
		if t.Capacity >= 10 {
			t.Plan = "ultra"
		} else if t.Capacity >= 5 {
			t.Plan = "pro"
		} else {
			t.Plan = "starter"
		}
	}
	err = q.QueryRow(`SELECT COUNT(*) FROM team_members WHERE license_id=? AND (status='active' OR
		(status='invited' AND julianday(invite_expires_at)>julianday('now')))`, licenseID).Scan(&t.Used)
	return &t, err
}

func ListTeamLicenses(db *sql.DB, owner int64) ([]TeamLicense, error) {
	rows, err := db.Query(`SELECT license_id FROM licences WHERE customer_id=? ORDER BY id`, owner)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []TeamLicense{}
	for _, id := range ids {
		t, e := teamLicense(db, id, owner)
		if e != nil {
			return nil, e
		}
		out = append(out, *t)
	}
	return out, nil
}

func teamMember(q teamQueryer, id string) (*TeamMember, error) {
	var m TeamMember
	err := q.QueryRow(`SELECT member_id,license_id,owner_customer_id,email,COALESCE(member_customer_id,0),status,COALESCE(invite_expires_at,'')
		FROM team_members WHERE member_id=?`, id).Scan(&m.MemberID, &m.LicenseID, &m.OwnerCustomerID, &m.Email, &m.MemberCustomerID, &m.Status, &m.InviteExpiresAt)
	if err != nil {
		return nil, ErrTeamAccess
	}
	m.Folders = []string{}
	rows, err := q.Query(`SELECT folder_id FROM team_folder_grants WHERE member_id=? ORDER BY folder_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		if err = rows.Scan(&f); err != nil {
			return nil, err
		}
		m.Folders = append(m.Folders, f)
	}
	return &m, rows.Err()
}

func listTeamMembers(q teamQueryer, where string, args ...any) ([]TeamMember, error) {
	rows, err := q.Query(`SELECT member_id FROM team_members WHERE `+where+` ORDER BY created_at,member_id`, args...)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []TeamMember{}
	for _, id := range ids {
		m, e := teamMember(q, id)
		if e != nil {
			return nil, e
		}
		result = append(result, *m)
	}
	return result, nil
}

func ListOwnedTeamMembers(db *sql.DB, owner int64) ([]TeamMember, error) {
	return listTeamMembers(db, "owner_customer_id=?", owner)
}
func ListMyTeamMemberships(db *sql.DB, email string) ([]TeamMember, error) {
	all, err := listTeamMembers(db, "email=? COLLATE NOCASE AND status='active'", email)
	if err != nil {
		return nil, err
	}
	out := []TeamMember{}
	for _, m := range all {
		if _, e := activeTeamMember(db, m.MemberID, email); e == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

func activeTeamMember(q teamQueryer, id, email string) (*TeamMember, error) {
	m, err := teamMember(q, id)
	if err != nil || m.Status != "active" || (email != "" && !strings.EqualFold(m.Email, email)) {
		return nil, ErrTeamAccess
	}
	t, err := teamLicense(q, m.LicenseID, m.OwnerCustomerID)
	if err != nil || !t.Enabled {
		return nil, ErrTeamAccess
	}
	// On a downgrade, only the oldest purchased number of active users retain access.
	var rank int
	err = q.QueryRow(`SELECT COUNT(*) FROM team_members WHERE license_id=? AND status='active' AND
		(created_at < (SELECT created_at FROM team_members WHERE member_id=?) OR
		(created_at=(SELECT created_at FROM team_members WHERE member_id=?) AND member_id<=?))`, m.LicenseID, id, id, id).Scan(&rank)
	if err != nil {
		return nil, err
	}
	if rank > t.Capacity {
		return nil, ErrTeamAccess
	}
	return m, nil
}

func setTeamFolders(tx *sql.Tx, m *TeamMember, folders []string) error {
	if len(folders) > MaxTeamFolderGrants {
		return errors.New("100 autorisations de dossiers maximum par utilisateur")
	}
	unique := map[string]bool{}
	for _, id := range folders {
		if id == "" || unique[id] {
			return errors.New("dossier vide ou dupliqué")
		}
		unique[id] = true
		var n int
		err := tx.QueryRow(`SELECT COUNT(*) FROM device_folders WHERE folder_id=? AND
			(customer_id=? OR license_id=?) AND (license_id IS NULL OR license_id='' OR license_id=?)`, id, m.OwnerCustomerID, m.LicenseID, m.LicenseID).Scan(&n)
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrTeamAccess
		}
	}
	if _, err := tx.Exec(`DELETE FROM team_folder_grants WHERE member_id=?`, m.MemberID); err != nil {
		return err
	}
	for _, id := range folders {
		if _, err := tx.Exec(`INSERT INTO team_folder_grants(member_id,folder_id) VALUES (?,?)`, m.MemberID, id); err != nil {
			return err
		}
	}
	return nil
}

func CreateTeamInvitation(db *sql.DB, owner int64, licenseID, email string, folders []string) (*TeamMember, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n\t") {
		return nil, errors.New("adresse e-mail invalide")
	}
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := teamLicense(tx, licenseID, owner)
	if err != nil || !t.Enabled {
		return nil, ErrTeamAccess
	}
	var self int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM customer_users WHERE customer_id=? AND email=? COLLATE NOCASE`, owner, email).Scan(&self); err != nil {
		return nil, err
	}
	if self > 0 {
		return nil, errors.New("le propriétaire dispose déjà de ses accès")
	}
	var existingID string
	err = tx.QueryRow(`SELECT member_id FROM team_members WHERE license_id=? AND email=? COLLATE NOCASE`, licenseID, email).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if existingID != "" {
		existing, e := teamMember(tx, existingID)
		if e != nil {
			return nil, e
		}
		if existing.Status == "active" {
			return nil, errors.New("cet utilisateur fait déjà partie de l'équipe")
		}
		if existing.Status == "invited" {
			end, _ := ParseSQLiteTime(existing.InviteExpiresAt)
			if end.After(time.Now().UTC()) {
				return nil, errors.New("une invitation est déjà en attente ; renvoyez-la ou révoquez-la")
			}
		}
	}
	if t.Used >= t.Capacity {
		return nil, ErrTeamCapacity
	}
	id := existingID
	if id == "" {
		id, err = randomDeviceCode("MEM-", 16)
		if err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	end := time.Now().UTC().Add(TeamInviteLifetime).Format(time.RFC3339)
	_, err = tx.Exec(`INSERT INTO team_members(member_id,license_id,owner_customer_id,email,status,invite_expires_at,created_at,updated_at)
		VALUES (?,?,?,?,'invited',?,?,?) ON CONFLICT(license_id,email) DO UPDATE SET status='invited',invite_hash=NULL,
		invite_expires_at=excluded.invite_expires_at,updated_at=excluded.updated_at,member_customer_id=NULL,accepted_at=NULL`, id, licenseID, owner, email, end, now, now)
	if err != nil {
		return nil, err
	}
	m := &TeamMember{MemberID: id, LicenseID: licenseID, OwnerCustomerID: owner, Email: email, Status: "invited", InviteExpiresAt: end, Folders: folders}
	if err = setTeamFolders(tx, m, folders); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return m, nil
}

func UpdateTeamFolders(db *sql.DB, owner int64, id string, folders []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	m, err := teamMember(tx, id)
	if err != nil || m.OwnerCustomerID != owner || m.Status == "revoked" {
		return ErrTeamAccess
	}
	t, err := teamLicense(tx, m.LicenseID, owner)
	if err != nil || !t.Enabled {
		return ErrTeamAccess
	}
	if err = setTeamFolders(tx, m, folders); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE team_members SET updated_at=CURRENT_TIMESTAMP WHERE member_id=?`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func RevokeTeamMember(db *sql.DB, owner int64, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	m, err := teamMember(tx, id)
	if err != nil || m.OwnerCustomerID != owner {
		return ErrTeamAccess
	}
	if _, err = tx.Exec(`UPDATE team_members SET status='revoked',invite_hash=NULL,updated_at=CURRENT_TIMESTAMP WHERE member_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM technician_sessions WHERE team_member_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM technician_2fa_challenges WHERE team_member_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Tokens exist in memory only; queued mail jobs store a member ID, never the token.
func IssueTeamInvitationToken(db *sql.DB, owner int64, id string) (string, string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	m, err := teamMember(tx, id)
	if err != nil || m.Status != "invited" || (owner > 0 && owner != m.OwnerCustomerID) {
		return "", "", ErrTeamInvite
	}
	end, err := ParseSQLiteTime(m.InviteExpiresAt)
	if err != nil || !end.After(time.Now().UTC()) {
		return "", "", ErrTeamInvite
	}
	t, err := teamLicense(tx, m.LicenseID, m.OwnerCustomerID)
	if err != nil || !t.Enabled || t.Used > t.Capacity {
		return "", "", ErrTeamInvite
	}
	token, err := generateSecureToken()
	if err != nil {
		return "", "", err
	}
	_, err = tx.Exec(`UPDATE team_members SET invite_hash=?,updated_at=CURRENT_TIMESTAMP WHERE member_id=?`, hashSessionToken(token), id)
	if err != nil {
		return "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	return token, m.Email, nil
}

// The invitation proves mailbox access, not a second factor. The returned login
// token still goes through the existing password/MFA-protected account flow.
func AcceptTeamInvitation(db *sql.DB, token string) (loginToken, email string, err error) {
	if len(token) < 32 || len(token) > 256 {
		return "", "", ErrTeamInvite
	}
	tx, err := db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRow(`SELECT member_id FROM team_members WHERE invite_hash=? AND status='invited'
		AND julianday(invite_expires_at)>julianday('now')`, hashSessionToken(token)).Scan(&id); err != nil {
		return "", "", ErrTeamInvite
	}
	m, err := teamMember(tx, id)
	if err != nil {
		return "", "", ErrTeamInvite
	}
	t, err := teamLicense(tx, m.LicenseID, m.OwnerCustomerID)
	if err != nil || !t.Enabled || t.Used > t.Capacity {
		return "", "", ErrTeamInvite
	}
	identity, err := ensureCustomer(tx, m.Email, "business", "")
	if err != nil {
		return "", "", err
	}
	if identity.ID == m.OwnerCustomerID {
		return "", "", ErrTeamInvite
	}
	res, err := tx.Exec(`UPDATE team_members SET status='active',member_customer_id=?,invite_hash=NULL,accepted_at=CURRENT_TIMESTAMP,
		updated_at=CURRENT_TIMESTAMP WHERE member_id=? AND status='invited' AND invite_hash=?`, identity.ID, id, hashSessionToken(token))
	if err != nil {
		return "", "", err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return "", "", ErrTeamInvite
	}
	loginToken, err = generateSecureToken()
	if err != nil {
		return "", "", err
	}
	_, err = tx.Exec(`INSERT INTO customer_login_tokens(token_hash,email,customer_id,expires_at) VALUES (?,?,?,?)`, hashSessionToken(loginToken), m.Email, identity.ID, time.Now().UTC().Add(customerLoginTokenLifetime).Format(time.RFC3339))
	if err != nil {
		return "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	return loginToken, m.Email, nil
}

func TeamMemberForSession(db *sql.DB, token string) (*TeamMember, error) {
	var id sql.NullString
	if err := db.QueryRow(`SELECT team_member_id FROM technician_sessions WHERE token=?`, hashSessionToken(token)).Scan(&id); err != nil {
		return nil, err
	}
	if !id.Valid || id.String == "" {
		return nil, nil
	}
	return activeTeamMember(db, id.String, "")
}

func TeamFolderPath(db *sql.DB, licenseID, folderID string) ([]string, error) {
	if folderID == "" {
		return []string{}, nil
	}
	rows, err := db.Query(`WITH RECURSIVE ancestors(folder_id,parent_folder_id) AS (
		SELECT f.folder_id,f.parent_folder_id FROM device_folders f JOIN licences l ON l.license_id=?
		WHERE f.folder_id=? AND (f.customer_id=l.customer_id OR f.license_id=l.license_id)
		AND (f.license_id IS NULL OR f.license_id='' OR f.license_id=l.license_id)
		UNION SELECT f.folder_id,f.parent_folder_id FROM device_folders f JOIN ancestors a ON f.folder_id=a.parent_folder_id
		JOIN licences l ON l.license_id=? WHERE (f.customer_id=l.customer_id OR f.license_id=l.license_id)
		AND (f.license_id IS NULL OR f.license_id='' OR f.license_id=l.license_id)) SELECT folder_id FROM ancestors LIMIT 33`, licenseID, folderID, licenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if len(out) > 32 {
		return nil, errors.New("arborescence trop profonde pour une autorisation réseau")
	}
	return out, rows.Err()
}

func TeamMemberCanAccessDevice(db *sql.DB, m *TeamMember, device *Device) (bool, error) {
	if m == nil || device == nil || !device.IsActive || device.LicenseID != m.LicenseID || device.FolderID == "" {
		return false, nil
	}
	current, err := activeTeamMember(db, m.MemberID, m.Email)
	if err != nil {
		return false, err
	}
	var version int
	if err = db.QueryRow(`SELECT peer_auth_version FROM devices WHERE device_id=?`, device.DeviceID).Scan(&version); err != nil {
		return false, err
	}
	if version != 1 {
		return false, nil
	}
	path, err := TeamFolderPath(db, device.LicenseID, device.FolderID)
	if err != nil {
		return false, err
	}
	for _, a := range path {
		for _, g := range current.Folders {
			if a == g {
				return true, nil
			}
		}
	}
	return false, nil
}

func technicianMemberTx(tx *sql.Tx, licenseID, email string) (string, error) {
	var owner int
	err := tx.QueryRow(`SELECT COUNT(*) FROM licences l WHERE l.license_id=? AND (l.email=? COLLATE NOCASE OR
		EXISTS(SELECT 1 FROM customer_users u WHERE u.customer_id=l.customer_id AND u.email=? COLLATE NOCASE AND u.role='owner'))`, licenseID, email, email).Scan(&owner)
	if err != nil {
		return "", err
	}
	if owner == 1 {
		return "", nil
	}
	var id string
	if err = tx.QueryRow(`SELECT member_id FROM team_members WHERE license_id=? AND email=? COLLATE NOCASE AND status='active'`, licenseID, email).Scan(&id); err != nil {
		return "", ErrTeamAccess
	}
	if _, err = activeTeamMember(tx, id, email); err != nil {
		return "", err
	}
	return id, nil
}

func createMemberTechnicianSessionTx(tx *sql.Tx, licenseID, memberID string, now time.Time) (string, error) {
	if memberID != "" {
		m, err := activeTeamMember(tx, memberID, "")
		if err != nil || m.LicenseID != licenseID {
			return "", ErrTeamAccess
		}
	}
	token, err := createTechnicianSessionTx(tx, licenseID, now)
	if err != nil {
		return "", err
	}
	if memberID != "" {
		_, err = tx.Exec(`UPDATE technician_sessions SET team_member_id=? WHERE token=?`, memberID, hashSessionToken(token))
	}
	return token, err
}

func CreatePersonalTechnicianSession(db *sql.DB, email, licenseID string) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	memberID, err := technicianMemberTx(tx, licenseID, email)
	if err != nil {
		return "", err
	}
	token, err := createMemberTechnicianSessionTx(tx, licenseID, memberID, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

const teamFoldersCTE = `WITH RECURSIVE allowed(folder_id) AS (
	SELECT g.folder_id FROM team_folder_grants g JOIN device_folders f ON f.folder_id=g.folder_id
	JOIN team_members m ON m.member_id=g.member_id WHERE m.member_id=? AND m.status='active'
	AND (f.customer_id=m.owner_customer_id OR f.license_id=m.license_id)
	AND (f.license_id IS NULL OR f.license_id='' OR f.license_id=m.license_id)
	UNION SELECT f.folder_id FROM device_folders f JOIN allowed a ON f.parent_folder_id=a.folder_id
	JOIN team_members m ON m.member_id=? WHERE (f.customer_id=m.owner_customer_id OR f.license_id=m.license_id)
	AND (f.license_id IS NULL OR f.license_id='' OR f.license_id=m.license_id)) `

func ListTeamDevicePage(db *sql.DB, memberID string, after, limit int) ([]Device, error) {
	m, err := activeTeamMember(db, memberID, "")
	if err != nil {
		return nil, err
	}
	if after < 0 || limit < 1 || limit > 200 {
		return nil, errors.New("pagination invalide")
	}
	rows, err := db.Query(teamFoldersCTE+`SELECT `+deviceColumns+` FROM devices WHERE license_id=? AND is_active=1 AND peer_auth_version=1 AND id>?
		AND folder_id IN (SELECT folder_id FROM allowed) ORDER BY id LIMIT ?`, memberID, memberID, m.LicenseID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, e := scanDevice(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func ListTeamFolders(db *sql.DB, memberID string) ([]DeviceFolder, error) {
	if _, err := activeTeamMember(db, memberID, ""); err != nil {
		return nil, err
	}
	rows, err := db.Query(teamFoldersCTE+`SELECT id,folder_id,COALESCE(customer_id,0),COALESCE(license_id,''),parent_folder_id,name,created_at,updated_at
		FROM device_folders WHERE folder_id IN (SELECT folder_id FROM allowed) ORDER BY name COLLATE NOCASE`, memberID, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceFolder{}
	visible := map[string]bool{}
	for rows.Next() {
		var f DeviceFolder
		if err = rows.Scan(&f.ID, &f.FolderID, &f.CustomerID, &f.LicenseID, &f.ParentFolderID, &f.Name, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
		visible[f.FolderID] = true
	}
	for i := range out {
		if !visible[out[i].ParentFolderID] {
			out[i].ParentFolderID = ""
		}
	}
	return out, rows.Err()
}
