package verification

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	contractmodels "github.com/aziz-tb/merio/internal/contract/models"
	custmodels "github.com/aziz-tb/merio/internal/customer/models"
	"github.com/aziz-tb/merio/internal/verification/models"
)

const phoneMinDigits = 7

// StandardCriterion evaluates one of the 11 built-in criteria against the
// context, returning whether it matched.
type StandardCriterion func(ctx models.CriterionContext) bool

// standardCriteria maps a criterion name to its evaluator. Adding a 12th is
// a one-line addition here.
var standardCriteria = map[string]StandardCriterion{
	"sender_email_matches_erp":           cSenderEmailMatchesERP,
	"mentioned_email_matches_erp":        cMentionedEmailMatchesERP,
	"phone_matches_erp":                  cPhoneMatchesERP,
	"contract_id_mentioned":              cContractIDMentioned,
	"customer_id_mentioned":              cCustomerIDMentioned,
	"firstname_mentioned":                cFirstnameMentioned,
	"lastname_mentioned":                 cLastnameMentioned,
	"exact_delivery_address_mentioned":   cExactDeliveryAddressMentioned,
	"exact_billing_address_mentioned":    cExactBillingAddressMentioned,
	"similar_delivery_address_mentioned": cSimilarDeliveryAddressMentioned,
	"similar_billing_address_mentioned":  cSimilarBillingAddressMentioned,
}

// EvaluateStandard dispatches to the named criterion. Unknown names return
// false (a rule referencing a removed criterion fails closed).
func EvaluateStandard(name string, ctx models.CriterionContext) bool {
	fn, ok := standardCriteria[name]
	if !ok {
		return false
	}
	return fn(ctx)
}

// --- the 11 standard criteria ---

// 1. sender_email_matches_erp — sender email equals the contract's email
// (or the customer's email).
func cSenderEmailMatchesERP(ctx models.CriterionContext) bool {
	s := normEmail(ctx.Message.SenderEmail)
	if s == "" {
		return false
	}
	for _, e := range erpEmails(ctx) {
		if normEmail(e) == s {
			return true
		}
	}
	return false
}

// 2. mentioned_email_matches_erp — any body email equals the contract's or
// customer's email.
func cMentionedEmailMatchesERP(ctx models.CriterionContext) bool {
	erp := erpEmails(ctx)
	for _, be := range ctx.Message.BodyEmails {
		bn := normEmail(be)
		if bn == "" {
			continue
		}
		for _, e := range erp {
			if normEmail(e) == bn {
				return true
			}
		}
	}
	return false
}

// 3. phone_matches_erp — a phone mentioned in the message body/subject
// matches the customer's or contract's phone. (Caller-ID matching applies to voice
// channels, which don't ship in v1.)
func cPhoneMatchesERP(ctx models.CriterionContext) bool {
	haystack := strings.TrimSpace(ctx.Message.Body + " " + ctx.Message.Subject)
	for _, ph := range erpPhones(ctx) {
		if containsPhoneDigits(haystack, ph) {
			return true
		}
	}
	return false
}

// 4. contract_id_mentioned — the ERP contract ID appears as a token in
// the message body or subject. Trusts the message itself, not upstream
// extraction (which may include email-lookup matches).
func cContractIDMentioned(ctx models.CriterionContext) bool {
	erpID := strings.TrimSpace(conContractID(ctx.Contract))
	if erpID == "" {
		return false
	}
	return containsToken(ctx.Message.Body, erpID) || containsToken(ctx.Message.Subject, erpID)
}

// 5. customer_id_mentioned — the ERP customer ID appears as a token in
// the message body or subject. Trusts the message itself, not upstream
// extraction.
func cCustomerIDMentioned(ctx models.CriterionContext) bool {
	erpID := strings.TrimSpace(custCustomerID(ctx.Customer))
	if erpID == "" {
		return false
	}
	return containsToken(ctx.Message.Body, erpID) || containsToken(ctx.Message.Subject, erpID)
}

