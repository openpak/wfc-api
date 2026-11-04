package database

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"openpak/wfc-common/logging"
)

// A Wii or DS cannot sign in, so linking one to an OpenPak account is a device-code flow:
// when an unlinked console logs in, the server issues a short pairing code, the game shows it
// (Mario Kart Wii prints it in its message of the day), and the player enters it at
// openpak.org/link. Codes live in Postgres, expire, and work once. The console is the WFC
// user_id; wfc_pairing_linked remembers which consoles are already linked so they are not
// offered a code again.

const (
	// PairingTTL is how long a code stays valid after it is issued.
	PairingTTL = 30 * time.Minute

	pairingAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 32 symbols, no I/O/0/1
	pairingLength   = 8
)

var (
	// ErrPairingCode is a code that is unknown, expired or already used. The three are one
	// answer on purpose: telling them apart would help someone guessing codes.
	ErrPairingCode = errors.New("pairing code unknown, expired or used")
)

const createPairing = `
CREATE TABLE IF NOT EXISTS wfc_pairing_codes (
	code text PRIMARY KEY,
	user_id bigint NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	expires_at timestamptz NOT NULL,
	used_at timestamptz
);
CREATE INDEX IF NOT EXISTS wfc_pairing_codes_user ON wfc_pairing_codes (user_id);
CREATE TABLE IF NOT EXISTS wfc_pairing_linked (
	user_id bigint PRIMARY KEY,
	linked_at timestamptz NOT NULL DEFAULT now()
);
`

// EnsurePairing creates the pairing tables.
func (c *Connection) EnsurePairing() error {
	_, err := c.pool.Exec(c.ctx, createPairing)
	if err != nil {
		logging.Error("DB", "pairing tables:", err)
	}
	return err
}

// NewPairingCode returns a random code of pairingLength symbols. The alphabet has 32
// symbols, a divisor of 256, so every byte maps evenly and there is no modulo bias.
func NewPairingCode() string {
	var raw [pairingLength]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, pairingLength)
	for i, b := range raw {
		out[i] = pairingAlphabet[int(b)%len(pairingAlphabet)]
	}
	return string(out)
}

// NormalizePairingCode upper-cases a typed code and drops spaces and dashes. ok is false when
// what is left cannot be a code.
func NormalizePairingCode(raw string) (string, bool) {
	c := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(raw)))
	if len(c) != pairingLength {
		return "", false
	}
	for _, r := range c {
		if !strings.ContainsRune(pairingAlphabet, r) {
			return "", false
		}
	}
	return c, true
}

// FormatPairingCode writes a code as XXXX-XXXX for display.
func FormatPairingCode(code string) string {
	if len(code) != pairingLength {
		return code
	}
	return code[:4] + "-" + code[4:]
}

// IssuePairingCode returns the unlinked console's code: the live one if it has one, a new one
// otherwise. A console that is already linked gets "", nil.
func (c *Connection) IssuePairingCode(ctx context.Context, userID uint64) (string, error) {
	var linked bool
	if err := c.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wfc_pairing_linked WHERE user_id = $1)`, int64(userID)).Scan(&linked); err != nil {
		return "", err
	}
	if linked {
		return "", nil
	}
	var code string
	err := c.pool.QueryRow(ctx,
		`SELECT code FROM wfc_pairing_codes WHERE user_id = $1 AND used_at IS NULL AND expires_at > now() ORDER BY created_at DESC LIMIT 1`,
		int64(userID)).Scan(&code)
	if err == nil {
		return code, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	// Expired and used rows are dead weight; drop them as we go.
	_, _ = c.pool.Exec(ctx, `DELETE FROM wfc_pairing_codes WHERE expires_at < now() - interval '1 day'`)
	for attempt := 0; attempt < 5; attempt++ {
		code = NewPairingCode()
		tag, err := c.pool.Exec(ctx,
			`INSERT INTO wfc_pairing_codes (code, user_id, expires_at) VALUES ($1, $2, now() + make_interval(secs => $3)) ON CONFLICT DO NOTHING`,
			code, int64(userID), PairingTTL.Seconds())
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 1 {
			return code, nil
		}
	}
	return "", errors.New("could not find a free pairing code")
}

// PairingTarget is the console a live code belongs to.
type PairingTarget struct {
	UserID    uint64
	ExpiresAt time.Time
	GameCode  string // the game profile that signed in last, for the confirm screen
	Name      string
}

// LookupPairingCode reports the console behind a live code without spending it.
func (c *Connection) LookupPairingCode(ctx context.Context, code string) (PairingTarget, error) {
	code, ok := NormalizePairingCode(code)
	if !ok {
		return PairingTarget{}, ErrPairingCode
	}
	var uid int64
	var t PairingTarget
	err := c.pool.QueryRow(ctx,
		`SELECT user_id, expires_at FROM wfc_pairing_codes WHERE code = $1 AND used_at IS NULL AND expires_at > now()`,
		code).Scan(&uid, &t.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PairingTarget{}, ErrPairingCode
	}
	if err != nil {
		return PairingTarget{}, err
	}
	t.UserID = uint64(uid)
	_ = c.pool.QueryRow(ctx,
		`SELECT gsbrcd, last_ingamesn FROM users WHERE user_id = $1 ORDER BY profile_id LIMIT 1`, uid).Scan(&t.GameCode, &t.Name)
	return t, nil
}

// RedeemPairingCode spends a live code and marks its console linked, in one statement, so two
// players entering the same code cannot both win.
func (c *Connection) RedeemPairingCode(ctx context.Context, code string) (uint64, error) {
	code, ok := NormalizePairingCode(code)
	if !ok {
		return 0, ErrPairingCode
	}
	var uid int64
	err := c.pool.QueryRow(ctx, `
WITH spent AS (
	UPDATE wfc_pairing_codes SET used_at = now()
	WHERE code = $1 AND used_at IS NULL AND expires_at > now()
	RETURNING user_id
), mark AS (
	INSERT INTO wfc_pairing_linked (user_id) SELECT user_id FROM spent ON CONFLICT DO NOTHING
)
SELECT user_id FROM spent`, code).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrPairingCode
	}
	if err != nil {
		return 0, err
	}
	return uint64(uid), nil
}

// ForgetPairing marks a console unlinked again (the account removed the link), so its next
// login is offered a code.
func (c *Connection) ForgetPairing(ctx context.Context, userID uint64) error {
	_, err := c.pool.Exec(ctx, `DELETE FROM wfc_pairing_linked WHERE user_id = $1`, int64(userID))
	return err
}
