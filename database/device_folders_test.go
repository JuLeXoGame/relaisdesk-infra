package database

import (
	"path/filepath"
	"testing"
)

func TestDeviceFoldersCRUD(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "folders_test.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	lic, err := CreateLicense(db, "tech@example.com", 30, 2, "Pro")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	// 1. Create root folder
	rootFolder, err := CreateDeviceFolder(db, 0, lic.LicenseID, "Siège Paris", "")
	if err != nil {
		t.Fatalf("CreateDeviceFolder root failed: %v", err)
	}
	if rootFolder.FolderID == "" || rootFolder.Name != "Siège Paris" || rootFolder.ParentFolderID != "" {
		t.Fatalf("Unexpected root folder: %+v", rootFolder)
	}

	// 2. Create subfolder under root folder
	subFolder, err := CreateDeviceFolder(db, 0, lic.LicenseID, "Comptabilité", rootFolder.FolderID)
	if err != nil {
		t.Fatalf("CreateDeviceFolder subfolder failed: %v", err)
	}
	if subFolder.ParentFolderID != rootFolder.FolderID || subFolder.Name != "Comptabilité" {
		t.Fatalf("Unexpected subfolder: %+v", subFolder)
	}

	// 3. Create sub-subfolder
	subSubFolder, err := CreateDeviceFolder(db, 0, lic.LicenseID, "Facturation Client", subFolder.FolderID)
	if err != nil {
		t.Fatalf("CreateDeviceFolder sub-subfolder failed: %v", err)
	}

	// 4. List folders
	folders, err := ListDeviceFolders(db, 0, lic.LicenseID)
	if err != nil {
		t.Fatalf("ListDeviceFolders failed: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("Expected 3 folders, got %d", len(folders))
	}

	// 5. Rename folder
	err = UpdateDeviceFolder(db, 0, lic.LicenseID, subFolder.FolderID, "Finance & Compta")
	if err != nil {
		t.Fatalf("UpdateDeviceFolder failed: %v", err)
	}
	renamed, err := GetDeviceFolder(db, 0, lic.LicenseID, subFolder.FolderID)
	if err != nil || renamed.Name != "Finance & Compta" {
		t.Fatalf("Expected renamed folder, got: %+v, err: %v", renamed, err)
	}

	// 6. Create a device and assign to subSubFolder
	dev, err := CreatePermanentEnrollment(db, 0, lic.LicenseID, "PC-COMPTA-01", "Poste compta")
	if err != nil {
		t.Fatalf("CreatePermanentEnrollment failed: %v", err)
	}
	if dev.FolderID != "" {
		t.Fatalf("New device should have empty folder_id, got %s", dev.FolderID)
	}

	err = SetDeviceFolder(db, 0, lic.LicenseID, dev.DeviceID, subSubFolder.FolderID)
	if err != nil {
		t.Fatalf("SetDeviceFolder failed: %v", err)
	}
	updatedDev, err := GetDeviceByID(db, dev.DeviceID)
	if err != nil || updatedDev.FolderID != subSubFolder.FolderID {
		t.Fatalf("Expected device in %s, got %+v, err: %v", subSubFolder.FolderID, updatedDev, err)
	}

	// 7. Update alias and folder via TechnicianUpdateDevice
	err = TechnicianUpdateDevice(db, lic.LicenseID, dev.DeviceID, "PC-COMPTA-01-BIS", "Notes maj", rootFolder.FolderID)
	if err != nil {
		t.Fatalf("TechnicianUpdateDevice with folder failed: %v", err)
	}
	updatedDev2, _ := GetDeviceByID(db, dev.DeviceID)
	if updatedDev2.FolderID != rootFolder.FolderID || updatedDev2.Alias != "PC-COMPTA-01-BIS" {
		t.Fatalf("TechnicianUpdateDevice did not update folder or alias properly: %+v", updatedDev2)
	}

	// 8. Re-assign to subFolder and test deleting parent folder (rootFolder)
	_ = SetDeviceFolder(db, 0, lic.LicenseID, dev.DeviceID, subFolder.FolderID)

	// Deleting rootFolder must delete rootFolder, subFolder, and subSubFolder,
	// and reset dev.FolderID to ""
	err = DeleteDeviceFolder(db, 0, lic.LicenseID, rootFolder.FolderID)
	if err != nil {
		t.Fatalf("DeleteDeviceFolder failed: %v", err)
	}

	remainingFolders, err := ListDeviceFolders(db, 0, lic.LicenseID)
	if err != nil {
		t.Fatalf("ListDeviceFolders failed: %v", err)
	}
	if len(remainingFolders) != 0 {
		t.Fatalf("Expected 0 folders after cascade delete, got %d", len(remainingFolders))
	}

	devAfterDelete, err := GetDeviceByID(db, dev.DeviceID)
	if err != nil {
		t.Fatalf("Device should still exist after folder delete, got err: %v", err)
	}
	if devAfterDelete.FolderID != "" {
		t.Fatalf("Device folder_id should have reset to '', got %s", devAfterDelete.FolderID)
	}
}

func TestDeviceFoldersSecurityBoundary(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "folders_sec_test.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	lic1, _ := CreateLicense(db, "tech1@example.com", 30, 2, "Pro")
	lic2, _ := CreateLicense(db, "tech2@example.com", 30, 2, "Pro")

	folder1, err := CreateDeviceFolder(db, 0, lic1.LicenseID, "Dossier Tech 1", "")
	if err != nil {
		t.Fatalf("Create folder 1 failed: %v", err)
	}

	// Tech 2 tries to create subfolder under Tech 1's folder -> must fail
	_, err = CreateDeviceFolder(db, 0, lic2.LicenseID, "Tentative Sous-Dossier", folder1.FolderID)
	if err == nil {
		t.Fatalf("Tech 2 should NOT be able to nest under Tech 1's folder")
	}

	// Tech 2 tries to rename Tech 1's folder -> must fail
	err = UpdateDeviceFolder(db, 0, lic2.LicenseID, folder1.FolderID, "Dossier Piraté")
	if err == nil {
		t.Fatalf("Tech 2 should NOT be able to update Tech 1's folder")
	}

	// Tech 2 tries to delete Tech 1's folder -> must fail
	err = DeleteDeviceFolder(db, 0, lic2.LicenseID, folder1.FolderID)
	if err == nil {
		t.Fatalf("Tech 2 should NOT be able to delete Tech 1's folder")
	}
}
