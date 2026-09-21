package handlers

// MaskLicenseKey masks a license key for safe logging.
func MaskLicenseKey(key string) string {
	if len(key) < 10 {
		return "****"
	}
	return key[:6] + "..." + key[len(key)-4:]
}

func MaskLicenseID(id string) string {
	if len(id) < 7 {
		return "****"
	}
	return id[:3] + "****" + id[len(id)-4:]
}
