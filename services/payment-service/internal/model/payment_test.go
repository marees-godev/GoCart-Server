package model

import "testing"

func TestNormalizePaymentMethod(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"NETBANKING", "NET_BANKING"},
		{"net_banking", "NET_BANKING"},
		{"NB", "NET_BANKING"},
		{"card", "CREDIT_CARD"},
		{"CREDIT_CARD", "CREDIT_CARD"},
		{"debitcard", "DEBIT_CARD"},
		{"upi", "UPI"},
		{"paypal", "PAYPAL"},
		{"wallet", "WALLET"},
		{"emi", "EMI"},
		{"mock", "MOCK"},
		{"CUSTOM_METHOD", "CUSTOM_METHOD"},
		{"", "CREDIT_CARD"},
	}

	for _, tt := range tests {
		got := NormalizePaymentMethod(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizePaymentMethod(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}
