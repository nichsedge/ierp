package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/commerce"
	"github.com/nichsedge/ierp/internal/config"
)

func handleInsertPay(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-pay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Account name (required)")
	cat := fs.String("category", "bank", "Category (bank, ewallet, crypto)")
	number := fs.String("number", "", "Account/wallet number")
	recipient := fs.String("recipient", "", "Beneficiary name")
	details := fs.String("details", "", "Details")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		fmt.Fprintf(stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("name is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := commerce.InsertPaymentAccount(database, commerce.PaymentAccount{
		Name:      *name,
		Category:  *cat,
		Number:    *number,
		Recipient: *recipient,
		Details:   *details,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sPayment destination #%d saved: %s%s\n", config.Green, id, *name, config.Reset)
	return nil
}

func handleListPay(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("pay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cat := fs.String("category", "", "Category")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	list, err := commerce.ListPaymentAccounts(database, *cat)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Payment Accounts (%d accounts) ===%s\n", config.Bold, len(list), config.Reset)
	for _, p := range list {
		fmt.Fprintf(stdout, "#%-3d | %-20s | %-10s | %s (%s)\n", p.ID, p.Name, p.Category, p.Number, p.Recipient)
	}
	return nil
}

func handleInsertReferral(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-referral", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Name/Service (required)")
	cat := fs.String("category", "general", "Category")
	code := fs.String("code", "", "Referral code")
	link := fs.String("link", "", "Referral link")
	benefit := fs.String("benefit", "", "User benefit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		fmt.Fprintf(stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("name is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := commerce.InsertReferral(database, commerce.Referral{
		Name:     *name,
		Category: *cat,
		Code:     *code,
		Link:     *link,
		Benefit:  *benefit,
		IsPublic: true,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sReferral #%d registered: %s%s\n", config.Green, id, *name, config.Reset)
	return nil
}

func handleListReferrals(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("referrals", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cat := fs.String("category", "", "Category")
	status := fs.String("status", "ACTIVE", "Status")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	list, err := commerce.ListReferrals(database, *cat, *status)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Affiliate Referrals (%d active) ===%s\n", config.Bold, len(list), config.Reset)
	for _, r := range list {
		fmt.Fprintf(stdout, "#%-3d | %-20s | Code: %-15s | %s\n", r.ID, r.Name, r.Code, r.Benefit)
	}
	return nil
}
