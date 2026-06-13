package verification

import (
	"strconv"
	"strings"

	"github.com/aziz-tb/merio/internal/verification/models"
)

type criterionMessageTemplate struct {
	Matched string
	Failed  string
}

type partialGroupMessage struct {
	Label      string
	Required   int
	Matched    int
	Successful []models.CriterionResult
	Failed     []models.CriterionResult
}

var standardCriterionMessageTemplates = map[string]criterionMessageTemplate{
	"sender_email_matches_erp": {
		Matched: "Absenderadresse **{sender_email}** stimmt mit hinterlegter E-Mail **{erp_email}** überein.",
		Failed:  "Absenderadresse **{sender_email}** stimmt nicht mit hinterlegter E-Mail **{erp_email}** überein.",
	},
	"mentioned_email_matches_erp": {
		Matched: "In der Nachricht erwähnte E-Mail **{mentioned_email}** stimmt mit hinterlegter E-Mail **{erp_email}** überein.",
		Failed:  "Keine in der Nachricht erwähnte E-Mail stimmt mit hinterlegter E-Mail **{erp_email}** überein.",
	},
	"phone_matches_erp": {
		Matched: "Hinterlegte Telefonnummer **{erp_phone}** wurde in der Nachricht erkannt.",
		Failed:  "Hinterlegte Telefonnummer **{erp_phone}** wurde nicht in der Nachricht erkannt.",
	},
	"contract_id_mentioned": {
		Matched: "Vertragsnummer **{contract_id}** wurde in der Nachricht erwähnt.",
		Failed:  "Vertragsnummer **{contract_id}** wurde nicht in der Nachricht erwähnt.",
	},
	"customer_id_mentioned": {
		Matched: "Kundennummer **{customer_id}** wurde in der Nachricht erwähnt.",
		Failed:  "Kundennummer **{customer_id}** wurde nicht in der Nachricht erwähnt.",
	},
	"firstname_mentioned": {
		Matched: "Vorname **{firstname}** wurde in der Nachricht erwähnt.",
		Failed:  "Vorname **{firstname}** wurde nicht in der Nachricht erwähnt.",
	},
	"lastname_mentioned": {
		Matched: "Nachname **{lastname}** wurde in der Nachricht erwähnt.",
		Failed:  "Nachname **{lastname}** wurde nicht in der Nachricht erwähnt.",
	},
	"exact_delivery_address_mentioned": {
		Matched: "Vollständige Lieferadresse **{delivery_address}** wurde in der Nachricht erwähnt.",
		Failed:  "Vollständige Lieferadresse **{delivery_address}** wurde nicht in der Nachricht erwähnt.",
	},
	"exact_billing_address_mentioned": {
		Matched: "Vollständige Rechnungsadresse **{billing_address}** wurde in der Nachricht erwähnt.",
		Failed:  "Vollständige Rechnungsadresse **{billing_address}** wurde nicht in der Nachricht erwähnt.",
	},
	"similar_delivery_address_mentioned": {
		Matched: "Teil der Lieferadresse **{delivery_address}** wurde in der Nachricht erwähnt.",
		Failed:  "Kein ausreichend eindeutiger Teil der Lieferadresse **{delivery_address}** wurde in der Nachricht erkannt.",
	},
	"similar_billing_address_mentioned": {
		Matched: "Teil der Rechnungsadresse **{billing_address}** wurde in der Nachricht erwähnt.",
		Failed:  "Kein ausreichend eindeutiger Teil der Rechnungsadresse **{billing_address}** wurde in der Nachricht erkannt.",
	},
}

func buildPartialVerificationMessage(groups []partialGroupMessage, ctx models.CriterionContext) string {
	var sections []string
	for _, g := range groups {
		label := strings.TrimSpace(g.Label)
		if label == "" {
			label = "Verifizierungsgruppe"
		}

		var b strings.Builder
		b.WriteString("Teilweise verifiziert: ")
		b.WriteString(strconv.Itoa(g.Matched))
		b.WriteString(" von ")
		b.WriteString(strconv.Itoa(g.Required))
		b.WriteString(" erforderlichen Kriterien in **")
		b.WriteString(escapeMarkdown(label))
		b.WriteString("** erfüllt.")

		if len(g.Successful) > 0 {
			b.WriteString("\n\n**Erfüllt:**")
			for _, res := range g.Successful {
				b.WriteString("\n- ")
				b.WriteString(criterionMessage(res, true, ctx))
			}
		}

		if len(g.Failed) > 0 {
			b.WriteString("\n\n**Nicht erfüllt:**")
			for _, res := range g.Failed {
				b.WriteString("\n- ")
				b.WriteString(criterionMessage(res, false, ctx))
			}
		}
		sections = append(sections, b.String())
	}
	return strings.Join(sections, "\n\n---\n\n")
}