// 6. firstname_mentioned — the customer's or contract's firstname appears
// as a token in the message body or subject.
func cFirstnameMentioned(ctx models.CriterionContext) bool {
	if name := strings.TrimSpace(conFirstname(ctx.Contract)); name != "" {
		if containsToken(ctx.Message.Body, name) || containsToken(ctx.Message.Subject, name) {
			return true
		}
	}
	if name := strings.TrimSpace(custFirstname(ctx.Customer)); name != "" {
		if containsToken(ctx.Message.Body, name) || containsToken(ctx.Message.Subject, name) {
			return true
		}
	}
	return false
}

// 7. lastname_mentioned — the customer's or contract's lastname appears
// as a token in the message body or subject.
func cLastnameMentioned(ctx models.CriterionContext) bool {
	if name := strings.TrimSpace(conLastname(ctx.Contract)); name != "" {
		if containsToken(ctx.Message.Body, name) || containsToken(ctx.Message.Subject, name) {
			return true
		}
	}
	if name := strings.TrimSpace(custLastname(ctx.Customer)); name != "" {
		if containsToken(ctx.Message.Body, name) || containsToken(ctx.Message.Subject, name) {
			return true
		}
	}
	return false
}

// 8. exact_delivery_address_mentioned — a full normalized ERP delivery
// address appears in the message body.
func cExactDeliveryAddressMentioned(ctx models.CriterionContext) bool {
	for _, addr := range deliveryAddrs(ctx) {
		if exactAddressInBody(ctx.Message.Body, addr) {
			return true
		}
	}
	return false
}

// 9. exact_billing_address_mentioned — same against billing addresses.
func cExactBillingAddressMentioned(ctx models.CriterionContext) bool {
	for _, addr := range billingAddrs(ctx) {
		if exactAddressInBody(ctx.Message.Body, addr) {
			return true
		}
	}
	return false
}

// 10. similar_delivery_address_mentioned — a meaningful delivery-address
// fragment appears in the message body.
func cSimilarDeliveryAddressMentioned(ctx models.CriterionContext) bool {
	for _, addr := range deliveryAddrs(ctx) {
		if containsAddressFragment(ctx.Message.Body, addr) {
			return true
		}
	}
	return false
}

// 11. similar_billing_address_mentioned — same against billing addresses.
func cSimilarBillingAddressMentioned(ctx models.CriterionContext) bool {
	for _, addr := range billingAddrs(ctx) {
		if containsAddressFragment(ctx.Message.Body, addr) {
			return true
		}
	}
	return false
}

// --- address match helpers ---

