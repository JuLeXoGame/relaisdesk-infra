package database

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBatchTeamLicensesPreservesEligibilityAndOwnerIsolation(t *testing.T) {
	db, owner, lic, _ := teamFixture(t)
	for i, capacity := range []int{1, 5, 10, 20} {
		other, err := CreateLicense(db, owner.Email, 30, capacity, "")
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			if err := RevokeLicense(db, other.LicenseID, "test"); err != nil {
				t.Fatal(err)
			}
		}
	}
	member, err := CreateTeamInvitation(db, owner.ID, lic.LicenseID, "invited@example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		got, err := ListTeamLicenses(db, owner.ID)
		if err != nil || len(got) != 5 {
			t.Fatalf("listing: %v %v", got, err)
		}
		for _, item := range got {
			single, err := teamLicense(db, item.LicenseID, owner.ID)
			if err != nil || !reflect.DeepEqual(item, *single) {
				t.Fatalf("batch differs from individual read: %+v %+v %v", item, single, err)
			}
		}
		foreign, err := ListTeamLicenses(db, owner.ID+100)
		if err != nil || len(foreign) != 0 {
			t.Fatal("owner isolation failed", err)
		}
	}
	check()
	if _, err := db.Exec(`UPDATE team_members SET invite_expires_at='2000-01-01' WHERE member_id=?`, member.MemberID); err != nil {
		t.Fatal(err)
	}
	check()
}

func BenchmarkTeamLicenseListing(b *testing.B) {
	db, err := InitDatabase(filepath.Join(b.TempDir(), "listing.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	owner, err := EnsureCustomer(db, "benchmark@example.test", "business", "Benchmark")
	if err != nil {
		b.Fatal(err)
	}
	ids := make([]string, 50)
	for i := range ids {
		lic, err := CreateLicense(db, owner.Email, 30, 5, fmt.Sprint(i))
		if err != nil {
			b.Fatal(err)
		}
		ids[i] = lic.LicenseID
	}
	b.Run("individual_reads", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, id := range ids {
				if _, err := teamLicense(db, id, owner.ID); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("batch", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := ListTeamLicenses(db, owner.ID); err != nil {
				b.Fatal(err)
			}
		}
	})
}