func criterionMessage(res models.CriterionResult, matched bool, ctx models.CriterionContext) string {
	if res.Type == "custom" {
		label := strings.TrimSpace(res.Label)
		if label == "" {
			label = "Benutzerdefiniertes Kriterium"
		}
		if matched {
			return "Benutzerdefiniertes Kriterium **" + escapeMarkdown(label) + "** erfüllt."
		}
		if strings.TrimSpace(res.Reason) != "" {
			return res.Reason
		}
		return "Benutzerdefiniertes Kriterium **" + escapeMarkdown(label) + "** nicht erfüllt."
	}

	tpl, ok := standardCriterionMessageTemplates[res.Name]
	if !ok {
		name := strings.TrimSpace(res.Name)
		if name == "" {
			name = "Unbekanntes Kriterium"
		}
		if matched {
			return "Standardkriterium **" + escapeMarkdown(name) + "** erfüllt."
		}
		return "Standardkriterium **" + escapeMarkdown(name) + "** nicht erfüllt."
	}
	if matched {
		return renderCriterionTemplate(tpl.Matched, ctx)
	}
	return renderCriterionTemplate(tpl.Failed, ctx)
}

func renderCriterionTemplate(tpl string, ctx models.CriterionContext) string {
	replacements := map[string]string{
		"{sender_email}":     messageValue(ctx.Message.SenderEmail),
		"{mentioned_email}":  messageValue(strings.Join(ctx.Message.BodyEmails, ", ")),
		"{erp_email}":        messageValue(formatERPValuesForMessage(erpEmails(ctx), normEmail)),
		"{erp_phone}":        messageValue(formatERPValuesForMessage(erpPhones(ctx), normPhone)),
		"{contract_id}":      messageValue(firstNonEmpty(asString(ctx.ExtractedAttrs["contractId"]), conContractID(ctx.Contract))),
		"{customer_id}":      messageValue(firstNonEmpty(asString(ctx.ExtractedAttrs["customerId"]), custCustomerID(ctx.Customer))),
		"{firstname}":        messageValue(firstNonEmpty(conFirstname(ctx.Contract), custFirstname(ctx.Customer))),
		"{lastname}":         messageValue(firstNonEmpty(conLastname(ctx.Contract), custLastname(ctx.Customer))),
		"{delivery_address}": messageValue(formatERPValuesForMessage(deliveryAddrs(ctx), normAddr)),
		"{billing_address}":  messageValue(formatERPValuesForMessage(billingAddrs(ctx), normAddr)),
	}
	out := tpl
	for key, val := range replacements {
		out = strings.ReplaceAll(out, key, val)
	}
	return out
}

// formatERPValuesForMessage renders ERP field values for a message, using
// normalize to dedupe case/format-insensitively, and joins distinct values
// with " bzw. " (German "or respectively") for when contract and customer
// fields differ.
func formatERPValuesForMessage(values []string, normalize func(string) string) string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		key := normalize(trimmed)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, trimmed)
	}
	return strings.Join(unique, " bzw. ")
}

func messageValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "nicht verfügbar"
	}
	return escapeMarkdown(value)
}

func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`*`, `\*`,
		`_`, `\_`,
		`[`, `\[`,
		`]`, `\]`,
		`(`, `\(`,
		`)`, `\)`,
		"`", "\\`",
		`#`, `\#`,
		`+`, `\+`,
		`-`, `\-`,
		`.`, `\.`,
		`!`, `\!`,
		`{`, `\{`,
		`}`, `\}`,
		`|`, `\|`,
		`>`, `\>`,
	)
	return replacer.Replace(s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
