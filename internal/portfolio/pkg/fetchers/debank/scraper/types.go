package scraper

// WalletInfo represents high-level wallet net worth and daily change.
type WalletInfo struct {
	TotalNetWorth *string `json:"total_net_worth"`
	Change24h     *string `json:"change_24h"`
}

// SocialInfo holds DeBank social profile metrics.
type SocialInfo struct {
	Ranking   *string `json:"ranking"`
	Followers *string `json:"followers"`
	Following *string `json:"following"`
	TVF       *string `json:"tvf"`
}

// TokenItem represents an on-chain token holding.
type TokenItem struct {
	Symbol *string `json:"symbol"`
	Chain  *string `json:"chain"`
	Price  *string `json:"price"`
	Amount *string `json:"amount"`
	Value  *string `json:"value"`
}

// TokenBalance represents a token balance within a DeFi protocol pool.
type TokenBalance struct {
	Symbol  *string `json:"symbol"`
	Balance string  `json:"balance"`
}

// PositionItem represents a specific protocol position (e.g., Staked, Supplied, Farm).
type PositionItem struct {
	Type   string         `json:"type"`
	Pool   string         `json:"pool"`
	Value  string         `json:"value"`
	Tokens []TokenBalance `json:"tokens"`
}

// ProtocolItem represents a DeFi protocol holding.
type ProtocolItem struct {
	Name      string         `json:"name"`
	Value     *string        `json:"value"`
	Positions []PositionItem `json:"positions"`
}

// ScrapeResult represents the complete scraped DeBank payload matching the Python schema.
type ScrapeResult struct {
	Timestamp string         `json:"timestamp"`
	Wallet    WalletInfo     `json:"wallet"`
	Social    SocialInfo     `json:"social"`
	Tokens    []TokenItem    `json:"tokens"`
	Protocols []ProtocolItem `json:"protocols"`
	NFTs      []any          `json:"nfts"`
}