// containsToken reports whether needle appears in haystack as a whole token
// or token phrase. Boundaries are Unicode-aware letters/numbers, unlike
// regexp's ASCII \b.
func containsToken(haystack, needle string) bool {
	n := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(needle)), " "))
	if n == "" {
		return false
	}
	h := strings.ToLower(strings.Join(strings.Fields(haystack), " "))
	if h == "" {
		return false
	}
	re, err := regexp.Compile(`(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(n) + `([^\p{L}\p{N}]|$)`)
	if err != nil {
		return false
	}
	return re.MatchString(h)
}

// containsPhoneDigits reports whether the distinctive suffix of erpPhone is
// present in the message digit stream, tolerating +49/0049/0 prefix variants.
func containsPhoneDigits(body, erpPhone string) bool {
	n := normPhone(erpPhone)
	if len(n) < phoneMinDigits {
		return false
	}
	bodyDigits := normPhone(body)
	if bodyDigits == "" {
		return false
	}
	suffix := n
	if len(suffix) > 9 {
		suffix = suffix[len(suffix)-9:]
	}
	return strings.Contains(bodyDigits, suffix)
}

// containsAddressFragment reports whether a useful ERP address fragment
// appears in body. Useful fragments have both a number token and a word token
// so cities or names alone do not satisfy address criteria.
func containsAddressFragment(body, erpAddress string) bool {
	if strings.TrimSpace(erpAddress) == "" {
		return false
	}
	normBody := normAddr(body)
	if normBody == "" {
		return false
	}
	raw := strings.ReplaceAll(erpAddress, "/", ",")
	fragments := strings.Split(raw, ",")
	for _, frag := range fragments {
		f := normAddr(frag)
		if f == "" || !hasNumberAndWord(f) {
			continue
		}
		if strings.Contains(normBody, f) {
			return true
		}
	}
	return false
}

// exactAddressInBody reports whether the full normalized ERP address appears
// verbatim in the normalized message body.
func exactAddressInBody(body, erpAddress string) bool {
	a := normAddr(erpAddress)
	if a == "" {
		return false
	}
	return strings.Contains(normAddr(body), a)
}

// hasNumberAndWord reports whether s contains at least one number token and
// one word token, which is the minimum signal for address fragments like
// "Kastanienallee 47" or "10435 Berlin".
func hasNumberAndWord(s string) bool {
	hasNum := false
	hasWord := false
	for _, tok := range strings.Fields(s) {
		if tok == "" {
			continue
		}
		hasDigit := false
		for _, r := range tok {
			if r >= '0' && r <= '9' {
				hasDigit = true
				break
			}
		}
		if hasDigit {
			hasNum = true
		} else if len(tok) >= 3 {
			hasWord = true
		}
	}
	return hasNum && hasWord
}

// --- ERP field accessors (nil- and null-safe) ---

func erpEmails(ctx models.CriterionContext) []string {
	return nonEmpty(conEmail(ctx.Contract), custEmail(ctx.Customer))
}

func erpPhones(ctx models.CriterionContext) []string {
	return nonEmpty(conPhone(ctx.Contract), custPhone(ctx.Customer))
}

func deliveryAddrs(ctx models.CriterionContext) []string {
	return nonEmpty(conDelivery(ctx.Contract), custDelivery(ctx.Customer))
}

func billingAddrs(ctx models.CriterionContext) []string {
	return nonEmpty(conBilling(ctx.Contract), custBilling(ctx.Customer))
}

func custEmail(c *custmodels.Customer) string {
	if c == nil || !c.Email.Valid {
		return ""
	}
	return c.Email.String
}
func custPhone(c *custmodels.Customer) string {
	if c == nil || !c.Phone.Valid {
		return ""
	}
	return c.Phone.String
}
func custFirstname(c *custmodels.Customer) string {
	if c == nil || !c.Firstname.Valid {
		return ""
	}
	return c.Firstname.String
}
func custLastname(c *custmodels.Customer) string {
	if c == nil || !c.Lastname.Valid {
		return ""
	}
	return c.Lastname.String
}
func custDelivery(c *custmodels.Customer) string {
	if c == nil || !c.DeliveryAddress.Valid {
		return ""
	}
	return c.DeliveryAddress.String
}
func custBilling(c *custmodels.Customer) string {
	if c == nil || !c.BillingAddress.Valid {
		return ""
	}
	return c.BillingAddress.String
}
func custCustomerID(c *custmodels.Customer) string {
	if c == nil {
		return ""
	}
	return c.CustomerID
}

func conEmail(c *contractmodels.Contract) string {
	if c == nil || !c.Email.Valid {
		return ""
	}
	return c.Email.String
}
func conPhone(c *contractmodels.Contract) string {
	if c == nil || !c.Phone.Valid {
		return ""
	}
	return c.Phone.String
}
func conFirstname(c *contractmodels.Contract) string {
	if c == nil || !c.Firstname.Valid {
		return ""
	}
	return c.Firstname.String
}
func conLastname(c *contractmodels.Contract) string {
	if c == nil || !c.Lastname.Valid {
		return ""
	}
	return c.Lastname.String
}
func conDelivery(c *contractmodels.Contract) string {
	if c == nil || !c.DeliveryAddress.Valid {
		return ""
	}
	return c.DeliveryAddress.String
}
func conBilling(c *contractmodels.Contract) string {
	if c == nil || !c.BillingAddress.Valid {
		return ""
	}
	return c.BillingAddress.String
}
func conContractID(c *contractmodels.Contract) string {
	if c == nil {
		return ""
	}
	return c.ContractID
}

// --- string helpers ---

// asString coerces an extracted-attribute value to a string. JSON numbers
// arrive as float64; whole numbers render without a decimal/exponent so an
// id like 12606412 doesn't become "1.2606412e+07".
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == math.Trunc(t) && !math.IsInf(t, 0) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func normEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normPhone strips everything but digits so differently-formatted numbers
// compare equal (e.g. "0176/616 04237" vs "017661604237").
func normPhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normAddr trims, lowercases, and collapses internal whitespace.
func normAddr(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}
