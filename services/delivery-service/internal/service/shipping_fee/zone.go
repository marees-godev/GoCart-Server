package shipping_fee

import (
	"strings"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

func ResolveZone(req *CalculationRequest) string {
	if req == nil {
		return ZoneRegional
	}

	cleanZone := strings.ToUpper(strings.TrimSpace(req.Zone))
	switch cleanZone {
	case ZoneIntraCity, "INTRACITY", "LOCAL", "SAME_CITY", "SAME-CITY":
		return ZoneIntraCity
	case ZoneTamilNadu, "TAMILNADU", "TAMIL NADU", "TN", "STATE", "SAME_STATE", "SAME-STATE":
		return ZoneTamilNadu
	case ZoneRegional, "NATIONAL", "INTERSTATE", "INTER-STATE", "INTER_STATE":
		return ZoneRegional
	}

	originCity := strings.ToLower(strings.TrimSpace(req.OriginCity))
	originState := strings.ToLower(strings.TrimSpace(req.OriginState))
	destCity := strings.ToLower(strings.TrimSpace(req.DestinationCity))
	destState := strings.ToLower(strings.TrimSpace(req.DestinationState))

	if (destCity == "" || destState == "") && strings.TrimSpace(req.ShippingAddress) != "" {
		addr := model.ParseShippingAddress(req.ShippingAddress)
		if destCity == "" {
			destCity = strings.ToLower(strings.TrimSpace(addr.City))
		}
		if destState == "" {
			destState = strings.ToLower(strings.TrimSpace(addr.State))
		}
	}

	if originCity != "" && destCity != "" && originCity == destCity {
		return ZoneIntraCity
	}

	isTN := func(state string) bool {
		s := strings.TrimSpace(strings.ToLower(state))
		return s == "tamil nadu" || s == "tamilnadu" || s == "tn"
	}

	if isTN(destState) && (isTN(originState) || originState == "") {
		return ZoneTamilNadu
	}

	if originState != "" && destState != "" && originState == destState {
		return ZoneTamilNadu
	}

	return ZoneRegional
}
