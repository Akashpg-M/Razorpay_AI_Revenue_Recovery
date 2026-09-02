package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"revenue-recovery/backend/internal/config"
	"revenue-recovery/backend/internal/integrations/razorpay"
	"revenue-recovery/backend/internal/store"
)

func main() {
	create := flag.Bool("create-payment-link", false, "create or reuse the deterministic Test Mode payment link fixture")
	flag.Parse()
	cfg := config.Load()
	client := razorpay.NewClient(cfg.RazorpayAPIURL, cfg.RazorpayKeyID, cfg.RazorpayKeySecret)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status := client.CheckAuthentication(ctx)
	result := map[string]any{"configured": status.Configured, "mode": status.Mode, "reachable": status.Reachable, "authenticated": status.Authenticated, "authentication_http_status": status.HTTPStatus, "error_code": status.ErrorCode, "api_base_url": client.BaseURL(), "operation": "GET /v1/payments?count=1"}
	if *create {
		if !status.Authenticated || status.Mode != "test" {
			result["payment_link_error"] = "authenticated Test Mode credentials are required"
			write(result, 2)
		}
		db, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			result["payment_link_error"] = "database_unavailable"
			write(result, 2)
		}
		defer db.Close()
		repository := store.NewPostgres(db)
		actionID := "demo-razorpay-check-action-v1"
		link, exists, err := repository.GetPaymentLink(ctx, actionID)
		created := false
		createStatus := 0
		if err == nil && !exists {
			link, createStatus, err = client.CreatePaymentLinkWithStatus(ctx, razorpay.PaymentLinkRequest{Amount: 100, Currency: "INR", Description: "RecoverOS synthetic Test Mode integration check", ReferenceID: "recoveros_test_mode_check"})
			if err == nil {
				raw, _ := json.Marshal(link)
				err = repository.SavePaymentLink(ctx, actionID, link, raw)
				created = true
			}
		}
		if err != nil {
			result["payment_link_error"] = safeError(err)
			write(result, 2)
		}
		fetched, fetchStatus, err := client.FetchPaymentLinkWithStatus(ctx, link.ID)
		if err != nil {
			result["payment_link_error"] = safeError(err)
			write(result, 2)
		}
		result["payment_link_api"] = "POST /v1/payment_links then GET /v1/payment_links/{id}"
		result["payment_link_created"] = created
		result["payment_link_reused"] = exists
		result["create_http_status"] = createStatus
		result["fetch_http_status"] = fetchStatus
		result["provider_object_type"] = "payment_link"
		result["provider_reference_persisted"] = true
		result["fetch_reconciliation_successful"] = fetched.ID == link.ID
		result["provider_status"] = fetched.Status
	}
	write(result, 0)
}
func safeError(err error) string {
	if apiErr, ok := err.(*razorpay.APIError); ok {
		return fmt.Sprintf("provider_http_%d", apiErr.StatusCode)
	}
	return "integration_check_failed"
}
func write(value map[string]any, code int) {
	encoded, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(string(encoded))
	os.Exit(code)
}
