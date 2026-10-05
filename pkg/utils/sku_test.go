package utils_test

import (
	"strings"
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/utils"
)

func TestGenerateSKU(t *testing.T) {
	sku1 := utils.GenerateSKU()
	sku2 := utils.GenerateSKU()

	if !strings.HasPrefix(sku1, "SKU-") {
		t.Errorf("expected SKU prefix SKU-, got %s", sku1)
	}

	if len(sku1) != 12 { // "SKU-" (4) + 8 chars = 12
		t.Errorf("expected SKU length 12, got %d (%s)", len(sku1), sku1)
	}

	if sku1 == sku2 {
		t.Errorf("expected generated SKUs to be distinct, got duplicates: %s", sku1)
	}
}
