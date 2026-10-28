package mcp

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/advisor"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/aistate"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

var serverTools = []Tool{
	{
		Name:        "get_portfolio_overview",
		Description: "Retrieve high-level portfolio totals (Net Worth IDR/USD, Liquid Cash, Investments, Asset Allocation).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"date": map[string]interface{}{"type": "string", "description": "Optional snapshot date in YYYY-MM-DD"},
			},
		},
	},
	{
		Name:        "get_holdings_breakdown",
		Description: "Retrieve detailed holdings filtered by category, asset class, or source platform.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"category":    map[string]interface{}{"type": "string", "description": "Filter by category (e.g. SBN, Spot, Bank Account)"},
				"asset_class": map[string]interface{}{"type": "string", "description": "Filter by broad asset class (e.g. Equities, Crypto, Fixed Income)"},
				"source":      map[string]interface{}{"type": "string", "description": "Filter by source (ksei, debank, binance, sansfinance)"},
			},
		},
	},
	{
		Name:        "get_ai_state",
		Description: "Fetch token-efficient unified AI financial state JSON and sovereign runway digest.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "get_adhd_action_card",
		Description: "Get the zero-friction ADHD action card: immediate 1-step DCA deposit directive, dust sweeper, and SBN rollover.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "audit_portfolio",
		Description: "Run automated portfolio health check (allocation drift, risk alerts, clutter score, maturity horizon).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "get_rebalancing_plan",
		Description: "Simulate cash deposit allocation to eliminate asset allocation drift.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"monthly_deposit_idr": map[string]interface{}{"type": "number", "description": "Monthly deposit amount in IDR (default: 5,000,000)"},
			},
		},
	},
	{
		Name:        "get_sovereign_runway",
		Description: "Calculate 3-tier sovereign runway matrix (Base Operating Reserve, Fortress Buffer, Deployable Surplus).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"monthly_burn_idr": map[string]interface{}{"type": "number", "description": "Monthly living expenses in IDR (default: 5,350,000)"},
			},
		},
	},
	{
		Name:        "get_upcoming_cashflow",
		Description: "Retrieve upcoming SBN Sukuk guaranteed coupon payouts and principal maturity timeline.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
}

// Server handles JSON-RPC 2.0 stdio messages.
type Server struct {
	dataDir string
	dbConn  *sql.DB
}

func NewServer(dataDir string, dbConn *sql.DB) *Server {
	return &Server{
		dataDir: dataDir,
		dbConn:  dbConn,
	}
}

func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		lineStr := strings.TrimSpace(string(line))
		if len(lineStr) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(lineStr), &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(&req)
	}
}

func (s *Server) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.sendResult(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "portfolio-mcp",
				"version": "2.0.0",
			},
		})
	case "notifications/initialized":
		// No response required for notifications
	case "tools/list":
		s.sendResult(req.ID, map[string]interface{}{
			"tools": serverTools,
		})
	case "tools/call":
		var callParams struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			s.sendError(req.ID, -32602, "Invalid params")
			return
		}

		res, err := s.callTool(callParams.Name, callParams.Arguments)
		if err != nil {
			s.sendResult(req.ID, ToolResult{
				Content: []TextContent{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
				IsError: true,
			})
			return
		}
		s.sendResult(req.ID, ToolResult{
			Content: []TextContent{{Type: "text", Text: res}},
		})
	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})
	default:
		if req.ID != nil {
			s.sendError(req.ID, -32601, fmt.Sprintf("Method '%s' not found", req.Method))
		}
	}
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	b, _ := json.Marshal(resp)
	os.Stdout.Write(b)
	os.Stdout.WriteString("\n")
}

func (s *Server) sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	b, _ := json.Marshal(resp)
	os.Stdout.Write(b)
	os.Stdout.WriteString("\n")
}

func (s *Server) loadLatestSnapshot() (*models.Snapshot, error) {
	snapPath := filepath.Join(s.dataDir, "latest_snapshot.json")
	data, err := os.ReadFile(snapPath)
	if err != nil {
		return nil, fmt.Errorf("could not read latest_snapshot.json: %w", err)
	}
	var snap models.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("could not parse latest_snapshot.json: %w", err)
	}
	return &snap, nil
}

