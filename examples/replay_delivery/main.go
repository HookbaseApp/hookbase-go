package main

import (
	"context"
	"fmt"
	"log"
	"os"

	hookbase "github.com/HookbaseApp/hookbase-go"
)

func main() {
	apiKey := os.Getenv("HOOKBASE_API_KEY")
	if apiKey == "" {
		log.Fatal("HOOKBASE_API_KEY environment variable is required")
	}

	client := hookbase.New(apiKey)
	ctx := context.Background()

	// List recent failed deliveries
	failed := hookbase.DeliveryFailed
	page, err := client.Deliveries.List(ctx, &hookbase.ListDeliveriesParams{
		Status: &failed,
		Limit:  hookbase.Ptr(5),
	})
	if err != nil {
		log.Fatalf("Failed to list deliveries: %v", err)
	}

	fmt.Printf("Found %d failed deliveries\n", len(page.Data))

	// Replay the first failed delivery
	if len(page.Data) > 0 {
		delivery := page.Data[0]
		fmt.Printf("Replaying delivery %s...\n", delivery.ID)

		result, err := client.Deliveries.Replay(ctx, delivery.ID)
		if err != nil {
			log.Fatalf("Failed to replay: %v", err)
		}

		fmt.Printf("Replayed: %s - %s\n", result.DeliveryID, result.Message)
	}
}
