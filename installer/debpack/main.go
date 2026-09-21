package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	binPath := flag.String("bin", "", "Chemin vers le binaire Linux compilé")
	pkgName := flag.String("pkg", "", "Nom du paquet Debian (ex: relaisdesk-configurator)")
	binName := flag.String("name", "", "Nom de l'exécutable dans /usr/bin")
	version := flag.String("version", "1.0.0", "Version du paquet")
	desc := flag.String("desc", "RelaisDesk - Support à distance", "Description du paquet")
	outPath := flag.String("out", "", "Chemin de sortie du fichier .deb")
	flag.Parse()

	if *binPath == "" || *pkgName == "" || *binName == "" || *outPath == "" {
		fmt.Println("Usage: debpack -bin <bin> -pkg <pkg> -name <name> -version <ver> -desc <desc> -out <out.deb>")
		os.Exit(1)
	}

	binData, err := os.ReadFile(*binPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur lecture binaire %s: %v\n", *binPath, err)
		os.Exit(1)
	}

	now := time.Now().UTC()

	// 1. debian-binary
	debBinary := []byte("2.0\n")

	// 2. control.tar.gz
	controlContent := fmt.Sprintf(`Package: %s
Version: %s
Section: utils
Priority: optional
Architecture: amd64
Maintainer: RelaisDesk <contact@relaisdesk.fr>
Description: %s
Installed-Size: %d
Recommends: zenity
`, *pkgName, *version, *desc, len(binData)/1024)

	var controlTarBuf bytes.Buffer
	gwControl := gzip.NewWriter(&controlTarBuf)
	twControl := tar.NewWriter(gwControl)

	controlHdr := &tar.Header{
		Name:     "./control",
		Mode:     0644,
		Size:     int64(len(controlContent)),
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	_ = twControl.WriteHeader(controlHdr)
	_, _ = twControl.Write([]byte(controlContent))
	if *pkgName == "relaisdesk-viewer" {
		script := viewerPreRemovalScript()
		if err := twControl.WriteHeader(&tar.Header{Name: "./prerm", Mode: 0755, Size: int64(len(script)), ModTime: now, Typeflag: tar.TypeReg}); err != nil {
			panic(err)
		}
		if _, err := twControl.Write([]byte(script)); err != nil {
			panic(err)
		}
		postScript := viewerPostInstallScript()
		if err := twControl.WriteHeader(&tar.Header{Name: "./postinst", Mode: 0755, Size: int64(len(postScript)), ModTime: now, Typeflag: tar.TypeReg}); err != nil {
			panic(err)
		}
		if _, err := twControl.Write([]byte(postScript)); err != nil {
			panic(err)
		}
	}
	_ = twControl.Close()
	_ = gwControl.Close()
	controlTarGz := controlTarBuf.Bytes()

	// 3. data.tar.gz
	var dataTarBuf bytes.Buffer
	gwData := gzip.NewWriter(&dataTarBuf)
	twData := tar.NewWriter(gwData)

	// Directories
	dirs := []string{"./usr", "./usr/bin", "./usr/share", "./usr/share/applications"}
	if *pkgName == "relaisdesk-viewer" {
		dirs = append(dirs, "./usr/share/polkit-1", "./usr/share/polkit-1/actions")
	}
	for _, dir := range dirs {
		_ = twData.WriteHeader(&tar.Header{
			Name:     dir,
			Mode:     0755,
			ModTime:  now,
			Typeflag: tar.TypeDir,
		})
	}

	// Executable in /usr/bin/
	binHdr := &tar.Header{
		Name:     "./usr/bin/" + *binName,
		Mode:     0755,
		Size:     int64(len(binData)),
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	_ = twData.WriteHeader(binHdr)
	_, _ = twData.Write(binData)

	// .desktop file
	useTerminal := "true"
	if *pkgName == "relaisdesk-viewer" {
		useTerminal = "false"
	}
	desktopContent := fmt.Sprintf(`[Desktop Entry]
Name=%s
Comment=%s
Exec=/usr/bin/%s
Terminal=%s
Type=Application
Categories=Utility;Network;
`, strings.Title(*pkgName), *desc, *binName, useTerminal)

	desktopHdr := &tar.Header{
		Name:     fmt.Sprintf("./usr/share/applications/%s.desktop", *pkgName),
		Mode:     0644,
		Size:     int64(len(desktopContent)),
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	_ = twData.WriteHeader(desktopHdr)
	_, _ = twData.Write([]byte(desktopContent))

	if *pkgName == "relaisdesk-viewer" {
		policyContent := viewerPolkitPolicy()
		policyHdr := &tar.Header{
			Name:     "./usr/share/polkit-1/actions/fr.relaisdesk.viewer.policy",
			Mode:     0644,
			Size:     int64(len(policyContent)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
		}
		_ = twData.WriteHeader(policyHdr)
		_, _ = twData.Write([]byte(policyContent))
	}

	_ = twData.Close()
	_ = gwData.Close()
	dataTarGz := dataTarBuf.Bytes()

	// 4. Assemble into AR format .deb archive
	if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur création dossier: %v\n", err)
		os.Exit(1)
	}

	outFile, err := os.CreateTemp(filepath.Dir(*outPath), ".relaisdesk-deb-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur création deb: %v\n", err)
		os.Exit(1)
	}
	tmpPath := outFile.Name()
	defer os.Remove(tmpPath)

	// Write global ar header
	if _, err := outFile.WriteString("!<arch>\n"); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur écriture deb: %v\n", err)
		os.Exit(1)
	}

	// Write members
	for _, member := range []struct {
		name string
		data []byte
	}{
		{name: "debian-binary", data: debBinary},
		{name: "control.tar.gz", data: controlTarGz},
		{name: "data.tar.gz", data: dataTarGz},
	} {
		if err := writeArMember(outFile, member.name, member.data, now); err != nil {
			_ = outFile.Close()
			fmt.Fprintf(os.Stderr, "Erreur écriture deb: %v\n", err)
			os.Exit(1)
		}
	}
	if err := outFile.Sync(); err != nil {
		_ = outFile.Close()
		fmt.Fprintf(os.Stderr, "Erreur finalisation deb: %v\n", err)
		os.Exit(1)
	}
	if err := outFile.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur fermeture deb: %v\n", err)
		os.Exit(1)
	}
	if err := os.Remove(*outPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Erreur remplacement deb: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(tmpPath, *outPath); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur installation deb: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Paquet Debian généré avec succès: %s (Taille: %.2f Mo)\n", *outPath, float64(len(binData)+len(controlTarGz)+len(dataTarGz))/(1024*1024))
}

