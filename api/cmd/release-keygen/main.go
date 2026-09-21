package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	output := flag.String("out", "", "fichier privé à créer hors du dépôt")
	flag.Parse()
	if *output == "" {
		log.Fatal("-out est obligatoire")
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o700); err != nil {
		log.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		log.Fatalf("création exclusive du fichier privé: %v", err)
	}
	if _, err := file.WriteString(base64.RawURLEncoding.EncodeToString(privateKey) + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(*output)
		log.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(*output)
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(*output)
		log.Fatal(err)
	}
	fmt.Printf("RELEASE_PUBLIC_KEY=%s\n", base64.RawURLEncoding.EncodeToString(publicKey))
}
