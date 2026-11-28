package dashboard

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/nichsedge/ierp/internal/commerce"
	"github.com/nichsedge/ierp/internal/expenses"
	"github.com/nichsedge/ierp/internal/finance"
)

func (s *Server) handleRunway(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	runway, err := finance.ComputeRunway(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, runway)
}

func (s *Server) handleFinanceSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		snaps, err := finance.ListSnapshots(s.db, 30)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": snaps,
			"total": len(snaps),
		})
		return
	}

	if r.Method == http.MethodPost {
		var snap finance.Snapshot
		if err := decodeJSON(w, r, &snap); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := finance.InsertSnapshot(s.db, snap)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleFinanceCommitments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status := r.URL.Query().Get("status")
		cat := r.URL.Query().Get("category")
		comms, err := finance.ListCommitments(s.db, status, cat)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": comms,
			"total": len(comms),
		})
		return
	}

	if r.Method == http.MethodPost {
		var comm finance.Commitment
		if err := decodeJSON(w, r, &comm); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := finance.InsertCommitment(s.db, comm)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleExpenses(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		month := r.URL.Query().Get("month")
		cat := r.URL.Query().Get("category")
		tag := r.URL.Query().Get("tag")
		expType := r.URL.Query().Get("type")
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		items, err := expenses.ListExpenses(s.db, expenses.ListOptions{
			Limit:    limit,
			Category: cat,
			Tag:      tag,
			Type:     expType,
			Month:    month,
		})
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}

		summary, _ := expenses.GetSummary(s.db, month, "", "")

		sendJSON(w, http.StatusOK, map[string]any{
			"items":   items,
			"total":   len(items),
			"summary": summary,
		})
		return
	}

	if r.Method == http.MethodPost {
		var exp expenses.Expense
		if err := decodeJSON(w, r, &exp); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := expenses.InsertExpense(s.db, exp)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleExpenseItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		sendError(w, http.StatusBadRequest, "Missing expense ID")
		return
	}
	id, err := strconv.Atoi(parts[2])
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid expense ID")
		return
	}

	if r.Method == http.MethodDelete {
		ok, err := expenses.DeleteExpense(s.db, id)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			sendError(w, http.StatusNotFound, "Expense not found")
			return
		}
		sendJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleCommerceAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		list, err := commerce.ListPaymentAccounts(s.db, "")
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if q != "" {
			lq := strings.ToLower(q)
			var filtered []commerce.PaymentAccount
			for _, item := range list {
				if strings.Contains(strings.ToLower(item.Name), lq) ||
					strings.Contains(strings.ToLower(item.Category), lq) ||
					strings.Contains(strings.ToLower(item.Recipient), lq) ||
					strings.Contains(strings.ToLower(item.Number), lq) ||
					strings.Contains(strings.ToLower(item.Details), lq) {
					filtered = append(filtered, item)
				}
			}
			list = filtered
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var p commerce.PaymentAccount
		if err := decodeJSON(w, r, &p); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := commerce.InsertPaymentAccount(s.db, p)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleCommerceReferrals(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		list, err := commerce.ListReferrals(s.db, "", "")
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if q != "" {
			lq := strings.ToLower(q)
			var filtered []commerce.Referral
			for _, item := range list {
				if strings.Contains(strings.ToLower(item.Name), lq) ||
					strings.Contains(strings.ToLower(item.Category), lq) ||
					strings.Contains(strings.ToLower(item.Code), lq) ||
					strings.Contains(strings.ToLower(item.Benefit), lq) {
					filtered = append(filtered, item)
				}
			}
			list = filtered
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var ref commerce.Referral
		if err := decodeJSON(w, r, &ref); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := commerce.InsertReferral(s.db, ref)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}