func (s *Server) callTool(name string, args map[string]interface{}) (string, error) {
	snap, err := s.loadLatestSnapshot()
	if err != nil {
		return "", err
	}

	switch name {
	case "get_portfolio_overview":
		overview := map[string]interface{}{
			"date":             snap.Metadata.Date,
			"exchange_rate":    snap.Metadata.ExchangeRate,
			"net_worth_idr":    snap.Totals.NetWorthIDR,
			"net_worth_usd":    snap.Totals.NetWorthUSD,
			"total_assets_idr": snap.Totals.TotalAssetsIDR,
			"bank_cash_idr":    snap.Totals.BankCashIDR,
			"investments_idr":  snap.Totals.InvestmentsIDR,
			"allocation":       snap.Allocation.ByAssetClass,
		}
		b, _ := json.MarshalIndent(overview, "", "  ")
		return string(b), nil

	case "get_holdings_breakdown":
		var filtered []models.Holding
		catFilter, _ := args["category"].(string)
		acFilter, _ := args["asset_class"].(string)
		srcFilter, _ := args["source"].(string)

		for _, h := range snap.AllHoldings {
			if catFilter != "" && !strings.EqualFold(h.Category, catFilter) {
				continue
			}
			if acFilter != "" && !strings.EqualFold(h.AssetClass, acFilter) {
				continue
			}
			if srcFilter != "" {
				src := strings.ToLower(srcFilter)
				hSrc := strings.ToLower(h.Source)
				if src == "debank" || src == "evm" {
					if hSrc != "debank" && hSrc != "evm wallet" {
						continue
					}
				} else if src == "alchemy" || src == "sol" || src == "solana" {
					if hSrc != "alchemy" && hSrc != "sol wallet" {
						continue
					}
				} else if !strings.EqualFold(h.Source, srcFilter) {
					continue
				}
			}
			filtered = append(filtered, h)
		}
		b, _ := json.MarshalIndent(filtered, "", "  ")
		return string(b), nil

	case "get_ai_state":
		statePath := filepath.Join(s.dataDir, "latest_ai_state.json")
		if data, err := os.ReadFile(statePath); err == nil {
			return string(data), nil
		}
		state, err := aistate.GenerateAIState(snap, s.dbConn)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(state, "", "  ")
		return string(b), nil

	case "get_adhd_action_card":
		state, err := aistate.GenerateAIState(snap, s.dbConn)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(state.ADHDFocusMetrics, "", "  ")
		return string(b), nil

	case "audit_portfolio":
		state, err := aistate.GenerateAIState(snap, s.dbConn)
		if err != nil {
			return "", err
		}
		audit := map[string]interface{}{
			"net_worth_idr":    snap.Totals.NetWorthIDR,
			"clutter_score":    state.ADHDFocusMetrics.ClutterAudit.ClutterScore,
			"clutter_rating":   state.ADHDFocusMetrics.ClutterAudit.ClutterRating,
			"fragmented_count": state.ADHDFocusMetrics.ClutterAudit.FragmentedCount,
			"dust_holdings":    state.ADHDFocusMetrics.ClutterAudit.DustHoldings,
			"directives":       state.ADHDFocusMetrics.ClutterAudit.Directives,
			"maturing_sukuk":   state.ADHDFocusMetrics.SBNReinvestmentPlaybook,
			"allocation_drift": state.AssetAllocation,
		}
		b, _ := json.MarshalIndent(audit, "", "  ")
		return string(b), nil

	case "get_rebalancing_plan":
		state, err := aistate.GenerateAIState(snap, s.dbConn)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(state.RebalancingPlan, "", "  ")
		return string(b), nil

	case "get_sovereign_runway":
		burn := 5350000.0
		if bVal, ok := args["monthly_burn_idr"].(float64); ok && bVal > 0 {
			burn = bVal
		}
		plan := advisor.ComputeSovereignPlan(snap.Totals.BankCashIDR, burn)
		b, _ := json.MarshalIndent(plan, "", "  ")
		return string(b), nil

	case "get_upcoming_cashflow":
		state, err := aistate.GenerateAIState(snap, s.dbConn)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(state.SukukAndCouponSchedule, "", "  ")
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}
