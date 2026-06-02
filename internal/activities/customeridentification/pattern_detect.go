package customeridentification

import (
	"context"
	"database/sql"
	"regexp"
	"strings"

	cimodels "github.com/abhinavxd/libredesk/internal/workflows/customeridentification"
)

type freshCustomerEmailRow struct {
	CustomerID string `db:"customer_id"`
}

type freshContractEmailRow struct {
	ContractID string `db:"contract_id"`
	CustomerID string `db:"customer_id"`
}

func (a *Activities) PatternDetectActivity(ctx context.Context, in cimodels.PatternDetectInput) (cimodels.PatternDetectOutput, error) {
	out := cimodels.PatternDetectOutput{
		Ran:            in.Settings.PatternDetectionEnabled,
		ExtractedAttrs: map[string]any{},
	}
	if !in.Settings.PatternDetectionEnabled {
		return out, nil
	}

	haystack := strings.TrimSpace(in.PreparedMessage.Subject + " " + in.PreparedMessage.Body)
	if match := firstRegexMatch(in.Settings.ContractNumberRegex, haystack); match != "" {
		out.ContractNumberMatch = &match
		out.ResolvedContractID = &match
		out.ExtractedAttrs["contractId"] = match
	}
	if match := firstRegexMatch(in.Settings.CustomerNumberRegex, haystack); match != "" {
		out.CustomerNumberMatch = &match
		out.ExtractedAttrs["customerId"] = match
	}

	if in.PreparedMessage.SenderEmail != "" {
		if match, err := a.cachedMatchByEmail(ctx, in.PreparedMessage.SenderEmail, in.Settings.ERPCacheTTLMinutes); err == nil && match != nil {
			out.SenderEmailMatch = match
			applyCachedMatch(out.ExtractedAttrs, match)
			if match.ContractID != nil && match.EntityID != "" && out.ResolvedContractID == nil {
				id := match.EntityID
				out.ResolvedContractID = &id
			}
		}
	}
	for _, email := range in.PreparedMessage.BodyEmails {
		match, err := a.cachedMatchByEmail(ctx, email, in.Settings.ERPCacheTTLMinutes)
		if err != nil || match == nil {
			continue
		}
		out.BodyEmailMatches = append(out.BodyEmailMatches, *match)
		applyCachedMatch(out.ExtractedAttrs, match)
		if match.ContractID != nil && match.EntityID != "" && out.ResolvedContractID == nil {
			id := match.EntityID
			out.ResolvedContractID = &id
		}
	}
	return out, nil
}

func firstRegexMatch(pattern, text string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || text == "" {
		return ""
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	return re.FindString(text)
}

func (a *Activities) cachedMatchByEmail(ctx context.Context, email string, ttlMinutes int) (*cimodels.CachedMatch, error) {
	if ttlMinutes <= 0 {
		return nil, nil
	}
	var customerRow freshCustomerEmailRow
	if err := a.q.FindFreshCustomerByEmail.GetContext(ctx, &customerRow, email, ttlMinutes); err == nil {
		return &cimodels.CachedMatch{
			Kind:       "customer",
			Email:      email,
			CustomerID: &customerRow.CustomerID,
			EntityID:   customerRow.CustomerID,
		}, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	var contractRow freshContractEmailRow
	if err := a.q.FindFreshContractByEmail.GetContext(ctx, &contractRow, email, ttlMinutes); err == nil {
		return &cimodels.CachedMatch{
			Kind:       "contract",
			Email:      email,
			CustomerID: &contractRow.CustomerID,
			ContractID: &contractRow.ContractID,
			EntityID:   contractRow.ContractID,
		}, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	return nil, nil
}

func applyCachedMatch(attrs map[string]any, match *cimodels.CachedMatch) {
	if match == nil {
		return
	}
	if match.Kind == "contract" && match.EntityID != "" {
		if _, exists := attrs["contractId"]; !exists {
			attrs["contractId"] = match.EntityID
		}
	}
	if match.Kind == "customer" && match.EntityID != "" {
		if _, exists := attrs["customerId"]; !exists {
			attrs["customerId"] = match.EntityID
		}
	}
	if match.Email != "" {
		if _, exists := attrs["email"]; !exists {
			attrs["email"] = match.Email
		}
	}
}
