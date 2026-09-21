package database

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	backupMagic     = "RDBKAES2"
	backupChunkSize = 1024 * 1024
)

// OpenBackupSource opens the live database without applying migrations or
// changing pragmas. The dedicated backup process must remain read-only with
// respect to the production schema.
func OpenBackupSource(dbPath string) (*sql.DB, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("chemin de base requis")
	}
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func LoadBackupKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	value := strings.TrimSpace(string(raw))
	var key []byte
	for _, decoder := range []func(string) ([]byte, error){base64.RawURLEncoding.DecodeString, base64.StdEncoding.DecodeString, hex.DecodeString} {
		decoded, decodeErr := decoder(value)
		if decodeErr == nil && len(decoded) == 32 {
			key = decoded
			break
		}
	}
	if len(key) != 32 {
		return nil, errors.New("la clé de sauvegarde doit contenir exactement 32 octets encodés en base64url, base64 ou hexadécimal")
	}
	return key, nil
}

func GenerateBackupKeyFile(path string) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.WriteString(file, base64.RawURLEncoding.EncodeToString(key)+"\n")
	return err
}

// CreateConsistentSnapshot uses SQLite itself rather than copying a live WAL
// database file. VACUUM INTO produces a transactionally consistent snapshot.
func CreateConsistentSnapshot(db *sql.DB, destination string) error {
	if db == nil {
		return errors.New("base SQLite requise")
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err == nil {
		return errors.New("le fichier de sauvegarde existe déjà")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return err
	}
	quoted := "'" + strings.ReplaceAll(filepath.ToSlash(abs), "'", "''") + "'"
	if _, err := db.Exec("VACUUM INTO " + quoted); err != nil {
		return fmt.Errorf("instantané SQLite cohérent: %w", err)
	}
	return os.Chmod(abs, 0600)
}

func VerifySQLiteBackup(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", abs+"?_query_only=1&_foreign_keys=on")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("échec integrity_check: %s", integrity)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("la sauvegarde contient une violation de clé étrangère")
	}
	return rows.Err()
}

func EncryptBackupFile(source, destination string, key []byte) error {
	if len(key) != 32 {
		return errors.New("clé AES-256 invalide")
	}
	input, err := os.Open(filepath.Clean(source))
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(filepath.Clean(destination), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(filepath.Clean(destination))
		}
	}()

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	noncePrefix := make([]byte, 8)
	if _, err := rand.Read(noncePrefix); err != nil {
		return err
	}
	writer := bufio.NewWriter(output)
	if _, err := writer.WriteString(backupMagic); err != nil {
		return err
	}
	if _, err := writer.Write(noncePrefix); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(backupChunkSize)); err != nil {
		return err
	}
	buffer := make([]byte, backupChunkSize)
	var counter uint32
	for {
		count, readErr := io.ReadFull(input, buffer)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return readErr
		}
		if count == 0 {
			break
		}
		nonce := make([]byte, aead.NonceSize())
		copy(nonce, noncePrefix)
		binary.BigEndian.PutUint32(nonce[8:], counter)
		sealed := aead.Seal(nil, nonce, buffer[:count], backupChunkAAD(uint32(backupChunkSize), counter, uint32(count), false))
		if err := binary.Write(writer, binary.BigEndian, uint32(count)); err != nil {
			return err
		}
		if _, err := writer.Write(sealed); err != nil {
			return err
		}
		if counter == ^uint32(0) {
			return errors.New("sauvegarde trop volumineuse")
		}
		counter++
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(0)); err != nil {
		return err
	}
	finalNonce := make([]byte, aead.NonceSize())
	copy(finalNonce, noncePrefix)
	binary.BigEndian.PutUint32(finalNonce[8:], counter)
	finalTag := aead.Seal(nil, finalNonce, nil, backupChunkAAD(uint32(backupChunkSize), counter, 0, true))
	if _, err := writer.Write(finalTag); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	committed = true
	return nil
}

func DecryptBackupFile(source, destination string, key []byte) error {
	if len(key) != 32 {
		return errors.New("clé AES-256 invalide")
	}
	input, err := os.Open(filepath.Clean(source))
	if err != nil {
		return err
	}
	defer input.Close()
	reader := bufio.NewReader(input)
	magic := make([]byte, len(backupMagic))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != backupMagic {
		return errors.New("format de sauvegarde chiffrée invalide")
	}
	noncePrefix := make([]byte, 8)
	if _, err := io.ReadFull(reader, noncePrefix); err != nil {
		return err
	}
	var chunkSize uint32
	if err := binary.Read(reader, binary.BigEndian, &chunkSize); err != nil || chunkSize == 0 || chunkSize > 16*1024*1024 {
		return errors.New("taille de bloc de sauvegarde invalide")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(filepath.Clean(destination), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(filepath.Clean(destination))
		}
	}()
	writer := bufio.NewWriter(output)
	var counter uint32
	for {
		var plainLength uint32
		if err := binary.Read(reader, binary.BigEndian, &plainLength); err != nil {
			return err
		}
		if plainLength == 0 {
			finalTag := make([]byte, aead.Overhead())
			if _, err := io.ReadFull(reader, finalTag); err != nil {
				return errors.New("fin de sauvegarde authentifiée absente")
			}
			finalNonce := make([]byte, aead.NonceSize())
			copy(finalNonce, noncePrefix)
			binary.BigEndian.PutUint32(finalNonce[8:], counter)
			if _, err := aead.Open(nil, finalNonce, finalTag, backupChunkAAD(chunkSize, counter, 0, true)); err != nil {
				return errors.New("fin de sauvegarde invalide: fichier tronqué ou altéré")
			}
			if _, err := reader.ReadByte(); err == nil {
				return errors.New("données inattendues après la fin de la sauvegarde")
			} else if !errors.Is(err, io.EOF) {
				return err
			}
			break
		}
		if plainLength > chunkSize {
			return errors.New("bloc chiffré trop volumineux")
		}
		sealed := make([]byte, int(plainLength)+aead.Overhead())
		if _, err := io.ReadFull(reader, sealed); err != nil {
			return err
		}
		nonce := make([]byte, aead.NonceSize())
		copy(nonce, noncePrefix)
		binary.BigEndian.PutUint32(nonce[8:], counter)
		plain, err := aead.Open(nil, nonce, sealed, backupChunkAAD(chunkSize, counter, plainLength, false))
		if err != nil {
			return errors.New("authentification de la sauvegarde impossible: clé incorrecte ou fichier altéré")
		}
		if _, err := writer.Write(plain); err != nil {
			return err
		}
		if counter == ^uint32(0) {
			return errors.New("sauvegarde trop volumineuse")
		}
		counter++
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	committed = true
	return nil
}

func backupChunkAAD(chunkSize, counter, plainLength uint32, final bool) []byte {
	aad := make([]byte, len(backupMagic)+13)
	copy(aad, backupMagic)
	binary.BigEndian.PutUint32(aad[len(backupMagic):], chunkSize)
	binary.BigEndian.PutUint32(aad[len(backupMagic)+4:], counter)
	binary.BigEndian.PutUint32(aad[len(backupMagic)+8:], plainLength)
	if final {
		aad[len(aad)-1] = 1
	}
	return aad
}

func CopyBackupFile(source, destination string) error {
	input, err := os.Open(filepath.Clean(source))
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(filepath.Clean(destination), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(filepath.Clean(destination))
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	ok = true
	return nil
}
