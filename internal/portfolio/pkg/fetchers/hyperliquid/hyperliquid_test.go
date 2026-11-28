package hyperliquid

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchWithURLAndStandardize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode req: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req["type"] {
		case "userVaultEquities":
			_, _ = w.Write([]byte(`[{"vaultAddress":"0xabc","equity":"519.5623"},{"vaultAddress":"0xdef","equity":"1.50"}]`))
		case "clearinghouseState":
			_, _ = w.Write([]byte(`{"marginSummary":{"accountValue":"25.00"},"withdrawable":"25.00"}`))
		case "spotClearinghouseState":
			_, _ = w.Write([]byte(`{"balances":[{"coin":"USDC","token":0,"total":"10.50","hold":"0.0"}]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	raw, holdings, err := FetchWithURL(srv.URL, "0x1234")
	if err != nil {
		t.Fatalf("FetchWithURL failed: %v", err)
	}
	if len(raw.Vaults) != 2 {
		t.Fatalf("expected 2 raw vaults, got %d", len(raw.Vaults))
	}
	if len(holdings) != 3 {
		t.Fatalf("expected 3 holdings above threshold, got %d", len(holdings))
	}

	if holdings[0].Source != "hyperliquid" || holdings[0].Category != "Yield / LP" || *holdings[0].ValueUSD != 519.5623 {
		t.Errorf("unexpected vault holding: %+v", holdings[0])
	}
	if holdings[1].Category != "Stablecoin" || *holdings[1].ValueUSD != 25.0 {
		t.Errorf("unexpected perp holding: %+v", holdings[1])
	}
	if holdings[2].Category != "Stablecoin" || *holdings[2].ValueUSD != 10.5 {
		t.Errorf("unexpected spot holding: %+v", holdings[2])
	}
}
