// Package auth hashes passwords (PBKDF2-SHA256) and signs session cookies.
package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const iterations = 310000

// HashPassword returns "pbkdf2-sha256$iter$salt$hash".
func HashPassword(pw string) (string, error) {
	if len([]rune(pw)) < 8 {
		return "", errors.New("password minimal 8 karakter")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, iterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func CheckPassword(hash, pw string) bool {
	p := strings.Split(hash, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(p[1])
	if err != nil || iter < 1000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(p[2])
	want, err2 := base64.RawStdEncoding.DecodeString(p[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Sessions are "uid.expiry.signature"; the signature covers uid, expiry and
// the password hash prefix, so changing the password logs out other devices.
type Sessions struct{ Key []byte }

func (s Sessions) sign(uid int64, exp int64, pwHash string) string {
	m := hmac.New(sha256.New, s.Key)
	fmt.Fprintf(m, "%d.%d.%s", uid, exp, pwHash)
	return hex.EncodeToString(m.Sum(nil))
}

func (s Sessions) Issue(uid int64, pwHash string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	return fmt.Sprintf("%d.%d.%s", uid, exp, s.sign(uid, exp, pwHash))
}

// Parse returns the user id if the token is well formed and not expired;
// Verify must still be called with the user's current password hash.
func (s Sessions) Parse(tok string) (uid int64, exp int64, sig string, ok bool) {
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		return 0, 0, "", false
	}
	uid, err1 := strconv.ParseInt(p[0], 10, 64)
	exp, err2 := strconv.ParseInt(p[1], 10, 64)
	if err1 != nil || err2 != nil || time.Now().Unix() > exp {
		return 0, 0, "", false
	}
	return uid, exp, p[2], true
}

func (s Sessions) Verify(uid, exp int64, sig, pwHash string) bool {
	return hmac.Equal([]byte(sig), []byte(s.sign(uid, exp, pwHash)))
}

// Token returns a random URL-safe token (engine API key, etc.).
func Token(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Roles in order of power.
var Roles = []string{"kontributor", "wartawan", "editor", "pemred", "admin"}

var RoleLabels = map[string]string{"kontributor": "Kontributor", "wartawan": "Wartawan", "editor": "Editor", "pemred": "Pemimpin Redaksi", "admin": "Admin"}

func rank(r string) int {
	for i, x := range Roles {
		if x == r {
			return i
		}
	}
	return -1
}

// CanPublish: editors and above approve and publish.
func CanPublish(role string) bool { return rank(role) >= rank("editor") }

// CanManage: pemred and admin manage team, regions, pages, settings.
func CanManage(role string) bool { return rank(role) >= rank("pemred") }
