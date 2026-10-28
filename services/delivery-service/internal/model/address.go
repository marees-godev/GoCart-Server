package model

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	postalCodeRegex = regexp.MustCompile(`^\d{5,6}(-\d{4})?$`)
	trailingPinRegex = regexp.MustCompile(`(?i)\b(\d{5,6})\b$`)

	knownStates = map[string]string{
		"tamil nadu":     "Tamil Nadu",
		"tamilnadu":      "Tamil Nadu",
		"tn":             "Tamil Nadu",
		"kerala":         "Kerala",
		"kl":             "Kerala",
		"karnataka":      "Karnataka",
		"ka":             "Karnataka",
		"andhra pradesh": "Andhra Pradesh",
		"ap":             "Andhra Pradesh",
		"telangana":      "Telangana",
		"ts":             "Telangana",
		"maharashtra":    "Maharashtra",
		"mh":             "Maharashtra",
		"delhi":          "Delhi",
		"dl":             "Delhi",
		"gujarat":        "Gujarat",
		"gj":             "Gujarat",
		"rajasthan":      "Rajasthan",
		"rj":             "Rajasthan",
		"uttar pradesh":  "Uttar Pradesh",
		"up":             "Uttar Pradesh",
		"west bengal":    "West Bengal",
		"wb":             "West Bengal",
		"punjab":         "Punjab",
		"pb":             "Punjab",
		"haryana":        "Haryana",
		"hr":             "Haryana",
		"wa":             "WA",
		"washington":     "WA",
		"ca":             "CA",
		"california":     "CA",
		"ny":             "NY",
		"new york":       "NY",
		"tx":             "TX",
		"texas":          "TX",
		"or":             "OR",
		"oregon":         "OR",
		"fl":             "FL",
		"florida":        "FL",
		"il":             "IL",
		"illinois":       "IL",
	}
)

func ParseShippingAddress(raw string) ShippingAddress {
	var addr ShippingAddress
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return addr
	}

	// 1. Try direct JSON unmarshal
	if err := json.Unmarshal([]byte(trimmed), &addr); err == nil && (addr.City != "" || addr.Street != "") {
		// Valid direct struct match
	}

	// 2. Check JSON map for common aliases
	var rawMap map[string]any
	if err := json.Unmarshal([]byte(trimmed), &rawMap); err == nil {
		if addr.Street == "" {
			for _, k := range []string{"street", "address", "address_line", "addressLine", "line1"} {
				if v, ok := rawMap[k].(string); ok && strings.TrimSpace(v) != "" {
					addr.Street = strings.TrimSpace(v)
					break
				}
			}
		}
		if addr.City == "" {
			for _, k := range []string{"city", "district", "town"} {
				if v, ok := rawMap[k].(string); ok && strings.TrimSpace(v) != "" {
					addr.City = strings.TrimSpace(v)
					break
				}
			}
		}
		if addr.State == "" {
			for _, k := range []string{"state", "province", "region"} {
				if v, ok := rawMap[k].(string); ok && strings.TrimSpace(v) != "" {
					addr.State = strings.TrimSpace(v)
					break
				}
			}
		}
		if addr.PostalCode == "" {
			for _, k := range []string{"postal_code", "postalCode", "zip", "zipcode", "zip_code", "pincode", "pin"} {
				if v, ok := rawMap[k].(string); ok && strings.TrimSpace(v) != "" {
					addr.PostalCode = strings.TrimSpace(v)
					break
				}
			}
		}
		if addr.Country == "" {
			for _, k := range []string{"country", "nation"} {
				if v, ok := rawMap[k].(string); ok && strings.TrimSpace(v) != "" {
					addr.Country = strings.TrimSpace(v)
					break
				}
			}
		}
		if addr.ContactNumber == "" {
			for _, k := range []string{"contact_number", "contactNumber", "phone", "phone_number", "phoneNumber", "mobile"} {
				if v, ok := rawMap[k].(string); ok && strings.TrimSpace(v) != "" {
					addr.ContactNumber = strings.TrimSpace(v)
					break
				}
			}
		}
	} else if addr.Street == "" {
		addr.Street = trimmed
	}

	NormalizeAddressFromTokens(&addr)
	return addr
}

func NormalizeAddressFromTokens(addr *ShippingAddress) {
	if addr == nil || addr.Street == "" {
		return
	}

	parts := strings.Split(addr.Street, ",")
	var tokens []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tokens = append(tokens, t)
		}
	}

	if len(tokens) == 0 {
		return
	}

	lastIdx := len(tokens) - 1

	// Extract postal code if last token is numeric
	if addr.PostalCode == "" && postalCodeRegex.MatchString(tokens[lastIdx]) {
		addr.PostalCode = tokens[lastIdx]
		tokens = tokens[:lastIdx]
		lastIdx = len(tokens) - 1
	}

	if len(tokens) == 0 {
		return
	}

	// Check if last token is a known state
	if addr.State == "" {
		lastNorm := strings.ToLower(tokens[lastIdx])
		if st, ok := knownStates[lastNorm]; ok {
			addr.State = st
			tokens = tokens[:lastIdx]
			lastIdx = len(tokens) - 1
		}
	}

	if len(tokens) == 0 {
		return
	}

	// If city is still empty, the last remaining token is the city
	if addr.City == "" {
		candidate := tokens[lastIdx]
		// Avoid purely numeric candidates
		if !postalCodeRegex.MatchString(candidate) {
			addr.City = candidate
		}
	}
}

func InferCityStateFromWarehouses(addr *ShippingAddress, warehouses []*Warehouse) {
	if addr == nil || len(warehouses) == 0 {
		return
	}

	haystack := strings.ToLower(addr.Street + " " + addr.City)

	for _, w := range warehouses {
		if w == nil {
			continue
		}
		wCity := strings.TrimSpace(w.City)
		if wCity != "" && strings.Contains(haystack, strings.ToLower(wCity)) {
			if addr.City == "" || strings.EqualFold(addr.City, wCity) {
				addr.City = wCity
			}
			if addr.State == "" && strings.TrimSpace(w.State) != "" {
				addr.State = strings.TrimSpace(w.State)
			}
			return
		}
		wState := strings.TrimSpace(w.State)
		if wState != "" && strings.Contains(haystack, strings.ToLower(wState)) {
			if addr.State == "" {
				addr.State = wState
			}
		}
	}
}

