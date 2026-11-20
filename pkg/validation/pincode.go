package validation

import (
	"strings"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
)

// Valid 2-digit Indian Postal Circle prefixes defined by Department of Posts (India Post).
var validIndianPincodePrefixes = map[string]bool{
	// Zone 1: Northern
	"11": true, // Delhi (110001 - 110099)
	"12": true, // Haryana
	"13": true, // Haryana
	"14": true, // Punjab
	"15": true, // Punjab
	"16": true, // Chandigarh, Punjab
	"17": true, // Himachal Pradesh
	"18": true, // Jammu & Kashmir, Ladakh
	"19": true, // Jammu & Kashmir, Ladakh

	// Zone 2: Northern
	"20": true, // Uttar Pradesh
	"21": true, // Uttar Pradesh
	"22": true, // Uttar Pradesh
	"23": true, // Uttar Pradesh
	"24": true, // Uttar Pradesh, Uttarakhand
	"25": true, // Uttar Pradesh
	"26": true, // Uttar Pradesh, Uttarakhand
	"27": true, // Uttar Pradesh
	"28": true, // Uttar Pradesh

	// Zone 3: Western
	"30": true, // Rajasthan
	"31": true, // Rajasthan
	"32": true, // Rajasthan
	"33": true, // Rajasthan
	"34": true, // Rajasthan
	"36": true, // Gujarat
	"37": true, // Gujarat
	"38": true, // Gujarat
	"39": true, // Gujarat, Daman & Diu, Dadra & Nagar Haveli

	// Zone 4: Western
	"40": true, // Maharashtra, Goa
	"41": true, // Maharashtra
	"42": true, // Maharashtra
	"43": true, // Maharashtra
	"44": true, // Maharashtra
	"45": true, // Madhya Pradesh
	"46": true, // Madhya Pradesh
	"47": true, // Madhya Pradesh
	"48": true, // Madhya Pradesh
	"49": true, // Chhattisgarh

	// Zone 5: Southern
	"50": true, // Telangana
	"51": true, // Andhra Pradesh
	"52": true, // Andhra Pradesh
	"53": true, // Andhra Pradesh
	"56": true, // Karnataka
	"57": true, // Karnataka
	"58": true, // Karnataka
	"59": true, // Karnataka

	// Zone 6: Southern
	"60": true, // Tamil Nadu, Puducherry
	"61": true, // Tamil Nadu
	"62": true, // Tamil Nadu
	"63": true, // Tamil Nadu
	"64": true, // Tamil Nadu
	"67": true, // Kerala, Lakshadweep
	"68": true, // Kerala
	"69": true, // Kerala

	// Zone 7: Eastern
	"70": true, // West Bengal
	"71": true, // West Bengal
	"72": true, // West Bengal
	"73": true, // West Bengal, Sikkim
	"74": true, // West Bengal, Andaman & Nicobar
	"75": true, // Odisha
	"76": true, // Odisha
	"77": true, // Odisha
	"78": true, // Assam
	"79": true, // Arunachal Pradesh, Manipur, Meghalaya, Mizoram, Nagaland, Tripura

	// Zone 8: Eastern
	"80": true, // Bihar
	"81": true, // Bihar, Jharkhand
	"82": true, // Bihar, Jharkhand
	"83": true, // Jharkhand
	"84": true, // Bihar
	"85": true, // Bihar

	// Zone 9: APS / Field Post Office
	"90": true,
	"91": true,
	"92": true,
	"93": true,
	"94": true,
	"95": true,
	"96": true,
	"97": true,
	"98": true,
	"99": true,
}

// ValidateIndianPincode validates an Indian postal PIN code according to official India Post rules.
func ValidateIndianPincode(pincode string) error {
	trimmed := strings.TrimSpace(pincode)
	if trimmed == "" {
		return errors.BadRequest("PIN code is required")
	}

	for _, ch := range trimmed {
		if ch < '0' || ch > '9' {
			return errors.BadRequest("invalid PIN code: must contain only numbers")
		}
	}

	if len(trimmed) != 6 {
		return errors.BadRequest("invalid PIN code: Indian PIN code must be exactly 6 digits")
	}

	if trimmed[0] == '0' {
		return errors.BadRequest("invalid PIN code: Indian PIN code cannot start with 0")
	}

	// Check for repeating identical digits (e.g. 111111, 222222, ..., 999999)
	allSame := true
	for i := 1; i < len(trimmed); i++ {
		if trimmed[i] != trimmed[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return errors.BadRequest("invalid PIN code: repeating digits are not allowed (e.g. 111111)")
	}

	// Check for sequential dummy numbers (e.g. 123456, 654321)
	seqAsc := true
	seqDesc := true
	for i := 1; i < len(trimmed); i++ {
		if trimmed[i] != trimmed[i-1]+1 {
			seqAsc = false
		}
		if trimmed[i] != trimmed[i-1]-1 {
			seqDesc = false
		}
	}
	if seqAsc || seqDesc {
		return errors.BadRequest("invalid PIN code: sequential patterns are not allowed")
	}

	// Common fake repetitive patterns
	fakePatterns := map[string]bool{
		"121212": true,
		"101010": true,
		"123123": true,
	}
	if fakePatterns[trimmed] {
		return errors.BadRequest("invalid PIN code: dummy pattern not allowed")
	}

	// Validate 2-digit postal circle prefix
	prefix := trimmed[:2]
	if !validIndianPincodePrefixes[prefix] {
		return errors.BadRequest("invalid PIN code: invalid postal circle prefix")
	}

	// Specific check for Delhi (prefix 11): valid range is strictly 110001 - 110099
	if prefix == "11" {
		if !strings.HasPrefix(trimmed, "1100") || trimmed == "110000" {
			return errors.BadRequest("invalid PIN code: Delhi PIN codes must be in the range 110001 - 110099")
		}
	}

	return nil
}

// ValidatePostalCode validates a postal code for a specified country (or India by default).
func ValidatePostalCode(postalCode string, country string) error {
	trimmed := strings.TrimSpace(postalCode)
	if trimmed == "" {
		return errors.BadRequest("postal_code is required")
	}

	for _, ch := range trimmed {
		if ch < '0' || ch > '9' {
			return errors.BadRequest("invalid postal_code: must contain only numbers")
		}
	}

	normCountry := strings.ToUpper(strings.TrimSpace(country))
	if normCountry == "" || normCountry == "IN" || normCountry == "IND" || normCountry == "INDIA" {
		err := ValidateIndianPincode(trimmed)
		if err != nil {
			// Adapt error message key to postal_code for address context
			msg := err.(*errors.AppError).Message
			msg = strings.Replace(msg, "PIN code", "postal_code", -1)
			return errors.BadRequest(msg)
		}
		return nil
	}

	if len(trimmed) < 4 {
		return errors.BadRequest("invalid postal_code: must be at least 4 digits")
	}
	if len(trimmed) > 10 {
		return errors.BadRequest("invalid postal_code: must not exceed 10 digits")
	}

	allSame := true
	for i := 1; i < len(trimmed); i++ {
		if trimmed[i] != trimmed[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return errors.BadRequest("invalid postal_code: repeating dummy digits are not allowed")
	}

	return nil
}