func viewerPreRemovalScript() string {
	return strings.ReplaceAll(`#!/bin/sh
set -e
if [ "$1" = remove ]; then
    if [ -x /var/lib/relaisdesk-fleet/viewer-agent ]; then
        /var/lib/relaisdesk-fleet/viewer-agent --unenroll >/dev/null 2>&1 || true
    fi
    if [ -e /etc/systemd/system/relaisdesk-fleet.service ]; then
        systemctl stop relaisdesk-fleet.service >/dev/null 2>&1 || true
        systemctl disable relaisdesk-fleet.service >/dev/null 2>&1 || true
        rm -f /etc/systemd/system/relaisdesk-fleet.service
        systemctl daemon-reload >/dev/null 2>&1 || true
    fi
fi
exit 0
`, "\r\n", "\n")
}

func viewerPostInstallScript() string {
	return strings.ReplaceAll(`#!/bin/sh
set -e
if [ "$1" = configure ]; then
    if [ -d /var/lib/relaisdesk-fleet ] && [ -f /var/lib/relaisdesk-fleet/viewer-agent ]; then
        cp -f /usr/bin/relaisdesk-viewer /var/lib/relaisdesk-fleet/viewer-agent
        chmod 700 /var/lib/relaisdesk-fleet/viewer-agent
        if systemctl is-active --quiet relaisdesk-fleet.service; then
            systemctl restart relaisdesk-fleet.service >/dev/null 2>&1 || true
        fi
    fi
fi
exit 0
`, "\r\n", "\n")
}

func viewerPolkitPolicy() string {
	return strings.ReplaceAll(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE policyconfig PUBLIC
 "-//freedesktop//DTD PolicyKit Policy Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/PolicyKit/1/policyconfig.dtd">
<policyconfig>
  <vendor>RelaisDesk</vendor>
  <vendor_url>https://relaisdesk.fr</vendor_url>
  <action id="fr.relaisdesk.viewer.pkexec">
    <description>Exécuter RelaisDesk avec les privilèges administrateur</description>
    <description xml:lang="fr">Exécuter RelaisDesk avec les privilèges administrateur</description>
    <message>Authentification requise pour installer l'accès permanent RelaisDesk</message>
    <message xml:lang="fr">Authentification requise pour installer l'accès permanent RelaisDesk</message>
    <defaults>
      <allow_any>auth_admin</allow_any>
      <allow_inactive>auth_admin</allow_inactive>
      <allow_active>auth_admin</allow_active>
    </defaults>
    <annotate key="org.freedesktop.policykit.exec.path">/usr/bin/relaisdesk-viewer</annotate>
    <annotate key="org.freedesktop.policykit.exec.allow_gui">true</annotate>
  </action>
</policyconfig>
`, "\r\n", "\n")
}

func writeArMember(w io.Writer, name string, content []byte, modTime time.Time) error {
	// AR Header: 60 bytes
	// 0-15:  Filename (16 chars, ended with / for SysV/Debian)
	// 16-27: Mod time (12 chars decimal)
	// 28-33: Owner ID (6 chars decimal)
	// 34-39: Group ID (6 chars decimal)
	// 40-47: File mode (8 chars octal)
	// 48-57: File size (10 chars decimal)
	// 58-59: Header magic (\x60\x0a)

	hdrName := name
	if !strings.HasSuffix(hdrName, "/") {
		hdrName += "/"
	}
	hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
		hdrName,
		modTime.Unix(),
		0,
		0,
		0100644,
		len(content),
	)

	if _, err := w.Write([]byte(hdr)); err != nil {
		return err
	}
	if _, err := w.Write(content); err != nil {
		return err
	}

	// If odd length, pad with a newline
	if len(content)%2 != 0 {
		if _, err := w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}
