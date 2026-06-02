// Package token manages short-lived authentication tokens that scripts
// running inside the Python sandbox use to call back into Merio.
//
// The token is presented as a normal `Authorization: Token <key>:<secret>`
// header (the format Merio's existing API-key auth already parses).
// The key has the prefix `sx_` so authenticateUser can dispatch to this
// package instead of the API-key user lookup. Both halves of the tuple
// are the full token string — the "secret" half is meaningless for these
// short-lived tokens, but using the existing Token-auth shape avoids
// inventing a new auth scheme.
//
// Tokens are stored in Redis. Resolution does not touch the DB. The user
// IDs the token authenticates against (invoker, system) are resolved
// against the user manager separately by the caller (authenticateUser
// loads the full user record for permission checks).
package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aziz-tb/merio/internal/exec/token/models"
	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

// TokenPrefix is the marker that lets authenticateUser recognize a script
// token vs an ordinary API key. All script tokens are "sx_" + hex.
const TokenPrefix = "sx_"

// redisKeyPrefix is what we put in Redis. We keep the "sx_" portion in the
// key so it's easy to spot (and eyeball-debug) what's there.
const redisKeyPrefix = "exec:token:"

// Manager owns the Redis-backed token store.
type Manager struct {
	rd *redis.Client
	lo *logf.Logger
}

// Opts are the dependencies Manager needs.
type Opts struct {
	Redis  *redis.Client
	Logger *logf.Logger
}

// New constructs a Manager. The same Manager is shared across all
// dispatchers; it's stateless apart from its Redis handle.
func New(opts Opts) (*Manager, error) {
	if opts.Redis == nil {
		return nil, errors.New("token: nil redis client")
	}
	if opts.Logger == nil {
		return nil, errors.New("token: nil logger")
	}
	return &Manager{rd: opts.Redis, lo: opts.Logger}, nil
}

// Issue creates a new token, stores it in Redis with the supplied TTL,
// and returns the bare Token struct (so callers can hand the token's
// Value to the sandbox as the callback secret).
//
// Concurrency: Issue is safe to call concurrently. Each call generates a
// fresh random Value, so collisions are practically impossible.
func (m *Manager) Issue(ctx context.Context, req models.IssueRequest) (*models.Token, error) {
	if req.InvokerUserID == 0 {
		return nil, errors.New("token: invoker user id is required")
	}
	if req.SystemUserID == 0 {
		return nil, errors.New("token: system user id is required")
	}
	if req.ExecutionID == "" {
		return nil, errors.New("token: execution id is required")
	}
	if req.TTL <= 0 {
		return nil, errors.New("token: ttl must be positive")
	}

	value, err := generateTokenValue()
	if err != nil {
		return nil, fmt.Errorf("token: generate value: %w", err)
	}

	tok := &models.Token{
		Value:          value,
		InvokerUserID:  req.InvokerUserID,
		SystemUserID:   req.SystemUserID,
		ExecutionID:    req.ExecutionID,
		ConversationID: req.ConversationID,
		ToolDepth:      req.ToolDepth,
		ToolName:       req.ToolName,
		TraceID:        req.TraceID,
		ExpiresAt:      time.Now().Add(req.TTL),
	}

	payload, err := json.Marshal(tok)
	if err != nil {
		return nil, fmt.Errorf("token: marshal: %w", err)
	}

	key := redisKey(value)
	if err := m.rd.Set(ctx, key, payload, req.TTL).Err(); err != nil {
		return nil, fmt.Errorf("token: redis set: %w", err)
	}

	return tok, nil
}

// Resolve looks up a token by the prefixed value the script presented
// (e.g. "sx_a1b2c3..."). It returns a ResolveResult with the user ID the
// caller should authenticate as, picked according to mode.
//
// Returns an error wrapping ErrTokenNotFound when:
//   - the token value doesn't have the sx_ prefix (caller bug)
//   - the token isn't in Redis (expired, revoked, or never issued)
//   - the token's stored ExpiresAt is in the past (defensive check; should
//     not normally happen because Redis TTL would have already expired it)
//
// Returns ErrInvalidActorMode if mode is empty or unrecognized.
func (m *Manager) Resolve(
	ctx context.Context,
	prefixedValue string,
	mode models.ActorMode,
) (*models.ResolveResult, error) {
	if !strings.HasPrefix(prefixedValue, TokenPrefix) {
		return nil, fmt.Errorf("token: missing prefix: %w", ErrTokenNotFound)
	}
	value := strings.TrimPrefix(prefixedValue, TokenPrefix)
	if value == "" {
		return nil, fmt.Errorf("token: empty value: %w", ErrTokenNotFound)
	}

	raw, err := m.rd.Get(ctx, redisKey(value)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("token: redis get: %w", err)
	}

	var tok models.Token
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		// Corruption in Redis — log and surface as not-found so the caller
		// uniformly handles "this token isn't usable" without inspecting
		// the underlying cause.
		m.lo.Error("corrupted token in redis", "key", redisKey(value), "error", err)
		return nil, fmt.Errorf("token: unmarshal: %w", ErrTokenNotFound)
	}

	if time.Now().After(tok.ExpiresAt) {
		// Defensive: the Redis TTL should have evicted by now. Either the
		// clock skewed or someone bumped the value's TTL. Refuse.
		return nil, fmt.Errorf("token: stored expiry passed: %w", ErrTokenNotFound)
	}

	res := &models.ResolveResult{Token: &tok, Mode: mode}
	switch mode {
	case models.ActorInvoker:
		res.UserID = tok.InvokerUserID
	case models.ActorSystem:
		res.UserID = tok.SystemUserID
	default:
		return nil, ErrInvalidActorMode
	}
	return res, nil
}

// Revoke deletes a token from Redis. Idempotent — deleting a missing key
// is not an error. Called by the dispatcher when an execution finishes
// (regardless of outcome) so we don't leak short-lived tokens past their
// useful life.
func (m *Manager) Revoke(ctx context.Context, prefixedValue string) error {
	value := strings.TrimPrefix(prefixedValue, TokenPrefix)
	if value == "" {
		return nil
	}
	if err := m.rd.Del(ctx, redisKey(value)).Err(); err != nil {
		return fmt.Errorf("token: redis del: %w", err)
	}
	return nil
}

// Errors returned by Manager.

var (
	// ErrTokenNotFound covers all "token isn't usable" cases (missing,
	// expired, malformed). The middleware rejects with 401 in every case;
	// distinguishing the cause would just leak information.
	ErrTokenNotFound = errors.New("token: not found")

	// ErrInvalidActorMode is returned when Resolve is called with an
	// unknown mode. This indicates a programming error in the caller;
	// the middleware should validate the X-Script-System header value
	// before mapping to ActorMode.
	ErrInvalidActorMode = errors.New("token: invalid actor mode")
)

// generateTokenValue returns a hex-encoded random string suitable for use
// as a token value. 32 bytes of entropy → 64 hex chars; collision-free
// for any realistic execution rate.
func generateTokenValue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// redisKey builds the Redis key for a token's bare value (without prefix).
func redisKey(bareValue string) string {
	return redisKeyPrefix + bareValue
}

// PrefixedValue returns the form of Token.Value that the dispatcher hands
// to the sandbox (and that the SDK puts in its Authorization header):
// "sx_<value>". Callers should never construct this string by hand.
func PrefixedValue(t *models.Token) string {
	return TokenPrefix + t.Value
}
