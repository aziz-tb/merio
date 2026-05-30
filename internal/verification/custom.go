package verification

import (
	"encoding/json"
	"strconv"
	"strings"

	contractmodels "github.com/abhinavxd/libredesk/internal/contract/models"
	convmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	customermodels "github.com/abhinavxd/libredesk/internal/customer/models"
	"github.com/abhinavxd/libredesk/internal/verification/models"
)

// EvaluateCustom reports whether a custom criterion matches (design §7.4):
// resolve the value at Key from the contract (fallback customer); if no
// value, fail; otherwise look for it (case-insensitive substring) in the
// message parts named by search_in.
func EvaluateCustom(c models.CustomCriterion, ctx models.CriterionContext) bool {
	val, found := resolveCustomValue(c, ctx)
	if !found {
		return false
	}
	needle := strings.ToLower(strings.TrimSpace(val))
	if needle == "" {
		return false
	}
	for _, part := range c.SearchIn {
		hay := searchInText(part, ctx.Message)
		if hay != "" && strings.Contains(strings.ToLower(hay), needle) {
			return true
		}
	}
	return false
}

// resolveCustomValue resolves the value at c.Key from the contract first,
// then the customer. Returns the value as a string and whether it was found
// (a present-but-empty value counts as found="" so reason rendering can
// still substitute it).
func resolveCustomValue(c models.CustomCriterion, ctx models.CriterionContext) (string, bool) {
	if ctx.Contract != nil {
		if v, ok := resolvePath(toMap(ctx.Contract), c.Key); ok && v != nil {
			return asString(v), true
		}
	}
	if ctx.Customer != nil {
		if v, ok := resolvePath(toMap(ctx.Customer), c.Key); ok && v != nil {
			return asString(v), true
		}
	}
	return "", false
}

// searchInText returns the message text for a search_in token. For the email
// channel: sender → sender email, subject → subject, message → body. Unknown
// tokens return "" (no match).
func searchInText(part string, msg convmodels.PreparedMessage) string {
	switch strings.ToLower(strings.TrimSpace(part)) {
	case "sender":
		return msg.SenderEmail
	case "subject":
		return msg.Subject
	case "message":
		return msg.Body
	default:
		return ""
	}
}

// toMap JSON-round-trips a struct (Customer/Contract) into a navigable map,
// then merges the raw Zone 2 record so both top-level ERP keys and
// extra_data.* custom criteria paths resolve.
func toMap(v any) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	switch x := v.(type) {
	case *customermodels.Customer:
		mergeExtraData(m, x.ExtraData)
	case *contractmodels.Contract:
		mergeExtraData(m, x.ExtraData)
	case customermodels.Customer:
		mergeExtraData(m, x.ExtraData)
	case contractmodels.Contract:
		mergeExtraData(m, x.ExtraData)
	}
	return m
}

func mergeExtraData(out map[string]any, raw json.RawMessage) {
	if out == nil || len(raw) == 0 {
		return
	}
	var extra map[string]any
	if err := json.Unmarshal(raw, &extra); err != nil {
		return
	}
	for k, v := range extra {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	out["extra_data"] = extra
}

// resolvePath walks a dot-notation path (e.g. "extra_data.malo") through
// nested maps and arrays, mirroring Batch 7's universal-detection path
// syntax. Returns the leaf value, or (nil,false) if any segment is missing.
func resolvePath(root map[string]any, path string) (any, bool) {
	if root == nil || strings.TrimSpace(path) == "" {
		return nil, false
	}
	var current any = root
	for _, seg := range strings.Split(path, ".") {
		switch v := current.(type) {
		case map[string]any:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			current = v[idx]
		default:
			return nil, false
		}
	}
	return current, true
}
